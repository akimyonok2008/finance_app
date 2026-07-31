package competitions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/competitions/rules"
	"github.com/ardakimyonok/finance_app/internal/money"
)

type fakeBasketAdjustments struct{}

func (fakeBasketAdjustments) AdjustPosition(_ context.Context, _, symbol string, quantity money.Quantity, _, _ time.Time) (string, money.Quantity, error) {
	if symbol == "OLD" {
		return "NEW", quantity.MulRatio(money.MustRatio("2")), nil
	}
	return symbol, quantity, nil
}

// activeEntry builds a baselined (EntryActive) entry directly, bypassing the
// join/baseline flow, so ranking tests can focus on valuation and promotion.
func activeEntry(id, competitionID, userID, symbol string, startValue string) CompetitionEntry {
	return CompetitionEntry{
		ID: id, CompetitionID: competitionID, UserID: userID,
		StartingIndex: money.MustIndexValue("100"),
		EntryStatus:   EntryActive, BaselineStatus: BaselineCompleted,
		EligibleStartingValueBase: money.MustAmount(startValue),
		Snapshots: []CompetitionEntrySnapshotPosition{{
			ID: id + "-pos", CompetitionEntryID: id, Symbol: symbol, AssetType: "stock",
			Quantity: money.QuantityFromFloat64(1), Currency: "USD", IncludedInScore: true,
			StartingPrice: money.MustPrice(startValue), StartingValueBase: money.MustAmount(startValue),
		}},
	}
}

func rankingEdition(t *testing.T, h *harness, id string) Competition {
	t.Helper()
	edition := Competition{
		ID: id, Name: "Ranking Test", Type: "engine",
		StartsAt: fixedTime.Add(-time.Hour), EndsAt: fixedTime.Add(time.Hour),
		CreatedAt: fixedTime, LifecycleStatus: LifecycleActive,
		ScoringScope:      "full_portfolio",
		RulesSnapshotJSON: legacyRulesSnapshot,
	}
	require.NoError(t, h.repo.CreateEdition(context.Background(), edition))
	return edition
}

func TestRefreshCompetitionRankings_PromotesWithSequentialRanksNoTies(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "rank-1")
	h.repo.SetDisplayNames(map[string]string{"u1": "Alpha", "u2": "Bravo", "u3": "Charlie"})

	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e2", edition.ID, "u2", "BBB", "100")))
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e3", edition.ID, "u3", "CCC", "100")))
	h.mp.Set("AAA", 120, "USD") // +20%
	h.mp.Set("BBB", 150, "USD") // +50%
	h.mp.Set("CCC", 90, "USD")  // -10%

	require.NoError(t, h.svc.RefreshCompetitionRankings(ctx))

	page, err := h.svc.EditionLeaderboard(ctx, edition.ID, 0, 10)
	require.NoError(t, err)
	require.False(t, page.Unavailable)
	require.Len(t, page.Entries, 3)
	assert.Equal(t, []string{"Bravo", "Alpha", "Charlie"}, []string{
		page.Entries[0].DisplayName, page.Entries[1].DisplayName, page.Entries[2].DisplayName,
	})
	assert.Equal(t, []int{1, 2, 3}, []int{page.Entries[0].Rank, page.Entries[1].Rank, page.Entries[2].Rank},
		"the full tie-break chain is a strict total order: ranks are always sequential, never shared")
	assert.Equal(t, 0, page.Entries[0].ReturnPercentage.Cmp(money.MustRatio("50")))
}

func TestRefreshCompetitionRankings_NormalizesSplitAndSymbolChange(t *testing.T) {
	h := newHarness(nil, nil)
	h.svc.SetBasketAdjustmentProvider(fakeBasketAdjustments{})
	edition := rankingEdition(t, h, "rank-split")
	require.NoError(t, h.repo.CreateEngineEntry(context.Background(), activeEntry("e1", edition.ID, "u1", "OLD", "100")))
	h.mp.Set("NEW", 50, "USD") // 2-for-1 split: 2 × 50 == original 1 × 100.

	require.NoError(t, h.svc.RefreshCompetitionRankings(context.Background()))
	page, err := h.svc.EditionLeaderboard(context.Background(), edition.ID, 0, 10)
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, 0, page.Entries[0].ReturnPercentage.Cmp(money.ZeroRatio()))
	assert.Equal(t, ReturnModelFixedBasketPriceV1, page.ReturnModel)
}

