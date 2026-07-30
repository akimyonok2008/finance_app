package rules

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// exampleOne and exampleTwo are the two specification examples proving that
// eligibility and scoring are independent documents: identical eligibility,
// different scoring scopes.
const exampleEligibility = `{
	"schema_version": 1,
	"all": [
		{
			"code": "minimum_crypto_weight",
			"label": "At least 30% crypto exposure",
			"metric": "portfolio_weight",
			"filter": {"asset_types": ["crypto"]},
			"operator": "gte",
			"value": "0.30"
		}
	]
}`

const exampleScoringFull = `{
	"schema_version": 1,
	"scope": "full_portfolio",
	"include_cash": true
}`

const exampleScoringMatching = `{
	"schema_version": 1,
	"scope": "matching_assets",
	"filter": {"asset_types": ["crypto"]},
	"include_cash": false
}`

func pos(instrumentID, assetType, mic, currency, valueBase string) PositionFacts {
	return PositionFacts{
		InstrumentID: instrumentID, AssetType: assetType, VenueMIC: mic,
		Currency: currency, ValueBase: money.MustAmount(valueBase),
	}
}

// factsWith builds PortfolioFacts with TotalValueBase = positions + cash.
func factsWith(cash string, positions ...PositionFacts) PortfolioFacts {
	total := money.MustAmount(cash)
	for _, p := range positions {
		total = total.Add(p.ValueBase)
	}
	return PortfolioFacts{
		Positions:      positions,
		CashValueBase:  money.MustAmount(cash),
		TotalValueBase: total,
	}
}

func TestParse_SpecExamplesValidate(t *testing.T) {
	e, s, err := ValidateDefinitionVersionPayloads([]byte(exampleEligibility), []byte(exampleScoringFull))
	require.NoError(t, err)
	assert.Len(t, e.All, 1)
	assert.Equal(t, ScopeFullPortfolio, s.Scope)
	assert.True(t, s.IncludeCash)

	_, s2, err := ValidateDefinitionVersionPayloads([]byte(exampleEligibility), []byte(exampleScoringMatching))
	require.NoError(t, err)
	assert.Equal(t, ScopeMatchingAssets, s2.Scope)
	assert.False(t, s2.IncludeCash)
	assert.Equal(t, []string{"crypto"}, s2.Filter.AssetTypes)
}

func TestParseEligibility_RejectsInvalidDocuments(t *testing.T) {
	cases := map[string]string{
		"unknown field":       `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"position_count","operator":"gte","value":"1","bogus":true}]}`,
		"unknown metric":      `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"vibes","operator":"gte","value":"1"}]}`,
		"unknown operator":    `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"position_count","operator":"~=","value":"1"}]}`,
		"float-ish value":     `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"position_count","operator":"gte","value":"abc"}]}`,
		"negative value":      `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"position_count","operator":"gte","value":"-1"}]}`,
		"missing code":        `{"schema_version":1,"all":[{"label":"l","metric":"position_count","operator":"gte","value":"1"}]}`,
		"no rules":            `{"schema_version":1}`,
		"wrong schema":        `{"schema_version":9,"all":[{"code":"c","label":"l","metric":"position_count","operator":"gte","value":"1"}]}`,
		"duplicate codes":     `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"position_count","operator":"gte","value":"1"},{"code":"c","label":"l2","metric":"asset_class_count","operator":"gte","value":"1"}]}`,
		"weight sans filter":  `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"portfolio_weight","operator":"gte","value":"0.3"}]}`,
		"filter not accepted": `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"cash_weight","filter":{"asset_types":["crypto"]},"operator":"gte","value":"0.3"}]}`,
		"between inverted":    `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"position_count","operator":"between","value":"5","value_high":"2"}]}`,
		"stray value_high":    `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"position_count","operator":"gte","value":"1","value_high":"2"}]}`,
		"empty filter value":  `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"portfolio_weight","filter":{"asset_types":[""]},"operator":"gte","value":"0.3"}]}`,
	}
	for name, doc := range cases {
		_, err := ParseEligibility([]byte(doc))
		assert.Error(t, err, name)
	}
}

func TestParseScoring_RejectsInvalidDocuments(t *testing.T) {
	cases := map[string]string{
		"unknown scope":           `{"schema_version":1,"scope":"vibes","include_cash":true}`,
		"full portfolio + filter": `{"schema_version":1,"scope":"full_portfolio","filter":{"asset_types":["crypto"]},"include_cash":true}`,
		"matching sans filter":    `{"schema_version":1,"scope":"matching_assets","include_cash":false}`,
		"universe sans name":      `{"schema_version":1,"scope":"custom_universe","filter":{"asset_types":["etf"]},"include_cash":false}`,
		"unknown field":           `{"schema_version":1,"scope":"full_portfolio","include_cash":true,"leverage":3}`,
	}
	for name, doc := range cases {
		_, err := ParseScoring([]byte(doc))
		assert.Error(t, err, name)
	}
}

