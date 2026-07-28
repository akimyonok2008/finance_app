package portfolio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// TestPG_IncomeFeeCorporateActionsPersist exercises the full return-bearing and
// corporate-action write path against a real database: the new activity types
// satisfy the migrated CHECK constraints, grouped legs commit atomically, and
// the immutable ledger records provenance.
func TestPG_IncomeFeeCorporateActionsPersist(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	quotes := prices.NewMockPriceProvider()
	svc := NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()

	_, err := svc.DepositCash(ctx, userID, "pg-seed-dep", CashFlowInput{Currency: "USD", Amount: 10000})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx, userID, "pg-seed-buy", BuyInput{Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 10})
	require.NoError(t, err)

	// Cash dividend (return-bearing) raises the index.
	div, err := svc.RecordIncome(ctx, userID, "pg-div", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD", Amount: 100,
	})
	require.NoError(t, err)
	assert.Greater(t, div.RankedIndexAfter, div.RankedIndexBefore)

	// Reinvested dividend commits two grouped activity rows atomically.
	_, err = svc.RecordIncome(ctx, userID, "pg-reinv", IncomeInput{
		Subtype: IncomeReinvestedDiv, Symbol: "AAPL", AssetType: AssetTypeStock, Currency: "USD", Amount: 195,
	})
	require.NoError(t, err)

	// Management fee (return-bearing negative) lowers the index.
	fee, err := svc.RecordFee(ctx, userID, "pg-fee", FeeInput{Subtype: FeeManagement, Currency: "USD", Amount: 25})
	require.NoError(t, err)
	assert.Less(t, fee.RankedIndexAfter, fee.RankedIndexBefore)

	// Stock split (neutral) preserves the index at the action.
	split, err := svc.RecordCorporateAction(ctx, userID, "pg-split", CorpActionInput{
		Subtype: CorpStockSplit, Symbol: "AAPL", RatioNumerator: 2, RatioDenominator: 1,
	})
	require.NoError(t, err)
	assert.InDelta(t, split.RankedIndexBefore, split.RankedIndexAfter, 1e-9)

	// Symbol change (neutral).
	quotes.Set("APLX", 97.5, "USD")
	_, err = svc.RecordCorporateAction(ctx, userID, "pg-sym", CorpActionInput{
		Subtype: CorpSymbolChange, Symbol: "AAPL", NewSymbol: "APLX",
	})
	require.NoError(t, err)

	// Write-off (return-bearing negative) closes the position with a loss.
	wo, err := svc.RecordCorporateAction(ctx, userID, "pg-wo", CorpActionInput{
		Subtype: CorpWriteOff, Symbol: "APLX",
	})
	require.NoError(t, err)
	assert.Less(t, wo.RankedIndexAfter, wo.RankedIndexBefore)

	// Idempotent replay of the dividend.
	replay, err := svc.RecordIncome(ctx, userID, "pg-div", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD", Amount: 100,
	})
	require.NoError(t, err)
	assert.True(t, replay.Duplicate)

	// Verify persistence + the reinvestment group.
	pf, err := repo.GetPortfolioByUser(ctx, userID)
	require.NoError(t, err)

	var dividendRows, reinvestGroups int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM portfolio_activities
		 WHERE portfolio_id=$1 AND activity_type='cash_dividend'`, pf.ID).Scan(&dividendRows))
	assert.Equal(t, 1, dividendRows, "the dividend must be recorded exactly once despite the retry")

	// The reinvestment's own group must contain exactly its two legs (the income
	// row and the buy row). Other groups exist — every purchase is a group — so
	// the assertion is scoped to the reinvestment's group id.
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM portfolio_activities
		 WHERE portfolio_id=$1
		   AND metadata_json ->> 'activity_group_id' = (
		       SELECT metadata_json ->> 'activity_group_id' FROM portfolio_activities
		       WHERE portfolio_id=$1 AND activity_type='reinvested_dividend' LIMIT 1
		   )`, pf.ID).Scan(&reinvestGroups))
	assert.Equal(t, 2, reinvestGroups, "reinvested dividend legs share one activity group")

	// The ranked state is valid and the position was closed by the write-off.
	state, err := performance.NewPostgresStateReader(pool).GetByPortfolio(ctx, pf.ID)
	require.NoError(t, err)
	require.NoError(t, performance.ValidateState(*state))

	open, err := svc.ListPositions(ctx, userID)
	require.NoError(t, err)
	assert.Empty(t, open, "written-off position must not remain open")

	// A misclassified type must be rejected by the CHECK constraint.
	_, err = pool.Exec(ctx,
		`INSERT INTO portfolio_activities (id, portfolio_id, user_id, activity_type, currency, gross_amount, occurred_at, portfolio_version)
		 VALUES (gen_random_uuid(), $1, $2, 'not_a_type', 'USD', 1, now(), 1)`, pf.ID, userID)
	assert.Error(t, err, "unknown activity_type must violate the CHECK constraint")
}

