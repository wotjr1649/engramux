-- Backlog 49: re-judge every stored event's host against spec 4.3's corrected
-- rule, and take apart the sessions the old one invented.
--
-- # What was wrong
--
-- Detection read its key rules before its transcript_path rule. Claude Code's
-- SessionStart payload carries `model` and no `prompt_id`, so the key rules
-- answered Codex for the first event of every Claude Code session - while the
-- transcript sat under .claude and would have said so. SessionEnd carries no
-- `model`, fell through to the path rule and landed correctly, so what each
-- session left behind was a Codex session row holding exactly one event,
-- created at the real session's start time, and standing `active` for ever. The
-- schema permits the pair because sessions are unique on (host, host_session_id)
-- rather than on the host session id alone.
--
-- # Why a re-scan is sound rather than a guess
--
-- The payload is stored verbatim and is never rewritten (I-10), so the host is
-- a function of a column this database still holds. 00001's own comment says
-- the CHECK exists so that "a later ruleset can re-scan without ambiguity about
-- what an old row was judged against"; this is that re-scan.
--
-- It is deliberately written as the rule and not as the symptom. Fixing only
-- `SessionStart` would leave `PreCompact` and `PostCompact`, which the corpus
-- also has no Claude Code capture of and which may carry the same keys. Any
-- event whose transcript_path names a host other than the one it is stored
-- under is corrected, in both directions.
--
-- Events with no transcript_path are not touched. The key rules are still
-- authoritative there, which is what the corrected order says.
--
-- # Cost
--
-- One update per corrected row, and the events_au trigger deletes and reinserts
-- that row in events_fts. `leaves` is not changed, so the tokens that come back
-- are the ones that left. The row count is the number of Claude Code sessions
-- this database has ever seen - one event each.

-- +goose Up

-- +goose StatementBegin
CREATE TEMP TABLE engramux_00006_remap AS
SELECT
    e.id              AS event_id,
    e.host            AS was,
    e.session_id      AS was_session,
    e.received_at     AS received_at,
    s.host_session_id AS hsid,
    s.project_id      AS project_id,
    s.status          AS status,
    s.created_at      AS created_at,
    s.ended_at        AS ended_at,
    CASE
        WHEN instr(replace(coalesce(json_extract(e.payload, '$.transcript_path'), ''), char(92), '/'), '/.claude/') > 0
            THEN 'claude-code'
        WHEN instr(replace(coalesce(json_extract(e.payload, '$.transcript_path'), ''), char(92), '/'), '/.codex/') > 0
            THEN 'codex'
    END AS want
FROM events e
JOIN sessions s ON s.id = e.session_id
WHERE json_valid(e.payload);
-- +goose StatementEnd

DELETE FROM engramux_00006_remap WHERE want IS NULL OR want = was;

-- The session each corrected event is moving to, where it does not exist yet.
-- A Claude Code session whose only captured event was its SessionStart has no
-- row under its real host at all, so this is not only a no-op guard.
INSERT OR IGNORE INTO sessions (id, project_id, host, host_session_id, status, created_at, ended_at)
SELECT DISTINCT want || ':' || hsid, project_id, want, hsid, status, created_at, ended_at
FROM engramux_00006_remap;

UPDATE events
SET host       = (SELECT r.want FROM engramux_00006_remap r WHERE r.event_id = events.id),
    session_id = (SELECT r.want || ':' || r.hsid FROM engramux_00006_remap r WHERE r.event_id = events.id)
WHERE id IN (SELECT event_id FROM engramux_00006_remap);

-- `first seen` is a session's created_at, and a session that has just received
-- an event older than itself would report a start after its own first event.
UPDATE sessions
SET created_at = (SELECT min(e.received_at) FROM events e WHERE e.session_id = sessions.id)
WHERE id IN (SELECT DISTINCT want || ':' || hsid FROM engramux_00006_remap)
  AND created_at > (SELECT min(e.received_at) FROM events e WHERE e.session_id = sessions.id);

-- The invented sessions, now that nothing points at them. Only the rows this
-- migration emptied: a session that was already empty is not this one's to
-- judge.
DELETE FROM sessions
WHERE id IN (SELECT DISTINCT was_session FROM engramux_00006_remap)
  AND NOT EXISTS (SELECT 1 FROM events e WHERE e.session_id = sessions.id);

DROP TABLE engramux_00006_remap;

-- +goose Down

-- There is nothing to restore. What this migration replaced was a wrong answer
-- derived from the same payload the right answer is derived from, so a Down
-- would have to reintroduce the defect by rule rather than recover a value -
-- and the sessions it deleted held no events by the time they were deleted.
-- Rolling back to 00005 leaves the corrected rows in place, which is the same
-- database a fresh install on that version would build from these payloads.
SELECT 1 WHERE 0;
