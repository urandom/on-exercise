package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urandom/on-exercise/internal/jsonx"
	"github.com/urandom/on-exercise/internal/state"
)

var errNotImplemented = errors.New("not implemented")

// ErrInvalidState wraps every error caused by the content of the application
// state file, so callers can tell bad input apart from other failures.
var ErrInvalidState = errors.New("invalid app state")

// JSONStore is the application state adapter backed by a local JSON file.
//
// Load reads from the opened file. Writes replace the file at its path, so
// after the first write the opened handle refers to the previous contents;
// Load must therefore be called once, before any write.
type JSONStore struct {
	f *os.File
}

func NewJSONStore(f *os.File) JSONStore {
	return JSONStore{f: f}
}

type record struct {
	EmployeeID *state.EmployeeID `json:"employee_id"`
	Enabled    *bool             `json:"enabled"`
	Groups     *state.Groups     `json:"groups"`
}

// Load decodes the application state, a JSON array of accounts, into accounts
// keyed by employee ID. It rejects the whole state if any account is invalid,
// reporting every invalid account by position.
func (s JSONStore) Load(ctx context.Context) (state.Accounts, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var records []record
	if err := jsonx.DecodeStrict(s.f, &records); err != nil {
		return nil, fmt.Errorf("%w: decoding %s: %w", ErrInvalidState, s.f.Name(), err)
	}

	accounts := make(state.Accounts, len(records))
	var errs []error
	for i, r := range records {
		acc, err := r.account()
		if err != nil {
			errs = append(errs, fmt.Errorf("record %d: %w", i+1, err))
			continue
		}

		if _, dup := accounts[acc.EmployeeID]; dup {
			errs = append(errs, fmt.Errorf("record %d: duplicate employee_id %q", i+1, acc.EmployeeID))
			continue
		}

		accounts[acc.EmployeeID] = acc
	}

	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidState, s.f.Name(), err)
	}

	return accounts, nil
}

func (r record) account() (state.Account, error) {
	var missing []string
	if r.EmployeeID == nil {
		missing = append(missing, "employee_id")
	}
	if r.Enabled == nil {
		missing = append(missing, "enabled")
	}
	if r.Groups == nil {
		missing = append(missing, "groups")
	}
	if len(missing) > 0 {
		return state.Account{}, fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}

	id := *r.EmployeeID
	if id == "" {
		return state.Account{}, errors.New("empty employee_id")
	}

	seen := make(state.GroupSet, len(*r.Groups))
	for _, g := range *r.Groups {
		if g == "" {
			return state.Account{}, fmt.Errorf("account %q: empty group name", id)
		}
		if seen.Contains(g) {
			return state.Account{}, fmt.Errorf("account %q: duplicate group %q", id, g)
		}
		seen[g] = struct{}{}
	}

	return state.Account{
		EmployeeID: id,
		Enabled:    *r.Enabled,
		Groups:     *r.Groups,
	}, nil
}

func (s JSONStore) CreateAccount(ctx context.Context, id state.EmployeeID) error {
	return errNotImplemented
}

func (s JSONStore) SetEnabled(ctx context.Context, id state.EmployeeID, enabled bool) error {
	return errNotImplemented
}

func (s JSONStore) AddGroup(ctx context.Context, id state.EmployeeID, group state.Group) error {
	return errNotImplemented
}

func (s JSONStore) RemoveGroup(ctx context.Context, id state.EmployeeID, group state.Group) error {
	return errNotImplemented
}
