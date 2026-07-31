package instrument

import "testing"

func TestClassifySector_CuratedSymbols(t *testing.T) {
	tests := []struct {
		symbol string
		want   Sector
	}{
		{"AAPL", SectorTechnology},
		{"aapl", SectorTechnology}, // case-insensitive
		{" MSFT ", SectorTechnology},
		{"GOOGL", SectorCommunicationServices},
		{"AMZN", SectorConsumerDiscretionary},
		{"JPM", SectorFinancials},
		{"JNJ", SectorHealthcare},
		{"XOM", SectorEnergy},
		{"NOT_A_REAL_TICKER", SectorUnknown},
		{"", SectorUnknown},
	}
	for _, tt := range tests {
		if got := ClassifySector(tt.symbol); got != tt.want {
			t.Errorf("ClassifySector(%q) = %q, want %q", tt.symbol, got, tt.want)
		}
	}
}

func TestSector_Valid(t *testing.T) {
	for _, s := range Sectors {
		if !s.Valid() {
			t.Errorf("Sector %q from the taxonomy's own list reported invalid", s)
		}
	}
	if Sector("not_a_sector").Valid() {
		t.Error("an arbitrary string must not validate as a Sector")
	}
}
