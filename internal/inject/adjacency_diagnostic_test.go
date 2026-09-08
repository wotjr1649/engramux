package inject_test

import (
	"testing"
)

// Receive-order proximity is not a host turn relationship. These synthetic
// counterexamples reject that proposed heuristic without adding it to Build.
func TestReceiveOrderDoesNotEstablishPromptAnswerLinkage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		order    []string
		selected string
	}{
		{"late second prompt mispairs its answer", []string{"p1", "s2", "p2", "s1"}, "s2"},
		{"late first answer is missed", []string{"p1", "p2", "s1", "s2"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := corpus(t)
			if _, err := db.ExecContext(t.Context(), "CREATE TABLE diagnostic_turns(id TEXT PRIMARY KEY, kind TEXT, received INTEGER)"); err != nil {
				t.Fatal(err)
			}
			for i, id := range tc.order {
				kind := "UserPromptSubmit"
				if id[0] == 's' {
					kind = "Stop"
				}
				if _, err := db.ExecContext(t.Context(), "INSERT INTO diagnostic_turns VALUES(?,?,?)", id, kind, i+1); err != nil {
					t.Fatal(err)
				}
			}
			// All rows represent the same host/project/session. The host's
			// oracle says s1 answers p1; that relationship is not a query input.
			var got string
			if err := db.QueryRowContext(t.Context(), `SELECT COALESCE((
				SELECT id FROM diagnostic_turns WHERE kind='Stop'
				AND received > (SELECT received FROM diagnostic_turns WHERE id='p1')
				AND received < (SELECT min(received) FROM diagnostic_turns WHERE kind='UserPromptSubmit' AND id!='p1')
				AND received < 100 ORDER BY received LIMIT 1), '')`).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.selected {
				t.Fatalf("adjacency result=%q, want demonstrated failure %q", got, tc.selected)
			}
			if got == "s1" {
				t.Fatal("fixture no longer falsifies adjacency")
			}
		})
	}
}
