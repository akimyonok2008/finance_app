package income

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// testGateway wires the income pipeline to a REAL portfolio service so tests
// assert genuine cash credits, ranked-performance effects, and immutable ledger
// activity — not just recorded calls. It mirrors the production incomeGateway.
type testGateway struct{ svc *portfolio.Service }

func (g testGateway) DiscoveryInstruments(ctx context.Context, since time.Time) ([]DiscoveryInstrument, error) {
	items, err := g.svc.IncomeDiscoveryInstruments(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]DiscoveryInstrument, 0, len(items))
	for _, item := range items {
		out = append(out, DiscoveryInstrument{InstrumentID: item.InstrumentID, Symbol: item.Symbol, AssetType: item.AssetType})
	}
	return out, nil
}

func (g testGateway) HistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]Holder, error) {
	holders, err := g.svc.IncomeHistoricalHolders(ctx, instrumentID, symbol)
	if err != nil {
		return nil, err
	}
	out := make([]Holder, 0, len(holders))
	for _, h := range holders {
		out = append(out, Holder{UserID: h.UserID, PortfolioID: h.PortfolioID, AssetType: h.AssetType, AcquiredAt: h.AcquiredAt})
	}
	return out, nil
}

func (g testGateway) EligibleQuantity(ctx context.Context, userID, instrumentID, symbol string, asOf time.Time) (money.Quantity, error) {
	return g.svc.EligibleQuantity(ctx, userID, instrumentID, symbol, asOf)
}

func (g testGateway) ApplyIncome(ctx context.Context, userID, requestID string, in AppliedIncome) error {
	at := in.PaymentDate
	base := portfolio.IncomeInput{
		Symbol: in.Symbol, AssetType: in.AssetType, Currency: in.Currency,
		Amount: in.Gross, Withholding: in.Withholding, Fee: in.Fee, Estimated: in.Estimated,
		ReinvestPrice: in.ReinvestPrice, PriceMethod: in.PriceMethod, IncomeEventID: in.IncomeEventID,
		TaxClassification: in.TaxClassification, OccurredAt: &at, Provenance: portfolio.ProvenanceProviderReported,
	}
	switch in.Classification {
	case ClassStockDividend:
		base.Subtype = portfolio.IncomeStockDividendSub
		base.StockRatioNum, base.StockRatioDen = in.StockRatioNum, in.StockRatioDen
	case ClassReturnOfCapital:
		base.Subtype = portfolio.IncomeReturnOfCapitalSub
	default:
		if in.Reinvest {
			base.Subtype = portfolio.IncomeReinvestedDiv
		} else {
			base.Subtype = mapType(in.Type)
		}
	}
	_, err := g.svc.RecordIncome(ctx, userID, requestID, base)
	return err
}

func (g testGateway) ApplyCorrection(ctx context.Context, userID, requestID string, adj CorrectionAdjustment) error {
	now := time.Now().UTC()
	if adj.Delta.Sign() >= 0 {
		_, err := g.svc.RecordIncome(ctx, userID, requestID, portfolio.IncomeInput{
			Subtype: portfolio.IncomeOtherProvider, Symbol: adj.Symbol, Currency: adj.Currency,
			Amount: adj.Delta, OccurredAt: &now, Provenance: portfolio.ProvenanceSystemGenerated,
		})
		return err
	}
	_, err := g.svc.RecordFee(ctx, userID, requestID, portfolio.FeeInput{
		Subtype: portfolio.FeeOther, Currency: adj.Currency, Amount: adj.Delta.Neg(), Symbol: adj.Symbol,
		Description: "income correction", OccurredAt: &now, Provenance: portfolio.ProvenanceSystemGenerated,
	})
	return err
}

func mapType(t Type) portfolio.IncomeSubtype {
	switch t {
	case TypeETFDistribution:
		return portfolio.IncomeETFDistribution
	case TypeCapitalGainsDist:
		return portfolio.IncomeCapitalGainsDist
	case TypeBondCoupon:
		return portfolio.IncomeBondCoupon
	default:
		return portfolio.IncomeCashDividend
	}
}

// --- test harness -------------------------------------------------------------

type harness struct {
	svc    *portfolio.Service
	repo   *portfolio.InMemoryRepository
	perf   *performance.Service
	income *Service
	store  *InMemoryStore
	prov   *ManualDevelopmentProvider
	now    time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pp := prices.NewMockPriceProvider()
	repo := portfolio.NewInMemoryRepository()
	svc := portfolio.NewService(repo, pp, fx.NewMockFXProvider())
	perf := performance.NewService(repo)
	perf.SetValuator(svc)
	store := NewInMemoryStore()
	prov := NewManualDevelopmentProvider()
	// The harness timeline is anchored well AHEAD of real wall-clock time so that
	// positions created "now" (the portfolio service uses the real clock for buys)
	// always predate the seeded ex-dates, matching the real-world ordering where a
	// user holds a position before a later dividend's ex-date.
	now := time.Now().UTC().AddDate(0, 0, 90)
	inc := NewService(prov, store, testGateway{svc: svc})
	inc.SetClock(func() time.Time { return now })
	inc.SetLookback(365 * 24 * time.Hour)
	return &harness{svc: svc, repo: repo, perf: perf, income: inc, store: store, prov: prov, now: now}
}

