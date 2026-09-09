// Not built under the race detector, and scripts/race.sh's own comment block
// carries the measurement and the reasoning. The short of it: nothing in this
// file is concurrent, so -race has nothing here to find, and at the highest
// multiplier that script has observed it would cost about 250 minutes against a
// 90-minute guard.
//
// This is the one exception to that script's "raise it here rather than
// skipping the gate", and it is an exception because the rule is about not
// quietly dropping coverage - there is none to drop. A gate with a goroutine in
// it does not get this line.

//go:build !race

package search_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wotjr1649/engramux/internal/search"
	"github.com/wotjr1649/engramux/internal/store"
)

// Gate M15 (memory spec 5): of the bytes the database holds, how many a
// session-level eviction would remove.
//
// # The criterion, and the sentence it must not lose
//
// Droppable means unreachable: an event is kept when a query cut from its own
// text returns it, as membership in the MATCH set and not as a rank. That is
// exactly what [TestEveryCandidateDocumentIsReachable] computes, over the same
// five classes and through the same builder, and this runs the same question
// over the live corpus instead of over the fixtures. An event carrying no
// cuttable literal in any class is never a candidate and therefore never kept.
//
// A session is dropped only when every event in it is droppable. No partial
// sessions.
//
// What the number answers is "under this query distribution, X% of bytes were
// never reached". It does not answer "X% is safe to delete". The spec's section
// carries that sentence in full and it is the one thing this file must not be
// read without.
//
// # Why it reads a snapshot rather than ingesting one
//
// The other corpus gates build a database from `.capture/fixtures-raw` through
// store.Ingest, because what they measure is the write path's index. This
// measures the corpus, and the corpus is the installed database - fifty times
// the fixtures and the thing retention would actually act on. So it opens a
// copy of it and asks its own index, which is also what makes the run tractable:
// nothing is re-ingested and nothing is rebuilt.
//
// The copy is a pair. `schtasks /end` is a hard kill, so the WAL holds committed
// frames the `.db` does not, and `.db` alone is a database missing them - copy
// both or measure a corpus that is not the one on the machine.
//
// # Nothing here logs a leaf, a query, a path or an id
//
// Every query it derives is cut from captured text, and 900 of 902 captures
// carry the user's own directory. AGENTS.md's row about TestPhase4Gate's corpus
// mode is what that costs. This emits counts, byte totals and figures only; do
// not add a query, a session id or a file name to it.
//
// Measured 2026-09-09 over a full run: 0 of the 39 `-v` lines carry a
// drive-letter path and 0 carry the OS user name, so the output is safe to
// paste. That was a design condition rather than an accident, and it is checked
// by grepping a run for `[A-Za-z]:[\\/]` rather than by reading it.
//
//	go test -p 1 -count=1 -timeout 2h -run TestGateM15 -v ./internal/search/
func TestGateM15SessionEvictionEarnsItsPlace(t *testing.T) {
	db, bytesOnDisk := openSnapshot(t, m15Snapshot)

	var (
		events, candidates, refused int
		reachedByClass              = make([]int, len(classes))
		candidatesByClass           = make([]int, len(classes))
		keptEvents, walkDisagreed   int
		separators                  int64
		total, kept                 m15Bytes
		sessions                    = map[string]*m15Session{}
		values                      m15Values
	)

	start := time.Now()
	for batch := range m15Batches(t, db) {
		if events%m15Progress < m15Batch {
			t.Logf("M15 sweep: %d events in %s", events, time.Since(start).Round(time.Second))
		}
		for _, e := range batch {
			events++
			total.add(e)
			values.add(e.leaves)
			if e.goLeafLen != e.leafLen {
				walkDisagreed++
			}
			if n := len(e.leaves); n > 1 {
				separators += int64(n - 1)
			}

			d := doc{leaves: e.leaves}
			reachable, isCandidate := false, false
			for i, c := range classes {
				q := c.derive(d)
				if q == "" {
					continue
				}
				candidatesByClass[i]++
				isCandidate = true
				tokens, err := search.QueryTokens(q)
				if err != nil {
					// The product would refuse it too, so the
					// document is not reachable by the query
					// this class derives from it - the same
					// fact a miss records. Counted apart
					// because a refusal is the query bounds
					// firing and a miss is the index.
					refused++
					continue
				}
				if m15Reaches(t, db, e.rowid, search.MatchExpression(tokens, search.MatchAll)) {
					reachedByClass[i]++
					reachable = true
				}
			}
			if isCandidate {
				candidates++
			}
			if reachable {
				keptEvents++
				kept.add(e)
			}

			s := sessions[e.session]
			if s == nil {
				s = &m15Session{allDroppable: true}
				sessions[e.session] = s
			}
			s.events++
			s.bytes.add(e)
			if reachable {
				s.allDroppable = false
			}
		}
	}
	sweep := time.Since(start)

	if events == 0 {
		t.Fatal("the snapshot holds no events; this measures nothing")
	}

	var droppedSessions int
	var dropped m15Bytes
	for _, s := range sessions {
		if !s.allDroppable {
			continue
		}
		droppedSessions++
		dropped.payload += s.bytes.payload
		dropped.leaves += s.bytes.leaves
		dropped.events += s.bytes.events
	}

	t.Logf("M15 corpus: %d events in %d sessions, %.1f MB of payload and %.1f MB of leaves, "+
		"%.1f MB on disk", events, len(sessions), mb(total.payload), mb(total.leaves), mb(bytesOnDisk))
	t.Logf("M15 sweep: %d candidate events of %d, %d refused by the builder, in %s",
		candidates, events, refused, sweep.Round(time.Second))
	// Asserted rather than reported, and it is the one assertion in this file
	// with teeth. Two things rest on it. A query is derived from the Go walk
	// and has to be found in an index built from the migration's SQL walk, so
	// a disagreement would show up as an unreachable event and be counted as
	// droppable - the failure mode that flatters the result. And the second
	// arm's byte totals are summed from the Go walk while the denominator is
	// the stored column, so the two have to be the same bytes or the ladder
	// is a fraction of the wrong whole.
	//
	// TestTheTwoWalksAgree holds this over 902 payloads and asserts far more
	// about each; what this adds is only the population.
	if walkDisagreed != 0 {
		t.Errorf("the Go walk and the stored SQL walk disagree by byte total on %d of %d events. "+
			"Every figure below is computed across that seam - fix the disagreement rather than "+
			"reading the numbers.", walkDisagreed, events)
	}
	t.Logf("M15 premise: the Go walk and the stored SQL walk agree by byte total on %d of %d events",
		events-walkDisagreed, events)

	// The exact cross-check the second arm needs, in bytes rather than in the
	// rounded megabytes a reader would compare by eye: every string value it
	// sums, plus one separator per gap, is the leaves column it reports a
	// fraction of.
	if got := values.total + separators; got != total.leaves {
		t.Errorf("the second arm sums %d bytes of string values and %d separators, and the leaves "+
			"column holds %d. Its ladder is a fraction of a whole it did not measure.",
			values.total, separators, total.leaves)
	}
	for i, c := range classes {
		t.Logf("M15 class %s: %d of %d candidate events reached",
			c.name, reachedByClass[i], candidatesByClass[i])
	}

	// The event-level figure is the upper bound the whole-session rule gives
	// up, and it is reported beside the real one so the cost of "no partial
	// sessions" is visible rather than inferred.
	t.Logf("M15 events: %d of %d reachable, so %d events (%.1f%% of payload+leaves bytes) are droppable "+
		"one at a time", keptEvents, events, events-keptEvents,
		100*share(total.text()-kept.text(), total.text()))

	var allSizes, droppedSizes []int
	for _, s := range sessions {
		allSizes = append(allSizes, s.events)
		if s.allDroppable {
			droppedSizes = append(droppedSizes, s.events)
		}
	}
	t.Logf("M15 sessions: %d of %d have no reachable event at all", droppedSessions, len(sessions))
	t.Logf("M15 session size, every session: %s", m15Spread(allSizes))
	t.Logf("M15 session size, the dropped ones: %s", m15Spread(droppedSizes))
	t.Logf("M15 VERDICT: session-level eviction removes %.2f%% of payload+leaves bytes "+
		"(%.1f MB of %.1f MB), %.2f%% of payload bytes, %.2f%% of events",
		100*share(dropped.text(), total.text()), mb(dropped.text()), mb(total.text()),
		100*share(dropped.payload, total.payload), 100*share(int64(dropped.events), int64(events)))

	if share(dropped.text(), total.text()) < m15Bar {
		t.Logf("M15: below the %.0f%% bar. By this gate's own terms the eviction code is not written "+
			"and this branch closes.", 100*m15Bar)
	} else {
		t.Logf("M15: at or above the %.0f%% bar. What that licenses is the eviction code being written, "+
			"and nothing about any particular session being safe to delete.", 100*m15Bar)
	}

	values.report(t, total.payload)
}

