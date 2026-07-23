package portfolio

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// Service holds the portfolio business logic: validation, ownership
// enforcement, and summary calculation. It depends only on the Repository,
// PriceProvider, and FXProvider interfaces.
type Service struct {
	repo         Repository
	provider     prices.PriceProvider
	fx           fx.FXProvider
	checkpointer RankedCheckpointer // optional; wired in main
}

// NewService wires a Service with its repository, price provider, and FX
// provider (for base-currency normalization).
func NewService(repo Repository, provider prices.PriceProvider, fxp fx.FXProvider) *Service {
	return &Service{repo: repo, provider: provider, fx: fxp}
}

// GetOrCreateDefaultPortfolio returns the user's portfolio, creating the default
// one on first access.
func (s *Service) GetOrCreateDefaultPortfolio(userID string) (*Portfolio, error) {
	p, err := s.repo.GetPortfolioByUser(userID)
	if err == nil {
		return p, nil
	}
	if err != ErrPortfolioNotFound {
		return nil, err
	}

	now := time.Now().UTC()
	p = &Portfolio{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      DefaultPortfolioName,
		Currency:  "USD",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreatePortfolio(p); err != nil {
		return nil, err
	}
	return p, nil
}

// AddPosition validates the input and creates a position in the user's default
// portfolio, LOCKING the baseline at the current market price. The client never
// supplies a price or currency: the backend fetches today's quote and stores
// price + quote currency as the immutable baseline. A fresh position therefore
// starts at exactly 0% gain (portfolio index contribution of 100), and ranked
// performance can only come from market moves after the add — historical buy
// prices do not exist in this model.
func (s *Service) AddPosition(ctx context.Context, userID string, in PositionInput) (*Position, error) {
	clean, quote, err := s.validatePosition(ctx, in)
	if err != nil {
		return nil, err
	}

	pf, err := s.GetOrCreateDefaultPortfolio(userID)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	pos := &Position{
		ID:              uuid.NewString(),
		UserID:          userID,
		PortfolioID:     pf.ID,
		Symbol:          clean.Symbol,
		AssetType:       clean.AssetType,
		Quantity:        clean.Quantity,
		AverageBuyPrice: quote.Price,    // locked baseline: today's price
		Currency:        quote.Currency, // quote currency of the baseline
		Status:          PositionStatusOpen,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Value the portfolio before/after the add on one consistent snapshot, then
	// persist the position and checkpoint the ranked index. The new capital enters
	// the ranked segment at its current price, so the mutation yields zero return.
	oldOpen := openPositions(existing)
	newOpen := append(append([]*Position(nil), oldOpen...), pos)
	cache := newQuoteCache()
	cache.seedPrice(clean.Symbol, quote)
	input, err := s.rankedInput(ctx, pf.ID, userID, oldOpen, newOpen, cache)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreatePosition(pos); err != nil {
		return nil, err
	}
	if err := s.applyRankedCheckpoint(ctx, input); err != nil {
		return nil, err
	}
	_ = s.recordArchiveSnapshot(ctx, userID)
	return pos, nil
}

// ReplaceWithStrategyWeights creates a fresh baseline from public percentage
// weights. It validates the entire target allocation first, then replaces the
// user's positions with neutral notional quantities that reproduce the requested
// weights at current market prices. No source quantities, money values, cost
// basis, or prices are copied.
func (s *Service) ReplaceWithStrategyWeights(ctx context.Context, userID string, weights []StrategyWeightInput) error {
	prepared, err := s.prepareStrategyWeights(ctx, weights)
	if err != nil {
		return err
	}

	pf, err := s.GetOrCreateDefaultPortfolio(userID)
	if err != nil {
		return err
	}
	existing, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	cache := newQuoteCache()
	newPositions := make([]*Position, 0, len(prepared))
	for _, p := range prepared {
		newPositions = append(newPositions, &Position{
			ID:              uuid.NewString(),
			UserID:          userID,
			PortfolioID:     pf.ID,
			Symbol:          p.Symbol,
			AssetType:       p.AssetType,
			Quantity:        p.Quantity,
			AverageBuyPrice: p.Price,
			Currency:        p.Currency,
			Status:          PositionStatusOpen,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		cache.seedPrice(p.Symbol, &prices.Price{Symbol: p.Symbol, Price: p.Price, Currency: p.Currency})
	}

	// Value the current basket and the copied basket on one snapshot, then replace
	// positions atomically and checkpoint. The copy starts a NEW segment at
	// current prices while preserving the accumulated ranked index — copying a
	// strategy can never reset ranked history to 100.
	oldOpen := openPositions(existing)
	input, err := s.rankedInput(ctx, pf.ID, userID, oldOpen, newPositions, cache)
	if err != nil {
		return err
	}
	if err := s.repo.ReplaceOpenPositions(userID, newPositions); err != nil {
		return err
	}
	if err := s.applyRankedCheckpoint(ctx, input); err != nil {
		return err
	}
	_ = s.recordArchiveSnapshot(ctx, userID)
	return nil
}

// ListPositions returns the requesting user's active/open positions only.
func (s *Service) ListPositions(userID string) ([]*Position, error) {
	positions, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return nil, err
	}
	return filterPositionsByStatus(positions, PositionStatusOpen), nil
}

func (s *Service) ListClosedPositions(ctx context.Context, userID string) ([]ClosedPositionSummary, error) {
	positions, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return nil, err
	}
	closed := filterPositionsByStatus(positions, PositionStatusClosed)
	out := make([]ClosedPositionSummary, 0, len(closed))
	for _, pos := range closed {
		view, err := s.closedPositionSummary(ctx, pos)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

// UpdatePosition updates the QUANTITY of a position the user owns. The symbol
// and locked baseline price are immutable — re-pricing on edit would let users
// reset a losing baseline, which breaks ranking fairness. To change symbols,
// delete and re-add (which locks a fresh baseline at that day's price).
// Updating a position that does not exist or belongs to another user returns
// ErrPositionNotFound.
func (s *Service) UpdatePosition(ctx context.Context, userID, positionID string, quantity float64) (*Position, error) {
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}

	existing, err := s.ownedPosition(userID, positionID)
	if err != nil {
		return nil, err
	}
	if positionStatus(existing) != PositionStatusOpen {
		return nil, ErrPositionClosed
	}

	all, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return nil, err
	}

	updated := *existing
	updated.Quantity = quantity
	updated.UpdatedAt = time.Now().UTC()

	// Build the post-mutation open set (same positions, target quantity changed)
	// and checkpoint. The extra/removed quantity enters at current prices, so a
	// resize — including scaling a winner — generates zero ranked return.
	oldOpen := openPositions(all)
	newOpen := replaceInSet(oldOpen, positionID, &updated)
	input, err := s.rankedInput(ctx, existing.PortfolioID, userID, oldOpen, newOpen, newQuoteCache())
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdatePosition(&updated); err != nil {
		return nil, err
	}
	if err := s.applyRankedCheckpoint(ctx, input); err != nil {
		return nil, err
	}
	_ = s.recordArchiveSnapshot(ctx, userID)
	return &updated, nil
}

// DeletePosition deletes an open position the user owns, else ErrPositionNotFound.
// It remains mistake cleanup only; closed positions cannot be deleted to erase
// realized performance history.
func (s *Service) DeletePosition(ctx context.Context, userID, positionID string) error {
	pos, err := s.ownedPosition(userID, positionID)
	if err != nil {
		return err
	}
	if positionStatus(pos) != PositionStatusOpen {
		return ErrPositionClosed
	}

	all, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return err
	}
	// Removing a position drops its value from the segment but preserves the
	// accumulated ranked index (deleting a loser cannot erase its history; the
	// last remaining position pauses tracking rather than resetting it).
	oldOpen := openPositions(all)
	newOpen := removeFromSet(oldOpen, positionID)
	input, err := s.rankedInput(ctx, pos.PortfolioID, userID, oldOpen, newOpen, newQuoteCache())
	if err != nil {
		return err
	}
	if err := s.repo.DeletePosition(positionID); err != nil {
		return err
	}
	if err := s.applyRankedCheckpoint(ctx, input); err != nil {
		return err
	}
	_ = s.recordArchiveSnapshot(ctx, userID)
	return nil
}

func (s *Service) ClosePosition(ctx context.Context, userID, positionID string) (ClosedPositionSummary, error) {
	pos, err := s.ownedPosition(userID, positionID)
	if err != nil {
		return ClosedPositionSummary{}, err
	}
	if positionStatus(pos) != PositionStatusOpen {
		return ClosedPositionSummary{}, ErrPositionClosed
	}
	quote, err := s.provider.GetLatestPrice(ctx, pos.Symbol)
	if err != nil || quote == nil {
		return ClosedPositionSummary{}, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
	}
	costBase, err := s.fx.Convert(ctx, pos.Quantity*pos.AverageBuyPrice, pos.Currency, fx.BaseCurrency)
	if err != nil {
		return ClosedPositionSummary{}, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
	}
	closeBase, err := s.fx.Convert(ctx, pos.Quantity*quote.Price, quote.Currency, fx.BaseCurrency)
	if err != nil {
		return ClosedPositionSummary{}, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
	}
	realized := closeBase - costBase
	realizedPct := 0.0
	if costBase != 0 {
		realizedPct = realized / costBase * 100
	}
	// Closing removes the position from the active basket. Checkpoint so the
	// ranked index is preserved across the close (it is a mutation, not a market
	// move); the remaining positions continue the segment, or tracking pauses if
	// this was the last one.
	all, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return ClosedPositionSummary{}, err
	}
	cache := newQuoteCache()
	cache.seedPrice(pos.Symbol, quote)
	oldOpen := openPositions(all)
	newOpen := removeFromSet(oldOpen, positionID)
	input, err := s.rankedInput(ctx, pos.PortfolioID, userID, oldOpen, newOpen, cache)
	if err != nil {
		return ClosedPositionSummary{}, err
	}

	now := time.Now().UTC()
	closePrice := quote.Price
	pos.Status = PositionStatusClosed
	pos.ClosedAt = &now
	pos.ClosePrice = &closePrice
	pos.CloseCurrency = quote.Currency
	pos.RealizedGainLossBase = round2(realized)
	pos.RealizedGainLossPercentage = round2(realizedPct)
	pos.UpdatedAt = now
	if err := s.repo.UpdatePosition(pos); err != nil {
		return ClosedPositionSummary{}, err
	}
	if err := s.applyRankedCheckpoint(ctx, input); err != nil {
		return ClosedPositionSummary{}, err
	}
	view, err := s.closedPositionSummary(ctx, pos)
	if err != nil {
		return ClosedPositionSummary{}, err
	}
	_ = s.recordArchiveSnapshot(ctx, userID)
	return view, nil
}

