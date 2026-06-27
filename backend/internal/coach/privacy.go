package coach

import "strings"

// ForbiddenPublicFields are JSON keys that must NEVER appear in the public
// top-10 context, the provider input, or the coach API response. They map to
// quantities, absolute money figures, and identity fields.
var ForbiddenPublicFields = []string{
	"quantity",
	"quantities",
	"average_buy_price",
	"avg_buy_price",
	"cost_basis",
	"current_value",
	"portfolio_value",
	"gain_loss", // note: "gain_loss_percentage" is allowed; tests match the quoted key form `"gain_loss":`
	"absolute_gain_loss",
	"user_id",
	"portfolio_id",
	"email",
	"brokerage",
	"starting_value",
	"starting_portfolio_value",
	"baseline_value",
	"password",
}

// forbiddenAdvicePhrases are directive phrasings the coach must never emit.
var forbiddenAdvicePhrases = []string{
	"you should buy",
	"you should sell",
	"buy now",
	"sell now",
	"guaranteed",
	"will go up",
	"will go down",
	"copy this portfolio",
	"this is financial advice",
	"price target",
}

// ContainsForbiddenAdvice reports whether text contains obvious advice/guarantee
// language. Used as a safety net over provider output and in tests.
func ContainsForbiddenAdvice(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range forbiddenAdvicePhrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// toPublicHolding projects a holding fact down to the only fields allowed in
// public competitive context: symbol, weight, asset type.
func toPublicHolding(h HoldingFact) PublicHolding {
	return PublicHolding{
		Symbol:           h.Symbol,
		WeightPercentage: h.WeightPercentage,
		AssetType:        h.AssetType,
	}
}
