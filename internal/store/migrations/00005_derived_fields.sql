-- Step 4's derived ranking columns (memory spec M-3). Three columns beside the
-- payload, never a rewrite of it (I-10).
--
-- This migration does not rebuild the FTS index and must never start to. Memory
-- spec M-2's decision 7 settled that Step 3 and Step 4 are two migrations
-- precisely because these columns are a ranking input rather than indexed text:
-- nothing here is added to events_fts, and the 1.0 spec 7.1's `00002` row - 1.30
-- s and roughly double the file at 8,177 events, against 17,043 now - is the cost
-- that decision exists to avoid paying twice.
--
-- Which is also why events_au is dropped around the backfill and put back
-- afterwards. It is the external-content update trigger, and an UPDATE that
-- touches only these three columns would still fire it, deleting and reinserting
-- every row's leaves into the index for no change at all - the rebuild this
-- migration is defined by not doing, arriving through the back door. The other
-- two triggers stay: the backfill inserts and deletes nothing.
--
-- Every expression below has a Go twin in [Derive], and TestTheTwoDerivedWalksAgree
-- is what holds them together. The shape they share is not decoration:
--
--   * json_valid() guards the whole statement, because json_type() raises on a
--     payload that is not JSON, and because it is the only thing on this side
--     that answers 0 for a nesting past SQLite's 1000 open containers. Go's
--     sqliteWillParse is that guard.
--   * json_type(...) = 'text' guards every read, because json_extract of a path
--     that names an object or an array returns that container's JSON *text* -
--     which would put a serialised structuredPatch in a ranking column. Go's
--     type assertions are that guard.
--   * COALESCE over NULLIF is first-non-empty and not a join. The paths column
--     has two sources that name the same file when both are present, and the
--     output column has three that are alternatives rather than parts.
--
-- What is deliberately not here, and it is a correction to M-3's own list rather
-- than an omission: an exit code and a success flag. Measured 2026-09-03 over
-- the 902 captures - tool_response.stderr is present on 241 documents and
-- non-empty on none, `success` appears on 3, and exactly one key in the whole
-- corpus matches /exit|return.?code|errno/. There is nothing to read, so nothing
-- is read. M-3's error spans are in derived_output, in prose, because that is
-- where the corpus keeps them.

-- +goose Up

-- A constant DEFAULT makes ADD COLUMN O(1) in SQLite: the default lives in the
-- schema and old rows are not rewritten to carry it. The empty string means
-- "this payload said nothing here" and is the same value Derive's zero value
-- carries, so a row the backfill skips and a row it processed to nothing are
-- indistinguishable on purpose - both are documents with no command.
ALTER TABLE events ADD COLUMN derived_cmd    TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN derived_paths  TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN derived_output TEXT NOT NULL DEFAULT '';

DROP TRIGGER events_au;

UPDATE events SET
    derived_cmd = CASE
        WHEN json_type(payload, '$.tool_input.command') = 'text'
        THEN json_extract(payload, '$.tool_input.command')
        ELSE ''
    END,
    derived_paths = COALESCE(
        NULLIF(CASE
            WHEN json_type(payload, '$.tool_input.file_path') = 'text'
            THEN json_extract(payload, '$.tool_input.file_path')
            ELSE ''
        END, ''),
        NULLIF(CASE
            WHEN json_type(payload, '$.tool_response.filePath') = 'text'
            THEN json_extract(payload, '$.tool_response.filePath')
            ELSE ''
        END, ''),
        ''
    ),
    derived_output = COALESCE(
        NULLIF(CASE
            WHEN json_type(payload, '$.tool_response.stdout') = 'text'
            THEN json_extract(payload, '$.tool_response.stdout')
            ELSE ''
        END, ''),
        NULLIF(CASE
            WHEN json_type(payload, '$.tool_response.content') = 'text'
            THEN json_extract(payload, '$.tool_response.content')
            ELSE ''
        END, ''),
        NULLIF(CASE
            WHEN json_type(payload, '$.tool_response') = 'text'
            THEN json_extract(payload, '$.tool_response')
            ELSE ''
        END, ''),
        ''
    )
WHERE json_valid(payload);

-- Put back exactly what 00002 created. Copied rather than referenced, because a
-- trigger has no other form: if 00002's body ever changes, this one has to
-- change with it, and TestTheFTSIndexSurvivesTheDerivedBackfill is what fails
-- when they drift.
-- +goose StatementBegin
CREATE TRIGGER events_au AFTER UPDATE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, leaves)
    VALUES ('delete', old.rowid, old.leaves);
    INSERT INTO events_fts(rowid, leaves) VALUES (new.rowid, new.leaves);
END;
-- +goose StatementEnd

-- +goose Down

ALTER TABLE events DROP COLUMN derived_output;
ALTER TABLE events DROP COLUMN derived_paths;
ALTER TABLE events DROP COLUMN derived_cmd;
