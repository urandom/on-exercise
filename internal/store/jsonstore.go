package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/urandom/on-exercise/internal/jsonx"
	"github.com/urandom/on-exercise/internal/state"
)

// ErrInvalidState wraps every error caused by the content of the application
// state file, so callers can tell bad input apart from other failures.
var ErrInvalidState = errors.New("invalid app state")

var (
	// ErrAccountExists is returned by CreateAccount for an existing ID, so
	// a retried create can never produce a duplicate account.
	ErrAccountExists = errors.New("account already exists")
	// ErrAccountNotFound is returned when a write targets a missing account.
	ErrAccountNotFound = errors.New("account not found")
	// ErrNotLoaded is returned by writes made before a successful Load.
	ErrNotLoaded = errors.New("state not loaded")
)

type record struct {
	EmployeeID *state.EmployeeID `json:"employee_id"`
	Enabled    *bool             `json:"enabled"`
	Groups     *state.Groups     `json:"groups"`
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

// JSONStore is the application state adapter backed by a local JSON file.
//
// Load reads the opened file and keeps the accounts in memory, in file order.
// Each write applies its change to a copy of them, saves the copy and only
// then keeps it, so the in-memory state always matches the file. This relies
// on a single reconciler running at a time: nothing else changes the file
// between writes.
//
// The store takes ownership of the opened file: Load reads it once and closes
// it, so no handle is held open when writes replace the file at its path
// (which Windows would refuse). Load must therefore be called exactly once,
// before any write.
type JSONStore struct {
	f *os.File
	// path is where writes save the state; it is the opened file's name.
	path string
	// accounts are the accounts in file order, set by Load and replaced
	// after every successful write. Nil until Load succeeds.
	accounts []state.Account
}

// NewJSONStore returns a store for the opened state file f. The store takes
// ownership of f and closes it in Load.
func NewJSONStore(f *os.File) *JSONStore {
	return &JSONStore{f: f, path: f.Name()}
}

// Load decodes the application state, a JSON array of accounts, into accounts
// keyed by employee ID. It rejects the whole state if any account is invalid,
// reporting every invalid account by position.
//
// Load closes the file whether or not it succeeds; a second call fails.
func (s *JSONStore) Load(ctx context.Context) (state.Accounts, error) {
	// For windows, otherwise writeAtomic would fail on the rename
	defer s.f.Close()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	list, err := decode(s.f, s.path)
	if err != nil {
		return nil, err
	}

	// Keep a non-nil slice even for an empty file, so writes can tell a
	// loaded empty state apart from no Load at all.
	if list == nil {
		list = []state.Account{}
	}
	s.accounts = list

	accounts := make(state.Accounts, len(list))
	for _, acc := range list {
		// Clone the groups so callers cannot change the store's copy.
		acc.Groups = slices.Clone(acc.Groups)
		accounts[acc.EmployeeID] = acc
	}

	return accounts, nil
}

// CreateAccount appends a disabled account with no groups. It fails with
// ErrAccountExists if the ID is already present.
func (s *JSONStore) CreateAccount(ctx context.Context, id state.EmployeeID) error {
	if s.accounts == nil {
		return ErrNotLoaded
	}
	if slices.ContainsFunc(s.accounts, func(acc state.Account) bool { return acc.EmployeeID == id }) {
		return fmt.Errorf("%w: %q", ErrAccountExists, id)
	}

	next := append(slices.Clone(s.accounts), state.Account{EmployeeID: id, Groups: state.Groups{}})

	return s.safeWrite(ctx, next)
}

// SetEnabled sets whether the account is enabled.
func (s *JSONStore) SetEnabled(ctx context.Context, id state.EmployeeID, enabled bool) error {
	next, acc, err := s.edit(id)
	if err != nil {
		return err
	}
	acc.Enabled = enabled

	return s.safeWrite(ctx, next)
}

// AddGroup adds group to the account. Adding a group the account already holds
// is an error.
func (s *JSONStore) AddGroup(ctx context.Context, id state.EmployeeID, group state.Group) error {
	next, acc, err := s.edit(id)
	if err != nil {
		return fmt.Errorf("adding group %q: %w", group, err)
	}
	if slices.Contains(acc.Groups, group) {
		return fmt.Errorf("account %q already holds group %q", id, group)
	}
	acc.Groups = append(acc.Groups, group)

	if err := s.safeWrite(ctx, next); err != nil {
		return fmt.Errorf("adding group %q: %w", group, err)
	}

	return nil
}

// RemoveGroup removes group from the account. Removing a group the account does
// not hold is an error.
func (s *JSONStore) RemoveGroup(ctx context.Context, id state.EmployeeID, group state.Group) error {
	next, acc, err := s.edit(id)
	if err != nil {
		return fmt.Errorf("removing group %q: %w", group, err)
	}
	i := slices.Index(acc.Groups, group)
	if i < 0 {
		return fmt.Errorf("account %q does not hold group %q", id, group)
	}
	acc.Groups = slices.Delete(acc.Groups, i, i+1)

	if err := s.safeWrite(ctx, next); err != nil {
		return fmt.Errorf("removing group %q: %w", group, err)
	}

	return nil
}

func (s *JSONStore) edit(id state.EmployeeID) ([]state.Account, *state.Account, error) {
	if s.accounts == nil {
		return nil, nil, ErrNotLoaded
	}

	i := slices.IndexFunc(s.accounts, func(acc state.Account) bool { return acc.EmployeeID == id })
	if i < 0 {
		return nil, nil, fmt.Errorf("%w: %q", ErrAccountNotFound, id)
	}

	next := slices.Clone(s.accounts)
	next[i].Groups = slices.Clone(next[i].Groups)

	return next, &next[i], nil
}

func (s *JSONStore) safeWrite(ctx context.Context, accounts []state.Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path := s.path
	data, err := encode(accounts)
	if err != nil {
		return err
	}
	if _, err := decode(bytes.NewReader(data), path); err != nil {
		return fmt.Errorf("refusing to write: %w", err)
	}

	if err := writeAtomic(path, data); err != nil {
		return err
	}
	s.accounts = accounts

	return nil
}

// writeAtomic replaces the file at path with data, keeping its permissions.
// It writes a temporary file in the same directory, syncs it, renames it over
// path and syncs the directory so the rename itself is durable.
//
// An error after the rename means the new contents may already be in place.
// It is still reported as a failure: a write must never look successful when
// it may not be durable, and the next run re-plans from whatever the file
// holds.
func writeAtomic(path string, data []byte) (err error) {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return err
	}

	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()

	return d.Sync()
}

