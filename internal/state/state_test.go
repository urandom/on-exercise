package state_test

import (
	"maps"
	"testing"

	"github.com/urandom/on-exercise/internal/state"
)

func TestGroupsSet(t *testing.T) {
	tests := []struct {
		name   string
		groups state.Groups
		want   state.GroupSet
	}{
		{name: "nil", groups: nil, want: state.GroupSet{}},
		{name: "empty", groups: state.Groups{}, want: state.GroupSet{}},
		{
			name:   "distinct groups",
			groups: state.Groups{"retail", "wiki"},
			want:   state.GroupSet{"retail": {}, "wiki": {}},
		},
		{
			name:   "duplicates collapse",
			groups: state.Groups{"retail", "wiki", "retail"},
			want:   state.GroupSet{"retail": {}, "wiki": {}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.groups.Set()
			if got == nil {
				t.Fatal("Set() = nil, want a non-nil set")
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("Set() = %v, want %v", got, tt.want)
			}
		})
	}
}
