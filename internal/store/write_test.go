package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/urandom/on-exercise/internal/state"
)

const initial = `[
  {"employee_id": "E002", "enabled": true, "groups": ["retail", "wiki"]},
  {"employee_id": "E900", "enabled": true, "groups": ["admin", "finance"]},
  {"employee_id": "E001", "enabled": false, "groups": []}
]
`

// newLoadedStore writes content to a state file with the given permissions
// and returns a store that has loaded it.
func newLoadedStore(t *testing.T, content string, perm os.FileMode) (*JSONStore, string) {
	t.Helper()

	s, path := newStore(t, content, perm)
	if _, err := s.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	return s, path
}

func newStore(t *testing.T, content string, perm os.FileMode) (*JSONStore, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })

	return NewJSONStore(f), path
}

func readAccounts(t *testing.T, path string) []state.Account {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	accounts, err := decode(f, path)
	if err != nil {
		t.Fatalf("state file no longer loads: %v", err)
	}

	return accounts
}

func assertAccounts(t *testing.T, got, want []state.Account) {
	t.Helper()

	if !slices.EqualFunc(got, want, func(a, b state.Account) bool {
		return a.EmployeeID == b.EmployeeID && a.Enabled == b.Enabled && slices.Equal(a.Groups, b.Groups)
	}) {
		t.Errorf("accounts =\n\t%+v\nwant\n\t%+v", got, want)
	}
}

func assertNoTempFiles(t *testing.T, path string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Errorf("leftover file %s", e.Name())
		}
	}
}

func TestWritesKeepOrderAndUntouchedAccounts(t *testing.T) {
	s, path := newLoadedStore(t, initial, 0o640)
	ctx := t.Context()

	// A sequence like a real apply: each write must build on the previous.
	steps := []func() error{
		func() error { return s.SetEnabled(ctx, "E002", false) },
		func() error { return s.RemoveGroup(ctx, "E002", "retail") },
		func() error { return s.CreateAccount(ctx, "E003") },
		func() error { return s.AddGroup(ctx, "E003", "finance") },
		func() error { return s.SetEnabled(ctx, "E003", true) },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	want := []state.Account{
		{EmployeeID: "E002", Enabled: false, Groups: state.Groups{"wiki"}},
		{EmployeeID: "E900", Enabled: true, Groups: state.Groups{"admin", "finance"}},
		{EmployeeID: "E001", Enabled: false, Groups: state.Groups{}},
		{EmployeeID: "E003", Enabled: true, Groups: state.Groups{"finance"}},
	}
	assertAccounts(t, readAccounts(t, path), want)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Errorf("permissions = %o, want 640", perm)
	}
	assertNoTempFiles(t, path)
}

func TestFailedWriteLeavesFileAndMemoryUntouched(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		write   func(*JSONStore) error
		wantErr error // nil: any error
	}{
		{
			name:    "create existing account",
			write:   func(s *JSONStore) error { return s.CreateAccount(context.Background(), "E002") },
			wantErr: ErrAccountExists,
		},
		{
			name:    "write to missing account",
			write:   func(s *JSONStore) error { return s.SetEnabled(context.Background(), "E404", true) },
			wantErr: ErrAccountNotFound,
		},
		{
			name:  "add group already held",
			write: func(s *JSONStore) error { return s.AddGroup(context.Background(), "E002", "retail") },
		},
		{
			name:  "remove group not held",
			write: func(s *JSONStore) error { return s.RemoveGroup(context.Background(), "E002", "finance") },
		},
		{
			name:    "context cancelled",
			write:   func(s *JSONStore) error { return s.RemoveGroup(cancelled, "E002", "retail") },
			wantErr: context.Canceled,
		},
		{
			name: "result would not load",
			write: func(s *JSONStore) error {
				return s.safeWrite(context.Background(), append(slices.Clone(s.accounts), s.accounts[0]))
			},
			wantErr: ErrInvalidState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, path := newLoadedStore(t, initial, 0o600)
			before := slices.Clone(s.accounts)

			err := tt.write(s)
			if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("write error = %v, want %v", err, tt.wantErr)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != initial {
				t.Errorf("state file changed:\n%s", got)
			}
			assertAccounts(t, s.accounts, before)
			assertNoTempFiles(t, path)
		})
	}
}

