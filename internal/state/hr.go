package state

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
