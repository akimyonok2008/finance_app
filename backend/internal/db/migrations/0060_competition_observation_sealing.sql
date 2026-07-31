-- Fixes a correctness gap in migration 0050: an observation set was marked
-- 'completed' as soon as the CURRENT worker batch's requested symbols/pairs
-- were captured, never checking whether that batch covered the competition's
-- entire entry population. Baseline batches at 200 entries and finalization
-- batches at 500 (see baseline.go/finalize.go), so any competition larger
-- than one batch could see its start/end observation set marked complete
-- after the FIRST batch while later batches still introduced symbols or
-- currency pairs no earlier batch had reason to request — and this status is
-- surfaced directly to admins (AdminObservationStatus), so the flag being
-- wrong was a real, user-visible correctness issue, not just cosmetic.
--
-- The application-side fix (see observations.go's captureObservations) now
-- only flips the set to 'sealed' once a pass both fully captures its own
-- batch's requested symbols/pairs AND that batch was the LAST one needed to
-- sweep the boundary's entire population (mirroring the finalization
-- generation's own lap-complete check). 'completed' is renamed to 'sealed'
-- to make that stronger guarantee unambiguous at the call sites.
--
-- Any set already marked 'completed' under the old, insufficient rule cannot
-- be trusted to reflect true full coverage, so it is reset to 'pending' here:
-- captured price/FX rows are untouched (already-captured symbols are never
-- re-fetched), and the next baseline/finalize pass re-derives the seal
-- correctly from the now-fixed rule.
ALTER TABLE competition_observation_sets
    DROP CONSTRAINT competition_observation_sets_observation_status_check;

UPDATE competition_observation_sets
SET observation_status = 'pending', completed_at = NULL
WHERE observation_status = 'completed';

ALTER TABLE competition_observation_sets
    ADD CONSTRAINT competition_observation_sets_observation_status_check
        CHECK (observation_status IN ('pending', 'sealed'));