func TestFailedFileWriteLeavesMemoryUntouched(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	s, path := newLoadedStore(t, initial, 0o600)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	if err := s.RemoveGroup(t.Context(), "E002", "retail"); err == nil {
		t.Fatal("RemoveGroup() succeeded in a read-only directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	// The next write must start from the saved state, without the failed
	// removal.
	if err := s.SetEnabled(t.Context(), "E001", true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	want := []state.Account{
		{EmployeeID: "E002", Enabled: true, Groups: state.Groups{"retail", "wiki"}},
		{EmployeeID: "E900", Enabled: true, Groups: state.Groups{"admin", "finance"}},
		{EmployeeID: "E001", Enabled: true, Groups: state.Groups{}},
	}
	assertAccounts(t, readAccounts(t, path), want)
}

func TestWriteBeforeLoad(t *testing.T) {
	s, _ := newStore(t, initial, 0o600)

	if err := s.CreateAccount(t.Context(), "E003"); !errors.Is(err, ErrNotLoaded) {
		t.Errorf("CreateAccount() error = %v, want %v", err, ErrNotLoaded)
	}
	if err := s.SetEnabled(t.Context(), "E002", false); !errors.Is(err, ErrNotLoaded) {
		t.Errorf("SetEnabled() error = %v, want %v", err, ErrNotLoaded)
	}
}

func TestLoadedEmptyStateAcceptsCreate(t *testing.T) {
	s, path := newLoadedStore(t, "[]", 0o600)

	if err := s.CreateAccount(t.Context(), "E001"); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	assertAccounts(t, readAccounts(t, path), []state.Account{{EmployeeID: "E001", Groups: state.Groups{}}})
}

func TestLoadClosesFile(t *testing.T) {
	s, _ := newLoadedStore(t, initial, 0o600)

	if _, err := s.f.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
		t.Errorf("reading the file after Load: error = %v, want %v", err, os.ErrClosed)
	}
	if _, err := s.Load(t.Context()); err == nil {
		t.Error("second Load() succeeded, want error")
	}
	// Writes go to the path, so they still work once the handle is closed.
	if err := s.SetEnabled(t.Context(), "E001", true); err != nil {
		t.Errorf("SetEnabled() after Load error = %v", err)
	}
}

func TestFailedLoadClosesFile(t *testing.T) {
	s, _ := newStore(t, "not json", 0o600)

	if _, err := s.Load(t.Context()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Load() error = %v, want %v", err, ErrInvalidState)
	}
	if _, err := s.f.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
		t.Errorf("reading the file after a failed Load: error = %v, want %v", err, os.ErrClosed)
	}
}

func TestGroupErrorsNameTheGroup(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		write   func(*JSONStore) error
		wantErr error
	}{
		{"add to missing account", func(s *JSONStore) error { return s.AddGroup(context.Background(), "E404", "finance") }, ErrAccountNotFound},
		{"remove from missing account", func(s *JSONStore) error { return s.RemoveGroup(context.Background(), "E404", "finance") }, ErrAccountNotFound},
		{"add already held", func(s *JSONStore) error { return s.AddGroup(context.Background(), "E900", "finance") }, nil},
		{"remove not held", func(s *JSONStore) error { return s.RemoveGroup(context.Background(), "E001", "finance") }, nil},
		{"add, write fails", func(s *JSONStore) error { return s.AddGroup(cancelled, "E001", "finance") }, context.Canceled},
		{"remove, write fails", func(s *JSONStore) error { return s.RemoveGroup(cancelled, "E900", "finance") }, context.Canceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newLoadedStore(t, initial, 0o600)

			err := tt.write(s)
			if err == nil {
				t.Fatal("write succeeded, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want it to wrap %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), `"finance"`) {
				t.Errorf("error = %q, want it to name the group", err)
			}
		})
	}
}
