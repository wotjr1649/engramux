# Search measurement clock, 2026-09-08

Two full regression runs on the session-resume branch failed different existing scaling
gates. The event ratio was 3.04 against 3.00; on the next run it passed and the native-memory
ratio was 2.76 against 2.00. Neither search statement had changed. The original observations
remain in `session-resume-2026-09-08.md`; an isolated event run passing was not treated as a fix.

`TestDiagnoseSearchCostPhases` reads the existing synthetic gate corpora with aliases of the
actual query builders, then separately calls the actual masking/excerpt functions. Every
instrumented result must equal the shipped search's complete result and count. It does not
change the gates, use private captures, or install instrumentation in the product.

The initial Go-clock probe returned zero for several nonempty processing phases. Inspection
of the installed Go toolchain's `runtime/time_windows_amd64.s` and `runtime/sys_windows_amd64.s`
found both clocks reading `_INTERRUPT_TIME`. Windows provides `QueryPerformanceCounter` and
its queried frequency for precise elapsed-time measurements. An initial diagnostic called
these fixed system functions directly and changed no timer resolution or host setting.
[Microsoft's interval-timing guidance](https://learn.microsoft.com/en-us/windows/win32/sysinfo/acquiring-high-resolution-time-stamps)
describes this use and the distinction from low-resolution interrupt time.

The command run was `go test -p 1 -count=1 -timeout 2m -run
'^TestDiagnoseSearch(ClockResolution|CostPhases)$' -v ./internal/search`, with
`ENGRAMUX_SEARCH_COST_DIAGNOSTIC=1`. The paired-clock run passed in 12.10 s. In its 50,000
adjacent samples, the Go value was unchanged 49,994 times and QPC 22,168 times at 10 MHz.
The largest Go step in that short probe was 544.9 microseconds; this is an observation,
not a bound on later timestamp error under load.

Both clocks then measured the same operations:

| Phase / example | QPC | Go clock |
|---|---:|---:|
| Event thin SQL, run 3 | 34.4557 ms | 29.3339 ms |
| Event thin postprocessing, run 1 | 0.5967 ms | 0 ms |
| Memory thin SQL, run 1 | 6.0297 ms | 2.5208 ms |
| Memory fat SQL, run 1 | 6.6577 ms | 11.4833 ms |
| Memory fat postprocessing, run 2 | 1.3291 ms | 0 ms |

The memory SQL ratio for that paired run is about 1.10 by QPC and 4.56 by the Go clock.
This establishes a measurement discrepancy on the same work, rather than inferring one from
separate successful and failing runs. Event postprocessing was also material: roughly 29–31 ms
for the twenty returned fat documents, versus about 0.3–0.6 ms for thin ones. A total-duration
ratio includes that legitimate returned-document cost and cannot attribute every excess to SQL.

The direct-call prototype passed the full suite, but the linter required an unsafe-call audit
(`G103`). It was removed rather than suppressing the findings. The retained implementation
uses `testing.Benchmark` and `B.Elapsed`: the installed Go toolchain's `testing/testing_windows.go`
implements that clock with QPC and its frequency. It introduces no direct native calls,
unsafe pointers, dependency, timer-resolution change or host setting.

The two scaling gates retain their corpora, matching population, returned-row limits,
one warm-up followed by best-of-three rule, and 3.00 / 2.00 thresholds. The standard benchmark
lifecycle calibrates by repeating complete four-call groups; the final group's result is
reported, not the minimum across calibration groups. No shipped Go file changed. With the
direct-call QPC prototype, the first focused run measured 2.45 and 1.21.

The sensitivity check deliberately put the event payload into the pre-LIMIT window and used
it in the outer result: the event gate failed at **16.38**. After restoring it, the equivalent
native-memory mutation failed at **21.65**. Neither mutation was retained. Both unchanged
queries then passed at **2.30** and **1.23**. These prototype observations are not a claim that a
timing test can never vary. The correction improves the instrument without reducing its bounds
or removing its ability to detect the original defect.

The retained standard-benchmark implementation also passed the focused gates, initially at
2.39 and 1.16. Repeating the two deliberate SQL mutations against that implementation failed
at **14.22** and **19.04**, respectively. After restoring both statements, the combined
diagnostic and gate run passed: event ratio **2.98**, memory ratio **1.31**. The standard
benchmark clock measured a 1.6706 ms sampling interval in which the Go clock did not advance.
This also shows the remaining practical limit: accurate timing does not remove real variation
or the event arm's substantial postprocessing cost.

On `go1.27.0 windows/amd64`, the retained implementation then passed the complete
`go test -p 1 -count=1 -timeout 10m ./...` run, including the real local corpus checks.
The search package completed in 222.288 s. The pinned linter exited 0 with `0 issues.`
No analyzer suppression was added.

The focused race check also passed: `go test -p 1 -race -count=1 -timeout 5m -run
'^(TestGateThe(SearchDoesNotReadPayloads|MemorySearchDoesNotReadBodies)ItDoesNotReturn|TestDiagnoseSearchClockResolution)$'
-v ./internal/search`, using the existing C compiler with CGO enabled for this test-only run.
The event and memory ratios were **1.36** and **1.13**, and the package completed in 201.895 s.
This is a focused race result; the feature commit's separate hosted full race result is linked
in the session-resume evidence.
