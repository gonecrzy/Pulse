# Pulse + Sindri integration roadmap

**Status:** Workspace setup and planning only. No application code has been changed, no tests have been run, and no production instance has been touched.

## Workspace and baseline

- Primary checkout: `/root/Pulse`, remote `origin` = `https://github.com/gonecrzy/Pulse.git`.
- Upstream: remote `upstream` = `https://github.com/rcourtman/Pulse.git`.
- Working branch: `feature/sindri-integration`, created from the clean fork baseline `origin/main` at `7374abf19b434c9a275df730d28234bea3943774`.
- Fetched comparison: fork `origin/main` is **0 commits ahead and 3 commits behind** `upstream/main` at `9fb404604b006822ef3a007442bc5ac5d1a8a900`.
- The Sindri reference checkout was inspected read-only and left unchanged.

### Upstream review and base recommendation

The three upstream-only commits are merge commit `9fb404604`, `208f0cbf1` (“Preserve backup-age incidents across unknown success dates”), and `6fb43d930` (a follow-up correction to its browser proof fixture). The substantive change is in `internal/alerts/backup_snapshot.go`: a still-listed backup with a missing, zero, or future success timestamp no longer counts as recovery, and the newest usable provider timestamp wins. Its tests cover provider order, linked/orphan subjects, occurrence retention, and exclusions/removal. This is a safety-relevant alert correctness fix, not a Sindri feature dependency.

**Workspace branch base:** `7374abf19` is the current fork baseline; this setup branch remains there and no synchronization was performed. **Recommended implementation base:** `9fb404604b006822ef3a007442bc5ac5d1a8a900` (`upstream/main`), which is a fast-forward descendant of the fork tip and contains the backup-age safety fix and its proof. Review/approve that fast-forward before application work; do not move either branch automatically. If the fork maintainer chooses to stay on `7374abf19`, account for the backup-age fix before changing related alert semantics.

## Architecture and file ownership

Pulse stays the monitoring and operations application. Use its existing resource registry, alert lifecycle, Patrol/AI, action planner, action lifecycle, typed runner, metrics store, and SolidJS frontend. New adapters feed those systems. Do not add a second Sindri backend, incident lifecycle, or remediation engine.

