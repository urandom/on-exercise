package app

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/urandom/on-exercise/internal/state"
)

var errNotImplemented = errors.New("not implemented")

// ErrUnmappedDep is returned when an active employee's department has no
// entry in the policy. The run is rejected before any write.
var ErrUnmappedDep = errors.New("active employees with unmapped departments")

type store interface {
	Load(context.Context) (state.Accounts, error)
	CreateAccount(context.Context, state.EmployeeID) error
	SetEnabled(context.Context, state.EmployeeID, bool) error
	AddGroup(context.Context, state.EmployeeID, state.Group) error
	RemoveGroup(context.Context, state.EmployeeID, state.Group) error
}

type employeeLoader interface {
	Load(context.Context) (state.Employees, error)
}

type policyLoader interface {
	Load(context.Context) (state.Policy, error)
}

type Result struct {
	ID   string
	Plan state.Plan
}

type App struct {
	store     store
	employees employeeLoader
	policy    policyLoader

	// runLog records planned actions, write outcomes and orphans. Every
	// record carries the run ID and employee ID.
	runLog *slog.Logger
}

// New creates an App that records planned actions, write outcomes and
// orphans to runLog.
func New(
	store store, employeeLoader employeeLoader, policyLoader policyLoader,
	runLog *slog.Logger,
) App {
	return App{
		store:     store,
		employees: employeeLoader,
		policy:    policyLoader,
		runLog:    runLog,
	}
}

// Run loads and validates all inputs, computes the plan and, if apply is set,
// applies it. It returns the run's ID, which is generated at the start of the
// run so that it is available even when the run fails.
//
// Every run is recorded in the run log under its run ID: the planned
// actions and orphans before any write, then, when applying, the outcome of
// each action (succeeded, failed or not_attempted), and finally how the run
// ended.
func (a App) Run(ctx context.Context, apply bool) (Result, error) {
	res := Result{ID: newRunID()}
	runLog := a.runLog.With(slog.String("run_id", res.ID))

	mode := "plan"
	if apply {
		mode = "apply"
	}
	runLog.Info("run_started", slog.String("mode", mode))

	accounts, employees, policy, err := a.loadAndValidateData(ctx)
	if err != nil {
		runLog.Error("run_rejected", slog.String("error", err.Error()))
		return res, err
	}

	res.Plan = a.plan(employees, accounts, policy)
	logPlan(runLog, res.Plan)

	if !apply {
		runLog.Info("run_finished", slog.String("result", "planned"))
		return res, nil
	}

	if err := a.apply(ctx, runLog, res.Plan); err != nil {
		// The error itself is on the failed outcome record, tied to its
		// action; repeating it here would count each failure twice.
		runLog.Error("run_finished", slog.String("result", "failed"))
		return res, err
	}
	runLog.Info("run_finished", slog.String("result", "applied"))

	return res, nil
}

func logPlan(log *slog.Logger, plan state.Plan) {
	for _, act := range plan.Actions {
		log.Info("planned", slog.Any("action", act))
	}
	for _, id := range plan.Orphans {
		log.Warn("orphan", slog.String("employee_id", string(id)))
	}
}

func (a App) loadAndValidateData(ctx context.Context) (state.Accounts, state.Employees, state.Policy, error) {
	accounts, err := a.store.Load(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading app state: %w", err)
	}

	employees, err := a.employees.Load(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading employees: %w", err)
	}

	policy, err := a.policy.Load(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading policy: %w", err)
	}

	err = validateEmployeeDepartments(employees, policy)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("validating employee departments: %w", err)
	}

	return accounts, employees, policy, nil
}

func validateEmployeeDepartments(employees state.Employees, policy state.Policy) error {
	var unmappedDepEmployees []state.EmployeeID
	for id, employee := range employees {
		if employee.Status != state.StatusActive {
			continue
		}

		_, mappedDep := policy[employee.Department]
		if !mappedDep {
			unmappedDepEmployees = append(unmappedDepEmployees, id)
		}
	}

	if len(unmappedDepEmployees) > 0 {
		slices.Sort(unmappedDepEmployees)

		return fmt.Errorf("%w: %q", ErrUnmappedDep, unmappedDepEmployees)
	}

	return nil
}

