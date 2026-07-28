package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEligibleQuantity_ReconstructsHistoricalQuantityTransformations(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil, nil)
	pf, err := repo.EnsureDefaultPortfolio(context.Background(), "u1")
	require.NoError(t, err)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	instrumentID := "11111111-1111-1111-1111-111111111111"
	repo.mu.Lock()
	agg := repo.aggregates[pf.ID]
	agg.activities = append(agg.activities,
		Activity{Type: ActivityOpeningBalance, Symbol: "OLD", InstrumentID: instrumentID, Quantity: testQuantityPtr("10"), OccurredAt: base},
		Activity{Type: ActivityBuy, Symbol: "OLD", InstrumentID: instrumentID, Quantity: testQuantityPtr("5"), OccurredAt: base.AddDate(0, 0, 1)},
		Activity{Type: ActivitySell, Symbol: "OLD", InstrumentID: instrumentID, Quantity: testQuantityPtr("3"), OccurredAt: base.AddDate(0, 0, 2)},
		Activity{Type: ActivityReinvestedDividend, Symbol: "NEW", InstrumentID: instrumentID, Quantity: testQuantityPtr("2"), OccurredAt: base.AddDate(0, 0, 3)},
		Activity{Type: ActivityStockSplit, Symbol: "NEW", InstrumentID: instrumentID, OccurredAt: base.AddDate(0, 0, 4), Metadata: map[string]any{"ratio_numerator": 2.0, "ratio_denominator": 1.0}},
		Activity{Type: ActivityStockDividend, Symbol: "NEW", InstrumentID: instrumentID, OccurredAt: base.AddDate(0, 0, 5), Metadata: map[string]any{"ratio_numerator": 1.0, "ratio_denominator": 10.0}},
		Activity{Type: ActivityReverseSplit, Symbol: "NEW", InstrumentID: instrumentID, OccurredAt: base.AddDate(0, 0, 6), Metadata: map[string]any{"ratio_numerator": 1.0, "ratio_denominator": 2.0}},
		Activity{Type: ActivityWriteOff, Symbol: "NEW", InstrumentID: instrumentID, OccurredAt: base.AddDate(0, 0, 8)},
	)
	repo.mu.Unlock()

	got, err := svc.EligibleQuantity(context.Background(), "u1", instrumentID, "NEW", base.AddDate(0, 0, 7))
	require.NoError(t, err)
	assert.InDelta(t, 15.4, got, 1e-9)
}

func TestEligibleQuantity_UsesStableIdentityAcrossHistoricalTickerAlias(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil, nil)
	pf, err := repo.EnsureDefaultPortfolio(context.Background(), "u1")
	require.NoError(t, err)
	at := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	instrumentID := "22222222-2222-2222-2222-222222222222"
	qty := testQuantity("12")

	repo.mu.Lock()
	repo.aggregates[pf.ID].activities = append(repo.aggregates[pf.ID].activities,
		Activity{Type: ActivityOpeningBalance, Symbol: "OLD", InstrumentID: instrumentID, Quantity: &qty, OccurredAt: at})
	repo.mu.Unlock()

	got, err := svc.EligibleQuantity(context.Background(), "u1", instrumentID, "NEW", at.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 12.0, got)
}

func TestIncomeDiscovery_PreservesRecentProviderAliasesForStableInstrument(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil, nil)
	pf, err := repo.EnsureDefaultPortfolio(context.Background(), "u1")
	require.NoError(t, err)
	now := time.Now().UTC()
	instrumentID := "33333333-3333-3333-3333-333333333333"

	repo.mu.Lock()
	repo.aggregates[pf.ID].activities = append(repo.aggregates[pf.ID].activities,
		Activity{Type: ActivityBuy, Symbol: "OLD", InstrumentID: instrumentID, AssetType: AssetTypeStock, OccurredAt: now.AddDate(0, 0, -10)},
		Activity{Type: ActivitySymbolChange, Symbol: "NEW", InstrumentID: instrumentID, AssetType: AssetTypeStock, OccurredAt: now.AddDate(0, 0, -5)})
	repo.mu.Unlock()

	items, err := svc.IncomeDiscoveryInstruments(context.Background(), now.AddDate(0, 0, -30))
	require.NoError(t, err)
	aliases := map[string]bool{}
	for _, item := range items {
		if item.InstrumentID == instrumentID {
			aliases[item.Symbol] = true
		}
	}
	assert.True(t, aliases["OLD"])
	assert.True(t, aliases["NEW"])
}
