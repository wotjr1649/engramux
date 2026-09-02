-- Backlog 34 (spec 7.1's read-deadline row): the one index the events table
-- carries beyond its primary key, and it exists for exactly one statement -
-- store.CellsQuery, the status reply's per-cell breakdown.
--
-- Covering on purpose. The breakdown groups by (host, event_name) and takes
-- min and max of received_at in each group. An index on the two grouping
-- columns alone orders the groups but leaves received_at in the table, so the
-- planner still visits every row of a b-tree whose rows carry the payload and
-- its leaves - the same pages a full scan reads, which is what the Phase 6
-- soak measured refusing nine status reads in 147 samples against a 150-180 MB
-- file. With received_at as the third column the whole statement is answered
-- from the index and the table's b-tree is never opened; cell_index_test.go
-- holds the plan to that, line for line.
--
-- Nothing else changes: no table, no FTS rebuild, no backfill. The index is
-- built by reading the table once, at the first start on this version.

-- +goose Up
CREATE INDEX events_by_cell ON events (host, event_name, received_at);

-- +goose Down
DROP INDEX events_by_cell;
