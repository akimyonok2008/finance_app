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

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(DISTINCT metadata_json ->> 'activity_group_id') FROM portfolio_activities
		 WHERE portfolio_id=$1 AND metadata_json ? 'activity_group_id'`, pf.ID).Scan(&reinvestGroups))
	assert.Equal(t, 1, reinvestGroups, "reinvested dividend legs share one activity group")

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
