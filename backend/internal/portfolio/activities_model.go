package portfolio

import (
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// This file extends the immutable activity ledger with income, fee, and
// corporate-action mechanics. Everything here is USER-REPORTED tracking data:
// the application records portfolio activity, it does not execute trades,
// connect to a brokerage, calculate tax, or give advice.

// Additional activity types layered on top of the base ledger
// (deposit/withdrawal/buy/sell/opening_balance in model.go).
const (
	// Income (return-bearing, positive).
	ActivityCashDividend       ActivityType = "cash_dividend"
	ActivityETFDistribution    ActivityType = "etf_distribution"
	ActivityInterestIncome     ActivityType = "interest_income"
	ActivityReinvestedDividend ActivityType = "reinvested_dividend"
	// Additional provider-driven income (return-bearing, positive).
	ActivityCapitalGainsDistribution ActivityType = "capital_gains_distribution"
	ActivityBondCoupon               ActivityType = "bond_coupon"
	ActivityCashInterest             ActivityType = "cash_interest"
	ActivityStakingReward            ActivityType = "staking_reward"
	ActivityOtherIncome              ActivityType = "other_income"

	// Return of capital (return-bearing cash credit that also reduces basis).
	ActivityReturnOfCapital ActivityType = "return_of_capital"
	// Stock dividend (neutral quantity transformation; no cash).
	ActivityStockDividend ActivityType = "stock_dividend"

	// Fees (return-bearing, negative).
	ActivityBuyFee        ActivityType = "buy_fee"
	ActivitySellFee       ActivityType = "sell_fee"
	ActivityManagementFee ActivityType = "management_fee"
	ActivityCustodyFee    ActivityType = "custody_fee"
	ActivityOtherFee      ActivityType = "other_fee"

	// Corporate actions (neutral transformations, except write_off which is
	// return-bearing negative).
	ActivityStockSplit   ActivityType = "stock_split"
	ActivityReverseSplit ActivityType = "reverse_split"
	ActivitySymbolChange ActivityType = "symbol_change"
	ActivityWriteOff     ActivityType = "write_off"
)

// PerformanceEffect classifies whether an activity's value change flows into the
// ranked index (a genuine return) or is neutralized (a rebalance/transformation
// of what the user already owns). It is decided by trusted domain logic from the
// activity type — never chosen by the API caller.
type PerformanceEffect string

const (
	// PerformanceEffectNeutral: the ranked index is identical immediately before
	// and after (deposits, buys/sells, splits, symbol changes).
	PerformanceEffectNeutral PerformanceEffect = "neutral"
	// PerformanceEffectReturn: the value change moves the ranked index (income
	// raises it, fees/write-offs lower it).
	PerformanceEffectReturn PerformanceEffect = "return"
)

// Provenance records where an income/fee/corporate-action record came from. The
// initial product only supports user_reported; the others are extension points
// for a future verified provider.
type Provenance string

const (
	ProvenanceUserReported       Provenance = "user_reported"
	ProvenanceProviderReported   Provenance = "provider_reported"
	ProvenanceSystemGenerated    Provenance = "system_generated"
	ProvenanceMigrationGenerated Provenance = "migration_generated"
)

// IncomeSubtype names the kind of income being recorded. Every subtype is
// created by the automatic provider-driven pipeline (or a restricted correction
// path); users never select one directly.
type IncomeSubtype string

const (
	IncomeCashDividend       IncomeSubtype = "cash_dividend"
	IncomeSpecialDividend    IncomeSubtype = "special_dividend"
	IncomeETFDistribution    IncomeSubtype = "etf_distribution"
	IncomeMutualFundDist     IncomeSubtype = "mutual_fund_distribution"
	IncomeCapitalGainsDist   IncomeSubtype = "capital_gains_distribution"
	IncomeBondCoupon         IncomeSubtype = "bond_coupon"
	IncomeFixedIncomeInt     IncomeSubtype = "fixed_income_interest"
	IncomeInterest           IncomeSubtype = "interest_income"
	IncomeCashInterest       IncomeSubtype = "cash_interest"
	IncomeStakingReward      IncomeSubtype = "staking_reward"
	IncomePaymentInLieu      IncomeSubtype = "payment_in_lieu"
	IncomeOtherProvider      IncomeSubtype = "other_provider_income"
	IncomeReinvestedDiv      IncomeSubtype = "reinvested_dividend"
	IncomeReturnOfCapitalSub IncomeSubtype = "return_of_capital"
	IncomeStockDividendSub   IncomeSubtype = "stock_dividend"
)

// FeeSubtype names the kind of fee being recorded.
type FeeSubtype string

const (
	FeeManagement FeeSubtype = "management_fee"
	FeeCustody    FeeSubtype = "custody_fee"
	FeeOther      FeeSubtype = "other_fee"
)

// CorpActionSubtype names the kind of corporate action being recorded.
type CorpActionSubtype string

const (
	CorpStockSplit   CorpActionSubtype = "stock_split"
	CorpReverseSplit CorpActionSubtype = "reverse_split"
	CorpSymbolChange CorpActionSubtype = "symbol_change"
	CorpWriteOff     CorpActionSubtype = "write_off"
)

// IncomeInput is a normalized income event ready to be applied to a portfolio.
// It is produced by the automatic income pipeline (or a restricted correction
// path), never by an arbitrary user request.
//
// Amounts are separated so the ledger stores gross, withholding, fees and net
// distinctly rather than one ambiguous figure:
//
//	net cash credited = Amount(gross) - Withholding - Fee
//
// The performance-relevant value change is the NET cash credited.
type IncomeInput struct {
	Subtype   IncomeSubtype
	Symbol    string // optional for cash interest; identifies the paying instrument otherwise
	AssetType string // optional; used when a reinvestment must create a new position
	Currency  string

	// Amount is the gross income in Currency (eligible quantity × amount per
	// unit). Withholding and Fee are account-specific deductions; net cash is
	// derived from all three.
	Amount      money.Amount
	Withholding money.Amount
	Fee         money.Amount

	// Estimated marks the gross amount as a provider expectation rather than a
	// confirmed broker receipt (surfaced to the user, correctable later).
	Estimated bool

	// Reinvestment estimation (only for reinvested dividends): when ReinvestPrice
	// is > 0 it is used as the execution price; otherwise the current tracked
	// market price is used. PriceMethod records which pricing rule was applied.
	ReinvestPrice money.Price
	PriceMethod   string

	// Stock-dividend ratio: new_qty = qty × (1 + Num/Den). Only used by the
	// stock-dividend plan.
	StockRatioNum money.Ratio
	StockRatioDen money.Ratio

	// IncomeEventID links the ledger activity back to the normalized provider
	// event for audit and reconciliation. TaxClassification is passed through for
	// attribution (never presented as tax advice).
	IncomeEventID     string
	TaxClassification string

	OccurredAt *time.Time
	Provenance Provenance
}

// NetCash returns the net cash credited by an ordinary/return income event.
func (in IncomeInput) NetCash() money.Amount { return in.Amount.Sub(in.Withholding).Sub(in.Fee) }

// FeeInput is a user-reported fee. Amount is a positive amount in Currency and
// is deducted from that cash balance. LinkedActivityID optionally groups the fee
// under a buy/sell/dividend/corporate-action it belongs to.
type FeeInput struct {
	Subtype          FeeSubtype
	Currency         string
	Amount           money.Amount
	Symbol           string // optional, informational
	LinkedActivityID string // optional grouping
	Description      string
	OccurredAt       *time.Time
	Provenance       Provenance
}

// CorpActionInput is a user-reported corporate action. Only the fields relevant
// to Subtype are used; validation enforces the required ones per subtype.
type CorpActionInput struct {
	Subtype CorpActionSubtype
	Symbol  string // affected / source symbol

	// Split / reverse split: new_qty = qty * numerator/denominator.
	RatioNumerator   float64
	RatioDenominator float64

	// Symbol change.
	NewSymbol string

	OccurredAt *time.Time
	Provenance Provenance
}

// activityMeta merges the base tracking metadata with activity-specific
// provenance/grouping so the immutable ledger stays auditable.
func activityMeta(base map[string]any, effect PerformanceEffect, prov Provenance, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+4+len(extra))
	for k, v := range base {
		out[k] = v
	}
	out["performance_effect"] = string(effect)
	if prov == "" {
		prov = ProvenanceUserReported
	}
	out["provenance"] = string(prov)
	for k, v := range extra {
		if v == nil {
			continue
		}
		out[k] = v
	}
	return out
}

// mutationEffect maps a mutation kind to its performance effect. This is the
// single trusted classification: callers cannot select it, and it is what the
// coordinator uses to decide whether to neutralize the ranked index.
func mutationEffect(kind MutationKind) PerformanceEffect {
	switch kind {
	case MutationIncome, MutationFee, MutationWriteOff, MutationReinvestedDividend,
		MutationReturnOfCapital:
		// Return of capital credits cash (a genuine value increase whose
		// counterpart is the security's ex-date price drop), so it is return-
		// bearing exactly once — but, unlike an ordinary dividend, it also reduces
		// the position's cost basis (handled in planReturnOfCapital). See
		// backend/README.md for the documented accounting policy.
		return PerformanceEffectReturn
	default:
		// add/resize/delete/close/replace/deposit/withdrawal/buy/sell/split/
		// reverse_split/symbol_change/stock_dividend are all neutral: a stock
		// dividend is a quantity transformation that preserves total basis and
		// value, so it must never create ranked return.
		return PerformanceEffectNeutral
	}
}
