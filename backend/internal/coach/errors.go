package coach

import "errors"

var (
	// ErrUnsupportedMode is returned when the request mode is not recognized.
	ErrUnsupportedMode = errors.New("unsupported coach mode")

	// ErrEmptyPortfolio is returned when the user has no positions to analyze.
	// The provider is never called in this case.
	ErrEmptyPortfolio = errors.New("portfolio has no positions to analyze")
)
