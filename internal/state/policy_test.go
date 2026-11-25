package state_test

import (
	"maps"
	"testing"

	"github.com/urandom/on-exercise/internal/state"
)

func TestPolicyManagedGroups(t *testing.T) {
	tests := []struct {
		name   string
		policy state.Policy
		want   state.GroupSet
	}{
		{
			name:   "empty policy manages nothing",
			policy: state.Policy{},
			want:   state.GroupSet{},
		},
		{
			name: "union of all departments' groups",
			policy: state.Policy{
				"Retail":  {"retail"},
				"Finance": {"finance"},
			},
			want: state.GroupSet{"retail": {}, "finance": {}},
		},
		{
			name: "group granted by several departments appears once",
			policy: state.Policy{
				"Retail": {"retail"},
				"Sales":  {"retail", "finance"},
			},
			want: state.GroupSet{"retail": {}, "finance": {}},
		},
		{
			name: "department with no groups adds nothing",
			policy: state.Policy{
				"Retail": {"retail"},
				"Legal":  {},
			},
			want: state.GroupSet{"retail": {}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.policy.ManagedGroups()
			if !maps.Equal(got, tt.want) {
				t.Errorf("ManagedGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGroupSetContains(t *testing.T) {
	managed := state.Policy{"Retail": {"retail"}}.ManagedGroups()

	if !managed.Contains("retail") {
		t.Error(`Contains("retail") = false, want true`)
	}
	// Look-alikes of managed groups are not managed.
	for _, g := range []state.Group{"Retail", "retail-admin", " retail", ""} {
		if managed.Contains(g) {
			t.Errorf("Contains(%q) = true, want false", g)
		}
	}
}
