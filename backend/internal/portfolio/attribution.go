package portfolio

import (
	"math"
	"sort"
)

// StandaloneFeesBase is the ONE definition of "fees that still have to be
// subtracted from economic attribution".
//
// Buy/sell fees are already netted into cost basis and realized P&L by the
// canonical trade contract, so FeeMetrics discloses that embedded portion
// separately; subtracting TotalFeesBase in full would double-count them. Both
// ReconcilePortfolioFinancials and the Performance tab's economic breakdown go
// through this function so they can never drift apart.
func StandaloneFeesBase(fees FeeMetrics) float64 {
	return round2(fees.TotalFeesBase - fees.EmbeddedInRealizedPnLBase)
}

// EconomicAttribution is the Performance tab's economic breakdown. It is the
// SAME decomposition ReconcilePortfolioFinancials checks the ledger's total
// economic result against — realized + unrealized + income - standalone fees —
// exposed for display instead of being recomputed by the client.
//
// TotalEconomicPnLBase is the LEDGER figure (current value + withdrawals -
// deposits). It is never derived from the ranked index. When the ledger cannot
// cover the full holding history it is nil and IsComplete is false, so the UI
// shows a truthful gap rather than a zero.
type EconomicAttribution struct {
	RealizedPnLBase    float64 `json:"realized_pnl_base"`
	UnrealizedPnLBase  float64 `json:"unrealized_pnl_base"`
	NetIncomeBase      float64 `json:"net_income_base"`
	StandaloneFeesBase float64 `json:"standalone_fees_base"`
	// AttributedTotalBase is realized + unrealized + income - standalone fees.
	AttributedTotalBase float64 `json:"attributed_total_base"`
	// TotalEconomicPnLBase is the ledger's own total. nil when incomplete.
	TotalEconomicPnLBase *float64 `json:"total_economic_pnl_base"`
	// UnattributedBase is ledger total - attributed total. A non-zero value is
	// disclosed rather than hidden; ReconcilePortfolioFinancials flags it.
	UnattributedBase  *float64 `json:"unattributed_base"`
	CalculationStatus string   `json:"calculation_status"`
	IsComplete        bool     `json:"is_complete"`
}

// CalculateEconomicAttribution assembles the breakdown from Step 1's existing
// metric groups. It performs no independent re-derivation of realized P&L,
// income, or fees — it only regroups the figures the ledger already produced.
func CalculateEconomicAttribution(
	open OpenHoldingsMetrics,
	realized RealizedMetrics,
	income IncomeMetrics,
	fees FeeMetrics,
	economic EconomicPerformance,
) EconomicAttribution {
	standalone := StandaloneFeesBase(fees)
	out := EconomicAttribution{
		RealizedPnLBase:    round2(realized.RealizedPnLBase),
		UnrealizedPnLBase:  round2(open.UnrealizedPnLBase),
		NetIncomeBase:      round2(income.TotalIncomeBase),
		StandaloneFeesBase: standalone,
		AttributedTotalBase: round2(
			open.UnrealizedPnLBase + realized.RealizedPnLBase +
				income.TotalIncomeBase - standalone,
		),
		CalculationStatus: economic.CalculationStatus,
		IsComplete:        economic.IsComplete,
	}
	if economic.IsComplete && economic.TotalPnLBase != nil {
		total := round2(*economic.TotalPnLBase)
		unattributed := round2(total - out.AttributedTotalBase)
		out.TotalEconomicPnLBase = &total
		out.UnattributedBase = &unattributed
	}
	return out
}

// InstrumentEconomics is one instrument's ledger-sourced economic result and
// the capital that produced it.
type InstrumentEconomics struct {
	Symbol            string
	AssetType         string
	CapitalBase       float64 // cost basis committed (open + closed episodes)
	UnrealizedPnLBase float64
	RealizedPnLBase   float64
	IncomeBase        float64
	FeesBase          float64 // standalone (non-embedded) fees booked to it
}

// TotalPnLBase is the instrument's economic result: price movement plus income
// minus its own standalone fees.
func (i InstrumentEconomics) TotalPnLBase() float64 {
	return i.UnrealizedPnLBase + i.RealizedPnLBase + i.IncomeBase - i.FeesBase
}

// InstrumentContribution is one instrument's contribution to portfolio return,
// in PERCENTAGE POINTS of the portfolio — not its standalone return.
type InstrumentContribution struct {
	Symbol    string `json:"symbol"`
	AssetType string `json:"asset_type,omitempty"`
	// WeightPercentage is the instrument's share of total committed capital.
	WeightPercentage float64 `json:"weight_percentage"`
	// InstrumentReturnPercentage is its standalone return, shown for context
	// only. Ranking is NEVER done on this field.
	InstrumentReturnPercentage *float64 `json:"instrument_return_percentage"`
	// ContributionPercentagePoints is weight x instrument return, i.e. the
	// instrument's economic result as a share of total committed capital.
	ContributionPercentagePoints float64 `json:"contribution_percentage_points"`
	EconomicResultBase           float64 `json:"economic_result_base"`
	IncomeBase                   float64 `json:"income_base"`
}

