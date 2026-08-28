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
-- External content requires the FTS column names to be the content table's
-- column names, which is why the indexed column is called `payload`. It is the
-- raw event bytes, unmasked, and that is I-10 working as designed: the database
-- keeps the original and masking happens at egress. An index of masked text
-- could not be kept in sync with the table `rebuild` reads.
--
-- One column, and no `project_id UNINDEXED` beside it. It looked like it would
-- let a project-scoped search filter inside the MATCH; the query plan says it
-- does not. A project-scoped search joins events on rowid and filters
-- events.project_id instead (spec 5.7).

-- +goose Up

-- No prefix index. Measured rather than argued: it takes a two-character Korean
-- prefix query at 18,020 events from 0.79 ms to 0.64 ms, and costs 2.8x the
-- index to do it (spec 5.7). A later migration plus a `rebuild` adds it if a
-- real workload ever wants it back.
CREATE VIRTUAL TABLE events_fts USING fts5(
    payload,
    content = 'events',
    tokenize = 'porter unicode61 remove_diacritics 2'
);

-- Index the rows that were already here. Only this runs over them: the triggers
-- below see nothing written before they existed, so without a rebuild the live
-- installation's captures would stay unsearchable forever and no check would
-- say so.
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
    INSERT INTO events_fts(rowid, payload) VALUES (new.rowid, new.payload);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER events_ad AFTER DELETE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, payload)
    VALUES ('delete', old.rowid, old.payload);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER events_au AFTER UPDATE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, payload)
    VALUES ('delete', old.rowid, old.payload);
    INSERT INTO events_fts(rowid, payload) VALUES (new.rowid, new.payload);
END;
-- +goose StatementEnd

-- +goose Down

DROP TRIGGER events_au;
DROP TRIGGER events_ad;
DROP TRIGGER events_ai;
DROP TABLE events_fts;
