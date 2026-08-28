-- Phase 1 schema (spec 6). This file is the truth: the design document holds no
-- DDL, because two of its earlier revisions tried to and got it wrong.
--
-- Every table is STRICT, so a column's declared type is enforced rather than
-- suggested. Under STRICT a column that is part of the PRIMARY KEY is also
-- implicitly NOT NULL, which closes SQLite's oldest footgun: in an ordinary
-- rowid table a `TEXT PRIMARY KEY` accepts NULL. The NOT NULLs below are written
-- out anyway, so the guarantee survives a table losing STRICT.
--
-- events_fts and its triggers are deliberately absent. FTS5 is Phase 4, and a
-- Phase 1 migration would freeze spec 5.7's tokenizer and external-content
-- decisions before they are made. Nothing in Phase 1's gate needs it:
-- foreign_key_check concerns foreign keys, and the five base tables carry them.

-- +goose Up

-- Project identity means: same repository, same worktree (spec 6). It survives
-- drive-letter case and trailing-separator differences, which is a property of
-- how root is normalised before it is hashed - the hash inputs are the code's
-- choice. root is UNIQUE as well as id: id is derived from root, so two rows
-- sharing a root can only mean the derivation changed or collided, and that is
-- worth failing on rather than storing twice.
CREATE TABLE projects (
    id         TEXT    NOT NULL PRIMARY KEY,
    root       TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

-- One row per host session (spec 6). Rows are created lazily on the first event,
-- so nothing here may depend on a SessionStart having arrived: in the corpus
-- only 9 of 19 sessions carry one and three Claude Code sessions have none at
-- all. ended_at is therefore the only nullable column, and created_at means "the
-- service first saw this session", not "the session started".
--
-- id combines host and host session id; the UNIQUE below is the same statement
-- made about the parts, so a bug in how they are combined shows up as a
-- constraint failure instead of a second row for one session.
CREATE TABLE sessions (
    id              TEXT    NOT NULL PRIMARY KEY,
    project_id      TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    host            TEXT    NOT NULL CHECK (host IN ('claude-code', 'codex', 'unknown')),
    host_session_id TEXT    NOT NULL,
    status          TEXT    NOT NULL CHECK (status IN ('active', 'stopped', 'ended')),
    created_at      INTEGER NOT NULL,
    ended_at        INTEGER,
    UNIQUE (host, host_session_id)
) STRICT;

-- Every captured event (spec 6).
--
-- id is the relay-minted UUIDv7 and *is* the idempotency key (I-05). There is no
-- idempotency_key column and one must not be added back: the best host-derived
-- key collapses 902 real captures into 762 groups, so a UNIQUE constraint on it
-- would reject 15.5% of real traffic - 114 SubagentStop rows share one key
-- because that event carries no identifier at all. Combined with the rule that a
-- duplicate ACKs `committed`, that design ACKs distinct events and drops them.
--
-- host and source carry CHECK constraints because their producers hand Go bare
-- strings: internal/host.Detect returns "claude-code", "codex" or "unknown" as
-- untyped values, and nothing in Go's type system stops a fourth one reaching
-- the column. `unknown` is reachable and is not an error - I-04 stores an
-- unclassifiable event rather than dropping it. source is set by the service
-- from the ingest path and is never carried in the envelope, so nothing a relay
-- sends can set it.
--
-- tool_use_id is nullable and non-unique on purpose: it pairs PreToolUse with
-- PostToolUse and is not an identity.
--
-- payload holds the bytes exactly as the host wrote them - Phase 1 gates on a
-- byte-for-byte round-trip, so nothing on the way in or out re-marshals it.
--
-- privacy_class carries internal/secret's Set in its stored form (sorted,
-- comma-joined class names, empty string for none) and redaction_version records
-- which ruleset judged the row, so a later ruleset can re-scan without ambiguity
-- about what an old row was judged against (spec 6.1).
CREATE TABLE events (
    id                TEXT    NOT NULL PRIMARY KEY,
    project_id        TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    session_id        TEXT    NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    host              TEXT    NOT NULL CHECK (host IN ('claude-code', 'codex', 'unknown')),
    source            TEXT    NOT NULL CHECK (source IN ('pipe', 'spool')),
    event_name        TEXT    NOT NULL,
    tool_name         TEXT,
    tool_use_id       TEXT,
    payload           TEXT    NOT NULL,
    privacy_class     TEXT    NOT NULL,
    redaction_version INTEGER NOT NULL,
    received_at       INTEGER NOT NULL
) STRICT;

-- Deterministic extraction. The schema slot exists and 1.0 writes nothing to it
-- (spec 5.8); no gate depends on it.
CREATE TABLE observations (
    id         TEXT    NOT NULL PRIMARY KEY,
    project_id TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    event_id   TEXT    NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL,
    body       TEXT    NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

-- Curated memory. Also an unwritten slot in 1.0 (spec 5.8). The UNIQUE below is
-- the conflict target writes will need: INSERT OR REPLACE is banned here because
-- it deletes and reinserts, which fires cascades and loses the rowid, so a write
-- has to be ON CONFLICT (project_id, key) DO UPDATE and that needs the index.
CREATE TABLE memory_items (
    id         TEXT    NOT NULL PRIMARY KEY,
    project_id TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    key        TEXT    NOT NULL,
    body       TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (project_id, key)
) STRICT;

-- +goose Down

DROP TABLE memory_items;
DROP TABLE observations;
DROP TABLE events;
DROP TABLE sessions;
DROP TABLE projects;
