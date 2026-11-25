package hr_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/urandom/on-exercise/internal/hr"
	"github.com/urandom/on-exercise/internal/state"
)

// Cases not covered by the app fixtures.
func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
		want    state.Employees
	}{
		{
			name: "valid",
			in:   `[{"employee_id":"E1","status":"active","department":"Retail"},{"employee_id":"E2","status":"inactive","department":"Finance"}]`,
			want: state.Employees{
				"E1": {EmployeeID: "E1", Status: state.StatusActive, Department: "Retail"},
				"E2": {EmployeeID: "E2", Status: state.StatusInactive, Department: "Finance"},
			},
		},
		{name: "null snapshot", in: `null`, wantErr: hr.ErrInvalidSnapshot},
		{name: "null record", in: `[null]`, wantErr: hr.ErrInvalidSnapshot},
		{name: "null field", in: `[{"employee_id":"E1","status":null,"department":"Retail"}]`, wantErr: hr.ErrInvalidSnapshot},
		{name: "empty department", in: `[{"employee_id":"E1","status":"inactive","department":""}]`, wantErr: hr.ErrInvalidSnapshot},
		{name: "status is case sensitive", in: `[{"employee_id":"E1","status":"Active","department":"Retail"}]`, wantErr: hr.ErrInvalidSnapshot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hr.NewLoader(strings.NewReader(tt.in)).Load(t.Context())
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("Load() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Load() = %v, want %v", got, tt.want)
			}
			for id, e := range tt.want {
				if got[id] != e {
					t.Errorf("Load()[%s] = %+v, want %+v", id, got[id], e)
				}
			}
		})
	}
}

func TestLoadReportsEveryInvalidRecord(t *testing.T) {
	in := `[
		{"employee_id":"E1","status":"active"},
		{"employee_id":"E2","status":"active","department":"Retail"},
		{"employee_id":"E2","status":"active","department":"Retail"}
	]`

	_, err := hr.NewLoader(strings.NewReader(in)).Load(t.Context())
	if !errors.Is(err, hr.ErrInvalidSnapshot) {
		t.Fatalf("Load() error = %v, want %v", err, hr.ErrInvalidSnapshot)
	}
	for _, want := range []string{"record 1: missing department", `record 3: duplicate employee_id "E2"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Load() error = %q, want it to contain %q", err, want)
		}
	}
}
