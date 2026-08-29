package search_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Microsoft/go-winio"
	"github.com/google/uuid"

	"github.com/wotjr1649/engramux/internal/ipc"
	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
	"github.com/wotjr1649/engramux/internal/service"
	"github.com/wotjr1649/engramux/internal/store"
)

// TestPhase4GateTheSearchEgressMasks is Phase 4's gate clause for I-10 at the
// second egress, and it has the shape Phase 1's clause 4 has: a runtime-
// generated secret goes in over the pipe, comes back findable by a term that is
// not the secret, and the row still holds it afterwards.
//
// It is a separate function from TestPhase4Gate rather than a subtest of it
// because it needs a running service. TestPhase4Gate builds its own database
// from a document set and never starts one; this clause is about what leaves
// the process, so nothing short of the real run loop, the real pipe and the
// real handler measures it.
//
// Everything below travels the pipe, ingest included. I-08 is why: the service
// holds the database exclusively (I-07), so this is the only surface there is.
//
// # What is not asserted, and why
//
// No message here prints the excerpt, the payload or the secret. The samples
// are generated at run time and are not real credentials, but a test whose
// failure output is the thing it was checking for is a bad habit to leave in a
// repository whose origin is public.
func TestPhase4GateTheSearchEgressMasks(t *testing.T) {
	sample := secrettest.Of(secret.ClassAPIKey)
	dir := t.TempDir()
	stop := runService(t, dir)

	id := mintID(t)
	ingest(t, id, gatePayload(sample.Value))

	// Clause 1: the term that is not the secret finds exactly the event
	// that carries it.
	hits := searchOverThePipe(t, term)
	if len(hits) != 1 {
		t.Fatalf("a search for the invented term returned %d hits, want 1", len(hits))
	}
	if hits[0].ID != id {
		t.Fatalf("the hit is event %q, want the one that was ingested, %q", hits[0].ID, id)
	}
	// The window is what the paddings in [gatePayload] are measured
	// against. If this number moves, those move with it - see that
	// function's comment.
	if n := utf8.RuneCountInString(hits[0].Excerpt); n != excerptWindow {
		t.Fatalf("the excerpt is %d runes, want %d: the paddings in gatePayload are tied to this number", n, excerptWindow)
	}

	// Clause 2: the excerpt says what was removed and does not carry it.
	assertMasked(t, "the excerpt for the invented term", hits[0].Excerpt, sample)
	if want := "[redacted-" + string(sample.Class) + "]"; !strings.Contains(hits[0].Excerpt, want) {
		t.Errorf("the excerpt does not carry %q, so it is not the window the secret was in", want)
	}

	// Clause 3: the index holds the original, so the secret finds the event
	// too - and the excerpt still does not carry it. This is the half that
	// separates "masked on the way out" from "never stored": a search that
	// could not find it would mean the index had been masked instead.
	bySecret := searchOverThePipe(t, sample.Value)
	if len(bySecret) != 1 {
		t.Fatalf("a search for the secret itself returned %d hits, want 1 - the index does not hold what was captured", len(bySecret))
	}
	if bySecret[0].ID != id {
		t.Fatalf("the hit is event %q, want %q", bySecret[0].ID, id)
	}
	assertMasked(t, "the excerpt for the secret itself", bySecret[0].Excerpt, sample)

	// Clause 4: and the row still holds it, byte for byte. That is the
	// "while the row still holds it" the clause is named for - masking on
	// the way out is only meaningful if nothing was masked on the way in.
	if err := stop(); err != nil {
		t.Fatalf("stop the service: %v", err)
	}
	assertTheRowStillHoldsIt(t, dir, id, sample)
}

// excerptWindow is internal/search's excerpt window, which this package's
// external test cannot read. It is restated here so that a change to it fails
// this gate loudly rather than quietly costing it the property the two
// deliberate breaks measured.
const excerptWindow = 240

// term is the distinctive invented word the gate searches for. It is in no
// fixture and in no payload but the one built below, and it is not the secret -
// which is the whole point of the clause: the event is findable by something
// that is safe to type.
const term = "frobnicatorZeta"

