-- Arena competition engine: persisted common boundary observations.
--
-- Fixes the critical correctness gap where the baseline worker's batching
-- (200 entries/pass) and the finalizer's retry loop each queried the live
-- price/FX providers fresh per pass, so entries baselined or finalized in
-- different passes could be priced from different real-world moments even
-- though they nominally share the same starts_at/ends_at.
--
-- One observation set per (competition, boundary) is captured incrementally
-- across passes and never re-queried once a symbol/pair is captured: the
-- first pass that needs a given symbol fetches and freezes it, every later
-- pass (and every entry, in any batch) reuses that exact stored value.

CREATE TABLE competition_observation_sets (
    id                  UUID PRIMARY KEY,
    competition_id      TEXT NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    boundary_type       TEXT NOT NULL CHECK (boundary_type IN ('start', 'end')),
    effective_at        TIMESTAMPTZ NOT NULL,
    provider            TEXT NOT NULL,
    observation_status  TEXT NOT NULL DEFAULT 'pending' CHECK (observation_status IN ('pending', 'completed')),
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (competition_id, boundary_type)
);

CREATE TABLE competition_price_observations (
    id                    UUID PRIMARY KEY,
    observation_set_id    UUID NOT NULL REFERENCES competition_observation_sets(id) ON DELETE CASCADE,
    symbol                TEXT NOT NULL,
    price                 NUMERIC(24, 8) NOT NULL,
    currency              TEXT NOT NULL,
    provider_observed_at  TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (observation_set_id, symbol)
);

CREATE TABLE competition_fx_observations (
    id                    UUID PRIMARY KEY,
    observation_set_id    UUID NOT NULL REFERENCES competition_observation_sets(id) ON DELETE CASCADE,
    from_currency         TEXT NOT NULL,
    to_currency           TEXT NOT NULL,
    rate                  NUMERIC(24, 12) NOT NULL,
    provider_observed_at  TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (observation_set_id, from_currency, to_currency)
);

CREATE INDEX competition_price_observations_set_idx ON competition_price_observations (observation_set_id);
CREATE INDEX competition_fx_observations_set_idx ON competition_fx_observations (observation_set_id);