func TestValueCompetitionEntry_IncludeFXFalseMeasuresLocalPriceOnly(t *testing.T) {
	h := newHarness(nil, nil)
	entry := activeEntry("e1", "c1", "u1", "EURSTOCK", "100")
	entry.Snapshots[0].Currency = "EUR"
	entry.Snapshots[0].StartingPriceCurrency = "EUR"
	entry.Snapshots[0].StartingValueBase = money.MustAmount("120")
	entry.EligibleStartingValueBase = money.MustAmount("120")
	h.mp.Set("EURSTOCK", 110, "EUR")
	includeFX := false

	_, ret, err := h.svc.valueCompetitionEntry(context.Background(), entry, newObservationMemo(h.svc), rules.Scoring{IncludeFX: &includeFX})
	require.NoError(t, err)
	assert.Equal(t, 0, ret.Cmp(money.MustRatio("10")))
}

func TestValueCompetitionEntry_PreservesAuthoritativeReturnPrecision(t *testing.T) {
	h := newHarness(nil, nil)
	entry := activeEntry("e1", "c1", "u1", "PRECISE", "100")
	h.mp.Set("PRECISE", 107.214873, "USD")

	_, ret, err := h.svc.valueCompetitionEntry(context.Background(), entry, newObservationMemo(h.svc), rules.Scoring{})
	require.NoError(t, err)
	assert.Equal(t, "7.214873", ret.String(), "valuation must not quantize authoritative return to display precision")
}

func TestEditionLeaderboard_CursorPagination(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "rank-2")
	h.repo.SetDisplayNames(map[string]string{"u1": "Alpha", "u2": "Bravo", "u3": "Charlie"})
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e2", edition.ID, "u2", "BBB", "100")))
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e3", edition.ID, "u3", "CCC", "100")))
	h.mp.Set("AAA", 120, "USD")
	h.mp.Set("BBB", 150, "USD")
	h.mp.Set("CCC", 90, "USD")
	require.NoError(t, h.svc.RefreshCompetitionRankings(ctx))

	first, err := h.svc.EditionLeaderboard(ctx, edition.ID, 0, 2)
	require.NoError(t, err)
	require.Len(t, first.Entries, 2)
	require.NotNil(t, first.NextCursor)
	assert.Equal(t, 2, *first.NextCursor)

	second, err := h.svc.EditionLeaderboard(ctx, edition.ID, *first.NextCursor, 2)
	require.NoError(t, err)
	require.Len(t, second.Entries, 1)
	assert.Nil(t, second.NextCursor)
	assert.Equal(t, 3, second.Entries[0].Rank)
}

func TestEditionLeaderboard_UnavailableBeforeFirstPromotion(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "rank-3")
	page, err := h.svc.EditionLeaderboard(ctx, edition.ID, 0, 10)
	require.NoError(t, err)
	assert.True(t, page.Unavailable, "no generation has ever been promoted: a controlled response, never a live scan")
	assert.Empty(t, page.Entries)
}

func TestEngineMyStatus_ReflectsProjectionRank(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "rank-4")
	h.repo.SetDisplayNames(map[string]string{"u1": "Alpha", "u2": "Bravo"})
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e2", edition.ID, "u2", "BBB", "100")))
	h.mp.Set("AAA", 120, "USD")
	h.mp.Set("BBB", 150, "USD")
	require.NoError(t, h.svc.RefreshCompetitionRankings(ctx))

	comp, err := h.svc.GetCompetition(ctx, edition.ID)
	require.NoError(t, err)
	st, err := h.svc.EngineMyStatus(ctx, comp, "u1")
	require.NoError(t, err)
	assert.True(t, st.Joined)
	assert.Equal(t, EntryActive, st.EntryStatus)
	assert.Equal(t, 2, st.CurrentRank)
	assert.Equal(t, 0, st.SprintReturnPercentage.Cmp(money.MustRatio("20")))
	require.NotNil(t, st.ValuedAt)

	notJoined, err := h.svc.EngineMyStatus(ctx, comp, "nobody")
	require.NoError(t, err)
	assert.False(t, notJoined.Joined)
}

