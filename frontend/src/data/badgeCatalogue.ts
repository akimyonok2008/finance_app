// Static catalogue of the 20 predetermined benchmark achievements.
//
// This mirrors the backend badge definitions (internal/benchmark.Badges) and
// supplies the richer display metadata the API response does not carry: the
// benchmark recipe string, the human unlock rule, the required edge, the
// category, and a plain-language explanation for the detail view.
//
// Keep `id` in sync with the backend badge keys — status/evidence is merged onto
// these entries by matching id.

export type BadgeDifficulty = "easy" | "medium" | "hard" | "elite";
export type BadgePeriod = "30D" | "90D" | "6M" | "1Y";
export type BadgeCategory = "investor" | "strategy";

export type BadgeCatalogueEntry = {
  id: string;
  name: string;
  difficulty: BadgeDifficulty;
  period: BadgePeriod;
  inspiredBy: string;
  benchmark: string;
  unlockRule: string;
  /** Outperformance in index points the rule requires (0 = beat with a positive return). */
  requiredEdgePoints: number;
  category: BadgeCategory;
  /** Plain-language meaning shown in the detail view. */
  explanation: string;
};

export const DIFFICULTY_ORDER: BadgeDifficulty[] = [
  "easy",
  "medium",
  "hard",
  "elite",
];

