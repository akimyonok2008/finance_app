package dna

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// minFocusContribution is the minimum aggregate weighted contribution (in
	// percentage points) a tag needs before it is surfaced as a focus area.
	minFocusContribution = 8.0
	maxFocusAreas        = 5
)

// focusAreas aggregates instrument focus tags weighted by position size and
// returns the top tags (3-5) that clear the minimum contribution threshold.
func focusAreas(items []classifiedPosition) []string {
	scores := map[string]float64{}
	for _, item := range items {
		for _, tag := range item.factors.FocusTags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			scores[tag] += item.pos.Weight * 100
		}
	}
	return topTags(scores, minFocusContribution, maxFocusAreas)
}

type scoredTag struct {
	name  string
	score float64
}

func topTags(scores map[string]float64, minScore float64, limit int) []string {
	tags := make([]scoredTag, 0, len(scores))
	for name, score := range scores {
		if score < minScore {
			continue
		}
		tags = append(tags, scoredTag{name: name, score: score})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].score == tags[j].score {
			return tags[i].name < tags[j].name
		}
		return tags[i].score > tags[j].score
	})
	out := make([]string, 0, limit)
	for i := 0; i < len(tags) && i < limit; i++ {
		out = append(out, tags[i].name)
	}
	return out
}

// buildExplanations produces short, human-readable strings describing what drove
// each dimension's score.
func buildExplanations(items []classifiedPosition, scores PortfolioDNAScores, con concentrationInfo, positionCount int) DNAExplanations {
	ex := DNAExplanations{
		Growth:        exposureExplanation("Growth", scores.Growth, items, func(f InstrumentDNAFactors) float64 { return f.Growth }),
		Income:        exposureExplanation("Income", scores.Income, items, func(f InstrumentDNAFactors) float64 { return f.Income }),
		Commodities:   exposureExplanation("Commodity", scores.Commodities, items, func(f InstrumentDNAFactors) float64 { return f.Commodities }),
		International: exposureExplanation("International", scores.International, items, func(f InstrumentDNAFactors) float64 { return f.International }),
		Defensive:     defensiveExplanation(scores, con, items),
		Concentration: concentrationExplanation(con),
		Volatility:    volatilityExplanation(scores, con, items),
	}
	if positionCount < 2 {
		note := "With a single position, Portfolio DNA may change significantly as the portfolio becomes more diversified."
		ex.Growth = append(ex.Growth, note)
	}
	return ex
}

// topContributors returns the symbols with the largest weight*factor
// contribution, up to limit, ignoring zero contributions.
func topContributors(items []classifiedPosition, factor func(InstrumentDNAFactors) float64, limit int) []string {
	type contribution struct {
		symbol string
		points float64
	}
	contribs := make([]contribution, 0, len(items))
	for _, item := range items {
		points := item.pos.Weight * factor(item.factors)
		if points <= 0 {
			continue
		}
		contribs = append(contribs, contribution{symbol: item.pos.Symbol, points: points})
	}
	sort.Slice(contribs, func(i, j int) bool {
		if contribs[i].points == contribs[j].points {
			return contribs[i].symbol < contribs[j].symbol
		}
		return contribs[i].points > contribs[j].points
	})
	out := make([]string, 0, limit)
	for i := 0; i < len(contribs) && i < limit; i++ {
		out = append(out, contribs[i].symbol)
	}
	return out
}

func exposureExplanation(label string, score int, items []classifiedPosition, factor func(InstrumentDNAFactors) float64) []string {
	drivers := topContributors(items, factor, 3)
	if len(drivers) == 0 {
		return []string{fmt.Sprintf("%s exposure is minimal across the current positions.", label)}
	}
	return []string{fmt.Sprintf("%s score (%d) is mainly driven by %s.", label, score, joinSymbols(drivers))}
}

func defensiveExplanation(scores PortfolioDNAScores, con concentrationInfo, items []classifiedPosition) []string {
	drivers := topContributors(items, func(f InstrumentDNAFactors) float64 { return f.Defensive }, 3)
	out := []string{}
	if len(drivers) == 0 {
		out = append(out, fmt.Sprintf("Defensive score is %d; the portfolio holds little defensive exposure.", scores.Defensive))
	} else {
		out = append(out, fmt.Sprintf("Defensive score (%d) comes mainly from %s.", scores.Defensive, joinSymbols(drivers)))
	}
	if con.score >= 50 {
		out = append(out, "Defensiveness is reduced because the portfolio is concentrated.")
	}
	if scores.Volatility >= 60 {
		out = append(out, "Elevated volatility also weakens the defensive profile.")
	}
	return out
}

func concentrationExplanation(con concentrationInfo) []string {
	out := []string{
		fmt.Sprintf("The top 3 positions represent %d%% of the portfolio.", pct(con.top3)),
	}
	if con.positionCount > 0 {
		out = append(out, fmt.Sprintf("The largest position is %d%% of the portfolio across %d holdings.", pct(con.top1), con.positionCount))
	}
	return out
}

func volatilityExplanation(scores PortfolioDNAScores, con concentrationInfo, items []classifiedPosition) []string {
	drivers := topContributors(items, func(f InstrumentDNAFactors) float64 { return f.Volatility }, 3)
	band := "low"
	switch {
	case scores.Volatility >= 76:
		band = "high"
	case scores.Volatility >= 51:
		band = "elevated"
	case scores.Volatility >= 26:
		band = "moderate"
	}
	if len(drivers) == 0 {
		return []string{fmt.Sprintf("Volatility is %s (%d).", band, scores.Volatility)}
	}
	msg := fmt.Sprintf("Volatility is %s (%d), driven mainly by %s.", band, scores.Volatility, joinSymbols(drivers))
	out := []string{msg}
	if con.score >= 50 {
		out = append(out, "Concentration adds to the overall volatility.")
	}
	return out
}

func joinSymbols(symbols []string) string {
	switch len(symbols) {
	case 0:
		return ""
	case 1:
		return symbols[0]
	case 2:
		return symbols[0] + " and " + symbols[1]
	default:
		return strings.Join(symbols[:len(symbols)-1], ", ") + ", and " + symbols[len(symbols)-1]
	}
}

// pct converts a decimal weight (0..1) to a rounded whole percentage.
func pct(weight float64) int {
	return clampInt(weight * 100)
}