func (h *harness) fund(t *testing.T, user, qty string) {
	t.Helper()
	_, err := h.svc.DepositCash(context.Background(), user, "dep-"+user, portfolio.CashFlowInput{Currency: "USD", Amount: testAmount("100000")})
	require.NoError(t, err)
	_, err = h.svc.BuyPosition(context.Background(), user, "buy-"+user, portfolio.BuyInput{Symbol: "AAPL", AssetType: portfolio.AssetTypeStock, Quantity: testQuantity(qty)})
	require.NoError(t, err)
}

func (h *harness) cashUSD(t *testing.T, user string) money.Amount {
	t.Helper()
	balances, err := h.repo.ListCashBalances(context.Background(), user)
	require.NoError(t, err)
	for _, c := range balances {
		if c.Currency == "USD" {
			return c.Amount
		}
	}
	return money.ZeroAmount()
}

func (h *harness) rankedIndex(t *testing.T, user string) float64 {
	t.Helper()
	rp, err := h.perf.CurrentRankedPerformance(context.Background(), user)
	require.NoError(t, err)
	return rp.RankedIndex
}

func dayPtr(base time.Time, days int) *time.Time {
	d := base.AddDate(0, 0, days)
	return &d
}

// --- tests --------------------------------------------------------------------

func TestCashDividend_AutoCreditedNoUserAction(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	cashBefore := h.cashUSD(t, "u1")
	rankedBefore := h.rankedIndex(t, "u1")

	ex := dayPtr(h.now, -5)
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-2026Q3", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("0.25"), Currency: "USD",
		ExDate: ex, PaymentDate: h.now.AddDate(0, 0, -1),
	})

	require.NoError(t, h.income.RunOnce(context.Background()))

	// 100 eligible shares × $0.25 = $25 credited; user did nothing.
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("25")), h.cashUSD(t, "u1"))
	assert.Greater(t, h.rankedIndex(t, "u1"), rankedBefore)

	views, err := h.income.ListIncomeEventViews(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, views, 1)
	assertAmountEqual(t, "25.0", views[0].NetAmount)
	assert.True(t, views[0].Estimated)
}

func TestCashDividend_DuplicateDeliveryNotDoubleCredited(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-D", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("0.25"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))
	cashAfterFirst := h.cashUSD(t, "u1")
	// Re-run twice: same provider event must not credit again.
	require.NoError(t, h.income.RunOnce(context.Background()))
	require.NoError(t, h.income.RunOnce(context.Background()))
	assertAmountValuesEqual(t, cashAfterFirst, h.cashUSD(t, "u1"))
}

func TestEligibility_UsesHistoricalHoldingsNotCurrentQuantity(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	// Sell 60 AFTER the ex-date. Current quantity becomes 40, but entitlement is
	// based on the 100 held on the ex-date.
	sellAt := h.now.AddDate(0, 0, -2)
	_, err := h.svc.SellPosition(context.Background(), "u1", "sell-1", portfolio.SellInput{
		Symbol: "AAPL", Quantity: testQuantity("60"), ExecutionPrice: testPrice("195"), EffectiveAt: &sellAt,
	})
	require.NoError(t, err)

	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-HIST", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1.0"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))

	// Eligible quantity = 100 (held on ex-date), not 40 (current). $1 × 100 = $100.
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("100")), h.cashUSD(t, "u1"))
}

func TestEligibility_FullySoldAfterExDateStillReceivesDividend(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	sellAt := h.now.AddDate(0, 0, -2)
	_, err := h.svc.SellPosition(context.Background(), "u1", "sell-all", portfolio.SellInput{
		Symbol: "AAPL", Quantity: testQuantity("100"), ExecutionPrice: testPrice("195"), EffectiveAt: &sellAt,
	})
	require.NoError(t, err)
	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-CLOSED", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})

	require.NoError(t, h.income.RunOnce(context.Background()))
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("100")), h.cashUSD(t, "u1"))
}