func TestParseScoring_CustomUniverseAndLegacyFlags(t *testing.T) {
	s, err := ParseScoring([]byte(`{"schema_version":1,"scope":"custom_universe","filter":{"universe":"tech-v1"},"include_cash":false}`))
	require.NoError(t, err)
	assert.Equal(t, "tech-v1", s.Filter.Universe)

	legacy, err := ParseScoring([]byte(`{"schema_version":1,"scope":"full_portfolio","include_cash":false,"legacy_join_time_baseline":true}`))
	require.NoError(t, err, "the migrated legacy sprint document must stay parseable")
	assert.True(t, legacy.LegacyJoinTimeBaseline)
}

func TestEvaluate_CryptoWeightThresholdExactDecimals(t *testing.T) {
	elig, err := ParseEligibility([]byte(exampleEligibility))
	require.NoError(t, err)
	now := time.Now()

	// 24.17% crypto: 2417 crypto / 10000 total (7583 stock, 0 cash).
	failing := factsWith("0", pos("i1", "crypto", "", "USD", "2417"), pos("i2", "stock", "", "USD", "7583"))
	res := Evaluate(elig, failing, now)
	assert.False(t, res.Eligible)
	require.Len(t, res.Rules, 1)
	ev := res.Rules[0]
	assert.Equal(t, "minimum_crypto_weight", ev.Code)
	assert.Equal(t, "30.00", ev.Required, "weight thresholds render as percentages")
	assert.Equal(t, "24.17", ev.Actual)
	assert.False(t, ev.Passed)
	assert.Equal(t, ReasonBelowRequired, ev.Reason)

	// Exactly 30.00%: gte must pass on the exact boundary — the classic
	// float-comparison bug this engine exists to avoid.
	boundary := factsWith("0", pos("i1", "crypto", "", "USD", "30"), pos("i2", "stock", "", "USD", "70"))
	res = Evaluate(elig, boundary, now)
	assert.True(t, res.Eligible)
	assert.Equal(t, "30.00", res.Rules[0].Actual)

	// Cash dilutes weight: 30 crypto of 100 positions + 20 cash = 25%.
	diluted := factsWith("20", pos("i1", "crypto", "", "USD", "30"), pos("i2", "stock", "", "USD", "70"))
	res = Evaluate(elig, diluted, now)
	assert.False(t, res.Eligible)
	assert.Equal(t, "25.00", res.Rules[0].Actual)
}

func TestEvaluate_AllAndAnyGroups(t *testing.T) {
	doc := `{
		"schema_version": 1,
		"all": [
			{"code":"min_positions","label":"At least 2 positions","metric":"position_count","operator":"gte","value":"2"}
		],
		"any": [
			{"code":"crypto_30","label":"30% crypto","metric":"portfolio_weight","filter":{"asset_types":["crypto"]},"operator":"gte","value":"0.30"},
			{"code":"etf_50","label":"50% ETF","metric":"portfolio_weight","filter":{"asset_types":["etf"]},"operator":"gte","value":"0.50"}
		]
	}`
	elig, err := ParseEligibility([]byte(doc))
	require.NoError(t, err)
	now := time.Now()

	// Satisfies all + second any-rule only.
	etfHeavy := factsWith("0", pos("i1", "etf", "", "USD", "60"), pos("i2", "stock", "", "USD", "40"))
	res := Evaluate(elig, etfHeavy, now)
	assert.True(t, res.Eligible)
	require.Len(t, res.Rules, 3, "evidence must cover every rule, passing or not")

	// Satisfies all but neither any-rule.
	neither := factsWith("0", pos("i1", "etf", "", "USD", "10"), pos("i2", "stock", "", "USD", "90"))
	assert.False(t, Evaluate(elig, neither, now).Eligible)

	// Satisfies an any-rule but fails all.
	single := factsWith("0", pos("i1", "crypto", "", "USD", "100"))
	assert.False(t, Evaluate(elig, single, now).Eligible)
}

