# GitHub Actions in the Pulse fork

The `sindri` branch is the fork's default integration and stable image source.
Ordinary branches receive the full validation set through pull requests to
`sindri`; push-only duplicate suites are not run on feature branches. The
`main` and `feature/sindri-integration` branch filters were retired. The latter
branch is no longer present on the origin remote.

The fork does not run upstream commercial release, signing, installer
publication, Helm publication, demo-server, or private-repository automation.

## Branch inventory comparison

| Branch | Before | After this cleanup branch |
| --- | --- | --- |
| `sindri` | 10 workflow YAML files. Push/PR filters still included `main`, `sindri`, and the retired integration branch; Build and Test was not reusable. | 10 workflow YAML files. PR validation targets `sindri`; stable image publication calls shared validations and is triggered only from `sindri`. |
| `feature/komodo-phase2-foundation` | 11 workflow YAML files: the same 10 plus `komodo-phase2f-image.yml`. Its Build and Test already supported `workflow_call`; the Komodo image workflow used it. | Not changed by this PR. After review, merge updated `sindri` into this branch, preserve the Komodo image workflow and implementation, and reconcile its inventory/package-writer trust assertions. |

The remote `feature/sindri-integration` branch no longer exists, and there is no
open PR using it. The `main` branch remains in the fork but is not the default
or active integration target; its workflow filters are removed.

## Retained workflows

| Workflow | Purpose and preserved checks | Before | After / dependencies |
| --- | --- | --- |
| `build-and-test.yml` | Secret scan; change detection; frontend, Go, script, and build checks; chart latency proof; benchmarks. | Push and PR to `main`, `sindri`, and the retired `feature/sindri-integration`; manual. | PRs to `sindri`, manual, or called by image workflows. Broad suite; one call gates each image publisher instead of a second standalone push run. |
| `canonical-governance.yml` | Workflow trust tests, mutation registry checks, and repository governance tests. | Push/PR to `main`, `sindri`, and the retired feature branch; manual. | PRs to `sindri`, manual, or called before stable publication. The standalone push run is removed. |
| `repo-boundary-audit.yml` | API import, paid-surface, and private-source boundary checks. | Push/PR to `main`, `sindri`, and the retired feature branch; manual. | PRs to `sindri`, manual, or called before stable publication. The standalone push run is removed. |
| `test-e2e.yml` | Source-bound browser/integration tests, disposable test images, and agent-registration lifecycle. | Path-filtered push/PR across `main`, `sindri`, and the retired feature branch; manual. | Relevant PRs to `sindri`, manual. Expensive; remains optional for unrelated paths and no longer repeats after merge. |
| `patrol-qualification-regression.yml` | Patrol catalogue, scoring, replay, lifecycle, and permission regression. | Path-filtered push/PR across `main`, `sindri`, and the retired feature branch; manual. | Relevant PRs to `sindri`, manual. Unique qualification coverage. |
| `unified-agent-native.yml` | Native installer/agent checks on Linux, macOS, and Windows; FreeBSD cross-build. | Path-filtered push/PR across `main`, `sindri`, and the retired feature branch; manual. | Relevant PRs to `sindri`, manual. Expensive five-platform matrix; no duplicate post-merge run. |
| `public-docs.yml` | Documentation recipes, public safety, mirrors, and entry-point checks. | Docs-path PRs and docs-path pushes to `main`, `sindri`, and the retired feature branch. | Docs-path PRs to `sindri`. The main validation workflow also checks the shipped-doc mirror. |
| `sindri-build.yml` | Runtime Docker build/smoke, exact-SHA GHCR publish, then published-image Compose health, restart, digest update, rollback, and volume checks. | Pushes to `feature/sindri-integration` and `sindri`; manual gated to the default branch. | Pushes to `sindri`; manual gated to default. Calls Build and Test, Canonical Governance, and Repo Boundary Audit before Docker/publish. Only `publish` has `packages: write`. |
| `security-scan.yml` | `govulncheck` on both Go modules; full and production npm audits for five package roots. | Weekly schedule and manual. | Same weekly schedule and manual. Scheduled jobs explicitly require `refs/heads/sindri`; approximately 52 scheduled runs/year are retained. |
| `dependency-advisory-watch.yml` | Reads the default branch frontend manifest/lockfile and reports new npm advisories without executing the audited branch. | Daily schedule and manual. | Manual only. Removes about 365 scheduled runs/year while preserving the advisory check. |
| `komodo-phase2f-image.yml` (feature branch only) | Shared Build and Test, repository governance, test-image build, GHCR test tags, published-image verification. | Push to `feature/komodo-phase2-foundation`; manual. | Remains only on that branch. Test tags stay isolated from the stable Sindri image. |

