package search

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/store"
)

// excerptRunes is how much of a matched event an excerpt shows: a window of
// this many runes, centred on the match. It is runes and not bytes so that the
// same query shows the same amount of text whether the payload is English or
// Korean, where a syllable is three bytes.
//
// 240 is about three terminal lines at 80 columns. Small enough that a screen
// of hits is still a screen, wide enough that a match has its sentence around
// it - which is the whole point of showing an excerpt rather than an id.
const excerptRunes = 240

// excerpt is what a hit shows of the event it matched: a window of the payload's
// text around the first query token in it, cut from a document
// [secret.Mask] has already been over.
//
// # Why this is Go and not snippet()
//
// FTS5 offers snippet() and highlight() and neither may be used here. The index
// is external-content, so highlight() takes its text from the content table and
// its markers from the index, and a desync silently puts the markers in the
// wrong place; snippet() against a missing base row returns some rows and then
// fails with `database disk image is malformed` in rows.Err(), which a loop
// that ignores Err() never sees. Both would also cut the *stored* payload,
// which is the one thing I-10 forbids: the row holds the secret and the
// excerpt must not.
//
// # Why the whole document is masked first
//
// Masking a fragment leaks, and it was measured on the real rules: a value cut
// from its key, an `sk-` prefix cut mid-token, a cut `Bearer`, a path split
// across the window, and Codex's string-carried JSON where an escaped quote
// defeats the credential rule all come back from [secret.MaskString]
// unchanged. So the payload is masked whole - the JSON path, where a rule sees
// its key and its value together - and only then is a window cut out of the
// result. [secret.MaskString] is never called on a fragment here.
//
// # Why there are no offsets
//
// Nothing this returns points into the stored bytes, and nothing can. Masking
// re-encodes the document whenever it changed anything, which sorts object keys
// and HTML-escapes '<', '>' and '&' - so the masked document's leaf order and
// characters are not the indexed document's. A query token can therefore be
// absent from the masked text even though the index matched it, which is what
// the leading-window fallback below is for. The two are not made to agree; they
// are not the same document.
func excerpt(payload []byte, tokens []string) string {
	// The same walk the index was built from ([store.Leaves]), over the
	// masked document rather than the stored one. A payload with no string
	// leaves - or one nested past SQLite's depth limit, which that walk
	// answers "" for - has no excerpt, and that is an empty string rather
	// than a special case here.
	text := store.Leaves(secret.Mask(payload))

	runes := []rune(text)
	if len(runes) <= excerptRunes {
		return text
	}

	start := 0
	if at := firstFold(text, tokens); at >= 0 {
		start = at - excerptRunes/2
	}
	// Clamped in this order: pulling the window back inside the end of the
	// text can push it before the start, and start wins.
	start = max(min(start, len(runes)-excerptRunes), 0)
	return string(runes[start : start+excerptRunes])
}

// firstFold returns the rune index of the earliest occurrence of any token in
// text, matched without regard to case, or -1 when none of them is there.
//
// The fold is [strings.Map] with [unicode.ToLower], which maps each rune to
// exactly one rune, so the folded copy has the same number of runes as the
// original at every position. That is what makes the conversion below exact,
// and it is not true of the byte offsets: U+212A KELVIN SIGN is three bytes and
// folds to a one-byte 'k', so a byte offset taken in the folded copy points
// somewhere else entirely in the original.
//
// This is simple case mapping and not full Unicode case folding - 'ß' does not
// match "SS". That is the documented behaviour rather than a gap to close: a
// token that is not found falls back to the leading window, which is already
// the answer for every token masking removed.
func firstFold(text string, tokens []string) int {
	folded := strings.Map(unicode.ToLower, text)
	best := -1
	for _, tok := range tokens {
		i := strings.Index(folded, strings.Map(unicode.ToLower, tok))
		if i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if best < 0 {
		return -1
	}
	return utf8.RuneCountInString(folded[:best])
}
