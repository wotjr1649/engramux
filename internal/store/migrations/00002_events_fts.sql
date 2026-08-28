-- Phase 4 search index (spec 5.7). As with 00001, this file is the truth: the
-- design document holds the decisions and no DDL.
--
-- events_fts is an *external content* table. It stores the index and no copy of
-- the text: `content='events'` points it at the base table, and an FTS row's
-- rowid is the events rowid - which exists because events is an ordinary rowid
-- table. That is not an accident of STRICT. A TEXT PRIMARY KEY under STRICT is
-- still a rowid table, and events must never become WITHOUT ROWID: SQLite
-- accepts `content=` on one at CREATE and fails at the first `rebuild`.
--
-- Because the index keys on events' implicit rowid, nothing may renumber it.
-- Nothing in 1.0 runs `VACUUM`, and SQLite documents that VACUUM may renumber
-- the rows of a table with no INTEGER PRIMARY KEY - which events is. A future
-- VACUUM, or a textual dump-and-restore, must therefore be followed by a
-- `rebuild`; `integrity-check` with rank=1 is what detects the desync it would
-- otherwise leave behind.
--
-- External content requires the FTS column names to be the content table's
-- column names, which is why the indexed column is called `leaves` on both
-- sides.
--
-- What it holds is the *string leaves* of the payload and not the payload,
-- because indexing the raw JSON indexes the structure and a key is then a token
-- of nearly every document. Spec 5.7 holds that measurement and the numbers.
-- The text is the original and unmasked, which is I-10 working as designed - the
-- database keeps the original and masking happens at egress - and an index of
-- masked text could not be kept in sync with the table `rebuild` reads anyway.
--
-- One column, and no `project_id UNINDEXED` beside it. It looked like it would
-- let a project-scoped search filter inside the MATCH; the query plan says it
-- does not. A project-scoped search joins events on rowid and filters
-- events.project_id instead (spec 5.7).

-- +goose Up

-- Nullable, and the distinction is load-bearing: NULL means "not computed",
-- which only the backfill below ever sees, and '' means "this payload has no
-- string leaves", which is an ordinary answer for `42` or `{}`.
ALTER TABLE events ADD COLUMN leaves TEXT;

-- The same walk store.Leaves does in Go, in SQL, for the rows that were here
-- before the column was: every string leaf of the payload, in document order,
-- newline-separated. json_tree's `id` is document order through nested objects
-- and arrays, and `type = 'text'` selects the string values - an object key is
-- json_tree's `key` column and never a row of its own, so structure is skipped
-- here exactly as it is in Go. TestTheTwoWalksAgree holds the two together over
-- the fixtures and every captured payload; the separator is char(10) on this
-- side and store.leafSeparator on the other.
--
-- coalesce, because group_concat over no rows is NULL rather than '': a payload
-- with no string leaves must read as the empty answer, not as "not computed".
--
-- json_valid, because json_tree raises `malformed JSON` rather than returning
-- nothing, and events.payload carries no CHECK that it is JSON - validity is a
-- property of the path in, not of the column. One unparseable row would
-- otherwise abort the migration and leave the service unable to start, which is
-- a far worse failure than one unsearchable event. store.Leaves answers "" for
-- the same bytes.
UPDATE events
SET leaves = CASE
    WHEN json_valid(payload) THEN coalesce(
        (SELECT group_concat(value, char(10) ORDER BY id)
         FROM json_tree(events.payload)
         WHERE type = 'text'), '')
    ELSE ''
END;

-- No prefix index. Measured rather than argued: it takes a two-character Korean
-- prefix query at 18,020 events from 0.79 ms to 0.64 ms, and costs 2.8x the
-- index to do it (spec 5.7). A later migration plus a `rebuild` adds it if a
-- real workload ever wants it back.
CREATE VIRTUAL TABLE events_fts USING fts5(
    leaves,
    content = 'events',
    tokenize = 'porter unicode61 remove_diacritics 2'
);

-- Index the rows that were already here, after the backfill has given them
-- something to index. Only this runs over them: the triggers below see nothing
-- written before they existed, so without a rebuild the live installation's
-- captures would stay unsearchable forever and no check would say so.
INSERT INTO events_fts(events_fts) VALUES('rebuild');

-- The index holds the same unredacted text the table does and has its own
-- switch for it (spec 5.7). Set after the rebuild, so nothing depends on
-- whether a rebuild preserves the %_config row.
INSERT INTO events_fts(events_fts, rank) VALUES('secure-delete', 1);

-- The external-content recipe from fts5.html 4.4.3. The delete command carries
-- the *old* values explicitly, and that is the mechanism rather than a nicety:
-- a plain delete against an external-content index reads the old values back
-- out of the content table, and in an AFTER trigger they are already gone. The
-- index would then keep tokens for text that no longer exists, which is what
-- `integrity-check` with rank=1 catches.
--
-- The delete command takes one value per column, which here is one value.
--
-- +goose StatementBegin
CREATE TRIGGER events_ai AFTER INSERT ON events BEGIN
    INSERT INTO events_fts(rowid, leaves) VALUES (new.rowid, new.leaves);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER events_ad AFTER DELETE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, leaves)
    VALUES ('delete', old.rowid, old.leaves);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER events_au AFTER UPDATE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, leaves)
    VALUES ('delete', old.rowid, old.leaves);
    INSERT INTO events_fts(rowid, leaves) VALUES (new.rowid, new.leaves);
END;
-- +goose StatementEnd

-- +goose Down

DROP TRIGGER events_au;
DROP TRIGGER events_ad;
DROP TRIGGER events_ai;
DROP TABLE events_fts;
ALTER TABLE events DROP COLUMN leaves;