func TestRefreshCompetitionRankings_FailsClosedWhenAnyEntryIsUnpriceable(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "rank-5")
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e2", edition.ID, "u2", "UNPRICED", "100")))
	h.mp.Set("AAA", 110, "USD")
	// UNPRICED is never set: incomplete participant coverage must withhold the
	// entire generation rather than silently publishing a one-user board.

	require.NoError(t, h.svc.RefreshCompetitionRankings(ctx))

	page, err := h.svc.EditionLeaderboard(ctx, edition.ID, 0, 10)
	require.NoError(t, err)
	assert.True(t, page.Unavailable)
	assert.Empty(t, page.Entries)

	first, err := h.repo.EnsureBuildingGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), first.Generation, "the incomplete generation is terminal and retry starts clean")

	h.mp.Set("UNPRICED", 120, "USD")
	require.NoError(t, h.svc.RefreshCompetitionRankings(ctx))
	page, err = h.svc.EditionLeaderboard(ctx, edition.ID, 0, 10)
	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
}

// TestRefreshCompetitionRankings_WithholdsPromotionWhileBaselineIncomplete is
// the regression test for the race where baseline batching (capped at
// defaultBaselineBatch) leaves some admitted entries still pending while
// ranking refresh already has a clean lap over the entries that ARE active:
// promoting there would publish (and later silently change) a partial board.
func TestRefreshCompetitionRankings_WithholdsPromotionWhileBaselineIncomplete(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "rank-baseline-race")

	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))
	h.mp.Set("AAA", 120, "USD")
	// A second entry has been admitted but not yet baselined (still in the
	// pending batch) — the population is not fully covered yet.
	require.NoError(t, h.repo.CreateEngineEntry(ctx, CompetitionEntry{
		ID: "e2", CompetitionID: edition.ID, UserID: "u2",
		EntryStatus: EntryAdmitted, BaselineStatus: BaselinePending,
		JoinedAt: fixedTime,
		Snapshots: []CompetitionEntrySnapshotPosition{{
			ID: "e2-pos", CompetitionEntryID: "e2", Symbol: "BBB", AssetType: "stock",
			Quantity: money.QuantityFromFloat64(1), Currency: "USD", IncludedInScore: true,
		}},
	}))

	require.NoError(t, h.svc.RefreshCompetitionRankings(ctx))

	page, err := h.svc.EditionLeaderboard(ctx, edition.ID, 0, 10)
	require.NoError(t, err)
	assert.True(t, page.Unavailable, "must stay unavailable, never publish a partial board")
	assert.Empty(t, page.Entries)

	// Once the second entry baselines (reaches a terminal state), the next
	// refresh promotes normally.
	require.NoError(t, h.repo.CompleteBaseline(ctx, "e2", money.MustAmount("100"), []PositionBaseline{{
		SnapshotID: "e2-pos", Symbol: "BBB", Quantity: money.QuantityFromFloat64(1),
		Price: money.MustPrice("100"), PriceCurrency: "USD",
		ValueBase: money.MustAmount("100"), Weight: money.MustRatio("1"), ObservedAt: fixedTime,
	}}, fixedTime))
	h.mp.Set("BBB", 130, "USD")

	require.NoError(t, h.svc.RefreshCompetitionRankings(ctx))
	page, err = h.svc.EditionLeaderboard(ctx, edition.ID, 0, 10)
	require.NoError(t, err)
	assert.False(t, page.Unavailable)
	assert.Len(t, page.Entries, 2)
}

