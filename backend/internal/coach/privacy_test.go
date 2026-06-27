package coach

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// assertNoForbiddenKeys checks that no forbidden field appears as a JSON key
// (matched in quoted-key form so "gain_loss_percentage" does not trip on the
// forbidden "gain_loss").
func assertNoForbiddenKeys(t *testing.T, label, body string) {
	t.Helper()
	for _, k := range ForbiddenPublicFields {
		assert.NotContainsf(t, body, `"`+k+`":`, "%s must not expose forbidden key %q", label, k)
	}
}

func threeUserService(t *testing.T) (*Service, []auth.User) {
	t.Helper()
	users := []auth.User{
		user("u1", "Alpha"),
		user("u2", "Bravo"),
		user("u3", "Charlie"),
	}
	sums := map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", []testPosition{
			{"AAPL", "stock", "USD", 600, 0.05},
			{"BTC-USD", "crypto", "TRY", 400, 0.03},
		}),
		"u2": summaryWith("u2", []testPosition{
			{"NVDA", "stock", "USD", 700, 0.25},
			{"AAPL", "stock", "USD", 300, 0.25},
		}),
		"u3": summaryWith("u3", []testPosition{
			{"SPY", "etf", "EUR", 1000, 0.18},
		}),
	}
	return newCoachService(users, sums), users
}

func TestPrivacy_PublicTop10ContextHasOnlySymbolsAndWeights(t *testing.T) {
	svc, _ := threeUserService(t)
	userFacts := buildUserFacts(svc.mustSummary(t, "u1"))

	top10 := svc.buildTop10Facts(context.Background(), "u1", userFacts)
	require.True(t, top10.Available)
	require.NotEmpty(t, top10.Portfolios)

	// Each public holding must carry exactly symbol, weight_percentage, asset_type.
	for _, p := range top10.Portfolios {
		for _, h := range p.Holdings {
			assert.NotEmpty(t, h.Symbol)
			assert.NotEmpty(t, h.AssetType)
		}
	}

	body := mustJSON(t, top10)
	assertNoForbiddenKeys(t, "top-10 context", body)
	// Allowed public keys are present...
	assert.Contains(t, body, `"weight_percentage":`)
	assert.Contains(t, body, `"symbol":`)
	// ...and no other user's email leaks.
	assert.NotContains(t, body, "u2@example.com")
	assert.NotContains(t, body, "u3@example.com")
}

func TestPrivacy_ProviderInputHasNoForbiddenFields(t *testing.T) {
	svc, _ := threeUserService(t)
	userFacts := buildUserFacts(svc.mustSummary(t, "u1"))

	input := CoachProviderInput{
		Mode:               ModeCompareTop10,
		User:               userFacts,
		Top10:              svc.buildTop10Facts(context.Background(), "u1", userFacts),
		SafetyInstructions: safetyInstructions(),
	}
	body := mustJSON(t, input)
	assertNoForbiddenKeys(t, "provider input", body)

	// The prompt built from the input must also be clean.
	prompt := BuildUserPrompt(input)
	assertNoForbiddenKeys(t, "user prompt", prompt)
}

func TestPrivacy_APIResponseLeaksNoOtherUserPrivateData(t *testing.T) {
	svc, _ := threeUserService(t)

	for _, mode := range []string{ModeCompareTop10, ModeAnalyzePortfolio} {
		resp, err := svc.Analyze(context.Background(), "u1", mode)
		require.NoError(t, err)
		body := mustJSON(t, resp)

		assertNoForbiddenKeys(t, "API response ("+mode+")", body)
		assert.NotContains(t, body, "u2@example.com")
		assert.NotContains(t, body, "u3@example.com")
		assert.NotContains(t, body, "pf-u2", "must not leak another portfolio id")
		assert.Contains(t, body, Disclaimer)
	}
}

func TestPrivacy_SystemPromptContainsNoAdviceRules(t *testing.T) {
	sys := strings.ToLower(BuildSystemPrompt())
	assert.Contains(t, sys, "analysis only")
	assert.Contains(t, sys, "must not recommend buying, selling, holding")
	assert.Contains(t, sys, "must not provide guaranteed predictions")
	assert.Contains(t, sys, strings.ToLower(Disclaimer))
}

// --- small test helpers ------------------------------------------------------

func (s *Service) mustSummary(t *testing.T, userID string) *portfolio.PortfolioSummary {
	t.Helper()
	sum, err := s.summaries.Summary(context.Background(), userID)
	require.NoError(t, err)
	require.NotNil(t, sum)
	return sum
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
