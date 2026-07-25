package portfolio

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// Service holds the portfolio business logic: queries, private monetary
// summaries and public-safe composition.
//
// It owns NO write path of its own. Every position mutation is delegated to the
// MutationCoordinator, which commits the position change together with the
// ranked-performance checkpoint, the aggregate version, the audit record and the
// outbox event in a single transaction.
type Service struct {
	repo        Repository
	provider    prices.PriceProvider
	fx          fx.FXProvider
	coordinator *MutationCoordinator
	ranked      RankedPerformanceProvider
}

type RankedPerformanceProvider interface {
	CurrentRankedPerformance(ctx context.Context, userID string) (*performance.RankedPerformance, error)
}

// NewService wires a Service with its repository, price provider, and FX
// provider. When the repository implements AggregateStore (both the in-memory
// and Postgres implementations do), the transactional mutation coordinator is
// wired automatically — there is no configuration under which mutations bypass
// the aggregate boundary.
func NewService(repo Repository, provider prices.PriceProvider, fxp fx.FXProvider) *Service {
	s := &Service{repo: repo, provider: provider, fx: fxp}
	if store, ok := repo.(AggregateStore); ok {
		s.coordinator = NewMutationCoordinator(store, repo, provider, fxp)
	}
	if states, ok := repo.(performance.StateReader); ok {
		ranked := performance.NewService(states)
		ranked.SetValuator(s)
		s.ranked = ranked
	}
	return s
}

func (s *Service) SetRankedPerformanceProvider(provider RankedPerformanceProvider) {
	s.ranked = provider
}

// Coordinator exposes the mutation boundary (used by the outbox processor and
// tests). It is nil only for a repository that is not an AggregateStore.
func (s *Service) Coordinator() *MutationCoordinator { return s.coordinator }

// GetOrCreateDefaultPortfolio returns the user's portfolio, creating the default
// one on first access. Creation is race-safe: concurrent first requests converge
// on one portfolio (UNIQUE (user_id) in Postgres, user index in memory).
func (s *Service) GetOrCreateDefaultPortfolio(ctx context.Context, userID string) (*Portfolio, error) {
	return s.repo.EnsureDefaultPortfolio(ctx, userID)
}

// Mutate is the single entry point for every position mutation. Callers that
// have an idempotency key (e.g. the Idempotency-Key header) set RequestID so a
// retry replays the original committed result instead of applying twice.
func (s *Service) Mutate(ctx context.Context, req MutationRequest) (MutationResult, error) {
	if s.coordinator == nil {
		return MutationResult{}, ErrUnsupportedMutation
	}
	return s.coordinator.Apply(ctx, req)
}

// AddPosition creates a position in the user's default portfolio, LOCKING the
// baseline at the current market price. The client never supplies a price or
// currency. The position write and the ranked checkpoint commit together, and
// the added capital enters the ranked segment at its current value — so the add
// itself generates exactly zero ranked return.
func (s *Service) AddPosition(ctx context.Context, userID string, in PositionInput) (*Position, error) {
	res, err := s.Mutate(ctx, MutationRequest{Kind: MutationAdd, UserID: userID, Input: in})
	if err != nil {
		return nil, err
	}
	return res.Position, nil
}

func (s *Service) DepositCash(ctx context.Context, userID, requestID string, input CashFlowInput) (MutationResult, error) {
	return s.Mutate(ctx, MutationRequest{
		Kind: MutationDeposit, UserID: userID, RequestID: requestID, CashFlow: input,
	})
}

func (s *Service) WithdrawCash(ctx context.Context, userID, requestID string, input CashFlowInput) (MutationResult, error) {
	return s.Mutate(ctx, MutationRequest{
		Kind: MutationWithdrawal, UserID: userID, RequestID: requestID, CashFlow: input,
	})
}

