package search

import (
	"errors"
	"fmt"
	"strings"
)

// The three ways a query is refused before it reaches SQLite. Each is an error
// and not an empty result set, because a caller that cannot tell "you asked for
// nothing" from "nothing matched" reports the wrong one to the person who
// typed it.
var (
	// ErrEmptyQuery is returned for a query that holds no token at all -
	// empty, or nothing but whitespace.
	ErrEmptyQuery = errors.New("search: the query has no tokens")

	// ErrTooManyTokens is returned for a query over [maxQueryTokens].
	ErrTooManyTokens = errors.New("search: the query has too many tokens")

	// ErrTokenTooLong is returned for a token over [maxTokenBytes].
	ErrTokenTooLong = errors.New("search: a query token is too long")
)

// The two bounds on a query, which are about not handing SQLite a pathological
// expression and not about the transport: [github.com/wotjr1649/engramux/internal/ipc.MaxFrameLen]
// already caps a request at 4 MiB, and 4 MiB of text is on the order of a
// million tokens.
//
// maxQueryTokens is 32 because that is far more than a person types and still a
// small expression to plan. It was chosen when every join was an implicit AND,
// where each token past the first can only narrow the result and the
// intersection of a handful of prefix phrases is empty on any real corpus.
// [MatchAny] inverts that half of the reasoning - there each token widens, and
// the cost of a long query is a large match set that `ORDER BY rank` scores in
// full - but it does not move the number: 32 bounds the expression either way,
// and what a large match set costs is spec 7.1's measurement rather than this
// cap's business.
//
// maxTokenBytes is 512 because a token is one whitespace-delimited run, and the
// longest one a person realistically types is a pasted absolute path. Windows'
// classic MAX_PATH is 260 characters, so 512 bytes leaves a whole path room
// even where its components are Korean - three bytes a syllable in UTF-8. It
// counts bytes rather than runes for the same reason the cap exists at all:
// what is bounded is the expression handed to SQLite.
const (
	maxQueryTokens = 32
	maxTokenBytes  = 512
)

// queryTokens splits the text a person typed into the tokens a query is built
// from, on Unicode whitespace and nothing else, and applies the two bounds.
//
// The tokens rather than the expression are what this returns, because more
// than the expression needs them: an excerpt has to find a query token in the
// text it is cutting, and re-splitting the same string in a second place is how
// the two quietly stop agreeing. [matchExpression] builds the expression from
// what this returns.
//
// No punctuation is stripped and nothing is interpreted - see
// [matchExpression]. A token that unicode61 tokenizes to nothing, `---` or a
// lone `"`, is legal input here and is not special-cased.
func queryTokens(text string) ([]string, error) {
	tokens := strings.Fields(text)
	if len(tokens) == 0 {
		return nil, ErrEmptyQuery
	}
	if len(tokens) > maxQueryTokens {
		return nil, fmt.Errorf("%w: %d tokens, cap %d", ErrTooManyTokens, len(tokens), maxQueryTokens)
	}
	for i, tok := range tokens {
		// The token itself stays out of the message. It is text a
		// person typed and an error travels into logs (I-10, spec 7.5);
		// its position and its size say everything a caller can act on.
		if len(tok) > maxTokenBytes {
			return nil, fmt.Errorf("%w: token %d is %d bytes, cap %d", ErrTokenTooLong, i+1, len(tok), maxTokenBytes)
		}
	}
	return tokens, nil
}

// Match is how [matchExpression] joins a query's tokens, and it is on the
// exported surface because the two callers of this package need different
// answers and must not be able to inherit each other's by accident.
//
// # Why there are two
//
// Until 2026-09-04 there was one, the implicit AND below, and it is right for
// the thing this index was built for: a literal somebody pastes back, where
// every token is in the document by construction. It is wrong for a question.
// Measured over the owner's 50 English questions, an AND returns an **empty
// match set for 42 of them** and puts the answer in the top ten for 1; the same
// queries as an OR reach 25, against an oracle ceiling of 37 (memory spec
// rev.11). A two-phase shape - AND, then OR only when the AND came back empty -
// reaches 20, which is worse than the plain OR, because on the 8 questions where
// the AND does match something it finds 1 answer where the OR finds 6. So the
// AND was not protecting those either.
//
// # Why it is a parameter and not a new default
//
// The injector must keep [MatchAll]. Its abstention is calibrated against the
// size of the match set an AND produces - that is what M6's zero-byte claim
// rests on - and handing it a selector that matches most of the corpus changes
// M5, M6 and M10 in one move, for a feature that ships off and whose activation
// gate has not run. So the choice is written at each call site, in the shape
// [searchWith] already uses for the derived-field boost and for the same reason.
type Match int

const (
	// MatchAll joins the tokens with FTS5's implicit AND: a document
	// matches only if every token is in it. Known-item retrieval, and the
	// injector.
	MatchAll Match = iota
	// MatchAny joins them with OR: a document matches if any token is,
	// and bm25 is what sorts the result. Questions, which is the MCP and
	// CLI surface.
	MatchAny
)

// matchExpression turns tokens into the expression [Search] hands to MATCH:
// each one wrapped in double quotes with any embedded double quote doubled, a
// star outside the closing quote, joined by a space for [MatchAll] and by OR
// for [MatchAny].
//
// # 1.0 offers no query language
//
// No syntax is interpreted. A person's AND, OR, NOT, NEAR, parentheses, carets,
// colons and stars are content and are quoted like every other token, so the
// only operator in the expression is the join [Match] names and the only
// wildcard is the one added here. The cost is that a phrase search is not
// expressible, and that is accepted for 1.0: the alternative is handing FTS5
// syntax to whoever typed the query, and FTS5 answers a hyphenated identifier
// with `no such column: time`.
//
// [MatchAny] is not a query language either, and the distinction is worth
// keeping: the caller chooses the join, the person typing does not. A typed OR
// is still quoted content.
//
// Quoting is not defensive spelling, it is what makes the shapes this corpus is
// full of work at all (spec 5.7). Bare `token*` is a syntax error on a
// hyphenated identifier and on a Windows path. Inside the quotes FTS5 hands the
// string to the tokenizer instead of to the query parser, so `"run-time"*`
// becomes the phrase `run` followed by a token starting with `time`, and a
// quoted Windows path becomes a phrase of its components. The star goes outside
// the closing quote because inside it is one more character to tokenize.
//
// The trailing star is not an optimisation either. unicode61 does not split a
// Latin stem from an attached Korean particle - `Codex는` is one token - so an
// exact search for `Codex` misses it, and a two-character Korean query matches
// nothing without it (spec 5.7).
func matchExpression(tokens []string, m Match) string {
	var b strings.Builder
	for i, tok := range tokens {
		if i > 0 {
			if m == MatchAny {
				b.WriteString(` OR `)
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('"')
		b.WriteString(strings.ReplaceAll(tok, `"`, `""`))
		b.WriteString(`"*`)
	}
	return b.String()
}