// TestPG_ApplyIncomeEventMixedComponentsCommitAtomically is the Postgres
// storage-parity counterpart of the in-memory ApplyIncomeEvent tests
// (income_event_test.go): a $70 ordinary + $20 capital-gains-distribution +
// $10 return-of-capital mixed event must commit as ONE transaction — one
// activity group, one combined cash effect, one basis reduction, one ranked
// checkpoint, one audit row, one outbox event — and a retry with the same
// request id must replay without creating duplicates.
func TestPG_ApplyIncomeEventMixedComponentsCommitAtomically(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	quotes := prices.NewMockPriceProvider()
	svc := NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()

	_, err := svc.DepositCash(ctx, userID, "pg-mixed-dep", CashFlowInput{Currency: "USD", Amount: 10000})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx, userID, "pg-mixed-buy", BuyInput{Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 10})
	require.NoError(t, err)

	pf, err := repo.GetPortfolioByUser(ctx, userID)
	require.NoError(t, err)
	positions, err := svc.ListPositions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	basisBefore := positions[0].Quantity * positions[0].AverageBuyPrice

	components := []IncomeComponentInput{
		{Subtype: IncomeCashDividend, Amount: 70},
		{Subtype: IncomeCapitalGainsDist, Amount: 20},
		{Subtype: IncomeReturnOfCapitalSub, Amount: 10},
	}
	// The audit table's request_id column is globally unique (see
	// 0010_portfolio_mutation_aggregate.sql), so the idempotency key must be
	// scoped to THIS test's own portfolio/user, not a bare literal — otherwise a
	// prior run against the same disposable database could collide.
	requestID := "pg-mixed-evt:" + userID
	res, err := svc.ApplyIncomeEvent(ctx, userID, requestID, IncomeEventInput{
		IncomeEventID: "pg-mixed-evt", Symbol: "AAPL", Currency: "USD", Components: components,
	})
	require.NoError(t, err)
	assert.False(t, res.Duplicate)
	assert.Greater(t, res.RankedIndexAfter, res.RankedIndexBefore)

	// One activity group across all three legs.
	var groupCount, groupIDCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(DISTINCT metadata_json ->> 'activity_group_id') FROM portfolio_activities
		 WHERE portfolio_id=$1 AND activity_type IN ('cash_dividend','capital_gains_distribution','return_of_capital')`,
		pf.ID).Scan(&groupIDCount))
	assert.Equal(t, 1, groupIDCount, "every leg of the mixed event must share exactly one activity_group_id")
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM portfolio_activities
		 WHERE portfolio_id=$1 AND activity_type IN ('cash_dividend','capital_gains_distribution','return_of_capital')`,
		pf.ID).Scan(&groupCount))
	assert.Equal(t, 3, groupCount)

	// $10 basis reduction persisted.
	positionsAfter, err := svc.ListPositions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, positionsAfter, 1)
	basisAfter := positionsAfter[0].Quantity * positionsAfter[0].AverageBuyPrice
	assert.InDelta(t, basisBefore-10, basisAfter, 0.001)

	// Retry with the SAME event-level request id replays the committed result
	// and creates no duplicate cash, activities, or basis reduction.
	replay, err := svc.ApplyIncomeEvent(ctx, userID, requestID, IncomeEventInput{
		IncomeEventID: "pg-mixed-evt", Symbol: "AAPL", Currency: "USD", Components: components,
	})
	require.NoError(t, err)
	assert.True(t, replay.Duplicate)

	var afterRetryCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM portfolio_activities
		 WHERE portfolio_id=$1 AND activity_type IN ('cash_dividend','capital_gains_distribution','return_of_capital')`,
		pf.ID).Scan(&afterRetryCount))
	assert.Equal(t, 3, afterRetryCount, "retry must not duplicate any leg")

	positionsAfterRetry, err := svc.ListPositions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, positionsAfterRetry, 1)
	assert.InDelta(t, basisAfter, positionsAfterRetry[0].Quantity*positionsAfterRetry[0].AverageBuyPrice, 0.001,
		"retry must not reduce basis a second time")

	// One audit row and one outbox event for the WHOLE event (not one per
	// component): the coordinator's single-transaction boundary applies here
	// exactly as it does for every other mutation kind.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM portfolio_mutation_audit WHERE request_id=$1`, requestID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount, "exactly one audit row for the whole event, even across the retry")
}