func TestEvaluate_MetricsAndOperators(t *testing.T) {
	now := time.Now()
	facts := factsWith("25",
		pos("i1", "stock", "XIST", "TRY", "50"),
		pos("i2", "stock", "XNAS", "USD", "15"),
		pos("i3", "crypto", "", "USD", "10"),
	) // total 100: cash 25%, XIST 50%, largest 50%
	facts.PortfolioAgeDays = 45

	cases := []struct {
		name string
		doc  string
		want bool
	}{
		{"cash lte", `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"cash_weight","operator":"lte","value":"0.25"}]}`, true},
		{"cash lt fails on boundary", `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"cash_weight","operator":"lt","value":"0.25"}]}`, false},
		{"venue MIC weight (XIST)", `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"portfolio_weight","filter":{"venue_mics":["XIST"]},"operator":"gte","value":"0.50"}]}`, true},
		{"largest position between", `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"largest_position_weight","operator":"between","value":"0.40","value_high":"0.60"}]}`, true},
		{"asset class count eq", `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"asset_class_count","operator":"eq","value":"2"}]}`, true},
		{"portfolio age", `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"portfolio_age_days","operator":"gte","value":"30"}]}`, true},
		{"filtered position count", `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"position_count","filter":{"currencies":["USD"]},"operator":"eq","value":"2"}]}`, true},
		{"currency filter weight", `{"schema_version":1,"all":[{"code":"c","label":"l","metric":"portfolio_weight","filter":{"currencies":["TRY"]},"operator":"gt","value":"0.49"}]}`, true},
	}
	for _, tc := range cases {
		elig, err := ParseEligibility([]byte(tc.doc))
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.want, Evaluate(elig, facts, now).Eligible, tc.name)
	}
}

func TestEvaluate_UniverseMembershipFailsClosed(t *testing.T) {
	doc := `{"schema_version":1,"all":[{"code":"tech","label":"Tech exposure","metric":"portfolio_weight","filter":{"universe":"tech-v1"},"operator":"gte","value":"0.40"}]}`
	elig, err := ParseEligibility([]byte(doc))
	require.NoError(t, err)
	now := time.Now()
	facts := factsWith("0", pos("aapl-id", "stock", "XNAS", "USD", "60"), pos("ko-id", "stock", "XNYS", "USD", "40"))

	// Universe not resolved at all: rule fails with an explicit reason, never
	// silently passes.
	res := Evaluate(elig, facts, now)
	assert.False(t, res.Eligible)
	assert.Equal(t, ReasonUnknownUniverse, res.Rules[0].Reason)

	// Resolved universe: membership is by stable instrument ID.
	facts.UniverseMembership = map[string]map[string]bool{"tech-v1": {"aapl-id": true}}
	res = Evaluate(elig, facts, now)
	assert.True(t, res.Eligible)
	assert.Equal(t, "60.00", res.Rules[0].Actual)
}

func TestEvaluate_EmptyPortfolioFailsClosedWithReason(t *testing.T) {
	elig, err := ParseEligibility([]byte(exampleEligibility))
	require.NoError(t, err)
	res := Evaluate(elig, PortfolioFacts{TotalValueBase: money.ZeroAmount(), CashValueBase: money.ZeroAmount()}, time.Now())
	assert.False(t, res.Eligible)
	assert.Equal(t, ReasonEmptyPortfolio, res.Rules[0].Reason)
}

// TestEvidence_NeverContainsAbsoluteValues serializes evidence for a large
// portfolio and asserts the private base-currency figures appear nowhere:
// evidence is percentages, counts and day counts only.
func TestEvidence_NeverContainsAbsoluteValues(t *testing.T) {
	elig, err := ParseEligibility([]byte(exampleEligibility))
	require.NoError(t, err)
	facts := factsWith("123456.78", pos("i1", "crypto", "", "USD", "987654.32"), pos("i2", "stock", "", "USD", "555555.55"))
	res := Evaluate(elig, facts, time.Now())

	serialized, err := json.Marshal(res)
	require.NoError(t, err)
	for _, private := range []string{"987654", "555555", "123456", "1666666"} {
		assert.False(t, strings.Contains(string(serialized), private),
			"evidence must not leak absolute value %q", private)
	}
}

func TestMatches_DimensionsANDValuesOR(t *testing.T) {
	p := pos("id1", "stock", "XIST", "TRY", "1")
	p.IssuerCountry = "TR"

	assert.True(t, Matches(&Filter{VenueMICs: []string{"XIST", "XNAS"}}, p, nil), "values within a dimension OR")
	assert.True(t, Matches(&Filter{VenueMICs: []string{"XIST"}, AssetTypes: []string{"stock"}}, p, nil), "dimensions AND")
	assert.False(t, Matches(&Filter{VenueMICs: []string{"XIST"}, AssetTypes: []string{"etf"}}, p, nil))
	assert.False(t, Matches(&Filter{IssuerCountries: []string{"US"}}, p, nil))
	assert.True(t, Matches(nil, p, nil), "no filter matches everything")
}
