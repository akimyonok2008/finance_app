package competitions

import "errors"

var (
	// ErrCompetitionNotFound → HTTP 404.
	ErrCompetitionNotFound = errors.New("competition not found")
	// ErrCompetitionNotActive → HTTP 400 (cannot join upcoming/completed).
	ErrCompetitionNotActive = errors.New("competition is not active")
	// ErrEmptyPortfolio → HTTP 400 (no positions / zero value at join time).
	ErrEmptyPortfolio = errors.New("cannot join with an empty or zero-value portfolio")
	// ErrJoinSnapshot → HTTP 400 (a position could not be priced or converted
	// while capturing the join snapshot).
	ErrJoinSnapshot = errors.New("cannot snapshot portfolio: a position is unpriceable or has an unsupported currency")
	// ErrEntryNotFound is an internal repository signal (user has no entry).
	ErrEntryNotFound = errors.New("competition entry not found")
)
