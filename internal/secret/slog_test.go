package secret_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
)

// logged runs f against a logger whose JSON handler is wrapped in the egress
// filter, and returns exactly the bytes that reached the writer. Everything
// below asserts on those bytes, because "the redactor was called" and "the
// redactor worked" look identical from anywhere else.
func logged(t *testing.T, f func(l *slog.Logger)) []byte {
	t.Helper()
	var buf bytes.Buffer
	f(slog.New(secret.NewLogHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	return buf.Bytes()
}

// decodeLine parses one log line, failing the test when it does not parse. A
// masked line that no longer parses is the second recorded trap (spec 6.1): a
// \S+ token pattern swallows the closing quote and brace.
func decodeLine(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(out, &rec); err != nil {
		t.Fatalf("the log line does not parse as JSON: %v\n%s", err, out)
	}
	return rec
}

// TestLogHandlerMasksEveryShape is I-10 at the log egress, one class at a time:
// a credential generated at run time goes through the handler in the message
// and in an attribute, and neither survives.
//
// The assertion is the secret's absence from the bytes, not the placeholder's
// presence. A handler that assigns to the slog.Attr its Record.Attrs callback
// hands it - which is by value, and the first recorded trap - passes every
// check that looks for the redactor's footprint and fails this one.
//
// The marker attribute and the message prefix are the other half: absence is
// also what dropping the record produces.
func TestLogHandlerMasksEveryShape(t *testing.T) {
	for _, s := range secrettest.All() {
		t.Run(string(s.Class)+"/"+s.Shape, func(t *testing.T) {
			out := logged(t, func(l *slog.Logger) {
				l.Info("egress: "+s.Value, "detail", s.Value, "keep", "marker-value")
			})
			if bytes.Contains(out, []byte(s.Secret)) {
				t.Fatalf("the %s secret survived the log handler:\n%s", s.Class, out)
			}
			rec := decodeLine(t, out)
			if got := rec["keep"]; got != "marker-value" {
				t.Fatalf("the untouched attribute reads %v, want %q - the record lost content it should keep",
					got, "marker-value")
			}
			msg, _ := rec["msg"].(string)
			if !strings.HasPrefix(msg, "egress: ") {
				t.Fatalf("msg = %q, want it to keep its %q prefix", msg, "egress: ")
			}
			if !strings.Contains(msg, "[redacted-"+string(s.Class)+"]") {
				t.Fatalf("msg = %q, want it to carry the %s placeholder", msg, s.Class)
			}
		})
	}
}

// TestLogHandlerMasksBoundAndGroupedAttributes covers the two paths a record's
// own attributes do not: an attribute bound by WithAttrs before the record
// existed, and one nested inside a group.
func TestLogHandlerMasksBoundAndGroupedAttributes(t *testing.T) {
	s := secrettest.Of(secret.ClassAPIKey)

	for _, tc := range []struct {
		name string
		log  func(l *slog.Logger)
	}{
		{"With", func(l *slog.Logger) { l.With("bound", s.Value).Info("egress") }},
		{"WithGroup", func(l *slog.Logger) { l.WithGroup("g").Info("egress", "nested", s.Value) }},
		{"Group value", func(l *slog.Logger) { l.Info("egress", slog.Group("g", "nested", s.Value)) }},
		{"group inside a bound attribute", func(l *slog.Logger) {
			l.With(slog.Group("g", "nested", s.Value)).Info("egress")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := logged(t, tc.log)
			if bytes.Contains(out, []byte(s.Secret)) {
				t.Fatalf("the secret survived %s:\n%s", tc.name, out)
			}
			if !bytes.Contains(out, []byte("[redacted-api-key]")) {
				t.Fatalf("%s: nothing was masked at all, so the attribute was dropped rather than filtered:\n%s",
					tc.name, out)
			}
			decodeLine(t, out)
		})
	}
}

// TestLogHandlerMasksAValueItsKeyNamesAsACredential is the one case where the
// key decides: a structured payload puts a credential in the value of a key
// that names it, and the value alone carries no shape to match on.
func TestLogHandlerMasksAValueItsKeyNamesAsACredential(t *testing.T) {
	const value = "no-shape-at-all"
	out := logged(t, func(l *slog.Logger) { l.Info("egress", "api_key", value) })

	if bytes.Contains(out, []byte(value)) {
		t.Fatalf("a value under an api_key key survived:\n%s", out)
	}
	if got := decodeLine(t, out)["api_key"]; got != "[redacted-credential]" {
		t.Fatalf("api_key = %v, want %q", got, "[redacted-credential]")
	}
}

// TestLogHandlerLeavesACarriedJSONDocumentParseable is the second recorded trap
// at this egress. A whole hook payload logged as one string is the case that
// matters - tool_response is itself a JSON document for Codex (spec 4.4) - and
// a token pattern that swallowed the closing quote and brace would leave a
// value that no longer parses while the line around it still did.
func TestLogHandlerLeavesACarriedJSONDocumentParseable(t *testing.T) {
	s := secrettest.Of(secret.ClassAPIKey)
	payload := `{"hook_event_name":"PreToolUse","tool_input":{"command":"echo ` + s.Value + `"}}`

	out := logged(t, func(l *slog.Logger) { l.Info("egress", "payload", payload) })
	if bytes.Contains(out, []byte(s.Secret)) {
		t.Fatalf("the secret survived inside the carried document:\n%s", out)
	}

	carried, ok := decodeLine(t, out)["payload"].(string)
	if !ok {
		t.Fatalf("the payload attribute is not a string:\n%s", out)
	}
	var doc struct {
		Event     string `json:"hook_event_name"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal([]byte(carried), &doc); err != nil {
		t.Fatalf("the masked payload no longer parses: %v\n%s", err, carried)
	}
	if doc.Event != "PreToolUse" {
		t.Fatalf("hook_event_name = %q, want %q", doc.Event, "PreToolUse")
	}
	if doc.ToolInput.Command != "echo [redacted-api-key]" {
		t.Fatalf("command = %q, want %q", doc.ToolInput.Command, "echo [redacted-api-key]")
	}
}

// TestLogHandlerKeepsWhatIsNotASecret pins the cost of filtering everything:
// values that matched nothing must reach the handler below with their own
// kinds, not as strings. A filter that stringified every value would pass every
// absence assertion above and quietly change every log line.
func TestLogHandlerKeepsWhatIsNotASecret(t *testing.T) {
	rec := decodeLine(t, logged(t, func(l *slog.Logger) {
		l.Info("egress", "count", 3, "ok", true, "took", 250*time.Millisecond, "name", "pipe")
	}))

	if got := rec["count"]; got != float64(3) {
		t.Errorf("count = %#v, want the number 3", got)
	}
	if got := rec["ok"]; got != true {
		t.Errorf("ok = %#v, want the boolean true", got)
	}
	if got := rec["took"]; got != float64(250*time.Millisecond) {
		t.Errorf("took = %#v, want the duration as a number", got)
	}
	if got := rec["name"]; got != "pipe" {
		t.Errorf("name = %#v, want %q", got, "pipe")
	}
}

// TestLogHandlerDelegatesEnabled keeps the wrapper from turning a disabled
// level into a written line: Enabled is the handler below's decision.
func TestLogHandlerDelegatesEnabled(t *testing.T) {
	var buf bytes.Buffer
	h := secret.NewLogHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled(Info) = true under a Warn handler, want false")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled(Error) = false under a Warn handler, want true")
	}

	slog.New(h).Info("dropped")
	if buf.Len() != 0 {
		t.Errorf("a disabled record was written: %s", buf.Bytes())
	}
}
