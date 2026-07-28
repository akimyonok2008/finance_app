package portfolio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// TestPG_EveryActivityTypeSatisfiesTheCheckConstraint enumerates every Go
// ActivityType constant and proves migration 0019's CHECK constraint accepts
// each one with type-appropriate minimal fields. This is the guard against
// future enum/whitelist drift between the Go domain and the Postgres schema
// (root cause of the bug this migration fixes: 0014 only whitelisted the
// first wave of activity types).
func TestPG_EveryActivityTypeSatisfiesTheCheckConstraint(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	userID := seedUser(t, pool)
	repo := NewPostgresRepository(pool)
	pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)

	f := func(v float64) *float64 { return &v }

	cases := []struct {
		name     string
		typ      ActivityType
		symbol   *string
		quantity *float64
		price    *float64
		gross    float64
	}{
		{"deposit", ActivityDeposit, nil, nil, nil, 100},
		{"withdrawal", ActivityWithdrawal, nil, nil, nil, 50},
		{"buy", ActivityBuy, strPtr("AAA"), f(10), f(5), 50},
		{"sell", ActivitySell, strPtr("AAA"), f(10), f(6), 60},
		{"opening_balance", ActivityOpeningBalance, strPtr("AAA"), f(10), f(5), 50},
		{"cash_dividend", ActivityCashDividend, strPtr("AAA"), nil, nil, 5},
		{"etf_distribution", ActivityETFDistribution, strPtr("AAA"), nil, nil, 5},
		{"interest_income", ActivityInterestIncome, nil, nil, nil, 5},
		{"reinvested_dividend", ActivityReinvestedDividend, strPtr("AAA"), nil, nil, 5},
		{"capital_gains_distribution", ActivityCapitalGainsDistribution, nil, nil, nil, 5},
		{"bond_coupon", ActivityBondCoupon, nil, nil, nil, 5},
		{"cash_interest", ActivityCashInterest, nil, nil, nil, 5},
		{"staking_reward", ActivityStakingReward, nil, nil, nil, 5},
		{"other_income", ActivityOtherIncome, nil, nil, nil, 5},
		{"return_of_capital", ActivityReturnOfCapital, strPtr("AAA"), nil, nil, 5},
		{"stock_dividend", ActivityStockDividend, strPtr("AAA"), f(1), nil, 0},
		{"buy_fee", ActivityBuyFee, nil, nil, nil, 5},
		{"sell_fee", ActivitySellFee, nil, nil, nil, 5},
		{"management_fee", ActivityManagementFee, nil, nil, nil, 5},
		{"custody_fee", ActivityCustodyFee, nil, nil, nil, 5},
		{"other_fee", ActivityOtherFee, nil, nil, nil, 5},
		{"stock_split", ActivityStockSplit, strPtr("AAA"), f(5), nil, 0},
		{"reverse_split", ActivityReverseSplit, strPtr("AAA"), f(5), nil, 0},
		{"symbol_change", ActivitySymbolChange, strPtr("AAA"), nil, nil, 0},
		{"write_off", ActivityWriteOff, strPtr("AAA"), f(5), nil, 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, `
				INSERT INTO portfolio_activities (
					id, portfolio_id, user_id, activity_type, symbol, currency,
					quantity, unit_price, gross_amount, occurred_at, portfolio_version
				) VALUES (gen_random_uuid(), $1, $2, $3, $4, 'USD', $5, $6, $7, now(), 1)`,
				pf.ID, userID, string(tc.typ), tc.symbol, tc.quantity, tc.price, tc.gross)
			assert.NoError(t, err, "activity type %q must satisfy the CHECK constraint", tc.typ)
		})
	}
}

func strPtr(s string) *string { return &s }

