package dna

import "strings"

// Classifier maps a normalized position to its instrument DNA factors. It is an
// interface so callers can inject richer instrument metadata later without
// touching the scoring math.
type Classifier interface {
	Classify(pos NormalizedPosition) InstrumentDNAFactors
}

// defaultClassifier resolves factors from a curated symbol map first, then falls
// back to deterministic rules based on asset type, sector, country, and
// currency. It never performs I/O.
type defaultClassifier struct {
	table map[string]InstrumentDNAFactors
}

func newDefaultClassifier() *defaultClassifier {
	return &defaultClassifier{table: instrumentDNAFactors}
}

// Classify returns the DNA factors for a position. Known symbols use the curated
// table; everything else is classified structurally.
func (c *defaultClassifier) Classify(pos NormalizedPosition) InstrumentDNAFactors {
	symbol := normalizeSymbol(pos.Symbol)
	if factors, ok := c.table[symbol]; ok {
		return factors
	}
	return c.fallback(pos)
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

// fallback classifies an unknown symbol from its structural metadata.
func (c *defaultClassifier) fallback(pos NormalizedPosition) InstrumentDNAFactors {
	assetType := strings.ToLower(strings.TrimSpace(pos.AssetType))
	sector := strings.ToLower(strings.TrimSpace(pos.Sector))
	symbol := normalizeSymbol(pos.Symbol)
	intl := inferInternational(pos)

	// Crypto is volatility-dominant and carries some commodity-like behavior.
	if assetType == "crypto" || looksLikeCrypto(symbol) {
		return InstrumentDNAFactors{
			Growth: 35, Income: 0, Commodities: 20, Defensive: 0,
			International: maxF(intl, 50), Volatility: 95,
			FocusTags: []string{"Crypto", "High Volatility"},
		}
	}

	// Sector / thematic overrides apply to both stocks and ETFs.
	switch {
	case sectorMatches(sector, "gold", "silver", "precious", "metal"):
		return InstrumentDNAFactors{
			Commodities: 100, Defensive: 40, Volatility: 70,
			International: intl, FocusTags: []string{"Precious Metals", "Commodities"},
		}
	case sectorMatches(sector, "uranium", "energy", "oil", "gas", "mining", "miner", "resource"):
		return InstrumentDNAFactors{
			Growth: 20, Income: 20, Commodities: 88, Defensive: 15, Volatility: 82,
			International: intl, FocusTags: []string{"Energy", "Commodities", "Cyclical"},
		}
	case sectorMatches(sector, "cash", "treasury", "money market", "t-bill", "bill"):
		return InstrumentDNAFactors{
			Income: 90, Defensive: 90, Volatility: 5,
			International: intl, FocusTags: []string{"Cash-Like", "Income", "Defensive"},
		}
	case sectorMatches(sector, "bond", "fixed income", "rates", "aggregate"):
		return InstrumentDNAFactors{
			Income: 65, Defensive: 50, Volatility: 45,
			International: intl, FocusTags: []string{"Bonds", "Income"},
		}
	case sectorMatches(sector, "dividend", "income"):
		return InstrumentDNAFactors{
			Growth: 25, Income: 80, Defensive: 60, Volatility: 35,
			International: intl, FocusTags: []string{"Dividend", "Income", "Defensive"},
		}
	case sectorMatches(sector, "technology", "tech", "semiconductor", "software", "ai"):
		return InstrumentDNAFactors{
			Growth: 80, Income: 5, Defensive: 15, Volatility: 70,
			International: intl, FocusTags: []string{"Technology", "Growth"},
		}
	case sectorMatches(sector, "utilities", "staples", "consumer defensive", "healthcare", "health"):
		return InstrumentDNAFactors{
			Growth: 30, Income: 40, Defensive: 65, Volatility: 30,
			International: intl, FocusTags: []string{"Defensive"},
		}
	}

	switch assetType {
	case "etf":
		tags := []string{"ETF"}
		if intl >= 50 {
			tags = []string{"Global", "ETF", "International"}
		}
		return InstrumentDNAFactors{
			Growth: 45, Income: 20, Defensive: 40, Volatility: 45,
			International: intl, FocusTags: tags,
		}
	default: // stock or unknown single security
		tags := []string{"Single Stock"}
		if intl >= 50 {
			tags = []string{"International", "Single Stock"}
		}
		return InstrumentDNAFactors{
			Growth: 45, Income: 10, Defensive: 20, Volatility: 65,
			International: intl, FocusTags: tags,
		}
	}
}

// inferInternational derives international exposure from the listing suffix,
// country, and currency. US-listed / USD holdings default to 0.
func inferInternational(pos NormalizedPosition) float64 {
	symbol := normalizeSymbol(pos.Symbol)
	if isNonUSListed(symbol) {
		return 100
	}
	country := strings.ToUpper(strings.TrimSpace(pos.Country))
	switch country {
	case "", "US", "USA", "U.S.", "UNITED STATES":
		// not international by country
	default:
		return 100
	}
	currency := strings.ToUpper(strings.TrimSpace(pos.Currency))
	if currency != "" && currency != "USD" {
		return 100
	}
	return 0
}

// isNonUSListed detects non-US listings by their exchange suffix (e.g. THYAO.IS).
func isNonUSListed(symbol string) bool {
	dot := strings.LastIndex(symbol, ".")
	if dot < 0 || dot == len(symbol)-1 {
		return false
	}
	suffix := symbol[dot+1:]
	// A crypto pair like BTC-USD has no dot suffix; guard common quote suffixes.
	switch suffix {
	case "USD", "USDT", "USDC":
		return false
	}
	return true
}

func looksLikeCrypto(symbol string) bool {
	return strings.HasSuffix(symbol, "-USD") ||
		strings.HasSuffix(symbol, "-USDT") ||
		strings.HasSuffix(symbol, "-USDC")
}

func sectorMatches(sector string, needles ...string) bool {
	if sector == "" {
		return false
	}
	for _, n := range needles {
		if strings.Contains(sector, n) {
			return true
		}
	}
	return false
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// instrumentDNAFactors is the curated fallback map of common tickers. Symbols
// are stored uppercase and matched after trimming/uppercasing the input, so the
// app's existing symbol format (including suffixes like THYAO.IS) is preserved.
var instrumentDNAFactors = map[string]InstrumentDNAFactors{
	"QQQ":      {Growth: 90, Income: 5, Commodities: 0, Defensive: 15, International: 5, Volatility: 65, FocusTags: []string{"Technology", "Growth", "US Large Cap"}},
	"SPY":      {Growth: 55, Income: 25, Commodities: 0, Defensive: 45, International: 5, Volatility: 45, FocusTags: []string{"US Large Cap", "Broad Market", "ETF"}},
	"VOO":      {Growth: 55, Income: 25, Commodities: 0, Defensive: 45, International: 5, Volatility: 45, FocusTags: []string{"US Large Cap", "Broad Market", "ETF"}},
	"IVV":      {Growth: 55, Income: 25, Commodities: 0, Defensive: 45, International: 5, Volatility: 45, FocusTags: []string{"US Large Cap", "Broad Market", "ETF"}},
	"VTI":      {Growth: 55, Income: 20, Commodities: 0, Defensive: 40, International: 5, Volatility: 45, FocusTags: []string{"US Total Market", "Broad Market", "ETF"}},
	"SCHD":     {Growth: 25, Income: 85, Commodities: 0, Defensive: 65, International: 5, Volatility: 35, FocusTags: []string{"Dividend", "Income", "Defensive"}},
	"VYM":      {Growth: 25, Income: 82, Commodities: 0, Defensive: 62, International: 5, Volatility: 35, FocusTags: []string{"Dividend", "Income", "Defensive"}},
	"SGOV":     {Growth: 0, Income: 95, Commodities: 0, Defensive: 95, International: 0, Volatility: 5, FocusTags: []string{"Treasury Bills", "Cash-Like", "Income", "Defensive"}},
	"BIL":      {Growth: 0, Income: 95, Commodities: 0, Defensive: 95, International: 0, Volatility: 5, FocusTags: []string{"Treasury Bills", "Cash-Like", "Income", "Defensive"}},
	"SHY":      {Growth: 0, Income: 80, Commodities: 0, Defensive: 80, International: 0, Volatility: 15, FocusTags: []string{"Short Bonds", "Income", "Defensive"}},
	"TLT":      {Growth: 0, Income: 65, Commodities: 0, Defensive: 45, International: 0, Volatility: 55, FocusTags: []string{"Long Bonds", "Rates", "Income"}},
	"BND":      {Growth: 0, Income: 70, Commodities: 0, Defensive: 60, International: 5, Volatility: 30, FocusTags: []string{"Aggregate Bonds", "Income", "Defensive"}},
	"LQD":      {Growth: 0, Income: 70, Commodities: 0, Defensive: 55, International: 5, Volatility: 35, FocusTags: []string{"Corporate Bonds", "Income"}},
	"TIP":      {Growth: 0, Income: 65, Commodities: 0, Defensive: 60, International: 0, Volatility: 30, FocusTags: []string{"Inflation-Protected", "Income", "Defensive"}},
	"GLD":      {Growth: 5, Income: 0, Commodities: 100, Defensive: 45, International: 0, Volatility: 60, FocusTags: []string{"Gold", "Commodities", "Precious Metals"}},
	"IAU":      {Growth: 5, Income: 0, Commodities: 100, Defensive: 45, International: 0, Volatility: 60, FocusTags: []string{"Gold", "Commodities", "Precious Metals"}},
	"SLV":      {Growth: 5, Income: 0, Commodities: 100, Defensive: 30, International: 0, Volatility: 80, FocusTags: []string{"Silver", "Commodities", "Precious Metals"}},
	"SIVR":     {Growth: 5, Income: 0, Commodities: 100, Defensive: 30, International: 0, Volatility: 80, FocusTags: []string{"Silver", "Commodities", "Precious Metals"}},
	"URA":      {Growth: 45, Income: 5, Commodities: 90, Defensive: 10, International: 40, Volatility: 85, FocusTags: []string{"Uranium", "Energy", "Commodities", "Thematic"}},
	"URNM":     {Growth: 45, Income: 5, Commodities: 90, Defensive: 10, International: 45, Volatility: 88, FocusTags: []string{"Uranium", "Energy", "Commodities", "Thematic"}},
	"XLE":      {Growth: 20, Income: 45, Commodities: 85, Defensive: 20, International: 5, Volatility: 70, FocusTags: []string{"Energy", "Commodities", "Cyclical"}},
	"BTC-USD":  {Growth: 35, Income: 0, Commodities: 20, Defensive: 0, International: 50, Volatility: 95, FocusTags: []string{"Crypto", "High Volatility"}},
	"ETH-USD":  {Growth: 40, Income: 0, Commodities: 15, Defensive: 0, International: 50, Volatility: 95, FocusTags: []string{"Crypto", "High Volatility"}},
	"AAPL":     {Growth: 70, Income: 15, Commodities: 0, Defensive: 35, International: 20, Volatility: 55, FocusTags: []string{"Technology", "Mega Cap", "Growth"}},
	"MSFT":     {Growth: 75, Income: 15, Commodities: 0, Defensive: 40, International: 20, Volatility: 50, FocusTags: []string{"Technology", "Mega Cap", "Growth"}},
	"NVDA":     {Growth: 95, Income: 5, Commodities: 0, Defensive: 10, International: 15, Volatility: 80, FocusTags: []string{"AI", "Semiconductors", "Growth", "High Volatility"}},
	"GOOGL":    {Growth: 72, Income: 5, Commodities: 0, Defensive: 30, International: 20, Volatility: 55, FocusTags: []string{"Technology", "Mega Cap", "Growth"}},
	"GOOG":     {Growth: 72, Income: 5, Commodities: 0, Defensive: 30, International: 20, Volatility: 55, FocusTags: []string{"Technology", "Mega Cap", "Growth"}},
	"AMZN":     {Growth: 78, Income: 0, Commodities: 0, Defensive: 20, International: 20, Volatility: 62, FocusTags: []string{"Technology", "Consumer", "Growth"}},
	"META":     {Growth: 80, Income: 5, Commodities: 0, Defensive: 15, International: 20, Volatility: 65, FocusTags: []string{"Technology", "Growth"}},
	"TSLA":     {Growth: 88, Income: 0, Commodities: 0, Defensive: 5, International: 25, Volatility: 88, FocusTags: []string{"Technology", "Growth", "High Volatility"}},
	"IJR":      {Growth: 45, Income: 20, Commodities: 0, Defensive: 25, International: 0, Volatility: 70, FocusTags: []string{"Small Caps", "US Equities"}},
	"IWM":      {Growth: 48, Income: 15, Commodities: 0, Defensive: 25, International: 0, Volatility: 70, FocusTags: []string{"Small Caps", "US Equities"}},
	"VXUS":     {Growth: 45, Income: 25, Commodities: 0, Defensive: 40, International: 100, Volatility: 50, FocusTags: []string{"International", "Broad Market", "ETF"}},
	"VEA":      {Growth: 45, Income: 25, Commodities: 0, Defensive: 40, International: 100, Volatility: 50, FocusTags: []string{"Developed Markets", "International", "ETF"}},
	"VWO":      {Growth: 55, Income: 20, Commodities: 0, Defensive: 25, International: 100, Volatility: 65, FocusTags: []string{"Emerging Markets", "International", "ETF"}},
	"EEM":      {Growth: 55, Income: 20, Commodities: 0, Defensive: 25, International: 100, Volatility: 68, FocusTags: []string{"Emerging Markets", "International", "ETF"}},
	"EFA":      {Growth: 45, Income: 25, Commodities: 0, Defensive: 40, International: 100, Volatility: 52, FocusTags: []string{"Developed Markets", "International", "ETF"}},
	"VT":       {Growth: 50, Income: 22, Commodities: 0, Defensive: 40, International: 50, Volatility: 48, FocusTags: []string{"Global", "Broad Market", "ETF"}},
	"THYAO.IS": {Growth: 45, Income: 10, Commodities: 0, Defensive: 10, International: 100, Volatility: 80, FocusTags: []string{"Turkey", "Airlines", "International"}},
}
