package state

// Policies maps each department to the managed groups its active employees
// are allowed to hold. They are assumed to have been validated.
//
// The set of managed groups is exactly the union of all groups in the policy;
// any other group is unmanaged and must never be added or removed.
type Policies map[Department]Groups

// IsManaged reports whether the group is controlled by the policy.
func (p Policies) IsManaged(g Group) bool {
	for _, groups := range p {
		for _, pg := range groups {
			if pg == g {
				return true
			}
		}
	}

	return false
}
