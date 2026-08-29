package secrettest_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/engramux/internal/secret"
	"github.com/wotjr1649/engramux/internal/secret/secrettest"
)

// payload wraps v in a hook-shaped JSON object. No key here names a credential,
// so a class the detector reports comes from the value and not from the key.
func payload(t *testing.T, v string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": v},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

// TestEverySampleIsDetectedAsExactlyItsOwnClass is the anti-vacuity gate. Every
// other test in this package family feeds the detector a generated sample; if a
// generator quietly emitted something harmless, those tests would still exercise
// the code path and could still be green while proving nothing about detection.
// This one fails the moment a generator stops emitting a credential shape.
//
// "Exactly" is the second half. The per-class detector tests assert an exact
// class set, which is only meaningful while each sample matches one class - a
// sample that matched two would make neutering either one redden both tests.
func TestEverySampleIsDetectedAsExactlyItsOwnClass(t *testing.T) {
	for _, s := range secrettest.All() {
		t.Run(string(s.Class)+"/"+s.Shape, func(t *testing.T) {
			got := secret.Detect(payload(t, s.Value)).String()
			if got != string(s.Class) {
				t.Errorf("Detect(sample) = %q, want exactly %q; the generator emits something the detector does not match, which makes every test that uses this sample vacuous", got, s.Class)
			}
		})
	}
}

// TestSecretIsTheRemovablePartOfValue pins the contract callers rely on when
// they assert absence after masking: Secret is a non-empty substring of Value.
func TestSecretIsTheRemovablePartOfValue(t *testing.T) {
	for _, s := range secrettest.All() {
		t.Run(string(s.Class)+"/"+s.Shape, func(t *testing.T) {
			if s.Secret == "" {
				t.Fatal("Secret is empty; an absence assertion against it would pass on anything")
			}
			if !strings.Contains(s.Value, s.Secret) {
				t.Errorf("Secret %q is not a substring of Value %q", s.Secret, s.Value)
			}
		})
	}
}

// TestNeedleIsALiveSubstringOfTheSecret pins what every egress test's absence
// assertion is built on.
//
// Three properties, and each one alone is satisfied by a Needle that would make
// those assertions vacuous: it has to be a substring of Secret, or it is not
// evidence about the secret at all; it has to be long enough not to collide
// with ordinary text in a reply document; and it has to survive JSON encoding
// unchanged, because every document a sweep reads came out of an encoder.
func TestNeedleIsALiveSubstringOfTheSecret(t *testing.T) {
	for _, s := range secrettest.All() {
		t.Run(string(s.Class)+"/"+s.Shape, func(t *testing.T) {
			n := s.Needle()
			if !strings.Contains(s.Secret, n) {
				t.Fatalf("Needle is not a substring of Secret")
			}
			// 8 is the shortest generated body in the table, the
			// user directory name. Anything shorter than that would
			// be a run this found in a shape's fixed prefix rather
			// than in the part that was generated.
			if len(n) < 8 {
				t.Fatalf("Needle is %d characters, too short to be the generated body", len(n))
			}
			b, err := json.Marshal(n)
			if err != nil {
				t.Fatalf("marshal the needle: %v", err)
			}
			if string(b[1:len(b)-1]) != n {
				t.Fatalf("the needle is rewritten by JSON encoding, so a search of an "+
					"encoded document could not find it: %d bytes became %d", len(n), len(b)-2)
			}
		})
	}
}

// TestAllCoversEverySpec61Shape pins the count spec 7.1 cites for the Phase 6
// audit.
//
// The audit's premise is that its payload carries every class, which a set of
// samples satisfies while missing individual shapes: drop the ghr_ sample and
// the other fourteen API-key shapes keep the class in the set. Then a rule for
// that prefix could be removed and the gate would still pass. The number is the
// only thing that catches it, and it is in the spec, so it is pinned here and
// the two move together.
func TestAllCoversEverySpec61Shape(t *testing.T) {
	const shapes = 36
	if got := len(secrettest.All()); got != shapes {
		t.Fatalf("All returns %d shapes, want %d. If a shape was added or removed on purpose, "+
			"spec 7.1's redaction-audit row carries this number too", got, shapes)
	}
}

// TestEveryClassHasASample fails when a class is added to the ruleset without a
// generator, which would leave it detected by nothing.
func TestEveryClassHasASample(t *testing.T) {
	var covered []secret.Class
	for _, s := range secrettest.All() {
		covered = append(covered, s.Class)
	}
	slices.Sort(covered)
	covered = slices.Compact(covered)
	if want := secret.Classes(); !slices.Equal(covered, want) {
		t.Errorf("samples cover %v, ruleset has %v", covered, want)
	}
}

// TestOfReturnsTheAskedForClass covers the accessor later tasks use to pull one
// secret for an end-to-end run.
func TestOfReturnsTheAskedForClass(t *testing.T) {
	for _, c := range secret.Classes() {
		if got := secrettest.Of(c); got.Class != c {
			t.Errorf("Of(%q).Class = %q", c, got.Class)
		}
	}
}

// TestSamplesAreGeneratedNotConstant is the "built in memory at test runtime"
// half of gate 4. A generator that returned a committed constant would pass
// every other test here.
func TestSamplesAreGeneratedNotConstant(t *testing.T) {
	first, second := secrettest.All(), secrettest.All()
	if len(first) != len(second) {
		t.Fatalf("All() returned %d then %d samples", len(first), len(second))
	}
	for i := range first {
		if first[i].Shape != second[i].Shape {
			t.Fatalf("sample %d changed shape between calls: %q then %q", i, first[i].Shape, second[i].Shape)
		}
		if first[i].Secret == second[i].Secret {
			t.Errorf("%s/%s produced the same secret twice; it is a constant, not a generated value", first[i].Class, first[i].Shape)
		}
	}
}
