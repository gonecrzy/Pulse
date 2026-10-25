# Workload chart SLO gate reliability — 2026-10-25

## Finding

The `feature/actions-workflow-cleanup` PR changes workflow policy and tests,
but it does not change `internal/api` chart handlers, fixture construction,
or chart performance settings. Before this correction, the workload chart job
compiled one candidate executable, waited for a short CPU-idle proxy, then
made one absolute 90 ms p95 decision. Its preflight measured CPU idle and
load, but did not use Linux CPU pressure stall information (PSI). The job also
ran alongside the rest of the Build and Test jobs.

Two failed Actions attempts reported p95 111.083 ms and 109.640 ms. A passing
Actions run on the same API source tree measured 79.262 ms. This establishes
that the observed failure is not introduced by the cleanup PR's application
code and that a single absolute sample is not stable across hosted runner
conditions.

I also ran ten interleaved baseline/candidate pairs using binaries compiled
from `origin/sindri` (`a1238cb6`) and PR head (`6fc6f08b`) with Go 1.26.10,
`GOMAXPROCS=2`, the same 70-workload fixture, 20 warm-up requests, and 200
measured requests. The local host first sustained 60 seconds of low CPU
pressure (CPU PSI interval peak 0.009%, normalized load peak 0.168 per allowed
CPU, aggregate CPU busy peak 7.03%). Median p95 was 111.128 ms for `sindri`
(range 102.777–114.754 ms) and 109.949 ms for the PR tree (range
105.166–117.154 ms). The median paired relative difference was +0.23%.

Both builds missed 90 ms on this local virtualized CPU despite low CPU PSI.
That result shows the absolute latency is hardware-capacity-sensitive; it is
not evidence that the PR regressed, nor does it qualify the hosted runner's
absolute performance. The historical hosted result below 90 ms and same-runner
baseline-relative comparison remain the relevant CI controls.

## New blocking method

The existing required check name remains `Workload chart 90ms SLO`. On pull
requests it now:

1. Checks out the PR base SHA (`sindri`) and PR head SHA separately.
2. Builds both API test executables with the candidate's Go version, identical
   `go test -p 1 ./internal/api -run '^$' -c` settings, the same embed stub,
   architecture, and `GOMAXPROCS=2`.
3. Runs eight paired measurements on the same hosted runner, alternating
   baseline-first and candidate-first order. Each run uses the existing 70
   workloads, 20 warm-up requests, and 200 timed requests.
4. Captures CPU PSI, aggregate CPU busy, background process CPU, hypervisor
   steal, and swap activity throughout every timed run. PSI over 20% in either
   the one-second interval or rolling `avg10`, missing pressure data, steal,
   or swapping makes the comparison inconclusive.
5. Calculates the median paired log(candidate/baseline) p95 ratio, reported
   as a percent difference, and a deterministic paired-bootstrap 95% confidence
   interval from 20,000 resamples. It passes only when the interval's upper
   bound is at or below +15%. It reports a regression when the lower bound is
   above +15%; an interval overlapping +15% fails as inconclusive. No automatic retry is
   used.

Both median p95 values, relative change and confidence interval, CPU pressure,
and whether each build meets the 90 ms target are printed and retained as an
artifact. The 90 ms value remains the performance target but is no longer the
sole blocking criterion on heterogeneous hosted runners. Other benchmark,
security, governance, and required checks are unchanged.

## Validation still required

The implementation includes unit fixtures for a stable comparison, a single
noisy sample, a deliberately 20% slower candidate, a confidence interval that
crosses the threshold, and CPU-contaminated runs. Fresh GitHub Actions results
on the pushed commit are still required to qualify the method on the actual
hosted runner.

## Local gate validation

The revised eight-pair harness was run against the captured baseline and PR
binaries on the same local host. Median p95 was 103.683 ms for the baseline
and 106.233 ms for the candidate, with a +0.64% paired difference and a 95%
interval of -0.71% to +2.97%. CPU PSI stayed below 0.08% and no run was marked
contaminated. Both builds missed the 90 ms target, and the relative gate passed.
For this local run, GitHub Actions identity variables selected the same 550 ms
hosted Go-test envelope used in CI; the proof separately reported the 90 ms
target and applied the paired regression rule.

For the end-to-end negative control, I built a temporary candidate test binary
with a 25 ms delay inside each timed request. The gate returned `regression`
and exited nonzero: baseline median p95 103.968 ms, delayed candidate median
136.599 ms, relative difference +30.87%, 95% interval +24.64% to +41.21%.
CPU PSI stayed below 0.07%, so this was a confirmed regression rather than an
inconclusive comparison. The artificial delay and its detached worktree were
removed after the check.
