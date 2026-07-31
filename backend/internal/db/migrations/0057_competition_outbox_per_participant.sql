-- Make achievement projection independently retryable per participant. The
-- original finalization event stored every participant in one JSON array, so
-- one failure replayed every earlier successful participant.

ALTER TABLE competition_outbox
    DROP CONSTRAINT competition_outbox_event_type_competition_id_key;

ALTER TABLE competition_outbox
    ADD COLUMN participant_id UUID REFERENCES users(id) ON DELETE CASCADE;

-- Preserve outstanding and historical delivery state while expanding legacy
-- batch rows. Empty batches intentionally produce no participant work.
INSERT INTO competition_outbox
    (id, event_type, competition_id, participant_ids, participant_id,
     attempt_count, claimed_at, next_attempt_at, processed_at, last_error,
     dead_lettered_at, created_at)
SELECT DISTINCT ON (o.event_type, o.competition_id, participant.value)
       gen_random_uuid(), o.event_type, o.competition_id, '[]'::jsonb,
       participant.value::uuid, o.attempt_count, o.claimed_at,
       o.next_attempt_at, o.processed_at, o.last_error, o.dead_lettered_at,
       o.created_at
FROM competition_outbox o
CROSS JOIN LATERAL jsonb_array_elements_text(o.participant_ids) participant(value)
WHERE o.participant_id IS NULL;

DELETE FROM competition_outbox WHERE participant_id IS NULL;

ALTER TABLE competition_outbox
    ALTER COLUMN participant_id SET NOT NULL,
    DROP COLUMN participant_ids,
    ADD CONSTRAINT competition_outbox_event_competition_participant_key
        UNIQUE (event_type, competition_id, participant_id);