The Komodo integration branch adds `komodo-phase2f-image.yml`. It is branch-
specific, calls the same Build and Test workflow, and publishes only its
feature-test tags. It is not part of the `sindri` workflow inventory and cannot
publish the stable image. Preserve that workflow and its trust test when this
policy is later propagated into the integration branch.

The retained PR checks are the same workflow/job checks currently used for
validation: Build and Test's secret scan, frontend/backend/script/build/SLO
jobs; Canonical Governance's `governance`; Repo Boundary Audit's `audit`; and
the path-matched Public docs `check`, Core E2E `E2E verdict`, Patrol
`deterministic-regression`, and Unified Agent Native platform/FreeBSD jobs.
The latter path-filtered checks cannot be required unconditionally because
GitHub does not emit them for unrelated changes. The stable image's validation,
runtime qualification, publish, and published Compose verification form a
post-push dependency chain, not PR branch-protection checks. The repository's
required-check settings are not stored in this tree and must be reviewed in
GitHub before merging this cleanup.

## Expected automatic-run reduction

The daily dependency watch drops from approximately 365 scheduled invocations
per year to zero; manual audits remain. The weekly security scan stays at about
52 invocations per year, all on `sindri`.

On a `sindri` push that touches code, the old filters could start up to eight
independent workflows: Build and Test, Canonical Governance, Repo Boundary
Audit, Core E2E, Patrol regression, native-agent checks, Public docs, and the
Sindri image pipeline. The revised branch push has one image orchestration run
which invokes Build and Test, Canonical Governance, and Repo Boundary Audit as
read-only reusable jobs before building and publishing. E2E, Patrol, native,
and docs validation remain on applicable PRs and no longer repeat after merge.
For feature pushes after this policy is propagated, the Komodo image workflow
remains the only automatic feature-branch pipeline.

## Workflow trust and publication

`python3 scripts/check_workflow_trust.py .github/workflows` checks pinned
dependencies, explicit permissions, runner budgets, checkout credentials, safe
expression handling, and other trust rules. Trust tests constrain reusable
workflows to local read-only validation jobs and package write to the stable
publisher's `publish` job. Every publisher uses the job-scoped `GITHUB_TOKEN`;
there are no personal access tokens or cross-repository credentials.

The stable channel is defined by the `sindri` source branch, not a mutable
container tag. Published images use the full source commit SHA, and Compose
accepts only that SHA tag or a recorded digest. This preserves rollback and
prevents feature branches from overwriting a stable tag. The image workflow
publishes only after shared validation, runtime build/smoke, then performs a
published-image Compose health, restart, digest update, rollback, and volume
retention check. No workflow deploys automatically.

## Fork and Actions settings

Workflow YAML cannot enable GitHub Actions at the fork level, change branch
protection, or grant GHCR package access. After review, an operator must enable
Actions, verify required PR checks and allowed-action policy, and grant the
repository's Actions token package-write access if GHCR rejects a publish.
Keep the fork's default branch set to `sindri`. Do not use a personal access
token to work around a GHCR permission failure.