// TestPG_UnknownActivityTypeRejected proves an unrecognized activity_type
// still violates the CHECK constraint after migration 0019.
func TestPG_UnknownActivityTypeRejected(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	userID := seedUser(t, pool)
	repo := NewPostgresRepository(pool)
	pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO portfolio_activities (id, portfolio_id, user_id, activity_type, currency, gross_amount, occurred_at, portfolio_version)
		VALUES (gen_random_uuid(), $1, $2, 'not_a_real_type', 'USD', 1, now(), 1)`, pf.ID, userID)
	assert.Error(t, err)
}

// TestPG_ReturnOfCapitalPersistsAndReconciles exercises return of capital
// against real Postgres: it credits cash, reduces the position's basis, is
// excluded from ordinary income, and reconciliation stays consistent.
func TestPG_ReturnOfCapitalPersistsAndReconciles(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	quotes := prices.NewMockPriceProvider()
	quotes.Set("T", 40, "USD")
	svc := NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()

	_, err := svc.DepositCash(ctx, userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx, userID, "buy", BuyInput{Symbol: "T", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)

	_, err = svc.RecordIncome(ctx, userID, "div", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "T", Currency: "USD", Amount: 20,
	})
	require.NoError(t, err)
	_, err = svc.RecordIncome(ctx, userID, "roc", IncomeInput{
		Subtype: IncomeReturnOfCapitalSub, Symbol: "T", Currency: "USD", Amount: 50,
	})
	require.NoError(t, err)

	summary, err := svc.Summary(ctx, userID)
	require.NoError(t, err)
	assert.InDelta(t, 20, summary.Income.TotalIncomeBase, 0.01)
	assert.InDelta(t, 50, summary.Income.ReturnOfCapitalBase, 0.01)
	assert.True(t, summary.Reconciliation.IsConsistent, "reasons: %v", summary.Reconciliation.Reasons)

	positions, err := svc.ListPositions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Less(t, positions[0].AverageBuyPrice, 40.0, "basis must be reduced by the return of capital")
}

// TestPG_StockDividendZeroGrossPersistsAndPreservesBasis proves a
// stock_dividend row (zero gross_amount) persists under the new constraint and
// preserves total cost basis / does not move the ranked index.
func TestPG_StockDividendZeroGrossPersistsAndPreservesBasis(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	quotes := prices.NewMockPriceProvider()
	quotes.Set("DIV", 100, "USD")
	svc := NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()

	_, err := svc.DepositCash(ctx, userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("5000")})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx, userID, "buy", BuyInput{Symbol: "DIV", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)

	before, err := svc.Summary(ctx, userID)
	require.NoError(t, err)

	res, err := svc.Mutate(ctx, MutationRequest{
		Kind: MutationStockDividend, UserID: userID, RequestID: "sd",
		Income: IncomeInput{Symbol: "DIV", StockRatioNum: 1, StockRatioDen: 10},
	})
	require.NoError(t, err)
	require.NotNil(t, res.Position)
	assert.InDelta(t, 11, res.Position.Quantity, 0.0001)

	after, err := svc.Summary(ctx, userID)
	require.NoError(t, err)
	assert.InDelta(t, before.ActiveCostBasisBase, after.ActiveCostBasisBase, 0.01, "stock dividend must preserve total basis")

	var grossAmount float64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT gross_amount FROM portfolio_activities WHERE portfolio_id=$1 AND activity_type='stock_dividend'`,
		res.Position.PortfolioID).Scan(&grossAmount))
	assert.Equal(t, 0.0, grossAmount)
}

