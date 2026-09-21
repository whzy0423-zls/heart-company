package server

// membershipContentLocked reports whether a resource may remain visible in a
// history/list response but its full content must be withheld. Keep this
// separate from mutation authorization: a read-only historical resource is
// still deletable, while its private body must not be serialized.
func membershipContentLocked(access membershipResourceMetadata) bool {
	if access.UpgradeRequired {
		return true
	}
	switch access.State {
	case resourceAccessReadOnlyOverLimit, resourceAccessLockedUpgrade:
		return true
	default:
		return false
	}
}
