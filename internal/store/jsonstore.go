package store

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urandom/on-exercise/internal/jsonx"
	"github.com/urandom/on-exercise/internal/state"
)

var errNotImplemented = errors.New("not implemented")

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

// record is a single account as it appears in the application state file.
// Fields are pointers so that a missing or null key decodes to nil and can be
// told apart from a zero value.
type record struct {
	EmployeeID *state.EmployeeID `json:"employee_id"`
	Enabled    *bool             `json:"enabled"`
	Groups     *state.Groups     `json:"groups"`
}

// Load decodes the application state, a JSON array of accounts, into accounts
// keyed by employee ID.
func (s JSONStore) Load(ctx context.Context) (state.Accounts, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// TODO: export a sentinel error (e.g. ErrInvalidState) and wrap every
	// decoding and validation error with it, so callers can use errors.Is.
	var records []record
	if err := jsonx.DecodeStrict(s.f, &records); err != nil {
		return nil, fmt.Errorf("decoding app state %s: %w", s.f.Name(), err)
	}

	// An empty state file is valid: the application may have no accounts
	// yet.
	accounts := make(state.Accounts, len(records))
	for _, r := range records {
		// TODO: reject a record with any nil field (missing or null key).
		// Until then, value turns nil into the zero value, so a missing
		// "enabled" reads as false and a missing "groups" as no groups.
		// TODO: reject an empty employee_id.
		// TODO: reject empty group names and duplicate groups within one
		// account.
		// TODO: reject a duplicate employee_id before inserting; the map
		// would otherwise keep the last record silently.
		id := value(r.EmployeeID)
		accounts[id] = state.Account{
			EmployeeID: id,
			Enabled:    value(r.Enabled),
			Groups:     value(r.Groups),
		}
	}

	return accounts, nil
}

// value returns *p, or the zero value if p is nil.
func value[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
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
