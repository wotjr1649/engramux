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
// maxQueryTokens is 32 because the tokens are joined by an implicit AND, so
// every token after the first can only narrow the result - past a handful the
// intersection of that many prefix phrases is empty on any real corpus. 32 is
// far more than a person types and still a small expression to plan.
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

// matchExpression turns tokens into the expression [Search] hands to MATCH:
// each one wrapped in double quotes with any embedded double quote doubled, a
// star outside the closing quote, joined by spaces.
//
// # 1.0 offers no query language
//
// No syntax is interpreted. A person's AND, OR, NOT, NEAR, parentheses, carets,
// colons and stars are content and are quoted like every other token, so the
// only operator in the expression is the implicit AND between tokens and the
// only wildcard is the one added here. The cost is that a phrase search is not
// expressible, and that is accepted for 1.0: the alternative is handing FTS5
// syntax to whoever typed the query, and FTS5 answers a hyphenated identifier
// with `no such column: time`.
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
func matchExpression(tokens []string) string {
	var b strings.Builder
	for i, tok := range tokens {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('"')
		b.WriteString(strings.ReplaceAll(tok, `"`, `""`))
		b.WriteString(`"*`)
	}
	return b.String()
}
