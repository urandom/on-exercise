package app

import (
	"context"
	"errors"

	"github.com/urandom/on-exercise/internal/state"
)

var errNotImplemented = errors.New("not implemented")

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

type App struct {
	store          store
	employeeLoader employeeLoader
	policyLoader   policyLoader
}

func New(store store, employeeLoader employeeLoader, policyLoader policyLoader) App {
	return App{
		store:          store,
		employeeLoader: employeeLoader,
		policyLoader:   policyLoader,
	}
}

// Run loads and validates all inputs, computes the plan and, if apply is set,
// applies it. It returns the run's ID, which is generated at the start of the
// run so that it is available even when the run fails.
func (a App) Run(ctx context.Context, apply bool) (string, error) {
	return "", errNotImplemented
}

// plan computes the ordered actions needed to bring the accounts in line with
// the HR snapshot and the policy. It performs no writes.
func (a App) plan(
	ctx context.Context, employees state.Employees, accounts state.Accounts,
	policies state.Policies,
) (state.Plan, error) {
	return state.Plan{}, errNotImplemented
}

// apply executes the plan in order through the store, stopping at the first
// failed write.
func (a App) apply(ctx context.Context, plan state.Plan) error {
	return errNotImplemented
}
