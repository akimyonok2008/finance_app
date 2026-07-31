-- Arena catalogue pagination: the catalogue query now filters by bucket
-- (live/upcoming/mine/completed) and paginates in SQL instead of loading
-- every competition row and filtering in Go. Legacy weekly-sprint rows
-- derive their bucket from starts_at/ends_at at query time (no stored
-- status), so index that comparison directly; engine editions already have
-- competitions_lifecycle_starts_idx (migration 0045) for their stored
-- lifecycle_status.
CREATE INDEX IF NOT EXISTS competitions_legacy_window_idx
    ON competitions (starts_at, ends_at)
    WHERE lifecycle_status = 'legacy';
