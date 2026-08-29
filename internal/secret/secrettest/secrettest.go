// Package secrettest generates credential-shaped strings at run time for tests
// of the secret detector.
//
// Nothing secret-shaped is committed to this repository. `origin` is public, a
// committed known-bad file trips push protection, and a deliberate one is
// indistinguishable from a real leak. Every value here is assembled in memory
// when it is asked for.
//
// Each sample matches exactly one of spec 6.1's classes, and that is
// load-bearing rather than tidy: the detector's per-class tests assert an exact
// class set, so a sample that matched two classes would make neutering either
// class redden both tests, and neither would say what it looks like it says.
// The random bodies are lowercase-only for the same reason - the base32
// alphabet holds no '-' or '_', and lowercase cannot spell AKIA, so a body
// cannot accidentally grow a second class's shape.
package secrettest

import (
	"crypto/sha256"
	"encoding/base32"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/engramux/internal/secret"
)

// Sample is one generated credential-shaped string and the class spec 6.1 puts
// it in.
type Sample struct {
	// Class is the single class secret.Detect must report for Value.
	Class secret.Class
	// Shape names the spec 6.1 shape this sample stands for. It is unique
	// across All and is used as a subtest name.
	Shape string
	// Value is the whole generated string, as it would appear inside a payload.
	Value string
	// Secret is the substring of Value that must not survive masking. It is
	// the whole of Value for the classes that are masked whole, and the token,
	// password or username alone for the classes that keep their context.
	Secret string
}

// alnumRun matches a run of the characters every generated body is drawn from.
var alnumRun = regexp.MustCompile(`[A-Za-z0-9]+`)

// Needle is the longest run of ASCII letters and digits inside [Sample.Secret],
// and it is what an egress test should search for rather than Secret itself.
//
// Both reasons were found by review rather than by a failing test, which is
// what a silently inert assertion looks like.
//
//   - **Secret is too much.** Searching for the whole of it catches only a mask
//     that removed nothing. A mask that removes a shape's *prefix* and leaves
//     the generated body behind leaks almost all of the credential, while
//     Secret is no longer contiguous in the output and [secret.Detect] no
//     longer matches either - the shape it matches on is the part that was
//     removed. Both halves of such a test report clean. The body is the part
//     that must not survive, and it is what this returns.
//   - **Secret is the wrong bytes.** Spec 6.1's private-key shape carries real
//     newlines, and every JSON encoder writes one as the two characters '\'
//     and 'n' - so a raw search of an encoded document could not match whatever
//     the mask did. A run of letters and digits survives JSON encoding
//     unchanged, HTML escaping included.
//
// The run is the generated body in every shape [All] produces, 8 characters at
// the shortest and 46 at the longest. A Secret with no letters or digits at all
// is returned whole; nothing here generates one.
func (s Sample) Needle() string {
	runs := alnumRun.FindAllString(s.Secret, -1)
	if len(runs) == 0 {
		return s.Secret
	}
	return slices.MaxFunc(runs, func(a, b string) int { return len(a) - len(b) })
}

// b32 is the alphabet every generated body is drawn from: A-Z and 2-7, and
// nothing else. No '-', no '_', no lowercase in the raw form. That is what lets
// a body be dropped into any shape without growing a second class.
var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// nonce counts calls so two bodies generated inside one clock tick still
// differ. Windows clock resolution is about 550 us and these calls are back to
// back, so the timestamp alone would repeat.
var nonce atomic.Uint64

// chars returns n characters of the b32 alphabet.
//
// This is deliberately not crypto/rand: crypto/rand reaches
// crypto/internal/randutil, which imports math/rand/v2, which golangci-lint
// 2.12.2 cannot type-check against the Go 1.27 stdlib - it aborts the whole run
// with exit 7 rather than reporting an issue. Nothing here needs unpredictable
// bytes, only values that differ per call and cannot be mistaken for a real
// credential, so a hash of pid, clock and counter is enough.
func chars(n int) string {
	var b strings.Builder
	for b.Len() < n {
		seed := strconv.Itoa(os.Getpid()) + ":" +
			strconv.FormatInt(time.Now().UnixNano(), 10) + ":" +
			strconv.FormatUint(nonce.Add(1), 10)
		sum := sha256.Sum256([]byte(seed))
		b.WriteString(b32.EncodeToString(sum[:]))
	}
	return b.String()[:n]
}

