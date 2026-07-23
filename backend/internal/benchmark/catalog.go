package benchmark

// Recipes is the catalogue of benchmark allocations, keyed by recipe id. Static
// recipes are resolved directly; dynamic ones (BUFFETT_13F) go through a
// DynamicRecipeResolver.
var Recipes = map[string]BenchmarkRecipe{
	"CASH": {
		ID: "CASH", Name: "Cash / T-Bill Proxy",
		Description: "Short-term Treasury bill benchmark.",
		Components:  []AssetAllocation{{Symbol: "SGOV", Weight: 1.0}},
	},
	"SPY": {
		ID: "SPY", Name: "S&P 500",
		Description: "Broad US equity benchmark.",
		Components:  []AssetAllocation{{Symbol: "SPY", Weight: 1.0}},
	},
	"VOO": {
		ID: "VOO", Name: "Bogle Broad Index",
		Description: "Broad US passive index benchmark.",
		Components:  []AssetAllocation{{Symbol: "VOO", Weight: 1.0}},
	},
	"GOLD": {
		ID: "GOLD", Name: "Gold",
		Description: "Gold benchmark.",
		Components:  []AssetAllocation{{Symbol: "GLD", Weight: 1.0}},
	},
	"BALANCED_60_40": {
		ID: "BALANCED_60_40", Name: "60/40 Portfolio",
		Description: "60% equities, 40% bonds.",
		Components: []AssetAllocation{
			{Symbol: "SPY", Weight: 0.6},
			{Symbol: "BND", Weight: 0.4},
		},
	},
	"GLOBAL_EQUITY": {
		ID: "GLOBAL_EQUITY", Name: "Global Equity",
		Description: "Global stock market benchmark.",
		Components:  []AssetAllocation{{Symbol: "VT", Weight: 1.0}},
	},
	"DIVIDEND": {
		ID: "DIVIDEND", Name: "Dividend Strategy",
		Description: "Dividend equity benchmark.",
		Components:  []AssetAllocation{{Symbol: "SCHD", Weight: 1.0}},
	},
	"INFLATION_DEFENSE": {
		ID: "INFLATION_DEFENSE", Name: "Inflation Defense",
		Description: "Gold plus cash defensive basket.",
		Components: []AssetAllocation{
			{Symbol: "GLD", Weight: 0.5},
			{Symbol: "SGOV", Weight: 0.5},
		},
	},
	"CMDTY": {
		ID: "CMDTY", Name: "Commodity Basket",
		Description: "Generic commodity basket used inside other benchmarks.",
		Components: []AssetAllocation{
			{Symbol: "GLD", Weight: 0.25},
			{Symbol: "SIVR", Weight: 0.25},
			{Symbol: "XLE", Weight: 0.25},
			{Symbol: "URA", Weight: 0.25},
		},
	},
	"ORACLE_BERKSHIRE": {
		ID: "ORACLE_BERKSHIRE", Name: "Berkshire Hathaway",
		Description: "Berkshire Hathaway shareholder return benchmark.",
		Components:  []AssetAllocation{{Symbol: "BRK.B", Weight: 1.0}},
	},
	"BUFFETT_13F": {
		ID: "BUFFETT_13F", Name: "Berkshire 13F Equity Basket",
		Description: "Dynamic benchmark based on Berkshire's disclosed equity portfolio.",
		Components:  []AssetAllocation{},
		Dynamic:     true,
	},
	"ALL_WEATHER": {
		ID: "ALL_WEATHER", Name: "All-Weather Style Benchmark",
		Description: "Diversified macro-resilient allocation.",
		Components: []AssetAllocation{
			{Symbol: "SPY", Weight: 0.3},
			{Symbol: "TLT", Weight: 0.4},
			{Symbol: "GLD", Weight: 0.15},
			{RecipeRef: "CMDTY", Weight: 0.075},
			{Symbol: "SGOV", Weight: 0.075},
		},
	},
	"MUNGER_QUALITY": {
		ID: "MUNGER_QUALITY", Name: "Munger Quality Compounder Benchmark",
		Description: "Quality-compounder inspired benchmark.",
		Components: []AssetAllocation{
			{Symbol: "QUAL", Weight: 0.4},
			{Symbol: "BRK.B", Weight: 0.3},
			{Symbol: "VOO", Weight: 0.2},
			{Symbol: "SGOV", Weight: 0.1},
		},
	},
	"GRAHAM_DEFENSIVE_VALUE": {
		ID: "GRAHAM_DEFENSIVE_VALUE", Name: "Graham Defensive Value Benchmark",
		Description: "Defensive value and income benchmark.",
		Components: []AssetAllocation{
			{Symbol: "VTV", Weight: 0.4},
			{Symbol: "SCHD", Weight: 0.3},
			{Symbol: "SGOV", Weight: 0.2},
			{Symbol: "GLD", Weight: 0.1},
		},
	},
	"LYNCH_GROWTH": {
		ID: "LYNCH_GROWTH", Name: "Lynch-Style Growth Equity Benchmark",
		Description: "Growth equity and active stock-picking style benchmark.",
		Components: []AssetAllocation{
			{Symbol: "VOO", Weight: 0.4},
			{Symbol: "IJR", Weight: 0.3},
			{Symbol: "QQQ", Weight: 0.2},
			{Symbol: "SGOV", Weight: 0.1},
		},
	},
	"SWENSEN_ENDOWMENT": {
		ID: "SWENSEN_ENDOWMENT", Name: "Swensen Endowment Benchmark",
		Description: "Institutional endowment-style diversified allocation.",
		Components: []AssetAllocation{
			{Symbol: "VOO", Weight: 0.3},
			{Symbol: "VXUS", Weight: 0.2},
			{Symbol: "BND", Weight: 0.2},
			{Symbol: "VNQ", Weight: 0.15},
			{Symbol: "GLD", Weight: 0.1},
			{Symbol: "SGOV", Weight: 0.05},
		},
	},
	"SOROS_MACRO": {
		ID: "SOROS_MACRO", Name: "Soros Global Macro Benchmark",
		Description: "Global macro-style benchmark.",
		Components: []AssetAllocation{
			{Symbol: "SPY", Weight: 0.3},
			{Symbol: "TLT", Weight: 0.2},
			{Symbol: "GLD", Weight: 0.2},
			{Symbol: "UUP", Weight: 0.15},
			{RecipeRef: "CMDTY", Weight: 0.15},
		},
	},
	"QUANT_STYLE": {
		ID: "QUANT_STYLE", Name: "Quant-Style Diversified Benchmark",
		Description: "Systematic diversified benchmark.",
		Components: []AssetAllocation{
			{Symbol: "SPY", Weight: 0.35},
			{Symbol: "QQQ", Weight: 0.25},
			{Symbol: "SGOV", Weight: 0.2},
			{Symbol: "GLD", Weight: 0.1},
			{Symbol: "USMV", Weight: 0.1},
		},
	},
	"DRUCKENMILLER_MACRO_GROWTH": {
		ID: "DRUCKENMILLER_MACRO_GROWTH", Name: "Druckenmiller Macro-Growth Benchmark",
		Description: "Flexible macro-growth inspired benchmark.",
		Components: []AssetAllocation{
			{Symbol: "QQQ", Weight: 0.35},
			{Symbol: "SPY", Weight: 0.25},
			{Symbol: "GLD", Weight: 0.15},
			{Symbol: "TLT", Weight: 0.1},
			{Symbol: "XLE", Weight: 0.1},
			{Symbol: "SGOV", Weight: 0.05},
		},
	},
}

