package app_test

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/urandom/on-exercise/internal/app"
	"github.com/urandom/on-exercise/internal/hr"
	"github.com/urandom/on-exercise/internal/policy"
	"github.com/urandom/on-exercise/internal/state"
	"github.com/urandom/on-exercise/internal/store"
)

// Each directory in testdata is one scenario with hr.json, app.json and
// policy.json inputs. want.json is the expected application state after a
// successful apply; after_*.json files hold the expected state after an
// injected write failure in TestRunRecovery.
//
//go:embed testdata
var fixtures embed.FS

var errInjected = errors.New("injected write failure")

// unchanged is used as wantState to assert that the application file is
// byte-for-byte identical to its state before the run.
const unchanged = "app.json"

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		// dir is the fixture directory; it defaults to name.
		dir     string
		apply   bool
		wantErr bool
		// wantWrites are the writes Run must attempt, in order.
		wantWrites []state.Action
		// wantState names the fixture the application file must match after
		// the run.
		wantState string
		// forbidden lists managed groups each account must not hold after the
		// run.
		forbidden map[state.EmployeeID][]state.Group
	}{
		// Joiners, movers and leavers.
		{
			name:  "joiner gets a new enabled account with department groups",
			dir:   "joiner",
			apply: true,
			wantWrites: []state.Action{
				create("E001"), add("E001", "retail"), enable("E001"),
			},
			wantState: "want.json",
			forbidden: map[state.EmployeeID][]state.Group{"E001": {"finance"}},
		},
		{
			name:       "rehire reuses the disabled account and keeps unmanaged groups",
			dir:        "rehire",
			apply:      true,
			wantWrites: []state.Action{add("E001", "finance"), enable("E001")},
			wantState:  "want.json",
			forbidden:  map[state.EmployeeID][]state.Group{"E001": {"retail"}},
		},
		{
			name:       "mover loses old department group before gaining the new one",
			dir:        "mover",
			apply:      true,
			wantWrites: []state.Action{remove("E001", "retail"), add("E001", "finance")},
			wantState:  "want.json",
			forbidden:  map[state.EmployeeID][]state.Group{"E001": {"retail"}},
		},
		{
			name:  "leaver is disabled before all managed groups are removed",
			dir:   "leaver",
			apply: true,
			wantWrites: []state.Action{
				disable("E001"), remove("E001", "finance"), remove("E001", "retail"),
			},
			wantState: "want.json",
			forbidden: map[state.EmployeeID][]state.Group{"E001": {"finance", "retail"}},
		},
		{
			name:       "already disabled leaver still loses leftover managed groups",
			dir:        "leaver_already_disabled",
			apply:      true,
			wantWrites: []state.Action{remove("E001", "finance")},
			wantState:  "want.json",
			forbidden:  map[state.EmployeeID][]state.Group{"E001": {"finance", "retail"}},
		},
		{
			name:       "leaver from an unmapped department is still offboarded",
			dir:        "leaver_unmapped_department",
			apply:      true,
			wantWrites: []state.Action{disable("E001"), remove("E001", "retail")},
			wantState:  "want.json",
			forbidden:  map[state.EmployeeID][]state.Group{"E001": {"finance", "retail"}},
		},
		{
			name:      "inactive employees without accounts need no action",
			dir:       "inactive_without_account",
			apply:     true,
			wantState: unchanged,
		},

		// Policy semantics.
		{
			name:       "managed group not granted by policy is removed from active employee",
			dir:        "unentitled_managed_group",
			apply:      true,
			wantWrites: []state.Action{remove("E001", "finance")},
			wantState:  "want.json",
			forbidden:  map[state.EmployeeID][]state.Group{"E001": {"finance"}},
		},
		{
			name:       "unmanaged groups, including look-alikes of managed ones, are preserved",
			dir:        "unmanaged_groups_preserved",
			apply:      true,
			wantWrites: []state.Action{remove("E001", "retail"), add("E001", "finance")},
			wantState:  "want.json",
			forbidden:  map[state.EmployeeID][]state.Group{"E001": {"retail"}},
		},
		{
			name:       "department mapped to no groups gets an account without groups",
			dir:        "department_without_groups",
			apply:      true,
			wantWrites: []state.Action{create("E001"), enable("E001")},
			wantState:  "want.json",
			forbidden:  map[state.EmployeeID][]state.Group{"E001": {"finance", "retail"}},
		},
		{
			name:       "group mapped from several departments",
			dir:        "shared_group",
			apply:      true,
			wantWrites: []state.Action{remove("E002", "retail"), add("E002", "finance")},
			wantState:  "want.json",
			forbidden: map[state.EmployeeID][]state.Group{
				"E001": {"retail"},
				"E002": {"retail"},
			},
		},
		{
			name:      "accounts absent from HR are left untouched",
			dir:       "orphans",
			apply:     true,
			wantState: unchanged,
		},

		// Repeatability.
		{
			name:      "no-op rerun when access already matches policy",
			dir:       "in_sync",
			apply:     true,
			wantState: unchanged,
			forbidden: map[state.EmployeeID][]state.Group{
				"E001": {"finance"},
				"E002": {"retail"},
				"E003": {"finance", "retail"},
			},
		},
		{
			name:  "mixed changes are ordered revocations first, then grants",
			dir:   "mixed",
			apply: true,
			wantWrites: []state.Action{
				disable("E003"),
				remove("E002", "retail"),
				remove("E003", "retail"),
				create("E004"),
				add("E002", "finance"),
				add("E004", "retail"),
				enable("E004"),
			},
			wantState: "want.json",
			forbidden: map[state.EmployeeID][]state.Group{
				"E001": {"finance"},
				"E002": {"retail"},
				"E003": {"finance", "retail"},
				"E004": {"finance"},
			},
		},
		{
			name:      "plan without apply writes nothing",
			dir:       "mixed",
			apply:     false,
			wantState: unchanged,
		},

		// Invalid input rejects the whole run with no writes. Each fixture is
		// otherwise actionable, so any partial write would be detected.
		{
			name:      "invalid_hr_malformed_json",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_hr_missing_field",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_hr_empty_id",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_hr_unknown_status",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_hr_unknown_field",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_hr_duplicate_id",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_hr_empty_snapshot",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_app_malformed_groups",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_app_missing_enabled",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_app_duplicate_id",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_policy_malformed",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_policy_empty",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_policy_empty_group_name",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_policy_duplicate_group",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_active_unmapped_department",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
		{
			name:      "invalid_department_case_mismatch",
			apply:     true,
			wantErr:   true,
			wantState: unchanged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.dir
			if dir == "" {
				dir = tt.name
			}
			appPath := setupApp(t, dir)

			before := readFile(t, appPath)
			writes, runID, err := run(t, dir, appPath, tt.apply, nil)
			if tt.wantErr && err == nil {
				t.Fatal("Run() succeeded, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
			if runID == "" {
				t.Error("Run() returned an empty run ID")
			}
			assertWrites(t, writes, tt.wantWrites)
			assertState(t, appPath, before, dir, tt.wantState)

			if tt.apply && !tt.wantErr {
				assertNoOpRerun(t, dir, appPath)
			}
			assertForbidden(t, appPath, tt.forbidden)
		})
	}
}

// TestRunRecovery fails one write during an apply, then checks that a second
// apply against the saved state completes the remaining work without
// repeating what already succeeded.
func TestRunRecovery(t *testing.T) {
	tests := []struct {
		name   string
		dir    string
		failOn state.Action
		// wantFailedWrites are the writes the failing run attempts, ending
		// with the one that fails.
		wantFailedWrites []state.Action
		// wantFailedState names the fixture the application file must match
		// after the failing run.
		wantFailedState string
		// wantRecoveryWrites are the writes the next run attempts.
		wantRecoveryWrites []state.Action
		forbidden          map[state.EmployeeID][]state.Group
	}{
		{
			name:             "failed disable leaves state unchanged and the next run offboards",
			dir:              "leaver",
			failOn:           disable("E001"),
			wantFailedWrites: []state.Action{disable("E001")},
			wantFailedState:  unchanged,
			wantRecoveryWrites: []state.Action{
				disable("E001"), remove("E001", "finance"), remove("E001", "retail"),
			},
			forbidden: map[state.EmployeeID][]state.Group{"E001": {"finance", "retail"}},
		},
		{
			name:             "failed group removal after disable is completed by the next run",
			dir:              "leaver",
			failOn:           remove("E001", "finance"),
			wantFailedWrites: []state.Action{disable("E001"), remove("E001", "finance")},
			wantFailedState:  "after_failed_remove.json",
			wantRecoveryWrites: []state.Action{
				remove("E001", "finance"), remove("E001", "retail"),
			},
			forbidden: map[state.EmployeeID][]state.Group{"E001": {"finance", "retail"}},
		},
		{
			name:               "failed grant after create resumes without a duplicate account",
			dir:                "joiner",
			failOn:             add("E001", "retail"),
			wantFailedWrites:   []state.Action{create("E001"), add("E001", "retail")},
			wantFailedState:    "after_failed_add.json",
			wantRecoveryWrites: []state.Action{add("E001", "retail"), enable("E001")},
			forbidden:          map[state.EmployeeID][]state.Group{"E001": {"finance"}},
		},
		{
			name:   "failed create keeps earlier revocations and the next run grants",
			dir:    "mixed",
			failOn: create("E004"),
			wantFailedWrites: []state.Action{
				disable("E003"),
				remove("E002", "retail"),
				remove("E003", "retail"),
				create("E004"),
			},
			wantFailedState: "after_failed_create.json",
			wantRecoveryWrites: []state.Action{
				create("E004"),
				add("E002", "finance"),
				add("E004", "retail"),
				enable("E004"),
			},
			forbidden: map[state.EmployeeID][]state.Group{
				"E002": {"retail"},
				"E003": {"finance", "retail"},
				"E004": {"finance"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appPath := setupApp(t, tt.dir)

			// First run: the injected write fails and the run reports it.
			before := readFile(t, appPath)
			writes, failedRunID, err := run(t, tt.dir, appPath, true, &tt.failOn)
			if !errors.Is(err, errInjected) {
				t.Fatalf("failing run: Run() error = %v, want it to wrap %v", err, errInjected)
			}
			if failedRunID == "" {
				t.Error("failing run: Run() returned an empty run ID")
			}
			assertWrites(t, writes, tt.wantFailedWrites)
			assertState(t, appPath, before, tt.dir, tt.wantFailedState)

			// Second run: starts from the saved state and finishes the job.
			before = readFile(t, appPath)
			writes, runID, err := run(t, tt.dir, appPath, true, nil)
			if err != nil {
				t.Fatalf("recovery run: Run() error = %v", err)
			}
			if runID == "" || runID == failedRunID {
				t.Errorf("recovery run: Run() run ID = %q, want a new non-empty ID (failed run was %q)", runID, failedRunID)
			}
			assertWrites(t, writes, tt.wantRecoveryWrites)
			assertState(t, appPath, before, tt.dir, "want.json")

			assertNoOpRerun(t, tt.dir, appPath)
			assertForbidden(t, appPath, tt.forbidden)
		})
	}
}

// setupApp copies the fixture's app.json to a temporary file and returns its
// path.
func setupApp(t *testing.T, dir string) string {
	t.Helper()

	appPath := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(appPath, fixture(t, dir, "app.json"), 0o600); err != nil {
		t.Fatal(err)
	}

	return appPath
}

func assertWrites(t *testing.T, got, want []state.Action) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Errorf("writes =\n\t%v\nwant\n\t%v", got, want)
	}
}

