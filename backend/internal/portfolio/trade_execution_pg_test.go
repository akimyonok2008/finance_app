package portfolio

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/instrument"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// TestPG_AutomaticFundingGroupPersists proves against a real database that an
// automatically-funded purchase commits all three legs (funding deposit, buy,
// buy fee) under one activity group, that the provenance columns added by
// migration 0021 are populated, and that the CHECK constraints accept them.
func TestPG_AutomaticFundingGroupPersists(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	svc := NewService(repo, prices.NewMockPriceProvider(), fx.NewMockFXProvider())
	ctx := context.Background()

	// No cash at all: the entire purchase must be funded automatically.
	_, err := svc.BuyPosition(ctx, userID, "pg-autofund", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("2"), Fee: testAmount("4"),
	})
	require.NoError(t, err)

	pf, err := repo.GetPortfolioByUser(ctx, userID)
	require.NoError(t, err)
	require.True(t, pf.AutoFundPurchases, "the preference column defaults to true")

	type row struct {
		kind        string
		gross       float64
		group       *string
		priceSource *string
		feeSource   *string
		recordedAt  *time.Time
		occurredAt  time.Time
	}
	rows, err := pool.Query(ctx, `
		SELECT activity_type, gross_amount, metadata_json->>'activity_group_id',
		       execution_price_source, fee_source, recorded_at, occurred_at
		FROM portfolio_activities WHERE portfolio_id=$1 ORDER BY activity_type`, pf.ID)
	require.NoError(t, err)
	defer rows.Close()

	var found []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.kind, &r.gross, &r.group, &r.priceSource,
			&r.feeSource, &r.recordedAt, &r.occurredAt))
		found = append(found, r)
	}
	require.NoError(t, rows.Err())
	require.Len(t, found, 3, "funding + buy + fee all committed")

	groups := map[string]bool{}
	byKind := map[string]row{}
	for _, r := range found {
		require.NotNil(t, r.group, "%s must carry an activity group id", r.kind)
		groups[*r.group] = true
		byKind[r.kind] = r
		require.NotNil(t, r.recordedAt, "recorded_at is populated")
	}
	assert.Len(t, groups, 1, "one purchase is one activity group")

	// 2 x 195 + 4 = 394 funded, 390 gross buy, 4 fee.
	assert.InDelta(t, 394.0, byKind["deposit"].gross, 1e-6)
	assert.InDelta(t, 390.0, byKind["buy"].gross, 1e-6)
	assert.InDelta(t, 4.0, byKind["buy_fee"].gross, 1e-6)

	require.NotNil(t, byKind["buy"].priceSource)
	assert.Equal(t, PriceSourceProviderEstimate, *byKind["buy"].priceSource)
	require.NotNil(t, byKind["buy"].feeSource)
	assert.Equal(t, FeeSourceUserRecorded, *byKind["buy"].feeSource)

	// Cash landed at exactly zero, and the position basis includes the fee.
	balances, err := repo.ListCashBalances(ctx, userID)
	require.NoError(t, err)
	for _, b := range balances {
		if b.Currency == "USD" {
			assertAmountEqual(t, "0", b.Amount)
		}
	}
	positions, err := svc.ListPositions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assertPriceEqual(t, "197", positions[0].AverageBuyPrice) // (390 + 4) / 2
}

