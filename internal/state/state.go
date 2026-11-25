package state

// Accounts is the app's list of accounts. They are assumed to have been validated.
type Accounts map[EmployeeID]Account

type EmployeeID string

type Groups []Group

// Set returns the groups as a GroupSet. Duplicates collapse into one entry.
func (gs Groups) Set() GroupSet {
	set := make(GroupSet, len(gs))
	for _, g := range gs {
		set[g] = struct{}{}
	}

	return set
}

type Group string

// GroupSet is an unordered set of groups.
type GroupSet map[Group]struct{}

// Contains reports whether g is in the set.
func (s GroupSet) Contains(g Group) bool {
	_, ok := s[g]
	return ok
}

// Account is a single application account as stored in the app state file.
type Account struct {
	EmployeeID EmployeeID
	Enabled    bool
	Groups     Groups
}
