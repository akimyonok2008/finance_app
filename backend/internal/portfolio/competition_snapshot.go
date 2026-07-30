package portfolio

import (
	"context"
	"errors"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// This file is the ONLY portfolio-domain surface the competition engine may
// read through: a narrow, read-only, version-consistent capture of the
// current composition. The competition package never touches repositories,
// the mutation coordinator, or valuation internals directly, and nothing
// here mutates portfolio state.

var (
	// ErrCompetitionSnapshotConflict means the portfolio kept mutating while
	// the snapshot was being valued; the caller may retry later. Mapped to a
	// retryable conflict at the API layer.
	ErrCompetitionSnapshotConflict = errors.New("portfolio changed while capturing competition snapshot")
	// ErrCompetitionSnapshotUnpriced means one or more positions could not be
	// priced with fresh quotes and the request demanded fresh pricing (a
	// competition join must never freeze a stale or partial valuation).
	ErrCompetitionSnapshotUnpriced = errors.New("portfolio has unpriced or stale positions")
)

// snapshotVersionAttempts bounds the read-value-recheck loop. Valuation holds
// NO database locks, so a concurrent mutation simply invalidates the attempt
// and we re-read; three attempts is plenty outside pathological churn.
const snapshotVersionAttempts = 3

// CompetitionSnapshotRequest configures a capture.
type CompetitionSnapshotRequest struct {
	// RequireFreshQuotes fails the capture (ErrCompetitionSnapshotUnpriced)
	// if any position quote is stale or missing. Joins set it; a lightweight
	// eligibility preview may tolerate stale quotes.
	RequireFreshQuotes bool
}

// CompetitionSnapshotPosition is one frozen position with stable instrument
// identity, exact values, and the classification metadata rule filters need.
// Classification fields are empty when the identity layer has no record —
// filters then simply don't match (fail closed), never guess from symbols.
type CompetitionSnapshotPosition struct {
	PositionID     string
	Symbol         string
	InstrumentID   string
	AssetType      string
	VenueMIC       string
	IssuerCountry  string
	ListingCountry string
	Currency       string // quote currency of the priced value
	Quantity       money.Quantity
	Price          money.Price
	PriceCurrency  string
	ValueBase      money.Amount
}

// CompetitionSnapshotCash is one frozen cash balance.
type CompetitionSnapshotCash struct {
	Currency  string
	Amount    money.Amount
	ValueBase money.Amount
}

// CompetitionPortfolioSnapshot is the complete, internally-consistent capture:
// every value was computed against the same PortfolioVersion (verified by
// re-reading the version after valuation). It is INTERNAL input for the
// competition domain — none of these absolute values may ever reach a client.
type CompetitionPortfolioSnapshot struct {
	PortfolioID        string
	PortfolioVersion   int64
	CapturedAt         time.Time
	BaseCurrency       string
	PositionsValueBase money.Amount
	CashValueBase      money.Amount
	TotalValueBase     money.Amount // positions + cash
	PortfolioCreatedAt time.Time
	Positions          []CompetitionSnapshotPosition
	Cash               []CompetitionSnapshotCash
}

// CompetitionSnapshotProvider is the interface the competition engine
// consumes; *Service implements it below.
type CompetitionSnapshotProvider interface {
	CaptureCompetitionSnapshot(ctx context.Context, userID string, req CompetitionSnapshotRequest) (CompetitionPortfolioSnapshot, error)
}

// CaptureCompetitionSnapshot values the user's current portfolio (positions
// via the canonical Summary valuation, cash balances, FX-normalized exact
// base values) without holding any database lock across provider calls:
//
//	read portfolio version -> value everything -> re-read version ->
//	retry (bounded) if a mutation landed in between.
func (s *Service) CaptureCompetitionSnapshot(ctx context.Context, userID string, req CompetitionSnapshotRequest) (CompetitionPortfolioSnapshot, error) {
	for attempt := 0; attempt < snapshotVersionAttempts; attempt++ {
		pf, err := s.GetPortfolio(ctx, userID)
		if err != nil {
			return CompetitionPortfolioSnapshot{}, err
		}
		versionBefore := pf.Version

		snap, err := s.captureAtVersion(ctx, userID, pf, req)
		if err != nil {
			return CompetitionPortfolioSnapshot{}, err
		}

		after, err := s.GetPortfolio(ctx, userID)
		if err != nil {
			return CompetitionPortfolioSnapshot{}, err
		}
		if after.Version == versionBefore {
			return snap, nil
		}
	}
	return CompetitionPortfolioSnapshot{}, ErrCompetitionSnapshotConflict
}

func (s *Service) captureAtVersion(ctx context.Context, userID string, pf *Portfolio, req CompetitionSnapshotRequest) (CompetitionPortfolioSnapshot, error) {
	summary, err := s.Summary(ctx, userID)
	if err != nil {
		return CompetitionPortfolioSnapshot{}, err
	}
	if req.RequireFreshQuotes && summary.QuoteStatus.StaleCount > 0 {
		return CompetitionPortfolioSnapshot{}, ErrCompetitionSnapshotUnpriced
	}

	// PositionSummary carries the exact base valuation; join the open
	// positions list by id for instrument identity.
	positions, err := s.repo.ListPositionsByUser(ctx, userID)
	if err != nil {
		return CompetitionPortfolioSnapshot{}, err
	}
	instrumentIDs := make(map[string]string, len(positions))
	for _, p := range positions {
		if p.Status == PositionStatusOpen || p.Status == "" {
			instrumentIDs[p.ID] = p.InstrumentID
		}
	}

	snap := CompetitionPortfolioSnapshot{
		PortfolioID:        summary.PortfolioID,
		PortfolioVersion:   pf.Version,
		CapturedAt:         time.Now().UTC(),
		BaseCurrency:       summary.BaseCurrency,
		PositionsValueBase: money.ZeroAmount(),
		CashValueBase:      money.ZeroAmount(),
		PortfolioCreatedAt: pf.CreatedAt,
	}
	for _, ps := range summary.Positions {
		if req.RequireFreshQuotes && ps.QuoteIsStale {
			return CompetitionPortfolioSnapshot{}, ErrCompetitionSnapshotUnpriced
		}
		pos := CompetitionSnapshotPosition{
			PositionID:    ps.PositionID,
			Symbol:        ps.Symbol,
			InstrumentID:  instrumentIDs[ps.PositionID],
			AssetType:     ps.AssetType,
			Currency:      ps.Currency,
			Quantity:      ps.Quantity,
			Price:         ps.CurrentPrice,
			PriceCurrency: ps.CurrentPriceCurrency,
			ValueBase:     ps.CurrentValueBase,
		}
		s.enrichSnapshotClassification(ctx, &pos)
		snap.PositionsValueBase = snap.PositionsValueBase.Add(pos.ValueBase)
		snap.Positions = append(snap.Positions, pos)
	}
	for _, cb := range summary.CashBalances {
		snap.CashValueBase = snap.CashValueBase.Add(cb.ValueBase)
		snap.Cash = append(snap.Cash, CompetitionSnapshotCash{
			Currency: cb.Currency, Amount: cb.Amount, ValueBase: cb.ValueBase,
		})
	}
	snap.TotalValueBase = snap.PositionsValueBase.Add(snap.CashValueBase)
	return snap, nil
}

// enrichSnapshotClassification fills venue/country metadata from the stable
// identity register. Best-effort by design: a position without a resolved
// identity keeps empty classification fields, so venue/country rule filters
// simply don't match it — the engine never infers classification from the
// symbol string.
func (s *Service) enrichSnapshotClassification(ctx context.Context, pos *CompetitionSnapshotPosition) {
	if s.identity == nil || pos.InstrumentID == "" {
		return
	}
	in, err := s.identity.InstrumentByID(ctx, pos.InstrumentID)
	if err != nil || in == nil {
		return
	}
	pos.VenueMIC = in.MIC
	pos.ListingCountry = in.Country
	if in.AssetType != "" {
		pos.AssetType = in.AssetType
	}
}