// m15Bar is memory spec 5's number and was registered before this file existed:
// below it the eviction code is not written. It is a share of bytes rather than
// of sessions because a session of three hundred tiny events and one of five
// enormous ones do not buy the same thing.
const m15Bar = 0.30

// m15Snapshot is the copied installed database, relative to this package. The
// pair - `.db` and `.db-wal` - is what a snapshot is; see the doc comment.
var m15Snapshot = filepath.Join("..", "..", ".capture", "m15", "m15.db")

// m15Batch is how many events one scan reads before the cursor is closed.
//
// It is batched rather than streamed because store.Open's pool holds exactly one
// connection (spec 5.4): a MATCH issued while a scan's rows are still open would
// wait for a connection the scan is holding, and wait forever. Reading a bounded
// window by rowid, closing it, and then asking the index is what keeps one
// connection enough.
const m15Batch = 500

// m15Progress is how often the sweep says where it is. Under `-v` the chatty
// printer flushes a t.Logf as it happens, which is the only way a run measured
// in minutes shows progress at all; without -v it is buffered and costs nothing.
const m15Progress = 5000

// m15Event is one row of the snapshot, with the walk already done.
type m15Event struct {
	rowid   int64
	session string
	payload int64
	leaves  []string
	leafLen int64
	// goLeafLen is what the Go walk would have joined to, in bytes. The
	// stored column is the migration's SQL walk, and the premise of this
	// whole measurement is that the two agree: a query derived from the Go
	// walk can only be found in an index built from the SQL one if they do.
	// TestTheTwoWalksAgree holds that over 902 payloads; this reports it
	// over fifty times as many, for free, because both numbers are already
	// in hand.
	//
	// The two walks order an object's members differently - leavesOf sorts
	// keys, store.Leaves keeps document order - so only the byte total is
	// comparable, and it is the same total for the same set of strings.
	goLeafLen int64
}

