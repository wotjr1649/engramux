package search

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
)

// leafPayload wraps text as the single string leaf of a JSON object, so that
// the text a test wrote is exactly the text the walk produces.
func leafPayload(t *testing.T, text string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]string{"prompt": text})
	if err != nil {
		t.Fatalf("marshal the payload: %v", err)
	}
	return b
}

// window is what the excerpt of text must be when the cut starts at start. It
// indexes runes and not bytes, which is the assertion: a byte-indexed cut
// produces different characters for every text below that is not ASCII.
func window(text string, start int) string {
	runes := []rune(text)
	return string(runes[start : start+excerptRunes])
}

// TestExcerptCentresTheFirstToken. The token sits at rune 600 of an 1106-rune
// text, so the window is [480, 720) and the assertion is the whole of it rather
// than "the excerpt contains the token" - a window twice as wide, or off by
// one, or the leading one, all contain it.
func TestExcerptCentresTheFirstToken(t *testing.T) {
	text := strings.Repeat("alpha ", 100) + "NEEDLE" + strings.Repeat("beta ", 100)

	got := excerpt(leafPayload(t, text), []string{"needle"})

	if want := window(text, 480); got != want {
		t.Errorf("excerpt =\n%q\nwant\n%q", got, want)
	}
}

// TestExcerptFallsBackToTheLeadingWindow is the case the brief's egress rests
// on: a query token that is not in the masked text at all, because masking
// removed it or because the tokenizer's stemming reached it and a literal
// search cannot. The answer is the start of the document, not nothing.
func TestExcerptFallsBackToTheLeadingWindow(t *testing.T) {
	text := strings.Repeat("alpha ", 100) + "NEEDLE" + strings.Repeat("beta ", 100)

	got := excerpt(leafPayload(t, text), []string{"zzznotinthistext"})

	if want := window(text, 0); got != want {
		t.Errorf("excerpt =\n%q\nwant\n%q", got, want)
	}
}

// TestExcerptCutsOnRuneBoundaries. Every rune here is three bytes, so a cut
// made on a byte offset lands inside one and produces U+FFFD; the exact-window
// comparison catches that, and the rune count catches a window that is short
// because the bytes were counted instead.
func TestExcerptCutsOnRuneBoundaries(t *testing.T) {
	text := strings.Repeat("가", 300) + "찾는말" + strings.Repeat("나", 300)

	got := excerpt(leafPayload(t, text), []string{"찾는말"})

	if want := window(text, 180); got != want {
		t.Errorf("excerpt =\n%q\nwant\n%q", got, want)
	}
	if n := utf8.RuneCountInString(got); n != excerptRunes {
		t.Errorf("the excerpt is %d runes, want %d", n, excerptRunes)
	}
	if !utf8.ValidString(got) {
		t.Errorf("the excerpt is not valid UTF-8: %q", got)
	}
}

// TestExcerptFindsATokenPastACaseFoldThatChangesLength. U+212A KELVIN SIGN is
// three bytes and lowercases to a one-byte 'k', so 200 of them ahead of the
// token put a 400-byte drift between the folded text and the original. An
// implementation that takes strings.Index's byte offset into the folded copy
// and cuts the original at it lands 400 bytes early; this pins that the offset
// is converted back through a rune count.
func TestExcerptFindsATokenPastACaseFoldThatChangesLength(t *testing.T) {
	text := strings.Repeat("K", 200) + strings.Repeat("pad ", 100) + "NEEDLE" + strings.Repeat("x ", 200)

	got := excerpt(leafPayload(t, text), []string{"needle"})

	// The token starts at rune 600: 200 Kelvin signs plus 400 runes of pad.
	if want := window(text, 480); got != want {
		t.Errorf("excerpt =\n%q\nwant\n%q", got, want)
	}
}

// TestExcerptOfAShortTextIsTheWholeText. Nothing is cut when there is nothing
// to cut, and the answer is the text itself and not a padded window.
func TestExcerptOfAShortTextIsTheWholeText(t *testing.T) {
	const text = "a short prompt with a needle in it"

	got := excerpt(leafPayload(t, text), []string{"needle"})

	if got != text {
		t.Errorf("excerpt = %q, want %q", got, text)
	}
}

// TestExcerptOfAPayloadWithNoTextIsEmpty. store.Leaves answers "" for a payload
// nested past SQLite's depth limit, so the walk this cuts from can legitimately
// hand back nothing. That is an empty excerpt and not a panic or a fallback
// window of a string that has no runes.
func TestExcerptOfAPayloadWithNoTextIsEmpty(t *testing.T) {
	const past = 1001
	deep := []byte(strings.Repeat("[", past) + `"nothing reads this"` + strings.Repeat("]", past))

	if got := excerpt(deep, []string{"nothing"}); got != "" {
		t.Errorf("excerpt = %q, want the empty string", got)
	}
}

// TestExcerptMasksTheWholeDocumentBeforeCutting is I-10 at this package's
// boundary, and the gate is the same assertion at the pipe's. A fragment of a
// payload does not mask - a token cut from its prefix comes back from
// MaskString unchanged - so the masking has to happen while the document is
// still whole.
func TestExcerptMasksTheWholeDocumentBeforeCutting(t *testing.T) {
	sample := secrettest.Of(secret.ClassAPIKey)
	text := "please deploy with " + sample.Value + " and report back"

	got := excerpt(leafPayload(t, text), []string{"deploy"})

	if strings.Contains(got, sample.Secret) {
		t.Errorf("the excerpt carries the secret")
	}
	// The excerpt is not printed on failure. It is a generated sample and
	// not a real credential, but a message that echoes what it was checking
	// for is a bad habit in a repository whose origin is public.
	if want := "[redacted-" + string(sample.Class) + "]"; !strings.Contains(got, want) {
		t.Errorf("the excerpt does not carry %q", want)
	}
}

