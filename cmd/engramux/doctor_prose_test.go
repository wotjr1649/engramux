package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/wotjr1649/engramux/internal/secret"
)

// TestDoctorsOwnProseSurvivesItsOwnMask holds an invariant nobody had written
// down, because the shape of the failure is not one a reader looks for.
//
// # What happened
//
// `doctor` masks the whole formatted line rather than each value, deliberately:
// a line assembled from two sources has no seam a caller could be trusted to
// mark. What follows from that and had not been noticed is that the format
// string is masked too - the product's own English is an input to its own
// redactor.
//
// The Authorization rule matches `bearer` followed by whitespace and a word,
// with no header name required, because a bare `Bearer xyz` in a log is the
// real shape. Two of this file's own sentences said "the bearer token", so
// `doctor` printed **"a copy of the bearer [redacted-authorization] in it"** -
// observed 2026-09-04 on the first install on a machine that had never run
// these binaries, in the very line that exists to tell a user their token has
// copies.
//
// # Why the fix is here and not in internal/secret
//
// The rule is right. Narrowing it so that English prose survives would weaken a
// control that exists because a bearer token can appear in a log with no header
// name in front of it, and a false positive costs a placeholder while a false
// negative costs a credential. The prose is what changes.
//
// # Why the source and not the output
//
// Asserting on a rendered report only covers the lines that report happens to
// produce, and the lines at risk are the ones somebody adds later. Every string
// literal in this file is every sentence `doctor` can print, so this reads them
// out of the syntax tree - which skips comments for free, and one of them
// legitimately contains `Bearer <token>`.
func TestDoctorsOwnProseSurvivesItsOwnMask(t *testing.T) {
	const src = "doctor.go"

	file, err := parser.ParseFile(token.NewFileSet(), src, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", src, err)
	}

	var checked int
	ast.Inspect(file, func(n ast.Node) bool {
		// An import path is a string literal this command never prints,
		// and two of them trip the opaque rule on their own: a module
		// path is a long run of letters, digits and slashes, which is
		// exactly what forty characters of base64 looks like. Skipping
		// the whole spec rather than pattern-matching the value is what
		// keeps that from being a hole a printed sentence could hide in.
		if _, ok := n.(*ast.ImportSpec); ok {
			return false
		}
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			// A raw string with an escape this unquoter will not take
			// is not a thing this file has, and guessing at one would
			// hide it.
			t.Errorf("unquote %s: %v", lit.Value, err)
			return true
		}
		checked++
		if masked := secret.MaskString(text); masked != text {
			t.Errorf("doctor prints a sentence its own mask rewrites.\n  wrote:  %q\n  prints: %q\n"+
				"the mask is right; the sentence is what changes. `bearer` followed by a word is "+
				"the shape that bites, because the Authorization rule needs no header name.",
				text, masked)
		}
		return true
	})

	// A parse that found nothing would pass silently, and this whole test
	// is about a failure that looks like nothing happening.
	if checked < 50 {
		t.Fatalf("only %d string literals found in %s, which is too few to be the whole file", checked, src)
	}
	t.Logf("%d string literals checked", checked)
}
