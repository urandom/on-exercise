package state

// Accounts is the app's list of accounts. They are assumed to have been validated.
type Accounts map[EmployeeID]Account

type EmployeeID string

type Groups []Group

type Group string

// Account is a single application account as stored in the app state file.
type Account struct {
	EmployeeID EmployeeID
	Enabled    bool
	Groups     Groups
}