// ContributionAnalysis is the Contributors / Detractors block.
//
// HONEST SCOPE — read this before trusting the numbers:
//
// Contribution is computed SINCE INCEPTION, not over a user-selected timeframe.
// A period-accurate attribution needs each instrument's valuation at the start
// AND end of the period at daily granularity; this system records portfolio-level
// ranked snapshots and portfolio-level valuation archives, but no per-instrument
// valuation history. Rather than fabricate period weights, the analysis is
// reported with Basis "since_inception" and the UI refuses to present it as a
// period figure.
//
// Within that scope the number is a real weight x return decomposition:
//
//	contribution_i = (capital_i / total_capital) * (result_i / capital_i)
//	              =  result_i / total_capital
//
// which is exactly the required form and is NOT a standalone instrument return.
// Contributions sum to the portfolio's own capital-weighted economic return, so
// they are additive and comparable across instruments.
type ContributionAnalysis struct {
	Basis             string                   `json:"basis"`
	CalculationStatus string                   `json:"calculation_status"`
	Available         bool                     `json:"available"`
	Reason            string                   `json:"reason,omitempty"`
	TotalCapitalBase  float64                  `json:"total_capital_base"`
	Contributors      []InstrumentContribution `json:"contributors"`
	Detractors        []InstrumentContribution `json:"detractors"`
	// UnattributedPercentagePoints is the part of the portfolio's economic
	// result that could NOT be traced to a specific instrument (portfolio-level
	// income such as cash interest, and management/custody fees). It is
	// disclosed, never silently folded into an instrument.
	UnattributedPercentagePoints float64 `json:"unattributed_percentage_points"`
}

// TopContributionCount is how many contributors and detractors are reported.
const TopContributionCount = 3

const (
	contributionReasonNoCapital = "Contribution analysis is unavailable: no committed capital has been recorded yet."
	// ContributionBasisSinceInception documents the scope limitation above.
	ContributionBasisSinceInception = "since_inception"
)

// CalculateContributions ranks instruments by their contribution to the
// portfolio, in percentage points.
//
// portfolioLevel is the economic result that belongs to no instrument (cash
// interest, management and custody fees); it is reported as unattributed.
func CalculateContributions(instruments []InstrumentEconomics, portfolioLevelBase float64) ContributionAnalysis {
	out := ContributionAnalysis{
		Basis:             ContributionBasisSinceInception,
		CalculationStatus: "complete",
		Contributors:      []InstrumentContribution{},
		Detractors:        []InstrumentContribution{},
	}

	total := 0.0
	for _, ins := range instruments {
		total += ins.CapitalBase
	}
	total = round2(total)
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		out.CalculationStatus = "incomplete"
		out.Reason = contributionReasonNoCapital
		return out
	}
	out.Available = true
	out.TotalCapitalBase = total

	rows := make([]InstrumentContribution, 0, len(instruments))
	for _, ins := range instruments {
		result := ins.TotalPnLBase()
		row := InstrumentContribution{
			Symbol:                       ins.Symbol,
			AssetType:                    ins.AssetType,
			WeightPercentage:             round2(ins.CapitalBase / total * 100),
			ContributionPercentagePoints: round4(result / total * 100),
			EconomicResultBase:           round2(result),
			IncomeBase:                   round2(ins.IncomeBase),
		}
		if ins.CapitalBase > 0 {
			ret := round2(result / ins.CapitalBase * 100)
			row.InstrumentReturnPercentage = &ret
		}
		rows = append(rows, row)
	}

	// Deterministic ordering: contribution first, symbol as the tiebreak.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ContributionPercentagePoints == rows[j].ContributionPercentagePoints {
			return rows[i].Symbol < rows[j].Symbol
		}
		return rows[i].ContributionPercentagePoints > rows[j].ContributionPercentagePoints
	})

	for _, row := range rows {
		if row.ContributionPercentagePoints > 0 && len(out.Contributors) < TopContributionCount {
			out.Contributors = append(out.Contributors, row)
		}
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].ContributionPercentagePoints < 0 && len(out.Detractors) < TopContributionCount {
			out.Detractors = append(out.Detractors, rows[i])
		}
	}

	out.UnattributedPercentagePoints = round4(portfolioLevelBase / total * 100)
	if out.UnattributedPercentagePoints != 0 {
		out.CalculationStatus = "incomplete"
	}
	return out
}

// buildInstrumentEconomics joins the position layer's committed capital and
// unrealized P&L with the ledger's per-symbol realized P&L, income and
// standalone fees. Realized P&L always comes from the ledger (partial sells
// leave the position open, so closed-position rows alone would under-report it);
// committed capital comes from both the open and the closed episodes.
func buildInstrumentEconomics(
	open []PositionSummary,
	closed []ClosedPositionSummary,
	ledger ledgerMetrics,
) []InstrumentEconomics {
	merged := map[string]*InstrumentEconomics{}
	get := func(symbol, assetType string) *InstrumentEconomics {
		key := normalizeSymbol(symbol)
		entry, ok := merged[key]
		if !ok {
			entry = &InstrumentEconomics{Symbol: key, AssetType: assetType}
			merged[key] = entry
		}
		if entry.AssetType == "" {
			entry.AssetType = assetType
		}
		return entry
	}

	for _, position := range open {
		entry := get(position.Symbol, position.AssetType)
		entry.CapitalBase += position.CostBasisBase
		entry.UnrealizedPnLBase += position.GainLossBase
	}
	for _, position := range closed {
		entry := get(position.Symbol, position.AssetType)
		entry.CapitalBase += position.ClosedCostBasisBase
	}
	for key, ledgerEntry := range ledger.bySymbol {
		if key == "" {
			continue
		}
		entry := get(key, ledgerEntry.AssetType)
		entry.RealizedPnLBase += ledgerEntry.RealizedPnLBase
		entry.IncomeBase += ledgerEntry.IncomeBase
		entry.FeesBase += ledgerEntry.FeesBase
	}

	out := make([]InstrumentEconomics, 0, len(merged))
	for _, entry := range merged {
		if entry.Symbol == "" {
			continue
		}
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}
