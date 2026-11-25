package policy

import (
	"context"
	"errors"
	"io"

	"github.com/urandom/on-exercise/internal/state"
)

var errNotImplemented = errors.New("not implemented")

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

func (l Loader) Load(ctx context.Context) (state.Policies, error) {
	return nil, errNotImplemented
}