| Priority | Capability and current gap | Pulse implementation area |
|---|---|---|
| 1 | **Personal-fork feature availability.** Add deployment/fork switches independently from commercial entitlements. Prefer a small fork-owned feature registry (for example `internal/features`) with config/API exposure and frontend presentation. Integrate with `internal/api`, `internal/config`, `frontend-modern/src/stores/sessionCapabilities.ts`, and existing settings surfaces. `pkg/licensing/feature_map.go` and `frontend-modern/src/stores/licenseEntitlements.ts` represent entitlements; do not silently turn fork switches into license grants. | New fork-level registry/config and tests; backend checks must enforce availability even when a UI hides a feature. |
| 2 | **Codex-LB exclusive AI gateway.** Pulse already has an AI provider registry, custom OpenAI-compatible endpoint support, and provider UI. Constrain the existing path rather than add an AI subsystem: `internal/config/ai.go`, `internal/ai/providers/`, `internal/ai/chat/`, Patrol in `internal/ai/`, AI routes in `internal/api/router_routes_ai_relay.go`, and the AI settings UI under `frontend-modern/src/features/settings/`. | One server-side provider route and model configuration; reject/disable alternate providers and prevent fallback around Codex-LB. Store credentials through Pulse's protected settings path, never source or planning documents. |
| 3 | **Native read-only Komodo monitoring.** No Komodo package or connector was found in the Pulse tree. Add a read-only provider adapter and feed canonical resources/observations through `internal/monitoring/` and `internal/unifiedresources/`; add connection/config handling through `internal/config/` and `internal/api/configapi/`. Reuse existing provider and integration settings UI patterns. | New provider package plus adapters and settings surface; no Komodo action executor or duplicate inventory database. |
| 4 | **Deterministic triage enhancements.** Pulse already has canonical alert evaluation/lifecycle, resource-aware incident synthesis, health assessment, and AI correlation/Patrol. Extend `internal/alerts/` and `internal/ai/correlation/` or a narrowly scoped deterministic package; pass conclusions and evidence into Patrol rather than creating another alert manager. Expose through existing alert/Patrol APIs and `frontend-modern/src/features/alerts/` and `features/patrol/`. | Facts, inferences, stale data, and unknowns remain distinct; deterministic results do not depend on model availability. |
| 5 | **Strict remediation policy and safety controls.** Keep plans in `internal/actionplanner/`, approvals/state in `internal/actionlifecycle/`, typed dispatch in `internal/actionrunner/`, and target/capability checks in `internal/agentcapabilities/`. Use existing action APIs and `frontend-modern/src/stores/actionInbox.ts` / action surfaces. | Extend the canonical action path. No model, Discord handler, or Komodo adapter dispatches around it. Default to approval-required; one executor per action class. |
| 6 | **Multi-alert operational investigation cases.** Pulse has alert incident synthesis in `internal/alerts/` and finding/investigation records in `internal/ai/investigation_records.go`, but these are not yet a durable operator-owned case spanning multiple alert occurrences. Add a case aggregate/persistence and API projection that references Pulse alert identities and Patrol investigations; extend alert/Patrol UI. | Preserve each alert's own lifecycle and identity. A case groups evidence and work; it does not replace alerts or Patrol. |
| 7 | **Long-term metric retention.** `pkg/metrics/` already owns SQLite history, rollups, pruning, and retention controls; `pkg/licensing` already defines `FeatureLongTermMetrics`. Extend limits, operator configuration, API history queries, and chart presentation only where requirements exceed the current store. | Measure row growth, write load, WAL/checkpoint, query cost, backup/restore, and retention boundaries before increasing retention. |
| 8 | **Discord conversational operations and approvals.** Pulse's `internal/notifications/` supports outbound Discord notifications via webhook/Apprise; no Discord command or interaction receiver was found. Add a transport adapter (new `internal/discord/` or a narrowly scoped notifications integration), authenticated actor mapping, and durable delivery/interaction handling. Route every approval to `internal/actionlifecycle/`; use existing scoped APIs and audit. | Discord is a transport, not an approval authority. Verify platform signatures or authenticated gateway identity, actor/role at approval and execution, one-time interaction identity, expiry, and idempotency. |

## Sindri behavior and test crosswalk

This is a behavior/test index, not a source-port list. The Sindri tests below identify scenarios to preserve; Pulse package tests and contracts are the starting point for equivalent coverage. Sindri's source was not copied.

| Sindri behavior and test anchors | Pulse equivalent or gap |
|---|---|
| Feature switches/capability contracts: `capabilities.py`, `config.py`; `tests/test_capabilities.py`, `test_configuration_boundaries.py`, `test_config.py`. | `internal/config`, `pkg/licensing/feature_map.go`, API entitlements and frontend session/license stores. Add fork-availability coverage as a distinct concept, including backend fail-closed behavior. |
| Codex-LB-only calls and provider failure handling: `ai.py`; `tests/test_ai.py`. | `internal/config/ai.go`, `internal/ai/providers/`, `internal/ai/chat/`; existing provider lifecycle/factory tests. Add tests proving all investigation/Patrol calls route through the configured gateway and cannot silently use another provider. |
| Read-only Komodo resource and workload synchronization: `komodo.py`; `tests/test_komodo.py`, `test_provider_native_checks.py`, `test_infrastructure_connections.py`. | No direct equivalent. Follow monitoring/provider integration tests, unified-resource adapter tests, config API tests, and least-privilege read-only contract tests. |
| Rule-based localization, source freshness, topology and recovery: `triage_engine.py`, `capabilities.py`, `topology.py`, `resolution_loop.py`; `tests/test_triage_engine.py`, `test_failure_localization.py`, `test_incident_aggregation.py`, `test_incident_reconciliation.py`, `test_resolution_loop.py`. | `internal/alerts/` lifecycle and incident synthesis, `internal/ai/correlation/`, Patrol triage and resource context. Extend table-driven tests for stale/missing/conflicting evidence and alert ordering/recovery. |
| Policy-gated typed remediation and verification: `remediation.py`, `remediation_authorization.py`; `tests/test_remediation.py`, `test_remediation_authorization.py`, `test_remediation_policy_routes.py`, `test_remediation_routes.py`. | `internal/actionplanner/`, `actionlifecycle/`, `actionrunner/`, `agentcapabilities/`; preserve approval, identity, audit, dispatch reconciliation, target freshness, and recovery verification at the canonical boundary. |
| Evidence-backed investigations and incident workflow: `incident_investigations.py`, `incident_projection.py`, `storage_incident_workflow.py`; `tests/test_incident_investigations.py`, `test_incident_workflow.py`, `test_incident_evidence.py`, `test_incident_projection.py`. | Patrol findings and `internal/ai/investigation_records.go` plus Pulse alert incidents are the nearest pieces. A durable multi-alert operational case is a gap and needs independent lifecycle/persistence/API/UI tests. |
| Historical metrics, rollups and cleanup: `storage.py`, `storage_metric_rollups.py`, `storage_retention.py`; `tests/test_metric_rollups.py`, `test_rollup_retention.py`, `test_storage_retention.py`, `test_retention_worker.py`. | `pkg/metrics/` and existing `pkg/licensing.FeatureLongTermMetrics`; validate tier boundaries, rollups, bounded maintenance, disk behavior, and backup/restore on Pulse's store. |
| Discord command/notification surface: `main.py`, `bot_commands/`, `notifications.py`; `tests/test_commands.py` and remediation tests. | Outbound Discord delivery exists in `internal/notifications/`; inbound conversations/approvals are a gap. New interaction tests must end at canonical action lifecycle APIs and must bind an individual actor. |

