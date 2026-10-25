# GitHub Actions in the Pulse fork

The `sindri` branch is the default integration branch and the source for the
stable Pulse image. Ordinary code changes receive validation through pull
requests to `sindri`; image workflows reuse those same validation workflows
before building or publishing. The retired `main` and
`feature/sindri-integration` workflow filters have been removed.

The fork does not run upstream commercial release, signing, installer
publication, Helm publication, demo-server, or private-repository automation.

## Branch inventory

| Branch | Workflow inventory and policy |
| --- | --- |
| `sindri` | Ten workflows. Pull-request validation targets `sindri`. The Sindri image workflow runs on pushes to `sindri` and publishes a full-commit-SHA image after shared validation and image qualification. |
| `feature/komodo-phase2-foundation` | Eleven workflows: the Sindri validation set plus `komodo-phase2f-image.yml`. That feature workflow runs on pushes to this branch, calls the reusable Build and Test workflow, and publishes only Komodo test-image tags. |

The workflow cleanup from PR #2 is incorporated here. `build-and-test.yml`
does not start a duplicate full suite on branch pushes: it runs for pull
requests targeting `sindri`, supports manual execution, and is callable by both
image workflows. The Komodo feature-image workflow remains enabled on its
designated branch.

## Retained workflows

| Workflow | Purpose | Triggers and publishing policy |
| --- | --- | --- |
| `build-and-test.yml` | Secret scan, change detection, frontend and Go checks, script checks, backend build, paired workload-chart p95 regression gate, and benchmarks. | Pull requests to `sindri`, manual runs, and reusable `workflow_call`. It has read-only repository permissions and cannot publish packages. |
| `canonical-governance.yml` | Workflow trust, mutation registry, and repository governance checks. | Pull requests to `sindri`, manual runs, and reusable `workflow_call`. Read-only permissions. |
| `repo-boundary-audit.yml` | API import, paid-surface, and private-source boundary checks. | Pull requests to `sindri`, manual runs, and reusable `workflow_call`. Read-only permissions. |
| `sindri-build.yml` | Build and qualify the runtime image, publish the exact source-SHA image, then pull and verify the published digest with Compose. | Pushes to `sindri` and manual runs gated to the default branch. Calls all three shared validation workflows before image publication. Only its `publish` job has `packages: write`. |
| `komodo-phase2f-image.yml` | Reuse Build and Test, run governance checks, build and smoke-test the runtime target, publish feature-test tags, and verify the published digest. | Pushes to `feature/komodo-phase2-foundation` and manual runs from that branch. Only its `publish` job has `packages: write`; the published-image verification job has `packages: read`. |
| `test-e2e.yml` | Source-bound browser and integration acceptance, including disposable image and agent-registration lifecycle tests. | Relevant path-filtered pull requests to `sindri` and manual runs. Optional for unrelated paths because it does not emit an always-present verdict. |
| `patrol-qualification-regression.yml` | Patrol catalog, scoring, replay, lifecycle, and permission regression checks. | Relevant pull requests to `sindri` and manual runs. |
| `unified-agent-native.yml` | Native installer and agent checks on Linux, macOS, and Windows, plus the FreeBSD cross-build contract. | Relevant path-filtered pull requests to `sindri` and manual runs. |
| `public-docs.yml` | Validate shipped documentation, mirrors, entry points, and documented safety recipes. | Relevant documentation pull requests and pushes to `sindri`; validation only, with no documentation publishing. |
| `dependency-advisory-watch.yml` | Audit the default branch's frontend lockfile for new npm advisories. | Manual only. The daily schedule was removed; the manual audit remains. |
| `security-scan.yml` | Scan Go modules with govulncheck and audit locked npm graphs. | Weekly schedule and manual runs. Scheduled execution is restricted to `sindri`. |

## Stable and feature-image publishing

The stable image channel is selected by the `sindri` source branch. It publishes
an immutable full-commit-SHA tag and records the registry digest for reproducible
use and rollback. It does not move `latest` or `sindri` tags.

The Komodo Phase 2F workflow publishes two feature-test tags from the image
that passed the runtime smoke test:

- `ghcr.io/gonecrzy/pulse:komodo-test` is the current feature-test convenience
  tag.
- `ghcr.io/gonecrzy/pulse:komodo-<12-character-commit-sha>` identifies the
  source commit.

It verifies the source revision and `linux/amd64` architecture before
publication, then pulls and smoke-tests the published image by digest. These
tags cannot overwrite the stable image channel. Neither workflow deploys or
changes a running Pulse container. Use the digest printed in the workflow
summary when a reproducible test deployment is needed.

## Workflow trust and retained checks

`python3 scripts/check_workflow_trust.py .github/workflows` checks pinned
dependencies, explicit permissions, runner budgets, checkout credentials, safe
expression handling, and other trust rules. The reusable validation workflows
have `contents: read` permissions and no secrets. GHCR package-write permission
is limited to the `publish` job in each of the two image workflows; the
published-image verification job receives package-read access only. Publishers
use the job-scoped `GITHUB_TOKEN`, with no personal access tokens or
cross-repository credentials.

The retained PR checks include Build and Test's secret scan, frontend and
backend tests, scripts, build, paired chart regression gate, and benchmarks;
Canonical Governance; Repo Boundary Audit; and the path-matched Public docs,
Core E2E, Patrol, and Unified Agent Native jobs. Path-filtered checks cannot be
required unconditionally because GitHub does not emit them for unrelated
changes. Image qualification, publication, and published-image verification
are separate post-push dependencies rather than PR branch-protection checks.

## Schedule and execution reduction

Removing the daily dependency-watch schedule avoids about 365 scheduled runs
per year while preserving manual audits. The weekly security scan remains at
about 52 runs per year, all on `sindri`.

On a `sindri` code push, `sindri-build.yml` orchestrates the shared Build and
Test, Canonical Governance, and Repo Boundary Audit workflows before the image
build and publication. The separate full push suites for those checks were
removed; relevant PR checks and manual execution remain. On the Komodo feature
branch, `komodo-phase2f-image.yml` is the feature-image pipeline and reuses
Build and Test instead of defining a parallel validation suite.

## Fork and Actions settings

Workflow YAML cannot enable GitHub Actions at the fork level, change branch
protection, or grant GHCR package access. An operator manages those settings
in GitHub. Keep the fork's default branch set to `sindri`, review the required
PR checks and allowed-action policy, and grant the repository's Actions token
package-write access if GHCR rejects publication. Do not use a personal access
token to work around a GHCR permission failure.
