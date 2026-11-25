package hr

import (
	"context"
	"fmt"
	"io"

	"github.com/urandom/on-exercise/internal/jsonx"
	"github.com/urandom/on-exercise/internal/state"
)

// Loader loads and validates the HR snapshot from a configured source.
//
// The source is an io.Reader, which can only be consumed once, so Load must
// be called at most once per Loader.
type Loader struct {
	r io.Reader
}

func NewLoader(r io.Reader) Loader {
	return Loader{r: r}
}

// record is a single HR record as it appears in the snapshot file. Fields are
// pointers so that a missing or null key decodes to nil and can be told apart
// from an empty value.
type record struct {
	EmployeeID *state.EmployeeID `json:"employee_id"`
	Status     *state.Status     `json:"status"`
	Department *state.Department `json:"department"`
}

// Load decodes the HR snapshot, a JSON array of records, into employees keyed
// by ID.
func (l Loader) Load(ctx context.Context) (state.Employees, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// TODO: export a sentinel error (e.g. ErrInvalidSnapshot) and wrap every
	// decoding and validation error with it, so callers can use errors.Is.
	var records []record
	if err := jsonx.DecodeStrict(l.r, &records); err != nil {
		return nil, fmt.Errorf("decoding HR snapshot: %w", err)
	}

	// TODO: reject an empty snapshot; it most likely means a bad export, and
	// would otherwise offboard everyone.

	employees := make(state.Employees, len(records))
	for _, r := range records {
		// TODO: reject a record with any nil field (missing or null key).
		// Until then, value turns nil into the zero value.
		// TODO: reject an empty employee_id.
		// TODO: reject a status other than state.StatusActive or
		// state.StatusInactive.
		// TODO: reject a duplicate employee_id before inserting; the map
		// would otherwise keep the last record silently.
		id := value(r.EmployeeID)
		employees[id] = state.Employee{
			EmployeeID: id,
			Status:     value(r.Status),
			Department: value(r.Department),
		}
	}

	return employees, nil
}

// value returns *p, or the zero value if p is nil.
func value[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
