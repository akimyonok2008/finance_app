package competitions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// fakeSnapshots returns a canned portfolio snapshot: the competition engine
// must consume ONLY this narrow boundary, so tests need no repositories,
// price providers or FX.
type fakeSnapshots struct {
	snap portfolio.CompetitionPortfolioSnapshot
	err  error
}

func (f fakeSnapshots) CaptureCompetitionSnapshot(context.Context, string, portfolio.CompetitionSnapshotRequest) (portfolio.CompetitionPortfolioSnapshot, error) {
	return f.snap, f.err
}

type fakeUniverses struct{ m map[string]map[string]bool }

func (f fakeUniverses) ResolveUniverses(_ context.Context, names []string) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	for _, n := range names {
		if members, ok := f.m[n]; ok {
			out[n] = members
		}
	}
	return out, nil
}

func snapPosition(instrumentID, assetType, mic, valueBase string) portfolio.CompetitionSnapshotPosition {
	return portfolio.CompetitionSnapshotPosition{
		PositionID: "p-" + instrumentID, Symbol: strings.ToUpper(instrumentID),
		InstrumentID: instrumentID, AssetType: assetType, VenueMIC: mic,
		Currency: "USD", ValueBase: money.MustAmount(valueBase),
		Quantity: money.QuantityFromFloat64(1),
	}
}

func snapshotOf(cash string, positions ...portfolio.CompetitionSnapshotPosition) portfolio.CompetitionPortfolioSnapshot {
	total := money.MustAmount(cash)
	posTotal := money.ZeroAmount()
	for _, p := range positions {
		posTotal = posTotal.Add(p.ValueBase)
		total = total.Add(p.ValueBase)
	}
	snap := portfolio.CompetitionPortfolioSnapshot{
		PortfolioID: "pf-1", PortfolioVersion: 7, CapturedAt: fixedTime,
		BaseCurrency: "USD", Positions: positions,
		PositionsValueBase: posTotal,
		CashValueBase:      money.MustAmount(cash),
		TotalValueBase:     total,
		PortfolioCreatedAt: fixedTime.Add(-90 * 24 * time.Hour),
	}
	if money.MustAmount(cash).Sign() > 0 {
		snap.Cash = []portfolio.CompetitionSnapshotCash{{
			Currency: "USD", Amount: money.MustAmount(cash), ValueBase: money.MustAmount(cash),
		}}
	}
	return snap
}

// cryptoEdition stores an engine edition whose eligibility requires >= 30%
// crypto exposure — pure configuration, no crypto-specific code anywhere.
func cryptoEdition(t *testing.T, h *harness) Competition {
	t.Helper()
	edition := Competition{
		ID: "crypto-aug-w1", Name: "Crypto Challenge — August Week 1", Type: "engine",
		StartsAt: fixedTime.Add(24 * time.Hour), EndsAt: fixedTime.Add(8 * 24 * time.Hour),
		CreatedAt: fixedTime, LifecycleStatus: LifecycleRegistrationOpen,
		ScoringScope: "full_portfolio",
		RulesSnapshotJSON: json.RawMessage(`{
			"schema_version": 1,
			"eligibility": {"schema_version":1,"all":[{"code":"minimum_crypto_weight","label":"At least 30% crypto exposure","metric":"portfolio_weight","filter":{"asset_types":["crypto"]},"operator":"gte","value":"0.30"}]},
			"scoring": {"schema_version":1,"scope":"full_portfolio","include_cash":true}
		}`),
	}
	require.NoError(t, h.repo.CreateCompetition(context.Background(), edition))
	return edition
}

func TestEligibilityPreview_EvaluatesEditionRulesAgainstSnapshot(t *testing.T) {
	h := newHarness(nil, nil)
	edition := cryptoEdition(t, h)

	// 24.17% crypto -> ineligible with exact percentage evidence.
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: snapshotOf("0",
		snapPosition("btc-id", "crypto", "", "2417"),
		snapPosition("aapl-id", "stock", "XNAS", "7583"),
	)})
	resp, err := h.svc.EligibilityPreview(context.Background(), edition.ID, "u1")
	require.NoError(t, err)
	assert.False(t, resp.Eligible)
	require.Len(t, resp.Rules, 1)
	assert.Equal(t, "minimum_crypto_weight", resp.Rules[0].Code)
	assert.Equal(t, "30.00", resp.Rules[0].Required)
	assert.Equal(t, "24.17", resp.Rules[0].Actual)
	assert.False(t, resp.Rules[0].Passed)

	// 40% crypto -> eligible.
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: snapshotOf("0",
		snapPosition("btc-id", "crypto", "", "40"),
		snapPosition("aapl-id", "stock", "XNAS", "60"),
	)})
	resp, err = h.svc.EligibilityPreview(context.Background(), edition.ID, "u1")
	require.NoError(t, err)
	assert.True(t, resp.Eligible)
}

