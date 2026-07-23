package performance

import "errors"

var (
	// ErrStateNotFound is returned by the repository when a portfolio has no
	// ranked-performance state yet.
	ErrStateNotFound = errors.New("ranked performance state not found")
	// ErrVersionConflict is returned when an optimistic update loses a race with
	// a concurrent writer. The caller should re-read and retry.
	ErrVersionConflict = errors.New("ranked performance state version conflict")
	// ErrInvalidState guards the persistence invariants (finite, positive index;
	// positive segment value when active).
	ErrInvalidState = errors.New("invalid ranked performance state")
)
