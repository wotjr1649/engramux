package secret_test

import (
	"testing"

	"github.com/wotjr1649/engramux/internal/secret"
)

// TestPhase6TheMaskedCorpusIsCleanUnderARescan is the payload half of spec 8's
// Phase 6 redaction audit, run over the real corpus rather than over anything
// this repository could commit.
//
// # What it adds to the tests that already exist
//
// internal/secret's own tests put a generated sample of each of spec 6.1's
// shapes through [secret.Mask] and assert the sample is gone. That says the
// rules work on the shapes the rules were written for. It cannot say anything
// about a shape nobody thought of, and the corpus is the only population of
// real ones this machine has: 900 of 902 captures carry an absolute user path,
// and every credential class together matches 4 files (spec 6.1, 7.5).
//
// The assertion is idempotence stated as an invariant rather than as a
// property of the rules: mask a payload, run the same detector over the result,
// and it must report nothing. A class that survives its own mask is either a
// rule that does not remove what it matched, or a mask whose placeholder the
// rules match again - and both are the same failure at an egress, which is a
// secret leaving the machine.
//
// # Why this is not circular
//
// Detect and Mask share [spansIn], so it is fair to ask whether this can fail
// at all. It can, in both of the directions that matter. A rule that matches
// without a capturing group masks its whole match, and one with a group keeps
// the surrounding context - so masking rewrites a *subset* of what detection
// found, and the residue is exactly what a rescan sees. And isPlaceholder is
// what stops a placeholder from being re-detected at all.
//
// **[verified]** The break-it step is isPlaceholder returning false, which is
// the whole of the idempotence mechanism and nothing else: masking a clean
// payload is unchanged by it - no placeholder is present on the first pass -
// and this test reddens. Changing the placeholder's own spelling does *not*
// redden it, because placeholder and isPlaceholder read the same constant and
// move together; that mutation was tried first and it is not the break-it step.
//
// # Nothing here prints a payload or a file name
//
// The corpus is real prompts, real commands and real file contents (spec 6.2).
// A failure reports how many payloads carried a surviving class and which
// classes those were - never a byte of the document that carried them.
func TestPhase6TheMaskedCorpusIsCleanUnderARescan(t *testing.T) {
	payloads := corpusPayloads(t)

	surviving := make(map[secret.Class]int)
	dirty := 0
	tagged := 0
	for _, p := range payloads {
		if len(secret.Detect(p)) != 0 {
			tagged++
		}
		if classes := secret.Detect(secret.Mask(p)); len(classes) != 0 {
			dirty++
			for _, c := range classes {
				surviving[c]++
			}
		}
	}

	t.Logf("%d payloads, %d of them tagged before masking", len(payloads), tagged)

	// The premise, checked rather than assumed. A corpus the detector finds
	// nothing in would satisfy the assertion below without exercising a
	// single rule, and that is a real possibility for a fresh capture
	// directory rather than a hypothetical.
	if tagged == 0 {
		t.Fatal("no payload in the corpus carries a secret at all, so this audit sweeps nothing")
	}
	if dirty != 0 {
		t.Errorf("%d of %d masked payloads still carry a secret; surviving classes and counts: %v",
			dirty, len(payloads), surviving)
	}
}
