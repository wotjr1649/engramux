# Admission contrast and inference feasibility, 2026-09-08

Restricting English `continue` and `resume` to the first token, or the second token after
`please`, fixes the original keyword-explanation example but does not settle admission.
The position arm leaves v2's other cues unchanged. It is a separate test-only hypothesis;
the v2 candidate, private replay files and excerpt judgements were not changed.

An independent reviewer supplied contrast cases before this probe. Root adapted three cases
to exercise the known unquoted keyword defect, literal word output, and a fully supplied schema.
The remaining cases retain the reviewer's phrasing. Ambiguous API-review and short Korean
continuation requests were kept unknown. Labels are exploratory synthetic estimates, not
owner judgements or a claim that prompt text reveals all current-context needs. In particular,
negating continuation is not a universal prohibition on consulting any earlier information.

`go test -p 1 -count=1 -timeout 2m -run '^TestMeasureAdmissionPositionContrast$' -v
./internal/inject` ran the ten cases:

| Arm | Needed admitted | Estimated unnecessary admitted | Ambiguous admitted |
| --- | ---: | ---: | ---: |
| Continuation v2 | 3/3 | 4/5 | 2/2 |
| Imperative position | 3/3 | 2/5 | 2/2 |

The remaining negative examples are a negated continuation containing `previous`, and an
explanation of a `previous` schema field whose definition is already supplied in the prompt.
The test reports the counterexamples; a passing test process is not a quality pass. This
position rule is not adopted and the 108-prompt holdout remains unopened. No new word-specific
exception was added to the v2 selector. The pinned linter exited 0.

## A technical assumption that needs current verification

The memory architecture defers local embedding inference because its named routes require a
sidecar or C runtime, and M-1 prohibits this product's own LLM calls and summarisation. Those
decisions were not changed. Current upstream sources show a different implementation route
worth assessing before concluding that every local semantic approach crosses the runtime
boundary: [GoMLX's compute repository](https://github.com/gomlx/compute) provides a pure Go
backend. This is a vendor capability statement, not a verified Engramux build or performance result.

[Hugot's README](https://github.com/knights-analytics/hugot) describes a no-cgo Go backend,
feature extraction and classification pipelines, and warns that the Go backend is intended
for smaller workloads/models rather than strong performance requirements. Its current
[Go-session implementation](https://github.com/knights-analytics/hugot/blob/main/hugot_go.go)
imports `github.com/gomlx/compute/gobackend` directly. These observations suggest a possible
in-process route; they do not establish Korean relevance or distinguish topic similarity
from a need for historical context.

The inspected [module file](https://github.com/knights-analytics/hugot/blob/main/go.mod) declares
Go 1.27.0 and dependencies for multiple backends. Merely reading that file does not prove
which libraries a no-cgo build includes or whether any startup path installs a runtime.
These links were inspected on 2026-09-08 at mutable upstream heads; no version has been
selected for adoption. Before any implementation decision, a specific version, tokenizer,
model licence, supported operators, Windows build, initialization behavior, resident memory
and warm/cold latency would need review and measurement. A public-fixture feasibility probe
would still not constitute a product design change or a selection-quality pass.

No model was downloaded, dependency added, inference run, host configuration changed or
private prompt sent to an external inference service. Semantic-model feasibility remains
unverified. Existing runtime/privacy boundaries, original labels and activation criteria stand.
