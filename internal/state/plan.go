package state

import (
	"fmt"
	"io"
)

// Op is a single kind of change to an application account.
type Op string

const (
	// OpCreate creates a new account that is disabled and has no groups.
	OpCreate Op = "create"
	// OpEnable enables an existing account.
	OpEnable Op = "enable"
	// OpDisable disables an existing account.
	OpDisable Op = "disable"
	// OpAddGroup adds a managed group to an account.
	OpAddGroup Op = "add_group"
	// OpRemoveGroup removes a managed group from an account.
	OpRemoveGroup Op = "remove_group"
)

// Action is one planned change to one account.
type Action struct {
	Op         Op
	EmployeeID EmployeeID
	// Group is set only for OpAddGroup and OpRemoveGroup.
	Group Group
}

// Plan is the result of reconciling the inputs.
type Plan struct {
	// Actions are applied in order.
	Actions []Action
	// Orphans are accounts that are absent from the HR snapshot. They are
	// outside the tool's authority: reported, never changed.
	Orphans []EmployeeID
}

// WriteTo writes the plan as human-readable text: the actions in the order
// they are applied, then the orphaned accounts. The output depends only on the
// plan, so equal plans always produce identical text.
//
// Lines are streamed to w as they are formatted. On a write error, WriteTo
// stops and returns the bytes written so far along with the error.
func (p Plan) WriteTo(w io.Writer) (int64, error) {
	cw := &countingWriter{w: w}

	if len(p.Actions) == 0 {
		cw.printf("No changes.\n")
	} else {
		cw.printf("Actions (%d, in order):\n", len(p.Actions))
		for _, a := range p.Actions {
			if a.Group != "" {
				cw.printf("  %-12s %s %s\n", a.Op, a.EmployeeID, a.Group)
			} else {
				cw.printf("  %-12s %s\n", a.Op, a.EmployeeID)
			}
		}
	}

	if len(p.Orphans) > 0 {
		cw.printf("Orphaned accounts, not in HR (%d, left unchanged):\n", len(p.Orphans))
		for _, id := range p.Orphans {
			cw.printf("  %s\n", id)
		}
	}

	return cw.n, cw.err
}

// countingWriter tracks the bytes written to w and keeps the first error.
// Once a write fails, later writes are skipped, so callers can format a
// sequence of lines and check the error once at the end.
type countingWriter struct {
	w   io.Writer
	n   int64
	err error
}

func (cw *countingWriter) printf(format string, args ...any) {
	if cw.err != nil {
		return
	}

	n, err := fmt.Fprintf(cw.w, format, args...)
	cw.n += int64(n)
	cw.err = err
}
