// Package telemetry holds process-wide Prometheus counters that need to be
// callable from packages which cannot import each other (or the server
// package, which would create an import cycle) without pulling in a full
// metrics-interface/dependency-injection chain for a handful of counters.
// Every counter here registers itself against the default Prometheus
// registry via promauto, so it is automatically served at GET /metrics
// (internal/server/metrics.go) with no additional wiring.
package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// These five counters answer one question: how much of the system is still
// running on the ticker-string fallback instead of resolved instrument
// identity? A deployment should watch all five trend toward (and stay at)
// zero as backfill completes and provider coverage improves.
var (
	instrumentResolutionAmbiguousTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "instrument_resolution_ambiguous_total",
		Help: "Buy or corporate-action instrument resolution attempts that matched more than one candidate identity and were rejected rather than guessed.",
	})
	instrumentResolutionUnresolvedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "instrument_resolution_unresolved_total",
		Help: "Buy or corporate-action instrument resolution attempts that matched no candidate identity.",
	})
	legacySymbolFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "legacy_symbol_fallback_total",
		Help: "Reads (income entitlement, holder discovery, pricing) that fell back to symbol-string matching because the record had no resolved instrument_id.",
	})
	providerMappingMissingTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "provider_mapping_missing_total",
		Help: "Attempts to translate a resolved instrument_id into a provider-facing ticker that found no usable alias.",
	})
	corporateActionQuarantinedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "corporate_action_quarantined_total",
		Help: "Corporate-action provider events that could not be matched to a stable instrument identity and were held rather than applied.",
	})
)

func IncInstrumentResolutionAmbiguous()  { instrumentResolutionAmbiguousTotal.Inc() }
func IncInstrumentResolutionUnresolved() { instrumentResolutionUnresolvedTotal.Inc() }
func IncLegacySymbolFallback()           { legacySymbolFallbackTotal.Inc() }
func IncProviderMappingMissing()         { providerMappingMissingTotal.Inc() }
func IncCorporateActionQuarantined()     { corporateActionQuarantinedTotal.Inc() }
