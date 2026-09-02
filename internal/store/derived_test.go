package store_test

import (
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/store"
)

// The three derived columns are read out of a payload by rules, and every rule
// has to have an exact SQL equivalent, because migration 00005's backfill runs
// the same rules over the rows that predate the column. TestTheTwoDerivedWalksAgree
// is what holds them together; this file is what says what the rules are.
//
// Every case asserts the whole [store.Derived] value and not one field of it,
// so a rule that starts writing into the wrong column fails here rather than in
// whatever measures that column later.
func TestDeriveReadsTheThreeColumnsOutOfAPayload(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		payload string
		want    store.Derived
	}{
		{
			name:    "a Bash tool call carries a command and nothing else",
			payload: `{"tool_name":"Bash","tool_input":{"command":"go test -p 1 ./...","description":"run the suite"}}`,
			want:    store.Derived{Cmd: "go test -p 1 ./..."},
		},
		{
			name:    "a Write tool call carries the path it is about to touch",
			payload: `{"tool_name":"Write","tool_input":{"file_path":"C:/x/derived.go","content":"package store"}}`,
			want:    store.Derived{Paths: "C:/x/derived.go"},
		},
		{
			name:    "a response that names the file it wrote carries the path",
			payload: `{"hook_event_name":"PostToolUse","tool_response":{"filePath":"C:/x/derived.go","type":"create"}}`,
			want:    store.Derived{Paths: "C:/x/derived.go"},
		},
		{
			name:    "the input path wins over the response path, because they are the same path",
			payload: `{"tool_input":{"file_path":"C:/from/input.go"},"tool_response":{"filePath":"C:/from/response.go"}}`,
			want:    store.Derived{Paths: "C:/from/input.go"},
		},
		{
			name:    "a command's output is the output column",
			payload: `{"tool_input":{"command":"go build ./..."},"tool_response":{"stdout":"ok\ndone","stderr":"","interrupted":false}}`,
			want:    store.Derived{Cmd: "go build ./...", Output: "ok\ndone"},
		},
		{
			name:    "stdout wins over content when a response carries both",
			payload: `{"tool_response":{"stdout":"from stdout","content":"from content"}}`,
			want:    store.Derived{Output: "from stdout"},
		},
		{
			name:    "an empty stdout falls through to content",
			payload: `{"tool_response":{"stdout":"","content":"from content"}}`,
			want:    store.Derived{Output: "from content"},
		},
		{
			name:    "a response that is itself a string is the output",
			payload: `{"hook_event_name":"PostToolUse","tool_response":"the whole answer"}`,
			want:    store.Derived{Output: "the whole answer"},
		},
		{
			name:    "an object response with neither field yields no output, not the object's JSON",
			payload: `{"tool_response":{"userModified":false,"structuredPatch":[]}}`,
			want:    store.Derived{},
		},
		{
			name:    "a list response yields no output, not the array's JSON",
			payload: `{"tool_response":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}`,
			want:    store.Derived{},
		},
		{
			name:    "a command that is not a string is not a command",
			payload: `{"tool_input":{"command":42}}`,
			want:    store.Derived{},
		},
		{
			name:    "a path that is not a string is not a path",
			payload: `{"tool_input":{"file_path":["a","b"]}}`,
			want:    store.Derived{},
		},
		{
			name:    "an empty command is the same as no command",
			payload: `{"tool_input":{"command":""}}`,
			want:    store.Derived{},
		},
		{
			name:    "a payload with none of the three yields all three empty",
			payload: `{"hook_event_name":"SessionStart","session_id":"s","cwd":"C:/x"}`,
			want:    store.Derived{},
		},
		{
			name:    "a payload that is not valid JSON yields all three empty",
			payload: `{"tool_input":{"command":"go test"`,
			want:    store.Derived{},
		},
		{
			name:    "two JSON values are not one payload, so nothing is derived",
			payload: `{"tool_input":{"command":"a"}}{"tool_input":{"command":"b"}}`,
			want:    store.Derived{},
		},
		{
			name:    "a bare JSON string is a payload with nothing in it to read",
			payload: `"just a string"`,
			want:    store.Derived{},
		},
		{
			// SQLite stops at 1000 open containers and Go stops at
			// 10000, so this payload is derivable to encoding/json and
			// refused by json_valid - which is what the backfill guards
			// on. Without sqliteWillParse the command below would be in
			// the column here and absent there, for one row, with
			// nothing saying so.
			name:    "a shallow command beside a nesting SQLite refuses is derived by neither walk",
			payload: `{"tool_input":{"command":"go test"},"deep":` + nestedArrays(1001) + `}`,
			want:    store.Derived{},
		},
		{
			name:    "a shallow command beside a nesting SQLite still accepts is derived",
			payload: `{"tool_input":{"command":"go test"},"deep":` + nestedArrays(996) + `}`,
			want:    store.Derived{Cmd: "go test"},
		},
		{
			name:    "the three columns are independent and all three can be set at once",
			payload: `{"tool_input":{"command":"cat x.go","file_path":"C:/x/x.go"},"tool_response":{"stdout":"package x"}}`,
			want: store.Derived{
				Cmd:    "cat x.go",
				Paths:  "C:/x/x.go",
				Output: "package x",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := store.Derive([]byte(tc.payload))
			if got != tc.want {
				t.Fatalf("Derive(%s)\n got %#v\nwant %#v", tc.name, got, tc.want)
			}
		})
	}
}

// nestedArrays returns depth arrays nested inside one another, which is the
// cheapest shape that reaches a container depth. The two callers above sit
// either side of SQLite's limit of 1000 open containers; the outer object and
// tool_input account for two of them, so 996 leaves room and 1001 does not.
func nestedArrays(depth int) string {
	return strings.Repeat("[", depth) + strings.Repeat("]", depth)
}