// m15Bytes is a byte total over some set of events.
type m15Bytes struct {
	payload, leaves int64
	events          int
}

func (b *m15Bytes) add(e m15Event) {
	b.payload += e.payload
	b.leaves += e.leafLen
	b.events++
}

// text is the events table's own text bytes: the payload and the second copy of
// its string leaves the index is built over. It is the denominator the verdict
// uses, because those two columns are what an evicted row actually returns.
func (b m15Bytes) text() int64 { return b.payload + b.leaves }

// m15Session is one session's running answer.
type m15Session struct {
	allDroppable bool
	events       int
	bytes        m15Bytes
}

func share(part, whole int64) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}

func mb(b int64) float64 { return float64(b) / (1 << 20) }

// m15Spread is a size distribution in one line. The brief's expectation was that
// a session of three hundred events is unlikely to hold no reachable one, so how
// large the dropped sessions are is what tells a real finding from an artefact
// of a corpus full of one-event sessions.
func m15Spread(sizes []int) string {
	if len(sizes) == 0 {
		return "none"
	}
	slices.Sort(sizes)
	at := func(p float64) int { return sizes[int(p*float64(len(sizes)-1))] }
	var total int
	for _, n := range sizes {
		total += n
	}
	return fmt.Sprintf("%d sessions holding %d events; min %d, p50 %d, p90 %d, max %d, mean %.1f",
		len(sizes), total, sizes[0], at(0.50), at(0.90), sizes[len(sizes)-1],
		float64(total)/float64(len(sizes)))
}

// openSnapshot opens a copied installed database and returns it with the size of
// the pair on disk. It skips when the copy is absent, the way every other corpus
// gate skips without `.capture/`.
func openSnapshot(t *testing.T, path string) (*sql.DB, int64) {
	t.Helper()
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no snapshot at %s; stop the service and copy the .db and .db-wal pair", path)
	}
	if err != nil {
		t.Fatalf("stat the snapshot: %v", err)
	}
	size := info.Size()
	if wal, err := os.Stat(path + "-wal"); err == nil {
		size += wal.Size()
	}

	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open the snapshot: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the snapshot: %v", err)
		}
	})
	return db, size
}

// m15Batches yields every event of the snapshot in rowid order, [m15Batch] at a
// time, with each payload already walked into its string leaves.
func m15Batches(t *testing.T, db *sql.DB) func(func([]m15Event) bool) {
	return func(yield func([]m15Event) bool) {
		t.Helper()
		var after int64
		for {
			batch := m15Scan(t, db, after)
			if len(batch) == 0 {
				return
			}
			after = batch[len(batch)-1].rowid
			if !yield(batch) {
				return
			}
		}
	}
}