// windowAt is [window] for a cut that is no longer excerptRunes wide, which is
// what an aligned window is when it gave runes back at one edge or both.
func windowAt(text string, start, end int) string {
	return string([]rune(text)[start:end])
}

// TestExcerptStartsAndEndsAtAWordBoundary is backlog 48's display half. The
// window is centred on the match and was aligned to nothing, so it began
// mid-word: `lUse`, out of a `PreToolUse` the cut landed inside, is the
// observation the row was opened on.
//
// The arithmetic is written out because the assertion is the exact window, the
// way every other test here asserts one. The match is at rune 660, so the
// unaligned window is [540, 780). 540 is the second rune of a `PreToolUse`, so
// the start moves to 550, past the space that ends it; 780 is inside a
// `PostToolUse`, so the end moves back to 773, the boundary before the space
// that begins it. Both edges move, and each edge is decided from the *unaligned*
// window - moving the start does not carry the end along with it, so the excerpt
// gives runes back rather than sliding down the text.
func TestExcerptStartsAndEndsAtAWordBoundary(t *testing.T) {
	text := strings.Repeat("PreToolUse ", 60) + "NEEDLE" + strings.Repeat("PostToolUse ", 60)

	got := excerpt(leafPayload(t, text), []string{"needle"})

	if want := windowAt(text, 550, 773); got != want {
		t.Errorf("excerpt =\n%q\nwant\n%q", got, want)
	}
	// The match is what the window exists to show, and an alignment wide
	// enough to move past it would pass the comparison above only by
	// agreeing with itself.
	if !strings.Contains(got, "NEEDLE") {
		t.Errorf("the aligned window no longer holds the match: %q", got)
	}
}

// TestExcerptKeepsARaggedEdgeWhenThereIsNoWordBoundary is the other half of the
// same rule, and it is why the alignment is bounded. A run of text with no space
// in it - a UUID, a path, a language that does not space its words - has no
// boundary to move to, and moving anyway would cut the window down for nothing.
// The answer there is the unaligned window, unchanged.
//
// The filler is broken by a hyphen every second rune, which is not a space and
// is therefore not a boundary. A plain run of letters would not do: 40 of them
// in a row is internal/secret's opaque-token rule, and the text this cuts from
// is the *masked* document, so the fixture would be measuring the mask.
func TestExcerptKeepsARaggedEdgeWhenThereIsNoWordBoundary(t *testing.T) {
	text := strings.Repeat("a-", 500) + "NEEDLE" + strings.Repeat("b-", 500)

	got := excerpt(leafPayload(t, text), []string{"needle"})

	if want := window(text, 880); got != want {
		t.Errorf("excerpt =\n%q\nwant\n%q", got, want)
	}
}

// TestTheAlignmentReachesExactlyExcerptAlign pins the bound itself, which the
// two window tests above cannot: one has a boundary within reach and the other
// has none at all, so both pass under a budget of any size.
//
// The strings are built from the constant rather than from 32, so this says the
// same thing whatever the budget is set to. What it holds is the arithmetic: a
// space excerptAlign-1 runes from the edge is reachable and the next one out is
// not, in both directions.
func TestTheAlignmentReachesExactlyExcerptAlign(t *testing.T) {
	const at = 1

	t.Run("forward, the last reachable space", func(t *testing.T) {
		runes := []rune(strings.Repeat("x", at+excerptAlign-1) + " " + "word")
		if got, want := alignForward(runes, at), at+excerptAlign; got != want {
			t.Errorf("alignForward = %d, want %d", got, want)
		}
	})

	t.Run("forward, one rune past it", func(t *testing.T) {
		runes := []rune(strings.Repeat("x", at+excerptAlign) + " " + "word")
		if got := alignForward(runes, at); got != -1 {
			t.Errorf("alignForward = %d, want -1: the space is past the budget", got)
		}
	})

	t.Run("back, the last reachable space", func(t *testing.T) {
		const head = "word "
		runes := []rune(head + strings.Repeat("x", excerptAlign))
		if got, want := alignBack(runes, len(head)+excerptAlign-1), len(head)-1; got != want {
			t.Errorf("alignBack = %d, want %d", got, want)
		}
	})

	t.Run("back, one rune past it", func(t *testing.T) {
		const head = "word "
		runes := []rune(head + strings.Repeat("x", excerptAlign+1))
		if got := alignBack(runes, len(head)+excerptAlign); got != -1 {
			t.Errorf("alignBack = %d, want -1: the space is past the budget", got)
		}
	})

	t.Run("an edge that cuts no word does not move", func(t *testing.T) {
		runes := []rune("word another")
		if got := alignForward(runes, len("word")+1); got != -1 {
			t.Errorf("alignForward = %d, want -1: the edge is already at a word start", got)
		}
		if got := alignBack(runes, len("word")); got != -1 {
			t.Errorf("alignBack = %d, want -1: the edge is already at a word end", got)
		}
	})
}