// TestPromoteGeneration_WithheldOnWriteFailure exercises the repository layer
// directly: a ranking-row write failure recorded during a lap must withhold
// promotion. A failed generation is terminal so its partial rows and latched
// failure state cannot contaminate a retry.
func TestPromoteGeneration_WithheldOnWriteFailure(t *testing.T) {
	repo := NewInMemoryCompetitionRepository()
	ctx := context.Background()
	competitionID := "c1"

	gen, err := repo.EnsureBuildingGeneration(ctx, competitionID)
	require.NoError(t, err)
	require.NoError(t, repo.UpsertRanking(ctx, competitionID, gen.Generation, CompetitionRankingRow{
		EntryID: "e1", UserID: "u1", Index: money.MustIndexValue("110"), ReturnPct: money.MustRatio("10"), ValuedAt: time.Now(),
	}))
	require.NoError(t, repo.AdvanceGeneration(ctx, competitionID, gen.Generation, "e1", 1, 1, 0, true /* writeFailed */))

	err = repo.PromoteGeneration(ctx, competitionID, gen.Generation, time.Now())
	assert.ErrorIs(t, err, ErrGenerationConflict, "a lap with a write failure must never be promoted")

	require.NoError(t, repo.FailGeneration(ctx, competitionID, gen.Generation, "write failed", time.Now()))
	again, err := repo.EnsureBuildingGeneration(ctx, competitionID)
	require.NoError(t, err)
	assert.Greater(t, again.Generation, gen.Generation)

	_, _, found, err := repo.ActiveGeneration(ctx, competitionID)
	require.NoError(t, err)
	assert.False(t, found, "nothing was ever promoted")
}

func TestPromoteGeneration_RanksByAuthoritativeReturnBeforeDisplayRounding(t *testing.T) {
	repo := NewInMemoryCompetitionRepository()
	repo.SetDisplayNames(map[string]string{"higher": "Zulu", "lower": "Alpha"})
	ctx := context.Background()
	gen, err := repo.EnsureBuildingGeneration(ctx, "precise")
	require.NoError(t, err)
	rows := []CompetitionRankingRow{
		{EntryID: "higher-entry", UserID: "higher", Index: money.MustIndexValue("107.2149"), ReturnPct: money.MustRatio("7.2149"), ValuedAt: fixedTime},
		{EntryID: "lower-entry", UserID: "lower", Index: money.MustIndexValue("107.2051"), ReturnPct: money.MustRatio("7.2051"), ValuedAt: fixedTime},
	}
	for _, row := range rows {
		require.NoError(t, repo.UpsertRanking(ctx, "precise", gen.Generation, row))
	}
	require.NoError(t, repo.AdvanceGeneration(ctx, "precise", gen.Generation, "lower-entry", 2, 2, 0, false))
	require.NoError(t, repo.PromoteGeneration(ctx, "precise", gen.Generation, fixedTime))
	ranked, err := repo.LeaderboardPage(ctx, "precise", 0, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 2)
	assert.Equal(t, "Zulu", ranked[0].DisplayName, "display-name tie-break must not replace the more precise financial result")
	assert.Equal(t, "7.2149", ranked[0].ReturnPercentage.String())
}

func finalizingEdition(t *testing.T, h *harness, id string, endsAt time.Time) Competition {
	t.Helper()
	edition := Competition{
		ID: id, Name: "Finalize Test", Type: "engine",
		StartsAt: endsAt.Add(-time.Hour), EndsAt: endsAt,
		CreatedAt: fixedTime, LifecycleStatus: LifecycleFinalizing,
		ScoringScope:      "full_portfolio",
		RulesSnapshotJSON: legacyRulesSnapshot,
	}
	require.NoError(t, h.repo.CreateEdition(context.Background(), edition))
	return edition
}

