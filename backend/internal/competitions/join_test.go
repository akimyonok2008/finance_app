package competitions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// engineEdition builds and stores a registration_open edition with an open
// join window around fixedTime and a crypto-30% eligibility rule.
func engineEdition(t *testing.T, h *harness, id, scoringDoc string) Competition {
	t.Helper()
	opens := fixedTime.Add(-1 * time.Hour)
	closes := fixedTime.Add(23 * time.Hour)
	edition := Competition{
		ID: id, Name: "Crypto Challenge", Type: "engine",
		StartsAt: fixedTime.Add(24 * time.Hour), EndsAt: fixedTime.Add(8 * 24 * time.Hour),
		CreatedAt: fixedTime, LifecycleStatus: LifecycleRegistrationOpen,
		JoinOpensAt: &opens, JoinClosesAt: &closes,
		ScoringScope: "full_portfolio",
		RulesSnapshotJSON: json.RawMessage(`{
			"schema_version": 1,
			"eligibility": {"schema_version":1,"all":[{"code":"minimum_crypto_weight","label":"At least 30% crypto exposure","metric":"portfolio_weight","filter":{"asset_types":["crypto"]},"operator":"gte","value":"0.30"}]},
			"scoring": ` + scoringDoc + `
		}`),
	}
	require.NoError(t, h.repo.CreateEdition(context.Background(), edition))
	return edition
}

func eligibleSnapshot() portfolio.CompetitionPortfolioSnapshot {
	return snapshotOf("20",
		snapPosition("btc-id", "crypto", "", "40"),
		snapPosition("aapl-id", "stock", "XNAS", "40"),
	) // 40% crypto of 100 total -> eligible at 30%
}

func TestJoin_EngineEditionRequiresIdempotencyKey(t *testing.T) {
	h := newHarness(nil, nil)
	edition := engineEdition(t, h, "crypto-w1", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: eligibleSnapshot()})

	_, err := h.svc.Join(context.Background(), edition.ID, "u1", "")
	assert.ErrorIs(t, err, ErrIdempotencyKeyRequired)

	resp, err := h.svc.Join(context.Background(), edition.ID, "u1", "key-1")
	require.NoError(t, err)
	assert.True(t, resp.Joined)
	assert.Equal(t, EntryAdmitted, resp.EntryStatus)
}

func TestJoin_IdempotentReplayAndConcurrencyBackstop(t *testing.T) {
	h := newHarness(nil, nil)
	edition := engineEdition(t, h, "crypto-w1", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: eligibleSnapshot()})
	ctx := context.Background()

	first, err := h.svc.Join(ctx, edition.ID, "u1", "key-1")
	require.NoError(t, err)
	// Replay (same or different key): the existing entry is returned, no
	// second entry is created.
	replay, err := h.svc.Join(ctx, edition.ID, "u1", "key-2")
	require.NoError(t, err)
	assert.Equal(t, first.EntryStatus, replay.EntryStatus)
	entries, err := h.repo.ListEntries(ctx, edition.ID)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "unique (competition, user) must hold")
}

