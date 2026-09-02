-- Step 3, the native memory index (memory spec rev.2, M-2). As with 00001 and
-- 00002, this file is the truth: the design documents hold the decisions and no
-- DDL.
--
-- This replaces the Phase 1 memory_items rather than adding a table beside it,
-- and the drop is safe for a reason that was checked rather than assumed: no
-- shipped code has ever written that table. Its only references are in
-- internal/store/migrate_test.go. So the drop cannot lose a row on any
-- installation, and the new index needs no rebuild - it is created over a table
-- that is empty by construction.
--
-- What the old shape could not hold, measured on the owner's machine on
-- 2026-09-02 and written up in the memory spec's M-2:
--
--   * No host column, so UNIQUE (project_id, key) makes Claude Code and Codex
--     collide on any key they share. Both hosts write a file called MEMORY.md.
--   * project_id NOT NULL REFERENCES projects, which neither host's memory can
--     satisfy. Claude Code keys a memory directory on a git root - of the three
--     that exist on that machine, none is a project this database has a row
--     for - and Codex's memory is global with a per-entry cwd that routinely
--     names a directory no event ever came from.
--
-- project_id is therefore a nullable convenience and project_path is what
-- scoping actually compares (M-2 decision 8). It is ON DELETE SET NULL and not
-- CASCADE, which makes it the one foreign key to projects that does not cascade:
-- a cascade here would be a promise the code cannot keep, because the memory
-- file is the host's and stays on disk, so the next collection tick re-indexes
-- exactly the row the cascade deleted.
--
-- host_modified_at is the host's own timestamp and is nullable, which is not
-- defensive: 1 of the 18 Claude Code notes on that machine carries no `modified`
-- key at all, and that field is what spec 3's P3 compares against. NULL means
-- the host wrote none. indexed_at is ours and is always known.
--
-- entry_key is the block within source_path, and it is '' for an item that is a
-- whole file - which is what the parser produces when it does not recognise the
-- format's own delimiters (M-2 decision 2). So the fallback is a value in this
-- column rather than a second table.
--
-- privacy_class and redaction_version carry what they carry on events: a memory
-- note is full of paths, and I-10 applies to it identically - the original text
-- is stored and masking happens at egress.

-- +goose Up

DROP TABLE memory_items;

CREATE TABLE memory_items (
    id                TEXT    NOT NULL PRIMARY KEY,
    host              TEXT    NOT NULL CHECK (host IN ('claude-code', 'codex')),
    kind              TEXT    NOT NULL,
    source_path       TEXT    NOT NULL,
    entry_key         TEXT    NOT NULL,
    project_path      TEXT    NOT NULL,
    project_id        TEXT             REFERENCES projects (id) ON DELETE SET NULL,
    title             TEXT    NOT NULL,
    body              TEXT    NOT NULL,
    host_modified_at  INTEGER,
    privacy_class     TEXT    NOT NULL,
    redaction_version INTEGER NOT NULL,
    indexed_at        INTEGER NOT NULL,
    UNIQUE (host, source_path, entry_key)
) STRICT;

-- The scoping predicate reads project_path and nothing else (M-2 decision 8), so
-- this is the index that statement needs. It is not covering and does not need
-- to be: the rows it selects are then read through the primary key, and there
-- are hundreds of them rather than the tens of thousands events has.
CREATE INDEX memory_items_by_project ON memory_items (project_path);

-- External content, on the same terms as events_fts and for the same reasons:
-- the index stores no second copy of the text, an FTS row's rowid is the
-- memory_items rowid, and the FTS column must be named for the content table's
-- column, which is why it is body on both sides.
--
-- One indexed column. The title is a display line cut from the same text the
-- body holds, so indexing it too would index the same words twice and change
-- nothing a query can reach.
--
-- The tokenizer clause is the events index's, word for word. M3 compares recall
-- between the two indexes, and a comparison across indexes that tokenise
-- differently measures the tokenizer instead of the corpus. No prefix index
-- here either - spec 5.7 priced that one at 2.7x the index for 0.15 ms, and this
-- index is three orders of magnitude smaller than the one that ruling was made
-- against.
CREATE VIRTUAL TABLE memory_fts USING fts5(
    body,
    content = 'memory_items',
    tokenize = 'unicode61 remove_diacritics 2'
);

-- No rebuild: the table above was created empty by this same migration.

INSERT INTO memory_fts(memory_fts, rank) VALUES('secure-delete', 1);

-- The external-content recipe from fts5.html 4.4.3, the same one 00002 uses.
-- The delete command carries the old value explicitly because an AFTER trigger
-- can no longer read it out of the content table.

-- +goose StatementBegin
CREATE TRIGGER memory_items_ai AFTER INSERT ON memory_items BEGIN
    INSERT INTO memory_fts(rowid, body) VALUES (new.rowid, new.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER memory_items_ad AFTER DELETE ON memory_items BEGIN
    INSERT INTO memory_fts(memory_fts, rowid, body) VALUES ('delete', old.rowid, old.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER memory_items_au AFTER UPDATE ON memory_items BEGIN
    INSERT INTO memory_fts(memory_fts, rowid, body) VALUES ('delete', old.rowid, old.body);
    INSERT INTO memory_fts(rowid, body) VALUES (new.rowid, new.body);
END;
-- +goose StatementEnd

-- +goose Down

DROP TRIGGER memory_items_au;
DROP TRIGGER memory_items_ad;
DROP TRIGGER memory_items_ai;
DROP TABLE memory_fts;
DROP INDEX memory_items_by_project;
DROP TABLE memory_items;

CREATE TABLE memory_items (
    id         TEXT    NOT NULL PRIMARY KEY,
    project_id TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    key        TEXT    NOT NULL,
    body       TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (project_id, key)
) STRICT;
