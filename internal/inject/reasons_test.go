package inject_test

import (
	"fmt"
	"testing"

	"github.com/wotjr1649/engramux/internal/inject"
)

func TestBuildReportsActualExclusions(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		own, command, survivor bool
		want                   string
	}{
		{"no match", false, false, false, inject.ReasonNoHits},
		{"prompt only", true, false, false, "matching prompt event excluded"},
		{"command only", false, true, false, "matching engramux commands excluded"},
		{"both filters", true, true, false, "matching prompt event excluded; matching engramux commands excluded"},
		{"survivor", true, true, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			own := event("UserPromptSubmit", "quaternion", "")
			var docs []seeded
			if tc.own {
				docs = append(docs, own)
			}
			if tc.command {
				docs = append(docs, event("PostToolUse", "quaternion", "engramux search quaternion"))
			}
			if tc.survivor {
				docs = append(docs, event("Stop", "quaternion", ""))
			}
			db := corpus(t, docs...)
			got, err := inject.Build(t.Context(), db, inject.Request{Prompt: "quaternion", ExcludeID: own.id})
			if err != nil {
				t.Fatal(err)
			}
			if got.Reason != tc.want {
				t.Fatalf("reason=%q want=%q", got.Reason, tc.want)
			}
			if tc.survivor {
				if len(got.Events) != 1 || got.Events[0] != docs[len(docs)-1].id || got.Text == "" {
					t.Fatal("surviving event changed")
				}
			} else if got.Text != "" || len(got.Events) != 0 || len(got.Memory) != 0 {
				t.Fatal("abstention emitted data")
			}
		})
	}
}

func TestBuildReportsMixedSuppression(t *testing.T) {
	for _, tc := range []struct {
		name             string
		events, memories int
		want             string
	}{
		{"memory ceiling and exclusions", 0, 201, "memory: " + inject.ReasonTooBroad + "; " + inject.ReasonOwnPrompt + "; " + inject.ReasonOwnCommands},
		{"event ceiling before filters", 201, 0, "events: " + inject.ReasonTooBroad},
		{"both ceilings", 201, 201, "events: " + inject.ReasonTooBroad + "; memory: " + inject.ReasonTooBroad},
		{"event survivor", 1, 201, ""},
		{"memory survivor", 201, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			own := event("UserPromptSubmit", "quaternion", "")
			docs := []seeded{own, event("PostToolUse", "quaternion", "engramux search quaternion")}
			for range tc.events {
				docs = append(docs, event("Stop", "quaternion", ""))
			}
			db := corpus(t, docs...)
			for i := range tc.memories {
				_, err := db.ExecContext(t.Context(), `INSERT INTO memory_items (id,host,kind,source_path,entry_key,project_path,project_id,title,body,host_modified_at,privacy_class,redaction_version,indexed_at) VALUES (?,'codex','entry','synthetic',?,'',NULL,'title','quaternion',0,'',1,0)`, fmt.Sprintf("m%d", i), fmt.Sprintf("k%d", i))
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := inject.Build(t.Context(), db, inject.Request{Prompt: "quaternion", ExcludeID: own.id})
			if err != nil {
				t.Fatal(err)
			}
			if got.Reason != tc.want {
				t.Fatalf("reason=%q want=%q", got.Reason, tc.want)
			}
			if tc.want != "" {
				if got.Text != "" || len(got.Events)+len(got.Memory) != 0 {
					t.Fatal("suppressed result emitted data")
				}
			} else if tc.events == 1 {
				if got.Text == "" || len(got.Events) != 1 || got.Events[0] != docs[2].id || len(got.Memory) != 0 {
					t.Fatal("event survivor changed")
				}
			} else if got.Text == "" || len(got.Events) != 0 || len(got.Memory) != 1 || got.Memory[0] != "m0" {
				t.Fatal("memory survivor changed")
			}
		})
	}
}