export const badgeCatalogue: BadgeCatalogueEntry[] = [
  {
    id: "cash_plus_30d",
    name: "Cash Plus",
    difficulty: "easy",
    period: "30D",
    inspiredBy: "Cash / T-Bill Proxy",
    benchmark: "100% SGOV",
    unlockRule: "Beat cash with a positive return",
    requiredEdgePoints: 0,
    category: "strategy",
    explanation:
      "Your portfolio grew and stayed ahead of a risk-free cash benchmark over 30 days. It is the first proof that taking market risk paid off versus simply holding T-bills.",
  },
  {
    id: "first_market_edge_30d",
    name: "First Market Edge",
    difficulty: "easy",
    period: "30D",
    inspiredBy: "S&P 500",
    benchmark: "100% SPY",
    unlockRule: "Beat SPY with a positive return",
    requiredEdgePoints: 0,
    category: "strategy",
    explanation:
      "Your portfolio outperformed the broad US market over 30 days with a positive return — an early sign your allocation added value over the index.",
  },
  {
    id: "gold_check_30d",
    name: "Gold Check",
    difficulty: "easy",
    period: "30D",
    inspiredBy: "Gold",
    benchmark: "100% GLD",
    unlockRule: "Beat gold with a positive return",
    requiredEdgePoints: 0,
    category: "strategy",
    explanation:
      "Your portfolio beat gold over 30 days while staying positive — evidence your strategy kept pace with a classic store-of-value hedge.",
  },
  {
    id: "balanced_start_30d",
    name: "Balanced Start",
    difficulty: "easy",
    period: "30D",
    inspiredBy: "60/40 Portfolio",
    benchmark: "60% SPY / 40% BND",
    unlockRule: "Beat a 60/40 portfolio with a positive return",
    requiredEdgePoints: 0,
    category: "strategy",
    explanation:
      "Your portfolio outperformed the textbook 60/40 stocks-and-bonds mix over 30 days with a positive return.",
  },
  {
    id: "bogle_badge_90d",
    name: "Bogle Badge",
    difficulty: "medium",
    period: "90D",
    inspiredBy: "Jack Bogle",
    benchmark: "100% VOO or SPY",
    unlockRule: "Beat the S&P 500 by +2 index points",
    requiredEdgePoints: 2,
    category: "strategy",
    explanation:
      "Your portfolio outperformed a broad passive index benchmark by a clear margin over 90 days. Inspired by Jack Bogle's index-investing philosophy — the bar every active strategy is measured against.",
  },
  {
    id: "global_allocator_90d",
    name: "Global Allocator Badge",
    difficulty: "medium",
    period: "90D",
    inspiredBy: "Global Equities",
    benchmark: "100% VT",
    unlockRule: "Beat global equities by +2 index points",
    requiredEdgePoints: 2,
    category: "strategy",
    explanation:
      "Your portfolio beat a total world equity benchmark by a clear margin over 90 days — outperformance measured against global, not just US, markets.",
  },
  {
    id: "dividend_challenger_90d",
    name: "Dividend Challenger",
    difficulty: "medium",
    period: "90D",
    inspiredBy: "Dividend Strategy",
    benchmark: "100% SCHD",
    unlockRule: "Beat the dividend benchmark by +2 index points",
    requiredEdgePoints: 2,
    category: "strategy",
    explanation:
      "Your portfolio outperformed a quality-dividend benchmark by a clear margin over 90 days.",
  },
  {
    id: "balanced_beater_90d",
    name: "Balanced Beater",
    difficulty: "medium",
    period: "90D",
    inspiredBy: "60/40 Portfolio",
    benchmark: "60% SPY / 40% BND",
    unlockRule: "Beat a 60/40 portfolio by +2 index points",
    requiredEdgePoints: 2,
    category: "strategy",
    explanation:
      "Your portfolio outperformed the 60/40 stocks-and-bonds benchmark by a clear margin over 90 days.",
  },
  {
    id: "inflation_shield_90d",
    name: "Inflation Shield",
    difficulty: "medium",
    period: "90D",
    inspiredBy: "Inflation Defense Basket",
    benchmark: "50% GLD / 50% SGOV",
    unlockRule: "Beat the inflation-defense basket by +2 index points",
    requiredEdgePoints: 2,
    category: "strategy",
    explanation:
      "Your portfolio outperformed a defensive gold-and-cash basket by a clear margin over 90 days.",
  },
  {
    id: "commodity_edge_90d",
    name: "Commodity Edge",
    difficulty: "medium",
    period: "90D",
    inspiredBy: "Commodity Basket",
    benchmark: "25% GLD / 25% SIVR / 25% XLE / 25% URA",
    unlockRule: "Beat the commodity basket by +2 index points",
    requiredEdgePoints: 2,
    category: "strategy",
    explanation:
      "Your portfolio outperformed a diversified commodity basket by a clear margin over 90 days.",
  },
  {
    id: "oracle_badge_6m",
    name: "Oracle Badge",
    difficulty: "hard",
    period: "6M",
    inspiredBy: "Warren Buffett",
    benchmark: "100% BRK.B",
    unlockRule: "Beat BRK.B by +3 index points",
    requiredEdgePoints: 3,
    category: "investor",
    explanation:
      "Your portfolio outperformed Berkshire Hathaway shares by a solid margin over six months. Inspired by Warren Buffett's Berkshire Hathaway approach.",
  },
  {
    id: "buffett_portfolio_6m",
    name: "Buffett Portfolio Badge",
    difficulty: "hard",
    period: "6M",
    inspiredBy: "Warren Buffett",
    benchmark: "Latest Berkshire 13F-style equity basket",
    unlockRule: "Beat the Berkshire 13F-style basket by +3 index points",
    requiredEdgePoints: 3,
    category: "investor",
    explanation:
      "Your portfolio outperformed a basket modeled on Berkshire's disclosed public equity holdings over six months. Inspired by Warren Buffett's disclosed investing approach.",
  },
  {
    id: "all_weather_6m",
    name: "All-Weather Badge",
    difficulty: "hard",
    period: "6M",
    inspiredBy: "Ray Dalio",
    benchmark: "30% SPY / 40% TLT / 15% GLD / 7.5% commodities / 7.5% SGOV",
    unlockRule: "Beat the all-weather benchmark by +3 index points",
    requiredEdgePoints: 3,
    category: "investor",
    explanation:
      "Your portfolio outperformed a diversified, macro-resilient allocation by a solid margin over six months. Inspired by Ray Dalio's all-weather approach.",
  },
  {
    id: "munger_6m",
    name: "Munger Badge",
    difficulty: "hard",
    period: "6M",
    inspiredBy: "Charlie Munger",
    benchmark: "40% QUAL / 30% BRK.B / 20% VOO / 10% SGOV",
    unlockRule: "Beat the quality-compounder benchmark by +3 index points",
    requiredEdgePoints: 3,
    category: "investor",
    explanation:
      "Your portfolio outperformed a quality-compounder benchmark by a solid margin over six months. Inspired by Charlie Munger's focus on high-quality businesses.",
  },
  {
    id: "graham_6m",
    name: "Graham Badge",
    difficulty: "hard",
    period: "6M",
    inspiredBy: "Benjamin Graham",
    benchmark: "40% VTV / 30% SCHD / 20% SGOV / 10% GLD",
    unlockRule: "Beat the defensive-value benchmark by +3 index points",
    requiredEdgePoints: 3,
    category: "investor",
    explanation:
      "Your portfolio outperformed a defensive value-and-income benchmark by a solid margin over six months. Inspired by Benjamin Graham's defensive-value principles.",
  },
  {
    id: "lynch_6m",
    name: "Lynch Badge",
    difficulty: "hard",
    period: "6M",
    inspiredBy: "Peter Lynch",
    benchmark: "40% VOO / 30% IJR / 20% QQQ / 10% SGOV",
    unlockRule: "Beat the Lynch-style growth benchmark by +4 index points",
    requiredEdgePoints: 4,
    category: "investor",
    explanation:
      "Your portfolio outperformed a growth-and-small-cap benchmark by a demanding margin over six months. Inspired by Peter Lynch's growth-oriented approach.",
  },
  {
    id: "swensen_6m",
    name: "Swensen Badge",
    difficulty: "hard",
    period: "6M",
    inspiredBy: "David Swensen",
    benchmark: "30% VOO / 20% VXUS / 20% BND / 15% VNQ / 10% GLD / 5% SGOV",
    unlockRule: "Beat the endowment benchmark by +3 index points",
    requiredEdgePoints: 3,
    category: "investor",
    explanation:
      "Your portfolio outperformed a diversified endowment-style allocation by a solid margin over six months. Inspired by David Swensen's institutional endowment approach.",
  },
  {
    id: "soros_1y",
    name: "Soros Badge",
    difficulty: "elite",
    period: "1Y",
    inspiredBy: "George Soros",
    benchmark: "30% SPY / 20% TLT / 20% GLD / 15% UUP / 15% commodities",
    unlockRule: "Beat the global macro benchmark by +5 index points",
    requiredEdgePoints: 5,
    category: "investor",
    explanation:
      "Your portfolio outperformed a global-macro benchmark by a wide margin across a full year. Inspired by George Soros's global-macro approach.",
  },
  {
    id: "quant_1y",
    name: "Quant Badge",
    difficulty: "elite",
    period: "1Y",
    inspiredBy: "Jim Simons",
    benchmark: "35% SPY / 25% QQQ / 20% SGOV / 10% GLD / 10% USMV",
    unlockRule: "Beat the quant-style benchmark by +5 index points",
    requiredEdgePoints: 5,
    category: "investor",
    explanation:
      "Your portfolio outperformed a systematic, diversified benchmark by a wide margin across a full year. Inspired by Jim Simons's quantitative approach.",
  },
  {
    id: "druckenmiller_1y",
    name: "Druckenmiller Badge",
    difficulty: "elite",
    period: "1Y",
    inspiredBy: "Stanley Druckenmiller",
    benchmark: "35% QQQ / 25% SPY / 15% GLD / 10% TLT / 10% XLE / 5% SGOV",
    unlockRule: "Beat the macro-growth benchmark by +5 index points",
    requiredEdgePoints: 5,
    category: "investor",
    explanation:
      "Your portfolio outperformed a flexible macro-growth benchmark by a wide margin across a full year. Inspired by Stanley Druckenmiller's macro-growth approach.",
  },
];

export const DIFFICULTY_LABEL: Record<BadgeDifficulty, string> = {
  easy: "Easy",
  medium: "Medium",
  hard: "Hard",
  elite: "Elite",
};

// Short "why it matters" note derived from difficulty, shown in the detail view.
export const DIFFICULTY_PRESTIGE: Record<BadgeDifficulty, string> = {
  easy: "A starter badge — a first proof your strategy can beat a simple benchmark.",
  medium:
    "A meaningful edge over a widely-followed benchmark, sustained across a quarter.",
  hard: "A demanding, sustained edge over a serious investor or strategy benchmark.",
  elite:
    "A rare, elite badge — a full year of outperformance over a sophisticated benchmark.",
};
