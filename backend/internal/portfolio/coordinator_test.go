package portfolio

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// These tests exercise the aggregate transaction boundary directly: rollback on
// injected faults, serialization of concurrent mutations to one portfolio, and
// idempotent replay of duplicate requests.

func newTxTestService() (*Service, *InMemoryRepository, *performance.Service, *prices.MockPriceProvider) {
	pp := prices.NewMockPriceProvider()
	repo := NewInMemoryRepository()
	svc := NewService(repo, pp, fx.NewMockFXProvider())
	perf := performance.NewService(repo)
	perf.SetValuator(svc)
	return svc, repo, perf, pp
}

// aggregateState reads back the committed truth: open positions and ranked state.
func readAggregate(t *testing.T, repo *InMemoryRepository, svc *Service, userID string) ([]*Position, *performance.State) {
	t.Helper()
	positions, err := svc.ListPositions(context.Background(), userID)
	require.NoError(t, err)
	pf, err := repo.GetPortfolioByUser(context.Background(), userID)
	if errors.Is(err, ErrPortfolioNotFound) {
		return positions, nil // aggregate was never created
	}
	require.NoError(t, err)
	state, err := repo.GetByPortfolio(context.Background(), pf.ID)
	if errors.Is(err, performance.ErrStateNotFound) {
		return positions, nil
	}
	require.NoError(t, err)
	return positions, state
}

// --- rollback: no partial core state ------------------------------------------

func TestRollback_FaultAtEveryStageLeavesNoPartialState(t *testing.T) {
	boom := errors.New("injected failure")
	stages := map[string]Faults{
		"position insert": {CreatePosition: boom},
		"ranked insert":   {InsertRankedState: boom},
		"version bump":    {SetPortfolioVersion: boom},
		"audit record":    {RecordAudit: boom},
		"outbox insert":   {AppendOutbox: boom},
		"commit":          {Commit: boom},
	}

	for name, faults := range stages {
		t.Run(name, func(t *testing.T) {
			svc, repo, _, _ := newTxTestService()
			repo.SetFaults(faults)

			_, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
			require.ErrorIs(t, err, boom, "the mutation must fail at the %s stage", name)

			positions, state := readAggregate(t, repo, svc, "u1")
			assert.Empty(t, positions, "no position may survive a rolled-back mutation")
			assert.Nil(t, state, "no ranked state may survive a rolled-back mutation")
			assert.Empty(t, repo.OutboxEvents(), "no outbox event may survive a rollback")
			assert.Empty(t, repo.AuditLog(), "no audit record may survive a rollback")
		})
	}
}

func TestRollback_FaultOnSecondMutationPreservesFirst(t *testing.T) {
	boom := errors.New("injected failure")
	svc, repo, _, _ := newTxTestService()

	first, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	_, stateBefore := readAggregate(t, repo, svc, "u1")
	require.NotNil(t, stateBefore)

	// A failing ranked-state UPDATE must not leave the position change behind.
	repo.SetFaults(Faults{UpdateRankedState: boom})
	_, err = svc.UpdatePosition(ctx(), "u1", first.ID, 999)
	require.ErrorIs(t, err, boom)

	positions, stateAfter := readAggregate(t, repo, svc, "u1")
	require.Len(t, positions, 1)
	assert.Equal(t, 1.0, positions[0].Quantity, "quantity must be unchanged after rollback")
	require.NotNil(t, stateAfter)
	assert.Equal(t, stateBefore.Version, stateAfter.Version, "ranked version must not advance")
	assert.Equal(t, *stateBefore.SegmentStartValueBase, *stateAfter.SegmentStartValueBase)
}

// --- concurrency ---------------------------------------------------------------