// gatePayload is one hook event whose free-text leaf carries the secret and
// [term] in that order, separated by a measured amount of filler.
//
// # The paddings are load-bearing
//
// They are what make the two deliberate breaks this gate was developed against
// go red, and they are measured against [excerptWindow]:
//
//   - The window is centred on the term, so it starts 120 runes before it.
//     The 96 runes between the secret and the term put that cut 13 characters
//     into the secret - past `sk-ant-api03-` and into its body. A window cut
//     before masking therefore hands secret.MaskString a fragment with no
//     prefix left on it, which is exactly the shape §4 measured coming back
//     unchanged, and 24 characters of the secret survive into the excerpt.
//   - After masking the secret is an 18-character placeholder, so it sits 114
//     runes before the term - inside the same 120, which is what lets clause 2
//     assert the excerpt is the window the secret was in and not some other
//     part of the document.
//
// The trailing filler keeps the window off the end of the text, so it is
// centred rather than clamped.
//
// # The term is space-delimited on both sides, and it has to be
//
// unicode61 splits on non-alphanumerics and on nothing else, so filler written
// straight up against the term makes one token of the three: the first draft
// indexed `contextfrobnicatorzetacontext` and a search for the term found
// nothing at all. [gap] is what keeps the boundaries.
//
// The cwd is the repository's placeholder user path (AGENTS.md). It is masked
// too, which is incidental here and is why the leading window in clause 3 is
// not empty of placeholders.
func gatePayload(value string) []byte {
	prompt := pad(40) + value + gap(96) + term + " " + pad(199)
	b, err := json.Marshal(map[string]string{
		"session_id":      "t6-egress-gate",
		"hook_event_name": "UserPromptSubmit",
		"prompt_id":       "t6-egress",
		"cwd":             `C:\Users\fixture\workspace\fixture-project`,
		"prompt":          prompt,
	})
	if err != nil {
		panic("gate: marshal the payload: " + err.Error())
	}
	return b
}

// pad returns exactly n runes of filler. Every rune is ASCII, so n runes is n
// bytes and the offsets [gatePayload] documents are the ones that occur.
func pad(n int) string {
	const word = "context "
	return strings.Repeat(word, n/len(word)+1)[:n]
}

// gap is [pad] with a space at each end, so that whatever it separates keeps
// its own token boundaries whatever n happens to cut the filler at.
func gap(n int) string { return " " + pad(n-2) + " " }

// assertMasked is clause 2 and the second half of clause 3: the excerpt carries
// neither the secret nor a usable run of it.
//
// The run is what the clause needs rather than the whole value. The window is
// cut from the masked document, so a break that cuts before masking hands back
// a fragment of the secret and not the whole of it - `strings.Contains` for the
// whole value would report clean while 24 of its 37 characters were on the
// wire. Twelve consecutive characters of a credential is a leak.
func assertMasked(t *testing.T, what, excerpt string, sample secrettest.Sample) {
	t.Helper()
	const run = 12
	if strings.Contains(excerpt, sample.Secret) {
		t.Errorf("%s carries the whole secret", what)
		return
	}
	if at := runIn(excerpt, sample.Secret, run); at >= 0 {
		t.Errorf("%s carries %d consecutive characters of the secret, starting at its character %d", what, run, at)
	}
}

// runIn returns the offset into s of the first run of n consecutive characters
// of s that text holds, or -1.
func runIn(text, s string, n int) int {
	r := []rune(s)
	for i := 0; i+n <= len(r); i++ {
		if strings.Contains(text, string(r[i:i+n])) {
			return i
		}
	}
	return -1
}

// assertTheRowStillHoldsIt is clause 4. The service has stopped, so I-07's
// exclusive hold is released and another process - this one - can open the file
// and read the column the excerpt was built from.
func assertTheRowStillHoldsIt(t *testing.T, dir, id string, sample secrettest.Sample) {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(dir, "engramux.db"))
	if err != nil {
		t.Fatalf("open the database after the service stopped: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	}()

	var payload string
	if err := db.QueryRowContext(t.Context(),
		`SELECT payload FROM events WHERE id = ?`, id).Scan(&payload); err != nil {
		t.Fatalf("read events.payload: %v", err)
	}
	if !strings.Contains(payload, sample.Secret) {
		t.Error("events.payload no longer holds the secret: it was masked on the way in, " +
			"so the egress proves nothing")
	}
}

// runService starts the real run loop in dir on its own goroutine and returns
// the function that stops it.
//
// It waits for a Status request to be answered rather than for a dial or a
// duration: pipe.Listen creates the pipe instance before Serve is called, so a
// dial succeeds while store.Open is still running.
func runService(t *testing.T, dir string) (stop func() error) {
	t.Helper()
	useTestPipeName(t)

	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx, dir) }()

	deadline := time.Now().Add(30 * time.Second)
	for !servingOK(t) {
		select {
		case err := <-done:
			cancel()
			t.Fatalf("Run returned before it served: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("Run did not answer a Status request within 30s of being started")
		}
		time.Sleep(20 * time.Millisecond)
	}

	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(20 * time.Second):
			t.Fatal("Run did not return within 20s of the context being cancelled")
			return nil
		}
	}
}

