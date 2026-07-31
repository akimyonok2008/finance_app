-- Seeds the Technology Challenge (competitions.TechnologyChallengeID): a
-- long-running competition open only to portfolios with at least 50%
-- technology-sector exposure (competitions/rules.Filter, evaluated at join
-- time in competitions/eligibility.go against instrument.Sector data
-- populated by migration 0041).
--
-- Unlike the weekly sprint, this row is not derived from the clock: status is
-- recomputed from starts_at/ends_at on every read (competitions.deriveStatus),
-- so a wide, fixed window just needs to already be "active" and stay that way
-- for a long time.

INSERT INTO competitions (id, name, type, starts_at, ends_at, status, eligibility_filter, created_at)
VALUES (
    'technology_challenge',
    'Technology Challenge',
    'sector_challenge',
    '2026-01-01T00:00:00Z',
    '2099-12-31T00:00:00Z',
    'active',
    '{"metric":"portfolio_weight","sectors":["technology"],"operator":">=","threshold":0.5}',
    now()
)
ON CONFLICT (id) DO NOTHING;
