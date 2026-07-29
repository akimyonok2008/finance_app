package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestIdentityCounters_IncrementAndAreScraped proves the five counters this
// package exposes actually register against the default Prometheus registry
// (so GET /metrics on the operations router serves them, see
// internal/server/router.go NewOperations) and that each Inc function moves
// only its own counter.
func TestIdentityCounters_IncrementAndAreScraped(t *testing.T) {
	before := testutil.ToFloat64(instrumentResolutionAmbiguousTotal)
	IncInstrumentResolutionAmbiguous()
	if got := testutil.ToFloat64(instrumentResolutionAmbiguousTotal); got != before+1 {
		t.Fatalf("instrument_resolution_ambiguous_total: got %v, want %v", got, before+1)
	}

	before = testutil.ToFloat64(instrumentResolutionUnresolvedTotal)
	IncInstrumentResolutionUnresolved()
	if got := testutil.ToFloat64(instrumentResolutionUnresolvedTotal); got != before+1 {
		t.Fatalf("instrument_resolution_unresolved_total: got %v, want %v", got, before+1)
	}

	before = testutil.ToFloat64(legacySymbolFallbackTotal)
	IncLegacySymbolFallback()
	if got := testutil.ToFloat64(legacySymbolFallbackTotal); got != before+1 {
		t.Fatalf("legacy_symbol_fallback_total: got %v, want %v", got, before+1)
	}

	before = testutil.ToFloat64(providerMappingMissingTotal)
	IncProviderMappingMissing()
	if got := testutil.ToFloat64(providerMappingMissingTotal); got != before+1 {
		t.Fatalf("provider_mapping_missing_total: got %v, want %v", got, before+1)
	}

	before = testutil.ToFloat64(corporateActionQuarantinedTotal)
	IncCorporateActionQuarantined()
	if got := testutil.ToFloat64(corporateActionQuarantinedTotal); got != before+1 {
		t.Fatalf("corporate_action_quarantined_total: got %v, want %v", got, before+1)
	}
}