// body is chars lowercased, which is what almost every shape wants.
func body(n int) string { return strings.ToLower(chars(n)) }

// hexBody maps a body onto the hex alphabet, for spec 6.1's "hex runs of 40
// characters or more".
func hexBody(n int) string {
	const digits = "0123456789abcdef"
	src := body(n)
	out := make([]byte, n)
	for i := range out {
		out[i] = digits[src[i]%16]
	}
	return string(out)
}

// whole is a sample whose entire value is removed by masking.
func whole(c secret.Class, shape, v string) Sample {
	return Sample{Class: c, Shape: shape, Value: v, Secret: v}
}

// All returns one freshly generated sample per shape in spec 6.1's table.
//
// gosec's G101 is entitled to flag this function: emitting credential-shaped
// strings is the whole point of it. Weakening a shape until a scanner stops
// recognising it would stop the detector recognising it too, and then every
// test that uses a sample would pass while proving nothing.
func All() []Sample {
	out := []Sample{
		whole(secret.ClassAPIKey, "sk-ant-", "sk-ant-api03-"+body(24)),
		whole(secret.ClassAPIKey, "sk-proj-", "sk-proj-"+body(24)),
		whole(secret.ClassAPIKey, "sk-", "sk-"+body(24)),
		whole(secret.ClassAPIKey, "github_pat_", "github_pat_"+body(30)),
		// The one shape spec 6.1 defines in uppercase: AKIA + 16 uppercase
		// alphanumerics, not a bare prefix.
		whole(secret.ClassAPIKey, "AKIA", "AKIA"+chars(16)),
	}
	for _, p := range []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"} {
		out = append(out, whole(secret.ClassAPIKey, p, p+body(30)))
	}
	for _, p := range []string{"xoxb-", "xoxa-", "xoxp-", "xoxr-", "xoxs-"} {
		out = append(out, whole(secret.ClassAPIKey, p, p+body(24)))
	}

	for _, kind := range []string{"RSA ", "OPENSSH ", ""} {
		out = append(out, whole(secret.ClassPrivateKey, "-----BEGIN "+kind+"PRIVATE KEY-----",
			"-----BEGIN "+kind+"PRIVATE KEY-----\n"+body(32)+"\n"+body(32)+"\n-----END "+kind+"PRIVATE KEY-----"))
	}

	for _, shape := range []string{"Authorization: Bearer", "Authorization:", "Bearer"} {
		tok := body(32)
		out = append(out, Sample{
			Class:  secret.ClassAuthorization,
			Shape:  shape,
			Value:  shape + " " + tok,
			Secret: tok,
		})
	}

	for _, name := range []string{"password=", "passwd=", "secret: ", "api_key: ", "api-key=", "token="} {
		v := body(20)
		out = append(out, Sample{
			Class:  secret.ClassCredential,
			Shape:  strings.TrimRight(name, "=: "),
			Value:  name + v,
			Secret: v,
		})
	}

	for _, scheme := range []string{"postgres", "mongodb+srv"} {
		pw := body(16)
		out = append(out, Sample{
			Class:  secret.ClassConnectionString,
			Shape:  scheme + "://",
			Value:  scheme + "://appuser:" + pw + "@db.internal.invalid:5432/appdb",
			Secret: pw,
		})
	}

	seed := body(20)
	out = append(out, Sample{
		Class:  secret.ClassDotenv,
		Shape:  "NAME=value block",
		Value:  "APP_MODE=production\nAPP_SEED=" + seed + "\nAPP_REGION=eu-west-1",
		Secret: seed,
	})

	out = append(out,
		whole(secret.ClassOpaque, "base64 run", body(46)+"=="),
		whole(secret.ClassOpaque, "hex run", hexBody(40)),
	)

	for _, shape := range []string{`C:\Users\`, "C:/Users/", `\\host\Users\`, "/c/Users/"} {
		user := "u" + body(7)
		sep := `\`
		if strings.Contains(shape, "/") {
			sep = "/"
		}
		out = append(out, Sample{
			Class:  secret.ClassUserPath,
			Shape:  shape,
			Value:  shape + user + sep + "dev" + sep + "main.go",
			Secret: user,
		})
	}
	return out
}

// Of returns the first sample of class c. It panics when c has no sample, which
// is a test wiring mistake and not a runtime condition.
func Of(c secret.Class) Sample {
	for _, s := range All() {
		if s.Class == c {
			return s
		}
	}
	panic("secrettest: no sample generates class " + string(c))
}