// TestPG_BuyIdempotencyConstraint proves the (portfolio_id, request_id) unique
// constraint plus the audit replay path leave exactly one committed effect when
// the same idempotency key is retried.
func TestPG_BuyIdempotencyConstraint(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	svc := NewService(repo, prices.NewMockPriceProvider(), fx.NewMockFXProvider())
	ctx := context.Background()

	_, err := svc.DepositCash(ctx, userID, "pg-idem-dep", CashFlowInput{Currency: "USD", Amount: testAmount("5000")})
	require.NoError(t, err)

	first, err := svc.BuyPosition(ctx, userID, "pg-idem-buy", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("3"), Fee: testAmount("2"),
	})
	require.NoError(t, err)
	retry, err := svc.BuyPosition(ctx, userID, "pg-idem-buy", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("3"), Fee: testAmount("2"),
	})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)
	assert.Equal(t, first.PortfolioVersion, retry.PortfolioVersion)

	pf, err := repo.GetPortfolioByUser(ctx, userID)
	require.NoError(t, err)
	var buys, fees int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE activity_type='buy'),
		        count(*) FILTER (WHERE activity_type='buy_fee')
		 FROM portfolio_activities WHERE portfolio_id=$1`, pf.ID).Scan(&buys, &fees))
	assert.Equal(t, 1, buys)
	assert.Equal(t, 1, fees)

	positions, err := svc.ListPositions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assertQuantityEqual(t, "3", positions[0].Quantity)
}

func TestPG_InstrumentIdentityMatchesMemoryPortfolioFlow(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	identities := instrument.NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	priceProvider := prices.NewMockPriceProvider()
	ticker := "M" + strings.ToUpper(uuid.NewString()[:8])
	priceProvider.Set(ticker, 430, "USD")
	svc := NewService(repo, priceProvider, fx.NewMockFXProvider())
	svc.SetInstrumentResolver(instrument.NewResolver(identities, nil))
	ctx := context.Background()

	createListing := func(exchange, figi string) instrument.Instrument {
		in, err := identities.CreateInstrument(ctx, instrument.Instrument{
			FIGI: figi, CurrentSymbol: ticker, ExchangeCode: exchange,
			Status: instrument.StatusActive, IdentityQuality: instrument.QualityResolved,
		})
		require.NoError(t, err)
		_, err = identities.CreateAlias(ctx, instrument.InstrumentAlias{
			InstrumentID: in.ID, AliasType: instrument.AliasTicker,
			AliasValue: ticker, ExchangeCode: exchange,
		})
		require.NoError(t, err)
		return in
	}
	nyse := createListing("UN", "BBG"+uuid.NewString())
	xetra := createListing("GY", "BBG"+uuid.NewString())

	first, err := svc.BuyPosition(ctx, userID, "pg-identity-un", BuyInput{
		Symbol: ticker, ExchangeCode: "UN", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	second, err := svc.BuyPosition(ctx, userID, "pg-identity-gy", BuyInput{
		Symbol: ticker, ExchangeCode: "GY", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	assert.Equal(t, nyse.ID, first.Position.InstrumentID)
	assert.Equal(t, xetra.ID, second.Position.InstrumentID)
	assert.NotEqual(t, first.Position.ID, second.Position.ID)

	activities, err := repo.ListActivities(ctx, userID, 20)
	require.NoError(t, err)
	buyIDs := map[string]bool{}
	for _, activity := range activities {
		if activity.Type == ActivityBuy {
			buyIDs[activity.InstrumentID] = true
			assert.Equal(t, ticker, activity.Symbol, "symbol remains the historical display snapshot")
		}
	}
	assert.True(t, buyIDs[nyse.ID])
	assert.True(t, buyIDs[xetra.ID])
}

func TestPG_IncomeDiscoveryAndEntitlementMatchHistoricalLedger(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	identities := instrument.NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	ticker := "I" + strings.ToUpper(uuid.NewString()[:8])
	priceProvider := prices.NewMockPriceProvider()
	priceProvider.Set(ticker, 100, "USD")
	svc := NewService(repo, priceProvider, fx.NewMockFXProvider())
	svc.SetInstrumentResolver(instrument.NewResolver(identities, nil))
	ctx := context.Background()

	in, err := identities.CreateInstrument(ctx, instrument.Instrument{
		FIGI: "BBG" + uuid.NewString(), CurrentSymbol: ticker, ExchangeCode: "UN",
		Status: instrument.StatusActive, IdentityQuality: instrument.QualityResolved,
	})
	require.NoError(t, err)
	_, err = identities.CreateAlias(ctx, instrument.InstrumentAlias{
		InstrumentID: in.ID, AliasType: instrument.AliasTicker,
		AliasValue: ticker, ExchangeCode: "UN", ValidFrom: time.Now().UTC().AddDate(0, 0, -30),
	})
	require.NoError(t, err)

	_, err = svc.DepositCash(ctx, userID, "income-pg-deposit", CashFlowInput{Currency: "USD", Amount: testAmount("10000")})
	require.NoError(t, err)
	now := time.Now().UTC()
	buyAt := now.AddDate(0, 0, -10)
	svc.Coordinator().SetClock(func() time.Time { return buyAt })
	buy, err := svc.BuyPosition(ctx, userID, "income-pg-buy", BuyInput{
		Symbol: ticker, ExchangeCode: "UN", AssetType: AssetTypeStock,
		Quantity: testQuantity("10"), ExecutionPrice: testPrice("100"),
	})
	require.NoError(t, err)
	require.Equal(t, in.ID, buy.Position.InstrumentID)
	sellAt := now.AddDate(0, 0, -2)
	svc.Coordinator().SetClock(func() time.Time { return sellAt })
	_, err = svc.SellPosition(ctx, userID, "income-pg-sell", SellInput{
		Symbol: ticker, Quantity: testQuantity("10"), ExecutionPrice: testPrice("105"),
	})
	require.NoError(t, err)
	svc.Coordinator().SetClock(func() time.Time { return time.Now().UTC() })

	discovered, err := svc.IncomeDiscoveryInstruments(ctx, now.AddDate(0, 0, -30))
	require.NoError(t, err)
	found := false
	for _, item := range discovered {
		if item.InstrumentID == in.ID {
			found = true
			assert.Equal(t, ticker, item.Symbol)
		}
	}
	assert.True(t, found, "recently closed stable instrument must be in provider discovery")

	eligible, err := svc.EligibleQuantity(ctx, userID, in.ID, ticker, now.AddDate(0, 0, -5))
	require.NoError(t, err)
	assertQuantityEqual(t, "10", eligible)
}