func TestEligibility_BuyAfterExDateReceivesNothing(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.DepositCash(context.Background(), "u1", "dep", portfolio.CashFlowInput{Currency: "USD", Amount: testAmount("100000")})
	require.NoError(t, err)
	buyAt := h.now.AddDate(0, 0, -2)
	_, err = h.svc.BuyPosition(context.Background(), "u1", "late-buy", portfolio.BuyInput{
		Symbol: "AAPL", AssetType: portfolio.AssetTypeStock, Quantity: testQuantity("100"),
		ExecutionPrice: testPrice("190"), EffectiveAt: &buyAt,
	})
	require.NoError(t, err)
	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-LATE", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})

	require.NoError(t, h.income.RunOnce(context.Background()))
	assertAmountValuesEqual(t, cashBefore, h.cashUSD(t, "u1"))
	ev, ok, err := h.store.GetEvent(context.Background(), "manual_dev:AAPL-LATE")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, StatusProcessedNoEntitlement, ev.Status)
}

func TestProcessedNoEntitlement_ReevaluatesAfterBackdatedBuy(t *testing.T) {
	h := newHarness(t)
	raw := ProviderIncomeEvent{
		ProviderEventID: "AAPL-REEVAL", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
		RetrievedAt: h.now,
	}
	_, err := h.store.UpsertEvent(context.Background(), normalize(h.prov.Name(), raw, h.now))
	require.NoError(t, err)
	require.NoError(t, h.income.Process(context.Background()))
	ev, ok, err := h.store.GetEvent(context.Background(), "manual_dev:AAPL-REEVAL")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, StatusProcessedNoEntitlement, ev.Status)

	_, err = h.svc.DepositCash(context.Background(), "u1", "dep", portfolio.CashFlowInput{Currency: "USD", Amount: testAmount("100000")})
	require.NoError(t, err)
	buyAt := h.now.AddDate(0, 0, -10)
	_, err = h.svc.BuyPosition(context.Background(), "u1", "backdated-buy", portfolio.BuyInput{
		Symbol: "AAPL", AssetType: portfolio.AssetTypeStock, Quantity: testQuantity("25"),
		ExecutionPrice: testPrice("180"), EffectiveAt: &buyAt,
	})
	require.NoError(t, err)
	cashBefore := h.cashUSD(t, "u1")
	require.NoError(t, h.income.Process(context.Background()))
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("25")), h.cashUSD(t, "u1"))
	// A further evaluation cannot credit twice.
	require.NoError(t, h.income.Process(context.Background()))
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("25")), h.cashUSD(t, "u1"))
}

func TestPaymentDate_NotCreditedBeforePayment(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-FUTURE", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("0.25"), Currency: "USD",
		ExDate: dayPtr(h.now, -1), PaymentDate: h.now.AddDate(0, 0, 10), // future
	})
	require.NoError(t, h.income.RunOnce(context.Background()))
	assertAmountValuesEqual(t, cashBefore, h.cashUSD(t, "u1")) // nothing credited yet

	ev, ok, err := h.store.GetEvent(context.Background(), "manual_dev:AAPL-FUTURE")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, StatusAwaitingPayment, ev.Status)
}

func TestCancelledEvent_NeverCredited(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-CXL", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("0.25"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1), Cancelled: true,
	})
	require.NoError(t, h.income.RunOnce(context.Background()))
	assertAmountValuesEqual(t, cashBefore, h.cashUSD(t, "u1"))
}

func TestWithholding_StoredSeparatelyAndReducesNet(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	h.income.SetPreferences(Preferences{UseEstimatedGross: true, Withholding: WithholdingProfile{DefaultRate: testRatio("0.15")}})
	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-WH", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1.0"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))

	// Gross $100, withholding 15% = $15, net $85 credited.
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("85")), h.cashUSD(t, "u1"))
	views, err := h.income.ListIncomeEventViews(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, views, 1)
	assertAmountEqual(t, "100.0", views[0].GrossAmount)
	assertAmountEqual(t, "15.0", views[0].Withholding)
	assertAmountEqual(t, "85.0", views[0].NetAmount)
}

func TestReinvestment_IncomeCountedOnceAndSharesAdded(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	h.income.SetPreferences(Preferences{UseEstimatedGross: true, ReinvestByDefault: true})
	rankedBefore := h.rankedIndex(t, "u1")
	cashBefore := h.cashUSD(t, "u1")

	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-RI", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1.0"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))

	// Net cash unchanged (income in, buy out), but shares increased and ranked
	// reflects the $100 income exactly once (value up ~$100 on ~ portfolio value).
	assertAmountValuesEqual(t, cashBefore, h.cashUSD(t, "u1"))
	assert.Greater(t, h.rankedIndex(t, "u1"), rankedBefore)
	positions, err := h.svc.ListPositions(context.Background(), "u1")
	require.NoError(t, err)
	qty := money.ZeroQuantity()
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			qty = p.Quantity
		}
	}
	assertQuantityEqual(t, "100.512820512820512821", qty)
}