// useTestPipeName moves this test's service and its dials onto a name unique to
// the test and the process, so a development service holding the real one is
// not in the way. It is process-wide, so this test cannot be parallel.
func useTestPipeName(t *testing.T) {
	t.Helper()
	t.Setenv(ipc.TestPipeSIDEnv, "engramux-test-"+strconv.Itoa(os.Getpid())+"-"+t.Name())
}

func pipeName(t *testing.T) string {
	t.Helper()
	name, err := ipc.CurrentPipeName()
	if err != nil {
		t.Fatalf("ipc.CurrentPipeName: %v", err)
	}
	return name
}

// servingOK reports whether a whole request completes. Every failure is a "not
// yet" rather than a test failure, because the interesting failure is the
// caller's deadline.
func servingOK(t *testing.T) bool {
	t.Helper()
	req, err := json.Marshal(ipc.Envelope{Version: ipc.Version, Type: ipc.Status})
	if err != nil {
		t.Fatalf("marshal the status request: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, pipeName(t))
	if err != nil {
		return false
	}
	defer func() { _ = c.Close() }()
	if err := c.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return false
	}
	if err := ipc.WriteFrame(c, req); err != nil {
		return false
	}
	raw, err := ipc.ReadFrame(c)
	if err != nil {
		return false
	}
	var reply ipc.StatusReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return false
	}
	return reply.Verify() == nil
}

// exchange sends one request frame and reads one reply frame.
func exchange(t *testing.T, req []byte) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, pipeName(t))
	if err != nil {
		t.Fatalf("dial the service: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close the connection: %v", err)
		}
	}()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set the deadline: %v", err)
	}
	if err := ipc.WriteFrame(conn, req); err != nil {
		t.Fatalf("write the request: %v", err)
	}
	raw, err := ipc.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read the reply: %v", err)
	}
	return raw
}

func mintID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("mint an ingest id: %v", err)
	}
	return id.String()
}

// ingest sends one IngestEvent over the pipe and requires a committed ACK.
func ingest(t *testing.T, id string, payload []byte) {
	t.Helper()
	req, err := json.Marshal(ipc.Envelope{
		Version:  ipc.Version,
		Type:     ipc.IngestEvent,
		IngestID: id,
		Payload:  payload,
	})
	if err != nil {
		t.Fatalf("marshal the ingest request: %v", err)
	}

	var ack ipc.Ack
	if err := json.Unmarshal(exchange(t, req), &ack); err != nil {
		t.Fatalf("decode the ack: %v", err)
	}
	if err := ack.Verify(id); err != nil {
		t.Fatalf("the service did not commit the event: %v", err)
	}
}

// searchOverThePipe sends one Search request and returns the hits of a reply it
// can accept. Verify is what tells a search reply from the rejected ACK the
// service answers a request it will not serve; without it an empty hit list
// would read as "nothing matched".
func searchOverThePipe(t *testing.T, query string) []ipc.SearchHit {
	t.Helper()
	payload, err := json.Marshal(ipc.SearchRequest{Query: query})
	if err != nil {
		t.Fatalf("marshal the search request: %v", err)
	}
	req, err := json.Marshal(ipc.Envelope{Version: ipc.Version, Type: ipc.Search, Payload: payload})
	if err != nil {
		t.Fatalf("marshal the search envelope: %v", err)
	}

	var reply ipc.SearchReply
	raw := exchange(t, req)
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("decode the search reply: %v", err)
	}
	if err := reply.Verify(); err != nil {
		// The reply is not printed: for a rejected ACK it is three
		// short fields, and for anything else it is bytes that came
		// off a pipe carrying payload text.
		t.Fatalf("the search reply did not verify: %v", err)
	}
	return reply.Hits
}