## Dependency-ordered roadmap and validation gates

Use an inline, incremental implementation on `feature/sindri-integration`, one reviewable phase at a time. Keep each phase revertible; stop before dispatch or cutover if its gate fails. These are proposed checks for later implementation, not checks run during workspace setup.

| Phase | Scope and dependency | Gate before proceeding |
|---|---|---|
| 0. Baseline and contracts | Decide whether the three upstream commits should be incorporated. Define public/private ownership, feature vs. entitlement semantics, Komodo read scopes, Codex-LB contract, action allowlist, identity, retention targets, and rollout thresholds. | Clean branch/base recorded; no sensitive runtime configuration in Git; API/resource/action contracts and owners written down. No application tests needed for this planning phase. |
| 1. Fork availability configuration | Add personal-fork feature registry and backend/UI exposure, separate from paid entitlements. | Go tests for feature config/API and frontend store/settings tests; prove disabled features are inaccessible through direct APIs, invalid configuration fails closed, and commercial entitlements retain their current meaning. Run Pulse's private-boundary audit. |
| 2. Codex-LB-only AI | Constrain provider selection and every Patrol/investigation call to the gateway. | `go test ./internal/config ./internal/ai/providers ./internal/ai/chat ./internal/ai`; frontend provider-settings tests. Mock-server tests cover auth, timeout, malformed response, redaction, no fallback, and AI-unavailable operation without breaking monitoring. |
| 3. Read-only Komodo | Add bounded polling, identity mapping, freshness, and unified resource adapters. | `go test ./internal/monitoring ./internal/unifiedresources ./internal/api/configapi` plus provider package tests. Contract fixtures prove read-only methods/scopes, stable IDs, pagination, stale/error states, no duplicate resources, and no mutation endpoint. |
| 4. Deterministic triage | Add rules over Pulse alerts/resources, Komodo observations, topology, and metric freshness; Patrol may explain evidence but cannot supply authorization. | `go test ./internal/alerts ./internal/ai/correlation ./internal/ai` plus alert API tests. Replay ordered, duplicate, stale, contradictory, unknown, and recovery fixtures; decisions must be deterministic and unknown must not become healthy. |
| 5. Strict action policy | Extend the existing planner/lifecycle/runner path with exact target/action policy, approval floor, limits, audit, and post-action verification. | `go test ./internal/actionplanner ./internal/actionlifecycle ./internal/actionrunner ./internal/agentcapabilities`; fake transports prove denied/stale/ambiguous/concurrent/expired requests never dispatch, approved requests dispatch once, and uncertain outcomes reconcile without blind retry. |
| 6. Long-term metrics | Configure retention/rollups against an explicit storage and freshness budget. | `go test ./pkg/metrics ./pkg/licensing` plus API history tests and a disposable load/restore qualification. Show bounded DB/WAL growth, query latency, pruning behavior, tier behavior, and verified restore. |
| 7. Multi-alert operational cases | Group alert occurrences and evidence into a persisted case; link Patrol investigations and action records. Depends on phases 4 and 6. | Alert/AI/API tests plus SolidJS case-workspace tests. Cover dedupe, reorder, split/merge, reopen, partial recovery, permissions, evidence links, and isolation of individual alert lifecycle. |
| 8. Discord operations and approvals | Add authenticated interaction transport and durable outbound/inbound state. Depends on phases 5 and 7. | Discord adapter and action lifecycle tests cover signature/actor/role checks, replay, duplicate interaction, expiry, role revocation, outbox retry, outage, and idempotent approval. Staging-only end-to-end proof; no production deployment is part of this plan. |