func TestFinalizeCompetitions_RetriesWithinWindowThenDisqualifiesAndCompletes(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	h.svc.SetFinalizationWindow(time.Hour)
	edition := finalizingEdition(t, h, "final-1", fixedTime)
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e2", edition.ID, "u2", "UNPRICED", "100")))
	h.mp.Set("AAA", 130, "USD") // +30%

	// Inside the window: the unpriceable entry blocks completion. The
	// competition is neither dropped nor completed — it stays in finalizing
	// for a retry, per the fail-closed policy.
	h.clk.Time = fixedTime.Add(30 * time.Minute)
	finalized, err := h.svc.FinalizeCompetitions(ctx)
	require.NoError(t, err)
	assert.Zero(t, finalized)
	comp, err := h.repo.GetCompetition(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, LifecycleFinalizing, comp.LifecycleStatus)

	// Past the window: the explicit resolution is to disqualify the failed
	// entry and finalize with everyone who remains — never a silent omission.
	h.clk.Time = fixedTime.Add(2 * time.Hour)
	finalized, err = h.svc.FinalizeCompetitions(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, finalized)

	comp, err = h.repo.GetCompetition(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, LifecycleCompleted, comp.LifecycleStatus)
	require.NotNil(t, comp.FinalizedAt)

	disqualified, err := h.repo.GetEntry(ctx, edition.ID, "u2")
	require.NoError(t, err)
	assert.Equal(t, EntryDisqualified, disqualified.EntryStatus)
	assert.NotEmpty(t, disqualified.DisqualificationReason)

	results, err := h.svc.Results(ctx, edition.ID)
	require.NoError(t, err)
	require.Len(t, results, 1, "only the successfully valued entry is scored into final results")
	assert.Equal(t, 1, results[0].Rank)
	assert.Equal(t, 0, results[0].ReturnPercentage.Cmp(money.MustRatio("30")))
}

func TestFinalizeCompetitions_ResultsAreImmutableAndReplayIdempotent(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := finalizingEdition(t, h, "final-2", fixedTime)
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))
	h.mp.Set("AAA", 105, "USD")
	h.clk.Time = fixedTime.Add(time.Minute)

	finalized, err := h.svc.FinalizeCompetitions(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, finalized)
	firstResults, err := h.svc.Results(ctx, edition.ID)
	require.NoError(t, err)
	require.Len(t, firstResults, 1)

	// Repricing AAA and re-running must never change a completed competition's
	// results: it is no longer in LifecycleFinalizing, so it is not even
	// re-examined.
	h.mp.Set("AAA", 500, "USD")
	finalized, err = h.svc.FinalizeCompetitions(ctx)
	require.NoError(t, err)
	assert.Zero(t, finalized, "a completed edition is never re-finalized")

	secondResults, err := h.svc.Results(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, firstResults, secondResults, "final results are immutable")

	// Direct repository-level idempotent replay of PromoteFinalizationGeneration
	// itself (simulating a worker retrying after a partial failure), even
	// against the generation number that actually succeeded.
	err = h.repo.PromoteFinalizationGeneration(ctx, edition.ID, 1, h.clk.Time)
	require.NoError(t, err, "replaying finalization on an already-completed edition is a no-op success")
	thirdResults, err := h.svc.Results(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, firstResults, thirdResults, "the replay must not overwrite the original immutable rows")
}

// TestFinalizeCompetitions_ReusesPersistedObservationAcrossRetries is the
// regression test for the "a delayed finalizer reprices against a later
// market moment" bug: the first pass captures AAA's end price while it also
// discovers a still-unpriceable entry, so it retries without writing
// results. Before the retry runs, AAA's live quote moves — but the already
// finalized entry's return must reflect the originally captured price, not
// the new one.
func TestFinalizeCompetitions_ReusesPersistedObservationAcrossRetries(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	h.svc.SetFinalizationWindow(2 * time.Hour)
	edition := finalizingEdition(t, h, "final-3", fixedTime)
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e2", edition.ID, "u2", "UNPRICED", "100")))
	h.mp.Set("AAA", 130, "USD") // +30% — captured into the persisted end observation set on this pass

	h.clk.Time = fixedTime.Add(30 * time.Minute)
	finalized, err := h.svc.FinalizeCompetitions(ctx)
	require.NoError(t, err)
	assert.Zero(t, finalized, "still inside the window: e2 blocks completion, so nothing is written yet")

	// The live quote moves on before the retry that finally disqualifies e2
	// and finalizes with e1.
	h.mp.Set("AAA", 500, "USD")
	h.clk.Time = fixedTime.Add(3 * time.Hour)
	finalized, err = h.svc.FinalizeCompetitions(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, finalized)

	results, err := h.svc.Results(ctx, edition.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].ReturnPercentage.Cmp(money.MustRatio("30")),
		"must reflect the price captured on the first pass, not the later live quote")
}

func TestResults_UnavailableBeforeCompletion(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "rank-6")
	_, err := h.svc.Results(ctx, edition.ID)
	assert.ErrorIs(t, err, ErrResultsNotAvailable)
}
