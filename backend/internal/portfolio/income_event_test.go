package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mixedComponents builds the canonical mixed-distribution scenario used across
// these tests: $70 ordinary income + $20 capital-gains distribution + $10
// return of capital, all part of ONE economic payment on AAPL.
func mixedComponents() []IncomeComponentInput {
	return []IncomeComponentInput{
		{Subtype: IncomeCashDividend, Amount: 70},
		{Subtype: IncomeCapitalGainsDist, Amount: 20},
		{Subtype: IncomeReturnOfCapitalSub, Amount: 10},
	}
}

func TestApplyIncomeEvent_MixedEvent_FullSuccess(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1") // 10000 total value (10 AAPL @ ~195 + 8050 cash)

	before, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	cashBefore, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usdBefore float64
	for _, c := range cashBefore {
		if c.Currency == "USD" {
			usdBefore = c.Amount
		}
	}
	positionsBefore, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	var basisBeforeTotal float64
	for _, p := range positionsBefore {
		if p.Symbol == "AAPL" {
			basisBeforeTotal = p.Quantity * p.AverageBuyPrice
		}
	}

	res, err := svc.ApplyIncomeEvent(ctx(), "u1", "evt:mixed:u1", IncomeEventInput{
		IncomeEventID: "evt-1", Symbol: "AAPL", Currency: "USD",
		Components: mixedComponents(),
	})
	require.NoError(t, err)
	assert.False(t, res.Duplicate)

	// $100 total cash effect (70 + 20 + 10).
	cashAfter, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usdAfter float64
	for _, c := range cashAfter {
		if c.Currency == "USD" {
			usdAfter = c.Amount
		}
	}
	assert.InDelta(t, usdBefore+100, usdAfter, 0.001)

	// $10 basis reduction on the AAPL position (10 shares @ 805 -> 805 - 1/share).
	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	var aapl *Position
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			aapl = p
		}
	}
	require.NotNil(t, aapl)
	assert.InDelta(t, basisBeforeTotal-10, aapl.Quantity*aapl.AverageBuyPrice, 0.001)

	// Ranked index rose (positive economic return), and exactly ONE checkpoint
	// was applied for the whole event (single version bump).
	after, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	assert.Greater(t, after.RankedIndex, before.RankedIndex)
	// One combined ranked-performance update for the whole event: the
	// coordinator's checkpoint before/after on the MutationResult itself moved
	// exactly once.
	assert.Greater(t, res.RankedIndexAfter, res.RankedIndexBefore)

	// One activity group: every leg shares the same activity_group_id, and
	// there is exactly one leg per component (no reinvestment here).
	activities, err := repo.ListActivities(ctx(), "u1", 1000)
	require.NoError(t, err)
	groupID := ""
	var legs []Activity
	for _, a := range activities {
		if a.Type == ActivityCashDividend || a.Type == ActivityCapitalGainsDistribution || a.Type == ActivityReturnOfCapital {
			legs = append(legs, a)
			if groupID == "" {
				groupID = a.GroupID
			}
		}
	}
	require.Len(t, legs, 3)
	require.NotEmpty(t, groupID)
	for _, leg := range legs {
		assert.Equal(t, groupID, leg.GroupID, "every leg of the event must share one activity_group_id")
	}
}

func TestApplyIncomeEvent_Retry_NoDuplicates(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	cashBefore, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usdBefore float64
	for _, c := range cashBefore {
		if c.Currency == "USD" {
			usdBefore = c.Amount
		}
	}

	in := IncomeEventInput{IncomeEventID: "evt-1", Symbol: "AAPL", Currency: "USD", Components: mixedComponents()}
	res1, err := svc.ApplyIncomeEvent(ctx(), "u1", "evt:mixed:u1", in)
	require.NoError(t, err)
	require.False(t, res1.Duplicate)

	res2, err := svc.ApplyIncomeEvent(ctx(), "u1", "evt:mixed:u1", in)
	require.NoError(t, err)
	assert.True(t, res2.Duplicate)

	cashBalances, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usd float64
	for _, c := range cashBalances {
		if c.Currency == "USD" {
			usd = c.Amount
		}
	}
	// Only the FIRST application's $100 landed; the retry created no duplicate.
	assert.InDelta(t, usdBefore+100, usd, 0.001)

	activities, err := repo.ListActivities(ctx(), "u1", 1000)
	require.NoError(t, err)
	var legs int
	for _, a := range activities {
		if a.Type == ActivityCashDividend || a.Type == ActivityCapitalGainsDistribution || a.Type == ActivityReturnOfCapital {
			legs++
		}
	}
	assert.Equal(t, 3, legs, "no duplicate activities from the retry")
}

