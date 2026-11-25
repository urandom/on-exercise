package policy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/urandom/on-exercise/internal/jsonx"
	"github.com/urandom/on-exercise/internal/state"
)

// ErrInvalidPolicy wraps every error caused by the content of the policy, so
// callers can tell bad input apart from other failures.
var ErrInvalidPolicy = errors.New("invalid policy")

// Loader loads and validates the department policy from a configured source.
//
// The source is an io.Reader, which can only be consumed once, so Load must
// be called at most once per Loader.
type Loader struct {
	r io.Reader
}

func NewLoader(r io.Reader) Loader {
	return Loader{r: r}
}

// Load decodes the policy, a JSON object mapping each department to the list
// of managed groups it grants. It rejects the whole policy if any entry is
// invalid, reporting every invalid entry.
func (l Loader) Load(ctx context.Context) (state.Policy, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// TODO: Repeated keys would be accepted, last one winning. Either a list
	// format or manual token decoding would detect them.
	var policy state.Policy
	if err := jsonx.DecodeStrict(l.r, &policy); err != nil {
		return nil, fmt.Errorf("%w: decoding: %w", ErrInvalidPolicy, err)
	}

	// Empty policy is likely a mistake
	if len(policy) == 0 {
		return nil, fmt.Errorf("%w: no departments", ErrInvalidPolicy)
	}

	// Departments are checked in sorted order so the error is the same on
	// every run.
	var errs []error
	for _, dept := range slices.Sorted(maps.Keys(policy)) {
		if err := validateDepartment(dept, policy[dept]); err != nil {
			errs = append(errs, err)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPolicy, err)
	}

	return policy, nil
}

func validateDepartment(dept state.Department, groups state.Groups) error {
	if dept == "" {
		return errors.New("empty department name")
	}
	if groups == nil {
		return fmt.Errorf("department %q: groups must be a list, not null", dept)
	}

	seen := make(state.GroupSet, len(groups))
	for _, g := range groups {
		if g == "" {
			return fmt.Errorf("department %q: empty group name", dept)
		}
		if seen.Contains(g) {
			return fmt.Errorf("department %q: duplicate group %q", dept, g)
		}
		seen[g] = struct{}{}
	}

	return nil
}
