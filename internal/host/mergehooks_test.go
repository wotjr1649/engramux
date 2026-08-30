package host

import (
	"encoding/json/jsontext"
	"strings"
	"testing"
)

// probeEntry is the ENTRY the tests install, which is what [MergeHooks] appends
// - an object carrying a matcher and a hooks array, not a bare hook. The
// distinction is the one a caller gets wrong: the matcher is a per-event value
// the event table owns, so the entry is built outside and written verbatim.
//
// Its shape is otherwise the smallest thing carrying the marker [isEngramux]
// looks for.
func probeEntry(event string) jsontext.Value {
	return jsontext.Value(`{"matcher":"*","hooks":[` +
		`{"type":"command","command":"C:/x/engramux.exe","event":"` + event + `"}]}`)
}

// twoEvents is the event set these tests merge, small enough that the expected
// documents can be written out whole.
var twoEvents = []string{"SessionStart", "Stop"}

// TestMergeHooksLeavesEverythingElseByteIdentical is the property the port
// exists for.
//
// The script this replaces round-trips the document through JSON.parse and
// JSON.stringify, which preserves key order but normalises escapes and number
// spellings. Go's encoding/json would be worse again: it sorts a map's keys, so
// a user's settings file would come back alphabetised. The document here is
// built to fail under either - keys out of alphabetical order, a number written
// with a trailing zero, an escape that denotes a printable character, and a
// character encoding/json escapes for HTML by default.
func TestMergeHooksLeavesEverythingElseByteIdentical(t *testing.T) {
	bs := string([]byte{92}) // one backslash, assembled so no layer rewrites it
	src := `{
  "zzz": "last key, first in the file",
  "num": 1.500,
  "esc": "` + bs + `u00e9",
  "html": "a <b> c",
  "hooks": {
    "Stop": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "someone-elses-tool.exe"
          }
        ]
      }
    ]
  },
  "aaa": "after hooks"
}
`

	got, err := MergeHooks([]byte(src), twoEvents, probeEntry)
	if err != nil {
		t.Fatalf("MergeHooks: %v", err)
	}

	for _, keep := range []string{
		`"zzz": "last key, first in the file"`,
		`"num": 1.500`,
		`"esc": "` + bs + `u00e9"`,
		`"html": "a <b> c"`,
		`"command": "someone-elses-tool.exe"`,
	} {
		if !strings.Contains(string(got), keep) {
			t.Errorf("MergeHooks did not preserve %s\ngot:\n%s", keep, got)
		}
	}

	// Key order, asserted as order and not as presence: zzz before num before
	// esc, and aaa after hooks. An alphabetising encoder passes every Contains
	// check above and fails this one.
	for _, pair := range [][2]string{
		{`"zzz"`, `"num"`}, {`"num"`, `"esc"`}, {`"esc"`, `"html"`},
		{`"html"`, `"hooks"`}, {`"hooks"`, `"aaa"`},
	} {
		if i, j := strings.Index(string(got), pair[0]), strings.Index(string(got), pair[1]); i < 0 || j < 0 || i > j {
			t.Errorf("key order lost: %s should come before %s\ngot:\n%s", pair[0], pair[1], got)
		}
	}
}

// TestMergeHooksIsIdempotent holds the property the script gets from dropping
// every Engramux entry before adding one back. A merge that appended would grow
// the file on every re-run, and a re-run with no rebuild is the common case.
func TestMergeHooksIsIdempotent(t *testing.T) {
	const src = `{"hooks":{}}`

	once, err := MergeHooks([]byte(src), twoEvents, probeEntry)
	if err != nil {
		t.Fatalf("first MergeHooks: %v", err)
	}
	twice, err := MergeHooks(once, twoEvents, probeEntry)
	if err != nil {
		t.Fatalf("second MergeHooks: %v", err)
	}
	if string(once) != string(twice) {
		t.Errorf("MergeHooks is not idempotent\nfirst:\n%s\nsecond:\n%s", once, twice)
	}
	if n := strings.Count(string(once), `"engramux.exe"`); n != 0 {
		t.Errorf("probe hook spelling changed; the count assertion below is measuring nothing")
	}
	if n := strings.Count(string(twice), "engramux.exe"); n != len(twoEvents) {
		t.Errorf("after two merges there are %d engramux hooks, want %d - one per event",
			n, len(twoEvents))
	}
}

