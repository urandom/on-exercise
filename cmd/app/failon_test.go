package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/urandom/on-exercise/internal/state"
	"github.com/urandom/on-exercise/internal/store"
)

const initial = `[
  {"employee_id": "E001", "enabled": true, "groups": ["retail"]},
  {"employee_id": "E002", "enabled": true, "groups": []}
]
`

func newFailingStore(t *testing.T, target string) (failingStore, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })

	s := failingStore{JSONStore: store.NewJSONStore(f), target: state.EmployeeID(target)}
	if _, err := s.Load(t.Context()); err != nil {
		t.Fatal(err)
	}

	return s, path
}

func TestFailingStoreFailsOnlyTheTarget(t *testing.T) {
	s, path := newFailingStore(t, "E001")

	// Every kind of write for the target fails and changes nothing.
	writes := map[string]func() error{
		"create":       func() error { return s.CreateAccount(t.Context(), "E001") },
		"disable":      func() error { return s.SetEnabled(t.Context(), "E001", false) },
		"add_group":    func() error { return s.AddGroup(t.Context(), "E001", "finance") },
		"remove_group": func() error { return s.RemoveGroup(t.Context(), "E001", "retail") },
	}
	for name, write := range writes {
		if err := write(); !errors.Is(err, errInjected) {
			t.Errorf("%s: error = %v, want %v", name, err, errInjected)
		}
	}
	if got, _ := os.ReadFile(path); string(got) != initial {
		t.Errorf("failed writes changed the file:\n%s", got)
	}

	// Writes for other employees go through.
	if err := s.AddGroup(t.Context(), "E002", "finance"); err != nil {
		t.Errorf("AddGroup(E002) error = %v, want nil", err)
	}
}

func TestFailingStoreWithoutTargetPassesThrough(t *testing.T) {
	s, _ := newFailingStore(t, "")

	if err := s.SetEnabled(t.Context(), "E001", false); err != nil {
		t.Errorf("SetEnabled() error = %v, want nil", err)
	}
	if err := s.CreateAccount(t.Context(), "E003"); err != nil {
		t.Errorf("CreateAccount() error = %v, want nil", err)
	}
}
