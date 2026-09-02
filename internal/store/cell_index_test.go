package store

import (
	"slices"
	"testing"
)

// TestTheCellQueryReadsOnlyTheCellIndex is backlog 34. The status reply's
// per-cell breakdown groups the whole events table by (host, event_name) and
// takes the span of received_at in each group, and until migration 00003 that
// was a full scan of a b-tree whose rows carry the payload and its leaves - so
// the cost of a `status` was the size of the file, and the soak measured nine
// refused reads in 147 samples against a 150-180 MB database (spec 7.1).
//
// The index has to be *covering*: an index on (host, event_name) alone orders
// the groups but every row's received_at is still fetched from the table, and
// that visits the same pages a full scan does. With received_at as the third
// column the planner answers the whole query from the index and never opens
// the table's b-tree, which is what "USING COVERING INDEX" says and what this
// test holds to the exact plan line. A second line - a temp b-tree for the
// GROUP BY - would mean the index order stopped matching the grouping.
//
// The statement is [CellsQuery] itself rather than a copy of it, so the plan
// under test is the plan the service runs.
func TestTheCellQueryReadsOnlyTheCellIndex(t *testing.T) {
	db := migrated(t)
	rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+CellsQuery)
	if err != nil {
		t.Fatalf("explain the cell query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan a plan line: %v", err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the plan: %v", err)
	}
	want := []string{"SCAN events USING COVERING INDEX events_by_cell"}
	if !slices.Equal(plan, want) {
		t.Fatalf("query plan = %q, want %q", plan, want)
	}
}
