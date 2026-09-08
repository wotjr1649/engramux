package secret

import (
	"reflect"
	"regexp/syntax"
	"testing"
)

func TestCredentialDelimiterNecessaryCondition(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"", false}, {"ordinary words", false}, {"token without assignment", false},
		{"token=sample-value", true}, {"password: sample-value", true},
		{"toKen = sample-value", true}, {"ſecret: sample-value", true},
		{"token\n:\t sample-value", true}, {"token：sample-value", false}, {"token＝sample-value", false},
	} {
		if credentialTextPossible(tc.text) != tc.want {
			t.Fatal("delimiter condition differs")
		}
	}
}

// Conservatively ignore zero-width assertions: an accepting path without an
// assignment delimiter must not exist even under that relaxed language.
func canMatchWithoutAssignment(t *testing.T, re *syntax.Regexp) bool {
	t.Helper()
	switch re.Op {
	case syntax.OpNoMatch:
		return false
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpAnyChar, syntax.OpAnyCharNotNL, syntax.OpStar, syntax.OpQuest:
		return true
	case syntax.OpLiteral:
		for _, r := range re.Rune {
			if r == ':' || r == '=' {
				return false
			}
		}
		return true
	case syntax.OpCharClass:
		for i := 0; i < len(re.Rune); i += 2 {
			if re.Rune[i] != re.Rune[i+1] || (re.Rune[i] != ':' && re.Rune[i] != '=') {
				return true
			}
		}
		return false
	case syntax.OpCapture, syntax.OpPlus:
		return canMatchWithoutAssignment(t, re.Sub[0])
	case syntax.OpRepeat:
		return re.Min == 0 || canMatchWithoutAssignment(t, re.Sub[0])
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !canMatchWithoutAssignment(t, sub) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			if canMatchWithoutAssignment(t, sub) {
				return true
			}
		}
		return false
	default:
		t.Fatal("unreviewed regex operation")
		return true
	}
}

func TestEveryCredentialTextRuleRequiresAssignment(t *testing.T) {
	check := func(pattern string) bool {
		re, err := syntax.Parse(pattern, syntax.Perl)
		if err != nil {
			t.Fatal(err)
		}
		return canMatchWithoutAssignment(t, re)
	}
	for _, tc := range []struct {
		pattern string
		without bool
	}{
		{`token[:=]value`, false}, {`token[:=]?value`, true}, {`token:value|plain`, true},
		{`(?:token=){2}`, false}, {`(?:token=){0,2}`, true}, {`[a-z]+`, true},
		{`.`, true}, {`[=:]`, false}, {`[=:a]`, true}, {`[^:=]`, true}, {`(?i)token[:=]`, false},
	} {
		if check(tc.pattern) != tc.without {
			t.Fatal("necessary-condition verifier failed")
		}
	}
	count := 0
	for _, r := range rules {
		if r.class == ClassCredential {
			count++
			if check(r.re.String()) {
				t.Fatal("credential rule permits delimiter-free text")
			}
		}
	}
	if count == 0 {
		t.Fatal("credential registry empty")
	}
}

func TestDelimiterGuardPreservesOriginalSpans(t *testing.T) {
	reference := func(key, leaf string) []span {
		var out []span
		for i, r := range rules {
			for _, m := range r.re.FindAllStringSubmatchIndex(leaf, -1) {
				start, end := m[0], m[1]
				if r.re.NumSubexp() > 0 && m[2] >= 0 {
					start, end = m[2], m[3]
				}
				if end <= start || isPlaceholder(leaf[start:end]) {
					continue
				}
				out = append(out, span{r.class, start, end, i})
			}
		}
		if leaf != "" && !isPlaceholder(leaf) && credentialKey.MatchString(key) {
			out = append(out, span{ClassCredential, 0, len(leaf), len(rules)})
		}
		return out
	}
	for _, key := range []string{"", "text", "password", "access_token", "toKen"} {
		for _, prefix := range []string{"", "token", "password", "ſecret", "toKen", "api_key", "Authorization", "Bearer"} {
			for _, middle := range []string{"", " ", "=", ":", "\n:\t", "\" : \"", "：", "\x00="} {
				for _, suffix := range []string{"", "sample-value", "[redacted-credential]", " x=y", " bearer sample-value"} {
					leaf := prefix + middle + suffix
					if !reflect.DeepEqual(spansIn(key, leaf), reference(key, leaf)) {
						t.Fatal("span oracle mismatch")
					}
				}
			}
		}
	}
	if got := string(Mask([]byte(`{"password":"sample-value"}`))); got != `{"password":"[redacted-credential]"}` {
		t.Fatal("structured sensitive key bypassed")
	}
}
