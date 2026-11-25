package state

// Policy maps each department to the managed groups its active employees
// are allowed to hold. It is assumed to have been validated.
//
// The set of managed groups is exactly the union of all groups in the policy;
// any other group is unmanaged and must never be added or removed.
type Policy map[Department]Groups

// ManagedGroups returns every group that at least one department in the
// policy grants. Groups outside this set are unmanaged and must never be added
// or removed.
func (p Policy) ManagedGroups() GroupSet {
	managed := GroupSet{}
	for _, groups := range p {
		for _, g := range groups {
			managed[g] = struct{}{}
		}
	}

	return managed
}