// CorrectActivity reconciles a user-recorded deposit or withdrawal to its
// actual amount. The correction flow begins from the Activity layer: the
// original activity is never edited, and this posts a compensating
// deposit/withdrawal for the delta, linked back via metadata
// (correction_of_activity_id). Buy/sell activities are rejected —
// retroactively adjusting a trade's quantity or price could conflict with
// activity that happened afterward (partial sells, closures, rebuys); the
// user should record an offsetting sell/buy instead.
func (s *Service) CorrectActivity(ctx context.Context, userID, requestID string, input ActivityCorrectionInput) (MutationResult, error) {
	activities, err := s.repo.ListActivities(ctx, userID, 100000)
	if err != nil {
		return MutationResult{}, err
	}
	var original *Activity
	for i := range activities {
		if activities[i].ID == input.ActivityID {
			original = &activities[i]
			break
		}
	}
	if original == nil {
		return MutationResult{}, ErrActivityNotFound
	}
	for _, a := range activities {
		if correctionOf, _ := a.Metadata["correction_of_activity_id"].(string); correctionOf == original.ID {
			return MutationResult{}, ErrActivityAlreadyCorrected
		}
	}

	switch original.Type {
	case ActivityDeposit, ActivityWithdrawal:
		if input.ActualAmount <= 0 {
			return MutationResult{}, ErrInvalidCashAmount
		}
		delta := input.ActualAmount - original.GrossAmount
		if delta == 0 {
			return MutationResult{}, ErrNothingToCorrect
		}
		cf := CashFlowInput{
			Currency: original.Currency, Amount: math.Abs(delta),
			CorrectionOf: original.ID, CorrectionReason: input.Reason,
		}
		depositDirection := (original.Type == ActivityDeposit && delta > 0) ||
			(original.Type == ActivityWithdrawal && delta < 0)
		if depositDirection {
			return s.DepositCash(ctx, userID, requestID, cf)
		}
		return s.WithdrawCash(ctx, userID, requestID, cf)
	default:
		return MutationResult{}, ErrCorrectionNotSupported
	}
}

func (s *Service) BuyPosition(ctx context.Context, userID, requestID string, input BuyInput) (MutationResult, error) {
	return s.Mutate(ctx, MutationRequest{
		Kind: MutationBuy, UserID: userID, RequestID: requestID, Buy: input,
	})
}

func (s *Service) SellPosition(ctx context.Context, userID, requestID string, input SellInput) (MutationResult, error) {
	return s.Mutate(ctx, MutationRequest{
		Kind: MutationSell, UserID: userID, RequestID: requestID, Sell: input,
	})
}

