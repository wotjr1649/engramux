package secret_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
)

// payloadWith builds a hook-shaped payload carrying v at tool_input.command.
// No key here names a credential, so a class the detector reports comes from
// the value and not from the key.
func payloadWith(t *testing.T, v string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "0198f000-0000-7000-8000-00000000abcd",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": v},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

// commandOf reads back tool_input.command from a masked payload.
func commandOf(t *testing.T, raw []byte) string {
	t.Helper()
	var v struct {
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("masked payload does not parse: %v\n%s", err, raw)
	}
	return v.ToolInput.Command
}

func TestVersionIsTheStoredRulesetIdentifier(t *testing.T) {
	if secret.Version != 1 {
		t.Errorf("Version = %d, want 1", secret.Version)
	}
}

// TestDetectClassifiesEveryShape is the per-class gate: a secret generated at
// test runtime is detected and tagged with the right class, for every class.
// The assertion is the exact stored value, not membership, so a rule that fired
// on the wrong class fails here.
func TestDetectClassifiesEveryShape(t *testing.T) {
	for _, s := range secrettest.All() {
		t.Run(string(s.Class)+"/"+s.Shape, func(t *testing.T) {
			if got := secret.Detect(payloadWith(t, s.Value)).String(); got != string(s.Class) {
				t.Errorf("Detect = %q, want exactly %q", got, s.Class)
			}
		})
	}
}

// TestDetectWalksDecodedJSONNotRawBytes is the difference between the two
// designs, made mechanical: neither secret appears in the payload's bytes at
// all, so a raw byte scan reports the payload clean. A Windows path is
// backslash-escaped by every JSON encoder, and the user identity class is the
// one that actually fires - 1,714 matches across 900 of 902 captures (spec
// 6.1) - so a byte scan would miss the class that carries the traffic.
func TestDetectWalksDecodedJSONNotRawBytes(t *testing.T) {
	key := secrettest.Of(secret.ClassAPIKey)
	user := secrettest.Of(secret.ClassUserPath)

	// \u00XX for the first byte of the key: legal JSON a host is free to emit,
	// and invisible to any scan of the encoded bytes.
	escaped := `\u00` + hex.EncodeToString([]byte{key.Value[0]}) + key.Value[1:]
	raw := []byte(`{"transcript_path":"C:\\Users\\` + user.Secret +
		`\\.claude\\t.jsonl","tool_input":{"command":"` + escaped + `"}}`)

	// Establish that a byte scan has nothing to find, or the rest proves nothing.
	if bytes.Contains(raw, []byte(key.Value)) {
		t.Fatalf("the api key is literally present in the encoded bytes; the escape did not take:\n%s", raw)
	}
	if bytes.Contains(raw, []byte(`C:\Users\`)) {
		t.Fatalf("the unescaped path is literally present in the encoded bytes:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte(`C:\\Users\\`)) {
		t.Fatalf("the escaped path is not present either; the fixture is wrong:\n%s", raw)
	}

	if got := secret.Detect(raw).String(); got != "api-key,user-path" {
		t.Errorf("Detect = %q, want %q; detection must walk the decoded JSON, where both secrets exist", got, "api-key,user-path")
	}
}

// TestDetectTreatsKeysAsStructure is the other direction of the same choice. A
// raw byte scan matches inside object keys, and a mask computed from those
// spans would rename a field - which the byte-for-byte round trip forbids.
func TestDetectTreatsKeysAsStructure(t *testing.T) {
	name := secrettest.Of(secret.ClassAPIKey).Value
	raw, err := json.Marshal(map[string]any{name: 1})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(name)) {
		t.Fatalf("the key is not in the bytes; the fixture is wrong:\n%s", raw)
	}
	if got := secret.Detect(raw).String(); got != "" {
		t.Errorf("Detect = %q, want empty: a field name is structure, not content", got)
	}

	// The one exception: a key that names a credential tags its own value,
	// because that is where a structured payload puts the value.
	raw, err = json.Marshal(map[string]any{
		"tool_input": map[string]any{"refresh_token": "abcdefgh", "count": 3},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := secret.Detect(raw).String(); got != "credential" {
		t.Errorf("Detect = %q, want %q for a value under a credential-named key", got, "credential")
	}
	var back struct {
		ToolInput struct {
			RefreshToken string `json:"refresh_token"`
			Count        int    `json:"count"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(secret.Mask(raw), &back); err != nil {
		t.Fatalf("masked payload does not parse: %v", err)
	}
	if back.ToolInput.RefreshToken != "[redacted-credential]" {
		t.Errorf("refresh_token = %q, want %q", back.ToolInput.RefreshToken, "[redacted-credential]")
	}
	if back.ToolInput.Count != 3 {
		t.Errorf("count = %d, want 3; masking must not touch non-string leaves", back.ToolInput.Count)
	}
}

// TestDetectReportsEveryMatchingClassSorted covers the multi-class case the
// stored value has to carry.
func TestDetectReportsEveryMatchingClassSorted(t *testing.T) {
	key := secrettest.Of(secret.ClassAPIKey)
	path := secrettest.Of(secret.ClassUserPath)
	opaque := secrettest.Of(secret.ClassOpaque)
	raw := payloadWith(t, "cp "+path.Value+" /tmp/"+opaque.Value+" # "+key.Value)

	want := "api-key,opaque,user-path"
	if got := secret.Detect(raw).String(); got != want {
		t.Errorf("Detect = %q, want %q", got, want)
	}
}

// TestDetectFallsBackToTextWhenThePayloadIsNotJSON: reporting a payload clean
// because it did not parse is the expensive direction, so it is not an option.
func TestDetectFallsBackToTextWhenThePayloadIsNotJSON(t *testing.T) {
	s := secrettest.Of(secret.ClassPrivateKey)
	raw := []byte("panic: hook wrote this to stderr\n" + s.Value)

	if err := json.Unmarshal(raw, new(any)); err == nil {
		t.Fatal("the input parses as JSON; this test is not exercising the fallback")
	}
	if got := secret.Detect(raw).String(); got != string(secret.ClassPrivateKey) {
		t.Errorf("Detect = %q, want %q", got, secret.ClassPrivateKey)
	}
	if got := secret.Mask(raw); bytes.Contains(got, []byte(s.Secret)) {
		t.Errorf("Mask left the key material in a non-JSON payload:\n%s", got)
	}
}

// TestMaskProducesParseableJSONWithoutTheSecret is gate 3, per shape.
func TestMaskProducesParseableJSONWithoutTheSecret(t *testing.T) {
	for _, s := range secrettest.All() {
		t.Run(string(s.Class)+"/"+s.Shape, func(t *testing.T) {
			got := secret.Mask(payloadWith(t, s.Value))

			var back map[string]any
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatalf("masked payload does not parse: %v\n%s", err, got)
			}
			if bytes.Contains(got, []byte(s.Secret)) {
				t.Errorf("masked payload still carries the generated secret:\n%s", got)
			}
			if back["hook_event_name"] != "PreToolUse" {
				t.Errorf("hook_event_name = %#v, want %q", back["hook_event_name"], "PreToolUse")
			}
			if back["session_id"] != "0198f000-0000-7000-8000-00000000abcd" {
				t.Errorf("session_id = %#v, want it untouched", back["session_id"])
			}
			if !strings.Contains(commandOf(t, got), "[redacted-"+string(s.Class)+"]") {
				t.Errorf("command = %q, want it to carry the %s placeholder", commandOf(t, got), s.Class)
			}
		})
	}
}

// TestMaskKeepsJSONNestedInsideAStringLeafParseable is the recorded failure,
// as a test. tool_response is a string for Codex (spec 4.4) and that string is
// routinely itself a JSON document, so the payload is JSON inside JSON. The
// outer document is re-encoded and so is valid whatever the span was; the inner
// one is just text, and a \S+ token pattern eats its closing quote and brace.
// Spec 6.1 bounds the token as [^\s"',}\]]+ for exactly this reason.
func TestMaskKeepsJSONNestedInsideAStringLeafParseable(t *testing.T) {
	tok := secrettest.Of(secret.ClassAuthorization).Secret
	// No whitespace after the token: \S+ then runs to the end of the document.
	inner := `{"headers":{"Authorization":"Bearer ` + tok + `"},"status":200}`
	raw, err := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUse",
		"tool_response":   inner,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var outer struct {
		ToolResponse string `json:"tool_response"`
	}
	if err := json.Unmarshal(secret.Mask(raw), &outer); err != nil {
		t.Fatalf("masked payload does not parse: %v", err)
	}
	if strings.Contains(outer.ToolResponse, tok) {
		t.Errorf("the bearer token survived masking: %s", outer.ToolResponse)
	}

	var in struct {
		Headers struct {
			Authorization string `json:"Authorization"`
		} `json:"headers"`
		Status int `json:"status"`
	}
	if err := json.Unmarshal([]byte(outer.ToolResponse), &in); err != nil {
		t.Fatalf("the JSON nested in tool_response no longer parses: %v\n%s", err, outer.ToolResponse)
	}
	if want := "Bearer [redacted-authorization]"; in.Headers.Authorization != want {
		t.Errorf("Authorization = %q, want %q", in.Headers.Authorization, want)
	}
	if in.Status != 200 {
		t.Errorf("status = %d, want 200", in.Status)
	}
}

// TestMaskRemovesTheIdentityAndKeepsThePath pins the exact masked bytes for the
// class that carries the traffic. The path is the memory worth keeping; the
// username is the part that must not leave the machine.
func TestMaskRemovesTheIdentityAndKeepsThePath(t *testing.T) {
	s := secrettest.Of(secret.ClassUserPath)
	want := strings.Replace(s.Value, s.Secret, "[redacted-user-path]", 1)
	if want == s.Value {
		t.Fatal("the sample's Secret is not in its Value")
	}
	if got := commandOf(t, secret.Mask(payloadWith(t, s.Value))); got != want {
		t.Errorf("masked command = %q, want %q", got, want)
	}
}

// TestMaskIsIdempotentAndUntagsWhatItMasked: a masked value can pass an egress
// twice, and a placeholder must not be mistaken for the thing it replaced.
func TestMaskIsIdempotentAndUntagsWhatItMasked(t *testing.T) {
	for _, s := range secrettest.All() {
		t.Run(string(s.Class)+"/"+s.Shape, func(t *testing.T) {
			once := secret.Mask(payloadWith(t, s.Value))
			twice := secret.Mask(once)
			if !bytes.Equal(once, twice) {
				t.Errorf("Mask is not idempotent:\n once: %s\ntwice: %s", once, twice)
			}
			if got := secret.Detect(once).String(); got != "" {
				t.Errorf("Detect(Mask(p)) = %q, want empty", got)
			}
		})
	}
}

// TestMaskLeavesACleanPayloadByteIdentical: nothing matched, nothing is
// rewritten - not even a re-encode, which would reorder keys and renumber
// floats in a log line that had no secret in it.
func TestMaskLeavesACleanPayloadByteIdentical(t *testing.T) {
	raw := payloadWith(t, "go test -p 1 ./internal/secret/...")
	if got := secret.Detect(raw).String(); got != "" {
		t.Fatalf("Detect = %q, want empty; the payload is not clean", got)
	}
	got := secret.Mask(raw)
	if !bytes.Equal(got, raw) {
		t.Errorf("Mask rewrote a clean payload:\n got: %s\nwant: %s", got, raw)
	}
}

// TestSetRoundTripsThroughText covers the stored form: privacy_class goes into
// a TEXT column and comes back out, and it has to compare equal on the way back.
func TestSetRoundTripsThroughText(t *testing.T) {
	raw := payloadWith(t, secrettest.Of(secret.ClassUserPath).Value+" "+secrettest.Of(secret.ClassOpaque).Value)
	got := secret.Detect(raw)
	if want := "opaque,user-path"; got.String() != want {
		t.Fatalf("Detect = %q, want %q", got, want)
	}
	if back := secret.ParseSet(got.String()); !slices.Equal(back, got) {
		t.Errorf("ParseSet(%q) = %v, want %v", got, back, got)
	}
	if got := secret.Set(nil).String(); got != "" {
		t.Errorf("empty Set stores as %q, want the empty string", got)
	}
	if got := secret.ParseSet(""); len(got) != 0 {
		t.Errorf("ParseSet(\"\") = %v, want empty", got)
	}
	// Order is canonical, not arrival order, so two rows that matched the same
	// classes hold the same bytes.
	if got := secret.ParseSet("user-path,api-key,user-path").String(); got != "api-key,user-path" {
		t.Errorf("ParseSet did not canonicalise: %q", got)
	}
}

// TestNoClassNameIsASubstringOfAnother keeps a LIKE filter on the stored value
// unambiguous, which is what the egress path will reach for.
func TestNoClassNameIsASubstringOfAnother(t *testing.T) {
	all := secret.Classes()
	for _, a := range all {
		for _, b := range all {
			if a != b && strings.Contains(string(a), string(b)) {
				t.Errorf("class %q contains class %q", a, b)
			}
		}
	}
}

// BenchmarkDetect measures the class that actually fires. The payload carries
// four absolute paths and no credential, which is what 900 of 902 real captures
// look like (spec 6.1).
func BenchmarkDetect(b *testing.B) {
	home := `C:\Users\` + strings.Repeat("x", 6)
	raw, err := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUse",
		"cwd":             home + `\dev\engramux`,
		"transcript_path": home + `\.claude\projects\engramux\t.jsonl`,
		"tool_input":      map[string]any{"file_path": home + `\dev\engramux\internal\secret\secret.go`},
		"tool_response":   map[string]any{"stdout": "ok\t" + home + `\dev\engramux\dist\engramux.exe`, "interrupted": false},
	})
	if err != nil {
		b.Fatalf("marshal: %v", err)
	}
	if got := secret.Detect(raw).String(); got != "user-path" {
		b.Fatalf("Detect = %q, want %q", got, "user-path")
	}
	b.ReportAllocs()
	for b.Loop() {
		secret.Detect(raw)
	}
}