// assertState checks the application file against the named fixture. For
// unchanged, it also checks the file was not rewritten since before.
func assertState(t *testing.T, appPath string, before []byte, dir, want string) {
	t.Helper()

	if want == unchanged {
		if got := readFile(t, appPath); !bytes.Equal(got, before) {
			t.Errorf("app file was rewritten:\n%s\nwant\n%s", got, before)
		}
		return
	}

	got, wantAccounts := loadAccounts(t, appPath), loadFixtureAccounts(t, dir, want)
	if !reflect.DeepEqual(normalize(got), normalize(wantAccounts)) {
		t.Errorf("accounts =\n\t%+v\nwant (%s)\n\t%+v", got, want, wantAccounts)
	}
}

// assertNoOpRerun applies the same inputs again and checks that nothing is
// written once access matches policy.
func assertNoOpRerun(t *testing.T, dir, appPath string) {
	t.Helper()

	before := readFile(t, appPath)
	writes, _, err := run(t, dir, appPath, true, nil)
	if err != nil {
		t.Fatalf("rerun: Run() error = %v", err)
	}
	if len(writes) != 0 {
		t.Errorf("rerun: writes = %v, want none", writes)
	}
	if got := readFile(t, appPath); !bytes.Equal(got, before) {
		t.Error("rerun: app file was rewritten")
	}
}

