package competitions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/competitions/rules"
	"github.com/ardakimyonok/finance_app/internal/money"
)

func snapshotPos(sector string, valueBase string) CompetitionEntrySnapshotPosition {
	return CompetitionEntrySnapshotPosition{
		Sector:            sector,
		StartingValueBase: money.MustAmount(valueBase),
	}
}

func TestFactsFromSnapshot_WeightsSumToOne(t *testing.T) {
	snap := []CompetitionEntrySnapshotPosition{
		snapshotPos("technology", "600"),
		snapshotPos("financials", "400"),
	}
	facts := factsFromSnapshot(snap, money.MustAmount("1000"))
	require.Len(t, facts, 2)
	assert.InDelta(t, 0.6, facts[0].Weight, 1e-9)
	assert.Equal(t, "technology", facts[0].Sector)
	assert.InDelta(t, 0.4, facts[1].Weight, 1e-9)
}

func TestFactsFromSnapshot_ZeroTotalReturnsNil(t *testing.T) {
	snap := []CompetitionEntrySnapshotPosition{snapshotPos("technology", "0")}
	facts := factsFromSnapshot(snap, money.ZeroAmount())
	assert.Nil(t, facts)
}

func TestCheckEligibility_NilFilterAdmitsEveryone(t *testing.T) {
	ok, err := checkEligibility(nil, nil, money.ZeroAmount())
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCheckEligibility_TechnologyChallengeThreshold(t *testing.T) {
	filter := &rules.Filter{
		Metric:    rules.MetricPortfolioWeight,
		Sectors:   []string{"technology"},
		Operator:  rules.OpGTE,
		Threshold: 0.5,
	}

	techHeavy := []CompetitionEntrySnapshotPosition{
		snapshotPos("technology", "700"),
		snapshotPos("financials", "300"),
	}
	ok, err := checkEligibility(filter, techHeavy, money.MustAmount("1000"))
	require.NoError(t, err)
	assert.True(t, ok, "70% technology exposure should clear the 50% threshold")

	techLight := []CompetitionEntrySnapshotPosition{
		snapshotPos("technology", "200"),
		snapshotPos("financials", "800"),
	}
	ok, err = checkEligibility(filter, techLight, money.MustAmount("1000"))
	require.NoError(t, err)
	assert.False(t, ok, "20% technology exposure must not clear the 50% threshold")
}

func TestSectorForSymbol_NilProviderIsUnknown(t *testing.T) {
	assert.Equal(t, unknownSector, sectorForSymbol(context.Background(), nil, "AAPL"))
}

type fakeSectors struct {
	m   map[string]string
	err error
}

func (f fakeSectors) SectorForSymbol(_ context.Context, symbol string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.m[symbol], nil
}

func TestSectorForSymbol_ProviderErrorFallsBackToUnknown(t *testing.T) {
	p := fakeSectors{err: assert.AnError}
	assert.Equal(t, unknownSector, sectorForSymbol(context.Background(), p, "AAPL"))
}

func TestSectorForSymbol_UsesProviderResult(t *testing.T) {
	p := fakeSectors{m: map[string]string{"AAPL": "technology"}}
	assert.Equal(t, "technology", sectorForSymbol(context.Background(), p, "AAPL"))
}
