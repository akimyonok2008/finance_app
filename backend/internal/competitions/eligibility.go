package competitions

import (
	"context"

	"github.com/ardakimyonok/finance_app/internal/competitions/rules"
	"github.com/ardakimyonok/finance_app/internal/money"
)

// unknownSector is the string form of instrument.SectorUnknown. It is
// duplicated here (rather than importing the instrument package for one
// constant) because eligibility evaluation only ever needs the string value,
// never the instrument package's richer Sector type or its taxonomy
// validation.
const unknownSector = "unknown"

// SectorProvider resolves a position's coarse sector classification at
// competition-join time. Implemented in production by an adapter over
// instrument.Resolver (see cmd/api/main.go); a nil SectorProvider makes every
// position classify as unknownSector, which is a safe default: a
// sector-gated competition simply becomes unjoinable until sector data is
// wired up, rather than silently admitting everyone.
type SectorProvider interface {
	SectorForSymbol(ctx context.Context, symbol string) (string, error)
}

// sectorForSymbol resolves symbol's sector via provider, falling back to
// unknownSector on a nil provider or a lookup error. A lookup failure must
// never abort the join: it only means that position won't count toward any
// sector-based eligibility filter.
func sectorForSymbol(ctx context.Context, provider SectorProvider, symbol string) string {
	if provider == nil {
		return unknownSector
	}
	sector, err := provider.SectorForSymbol(ctx, symbol)
	if err != nil || sector == "" {
		return unknownSector
	}
	return sector
}

// factsFromSnapshot converts join-time snapshot positions into the
// privacy-safe rules.PositionFacts eligibility evaluation needs: each
// position's weight as a fraction of the snapshot's total starting value.
// Positions are already priced/converted to base currency by JoinCompetition,
// so a plain float64 ratio is precise enough here — eligibility is a
// yes/no gate, not a balance calculation.
func factsFromSnapshot(snapshot []CompetitionEntrySnapshotPosition, totalBase money.Amount) []rules.PositionFacts {
	if totalBase.Cmp(money.ZeroAmount()) <= 0 {
		return nil
	}
	total := totalBase.Float64()
	facts := make([]rules.PositionFacts, 0, len(snapshot))
	for _, s := range snapshot {
		facts = append(facts, rules.PositionFacts{
			Sector: s.Sector,
			Weight: s.StartingValueBase.Float64() / total,
		})
	}
	return facts
}

// checkEligibility evaluates a competition's filter against a join-time
// snapshot. A nil filter (or the zero value) admits everyone.
func checkEligibility(filter *rules.Filter, snapshot []CompetitionEntrySnapshotPosition, totalBase money.Amount) (bool, error) {
	if filter == nil || filter.IsZero() {
		return true, nil
	}
	return rules.Evaluate(*filter, factsFromSnapshot(snapshot, totalBase))
}
