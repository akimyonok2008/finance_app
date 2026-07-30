-- users.avatar_key (migration 0001) never had the length cap the profiles
-- table's avatar_key got in migration 0002. It's user-controlled free text
-- rendered directly as an avatar badge's contents (there is no image/upload
-- avatar system), so an unbounded value can overflow that fixed-size UI
-- chrome. The application layer (auth.Service) now truncates at write time,
-- but this backfills any pre-existing rows and adds the same CHECK
-- constraint as profiles for defense in depth.
UPDATE users SET avatar_key = left(avatar_key, 40) WHERE char_length(avatar_key) > 40;

ALTER TABLE users
    ADD CONSTRAINT users_avatar_key_length CHECK (char_length(avatar_key) <= 40);