Across phases, run the repository's standard formatting/static checks and focused Go/SolidJS suites. Run broader CI suites before merge. Add end-to-end or live-provider checks only against disposable/staging systems with read-only credentials until a separately approved action qualification. Do not create or exercise a production cutover as part of implementation.

## Public/private and license boundary

- Pulse's root `LICENSE` is MIT. Preserve its copyright/license notices in any copied or substantially reused Pulse source.
- `docs/REPO_BOUNDARY.md` marks Pulse core public and defines private enterprise/business ownership; private in-app behavior crossing that boundary must use the established `pkg/...` contracts rather than leaking private implementations into `internal/...`.
- No tracked root `LICENSE`, `COPYING`, or `NOTICE` was found in Sindri. Treat its implementation as reference behavior only until source provenance/license is resolved. Independently implement against Pulse contracts; do not copy Sindri code or architecture report details into the product.
- Keep personal deployment switches, gateway endpoints, tokens, infrastructure names, allowlists, and Discord identities out of public defaults, fixtures, telemetry, and checked-in docs. Keep user-specific values in protected runtime configuration.
- Do not use a fork flag to bypass Pulse commercial entitlement checks. Phase 0 must settle how a personal deployment may enable features whose existing Pulse surfaces are entitlement-gated, especially action approvals and extended retention.

## Open implementation blockers

1. **Upstream baseline decision:** whether to fast-forward the fork to `9fb404604` before application work, or retain `7374abf19` and account for the backup-age fix separately.
2. **Feature ownership:** exact feature-switch list and which toggles are fork-only, public-core, or private/commercial; whether any entitlement-gated surface is available to this personal fork.
3. **AI contract:** Codex-LB model/API compatibility, timeout/budget expectations, secret source, and approved endpoint configuration. No live endpoint or credential was inspected.
4. **Komodo contract:** supported Komodo version, read-only token scope, resource types/fields, identity mapping, polling limits, and expected freshness. Do not add repair parity for the initial release.
5. **Safety policy:** explicit action/resource allowlist, approval actor identity, max blast radius/concurrency, cooldowns, and recovery evidence criteria. No remediation can be enabled until these values are reviewed.
6. **Metric budget:** retention horizon, resolutions, disk/WAL budget, backup location/restore target, and latency limits.
7. **Discord identity:** bot/interaction mode, allowed users/roles, revocation behavior, and fallback for approval while Discord is unavailable.

These decisions do not block the repository setup or future read-only development, but they block the corresponding live integrations and any action authority. The existing Sindri reports contain additional contract questions; consult them read-only if a later phase needs those details.

## Git checkpoint discipline

- Work on `feature/sindri-integration`; create descriptive, logically separated commits for each approved implementation phase.
- Run the phase's focused tests and relevant broader checks before committing, and record exact commands/results with that checkpoint.
- Push completed review checkpoints to `origin/feature/sindri-integration` so the architecture reviewer can inspect them on GitHub. Report branch, commit SHA, changed files, tests, and remaining issues.
- For policy, authentication, action execution, AI tools, or feature-entitlement changes, publish and review a checkpoint before starting dependent work.
- Never commit secrets, environment files, production credentials, or private Sindri artifacts. Do not push directly to `main`, force-push a reviewed checkpoint, or merge without explicit authorization.
