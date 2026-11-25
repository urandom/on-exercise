package store

import (
	"context"
	"errors"
	"os"

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

func (s JSONStore) Load(ctx context.Context) (state.Accounts, error) {
	return nil, errNotImplemented
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