// m15Scan reads one window of events past rowid after.
//
// octet_length and not length: length() over TEXT counts characters, so a
// payload of Korean would be reported at a third of the bytes it occupies, and
// every fraction below it would be wrong in the direction that flatters the
// result.
func m15Scan(t *testing.T, db *sql.DB, after int64) []m15Event {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `
		SELECT rowid, session_id, octet_length(payload), octet_length(leaves), payload
		FROM events
		WHERE rowid > ?
		ORDER BY rowid
		LIMIT ?`, after, m15Batch)
	if err != nil {
		t.Fatalf("scan the snapshot: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []m15Event
	for rows.Next() {
		var e m15Event
		var payload []byte
		var leafLen sql.NullInt64
		if err := rows.Scan(&e.rowid, &e.session, &e.payload, &leafLen, &payload); err != nil {
			t.Fatalf("scan a row: %v", err)
		}
		// NULL means "not computed", which only 00002's backfill ever
		// saw. A row carrying one in the installed database would mean
		// the migration did not reach it, and counting it as zero bytes
		// would hide that inside a byte fraction.
		if !leafLen.Valid {
			t.Fatalf("an event's leaves column is NULL, so migration 00002 did not reach it")
		}
		e.leafLen = leafLen.Int64
		e.leaves = leavesOf(json.RawMessage(payload))
		for _, l := range e.leaves {
			e.goLeafLen += int64(len(l))
		}
		if n := len(e.leaves); n > 1 {
			e.goLeafLen += int64(n - 1) // one separator between each pair
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate the snapshot: %v", err)
	}
	return out
}

// m15Reaches asks the snapshot's own index whether one row matches one
// expression. It is the sweep's [reaches] with the rowid already in hand - the
// caller scanned it - so there is no lookup by id and no interpolated table
// name.
func m15Reaches(t *testing.T, db *sql.DB, rowid int64, expr string) bool {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), `
		SELECT count(*)
		FROM events_fts
		WHERE rowid = ? AND events_fts MATCH ?`, rowid, expr).Scan(&n); err != nil {
		t.Fatalf("MATCH the expression built for one candidate: %v", err)
	}
	return n == 1
}

// m15Values is the exploratory second arm: the distribution of string-value
// lengths, and what truncating the values above a threshold would recover.
//
// It carries no bar, deliberately and per the spec: the choice between levers is
// being made after the numbers rather than before them, which is a weaker
// instrument than M15's own arm and is recorded as such rather than presented as
// a result of the same weight.
type m15Values struct {
	lengths []int64
	total   int64
}

func (v *m15Values) add(leaves []string) {
	for _, l := range leaves {
		v.lengths = append(v.lengths, int64(len(l)))
		v.total += int64(len(l))
	}
}

// m15Ladder is the thresholds the second arm reports, in bytes.
var m15Ladder = []int{64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384}

func (v *m15Values) report(t *testing.T, payloadBytes int64) {
	t.Helper()
	if len(v.lengths) == 0 {
		t.Fatal("the corpus carries no string values; the second arm measures nothing")
	}
	slices.Sort(v.lengths)
	pct := func(p float64) int64 {
		i := int(p * float64(len(v.lengths)-1))
		return v.lengths[i]
	}
	t.Logf("M15 arm 2: %d string values holding %.1f MB, which is %.1f%% of the payload bytes",
		len(v.lengths), mb(v.total), 100*share(v.total, payloadBytes))
	t.Logf("M15 arm 2: value length p50 %d, p90 %d, p99 %d, p99.9 %d, max %d B",
		pct(0.50), pct(0.90), pct(0.99), pct(0.999), v.lengths[len(v.lengths)-1])

	for _, threshold := range m15Ladder {
		var over int
		var recovered int64
		// Sorted ascending, so the first value over the threshold marks
		// the tail and nothing before it contributes.
		i, _ := slices.BinarySearch(v.lengths, int64(threshold))
		for _, l := range v.lengths[i:] {
			if l <= int64(threshold) {
				continue
			}
			over++
			recovered += l - int64(threshold)
		}
		t.Logf("M15 arm 2: truncating at %6d B touches %7d values and recovers %6.1f MB of payload "+
			"(%5.1f%%), or %6.1f MB counting the leaves copy",
			threshold, over, mb(recovered), 100*share(recovered, payloadBytes), mb(2*recovered))
	}
}
