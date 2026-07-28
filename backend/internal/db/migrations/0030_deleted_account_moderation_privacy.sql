-- Correct moderation retention for databases that already applied 0028.
--
-- 0028 preserved reports across the application's real hard-delete path, but
-- its mapping table retained the original user UUID and report_evidence kept
-- raw sender, participant, and conversation identifiers. Snapshot opaque ids
-- into evidence, remove those raw evidence identifiers, and erase the mapping
-- row atomically whenever a user is deleted.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE report_evidence
    ADD COLUMN IF NOT EXISTS sender_subject_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS participant_subject_ids TEXT[] NULL,
    ADD COLUMN IF NOT EXISTS conversation_subject_id TEXT NULL
        DEFAULT encode(gen_random_bytes(16), 'hex');

-- Evidence may reference users not otherwise present in a report/action role,
-- so make sure each raw evidence identifier has an opaque mapping before the
-- one-time backfill.
INSERT INTO moderation_subject_ids (user_id)
SELECT DISTINCT user_id
FROM (
    SELECT sender_id AS user_id
    FROM report_evidence
    WHERE sender_id IS NOT NULL

    UNION

    SELECT p.participant_id::uuid AS user_id
    FROM report_evidence, unnest(participant_ids) AS p(participant_id)
) evidence_users
WHERE user_id IS NOT NULL
ON CONFLICT (user_id) DO NOTHING;

UPDATE report_evidence e
SET sender_subject_id = COALESCE(
    (SELECT m.subject_id
     FROM moderation_subject_ids m
     WHERE m.user_id = e.sender_id),
    (SELECT r.reported_subject_id
     FROM user_reports r
     WHERE r.id = e.report_id)
)
WHERE e.sender_subject_id IS NULL;

UPDATE report_evidence e
SET participant_subject_ids = ARRAY(
    SELECT m.subject_id
    FROM unnest(e.participant_ids) WITH ORDINALITY AS p(user_id, ordinal)
    JOIN moderation_subject_ids m ON m.user_id = p.user_id::uuid
    ORDER BY p.ordinal
)
WHERE e.participant_subject_ids IS NULL;

UPDATE report_evidence
SET conversation_subject_id = encode(gen_random_bytes(16), 'hex')
WHERE conversation_subject_id IS NULL;

ALTER TABLE report_evidence
    ALTER COLUMN sender_subject_id SET NOT NULL,
    ALTER COLUMN participant_subject_ids SET DEFAULT '{}',
    ALTER COLUMN participant_subject_ids SET NOT NULL,
    ALTER COLUMN conversation_subject_id SET NOT NULL;

UPDATE report_evidence
SET sender_id = NULL,
    participant_ids = '{}',
    conversation_id = NULL;

-- Reports/actions/evidence now own their opaque subject snapshots. Remove
-- mappings left behind by hard deletes that happened before this correction.
DELETE FROM moderation_subject_ids m
WHERE NOT EXISTS (
    SELECT 1 FROM users u WHERE u.id = m.user_id
);

-- Make erasure an invariant of the schema, not an application convention.
ALTER TABLE moderation_subject_ids
    DROP CONSTRAINT IF EXISTS moderation_subject_ids_user_id_fkey;
ALTER TABLE moderation_subject_ids
    ADD CONSTRAINT moderation_subject_ids_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
