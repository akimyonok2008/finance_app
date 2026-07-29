package portfolio

import "errors"

// Domain errors. Handlers map these to HTTP status codes.
var (
	// ErrSymbolRequired is returned when a position has no symbol.
	ErrSymbolRequired = errors.New("symbol is required")
	// ErrInvalidAssetType is returned for an asset_type outside stock/etf/crypto.
	ErrInvalidAssetType = errors.New("asset_type must be one of: stock, etf, crypto")
	// ErrInvalidQuantity is returned when quantity is not greater than zero.
	ErrInvalidQuantity = errors.New("quantity must be greater than 0")
	// ErrInvalidWeights is returned when copied strategy weights are malformed.
	ErrInvalidWeights = errors.New("strategy weights must be positive, unique, and total 100")
	// ErrUnsupportedSymbol is returned when a symbol has an invalid format or
	// cannot be priced by the active provider. Maps to HTTP 400.
	ErrUnsupportedSymbol = errors.New("unsupported or unpriceable symbol")
	// ErrUnsupportedCurrency is returned when a symbol's quote currency cannot
	// be converted to the base currency. Maps to HTTP 400.
	ErrUnsupportedCurrency = errors.New("unsupported currency")
	ErrInvalidCashAmount   = errors.New("cash amount must be greater than 0")
	ErrInsufficientCash    = errors.New("insufficient cash")
	ErrCashBalanceNotFound = errors.New("cash balance not found")
	ErrInvalidSaleQuantity = errors.New("sale quantity must be greater than 0 and not exceed the owned quantity")
	ErrInvalidSalePrice    = errors.New("execution price must be greater than 0")
	ErrInvalidSaleFee      = errors.New("sale fee must be non-negative and less than gross proceeds")
	ErrDuplicateActivity   = errors.New("activity already recorded")

	// ErrActivityNotFound is returned when a correction references an unknown
	// activity, or an activity that does not belong to the requesting user.
	ErrActivityNotFound = errors.New("activity not found")
	// ErrActivityAlreadyCorrected is returned when an activity already has a
	// compensating correction on record; corrections cannot stack.
	ErrActivityAlreadyCorrected = errors.New("activity already corrected")
	// ErrCorrectionNotSupported is returned for activity types that cannot be
	// safely corrected after the fact (buy/sell may have been followed by other
	// activity against the same position episode). Record an offsetting
	// buy/sell instead.
	ErrCorrectionNotSupported = errors.New("this activity type cannot be corrected; record an offsetting buy or sell instead")
	// ErrNothingToCorrect is returned when the corrected amount equals the
	// original amount, so no compensating activity would be created.
	ErrNothingToCorrect = errors.New("corrected amount matches the original activity; nothing to correct")
	// ErrCorrectionSupersededByLaterActivity is returned when correcting a
	// buy/sell is unsafe because a later activity already exists in the same
	// position episode (another buy, a partial/full sell, a split, a
	// reinvested dividend). Only the most recent event in an episode can be
	// corrected; anything else would retroactively change basis/quantity math
	// that later activity already depended on.
	ErrCorrectionSupersededByLaterActivity = errors.New("a later transaction exists in this position's history; this trade can no longer be corrected — record an offsetting trade instead")
	// ErrCorrectionRankedConflict is returned when correcting a buy/sell would
	// change state that an already-computed, trusted ranked snapshot
	// captured. Ranked history is never rebuilt: the correction is refused so
	// ranked history stays internally consistent.
	ErrCorrectionRankedConflict = errors.New("this trade contributed to an already-established ranked snapshot and can no longer be corrected")

	// ErrPositionNotFound is returned when a position does not exist OR does not
	// belong to the requesting user. The same error is used for both cases so
	// the API never reveals the existence of another user's positions.
	ErrPositionNotFound = errors.New("position not found")
	// ErrPositionClosed is returned when an action requires an open position but
	// the position has already been closed.
	ErrPositionClosed = errors.New("position is already closed")
	// ErrPositionIsPriceable guards the write-off escape hatch: it is only for
	// a position genuinely stuck with no available market price. A priceable
	// position must be sold normally — write-off is not a general-purpose way
	// to erase a losing position from realized results.
	ErrPositionIsPriceable = errors.New("this position still has an available market price; sell it instead of writing it off")
	// ErrPortfolioNotFound signals a broken or incomplete account aggregate.
	// Read paths return it without creating data.
	ErrPortfolioNotFound = errors.New("portfolio not found")

	// ErrPriceProvider wraps any failure from the price provider so the handler
	// can respond with 502 Bad Gateway.
	ErrPriceProvider = errors.New("price provider error")

	// ErrMutationConflict is returned when a mutation could not be applied
	// because the portfolio kept changing underneath it (the bounded rebuild
	// budget was exhausted). Maps to HTTP 409 Conflict; the client may retry.
	ErrMutationConflict = errors.New("portfolio changed concurrently; retry the mutation")
	// ErrIdempotencyConflict means a key was reused for a different operation
	// or normalized request payload. The original mutation remains committed.
	ErrIdempotencyConflict = errors.New("Idempotency-Key was already used for a different request")
	// ErrRankedInvariant is returned when a computed checkpoint would generate a
	// ranked return from the mutation itself. It aborts the transaction rather
	// than committing corrupted ranked history, and always indicates a bug.
	ErrRankedInvariant = errors.New("mutation would alter ranked performance")
	// ErrInvalidBuyPrice is returned when a user-entered buy execution price is
	// not a finite positive number.
	ErrInvalidBuyPrice = errors.New("execution price must be greater than 0")
	// ErrInvalidBuyFee is returned when a user-entered buy fee is negative or
	// not finite.
	ErrInvalidBuyFee = errors.New("purchase fee must be non-negative")

	// ErrImplausibleExecutionPrice is returned when a user-entered execution
	// price for a trade is wildly inconsistent with the tracked market quote
	// pinned for the same mutation — e.g. claiming a fill far below or above
	// the current price. It exists because current holdings P&L and public
	// open/closed return percentages are built directly from this price,
	// unlike the ranked leaderboard index, which values every position at the
	// tracked quote and is unaffected by it. There is no backdating exemption:
	// every trade is recorded against the live quote at the time it is
	// entered, so there is always a real comparator.
	ErrImplausibleExecutionPrice = errors.New("execution price is too far from the current market price")

	ErrInstrumentIdentityAmbiguous          = errors.New("instrument identity is ambiguous; provide exchange or MIC")
	ErrInstrumentIdentityUnresolvedConflict = errors.New("an unresolved open position already uses this symbol; resolve the instrument before adding another listing")
	// ErrInstrumentIdentityUnresolved is returned instead of saving a
	// ticker-only position when the service is configured with
	// InstrumentResolutionRequired (see config.InstrumentResolutionRequired,
	// forced true under APP_ENV=production).
	ErrInstrumentIdentityUnresolved = errors.New("instrument identity could not be resolved for this ticker; provide exchange/MIC or verify the symbol")
	// ErrReconciliationItemNotFound is returned when resolving/rejecting a
	// reconciliation-queue item that doesn't exist or is no longer pending.
	ErrReconciliationItemNotFound = errors.New("identity reconciliation item not found or already resolved")

	// ErrUnsupportedMutation guards the mutation-kind switch.
	ErrUnsupportedMutation = errors.New("unsupported portfolio mutation")
)