// plan computes the ordered actions needed to bring the accounts in line with
// the HR snapshot and the policy. It performs no writes.
func (a App) plan(
	employees state.Employees, accounts state.Accounts,
	policy state.Policy,
) state.Plan {
	plan := state.Plan{}
	allManagedGroups := policy.ManagedGroups()

	for id, employee := range employees {
		managedGroups := policy[employee.Department]

		acc, foundAcc := accounts[id]
		if !foundAcc {
			if employee.Status != state.StatusActive {
				continue
			}

			plan.Actions = append(plan.Actions, state.Action{
				Op:         state.OpCreate,
				EmployeeID: id,
			})
		}

		if acc.Enabled && employee.Status != state.StatusActive {
			plan.Actions = append(plan.Actions, state.Action{
				Op:         state.OpDisable,
				EmployeeID: id,
			})
		} else if !acc.Enabled && employee.Status == state.StatusActive {
			plan.Actions = append(plan.Actions, state.Action{
				Op:         state.OpEnable,
				EmployeeID: id,
			})
		}

		// Leavers need to be stripped of all managed groups, so their entitled
		// set stays empty. They also don't get any new ones.
		var emplSet state.GroupSet
		accSet := acc.Groups.Set()
		if employee.Status == state.StatusActive {
			emplSet = managedGroups.Set()

			for _, grp := range managedGroups {
				if !accSet.Contains(grp) {
					plan.Actions = append(plan.Actions, state.Action{
						Op:         state.OpAddGroup,
						EmployeeID: id,
						Group:      grp,
					})
				}
			}
		}

		for _, grp := range acc.Groups {
			if allManagedGroups.Contains(grp) && !emplSet.Contains(grp) {
				plan.Actions = append(plan.Actions, state.Action{
					Op:         state.OpRemoveGroup,
					EmployeeID: id,
					Group:      grp,
				})
			}
		}
	}

	for id := range accounts {
		if _, found := employees[id]; found {
			continue
		}

		plan.Orphans = append(plan.Orphans, id)
	}

	// opRank ensures that actions are applied in order of revocation first,
	// then grants. Disabling is first to cut off access immediately. Removal of
	// managed groups is next, followed by creating a disabled account, then
	// adding managed groups. Enabling is last. This way an account cannot be
	// live without its groups already applied.
	opRank := map[state.Op]int{
		state.OpDisable:     0,
		state.OpRemoveGroup: 1,
		state.OpCreate:      2,
		state.OpAddGroup:    3,
		state.OpEnable:      4,
	}

	slices.SortFunc(plan.Actions, func(a, b state.Action) int {
		return cmp.Or(
			cmp.Compare(opRank[a.Op], opRank[b.Op]),
			cmp.Compare(a.EmployeeID, b.EmployeeID),
			cmp.Compare(a.Group, b.Group),
		)
	})
	slices.Sort(plan.Orphans)

	return plan
}

// apply executes the plan in order through the store, stopping at the first
// failed write.
func (a App) apply(ctx context.Context, log *slog.Logger, plan state.Plan) error {
	for i, act := range plan.Actions {
		if err := a.write(ctx, act); err != nil {
			// The outcome is logged from the write's own error, so a failed
			// write can never be recorded as a success. Everything after it
			// is recorded as not attempted rather than left unaccounted for.
			log.Error("outcome",
				slog.Any("action", act),
				slog.String("result", resultFailed),
				slog.String("error", err.Error()),
			)
			for _, rest := range plan.Actions[i+1:] {
				log.Warn("outcome", slog.Any("action", rest), slog.String("result", resultNotAttempted))
			}

			return fmt.Errorf("applying plan for %q and op %q: %w", act.EmployeeID, act.Op, err)
		}

		log.Info("outcome", slog.Any("action", act), slog.String("result", resultSucceeded))
	}

	return nil
}

// Outcome results recorded for each planned action during apply.
const (
	resultSucceeded    = "succeeded"
	resultFailed       = "failed"
	resultNotAttempted = "not_attempted"
)

// write performs a single action through the store.
func (a App) write(ctx context.Context, act state.Action) error {
	switch act.Op {
	case state.OpDisable:
		return a.store.SetEnabled(ctx, act.EmployeeID, false)
	case state.OpRemoveGroup:
		return a.store.RemoveGroup(ctx, act.EmployeeID, act.Group)
	case state.OpCreate:
		return a.store.CreateAccount(ctx, act.EmployeeID)
	case state.OpAddGroup:
		return a.store.AddGroup(ctx, act.EmployeeID, act.Group)
	case state.OpEnable:
		return a.store.SetEnabled(ctx, act.EmployeeID, true)
	default:
		return errors.New("unknown op")
	}
}

// newRunID returns a random 128-bit run ID as a hex string.
func newRunID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