func decode(r io.Reader, name string) ([]state.Account, error) {
	var records []record
	if err := jsonx.DecodeStrict(r, &records); err != nil {
		return nil, fmt.Errorf("%w: decoding %s: %w", ErrInvalidState, name, err)
	}

	accounts := make([]state.Account, 0, len(records))
	seen := make(map[state.EmployeeID]struct{}, len(records))
	var errs []error
	for i, r := range records {
		acc, err := r.account()
		if err != nil {
			errs = append(errs, fmt.Errorf("record %d: %w", i+1, err))
			continue
		}

		if _, dup := seen[acc.EmployeeID]; dup {
			errs = append(errs, fmt.Errorf("record %d: duplicate employee_id %q", i+1, acc.EmployeeID))
			continue
		}
		seen[acc.EmployeeID] = struct{}{}

		accounts = append(accounts, acc)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidState, name, err)
	}

	return accounts, nil
}

func encode(accounts []state.Account) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("[")
	for i, acc := range accounts {
		// A nil slice would encode as null, which decode rejects.
		groups := acc.Groups
		if groups == nil {
			groups = state.Groups{}
		}

		line, err := json.Marshal(record{
			EmployeeID: &acc.EmployeeID,
			Enabled:    &acc.Enabled,
			Groups:     &groups,
		})
		if err != nil {
			return nil, fmt.Errorf("encoding account %q: %w", acc.EmployeeID, err)
		}

		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString("\n  ")
		buf.Write(line)
	}
	buf.WriteString("\n]\n")

	return buf.Bytes(), nil
}
