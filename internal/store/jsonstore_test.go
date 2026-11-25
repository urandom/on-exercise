package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/urandom/on-exercise/internal/state"
	"github.com/urandom/on-exercise/internal/store"
)

// Cases not covered by the app fixtures.
func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
		want    state.Accounts
	}{
		{name: "empty state is valid", in: `[]`, want: state.Accounts{}},
		{
			name: "valid",
			in:   `[{"employee_id":"E1","enabled":true,"groups":["retail","wiki"]},{"employee_id":"E2","enabled":false,"groups":[]}]`,
			want: state.Accounts{
				"E1": {EmployeeID: "E1", Enabled: true, Groups: state.Groups{"retail", "wiki"}},
				"E2": {EmployeeID: "E2", Enabled: false, Groups: state.Groups{}},
			},
		},
		{name: "missing groups", in: `[{"employee_id":"E1","enabled":true}]`, wantErr: store.ErrInvalidState},
		{name: "null groups", in: `[{"employee_id":"E1","enabled":true,"groups":null}]`, wantErr: store.ErrInvalidState},
		{name: "empty employee_id", in: `[{"employee_id":"","enabled":true,"groups":[]}]`, wantErr: store.ErrInvalidState},
		{name: "empty group name", in: `[{"employee_id":"E1","enabled":true,"groups":[""]}]`, wantErr: store.ErrInvalidState},
		{name: "duplicate group", in: `[{"employee_id":"E1","enabled":true,"groups":["retail","retail"]}]`, wantErr: store.ErrInvalidState},
		{name: "unknown field", in: `[{"employee_id":"E1","enabled":true,"groups":[],"admin":true}]`, wantErr: store.ErrInvalidState},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app.json")
			if err := os.WriteFile(path, []byte(tt.in), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			got, err := store.NewJSONStore(f).Load(t.Context())
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("Load() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