// assertForbidden checks that no account holds any of its forbidden groups.
func assertForbidden(t *testing.T, appPath string, forbidden map[state.EmployeeID][]state.Group) {
	t.Helper()

	if len(forbidden) == 0 {
		return
	}

	accounts := loadAccounts(t, appPath)
	for id, groups := range forbidden {
		acc, ok := accounts[id]
		if !ok {
			continue
		}
		for _, g := range groups {
			if slices.Contains(acc.Groups, g) {
				t.Errorf("account %s holds forbidden group %q: %v", id, g, acc.Groups)
			}
		}
	}
}

// run executes a single App.Run against the fixtures in dir and the
// application file at appPath, returning the writes it attempted.
func run(
	t *testing.T, dir, appPath string, apply bool, failOn *state.Action,
) ([]state.Action, string, error) {
	t.Helper()

	f, err := os.Open(appPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	s := &recordingStore{JSONStore: store.NewJSONStore(f), failOn: failOn}
	a := app.New(
		s,
		hr.NewLoader(bytes.NewReader(fixture(t, dir, "hr.json"))),
		policy.NewLoader(bytes.NewReader(fixture(t, dir, "policy.json"))),
	)

	runID, err := a.Run(t.Context(), apply)

	return s.writes, runID, err
}

// recordingStore wraps the file-backed store, records every attempted write
// and fails the write matching failOn without performing it.
type recordingStore struct {
	store.JSONStore
	failOn *state.Action
	writes []state.Action
}

func (s *recordingStore) record(a state.Action) error {
	s.writes = append(s.writes, a)
	if s.failOn != nil && *s.failOn == a {
		return errInjected
	}

	return nil
}

func (s *recordingStore) CreateAccount(ctx context.Context, id state.EmployeeID) error {
	if err := s.record(create(id)); err != nil {
		return err
	}

	return s.JSONStore.CreateAccount(ctx, id)
}

func (s *recordingStore) SetEnabled(ctx context.Context, id state.EmployeeID, enabled bool) error {
	a := disable(id)
	if enabled {
		a = enable(id)
	}
	if err := s.record(a); err != nil {
		return err
	}

	return s.JSONStore.SetEnabled(ctx, id, enabled)
}

func (s *recordingStore) AddGroup(ctx context.Context, id state.EmployeeID, g state.Group) error {
	if err := s.record(add(id, g)); err != nil {
		return err
	}

	return s.JSONStore.AddGroup(ctx, id, g)
}

func (s *recordingStore) RemoveGroup(ctx context.Context, id state.EmployeeID, g state.Group) error {
	if err := s.record(remove(id, g)); err != nil {
		return err
	}

	return s.JSONStore.RemoveGroup(ctx, id, g)
}

func create(id state.EmployeeID) state.Action {
	return state.Action{Op: state.OpCreate, EmployeeID: id}
}

func enable(id state.EmployeeID) state.Action {
	return state.Action{Op: state.OpEnable, EmployeeID: id}
}

func disable(id state.EmployeeID) state.Action {
	return state.Action{Op: state.OpDisable, EmployeeID: id}
}

func add(id state.EmployeeID, g state.Group) state.Action {
	return state.Action{Op: state.OpAddGroup, EmployeeID: id, Group: g}
}

func remove(id state.EmployeeID, g state.Group) state.Action {
	return state.Action{Op: state.OpRemoveGroup, EmployeeID: id, Group: g}
}

func fixture(t *testing.T, dir, name string) []byte {
	t.Helper()

	data, err := fixtures.ReadFile(path.Join("testdata", dir, name))
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func readFile(t *testing.T, p string) []byte {
	t.Helper()

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

// loadAccounts reads an application file through the real store, so expected
// and actual states are decoded the same way.
func loadAccounts(t *testing.T, p string) state.Accounts {
	t.Helper()

	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	accounts, err := store.NewJSONStore(f).Load(context.Background())
	if err != nil {
		t.Fatalf("loading %s: %v", p, err)
	}

	return accounts
}

func loadFixtureAccounts(t *testing.T, dir, name string) state.Accounts {
	t.Helper()

	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, fixture(t, dir, name), 0o600); err != nil {
		t.Fatal(err)
	}

	return loadAccounts(t, p)
}

func normalize(accounts state.Accounts) state.Accounts {
	out := make(state.Accounts, len(accounts))
	for id, acc := range accounts {
		groups := slices.Clone(acc.Groups)
		slices.Sort(groups)
		if len(groups) == 0 {
			groups = nil
		}
		acc.Groups = groups
		out[id] = acc
	}

	return out
}