// TestPG_PartialAndFinalSaleShareOneClosedEpisode proves a partial sale
// followed by the closing sale share one position_episode_id and produce one
// aggregated closed-position summary against real Postgres.
func TestPG_PartialAndFinalSaleShareOneClosedEpisode(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	quotes := prices.NewMockPriceProvider()
	quotes.Set("NFLX", 100, "USD")
	svc := NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()

	_, err := svc.DepositCash(ctx, userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("10000")})
	require.NoError(t, err)
	buyRes, err := svc.BuyPosition(ctx, userID, "buy", BuyInput{Symbol: "NFLX", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)
	episodeID := buyRes.Position.ID

	quotes.Set("NFLX", 110, "USD")
	partial, err := svc.SellPosition(ctx, userID, "sell1", SellInput{
		PositionID: episodeID, Symbol: "NFLX", Quantity: testQuantity("4"), ExecutionPrice: testPrice("110"),
	})
	require.NoError(t, err)
	require.Nil(t, partial.Closed)

	quotes.Set("NFLX", 130, "USD")
	final, err := svc.SellPosition(ctx, userID, "sell2", SellInput{
		PositionID: episodeID, Symbol: "NFLX", Quantity: testQuantity("6"), ExecutionPrice: testPrice("130"),
	})
	require.NoError(t, err)
	require.NotNil(t, final.Closed)
	assert.Equal(t, episodeID, final.Closed.ID)

	acts, err := repo.ListActivitiesByPositionEpisode(ctx, userID, episodeID)
	require.NoError(t, err)
	sellCount := 0
	for _, a := range acts {
		if a.Type == ActivitySell {
			sellCount++
		}
	}
	assert.Equal(t, 2, sellCount)

	closedList, err := svc.ListClosedPositions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, closedList, 1)
	wantRealized := (4*110.0 - 4*100.0) + (6*130.0 - 6*100.0)
	assert.InDelta(t, wantRealized, closedList[0].RealizedGainLossBase, 0.01)
}

// TestPG_RebuyAfterClosureCreatesNewEpisode proves rebuying the same symbol
// after a full close creates a brand-new episode and never mutates the closed
// one, against real Postgres.
func TestPG_RebuyAfterClosureCreatesNewEpisode(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	quotes := prices.NewMockPriceProvider()
	quotes.Set("GME", 20, "USD")
	svc := NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()

	_, err := svc.DepositCash(ctx, userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("10000")})
	require.NoError(t, err)
	firstBuy, err := svc.BuyPosition(ctx, userID, "buy1", BuyInput{Symbol: "GME", AssetType: AssetTypeStock, Quantity: testQuantity("5")})
	require.NoError(t, err)

	quotes.Set("GME", 25, "USD")
	closeRes, err := svc.SellPosition(ctx, userID, "sellall", SellInput{
		PositionID: firstBuy.Position.ID, Symbol: "GME", Quantity: testQuantity("5"), ExecutionPrice: testPrice("25"),
	})
	require.NoError(t, err)
	require.NotNil(t, closeRes.Closed)

	secondBuy, err := svc.BuyPosition(ctx, userID, "buy2", BuyInput{Symbol: "GME", AssetType: AssetTypeStock, Quantity: testQuantity("3")})
	require.NoError(t, err)
	assert.NotEqual(t, firstBuy.Position.ID, secondBuy.Position.ID)
}

// TestPG_SaleFeeIdempotentRetryDoesNotDuplicate proves retrying the same sale
// request id does not duplicate the fee, cash credit, or episode totals.
func TestPG_SaleFeeIdempotentRetryDoesNotDuplicate(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	quotes := prices.NewMockPriceProvider()
	quotes.Set("AAPL", 100, "USD")
	svc := NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()

	_, err := svc.DepositCash(ctx, userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("10000")})
	require.NoError(t, err)
	buyRes, err := svc.BuyPosition(ctx, userID, "buy", BuyInput{Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)

	quotes.Set("AAPL", 120, "USD")
	sellInput := SellInput{PositionID: buyRes.Position.ID, Symbol: "AAPL", Quantity: testQuantity("10"), ExecutionPrice: testPrice("120"), Fee: testAmount("15")}
	first, err := svc.SellPosition(ctx, userID, "sell-retry", sellInput)
	require.NoError(t, err)
	require.NotNil(t, first.Closed)
	second, err := svc.SellPosition(ctx, userID, "sell-retry", sellInput)
	require.NoError(t, err)
	assert.True(t, second.Duplicate, "the retry must be recognized as a duplicate, not re-applied")

	var feeCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM portfolio_activities WHERE portfolio_id=$1 AND activity_type='sell_fee'`,
		buyRes.Position.PortfolioID).Scan(&feeCount))
	assert.Equal(t, 1, feeCount, "the retried sale must not duplicate the sell_fee activity")

	_, totalCash, err := svc.CashBalances(ctx, userID)
	require.NoError(t, err)
	assert.InDelta(t, 10000-1000+10*120-15, totalCash, 0.01)
}