// TestMergeHooksKeepsWhatItDidNotWrite covers the three entry shapes the script
// distinguishes and a Go port is likely to collapse.
func TestMergeHooksKeepsWhatItDidNotWrite(t *testing.T) {
	src := `{"hooks":{"Stop":[` +
		// An entry holding only ours: it goes away entirely.
		`{"matcher":"*","hooks":[{"type":"command","command":"c:/engramux.exe"}]},` +
		// An entry holding ours and someone else's: theirs survives, in place.
		`{"matcher":"a","hooks":[{"type":"command","command":"c:/engramux.exe"},{"type":"command","command":"theirs.exe"}]},` +
		// An entry with no hooks key at all: not ours to judge, kept as is.
		`{"matcher":"b","note":"no hooks key"}` +
		`]}}`

	got, err := MergeHooks([]byte(src), []string{"Stop"}, probeEntry)
	if err != nil {
		t.Fatalf("MergeHooks: %v", err)
	}
	s := string(got)

	if strings.Count(s, `"theirs.exe"`) != 1 {
		t.Errorf("the foreign hook did not survive exactly once\ngot:\n%s", s)
	}
	if !strings.Contains(s, `"no hooks key"`) {
		t.Errorf("an entry with no hooks key was dropped\ngot:\n%s", s)
	}
	if strings.Count(s, `"matcher": "*"`) != 1 {
		t.Errorf(`the entry that held only ours should be gone and one new "*" entry written; got:\n%s`, s)
	}
	// One engramux hook, not two: the old one is dropped before the new one is
	// added, and the entry that held only it does not survive empty.
	if n := strings.Count(s, "engramux.exe"); n != 1 {
		t.Errorf("%d engramux hooks after a merge over an existing install, want 1\ngot:\n%s", n, s)
	}
}

// TestMergeHooksCreatesTheHooksObject covers a configuration file that has
// never had a hook in it, which is every first install.
func TestMergeHooksCreatesTheHooksObject(t *testing.T) {
	got, err := MergeHooks([]byte(`{"model":"opus"}`), twoEvents, probeEntry)
	if err != nil {
		t.Fatalf("MergeHooks: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, `"model": "opus"`) {
		t.Errorf("the existing document was lost\ngot:\n%s", s)
	}
	for _, event := range twoEvents {
		if !strings.Contains(s, `"`+event+`"`) {
			t.Errorf("event %s was not written\ngot:\n%s", event, s)
		}
	}
	if !jsontext.Value(s).IsValid() {
		t.Errorf("the result is not valid JSON:\n%s", s)
	}
}

// TestMergeHooksRemoves is the --remove half. Passing no hook to write is how
// removal is spelled, so the two paths cannot drift apart.
func TestMergeHooksRemoves(t *testing.T) {
	installed, err := MergeHooks([]byte(`{"hooks":{}}`), twoEvents, probeEntry)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	removed, err := MergeHooks(installed, twoEvents, nil)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if strings.Contains(string(removed), "engramux.exe") {
		t.Errorf("a hook survived removal\ngot:\n%s", removed)
	}
}

// TestIndentingChangesOnlyWhitespace is the measurement [MergeHooks]'s comment
// rests on, and without it that comment is an assertion about a standard
// library this project does not control.
//
// The document carries one of each thing an encoder is likely to normalise:
// members out of alphabetical order, a number with a trailing zero, an escape
// denoting a printable character, and a character encoding/json escapes for
// HTML. Indenting has to move whitespace and nothing else, and compacting has
// to give the original back byte for byte.
func TestIndentingChangesOnlyWhitespace(t *testing.T) {
	bs := string([]byte{92})
	src := `{"b":1,"a":{"z":[1,2],"y":"` + bs + `u00e9"},"n":1.500,"h":"<script>"}`

	indented := jsontext.Value(src)
	if err := indented.Indent(jsontext.WithIndent("  ")); err != nil {
		t.Fatalf("Indent: %v", err)
	}
	for _, keep := range []string{`"b"`, `"` + bs + `u00e9"`, `1.500`, `"<script>"`} {
		if !strings.Contains(string(indented), keep) {
			t.Errorf("Indent did not preserve %s\ngot:\n%s", keep, indented)
		}
	}
	if i, j := strings.Index(string(indented), `"b"`), strings.Index(string(indented), `"a"`); i > j {
		t.Errorf("Indent reordered members\ngot:\n%s", indented)
	}

	compacted := jsontext.Value(indented).Clone()
	if err := compacted.Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if string(compacted) != src {
		t.Errorf("indent then compact is not the identity\n got: %s\nwant: %s", compacted, src)
	}
}