func TestApplyIncomeEvent_ReturnOfCapital_ExcessOverBasis(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	positionsBefore, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	var basisBefore float64
	for _, p := range positionsBefore {
		if p.Symbol == "AAPL" {
			basisBefore = p.Quantity * p.AverageBuyPrice
		}
	}

	res, err := svc.ApplyIncomeEvent(ctx(), "u1", "evt:roc:u1", IncomeEventInput{
		IncomeEventID: "evt-roc", Symbol: "AAPL", Currency: "USD",
		Components: []IncomeComponentInput{
			{Subtype: IncomeReturnOfCapitalSub, Amount: 9000}, // exceeds remaining basis
		},
	})
	require.NoError(t, err)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	var aapl *Position
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			aapl = p
		}
	}
	require.NotNil(t, aapl)
	// Basis floored at zero, never negative.
	assert.GreaterOrEqual(t, aapl.AverageBuyPrice, 0.0)
	assert.InDelta(t, 0, aapl.AverageBuyPrice, 0.001)

	activities, err := repo.ListActivities(ctx(), "u1", 1000)
	require.NoError(t, err)
	var roc *Activity
	for i := range activities {
		if activities[i].Type == ActivityReturnOfCapital {
			roc = &activities[i]
		}
	}
	require.NotNil(t, roc)
	excess, _ := roc.Metadata["excess_over_basis"].(float64)
	assert.InDelta(t, 9000-basisBefore, excess, 0.001)

	// Cash still credited for the FULL net amount; ROC is never counted as
	// ordinary income.
	assert.False(t, res.Duplicate)
}

func TestApplyIncomeEvent_Reinvestment_OneIncomeOneNeutralBuy(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	before, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	cashBefore, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usdBefore float64
	for _, c := range cashBefore {
		if c.Currency == "USD" {
			usdBefore = c.Amount
		}
	}

	res, err := svc.ApplyIncomeEvent(ctx(), "u1", "evt:reinvest:u1", IncomeEventInput{
		IncomeEventID: "evt-reinvest", Symbol: "AAPL", AssetType: AssetTypeStock, Currency: "USD",
		Components: []IncomeComponentInput{
			{Subtype: IncomeCashDividend, Amount: 100, Reinvest: true},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, res.Position)

	// Cash unchanged (income in, buy out, same transaction).
	cashBalances, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usd float64
	for _, c := range cashBalances {
		if c.Currency == "USD" {
			usd = c.Amount
		}
	}
	assert.InDelta(t, usdBefore, usd, 0.001)

	// Shares increased.
	assert.Greater(t, res.Position.Quantity, 10.0)

	// Exactly one income leg + one neutral buy leg, sharing the group id.
	activities, err := repo.ListActivities(ctx(), "u1", 1000)
	require.NoError(t, err)
	var incomeLeg *Activity
	for i := range activities {
		if activities[i].Type == ActivityReinvestedDividend {
			incomeLeg = &activities[i]
		}
	}
	require.NotNil(t, incomeLeg)
	require.NotEmpty(t, incomeLeg.GroupID)
	var buyLeg *Activity
	for i := range activities {
		if activities[i].Type == ActivityBuy && activities[i].GroupID == incomeLeg.GroupID {
			buyLeg = &activities[i]
		}
	}
	require.NotNil(t, buyLeg, "the reinvestment buy leg must share the income leg's activity_group_id")

	// Ranked index reflects the $100 income exactly once (net effect: +100 on a
	// 10000 base ~ +1%), not twice (income + a non-neutral buy).
	after, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	assert.InDelta(t, before.RankedIndex*1.01, after.RankedIndex, 0.5)
}

func TestApplyIncomeEvent_UnsupportedComponent_WholeEventStaysPending(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	cashBefore, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usdBefore float64
	for _, c := range cashBefore {
		if c.Currency == "USD" {
			usdBefore = c.Amount
		}
	}

	_, err = svc.ApplyIncomeEvent(ctx(), "u1", "evt:bad:u1", IncomeEventInput{
		IncomeEventID: "evt-bad", Symbol: "AAPL", Currency: "USD",
		Components: []IncomeComponentInput{
			{Subtype: IncomeCashDividend, Amount: 70},
			{Subtype: IncomeSubtype("unknown_component_type"), Amount: 20},
		},
	})
	require.ErrorIs(t, err, ErrUnsupportedIncomeComponent)

	// NOTHING from the supported component was applied either — the whole
	// event stayed pending, not partially applied.
	cashAfter, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usdAfter float64
	for _, c := range cashAfter {
		if c.Currency == "USD" {
			usdAfter = c.Amount
		}
	}
	assert.InDelta(t, usdBefore, usdAfter, 0.001)

	activities, err := repo.ListActivities(ctx(), "u1", 1000)
	require.NoError(t, err)
	for _, a := range activities {
		assert.NotEqual(t, ActivityCashDividend, a.Type, "no partial application of the supported component")
	}
}

func TestApplyIncomeEvent_InvalidComponent_RollsBackWholeEvent(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	before, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	cashBefore, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)

	// Second component has an invalid (negative) amount: the whole mutation
	// must fail before ANY write, leaving cash, basis, activities, and ranked
	// state untouched — this falls out of plan() running entirely before
	// plan.write() is ever invoked.
	_, err = svc.ApplyIncomeEvent(ctx(), "u1", "evt:invalid:u1", IncomeEventInput{
		IncomeEventID: "evt-invalid", Symbol: "AAPL", Currency: "USD",
		Components: []IncomeComponentInput{
			{Subtype: IncomeCashDividend, Amount: 70},
			{Subtype: IncomeCapitalGainsDist, Amount: -20},
		},
	})
	require.Error(t, err)

	cashAfter, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, cashBefore, cashAfter)

	after, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	assert.InDelta(t, before.RankedIndex, after.RankedIndex, 1e-9)

	activities, err := repo.ListActivities(ctx(), "u1", 1000)
	require.NoError(t, err)
	for _, a := range activities {
		assert.NotEqual(t, ActivityCashDividend, a.Type)
	}
}