// assertConsistent reloads committed state and verifies the central invariant:
// the ranked segment start corresponds to the FINAL active position set.
func assertConsistent(t *testing.T, repo *InMemoryRepository, svc *Service, perf *performance.Service, userID string) {
	t.Helper()
	positions, state := readAggregate(t, repo, svc, userID)
	require.NotNil(t, state)

	_, valueBase, hasActive, err := svc.PortfolioValueBase(context.Background(), userID)
	require.NoError(t, err)

	if !hasActive {
		assert.Equal(t, performance.StatusPaused, state.Status)
		assert.Nil(t, state.SegmentStartValueBase)
		return
	}
	require.Equal(t, performance.StatusActive, state.Status)
	require.NotNil(t, state.SegmentStartValueBase)
	assert.InDeltaf(t, valueBase, *state.SegmentStartValueBase, 1e-6,
		"segment start (%v) must equal the value of the final %d-position set (%v)",
		*state.SegmentStartValueBase, len(positions), valueBase)

	// No mutation-generated return: with prices unchanged since the last
	// checkpoint, the live index must still equal the stored checkpoint.
	rp, err := perf.CurrentRankedPerformance(context.Background(), userID)
	require.NoError(t, err)
	assert.InDelta(t, state.CheckpointIndex, rp.RankedIndex, 1e-6)
}

func TestConcurrent_SimultaneousAddsSerialize(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	positions, state := readAggregate(t, repo, svc, "u1")
	assert.Len(t, positions, n)
	// Version advanced exactly once per committed mutation — no lost updates.
	assert.Equal(t, int64(n), state.Version)
	assert.Len(t, repo.OutboxEvents(), n, "one event per committed mutation")
	assertConsistent(t, repo, svc, perf, "u1")
}

func TestConcurrent_MixedMutationsRemainConsistent(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	seed, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 5})
	require.NoError(t, err)
	other, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "MSFT", AssetType: "stock", Quantity: 2})
	require.NoError(t, err)

	var wg sync.WaitGroup
	run := func(fn func()) {
		wg.Add(1)
		go func() { defer wg.Done(); fn() }()
	}
	// Add, resize, delete, close and a whole-portfolio replacement all racing.
	run(func() {
		_, _ = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "SPY", AssetType: "etf", Quantity: 3})
	})
	run(func() { _, _ = svc.UpdatePosition(ctx(), "u1", seed.ID, 11) })
	run(func() { _ = svc.DeletePosition(ctx(), "u1", other.ID) })
	run(func() { _, _ = svc.ClosePosition(ctx(), "u1", seed.ID) })
	run(func() {
		_ = svc.ReplaceWithStrategyWeights(ctx(), "u1", []StrategyWeightInput{
			{Symbol: "NVDA", AssetType: "stock", WeightPercentage: 100},
		})
	})
	wg.Wait()

	// Whatever interleaving won, the aggregate must be internally consistent.
	assertConsistent(t, repo, svc, perf, "u1")
}

func TestConcurrent_DoubleCloseAndDoubleDeleteAreSafe(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	pos, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 4})
	require.NoError(t, err)
	_, err = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "MSFT", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)

	// Two concurrent closes of the SAME position: exactly one may succeed.
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = svc.ClosePosition(ctx(), "u1", pos.ID)
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, ErrPositionClosed)
		}
	}
	assert.Equal(t, 1, successes, "a position may only be closed once")
	assertConsistent(t, repo, svc, perf, "u1")
}

func TestConcurrent_LastDeleteAndAddRace(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	pos, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = svc.DeletePosition(ctx(), "u1", pos.ID) }()
	go func() {
		defer wg.Done()
		_, _ = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "SPY", AssetType: "etf", Quantity: 1})
	}()
	wg.Wait()

	// Either ordering is valid; the state must match the final composition and
	// pause only when genuinely empty.
	assertConsistent(t, repo, svc, perf, "u1")
}

func TestConcurrent_DefaultPortfolioCreationRace(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	const n = 10
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if pf, err := svc.GetOrCreateDefaultPortfolio(ctx(), "u1"); err == nil {
				ids[i] = pf.ID
			}
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		assert.Equal(t, ids[0], ids[i], "one default portfolio per user")
	}
	pf, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, ids[0], pf.ID)
}

func TestConcurrent_RankedStateInitializedExactlyOnce(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	const n = 6
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
		}()
	}
	wg.Wait()

	_, state := readAggregate(t, repo, svc, "u1")
	require.NotNil(t, state)
	// Initialized once at 100, then one increment per further mutation.
	assert.Equal(t, int64(n), state.Version)
	assert.InDelta(t, 100.0, state.CheckpointIndex, 1e-9,
		"all mutations are neutral, so the checkpoint stays at the epoch index")
}

