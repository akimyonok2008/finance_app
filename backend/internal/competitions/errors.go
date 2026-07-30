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

	// --- competition engine (definitions / editions / lifecycle) ---

	// ErrDefinitionNotFound → HTTP 404 on admin reads.
	ErrDefinitionNotFound = errors.New("competition definition not found")
	// ErrDefinitionExists → HTTP 409 (duplicate id or slug).
	ErrDefinitionExists = errors.New("competition definition already exists")
	// ErrDefinitionVersionNotFound → HTTP 404.
	ErrDefinitionVersionNotFound = errors.New("competition definition version not found")
	// ErrDefinitionVersionExists → HTTP 409. Versions are immutable and
	// append-only: an existing (definition, version) can never be rewritten.
	ErrDefinitionVersionExists = errors.New("competition definition version already exists")
	// ErrInvalidLifecycleTransition → HTTP 409 (transition not in the
	// validated state machine, e.g. completed -> active).
	ErrInvalidLifecycleTransition = errors.New("invalid competition lifecycle transition")
	// ErrLifecycleConflict → HTTP 409 (the edition was not in the expected
	// state when the guarded transition ran — someone else moved it first).
	ErrLifecycleConflict = errors.New("competition lifecycle changed concurrently")
	// ErrEditionExists → HTTP 409 (duplicate edition id).
	ErrEditionExists = errors.New("competition edition already exists")
	// ErrInvalidRuleDocument → HTTP 400 on admin writes: the eligibility or
	// scoring payload failed typed-schema validation and was NOT persisted.
	ErrInvalidRuleDocument = errors.New("invalid competition rule document")
	// ErrEligibilityUnavailable → HTTP 503: no portfolio snapshot boundary is
	// wired (memory-mode deployments without the engine enabled).
	ErrEligibilityUnavailable = errors.New("eligibility evaluation is unavailable")

	// --- join / withdrawal / baseline ---

	// ErrIdempotencyKeyRequired → HTTP 400: engine joins are mutations and
	// follow the application-wide Idempotency-Key convention.
	ErrIdempotencyKeyRequired = errors.New("Idempotency-Key header is required")
	// ErrJoinWindowClosed → HTTP 409: outside [join_opens_at, join_closes_at]
	// or the edition is not in registration_open. Late joins are structurally
	// impossible for engine editions — everyone is measured from the same
	// baseline.
	ErrJoinWindowClosed = errors.New("the join window for this competition is not open")
	// ErrNotEligible → HTTP 422 with rule evidence in the response body.
	ErrNotEligible = errors.New("portfolio does not meet the competition's eligibility rules")
	// ErrNothingToScore → HTTP 422: eligibility passed but the scoring
	// configuration selects an empty (zero-value) sleeve.
	ErrNothingToScore = errors.New("no portfolio component matches the competition's scoring scope")
	// ErrWithdrawalClosed → HTTP 409 (after join_closes_at, or entry already
	// past admitted).
	ErrWithdrawalClosed = errors.New("withdrawal is no longer possible for this competition")
	// ErrEntryConflict is the internal guarded-update signal (entry state
	// changed concurrently).
	ErrEntryConflict = errors.New("competition entry changed concurrently")
	// ErrEntryExists is the internal duplicate-entry signal; the join flow
	// resolves it by re-reading the existing entry (idempotent replay).
	ErrEntryExists = errors.New("competition entry already exists")

	// --- ranking projection / finalization ---

	// ErrRankingUnavailable → HTTP 503: no ranking generation has ever been
	// promoted for this edition yet. Never triggers a live O(N) fallback scan
	// — see Service.EditionLeaderboard.
	ErrRankingUnavailable = errors.New("competition ranking projection is not ready yet")
	// ErrResultsNotAvailable → HTTP 409: the edition has not completed, so
	// there are no immutable final results to read.
	ErrResultsNotAvailable = errors.New("competition has not completed; final results are not available")
	// ErrGenerationConflict is the internal guarded-promotion signal (another
	// worker replica already advanced this generation).
	ErrGenerationConflict = errors.New("competition ranking generation changed concurrently")
)
