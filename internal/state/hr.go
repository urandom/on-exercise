package state

import "log/slog"

// Employees is the HR snapshot keyed by employee ID. They are assumed to have been validated.
type Employees map[EmployeeID]Employee

type Department string

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// Employee is a single HR record from the HR snapshot.
type Employee struct {
	EmployeeID EmployeeID
	Status     Status
	Department Department
}

// LogValue logs an employee as their ID only. The rest is considered sensitive
// information.
func (e Employee) LogValue() slog.Value {
	return slog.StringValue(string(e.EmployeeID))
}

// LogValue logs the snapshot as its number of employees only.
func (e Employees) LogValue() slog.Value {
	return slog.IntValue(len(e))
}
