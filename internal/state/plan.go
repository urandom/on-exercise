package state

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