// --- idempotency ----------------------------------------------------------------

func TestIdempotency_RetriedAddAppliesOnce(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	req := MutationRequest{
		Kind: MutationAdd, UserID: "u1", RequestID: "req-add-1",
		Input: PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 3},
	}
	first, err := svc.Mutate(ctx(), req)
	require.NoError(t, err)
	require.False(t, first.Duplicate)

	second, err := svc.Mutate(ctx(), req) // client retry after a timeout
	require.NoError(t, err)
	assert.True(t, second.Duplicate, "retry must be recognised as a duplicate")
	require.NotNil(t, second.Position)
	assert.Equal(t, first.Position.ID, second.Position.ID, "the original result is replayed")

	positions, state := readAggregate(t, repo, svc, "u1")
	assert.Len(t, positions, 1, "a retry must not create a second position")
	assert.Equal(t, int64(1), state.Version, "a retry must not advance ranked state")
	assert.Len(t, repo.OutboxEvents(), 1, "a retry must not emit a second event")
}

func TestIdempotency_RetriedCloseAndReplaceApplyOnce(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	pos, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 2})
	require.NoError(t, err)
	_, err = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "MSFT", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)

	closeReq := MutationRequest{Kind: MutationClose, UserID: "u1", RequestID: "req-close-1", PositionID: pos.ID}
	_, err = svc.Mutate(ctx(), closeReq)
	require.NoError(t, err)
	dup, err := svc.Mutate(ctx(), closeReq)
	require.NoError(t, err, "a retried close must not fail with 'already closed'")
	assert.True(t, dup.Duplicate)

	replaceReq := MutationRequest{
		Kind: MutationReplace, UserID: "u1", RequestID: "req-replace-1",
		Weights: []StrategyWeightInput{{Symbol: "SPY", AssetType: "etf", WeightPercentage: 100}},
	}
	_, err = svc.Mutate(ctx(), replaceReq)
	require.NoError(t, err)
	dup, err = svc.Mutate(ctx(), replaceReq)
	require.NoError(t, err)
	assert.True(t, dup.Duplicate)

	open, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	assert.Len(t, open, 1, "the replacement must have been applied exactly once")
	assert.Equal(t, "SPY", open[0].Symbol)

	events := repo.OutboxEvents()
	assert.Len(t, events, 4, "add, add, close, replace — retries emit nothing extra")
}

// --- audit trail ------------------------------------------------------------------

func TestAudit_ProvesEveryMutationIsRankNeutral(t *testing.T) {
	svc, repo, _, pp := newTxTestService()
	pos, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	pp.Set("AAPL", 300, "USD") // market move between mutations
	_, err = svc.UpdatePosition(ctx(), "u1", pos.ID, 50)
	require.NoError(t, err)
	pp.Set("AAPL", 150, "USD")
	_, err = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "SPY", AssetType: "etf", Quantity: 10})
	require.NoError(t, err)
	require.NoError(t, svc.DeletePosition(ctx(), "u1", pos.ID))

	log := repo.AuditLog()
	require.Len(t, log, 4)
	for _, entry := range log {
		assert.InDeltaf(t, entry.RankedIndexBefore, entry.RankedIndexAfter, 1e-9,
			"%s mutation generated ranked return: %v -> %v",
			entry.MutationType, entry.RankedIndexBefore, entry.RankedIndexAfter)
		assert.Equal(t, entry.PortfolioVersionBefore+1, entry.PortfolioVersionAfter)
		assert.Equal(t, entry.PerformanceVersionBefore+1, entry.PerformanceVersionAfter)
		assert.False(t, math.IsNaN(entry.RankedIndexAfter))
	}
}

// --- context ------------------------------------------------------------------------

func TestContext_CancelledBeforeMutationDoesNothing(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.AddPosition(cancelled, "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.Error(t, err, "a cancelled context must abort the mutation")

	positions, _ := readAggregate(t, repo, svc, "u1")
	assert.Empty(t, positions)
	assert.Empty(t, repo.OutboxEvents())
}