func (s *Service) CashBalances(ctx context.Context, userID string) ([]CashBalanceView, float64, error) {
	balances, err := s.repo.ListCashBalances(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	views := make([]CashBalanceView, 0, len(balances))
	var total float64
	for _, balance := range balances {
		value, err := s.fx.Convert(ctx, balance.Amount, balance.Currency, fx.BaseCurrency)
		if err != nil {
			return nil, 0, ErrUnsupportedCurrency
		}
		total += value
		views = append(views, CashBalanceView{
			Currency: balance.Currency, Amount: round2(balance.Amount), ValueBase: round2(value),
		})
	}
	return views, total, nil
}

func (s *Service) Activities(ctx context.Context, userID string, limit int) ([]ActivityView, error) {
	activities, err := s.repo.ListActivities(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityView, 0, len(activities))
	for _, activity := range activities {
		out = append(out, activityView(activity))
	}
	return out, nil
}

func (s *Service) ActivityList(ctx context.Context, userID, category, symbol string, limit, offset int) (ActivityListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	activities, err := s.repo.ListActivities(ctx, userID, 100000)
	if err != nil {
		return ActivityListResponse{}, err
	}
	category = strings.ToLower(strings.TrimSpace(category))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	filtered := make([]ActivityView, 0, len(activities))
	for _, activity := range activities {
		if symbol != "" && normalizeSymbol(activity.Symbol) != symbol {
			continue
		}
		if category != "" && category != "all" && activityCategory(activity.Type) != category {
			continue
		}
		filtered = append(filtered, activityView(activity))
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	var next *int
	if end < total {
		value := end
		next = &value
	}
	return ActivityListResponse{Items: filtered[offset:end], NextOffset: next, Total: total}, nil
}

func (s *Service) ActivityDetail(ctx context.Context, userID, activityID string) (ActivityView, error) {
	activities, err := s.repo.ListActivities(ctx, userID, 100000)
	if err != nil {
		return ActivityView{}, err
	}
	for _, activity := range activities {
		if activity.ID == activityID {
			return activityView(activity), nil
		}
	}
	return ActivityView{}, ErrPositionNotFound
}

func activityView(activity Activity) ActivityView {
	origin := activity.Origin
	if origin == "" {
		origin = activityOrigin(activity)
	}
	status := activity.Status
	if status == "" {
		status = "completed"
	}
	groupID := activity.GroupID
	if groupID == "" {
		groupID, _ = activity.Metadata["activity_group_id"].(string)
	}
	episodeID := activity.PositionEpisodeID
	if episodeID == "" {
		episodeID, _ = activity.Metadata["position_episode_id"].(string)
	}
	return ActivityView{
		ID: activity.ID, Type: activity.Type, Symbol: activity.Symbol,
		AssetType: activity.AssetType, Currency: activity.Currency,
		Quantity: activity.Quantity, UnitPrice: activity.UnitPrice,
		GrossAmount:                round2(activity.GrossAmount),
		CostBasisAllocated:         activity.CostBasisAllocated,
		RealizedGainLossBase:       activity.RealizedGainLossBase,
		RealizedGainLossPercentage: activity.RealizedGainLossPercentage,
		OccurredAt:                 activity.OccurredAt.Format(time.RFC3339),
		PortfolioVersion:           activity.PortfolioVersion,
		Origin:                     origin,
		Status:                     status,
		GroupID:                    groupID,
		PositionEpisodeID:          episodeID,
		FeeAmount:                  round2(activity.FeeAmount),
		NetAmount:                  round2(activity.NetAmount),
	}
}

func activityOrigin(activity Activity) string {
	if provenance, ok := activity.Metadata["provenance"].(string); ok {
		switch provenance {
		case string(ProvenanceProviderReported):
			return "provider_generated"
		case string(ProvenanceSystemGenerated):
			return "system_generated"
		case string(ProvenanceMigrationGenerated):
			return "migration_generated"
		}
	}
	if legacy, _ := activity.Metadata["legacy_import"].(bool); legacy || activity.Type == ActivityOpeningBalance {
		return "migration_generated"
	}
	switch activity.Type {
	case ActivityCashDividend, ActivityETFDistribution, ActivityInterestIncome,
		ActivityReinvestedDividend, ActivityStockSplit, ActivityReverseSplit,
		ActivitySymbolChange, ActivityStockDividend:
		return "system_generated"
	default:
		return "user_recorded"
	}
}

func activityCategory(kind ActivityType) string {
	switch kind {
	case ActivityDeposit, ActivityWithdrawal, ActivityOpeningBalance:
		return "cash_flows"
	case ActivityBuy, ActivitySell:
		return "trades"
	case ActivityCashDividend, ActivityETFDistribution, ActivityInterestIncome,
		ActivityReinvestedDividend, ActivityReturnOfCapital, ActivityStockDividend:
		return "income"
	case ActivityBuyFee, ActivitySellFee, ActivityManagementFee, ActivityCustodyFee, ActivityOtherFee:
		return "fees"
	case ActivityStockSplit, ActivityReverseSplit, ActivitySymbolChange, ActivityWriteOff:
		return "automatic_adjustments"
	default:
		return "all"
	}
}

func (s *Service) PreviewSell(ctx context.Context, userID string, input SellInput) (SellPreview, error) {
	position, err := s.repo.GetPosition(ctx, strings.TrimSpace(input.PositionID))
	if err != nil || position.UserID != userID {
		return SellPreview{}, ErrPositionNotFound
	}
	if positionStatus(position) != PositionStatusOpen {
		return SellPreview{}, ErrPositionClosed
	}
	if !finitePositive(input.Quantity) || input.Quantity > position.Quantity+1e-9 {
		return SellPreview{}, ErrInvalidSaleQuantity
	}
	price := input.ExecutionPrice
	if price == 0 {
		quote, quoteErr := s.provider.GetLatestPrice(ctx, position.Symbol)
		if quoteErr != nil || quote == nil {
			return SellPreview{}, ErrPriceProvider
		}
		price = quote.Price
	}
	if !finitePositive(price) {
		return SellPreview{}, ErrInvalidSalePrice
	}
	gross := input.Quantity * price
	if !isFinite(input.Fee) || input.Fee < 0 || input.Fee >= gross {
		return SellPreview{}, ErrInvalidSaleFee
	}
	remaining := position.Quantity - input.Quantity
	if remaining <= 1e-8 {
		remaining = 0
	}
	allocatedBasis := input.Quantity * position.AverageBuyPrice
	net := gross - input.Fee
	realizedLocal := net - allocatedBasis
	realizedBase, err := s.fx.Convert(ctx, realizedLocal, position.Currency, fx.BaseCurrency)
	if err != nil {
		return SellPreview{}, ErrUnsupportedCurrency
	}
	return SellPreview{
		PositionID: position.ID, PositionEpisodeID: position.ID, Symbol: position.Symbol,
		AvailableQuantity: round2(position.Quantity), SoldQuantity: round2(input.Quantity),
		RemainingQuantity: round2(remaining), ExecutionPrice: round2(price),
		GrossProceeds: round2(gross), Fee: round2(input.Fee), NetProceeds: round2(net),
		AllocatedBasis: round2(allocatedBasis), EstimatedRealizedPnL: round2(realizedBase),
		WillClosePosition: remaining == 0, ProceedsCurrency: position.Currency,
		BaseCurrency: fx.BaseCurrency,
	}, nil
}

// UpdatePosition updates the QUANTITY of a position the user owns. Ownership and
// open status are verified INSIDE the transaction, against the locked position
// set. The added or removed quantity enters at current prices, so resizing —
// including scaling a winner — cannot create retroactive gains.
func (s *Service) UpdatePosition(ctx context.Context, userID, positionID string, quantity float64) (*Position, error) {
	res, err := s.Mutate(ctx, MutationRequest{
		Kind: MutationResize, UserID: userID, PositionID: positionID, Quantity: quantity,
	})
	if err != nil {
		return nil, err
	}
	return res.Position, nil
}

// DeletePosition removes an open position. The accumulated ranked index is
// preserved (deleting a loser cannot erase its history); removing the final
// position pauses ranked tracking rather than resetting it.
func (s *Service) DeletePosition(ctx context.Context, userID, positionID string) error {
	_, err := s.Mutate(ctx, MutationRequest{
		Kind: MutationDelete, UserID: userID, PositionID: positionID,
	})
	return err
}

// ClosePosition realizes a position. The realized result and the ranked
// checkpoint are computed from the SAME pinned price/FX observation and commit
// together, so a close can never be recorded without its checkpoint.
func (s *Service) ClosePosition(ctx context.Context, userID, positionID string) (ClosedPositionSummary, error) {
	res, err := s.Mutate(ctx, MutationRequest{
		Kind: MutationClose, UserID: userID, PositionID: positionID,
	})
	if err != nil {
		return ClosedPositionSummary{}, err
	}
	if res.Closed == nil {
		return ClosedPositionSummary{}, ErrPositionNotFound
	}
	return *res.Closed, nil
}

// ReplaceWithStrategyWeights replaces the portfolio from public percentage
// weights. The entire target allocation is validated and priced before anything
// destructive happens, the swap is atomic (closed history preserved), and the
// accumulated ranked index carries over into a new segment — copying a strategy
// can never reset ranked history to 100.
func (s *Service) ReplaceWithStrategyWeights(ctx context.Context, userID string, weights []StrategyWeightInput) error {
	_, err := s.Mutate(ctx, MutationRequest{
		Kind: MutationReplace, UserID: userID, Weights: weights,
	})
	return err
}

// ListPositions returns the requesting user's open positions only.
func (s *Service) ListPositions(ctx context.Context, userID string) ([]*Position, error) {
	positions, err := s.repo.ListPositionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return filterPositionsByStatus(positions, PositionStatusOpen), nil
}

func (s *Service) ListClosedPositions(ctx context.Context, userID string) ([]ClosedPositionSummary, error) {
	positions, err := s.repo.ListPositionsByUser(ctx, userID)
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

// Summary prices the user's positions and returns the private portfolio summary.
// PortfolioIndex here is the CURRENT-BASKET figure (value vs the sum of position
// baselines) — a private display value. It is NOT the ranked career index; that
// comes only from the performance service.
func (s *Service) Summary(ctx context.Context, userID string) (*PortfolioSummary, error) {
	pf, err := s.GetOrCreateDefaultPortfolio(ctx, userID)
	if err != nil {
		return nil, err
	}

	positions, err := s.repo.ListPositionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	cashViews, totalCash, err := s.CashBalances(ctx, userID)
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
	activities, err := s.repo.ListActivities(ctx, userID, 100000)
	if err != nil {
		return nil, err
	}
	ledger, err := s.summarizeLedger(ctx, activities)
	if err != nil {
		return nil, err
	}
	summary.CashBalances = cashViews
	summary.TotalCashValueBase = round2(totalCash)
	summary.CurrentValue = round2(summary.ActiveCurrentValueBase + totalCash)
	summary.RealizedGainLossBase = round2(ledger.realized)

	// Deprecated compatibility aliases now have one precise scope: open
	// holdings only. They are not total P&L and never use ranked return.
	summary.TotalCostBasis = round2(summary.ActiveCostBasisBase)
	summary.GainLoss = round2(summary.UnrealizedGainLossBase)
	if summary.ActiveCostBasisBase > 0 {
		summary.GainLossPercentage = round2(summary.UnrealizedGainLossBase / summary.ActiveCostBasisBase * 100)
	} else {
		summary.GainLossPercentage = 0
	}

	ranked := RankedPerformanceView{Index: 100, ReturnPercentage: 0, TrackingStatus: "unavailable"}
	if s.ranked != nil {
		current, rankedErr := s.ranked.CurrentRankedPerformance(ctx, userID)
		if rankedErr != nil {
			return nil, rankedErr
		}
		ranked = RankedPerformanceView{
			Index: round2(current.RankedIndex), ReturnPercentage: round2(current.RankedReturnPercentage),
			TrackingStatus: string(current.Status),
		}
	}
	summary.PortfolioIndex = ranked.Index

	summary.RankedPerformance = ranked
	summary.Valuation = PortfolioValuation{
		OpenHoldingsMarketValueBase: round2(summary.ActiveCurrentValueBase),
		CashValueBase:               round2(totalCash),
		CurrentPortfolioValueBase:   round2(summary.CurrentValue),
	}
	summary.OpenHoldings = CalculateUnrealizedMetrics(
		summary.ActiveCurrentValueBase, summary.ActiveCostBasisBase,
	)
	summary.Realized = RealizedMetrics{RealizedPnLBase: round2(ledger.realized)}
	summary.Income = ledger.income
	summary.Fees = ledger.fees
	summary.EconomicPerformance = calculateEconomicPerformance(
		summary.CurrentValue, ledger, len(summaries)+len(closedSummaries) > 0,
	)
	summary.Reconciliation = ReconcilePortfolioFinancials(
		summary.RankedPerformance, summary.Valuation, summary.OpenHoldings,
		summary.Realized, summary.Income, summary.Fees, summary.EconomicPerformance,
	)
	if !summary.Reconciliation.IsConsistent {
		slog.Warn("portfolio metric reconciliation failed",
			"portfolio_id", pf.ID,
			"reason_codes", summary.Reconciliation.Reasons,
		)
	} else if !summary.EconomicPerformance.IsComplete {
		slog.Info("portfolio total pnl is incomplete",
			"portfolio_id", pf.ID,
			"calculation_status", summary.EconomicPerformance.CalculationStatus,
		)
	}
	if summary.CurrentValue > 0 {
		for i := range summary.CashBalances {
			summary.CashBalances[i].WeightPercentage =
				round2(summary.CashBalances[i].ValueBase / summary.CurrentValue * 100)
		}
	}
	summary.QuoteStatus = summarizeQuoteStatus(summaries)
	return &summary, nil
}

type ledgerMetrics struct {
	deposits, withdrawals, opening, realized float64
	income                                   IncomeMetrics
	fees                                     FeeMetrics
	hasBuy, hasOpening                       bool
}

func (s *Service) summarizeLedger(ctx context.Context, activities []Activity) (ledgerMetrics, error) {
	var result ledgerMetrics
	for _, activity := range activities {
		amountBase, err := s.fx.Convert(ctx, activity.GrossAmount, activity.Currency, fx.BaseCurrency)
		if err != nil {
			return ledgerMetrics{}, fmt.Errorf("%w: ledger %s: %v", ErrPriceProvider, activity.Type, err)
		}
		switch activity.Type {
		case ActivityDeposit:
			result.deposits += amountBase
		case ActivityWithdrawal:
			result.withdrawals += amountBase
		case ActivityOpeningBalance:
			result.opening += amountBase
			result.hasOpening = true
		case ActivityBuy:
			result.hasBuy = true
		case ActivitySell:
			if activity.RealizedGainLossBase != nil {
				result.realized += *activity.RealizedGainLossBase
			}
		case ActivityCashDividend, ActivityReinvestedDividend:
			result.income.DividendsBase += amountBase
		case ActivityETFDistribution, ActivityCapitalGainsDistribution, ActivityReturnOfCapital:
			result.income.DistributionsBase += amountBase
		case ActivityInterestIncome, ActivityBondCoupon, ActivityCashInterest:
			result.income.InterestBase += amountBase
		case ActivityStakingReward, ActivityOtherIncome:
			result.income.OtherIncomeBase += amountBase
		case ActivityBuyFee, ActivitySellFee:
			result.fees.TransactionFeesBase += amountBase
		case ActivityManagementFee:
			result.fees.ManagementFeesBase += amountBase
		case ActivityCustodyFee:
			result.fees.CustodyFeesBase += amountBase
		case ActivityOtherFee:
			result.fees.OtherFeesBase += amountBase
		}
	}
	result.income.DividendsBase = round2(result.income.DividendsBase)
	result.income.DistributionsBase = round2(result.income.DistributionsBase)
	result.income.InterestBase = round2(result.income.InterestBase)
	result.income.OtherIncomeBase = round2(result.income.OtherIncomeBase)
	result.income.TotalIncomeBase = round2(
		result.income.DividendsBase + result.income.DistributionsBase +
			result.income.InterestBase + result.income.OtherIncomeBase,
	)
	result.fees.TransactionFeesBase = round2(result.fees.TransactionFeesBase)
	result.fees.ManagementFeesBase = round2(result.fees.ManagementFeesBase)
	result.fees.CustodyFeesBase = round2(result.fees.CustodyFeesBase)
	result.fees.OtherFeesBase = round2(result.fees.OtherFeesBase)
	result.fees.TotalFeesBase = round2(
		result.fees.TransactionFeesBase + result.fees.ManagementFeesBase +
			result.fees.CustodyFeesBase + result.fees.OtherFeesBase,
	)
	result.deposits = round2(result.deposits)
	result.withdrawals = round2(result.withdrawals)
	result.opening = round2(result.opening)
	result.realized = round2(result.realized)
	return result, nil
}

func calculateEconomicPerformance(currentValue float64, ledger ledgerMetrics, hasPositions bool) EconomicPerformance {
	if ledger.hasOpening {
		return EconomicPerformance{CalculationStatus: "legacy_estimate", IsComplete: false}
	}
	if hasPositions && !ledger.hasBuy {
		return EconomicPerformance{CalculationStatus: "insufficient_history", IsComplete: false}
	}

	netContributions := ledger.deposits - ledger.withdrawals
	totalPnL := currentValue + ledger.withdrawals - ledger.deposits
	result := EconomicPerformance{
		TotalPnLBase: &totalPnL, NetContributionsBase: &netContributions,
		CalculationStatus: "complete", IsComplete: true,
	}
	if netContributions > 0 {
		returnPercentage := totalPnL / netContributions * 100
		result.ReturnPercentage = &returnPercentage
	}
	result.TotalPnLBase = roundedPointer(result.TotalPnLBase)
	result.NetContributionsBase = roundedPointer(result.NetContributionsBase)
	result.ReturnPercentage = roundedPointer(result.ReturnPercentage)
	return result
}

func (s *Service) Archives(ctx context.Context, userID, rawTimeframe string) (*PortfolioArchives, error) {
	tf := parseArchiveTimeframe(rawTimeframe)
	now := time.Now().UTC()
	from := now.Add(-archiveWindow(tf))
	snapshots, err := s.repo.ListArchiveSnapshots(ctx, userID, from.Format(time.RFC3339), now.Format(time.RFC3339))
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

// RecordDailySnapshot records at most one archive snapshot per portfolio per UTC
// day. Uniqueness is enforced by the DATABASE, not by a check-then-insert, so
// concurrent workers or multiple instances cannot create duplicates. It returns
// whether a new snapshot was written.
func (s *Service) RecordDailySnapshot(ctx context.Context, userID string) (bool, error) {
	summary, err := s.Summary(ctx, userID)
	if err != nil {
		return false, err
	}
	if len(summary.Positions) == 0 && len(summary.ClosedPositions) == 0 &&
		summary.TotalCashValueBase == 0 {
		return false, nil // nothing to track yet
	}
	return s.recordArchiveSnapshotFrom(ctx, summary)
}

func (s *Service) recordArchiveSnapshotFrom(ctx context.Context, summary *PortfolioSummary) (bool, error) {
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
		CashBalances:           append([]CashBalanceView(nil), summary.CashBalances...),
		TotalCashValueBase:     summary.TotalCashValueBase,
	}
	return s.repo.CreateArchiveSnapshot(ctx, snapshot)
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
		view.CashBalances = append([]CashBalanceView(nil), s.CashBalances...)
		view.TotalCashValueBase = round2(s.TotalCashValueBase)
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
	out := make([]*Position, 0)
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

// validateAndNormalize checks a PositionInput and returns a normalized copy
// (symbol upper-cased, whitespace trimmed). It enforces the safe-symbol format
// but does NOT check priceability — the coordinator does that when it pins the
// quote, before any database write.
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
	if !finitePositive(in.Quantity) {
		return PositionInput{}, ErrInvalidQuantity
	}

	return PositionInput{
		Symbol:    symbol,
		AssetType: assetType,
		Quantity:  in.Quantity,
	}, nil
}