func TestJoin_WindowEnforcement(t *testing.T) {
	h := newHarness(nil, nil)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: eligibleSnapshot()})
	ctx := context.Background()

	// Window not open yet: registration_open lifecycle but join_opens_at in
	// the future.
	future := engineEdition(t, h, "future-w", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	h.clk.Time = future.JoinOpensAt.Add(-30 * time.Minute)
	_, err := h.svc.Join(ctx, future.ID, "u1", "k")
	assert.ErrorIs(t, err, ErrJoinWindowClosed)
	h.clk.Time = fixedTime

	// Registration closed lifecycle: no late joins, even though legacy
	// sprints would have allowed joining mid-competition.
	closed := engineEdition(t, h, "closed-w", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	require.NoError(t, h.repo.TransitionLifecycle(ctx, closed.ID, LifecycleRegistrationOpen, LifecycleRegistrationClosed, fixedTime))
	_, err = h.svc.Join(ctx, closed.ID, "u1", "k")
	assert.ErrorIs(t, err, ErrJoinWindowClosed)

	// Active competition: joining is structurally impossible.
	active := engineEdition(t, h, "active-w", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	require.NoError(t, h.repo.TransitionLifecycle(ctx, active.ID, LifecycleRegistrationOpen, LifecycleRegistrationClosed, fixedTime))
	require.NoError(t, h.repo.TransitionLifecycle(ctx, active.ID, LifecycleRegistrationClosed, LifecycleActive, fixedTime))
	_, err = h.svc.Join(ctx, active.ID, "u1", "k")
	assert.ErrorIs(t, err, ErrJoinWindowClosed)
}

func TestJoin_IneligiblePortfolioRejectedWithEvidence(t *testing.T) {
	h := newHarness(nil, nil)
	edition := engineEdition(t, h, "crypto-w1", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: snapshotOf("0",
		snapPosition("btc-id", "crypto", "", "10"),
		snapPosition("aapl-id", "stock", "XNAS", "90"),
	)})

	resp, err := h.svc.Join(context.Background(), edition.ID, "u1", "k")
	assert.ErrorIs(t, err, ErrNotEligible)
	require.NotNil(t, resp)
	assert.False(t, resp.Joined)
	require.Len(t, resp.Eligibility, 1)
	assert.Equal(t, "10.00", resp.Eligibility[0].Actual)

	entries, err := h.repo.ListEntries(context.Background(), edition.ID)
	require.NoError(t, err)
	assert.Empty(t, entries, "an ineligible join must not create an entry")
}

func TestJoin_FreezesScoringCompositionIndependentlyOfEligibility(t *testing.T) {
	// Same eligibility (30% crypto), two scoring configurations: the frozen
	// inclusion flags must differ — eligibility and scoring are independent.
	ctx := context.Background()

	full := newHarness(nil, nil)
	fullEdition := engineEdition(t, full, "crypto-w1", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	full.svc.SetSnapshotProvider(fakeSnapshots{snap: eligibleSnapshot()})
	_, err := full.svc.Join(ctx, fullEdition.ID, "u1", "k")
	require.NoError(t, err)
	entry, err := full.repo.GetEntry(ctx, fullEdition.ID, "u1")
	require.NoError(t, err)
	for _, s := range entry.Snapshots {
		assert.True(t, s.IncludedInScore, "full_portfolio scores every position")
	}
	require.Len(t, entry.CashSnapshots, 1)
	assert.True(t, entry.CashSnapshots[0].IncludedInScore, "include_cash=true freezes cash into the sleeve")
	// provisional included value = 40 + 40 + 20 cash
	assert.Equal(t, 0, entry.EligibleStartingValueBase.Cmp(money.MustAmount("100")))

	matching := newHarness(nil, nil)
	matchingEdition := engineEdition(t, matching, "crypto-w1", `{"schema_version":1,"scope":"matching_assets","filter":{"asset_types":["crypto"]},"include_cash":false}`)
	matching.svc.SetSnapshotProvider(fakeSnapshots{snap: eligibleSnapshot()})
	_, err = matching.svc.Join(ctx, matchingEdition.ID, "u1", "k")
	require.NoError(t, err)
	entry, err = matching.repo.GetEntry(ctx, matchingEdition.ID, "u1")
	require.NoError(t, err)
	included := map[string]bool{}
	for _, s := range entry.Snapshots {
		included[s.Symbol] = s.IncludedInScore
	}
	assert.True(t, included["BTC-ID"], "crypto position is scored")
	assert.False(t, included["AAPL-ID"], "non-matching position is frozen but not scored")
	assert.False(t, entry.CashSnapshots[0].IncludedInScore, "include_cash=false excludes cash")
	// provisional included value = crypto only
	assert.Equal(t, 0, entry.EligibleStartingValueBase.Cmp(money.MustAmount("40")))

	// Evidence persisted with the entry.
	assert.NotEmpty(t, entry.EligibilityEvidenceJSON)
	assert.Equal(t, int64(7), entry.PortfolioVersion)
}

func TestWithdraw_OnlyBeforeRegistrationCloses(t *testing.T) {
	h := newHarness(nil, nil)
	edition := engineEdition(t, h, "crypto-w1", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: eligibleSnapshot()})
	ctx := context.Background()

	_, err := h.svc.Join(ctx, edition.ID, "u1", "k")
	require.NoError(t, err)
	require.NoError(t, h.svc.Withdraw(ctx, edition.ID, "u1"))

	entry, err := h.repo.GetEntry(ctx, edition.ID, "u1")
	require.NoError(t, err)
	assert.Equal(t, EntryWithdrawn, entry.EntryStatus)
	require.NotNil(t, entry.WithdrawnAt)

	// Second withdrawal conflicts (already withdrawn).
	assert.ErrorIs(t, h.svc.Withdraw(ctx, edition.ID, "u1"), ErrWithdrawalClosed)

	// After join_closes_at, withdrawal is refused.
	h2 := newHarness(nil, nil)
	e2 := engineEdition(t, h2, "crypto-w1", `{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
	h2.svc.SetSnapshotProvider(fakeSnapshots{snap: eligibleSnapshot()})
	_, err = h2.svc.Join(ctx, e2.ID, "u1", "k")
	require.NoError(t, err)
	h2.clk.Time = e2.JoinClosesAt.Add(time.Minute)
	assert.ErrorIs(t, h2.svc.Withdraw(ctx, e2.ID, "u1"), ErrWithdrawalClosed)
}

func TestAdvanceCompetitionLifecycles_FollowsTimestamps(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	opens := fixedTime.Add(1 * time.Hour)
	closes := fixedTime.Add(2 * time.Hour)
	edition := Competition{
		ID: "sched-1", Name: "Scheduled", Type: "engine",
		StartsAt: fixedTime.Add(3 * time.Hour), EndsAt: fixedTime.Add(4 * time.Hour),
		CreatedAt: fixedTime, LifecycleStatus: LifecyclePublished,
		JoinOpensAt: &opens, JoinClosesAt: &closes,
		ScoringScope:      "full_portfolio",
		RulesSnapshotJSON: json.RawMessage(`{"schema_version":1}`),
	}
	require.NoError(t, h.repo.CreateEdition(ctx, edition))

	step := func(at time.Time, want string) {
		h.clk.Time = at
		require.NoError(t, h.svc.AdvanceCompetitionLifecycles(ctx))
		got, err := h.repo.GetCompetition(ctx, edition.ID)
		require.NoError(t, err)
		assert.Equal(t, want, got.LifecycleStatus, "at %s", at)
	}
	step(fixedTime, LifecyclePublished)                          // nothing due yet
	step(opens, LifecycleRegistrationOpen)                       // join window opens
	step(closes, LifecycleRegistrationClosed)                    // join window closes
	step(edition.StartsAt, LifecycleActive)                      // competition starts
	step(edition.EndsAt.Add(time.Minute), LifecycleFinalizing)   // competition ends
	step(edition.EndsAt.Add(2*time.Minute), LifecycleFinalizing) // idempotent
}

func TestRunCompetitionBaselines_CommonBaselineNormalizesTo100(t *testing.T) {
	h := newHarness(nil, nil)
	edition := engineEdition(t, h, "crypto-w1", `{"schema_version":1,"scope":"matching_assets","filter":{"asset_types":["crypto"]},"include_cash":false}`)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: eligibleSnapshot()})
	ctx := context.Background()

	_, err := h.svc.Join(ctx, edition.ID, "u1", "k")
	require.NoError(t, err)

	// Move to starts_at, close registration, activate, and set start prices.
	h.clk.Time = edition.StartsAt.Add(time.Minute)
	require.NoError(t, h.repo.TransitionLifecycle(ctx, edition.ID, LifecycleRegistrationOpen, LifecycleRegistrationClosed, h.clk.Time))
	h.mp.Set("BTC-ID", 50, "USD")

	completed, err := h.svc.RunCompetitionBaselines(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, completed)

	entry, err := h.repo.GetEntry(ctx, edition.ID, "u1")
	require.NoError(t, err)
	assert.Equal(t, EntryActive, entry.EntryStatus)
	assert.Equal(t, BaselineCompleted, entry.BaselineStatus)
	require.NotNil(t, entry.BaselineCompletedAt)
	// Official baseline: 1 unit of BTC-ID at the start price of 50 (quantity
	// defaults to zero in snapPosition — set below), so validate via weights.
	for _, s := range entry.Snapshots {
		if !s.IncludedInScore {
			continue
		}
		assert.Equal(t, 0, s.StartingWeight.Cmp(money.MustRatio("1")),
			"a single included position carries the full weight")
		require.NotNil(t, s.BaselinePriceObservedAt)
	}

	// Re-running is a no-op (guarded on admitted+pending).
	completed, err = h.svc.RunCompetitionBaselines(ctx)
	require.NoError(t, err)
	assert.Zero(t, completed)
}

func TestRunCompetitionBaselines_FailsClosedThenMarksExplicitly(t *testing.T) {
	h := newHarness(nil, nil)
	edition := engineEdition(t, h, "crypto-w1", `{"schema_version":1,"scope":"full_portfolio","include_cash":false}`)
	h.svc.SetSnapshotProvider(fakeSnapshots{snap: snapshotOf("0",
		snapPosition("btc-id", "crypto", "", "40"),
		snapPosition("aapl-id", "stock", "XNAS", "60"),
	)})
	ctx := context.Background()
	_, err := h.svc.Join(ctx, edition.ID, "u1", "k")
	require.NoError(t, err)
	require.NoError(t, h.repo.TransitionLifecycle(ctx, edition.ID, LifecycleRegistrationOpen, LifecycleRegistrationClosed, fixedTime))

	// Only one of the two symbols has a start price: the entry must NOT be
	// partially baselined — it stays pending inside the window...
	h.clk.Time = edition.StartsAt.Add(time.Minute)
	h.mp.Set("BTC-ID", 50, "USD")
	completed, err := h.svc.RunCompetitionBaselines(ctx)
	require.NoError(t, err)
	assert.Zero(t, completed)
	entry, err := h.repo.GetEntry(ctx, edition.ID, "u1")
	require.NoError(t, err)
	assert.Equal(t, EntryAdmitted, entry.EntryStatus)
	assert.Equal(t, BaselinePending, entry.BaselineStatus)

	// ...and is explicitly failed with evidence once the operational window
	// elapses — never silently omitted.
	h.clk.Time = edition.StartsAt.Add(2 * time.Hour)
	completed, err = h.svc.RunCompetitionBaselines(ctx)
	require.NoError(t, err)
	assert.Zero(t, completed)
	entry, err = h.repo.GetEntry(ctx, edition.ID, "u1")
	require.NoError(t, err)
	assert.Equal(t, EntryBaselineFailed, entry.EntryStatus)
	assert.Equal(t, BaselineFailed, entry.BaselineStatus)
	assert.Contains(t, entry.DisqualificationReason, "AAPL-ID")
}

// admittedEntry builds a CompetitionEntry awaiting baseline (admitted,
// baseline pending) for tests that drive RunCompetitionBaselines directly
// against the repository, bypassing the Join eligibility/window flow.
func admittedEntry(id, competitionID, userID, symbol string) CompetitionEntry {
	return CompetitionEntry{
		ID: id, CompetitionID: competitionID, UserID: userID,
		EntryStatus: EntryAdmitted, BaselineStatus: BaselinePending,
		Snapshots: []CompetitionEntrySnapshotPosition{{
			ID: id + "-pos", CompetitionEntryID: id, Symbol: symbol, AssetType: "crypto",
			Quantity: money.QuantityFromFloat64(1), Currency: "USD", IncludedInScore: true,
		}},
	}
}

// TestRunCompetitionBaselines_ReusesPersistedObservationAcrossPasses is the
// regression test for the "batches see different start prices" bug: entry u1
// is baselined in one pass at BTC-ID=50, the live quote then moves to 100
// before u2 is admitted and baselined in a later pass. u2 must still receive
// BTC-ID=50 — the persisted observation captured by the first pass — never a
// fresh live query.
func TestRunCompetitionBaselines_ReusesPersistedObservationAcrossPasses(t *testing.T) {
	h := newHarness(nil, nil)
	edition := engineEdition(t, h, "crypto-w1", `{"schema_version":1,"scope":"matching_assets","filter":{"asset_types":["crypto"]},"include_cash":false}`)
	ctx := context.Background()

	require.NoError(t, h.repo.CreateEngineEntry(ctx, admittedEntry("e1", edition.ID, "u1", "BTC-ID")))
	require.NoError(t, h.repo.TransitionLifecycle(ctx, edition.ID, LifecycleRegistrationOpen, LifecycleRegistrationClosed, fixedTime))

	h.clk.Time = edition.StartsAt.Add(time.Minute)
	h.mp.Set("BTC-ID", 50, "USD")

	completed, err := h.svc.RunCompetitionBaselines(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, completed)

	// The live quote moves on before the second participant is admitted and
	// baselined in a later pass.
	h.mp.Set("BTC-ID", 100, "USD")
	require.NoError(t, h.repo.CreateEngineEntry(ctx, admittedEntry("e2", edition.ID, "u2", "BTC-ID")))

	completed, err = h.svc.RunCompetitionBaselines(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, completed)

	u1, err := h.repo.GetEntry(ctx, edition.ID, "u1")
	require.NoError(t, err)
	u2, err := h.repo.GetEntry(ctx, edition.ID, "u2")
	require.NoError(t, err)
	for _, e := range []CompetitionEntry{*u1, *u2} {
		for _, s := range e.Snapshots {
			assert.Equal(t, 0, s.StartingPrice.Cmp(money.MustPrice("50")),
				"user %s must be baselined at the frozen start observation, not the later live quote", e.UserID)
		}
	}
}
