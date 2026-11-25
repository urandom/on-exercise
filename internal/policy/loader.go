package policy

import (
	"context"
	"fmt"
	"io"

	"github.com/urandom/on-exercise/internal/jsonx"
	"github.com/urandom/on-exercise/internal/state"
)

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
// of managed groups it grants.
func (l Loader) Load(ctx context.Context) (state.Policies, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// TODO: export a sentinel error (e.g. ErrInvalidPolicy) and wrap every
	// decoding and validation error with it, so callers can use errors.Is.
	// TODO: a JSON object with a repeated department key decodes with the
	// last value winning. Decide whether to detect that (token-level decode)
	// or accept it.
	var policies state.Policies
	if err := jsonx.DecodeStrict(l.r, &policies); err != nil {
		return nil, fmt.Errorf("decoding policy: %w", err)
	}

	// TODO: reject an empty policy (including a JSON null, which decodes to
	// a nil map without error).
	// TODO: reject an empty department name.
	// TODO: reject a null group list (decodes to nil without error); an
	// explicit [] is valid and grants no groups.
	// TODO: reject empty group names and duplicate groups within one
	// department.

	return policies, nil
}