// Summary fetches the user's positions, prices each through the provider, and
// returns the calculated portfolio summary. Any provider failure is wrapped in
// ErrPriceProvider.
func (s *Service) Summary(ctx context.Context, userID string) (*PortfolioSummary, error) {
	pf, err := s.GetOrCreateDefaultPortfolio(userID)
	if err != nil {
		return nil, err
	}

	positions, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return nil, err
	}

	summaries := make([]PositionSummary, 0, len(positions))
	closedSummaries := make([]ClosedPositionSummary, 0)
	for _, pos := range positions {
		if positionStatus(pos) == PositionStatusClosed {
			closed, err := s.closedPositionSummary(ctx, pos)
			if err != nil {
				return nil, err
			}
			closedSummaries = append(closedSummaries, closed)
			continue
		}
		price, err := s.provider.GetLatestPrice(ctx, pos.Symbol)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
		}

		costLocal := pos.Quantity * pos.AverageBuyPrice
		valueLocal := pos.Quantity * price.Price
		// Cost basis converts from the position's purchase currency; current
		// value converts from the price's quote currency.
		costBase, err := s.fx.Convert(ctx, costLocal, pos.Currency, fx.BaseCurrency)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
		}
		valueBase, err := s.fx.Convert(ctx, valueLocal, price.Currency, fx.BaseCurrency)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
		}

		summaries = append(summaries, CalculatePositionSummary(pos, price.Price, price.Currency, costBase, valueBase, fx.BaseCurrency))
		i := len(summaries) - 1
		summaries[i].QuoteProvider = price.Source
		summaries[i].QuoteProviderStatus = price.ProviderStatus
		summaries[i].QuoteIsStale = price.IsStale
		if !price.FetchedAt.IsZero() {
			summaries[i].QuoteFetchedAt = price.FetchedAt.Format(time.RFC3339)
		}
		if !price.ExpiresAt.IsZero() {
			summaries[i].QuoteExpiresAt = price.ExpiresAt.Format(time.RFC3339)
		}
	}

	summary := CalculatePortfolioSummary(userID, pf.ID, fx.BaseCurrency, summaries, closedSummaries)
	summary.QuoteStatus = summarizeQuoteStatus(summaries)
	return &summary, nil
}

