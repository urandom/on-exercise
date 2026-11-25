package state_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/urandom/on-exercise/internal/state"
)

var _ io.WriterTo = state.Plan{}

func TestPlanWriteTo(t *testing.T) {
	tests := []struct {
		name string
		plan state.Plan
		want string
	}{
		{
			name: "empty plan",
			want: "No changes.\n",
		},
		{
			name: "actions and orphans",
			plan: state.Plan{
				Actions: []state.Action{
					{Op: state.OpDisable, EmployeeID: "E002"},
					{Op: state.OpRemoveGroup, EmployeeID: "E002", Group: "finance"},
					{Op: state.OpCreate, EmployeeID: "E003"},
					{Op: state.OpAddGroup, EmployeeID: "E003", Group: "retail"},
					{Op: state.OpEnable, EmployeeID: "E003"},
				},
				Orphans: []state.EmployeeID{"X001"},
			},
			want: `Actions (5):
  disable      E002
  remove_group E002 finance
  create       E003
  add_group    E003 retail
  enable       E003
Orphaned accounts, not in HR (1, left unchanged):
  X001
`,
		},
		{
			name: "orphans only",
			plan: state.Plan{Orphans: []state.EmployeeID{"X001", "X002"}},
			want: `No changes.
Orphaned accounts, not in HR (2, left unchanged):
  X001
  X002
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			n, err := tt.plan.WriteTo(&sb)
			if err != nil {
				t.Fatalf("WriteTo() error = %v", err)
			}
			if got := sb.String(); got != tt.want {
				t.Errorf("WriteTo() =\n%s\nwant\n%s", got, tt.want)
			}
			if n != int64(sb.Len()) {
				t.Errorf("WriteTo() n = %d, want %d", n, sb.Len())
			}
		})
	}
}

// failingWriter accepts limit bytes, then fails every write.
type failingWriter struct {
	limit int
	sb    strings.Builder
}

var errWrite = errors.New("write failed")

func (fw *failingWriter) Write(p []byte) (int, error) {
	if fw.sb.Len()+len(p) > fw.limit {
		return 0, errWrite
	}
	return fw.sb.Write(p)
}

func TestPlanWriteToStopsOnError(t *testing.T) {
	plan := state.Plan{Orphans: []state.EmployeeID{"X001", "X002"}}
	fw := &failingWriter{limit: len("No changes.\n")}

	n, err := plan.WriteTo(fw)
	if !errors.Is(err, errWrite) {
		t.Fatalf("WriteTo() error = %v, want %v", err, errWrite)
	}
	if want := "No changes.\n"; fw.sb.String() != want {
		t.Errorf("written = %q, want %q", fw.sb.String(), want)
	}
	if n != int64(fw.sb.Len()) {
		t.Errorf("WriteTo() n = %d, want %d", n, fw.sb.Len())
	}
}