func TestEligibilityPreview_ResponseNeverLeaksAbsoluteValues(t *testing.T) {
	h := newHarness(nil, nil)
	edition := cryptoEdition(t, h)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: snapshotOf("123456.78",
		snapPosition("btc-id", "crypto", "", "987654.32"),
		snapPosition("aapl-id", "stock", "XNAS", "555555.55"),
	)})

	resp, err := h.svc.EligibilityPreview(context.Background(), edition.ID, "u1")
	require.NoError(t, err)
	serialized, err := json.Marshal(resp)
	require.NoError(t, err)
	for _, private := range []string{"987654", "555555", "123456", "pf-1", "btc-id", "AAPL"} {
		assert.NotContains(t, string(serialized), private,
			"eligibility response must not leak %q", private)
	}
}

func TestEligibilityPreview_LegacySprintUsesLegacyDocument(t *testing.T) {
	h := newHarness(nil, nil)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: snapshotOf("0")}) // empty portfolio

	// The auto-created weekly sprint has no stamped rules; the canonical
	// legacy document (non-empty portfolio) applies.
	resp, err := h.svc.EligibilityPreview(context.Background(), compID(), "u1")
	require.NoError(t, err)
	assert.False(t, resp.Eligible)
	require.Len(t, resp.Rules, 1)
	assert.Equal(t, "non_empty_portfolio", resp.Rules[0].Code)

	h.svc.SetSnapshotProvider(fakeSnapshots{snap: snapshotOf("0", snapPosition("aapl-id", "stock", "XNAS", "100"))})
	resp, err = h.svc.EligibilityPreview(context.Background(), compID(), "u1")
	require.NoError(t, err)
	assert.True(t, resp.Eligible)
}

func TestEligibilityPreview_UniverseRulesResolveByInstrumentID(t *testing.T) {
	h := newHarness(nil, nil)
	edition := Competition{
		ID: "tech-sep", Name: "Technology Challenge", Type: "engine",
		StartsAt: fixedTime.Add(24 * time.Hour), EndsAt: fixedTime.Add(8 * 24 * time.Hour),
		CreatedAt: fixedTime, LifecycleStatus: LifecycleRegistrationOpen,
		ScoringScope: "matching_assets",
		RulesSnapshotJSON: json.RawMessage(`{
			"schema_version": 1,
			"eligibility": {"schema_version":1,"all":[{"code":"tech_exposure","label":"At least 40% technology exposure","metric":"portfolio_weight","filter":{"universe":"tech-v1"},"operator":"gte","value":"0.40"}]},
			"scoring": {"schema_version":1,"scope":"custom_universe","filter":{"universe":"tech-v1"},"include_cash":false}
		}`),
	}
	require.NoError(t, h.repo.CreateCompetition(context.Background(), edition))
	snap := snapshotOf("0",
		snapPosition("aapl-id", "stock", "XNAS", "60"),
		snapPosition("ko-id", "stock", "XNYS", "40"),
	)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: snap})

	// No resolver wired: universe rules fail closed, never pass open.
	resp, err := h.svc.EligibilityPreview(context.Background(), edition.ID, "u1")
	require.NoError(t, err)
	assert.False(t, resp.Eligible)

	h.svc.SetUniverseResolver(fakeUniverses{m: map[string]map[string]bool{
		"tech-v1": {"aapl-id": true},
	}})
	resp, err = h.svc.EligibilityPreview(context.Background(), edition.ID, "u1")
	require.NoError(t, err)
	assert.True(t, resp.Eligible)
	assert.Equal(t, "60.00", resp.Rules[0].Actual)
}

func TestEligibilityPreview_UnavailableWithoutSnapshotBoundary(t *testing.T) {
	h := newHarness(nil, nil)
	_, err := h.svc.EligibilityPreview(context.Background(), compID(), "u1")
	assert.ErrorIs(t, err, ErrEligibilityUnavailable)
}
