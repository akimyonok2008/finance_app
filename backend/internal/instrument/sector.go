package instrument

import "strings"

// Sector is a coarse GICS-like industry classification persisted on an
// instrument's identity row. It is intentionally a small, closed taxonomy
// rather than full GICS (11 sectors, ~150 sub-industries): nothing in this
// codebase needs sub-industry granularity yet, and a small closed set is what
// lets a CHECK constraint (migration 0041) guarantee the column never drifts
// into free text.
type Sector string

const (
	SectorTechnology            Sector = "technology"
	SectorHealthcare            Sector = "healthcare"
	SectorFinancials            Sector = "financials"
	SectorConsumerDiscretionary Sector = "consumer_discretionary"
	SectorConsumerStaples       Sector = "consumer_staples"
	SectorEnergy                Sector = "energy"
	SectorIndustrials           Sector = "industrials"
	SectorMaterials             Sector = "materials"
	SectorUtilities             Sector = "utilities"
	SectorRealEstate            Sector = "real_estate"
	SectorCommunicationServices Sector = "communication_services"
	// SectorUnknown is the default for instruments nothing has classified yet.
	// It is a valid, storable value, not an error sentinel: most instruments
	// created via OpenFIGI resolution will carry it until curated.
	SectorUnknown Sector = "unknown"
)

// Sectors lists every value the instrument_master.sector CHECK constraint
// accepts, in the same order as migration 0041.
var Sectors = []Sector{
	SectorTechnology, SectorHealthcare, SectorFinancials, SectorConsumerDiscretionary,
	SectorConsumerStaples, SectorEnergy, SectorIndustrials, SectorMaterials,
	SectorUtilities, SectorRealEstate, SectorCommunicationServices, SectorUnknown,
}

// Valid reports whether s is one of the taxonomy's closed set of values.
func (s Sector) Valid() bool {
	for _, v := range Sectors {
		if s == v {
			return true
		}
	}
	return false
}

// ClassifySector infers a coarse sector for a ticker from a small curated seed
// list, keyed on the same normalized ticker aliases carry.
//
// OpenFIGI's mapping response includes a marketSector field, but it reports a
// broad asset class ("Equity", "Corp", "Govt", "Comdty", ...), not a GICS
// industry sector — it cannot tell "Technology" from "Healthcare" for two
// equities, so it is deliberately not used as a classification source here.
// An unknown ticker classifies as SectorUnknown rather than guessing from
// weaker signals.
func ClassifySector(symbol string) Sector {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if sector, ok := curatedSectors[sym]; ok {
		return sector
	}
	return SectorUnknown
}

// curatedSectors is a manually curated seed list covering large, liquid
// names likely to appear in real portfolios. It is deliberately small: a
// wrong guess (e.g. from a heuristic on the company name) is worse than an
// honest SectorUnknown, since eligibility rules built on top of this data
// must not silently misclassify a holding.
var curatedSectors = map[string]Sector{
	"AAPL": SectorTechnology,
	"MSFT": SectorTechnology,
	"NVDA": SectorTechnology,
	"AVGO": SectorTechnology,
	"AMD":  SectorTechnology,
	"CRM":  SectorTechnology,
	"ORCL": SectorTechnology,
	"ADBE": SectorTechnology,
	"INTC": SectorTechnology,
	"CSCO": SectorTechnology,
	"QQQ":  SectorTechnology, // tech-heavy index ETF, treated as a sector proxy

	"GOOGL": SectorCommunicationServices,
	"GOOG":  SectorCommunicationServices,
	"META":  SectorCommunicationServices,
	"NFLX":  SectorCommunicationServices,
	"DIS":   SectorCommunicationServices,

	"AMZN": SectorConsumerDiscretionary,
	"TSLA": SectorConsumerDiscretionary,
	"HD":   SectorConsumerDiscretionary,
	"MCD":  SectorConsumerDiscretionary,
	"NKE":  SectorConsumerDiscretionary,

	"JPM": SectorFinancials,
	"BAC": SectorFinancials,
	"WFC": SectorFinancials,
	"GS":  SectorFinancials,
	"V":   SectorFinancials,
	"MA":  SectorFinancials,

	"JNJ":  SectorHealthcare,
	"UNH":  SectorHealthcare,
	"PFE":  SectorHealthcare,
	"LLY":  SectorHealthcare,
	"ABBV": SectorHealthcare,

	"XOM": SectorEnergy,
	"CVX": SectorEnergy,
	"XLE": SectorEnergy,

	"PG":  SectorConsumerStaples,
	"KO":  SectorConsumerStaples,
	"PEP": SectorConsumerStaples,
	"WMT": SectorConsumerStaples,

	"CAT": SectorIndustrials,
	"BA":  SectorIndustrials,
	"GE":  SectorIndustrials,

	"NEE": SectorUtilities,
	"DUK": SectorUtilities,

	"LIN": SectorMaterials,
	"FCX": SectorMaterials,

	"PLD": SectorRealEstate,
	"AMT": SectorRealEstate,
}