func (s *Service) Archives(ctx context.Context, userID, rawTimeframe string) (*PortfolioArchives, error) {
	tf := parseArchiveTimeframe(rawTimeframe)
	now := time.Now().UTC()
	from := now.Add(-archiveWindow(tf))
	snapshots, err := s.repo.ListArchiveSnapshots(userID, from.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	points := make([]PortfolioArchivePoint, 0, len(snapshots))
	for _, s := range snapshots {
		points = append(points, PortfolioArchivePoint{
			CapturedAt:         s.CapturedAt.Format(time.RFC3339),
			PortfolioIndex:     round2(s.PortfolioIndex),
			GainLossPercentage: round2(s.GainLossPercentage),
		})
	}
	var earliest, latest *PortfolioArchiveSnapshotView
	if len(snapshots) > 0 {
		earliest = archiveSnapshotView(snapshots[0], false)
		latest = archiveSnapshotView(snapshots[len(snapshots)-1], true)
	}
	return &PortfolioArchives{
		Timeframe:        tf,
		From:             from.Format(time.RFC3339),
		To:               now.Format(time.RFC3339),
		Points:           points,
		EarliestSnapshot: earliest,
		LatestSnapshot:   latest,
	}, nil
}

func (s *Service) recordArchiveSnapshot(ctx context.Context, userID string) error {
	summary, err := s.Summary(ctx, userID)
	if err != nil {
		return err
	}
	return s.recordArchiveSnapshotFrom(summary)
}

// RecordDailySnapshot records at most one archive snapshot per user per UTC day,
// giving benchmark/achievement evaluation a continuous portfolio index series
// even when the user does not change their portfolio. It returns whether a new
// snapshot was written. Users with no active or closed positions are skipped.
func (s *Service) RecordDailySnapshot(ctx context.Context, userID string) (bool, error) {
	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// Use the full day as the range: RFC3339 bounds are second-precision, so a
	// bound of `now` would exclude a snapshot written earlier in the same second.
	dayEnd := dayStart.Add(24 * time.Hour)
	existing, err := s.repo.ListArchiveSnapshots(userID, dayStart.Format(time.RFC3339), dayEnd.Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	if len(existing) > 0 {
		return false, nil // already snapshotted today
	}
	summary, err := s.Summary(ctx, userID)
	if err != nil {
		return false, err
	}
	if len(summary.Positions) == 0 && len(summary.ClosedPositions) == 0 {
		return false, nil // nothing to track yet
	}
	if err := s.recordArchiveSnapshotFrom(summary); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) recordArchiveSnapshotFrom(summary *PortfolioSummary) error {
	snapshot := &PortfolioArchiveSnapshot{
		ID:                     uuid.NewString(),
		UserID:                 summary.UserID,
		PortfolioID:            summary.PortfolioID,
		CapturedAt:             time.Now().UTC(),
		BaseCurrency:           summary.BaseCurrency,
		PortfolioIndex:         summary.PortfolioIndex,
		GainLossPercentage:     summary.GainLossPercentage,
		TotalCostBasis:         summary.TotalCostBasis,
		CurrentValue:           summary.CurrentValue,
		UnrealizedGainLossBase: summary.UnrealizedGainLossBase,
		RealizedGainLossBase:   summary.RealizedGainLossBase,
		Positions:              append([]PositionSummary(nil), summary.Positions...),
		ClosedPositions:        append([]ClosedPositionSummary(nil), summary.ClosedPositions...),
	}
	return s.repo.CreateArchiveSnapshot(snapshot)
}

func (s *Service) closedPositionSummary(ctx context.Context, pos *Position) (ClosedPositionSummary, error) {
	costBase, err := s.fx.Convert(ctx, pos.Quantity*pos.AverageBuyPrice, pos.Currency, fx.BaseCurrency)
	if err != nil {
		return ClosedPositionSummary{}, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
	}
	closePrice := 0.0
	if pos.ClosePrice != nil {
		closePrice = *pos.ClosePrice
	}
	closedAt := ""
	if pos.ClosedAt != nil {
		closedAt = pos.ClosedAt.Format(time.RFC3339)
	}
	return ClosedPositionSummary{
		ID:                         pos.ID,
		Symbol:                     pos.Symbol,
		AssetType:                  pos.AssetType,
		Quantity:                   pos.Quantity,
		BaselinePrice:              pos.AverageBuyPrice,
		BaselineCurrency:           pos.Currency,
		ClosePrice:                 round2(closePrice),
		ClosePriceCurrency:         firstNonEmpty(pos.CloseCurrency, pos.Currency),
		ClosedAt:                   closedAt,
		RealizedGainLossBase:       round2(pos.RealizedGainLossBase),
		RealizedGainLossPercentage: round2(pos.RealizedGainLossPercentage),
		ClosedCostBasisBase:        round2(costBase),
		BaseCurrency:               fx.BaseCurrency,
	}, nil
}

func summarizeQuoteStatus(positions []PositionSummary) QuoteStatus {
	status := QuoteStatus{TotalQuotes: len(positions)}
	for _, p := range positions {
		if status.Provider == "" && p.QuoteProvider != "" {
			status.Provider = p.QuoteProvider
		}
		if p.QuoteIsStale {
			status.StaleCount++
		}
		if p.QuoteProviderStatus != "" && (status.ProviderStatus == "" || p.QuoteIsStale) {
			status.ProviderStatus = p.QuoteProviderStatus
		}
		if p.QuoteFetchedAt > status.LastFetchedAt {
			status.LastFetchedAt = p.QuoteFetchedAt
		}
	}
	if status.Provider == "" {
		status.Provider = "mock"
	}
	if status.ProviderStatus == "" {
		status.ProviderStatus = "ok"
	}
	return status
}

func archiveSnapshotView(s *PortfolioArchiveSnapshot, includePrivateDetails bool) *PortfolioArchiveSnapshotView {
	view := &PortfolioArchiveSnapshotView{
		CapturedAt:         s.CapturedAt.Format(time.RFC3339),
		PortfolioIndex:     round2(s.PortfolioIndex),
		GainLossPercentage: round2(s.GainLossPercentage),
	}
	if includePrivateDetails {
		view.TotalCostBasis = round2(s.TotalCostBasis)
		view.CurrentValue = round2(s.CurrentValue)
		view.UnrealizedGainLossBase = round2(s.UnrealizedGainLossBase)
		view.RealizedGainLossBase = round2(s.RealizedGainLossBase)
		view.Positions = append([]PositionSummary(nil), s.Positions...)
		view.ClosedPositions = append([]ClosedPositionSummary(nil), s.ClosedPositions...)
	}
	return view
}

func parseArchiveTimeframe(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case ArchiveTimeframe1W:
		return ArchiveTimeframe1W
	case ArchiveTimeframe3M:
		return ArchiveTimeframe3M
	case ArchiveTimeframe6M:
		return ArchiveTimeframe6M
	case ArchiveTimeframe1Y:
		return ArchiveTimeframe1Y
	default:
		return ArchiveTimeframe1M
	}
}

func archiveWindow(tf string) time.Duration {
	const day = 24 * time.Hour
	switch tf {
	case ArchiveTimeframe1W:
		return 7 * day
	case ArchiveTimeframe3M:
		return 90 * day
	case ArchiveTimeframe6M:
		return 180 * day
	case ArchiveTimeframe1Y:
		return 365 * day
	default:
		return 30 * day
	}
}

func filterPositionsByStatus(positions []*Position, status string) []*Position {
	out := make([]*Position, 0, len(positions))
	for _, p := range positions {
		if positionStatus(p) == status {
			out = append(out, p)
		}
	}
	return out
}

func positionStatus(p *Position) string {
	if p == nil || p.Status == "" {
		return PositionStatusOpen
	}
	return p.Status
}

func firstNonEmptyStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return PositionStatusOpen
	}
	return status
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// ownedPosition fetches a position and confirms it belongs to userID. Both a
// missing position and a foreign one collapse to ErrPositionNotFound so the API
// never discloses another user's data.
func (s *Service) ownedPosition(userID, positionID string) (*Position, error) {
	pos, err := s.repo.GetPosition(positionID)
	if err != nil {
		return nil, ErrPositionNotFound
	}
	if pos.UserID != userID {
		return nil, ErrPositionNotFound
	}
	return pos, nil
}