// beatAndPositive is a small helper for the common rule shape.
func beatAndPositive(edge float64) UnlockRule {
	return UnlockRule{Kind: RuleBeatByPointsAndPositiv, RequiredEdgePoints: edge, RequiresPositiveReturn: true}
}

func positiveAndBeat() UnlockRule {
	return UnlockRule{Kind: RulePositiveAndBeat, RequiredEdgePoints: 0, RequiresPositiveReturn: true}
}

// Badges is the ordered badge catalogue.
var Badges = []Badge{
	{
		ID: "cash_plus_30d", Name: "Cash Plus", Difficulty: DifficultyEasy, Period: Period30D,
		Description: "Beat cash with a positive return.", RecipeID: "CASH", Rule: positiveAndBeat(),
	},
	{
		ID: "first_market_edge_30d", Name: "First Market Edge", Difficulty: DifficultyEasy, Period: Period30D,
		Description: "Beat SPY with a positive return.", RecipeID: "SPY", Rule: positiveAndBeat(),
	},
	{
		ID: "gold_check_30d", Name: "Gold Check", Difficulty: DifficultyEasy, Period: Period30D,
		Description: "Beat gold with a positive return.", RecipeID: "GOLD", Rule: positiveAndBeat(),
	},
	{
		ID: "balanced_start_30d", Name: "Balanced Start", Difficulty: DifficultyEasy, Period: Period30D,
		Description: "Beat a 60/40 portfolio with a positive return.", RecipeID: "BALANCED_60_40", Rule: positiveAndBeat(),
	},
	{
		ID: "bogle_badge_90d", Name: "Bogle Badge", Difficulty: DifficultyMedium, Period: Period90D,
		Description: "Beat the broad index benchmark by +2 points.", InspiredBy: "Jack Bogle",
		RecipeID: "VOO", Rule: beatAndPositive(2),
	},
	{
		ID: "global_allocator_90d", Name: "Global Allocator Badge", Difficulty: DifficultyMedium, Period: Period90D,
		Description: "Beat global equities by +2 points.", RecipeID: "GLOBAL_EQUITY", Rule: beatAndPositive(2),
	},
	{
		ID: "dividend_challenger_90d", Name: "Dividend Challenger", Difficulty: DifficultyMedium, Period: Period90D,
		Description: "Beat the dividend benchmark by +2 points.", RecipeID: "DIVIDEND", Rule: beatAndPositive(2),
	},
	{
		ID: "balanced_beater_90d", Name: "Balanced Beater", Difficulty: DifficultyMedium, Period: Period90D,
		Description: "Beat the 60/40 benchmark by +2 points.", RecipeID: "BALANCED_60_40", Rule: beatAndPositive(2),
	},
	{
		ID: "inflation_shield_90d", Name: "Inflation Shield", Difficulty: DifficultyMedium, Period: Period90D,
		Description: "Beat the inflation-defense basket by +2 points.", RecipeID: "INFLATION_DEFENSE", Rule: beatAndPositive(2),
	},
	{
		ID: "commodity_edge_90d", Name: "Commodity Edge", Difficulty: DifficultyMedium, Period: Period90D,
		Description: "Beat the commodity basket by +2 points.", RecipeID: "CMDTY", Rule: beatAndPositive(2),
	},
	{
		ID: "oracle_badge_6m", Name: "Oracle Badge", Difficulty: DifficultyHard, Period: Period6M,
		Description: "Beat Berkshire Hathaway by +3 points.", InspiredBy: "Warren Buffett",
		RecipeID: "ORACLE_BERKSHIRE", Rule: beatAndPositive(3),
	},
	{
		ID: "buffett_portfolio_6m", Name: "Buffett Portfolio Badge", Difficulty: DifficultyHard, Period: Period6M,
		Description: "Beat Berkshire's disclosed public equity basket by +3 points.", InspiredBy: "Warren Buffett",
		RecipeID: "BUFFETT_13F", Rule: beatAndPositive(3),
	},
	{
		ID: "all_weather_6m", Name: "All-Weather Badge", Difficulty: DifficultyHard, Period: Period6M,
		Description: "Beat the All-Weather benchmark by +3 points.", InspiredBy: "Ray Dalio",
		RecipeID: "ALL_WEATHER", Rule: beatAndPositive(3),
	},
	{
		ID: "munger_6m", Name: "Munger Badge", Difficulty: DifficultyHard, Period: Period6M,
		Description: "Beat the quality-compounder benchmark by +3 points.", InspiredBy: "Charlie Munger",
		RecipeID: "MUNGER_QUALITY", Rule: beatAndPositive(3),
	},
	{
		ID: "graham_6m", Name: "Graham Badge", Difficulty: DifficultyHard, Period: Period6M,
		Description: "Beat the defensive-value benchmark by +3 points.", InspiredBy: "Benjamin Graham",
		RecipeID: "GRAHAM_DEFENSIVE_VALUE", Rule: beatAndPositive(3),
	},
	{
		ID: "lynch_6m", Name: "Lynch Badge", Difficulty: DifficultyHard, Period: Period6M,
		Description: "Beat the Lynch-style growth benchmark by +4 points.", InspiredBy: "Peter Lynch",
		RecipeID: "LYNCH_GROWTH", Rule: beatAndPositive(4),
	},
	{
		ID: "swensen_6m", Name: "Swensen Badge", Difficulty: DifficultyHard, Period: Period6M,
		Description: "Beat the endowment benchmark by +3 points.", InspiredBy: "David Swensen",
		RecipeID: "SWENSEN_ENDOWMENT", Rule: beatAndPositive(3),
	},
	{
		ID: "soros_1y", Name: "Soros Badge", Difficulty: DifficultyElite, Period: Period1Y,
		Description: "Beat the global macro benchmark by +5 points.", InspiredBy: "George Soros",
		RecipeID: "SOROS_MACRO", Rule: beatAndPositive(5),
	},
	{
		ID: "quant_1y", Name: "Quant Badge", Difficulty: DifficultyElite, Period: Period1Y,
		Description: "Beat the quant-style benchmark by +5 points.", InspiredBy: "Jim Simons",
		RecipeID: "QUANT_STYLE", Rule: beatAndPositive(5),
	},
	{
		ID: "druckenmiller_1y", Name: "Druckenmiller Badge", Difficulty: DifficultyElite, Period: Period1Y,
		Description: "Beat the macro-growth benchmark by +5 points.", InspiredBy: "Stanley Druckenmiller",
		RecipeID: "DRUCKENMILLER_MACRO_GROWTH", Rule: beatAndPositive(5),
	},
}
