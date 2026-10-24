# GitHub Actions in the personal fork

This directory contains the independent fork's build, validation, security, and
GHCR image qualification workflows. The fork does not run upstream commercial
release, signing, installer publication, Helm publication, demo-server, or
private-repository automation.

## Retained workflows

| Workflow | Purpose | Trigger notes |
| --- | --- | --- |
| `sindri-build.yml` | Build the runtime image, publish the exact source-SHA image to GHCR, then pull and qualify it with Compose. | Pushes to `feature/sindri-integration` and `sindri`; manual runs are restricted to the default branch. |
| `build-and-test.yml` | Secret scan, frontend validation, Go tests, script checks, and backend build. | Push and PR to `main`, `sindri`, or `feature/sindri-integration`; docs-only changes skip code jobs. |
| `canonical-governance.yml` | Local workflow trust, mutation registry, and repository governance checks. | Push and PR to `main`, `sindri`, or `feature/sindri-integration`. No cross-repository credentials. |
| `repo-boundary-audit.yml` | Enforce API, paid-surface, and private-source boundaries. | Pull requests, pushes to `main`, `sindri`, or `feature/sindri-integration`, and manual runs. |
| `test-e2e.yml` | Source-bound browser and integration acceptance. | Path-filtered PRs and pushes to `main`, `sindri`, or `feature/sindri-integration`; keep it optional in branch protection because no always-present verdict is emitted for unrelated paths. |
| `dependency-advisory-watch.yml` | Audit the current default branch's frontend lockfile for new npm advisories. | Daily schedule and manual runs; reads the default branch's lockfile as data. |
| `security-scan.yml` | Scan Go modules with govulncheck and locked npm graphs for advisories. | Weekly schedule and manual runs. No release artifacts or upstream image references. |
| `patrol-qualification-regression.yml` | Validate deterministic Patrol catalog, scoring, replay, and permission behavior. | Relevant PRs/pushes to `main`, `sindri`, or `feature/sindri-integration`, and manual runs; no scheduled release comparison. |
| `unified-agent-native.yml` | Run native installer and agent checks on Linux, macOS, and Windows. | Relevant path-filtered PRs/pushes to `main`, `sindri`, or `feature/sindri-integration`, and manual runs. |
| `public-docs.yml` | Validate shipped documentation, mirrors, and documented safety recipes. | Relevant docs PRs/pushes. This is validation only and does not publish documentation. |

## Trust checks

`python3 scripts/check_workflow_trust.py .github/workflows` checks every
retained workflow for pinned dependencies, explicit permissions, runner
budgets, checkout credentials, safe expression handling, and other workflow
trust rules. The security checks consume repository source and public package
registries only. The GHCR publisher is the only workflow with package-write
permission.

## Delivery

The supported delivery path is GitHub Actions to GHCR to Docker Compose or
Komodo. The image workflow builds from the exact triggering source revision,
qualifies the image before publication, publishes an immutable SHA tag, then
pulls that tag and verifies startup, health, and persistent-volume behavior.
