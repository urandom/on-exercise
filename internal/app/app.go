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
	Load(context.Context) (state.Policies, error)
}

type Result struct {
	ID   string
	Plan state.Plan
}

type App struct {
	store     store
	employees employeeLoader
	policies  policyLoader

	// auditLog records planned actions, write outcomes and orphans. Every
	// record carries the run ID and employee ID.
	auditLog *slog.Logger
}

// New creates an App that records planned actions, write outcomes and
// orphans to auditLog.
func New(
	store store, employeeLoader employeeLoader, policyLoader policyLoader,
	auditLog *slog.Logger,
) App {
	return App{
		store:     store,
		employees: employeeLoader,
		policies:  policyLoader,
		auditLog:  auditLog,
	}
}

// Run loads and validates all inputs, computes the plan and, if apply is set,
// applies it. It returns the run's ID, which is generated at the start of the
// run so that it is available even when the run fails.
func (a App) Run(ctx context.Context, apply bool) (Result, error) {
	res := Result{ID: newRunID()}

	accounts, employees, policies, err := a.loadAndValidateData(ctx)
	if err != nil {
		return res, err
	}

	res.Plan = a.plan(employees, accounts, policies)

	if !apply {
		return res, nil
	}

	return res, a.apply(ctx, res.Plan)
}

func (a App) loadAndValidateData(ctx context.Context) (state.Accounts, state.Employees, state.Policies, error) {
	accounts, err := a.store.Load(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading app state: %w", err)
	}

	employees, err := a.employees.Load(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading employees: %w", err)
	}

	policies, err := a.policies.Load(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading policies: %w", err)
	}

	err = validateEmployeeDepartments(employees, policies)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("validating employee departments: %w", err)
	}

	return accounts, employees, policies, nil
}

func validateEmployeeDepartments(employees state.Employees, policies state.Policies) error {
	var unmappedDepEmployees []state.EmployeeID
	for id, employee := range employees {
		if employee.Status != state.StatusActive {
			continue
		}

		_, mappedDep := policies[employee.Department]
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
	policies state.Policies,
) state.Plan {
	plan := state.Plan{}
	allManagedGroups := policies.ManagedGroups()

	for id, employee := range employees {
		managedGroups := policies[employee.Department]

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
func (a App) apply(ctx context.Context, plan state.Plan) error {
	return errNotImplemented
}

// newRunID returns a random 128-bit run ID as a hex string.
func newRunID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