func TestReturnOfCapital_ReducesBasisNotOrdinaryIncome(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-ROC", Type: TypeReturnOfCapital,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1.0"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))

	// Cash increases by $100; basis reduced (per-share baseline lowered).
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("100")), h.cashUSD(t, "u1"))
	positions, err := h.svc.ListPositions(context.Background(), "u1")
	require.NoError(t, err)
	basis := money.ZeroPrice()
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			basis = p.AverageBuyPrice
		}
	}
	assertPriceEqual(t, "194", basis) // baseline reduced by $1/share
}

func TestStockDividend_NoCashNoRankedJump(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	cashBefore := h.cashUSD(t, "u1")
	rankedBefore := h.rankedIndex(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-SD", Type: TypeStockDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("0.10"), Currency: "USD", // 10% stock dividend
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))

	assertAmountValuesEqual(t, cashBefore, h.cashUSD(t, "u1"))                   // no cash
	assert.InDelta(t, rankedBefore, h.rankedIndex(t, "u1"), rankedBefore*0.0005) // no ranked jump
	positions, err := h.svc.ListPositions(context.Background(), "u1")
	require.NoError(t, err)
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			assertQuantityEqual(t, "110", p.Quantity) // +10%
		}
	}
}

func TestMixedDistribution_ComponentsAppliedIndependently(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	cashBefore := h.cashUSD(t, "u1")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "ETF-MIX", Type: TypeETFDistribution,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1.0"), Currency: "USD",
		Components: []ProviderComponent{
			{Type: TypeETFDistribution, AmountPerUnit: testPrice("0.70")},
			{Type: TypeCapitalGainsDist, AmountPerUnit: testPrice("0.20")},
			{Type: TypeReturnOfCapital, AmountPerUnit: testPrice("0.10")},
		},
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))
	// All three components credit cash: (0.70 + 0.20 + 0.10) × 100 = $100.
	assertAmountValuesEqual(t, cashBefore.Add(testAmount("100")), h.cashUSD(t, "u1"))
}

func TestUnsupportedInterest_StaysUnresolved(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	cashBefore := h.cashUSD(t, "u1")
	// A market provider must not fabricate account-specific cash interest.
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "CASH-INT", Type: TypeCashInterest,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("5.0"), Currency: "USD",
		PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))
	// Filtered out by the market provider entirely — never credited.
	assertAmountValuesEqual(t, cashBefore, h.cashUSD(t, "u1"))
	_, ok, err := h.store.GetEvent(context.Background(), "manual_dev:CASH-INT")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCorrection_CompensatesWithoutFabricatingIncome(t *testing.T) {
	h := newHarness(t)
	h.fund(t, "u1", "100")
	h.prov.Seed(ProviderIncomeEvent{
		ProviderEventID: "AAPL-CORR", Type: TypeCashDividend,
		Instrument: InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: testPrice("1.0"), Currency: "USD",
		ExDate: dayPtr(h.now, -5), PaymentDate: h.now.AddDate(0, 0, -1),
	})
	require.NoError(t, h.income.RunOnce(context.Background()))
	cashAfterApply := h.cashUSD(t, "u1") // +$100 estimated gross

	// Broker reports actual withholding of $15 → net should be $85. The correction
	// posts a -$15 compensating adjustment.
	err := h.income.HandleCorrection(context.Background(), "u1", Correction{
		IncomeEventID: "manual_dev:AAPL-CORR", PortfolioID: portfolioID(t, h, "u1"),
		Kind: CorrectionActualWithholding, RequestID: "corr-1", ActualWithholding: testAmount("15"),
	})
	require.NoError(t, err)
	assertAmountValuesEqual(t, cashAfterApply.Sub(testAmount("15")), h.cashUSD(t, "u1"))

	// Idempotent: replaying the same correction requestID does not double-adjust.
	err = h.income.HandleCorrection(context.Background(), "u1", Correction{
		IncomeEventID: "manual_dev:AAPL-CORR", PortfolioID: portfolioID(t, h, "u1"),
		Kind: CorrectionActualWithholding, RequestID: "corr-1", ActualWithholding: testAmount("15"),
	})
	// Second call sees the application already corrected → ErrNotApplied, no change.
	assert.ErrorIs(t, err, ErrNotApplied)
	assertAmountValuesEqual(t, cashAfterApply.Sub(testAmount("15")), h.cashUSD(t, "u1"))
}

func portfolioID(t *testing.T, h *harness, user string) string {
	t.Helper()
	pf, err := h.svc.GetOrCreateDefaultPortfolio(context.Background(), user)
	require.NoError(t, err)
	return pf.ID
}