// validatePosition runs format validation, fetches the current quote (which is
// both the priceability gate AND the baseline to lock), and confirms the quote
// currency is FX-convertible. This keeps unpriceable tickers and unknown
// currencies out of the repository so they can never later break summaries or
// leaderboards.
func (s *Service) validatePosition(ctx context.Context, in PositionInput) (PositionInput, *prices.Price, error) {
	clean, err := validateAndNormalize(in)
	if err != nil {
		return PositionInput{}, nil, err
	}
	quote, err := s.provider.GetLatestPrice(ctx, clean.Symbol)
	if err != nil || quote == nil {
		return PositionInput{}, nil, ErrUnsupportedSymbol
	}
	if _, err := s.fx.GetRate(ctx, quote.Currency, fx.BaseCurrency); err != nil {
		return PositionInput{}, nil, ErrUnsupportedCurrency
	}
	return clean, quote, nil
}

type preparedStrategyPosition struct {
	Symbol    string
	AssetType string
	Quantity  float64
	Price     float64
	Currency  string
}

func (s *Service) prepareStrategyWeights(ctx context.Context, weights []StrategyWeightInput) ([]preparedStrategyPosition, error) {
	if len(weights) == 0 {
		return nil, ErrInvalidWeights
	}

	seen := map[string]bool{}
	var total float64
	out := make([]preparedStrategyPosition, 0, len(weights))

	for _, item := range weights {
		symbol, err := prices.ValidateAndNormalizeSymbol(item.Symbol)
		if err != nil {
			return nil, ErrUnsupportedSymbol
		}
		if seen[symbol] {
			return nil, ErrInvalidWeights
		}
		seen[symbol] = true

		assetType := strings.ToLower(strings.TrimSpace(item.AssetType))
		if !validAssetTypes[assetType] {
			return nil, ErrInvalidAssetType
		}
		if item.WeightPercentage <= 0 || !isFinite(item.WeightPercentage) {
			return nil, ErrInvalidWeights
		}
		total += item.WeightPercentage

		quote, err := s.provider.GetLatestPrice(ctx, symbol)
		if err != nil || quote == nil || quote.Price <= 0 {
			return nil, ErrUnsupportedSymbol
		}
		rate, err := s.fx.GetRate(ctx, quote.Currency, fx.BaseCurrency)
		if err != nil || rate <= 0 {
			return nil, ErrUnsupportedCurrency
		}
		quantity := item.WeightPercentage / (quote.Price * rate)
		if quantity <= 0 || !isFinite(quantity) {
			return nil, ErrInvalidWeights
		}
		out = append(out, preparedStrategyPosition{
			Symbol: symbol, AssetType: assetType, Quantity: quantity,
			Price: quote.Price, Currency: quote.Currency,
		})
	}
	if math.Abs(total-100) > 0.5 {
		return nil, ErrInvalidWeights
	}
	return out, nil
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// validateAndNormalize checks a PositionInput and returns a normalized copy
// (symbol upper-cased, whitespace trimmed). It enforces the safe-symbol format
// but does NOT check priceability.
func validateAndNormalize(in PositionInput) (PositionInput, error) {
	if strings.TrimSpace(in.Symbol) == "" {
		return PositionInput{}, ErrSymbolRequired
	}
	symbol, err := prices.ValidateAndNormalizeSymbol(in.Symbol)
	if err != nil {
		return PositionInput{}, ErrUnsupportedSymbol
	}
	assetType := strings.ToLower(strings.TrimSpace(in.AssetType))
	if !validAssetTypes[assetType] {
		return PositionInput{}, ErrInvalidAssetType
	}
	if in.Quantity <= 0 {
		return PositionInput{}, ErrInvalidQuantity
	}

	return PositionInput{
		Symbol:    symbol,
		AssetType: assetType,
		Quantity:  in.Quantity,
	}, nil
}
