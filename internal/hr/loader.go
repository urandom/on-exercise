package hr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/urandom/on-exercise/internal/jsonx"
	"github.com/urandom/on-exercise/internal/state"
)

// ErrInvalidSnapshot wraps every error caused by the content of the HR
// snapshot, so callers can tell bad input apart from other failures.
var ErrInvalidSnapshot = errors.New("invalid HR snapshot")

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

type record struct {
	EmployeeID *state.EmployeeID `json:"employee_id"`
	Status     *state.Status     `json:"status"`
	Department *state.Department `json:"department"`
}

// Load decodes the HR snapshot, a JSON array of records. It rejects the whole
// snapshot if any record is invalid, reporting every invalid record by
// position.
func (l Loader) Load(ctx context.Context) (state.Employees, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var records []record
	if err := jsonx.DecodeStrict(l.r, &records); err != nil {
		return nil, fmt.Errorf("%w: decoding: %w", ErrInvalidSnapshot, err)
	}

	// Prevent accidental offboarding by a bad export
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: no records", ErrInvalidSnapshot)
	}

	employees := make(state.Employees, len(records))
	var errs []error
	for i, r := range records {
		e, err := r.employee()
		if err != nil {
			errs = append(errs, fmt.Errorf("record %d: %w", i+1, err))
			continue
		}

		if _, dup := employees[e.EmployeeID]; dup {
			errs = append(errs, fmt.Errorf("record %d: duplicate employee_id %q", i+1, e.EmployeeID))
			continue
		}

		employees[e.EmployeeID] = e
	}

	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSnapshot, err)
	}

	return employees, nil
}

func (r record) employee() (state.Employee, error) {
	var missing []string
	if r.EmployeeID == nil {
		missing = append(missing, "employee_id")
	}
	if r.Status == nil {
		missing = append(missing, "status")
	}
	if r.Department == nil {
		missing = append(missing, "department")
	}
	if len(missing) > 0 {
		return state.Employee{}, fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}

	id := *r.EmployeeID
	if id == "" {
		return state.Employee{}, errors.New("empty employee_id")
	}

	switch *r.Status {
	case state.StatusActive, state.StatusInactive:
	default:
		return state.Employee{}, fmt.Errorf("employee %q: unknown status %q", id, *r.Status)
	}

	if *r.Department == "" {
		return state.Employee{}, fmt.Errorf("employee %q: empty department", id)
	}

	return state.Employee{
		EmployeeID: id,
		Status:     *r.Status,
		Department: *r.Department,
	}, nil
}
