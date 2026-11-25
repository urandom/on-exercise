package policy_test

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/urandom/on-exercise/internal/policy"
	"github.com/urandom/on-exercise/internal/state"
)

// Cases not covered by the app fixtures.
func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
		want    state.Policy
	}{
		{
			name: "department with no groups is valid",
			in:   `{"Retail":["retail"],"Legal":[]}`,
			want: state.Policy{"Retail": {"retail"}, "Legal": {}},
		},
		{name: "null policy", in: `null`, wantErr: policy.ErrInvalidPolicy},
		{name: "null group list", in: `{"Retail":null}`, wantErr: policy.ErrInvalidPolicy},
		{name: "empty department name", in: `{"":["retail"]}`, wantErr: policy.ErrInvalidPolicy},
		{name: "null group name", in: `{"Retail":[null]}`, wantErr: policy.ErrInvalidPolicy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := policy.NewLoader(strings.NewReader(tt.in)).Load(t.Context())
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("Load() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if !maps.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("Load() = %v, want %v", got, tt.want)
			}
		})
	}
}
