package search

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// TestMatchExpression pins the exact expression every shape produces, because
// the rule is a string transformation and a property assertion cannot tell a
// correct one from a nearly correct one: `"run-time"*` and `"run-time*"` differ
// by one character and only the first is a prefix query.
//
// Every row is a shape spec 5.7 names or the corpus derives. The Windows path
// and the embedded quote are the two that a shell heredoc corrupts, which is
// why this file is written with a file-write tool (AGENTS.md).
func TestMatchExpression(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
		toks []string
	}{
		{"a plain word", "budget", `"budget"*`, []string{"budget"}},
		{"a hyphenated identifier", "run-time", `"run-time"*`, []string{"run-time"}},
		// Bare, this answers `no such column: C` (spec 5.7). Quoted, FTS5
		// hands the whole string to the tokenizer and it becomes a phrase.
		{"a Windows path", `C:\Users\fixture\main_test.go`, `"C:\Users\fixture\main_test.go"*`, []string{`C:\Users\fixture\main_test.go`}},
		{"a bare AND", "AND", `"AND"*`, []string{"AND"}},
		{"a lone double quote", `"`, `""""*`, []string{`"`}},
		// One of the corpus's own two-token derivations, cut to one token.
		{"an embedded double quote", `CD="C:/Temp";`, `"CD=""C:/Temp"";"*`, []string{`CD="C:/Temp";`}},
		{"a Korean word", "파일", `"파일"*`, []string{"파일"}},
		{"two tokens", "읽기 파일", `"읽기"* "파일"*`, []string{"읽기", "파일"}},
		// A token unicode61 tokenizes to nothing is legal input and is not
		// special-cased: FTS5 drops it from the AND rather than zeroing it.
		{"a token that tokenizes to nothing", "--- 파일", `"---"* "파일"*`, []string{"---", "파일"}},
		{"surrounding and repeated whitespace", "  foo\t\n bar  ", `"foo"* "bar"*`, []string{"foo", "bar"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toks, err := queryTokens(tc.text)
			if err != nil {
				t.Fatalf("queryTokens(%q): %v", tc.text, err)
			}
			if !slices.Equal(toks, tc.toks) {
				t.Errorf("queryTokens(%q) = %q, want %q", tc.text, toks, tc.toks)
			}
			if got := matchExpression(toks); got != tc.want {
				t.Errorf("matchExpression(%q) = %q, want %q", toks, got, tc.want)
			}
		})
	}
}

// TestQueryBounds pins the three refusals to their sentinels. Each is an error
// and not an empty result: a caller that cannot tell "you asked for nothing"
// from "nothing matched" reports the wrong thing to the person who typed it.
func TestQueryBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want error
	}{
		{"no text at all", "", ErrEmptyQuery},
		{"whitespace only", " \t\n ", ErrEmptyQuery},
		{"one token over the token cap", strings.TrimSpace(strings.Repeat("a ", maxQueryTokens+1)), ErrTooManyTokens},
		{"one byte over the token length cap", strings.Repeat("a", maxTokenBytes+1), ErrTokenTooLong},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toks, err := queryTokens(tc.text)
			if !errors.Is(err, tc.want) {
				t.Fatalf("queryTokens: err = %v, want %v", err, tc.want)
			}
			if toks != nil {
				t.Errorf("queryTokens returned %d tokens beside the error, want none", len(toks))
			}
		})
	}

	// The caps admit what sits exactly on them, so an off-by-one in the
	// comparison is a failure here and not a silently narrower search.
	t.Run("at both caps", func(t *testing.T) {
		at := strings.TrimSpace(strings.Repeat("a ", maxQueryTokens-1)) + " " + strings.Repeat("b", maxTokenBytes)
		toks, err := queryTokens(at)
		if err != nil {
			t.Fatalf("queryTokens at the caps: %v", err)
		}
		if len(toks) != maxQueryTokens {
			t.Fatalf("queryTokens returned %d tokens, want %d", len(toks), maxQueryTokens)
		}
		if len(toks[len(toks)-1]) != maxTokenBytes {
			t.Errorf("the last token is %d bytes, want %d", len(toks[len(toks)-1]), maxTokenBytes)
		}
	})

	// The cap counts bytes and not runes, because it is there to bound what
	// is handed to SQLite. One Hangul syllable is three bytes in UTF-8, so a
	// token of maxTokenBytes/3 syllables is at the cap and one more is over.
	t.Run("the length cap counts bytes", func(t *testing.T) {
		if _, err := queryTokens(strings.Repeat("가", maxTokenBytes/3+1)); !errors.Is(err, ErrTokenTooLong) {
			t.Errorf("queryTokens: err = %v, want %v", err, ErrTokenTooLong)
		}
	})
}
