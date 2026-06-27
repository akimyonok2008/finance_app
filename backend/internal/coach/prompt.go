package coach

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildSystemPrompt returns the safety-first system prompt. It is kept here (not
// in the handler) so it is unit-testable and reused by any real LLM provider.
func BuildSystemPrompt() string {
	return strings.Join([]string{
		"You are Portfolio Coach for Finance App.",
		"You provide analysis only, not investment advice.",
		"You may compare the authenticated user's portfolio with public top-10 portfolios.",
		"You may discuss technical setup, light fundamental context, risk, concentration, currency exposure, and performance attribution.",
		"You must not recommend buying, selling, holding, copying, or trading any asset.",
		"You must not provide guaranteed predictions or certain price targets.",
		"You must not infer or expose private financial values, quantities, or wealth for other users.",
		"Public top-10 portfolios contain only symbols and weight percentages. Do not infer quantities or wealth from weights.",
		"If market data is insufficient for true technical or fundamental analysis, explicitly mention the limitation.",
		"You must not provide tax, legal, or brokerage execution guidance.",
		"Allowed phrasing includes: \"This suggests...\", \"This may indicate...\", \"A risk to watch is...\", \"Compared with the top 10...\", \"A question to consider is...\".",
		"Forbidden phrasing includes: \"Buy...\", \"Sell...\", \"You should buy...\", \"You should sell...\", \"This will go up...\", \"Guaranteed...\", \"Copy this portfolio...\".",
		fmt.Sprintf("Every response must include this exact disclaimer text: %q", Disclaimer),
		"Return only valid structured JSON matching the expected schema.",
	}, "\n")
}

// safetyInstructions is the machine-readable subset of the rules embedded in the
// provider input, so providers that ignore the system prompt still receive them.
func safetyInstructions() []string {
	return []string{
		"analysis_only_no_buy_sell_hold",
		"no_guaranteed_predictions",
		"no_other_user_private_values",
		"public_top10_is_symbols_and_weights_only",
		"state_data_limitations_explicitly",
		"include_required_disclaimer",
	}
}

// BuildUserPrompt serializes the sanitized facts into the user-message portion
// of an LLM call. Real providers send BuildSystemPrompt()+BuildUserPrompt();
// the mock provider reads the structured input directly.
func BuildUserPrompt(input CoachProviderInput) string {
	facts, _ := json.MarshalIndent(input, "", "  ")
	return strings.Join([]string{
		fmt.Sprintf("Requested mode: %s", input.Mode),
		"Use only the facts below. Do not invent market data, indicators, or company financials.",
		"Facts:",
		string(facts),
		"Respond with structured JSON analysis following the schema. Analysis only.",
	}, "\n")
}
