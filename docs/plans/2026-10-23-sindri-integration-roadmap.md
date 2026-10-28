Warning: truncated output (original token count: 10991)
Total output lines: 178

# Pulse + Sindri integration roadmap

**Status (2026-10-25):** Komodo Phase 2E monitoring integration is implemented on `feature/komodo-phase2-foundation` and is covered by fixtures. Live qualification passed against Core `2.3.4` through the operator-supplied `http://10.1.10.101:9120` LAN endpoint: 11 Servers, 32 Stacks, no Deployments or Swarms, 43 stable identities across two reads, 43 resources with state, and 32 resolved Stack-to-Server parent references. Normalization produced 43 records and 32 `hosted_by` relationships with no unresolved parents. All four inventory list methods succeeded. `GetPermission` returned level-only `Read` 86 times; effective specific permissions were operator-attested for all four kinds and were not API-verified. The HTTP exception exists only in the explicitly opted-in test harness for this exact address; production connection validation and the monitoring client remain HTTPS-only. Komodo polling still uses only allowlisted read methods. Read-only is the current Phase 2 implementation boundary, not a permanent product limitation: future provider actions must be separately enabled by Pulse policy and requested through Pulse's canonical authorization, approval, execution, audit, and verification lifecycle. The upstream Komodo credential alone never grants Pulse action authority. Phase 2F adds a branch-scoped test-image workflow and isolated Compose procedure; the first registry result is pending the post-push workflow run, so no GHCR digest is asserted here. No action methods, merge, or production deployment have occurred. Codex-LB remains optional later work; Phase 0 read-only and action-safety contracts remain in force.

### Phase 2F — Automated test image and isolated acceptance

The `komodo-phase2f-image.yml` workflow runs only for pushes to
`feature/komodo-phase2-foundation` and manual dispatches targeting that branch.
It calls the existing full `build-and-test.yml` workflow and runs the
`canonical-governance.yml` checks, builds the existing production
`Dockerfile` `runtime` target for `linux/amd64`, and runs
`deploy/sindri/verify-image.sh` before publication. A separate publisher job
has the only `packages: write` permission and publishes
`ghcr.io/gonecrzy/pulse:komodo-test` plus
`ghcr.io/gonecrzy/pulse:komodo-<first-12-commit-characters>`. The workflow checks
that both tags resolve to the same digest, then pulls by digest and repeats the
runtime/health/data-persistence smoke test. It does not publish stable, latest,
production, or Sindri tags and does not deploy.

Existing `.github/workflows/sindri-build.yml` remains the exact-full-SHA GHCR
pipeline for `feature/sindri-integration` and `sindri`; its triggers and tag
convention are unchanged. `build-and-test.yml` is callable so the Phase 2F
pipeline reuses the existing full test suite. The Phase 2F workflow runs the
same bounded checks as `canonical-governance.yml`; that workflow's triggers and
permissions remain unchanged. The personal fork has no
separate upstream commercial release/signing/installer publication pipeline.

`deploy/komodo-phase2f/compose.yml` defines a distinct fixed container and
network, loopback port, and explicitly supplied persistent bind directory. Its
`.env.example` contains only image digest, data path, port, and timezone; the
Komodo credential is entered through Pulse's protected UI. The live Compose
acceptance remains an operator action and requires trusted HTTPS/DNS on the test
host. The workflow itself uses fixtures and local smoke tests only and has no
Komodo or Proxmox credentials.

### Phase 2G — End-to-end topology and incident correlation audit

**Status (2026-10-26):** The canonical identity, Komodo topology, availability
association, incident correlation, Patrol query context, and resource detail
surfaces were audited and repaired on `feature/komodo-phase2-foundation`.
Changes reuse the existing `ResourceRegistry`, directed relationship graph,
alert synthesis, and `pulse_query` tool. The audit used synthetic fixtures
only; it did not contact or disrupt production infrastructure.

| Subsystem | Existing behavior | Tested? | Confirmed defect | Fix |
|---|---|---|---|---|
| Identity reconciliation | Explicit organization-scoped Proxmox/Komodo identity links persist and retain Proxmox metrics targets. No name-based merge is performed. | Yes: persisted synthetic `www-DMZ` Proxmox VMID 50110 ↔ Komodo Server across store restart; existing Jellyfin identity/link tests remain. | No defect found in explicit link resolution. `Alfheim` and `Public-DMZ` operator mappings are not present in repository fixtures or a live snapshot, so those specific mappings remain unverified. | Added an end-to-end test proving one canonical host, observations from both providers, VM history target preservation, Stack hosting, and check association after restart. |
| Komodo topology ingestion | Complete inventory snapshots normalize Server/Stack parent references into canonical `hosted_by` relationships. | Yes: normalized parent, provenance, and restart tests. | Normalization emitted a dangling active edge when a referenced Server/Swarm was absent from the same inventory, and the Komodo edge omitted explicit provider provenance. | Retain the provider parent reference for later reconciliation, emit an active edge only when its parent is present, and stamp Komodo provenance. |
| Availability association | Availability checks are separate canonical network-endpoint resources, linked with `checks` edges and projected onto their selected resource. | Yes: Stack-linked synthetic check through store restart; existing change/remove and reconciliation tests. | The resolved canonical target name/ID was not carried into the check view, and details could miss inbound check edges. | Record derived `resolvedResourceId` and target name separately from the configured link; project the canonical graph in both directions. |
| Incident correlation | Active relationships supply incident context; alert lifecycle handles deduplication and recovery. | Yes: stale-edge regression plus existing recovery, notification, and dedup tests. | A stale provider relationship could still justify causal suppression of an independently valid availability alert. | Only current evidence can suppress a supported infrastructure symptom. Stale/unknown edges remain context but cannot suppress a valid independent alert. |
| Notifications | Existing notification lifecycle delivers fired/recovered alerts and deduplicates repeated incidents. | Yes: existing lifecycle tests and the independent availability alert regression. | The stale relationship could prevent delivery before NotificationManager received the check alert. | Keep notification manager behavior unchanged; preserve the alert until normal lifecycle dedup/recovery policy applies. |
| Patrol / AI diagnosis | `pulse_query topology` exposed a globally capped list of relationship IDs; `get` and `search` omitted Komodo resources and network endpoints. | Yes: scoped synthetic check → Stack → merged Proxmox/Komodo host traversal, provider freshness, disagreement, and edge evidence. | The model lacked a bounded, resource-scoped canonical neighborhood with endpoint status and provenance. | Add canonical Komodo/endpoint `get` and `search`, and bidirectional `topology` traversal from a canonical ID (default depth 2, max 5), with source observations, check outcomes, edge provenance, confidence, and freshness. Source error text is not copied into AI results. No repair execution authority was added. |
| UI consistency | Resource details/facets returned relationships attached directly to the resource; availability cards omitted the resolved target label; incident events did not render related-resource evidence. | Yes: resource API, availability card, incident event component, and full frontend suites. | Incoming `hosted_by` / `checks` edges and related incident resources were missing from those views. | Use the registry graph projection in details and facets, display the resolved linked resource, and render related canonical IDs in incident evidence. |

**Validation:** Full tests passed for `internal/komodo` (110),
`internal/unifiedresources` (3,054), `internal/ai/tools` (1,916),
`internal/api/resourceapi` (212), and `internal/ai` (2,138). The full
`internal/alerts` suite has one root-only permission-test failure:
`TestLoadHistory_PermissionError` expects mode `000` to block reads, which UID
0 bypasses. The remaining alert suite passed (2,782 tests) with that one test
excluded. The full frontend suite passed 22,185 tests with 3 skipped, and
`tsc --noEmit` passed. Focused regression tests for stale suppression,
missing-parent normalization, persisted topology, bidirectional API projection,
and linked-resource presentation passed.

**Live limits:** No operator runtime resource store or credential-bearing
provider snapshot was available in this audit. The actual `www-DMZ`, `Alfheim`,
`Jellyfin-DMZ`, and `Public-DMZ` operator links therefore require the isolated
live acceptance checklist below. No verified Traefik or other reverse-proxy
dependency was present in fixtures, so no affected services are inferred from
proxy failure. Scoped graph data is available through existing AI query tools.
The Phase 2I hierarchy below adds navigable resource detail topology;
dependency-aware notification routing remains future work.

**Image result:** [Workflow run #22](https://github.com/gonecrzy/Pulse/actions/runs/40405935417)
passed for source commit `ffb76e771e0fafc579d95409bb40d5b0ce861cb5`. It built
and smoke-tested the `linux/amd64` runtime, published the commit-specific
`ghcr.io/gonecrzy/pulse:komodo-ffb76e771e0f` and convenience
`ghcr.io/gonecrzy/pulse:komodo-test` tags, and verified both resolve to
`sha256:af50282ca9d0e3909bbce72b9212b9488f2a46287e904c360222d1bd196056be`.
The workflow then pulled and smoke-tested
`ghcr.io/gonecrzy/pulse@sha256:af50282ca9d0e3909bbce72b9212b9488f2a46287e904c360222d1bd196056be`,
including architecture, source label, startup, health, and `/data` restart
persistence. No merge to `sindri` or deployment is authorized by this checkpoint.

### Phase 2I — Hierarchical infrastructure topology

The resource registry now presents a branching hierarchy rooted at verified
physical infrastructure. PVE VMs nest below their Proxmox host; an
operator-confirmed Komodo Server folds into that VM identity; verified Stacks
nest under the canonical machine; availability checks attach to their Stack or
service. Host disks remain owned by the host. The regular Proxmox QEMU config
observation supplies native virtual-disk volume IDs, which produce
`stores_on` relationships only after exact storage-ID and instance matching.
No disk-to-SSD or routing relationships are inferred from proximity.

Resource detail keeps direct hosting, monitoring, and provider-verified
backing visible, with shared-host resources and cross-branch graph links in
expandable context. Patrol uses the same canonical graph and labels direct
dependency, verified backing, cross-branch relationship, shared infrastructure,
and potential impact separately. Canonical graph reads deduplicate repeated
same-owner edges while preserving separate provider/manual provenance.

The regression fixture covers two VMs on one physical host, multiple Stacks
and HTTPS checks per VM, same-name disks with separate native IDs, exact and
shared PVE storage IDs, a cross-host service route, stale config evidence, and
organization isolation. The operator-reported live 32-edge snapshot was not
available in this workspace; fixture IDs are synthetic and no production
provider was contacted. Resource detail traversal stays at two hops and caps
its context at 100 resources / 200 relationships; the UI keeps shared-host
branches collapsed. The immutable image reference is reported after the
feature branch publication workflow completes.

### Phase 2J — Patrol architecture, entitlements, and Sindri integration audit

**Status (2026-10-27):** A source-level audit of Patrol, Assistant, findings,
investigations, action execution, licensing, provider integration, and the
current frontend is recorded in
[the Phase 2J architecture audit](2026-10-27-patrol-sindri-architecture-audit.md).
It includes the capability and entitlement matrices, runtime call graph,
Codex-LB assessment, UI recommendation, ordered checkpoints, and migration and
safety risks. This is a documentation-only checkpoint; no runtime code or
tests changed.

The audit updates the implementation status of this roadmap:

- The fork feature profile is implemented and default-empty. Only
  `fork.ai_investigation` and `fork.governed_actions` are marked implemented;
  the former does not replace Pulse's commercial gates for general automatic
  Patrol investigations or alert analysis.
- Komodo monitoring, canonical cross-provider topology, resource-scoped
  topology queries, and the bounded Patrol evidence path are implemented in
  the completed Phase 2E/2G/2I work above. Do not repeat those foundations.
- OpenAI-compatible custom base URLs and model/tool readiness checks already
  exist. Codex-LB-only provider enforcement is still unavailable because
  `fork.codex_lb_gateway` is registered but not implemented. No fallback to an
  alternate provider should be added.
- Pulse's typed action lifecycle, operator approval and existing execution
  adapters are reusable. Komodo mutation adapters, the `fork.action_safety`
  feature, operator-owned multi-alert cases, ordered repair plans and inbound
  Discord approvals remain future work. Outbound Discord notifications are
  already supported.
- `/patrol/*` is the primary frontend workspace. The Assistant chat launcher
  has been retired; the shared backend chat service remains used by
  investigations. Add Sindri context to Patrol rather than creating a second
  navigation or AI runtime.

The Phase 2J audit's seven ordered checkpoints supersede earlier roadmap rows
that proposed building the fork profile, Komodo monitoring, topology ingestion,
or generic action lifecycle from scratch. Preserve the commercial entitlement,
authentication, tenant isolation, credential, approval and execution safety
boundaries documented here. No action method, production change, deployment or
merge into `sindri` is authorized by this audit.

## Workspace and baseline

- Primary checkout: `/root/Pulse`, remote `origin` = `https://github.com/gonecrzy/Pulse.git`.
- Upstream: remote `upstream` = `https://github.com/rcourtman/Pulse.git`.
- Current focused checkpoint branch: `feature/komodo-phase2-foundation`, created from current `origin/sindri`; do not merge it into `sindri` before independent review.
- Historical branch: `feature/sindri-integration`, originally created from fork `main` at `7374abf19b434c9a275df730d28234bea3943774`.
- The published branch includes planning checkpoint `beabcd7082446ce0c4cccd472dce3b41d3be7bd3` on that fork baseline.
- Fork `origin/main` is 0 commits ahead and 10 commits behind current `upstream/main` at `ece25327105f8be262dc9639caa4ce08282c77cc`.
- The Sindri reference checkout was inspected read-only and left unchanged.

### Upstream review and history-preserving integration

The initial upstream review covered merge commit `9fb404604`, `208f0cbf1` (“Preserve backup-age incidents across unknown success dates”), and `6fb43d930` (a browser proof fixture correction). The substantive alert change is in `internal/alerts/backup_snapshot.go`: a listed backup with a missing, zero, or future success timestamp no longer counts as recovery, and the newest usable provider timestamp wins. Tests cover provider order, linked/orphan subjects, occurrence retention, and exclusions/removal.

Before synchronization, a fresh fetch found upstream at `ece25327105f8be262dc9639caa4ce08282c77cc`. The intervening changes were reviewed: `92f20dafd` hides Re-investigate while a fix remains queued, avoiding duplicate action plans; `d07080e8f` and its current merge lineage retain snapshot warnings when a listed snapshot has an invalid date and add lifecycle, policy, identity, and real-consumer browser proof. These are first-party Pulse changes with no API/schema or dependency changes. The public verification record contains only a synthetic UI scenario and no credentials or deployment values. The newer reviewed tip is integrated by merge commit `80b04593ee49a326f404ce98c5e50cf61439eba1`, with prior branch head `71ad40bda11140b7d798827cab584b9f44edd5b8` as first parent and upstream as second parent. The published checkpoint `63e1d72afd47fda39b37205b7fd3c7a4436dc696` remains an ancestor. Preserve this history; do not reset, rebase, recreate, or force-push it. At the Phase 2I baseline, the local checkpoint still needed a normal fast-forward push to the feature branch.

## Architecture and file ownership

Pulse remains the monitoring and operations application. Use its existing resource registry, alert lifecycle, Patrol/AI, action planner, action lifecycle, typed runner, metrics store, and SolidJS frontend. New adapters feed those systems. Do not add a second Sindri backend, incident lifecycle, or remediation engine.

| Priority | Capability and current gap | Pulse implementation area |
|---|---|---|
| 1 | **Personal-fork feature availability.** Use one backend-owned `ForkProfile` and central capability resolver. The profile is a validated allowlist of fork-public feature IDs; frontend visibility is derived from that server result. Integrate with `internal/api`, `internal/config`, `frontend-modern/src/stores/sessionCapabilities.ts`, and existing settings surfaces. Keep fork availability distinct from `pkg/licensing/feature_map.go` and `frontend-modern/src/stores/licenseEntitlements.ts`. | One profile registry/config and one enforcement point shared by APIs and protected action admission. No feature-specific entitlement bypasses scattered through handlers. See the model below. |
| 2 | **Native read-only Komodo monitoring.** Use Pulse's existing provider and supplemental-record boundaries, unified resource registry, connection settings, and infrastructure settings UI. Add a bounded, explicitly allowlisted Komodo Core HTTP client, protected connection CRUD, and an organization-scoped poller. | A successful empty inventory, failed collection, stale last-known records, and intentional disablement remain distinct. Komodo creates no action authority and no duplicate inventory database. Preserve explicit operator identity links; never auto-merge by name. |
| Later, optional | **Codex-LB exclusive AI gateway.** Only if separately selected, constrain the existing provider path in `internal/config/ai.go`, `internal/ai/providers/`, `internal/ai/chat/`, Patrol, AI routes, and AI settings UI. | AI remains optional. Komodo polling, canonical alert evaluation, alerts, and notifications work independently of AI availability. |
| 4 | **Deterministic triage enhancements.** Pulse already has canonical alert evaluation/lifecycle, resource-aware incident synthesis, health assessment, and AI correlation/Patrol. Extend `internal/alerts/` and `internal/ai/correlation/` or a narrowly scoped deterministic package; pass conclusions and evidence into Patrol rather than creating another alert manager. Expose through existing alert/Patrol APIs and `frontend-modern/src/features/alerts/` and `features/patrol/`. | Facts, inferences, stale data, and unknowns remain distinct; deterministic results do not depend on model availability. |
| 5 | **Strict remediation policy and safety controls.** Keep plans in `internal/actionplanner/`, approvals/state in `internal/actionlifecycle/`, typed dispatch in `internal/actionrunner/`, and target/capability checks in `internal/agentcapabilities/`. Use existing action APIs and `frontend-modern/src/stores/actionInbox.ts` / action surfaces. | Extend Pulse's canonical action-admission boundary. Both automation controls, all approvals, and every dispatch source use the same lifecycle and durable policy state. No model, Discord handler, or Komodo adapter dispatches around it. |
| 6 | **Multi-alert operational cases and multi-step repair plans.** Pulse has alert incident synthesis in `internal/alerts/` and finding/investigation records in `internal/ai/investigation_records.go`, but no durable operator-owned case spanning alert occurrences. Add a case aggregate/persistence and API projection referencing Pulse alert identities and Patrol investigations; extend alert/Patrol UI. | A case groups evidence and work without replacing alert lifecycles. Repair plans and their ordered steps have explicit persisted state machines, approval bindings, and stop-on-failure behavior described below. |
| 7 | **Long-term metric retention.** `pkg/metrics/` already owns SQLite history, rollups, pruning, and retention controls; `pkg/licensing` already defines `FeatureLongTermMetrics`. Extend limits, operator configuration, API history queries, and chart presentation only where requirements exceed the current store. | Measure row growth, write load, WAL/checkpoint, query cost, backup/restore, and retention boundaries before increasing retention. Do not synthesize a commercial entitlement. |
| 8 | **Discord conversational operations and approvals.** Pulse's `internal/notifications/` supports outbound Discord notifications via webhook/Apprise; no Discord command or interaction receiver was found. Add a transport adapter (new `internal/discord/` or a narrowly scoped notifications integration), authenticated actor mapping, and durable delivery/interaction handling. Route every approval to `internal/actionlifecycle/`; use existing scoped APIs and audit. | Discord is a transport, not an approval authority. Verify platform signatures or authenticated gateway identity, actor/role at approval and execution, one-time interaction identity, expiry, and idempotency. Both automation controls apply to Discord-triggered work. |

## Personal-fork feature and entitlement model

Use one deployment-level `ForkProfile`, validated by a central backend capability resolver. The profile contains an explicit allowlist of feature IDs; it is not a license, entitlement claim, or arbitrary per-handler override. A registry entry must state the feature's implementation provenance and whether its public implementation is present in the checked-out fork.

- The profile may expose existing Pulse MIT-licensed public implementations, including the Patrol investigation engine and the typed governed-action pipeline, as fork capabilities. It may also expose independently added compatible public implementations included in this fork. Record provenance in the compiled feature registry and review, not as a user-editable entitlement. Reuse `internal/ai/chat/service_investigation.go`, `internal/ai/investigation_records.go`, and Pulse's existing planner/lifecycle/runner; do not create a parallel Sindri AI engine.
- Keep capability IDs such as `fork.ai_investigation` and `fork.governed_actions` distinct from every `pkg/licensing` feature ID. Commercial entitlement responses stay truthful and unchanged; reporting a public fork capability must never set `ai_alerts`, `ai_autofix`, or another license claim. A public implementation can be made available in this independent fork without claiming that a commercial entitlement or private source is available.
- The central resolver returns an availability source (`commercial_license`, `fork_public`, or `unavailable`) as backend data, with fork capabilities serialized separately from license features. Where an existing public implementation is currently exposed through a commercial extension adapter, Phase 1 binds the public fork path once through the central availability service and route/service wiring; it does not add local exceptions to each handler or redefine `HasLicenseFeature`.
- Commercial/private implementations remain unavailable unless present and authorized by Pulse's existing commercial entitlement check. The fork profile cannot report, mint, or alter an entitlement and cannot imply access to private code.
- Enforce one resolver result at the relevant authenticated API route groups (including `/api/ai/patrol/...`, `/api/ai/findings/...`, and `/api/actions/...` in `internal/api/router_routes_ai_relay.go` and `internal/api/router_routes_monitoring.go`), at service entry points (`internal/ai/service.go` and the investigation service), and at the canonical `internal/actionlifecycle.Service` admission before dispatch. These are three enforcement layers over one resolver policy: route rejection for direct HTTP access, service enforcement for non-HTTP callers, and final action-admission enforcement. The frontend receives the backend result for presentation only.
- Do not scatter license bypasses through handlers, mutate `HasLicenseFeature`, or repurpose commercial entitlement fields. Existing product-specific commercial gates continue to apply to commercial/private implementations and operation entitlements; fork availability applies only to explicitly registered public implementations.
- Adding a public feature means maintaining its implementation in this fork and passing its required validation gates. A feature absent from the fork is not enabled by setting a flag.
- Invalid profile values, missing policy state, or resolver errors fail closed for the affected feature. There are no per-feature conditional bypasses outside the resolver and the canonical enforcement boundary.

The exact target feature IDs, schema version, default-empty state, central backend resolver, and implementation evidence are specified in [Phase 0 contracts](2026-10-23-sindri-phase-0-contracts.md#1-personal-fork-public-feature-profile). Only a complete implementation present in this fork may resolve available; the profile never alters commercial license entitlements. All listed IDs are disabled at Phase 0.

## Automation controls at the canonical action-admission boundary

Implement two separate, durable policy controls in Pulse's existing `internal/actionlifecycle` admission path. The API, Patrol, AI tools, agents, and future Discord transport must all submit to that same boundary. A UI toggle or caller-side check is insufficient. Each admission decision records the control values and policy revision it observed.

| Control | New actions and manual work | Pending/approved work | In-flight work and restart |
|---|---|---|---|
| **Disable Automation** | Blocks automated action proposals from becoming approvals or dispatches, automated approvals, and autonomous dispatch. Non-mutating observation and recommendations may continue. Human-authored manual actions remain available through the ordinary actor, policy, freshness, and approval checks. | Pending automation-origin plans remain visible. A human may explicitly review and approve one through the canonical lifecycle; execution must require an explicit human execute request and may not auto-dispatch after approval. No background queue may resume it automatically. | A dispatch already admitted may finish and must be reconciled. No subsequent repair-plan step is automatically admitted; each eligible remaining step requires a separate explicit human execute request and full revalidation. The durable disabled state survives restart; startup does not resume automation-origin work. Re-enable is an explicit authorized policy change and never auto-executes a queued action. |
| **Total Emergency Stop** | Blocks every new executable action admission, regardless of human or automated origin, including approvals that would authorize dispatch and execution of already-approved but not-yet-dispatched work. Read-only monitoring, evidence review, and non-executable previews may remain available. | Pending and approved actions stay inspectable but blocked; they cannot be approved into executable state or dispatched. Rejection/cancellation and audit remain available. Clearing the stop does not release a queue: each candidate requires explicit review, current-policy/resource/freshness checks, and any required fresh approvals. | An admitted external operation cannot be assumed cancellable. Persist a stop request, prevent every later step, and request cancellation only when the typed executor explicitly supports safe cancellation. Reconcile durable dispatch evidence; label unresolved results uncertain and never retry blindly. The stop latch survives restart and remains set until an authorized explicit clear. Startup with unreadable/corrupt stop state denies all action admission. |

Both controls are checked atomically with policy revision, resource freshness/revision, budget and concurrency reservation, action identity, and durable dispatch-attempt creation. The emergency-stop update is acknowledged only after durable persistence. Any authenticated human may request the stop. Clearing it requires explicit confirmation by an authenticated Pulse administrator, step-up authentication, and a durable audit record. A deployment may choose a second distinct administrator as an additional policy, but two-person authorization is not mandatory. Clearing never changes Disable Automation and never resumes queued work. If policy storage is unavailable, all action admission fails closed; no in-memory or last-known-good fallback may authorize work. If a stop request cannot be durably recorded, report that it was not confirmed, keep admissions denied while storage is unavailable, and require the operator to retry/verify the persisted latch after recovery.

Disable Automation is not a substitute for Total Emergency Stop: the former allows explicitly human-driven work under policy; the latter blocks all new executable actions. Neither control can undo a side effect already sent to an external provider.

## Remediation identity, policy, and durable admission requirements

Keep all remediation inside Pulse's planner, lifecycle, runner, and capability checks. Sindri/Patrol/Discord/Komodo may propose evidence or request a plan; none may call a provider executor directly. The strict remediation acceptance gate must establish all of the following:

1. **Exact allowlists.** An executable action must match an explicit tuple of organization, provider, canonical resource kind and stable resource ID, typed action ID, and schema-validated arguments. Wildcard provider/resource/action entries are not permitted in the initial release. A group request expands to individual materialized targets; each target is independently authorized. Resource discovery and Komodo connection setup never add allowlist entries.
2. **Immutable identity.** Each action has a stable action ID and canonical plan hash covering origin/requester, provider, exact resource ID, action type, normalized arguments, ordered step IDs, and relevant constraints. Approval evidence binds action ID and hash. Refresh, target change, argument change, reorder, or any plan edit creates a new ID/hash and invalidates prior approvals; records are append-only and never mutate an approved intent in place. Propagate a stable idempotency key to the typed runner where supported.
3. **Resource and policy revisions.** Capture the canonical resource revision/version and policy revision when planning. Recheck both before approval and atomically at each step's admission. Missing, stale, ambiguous, replaced, or changed resource identity; changed allowlist/policy; or changed control revision blocks dispatch and requires fresh evidence plus a new plan/approval. Never silently retarget by display name.
4. **Durable budgets.** Define per-organization and, where needed, per-actor, per-resource, and per-action budgets with explicit windows and units. Reserve budget transactionally with the action-admission/dispatch record before sending work, settle it from reconciled outcomes, and retain reservations on unknown outcomes. A database error, duplicate request, or concurrent race must not overspend or fall back to process memory.
5. **Cooldowns.** Persist cooldown keys for the exact organization/resource/action tuple. Start the cooldown no later than dispatch admission; an uncertain result retains the cooldown until reconciled or explicitly resolved under a separately reviewed policy. No automatic retry may bypass it.
6. **Concurrency.** Enforce configurable global, organization, resource, and action-class limits using durable atomic reservations/leases. Unknown executions retain their slot until reconciled or explicitly adjudicated; restart recovery must not create a second concurrent dispatch.
7. **Fresh evidence.** Define source-specific maximum ages, health/freshness requirements, and provenance. Revalidate before each step; missing, stale, contradictory, or unknown evidence blocks admission and requests refresh. Unknown is never treated as healthy. A long-running plan does not inherit freshness from its initial triage.
8. **Database and dispatch failures.** If durable policy, lifecycle, budget, or audit storage is unavailable before admission, deny dispatch. If storage fails after durable admission but before send, reconcile the durable attempt before any retry. If provider work may have been sent but result persistence/evidence fails, mark the outcome uncertain, keep budget/cooldown/concurrency reservations, and reconcile using the runner's durable dispatch record; never blind-retry.
9. **Actor and approval binding.** Authenticate the individual actor and current role at approval and execution. Preserve separation-of-duties rules. Every required approval binds immutable action/plan identity, policy revision, resource revision, actor, method, and expiry. Approval or Discord identity cannot expand scope.

The Phase 0 numerical defaults are a three-step plan ceiling, one deployment-wide mutation slot, one mutation per exact target/action per 60 minu…991 tokens truncated…est. **Step-level approval** is required only for a step selected by an explicitly configured higher-risk policy, and binds that exact step hash and target. Approval rules are policy-versioned. A single-administrator personal deployment may approve its own plan with normal authentication, configured step-up authentication, and a durable record of requester/approver identity. A distinct approver is required only when an explicit policy asks for one, not by default.
- Admission rechecks the current automation controls, actor, plan/step identity, policy and resource revisions, freshness, budget, cooldown, concurrency, and executor capability for each step. Approval of a whole plan does not pre-admit or pre-dispatch later steps.
- **Stop on failure:** the initial release always stops before admitting later steps when a step fails, is blocked, or has an uncertain outcome, even if later steps appear independent. Completed steps remain recorded; no automatic rollback or compensation is implied. Any compensating action is a new exact typed action with its own policy and approval. No automatic retry is allowed after an uncertain result.
- An emergency stop blocks all later steps and requests safe cancellation only when explicitly supported. Disable Automation prevents autonomous continuation and requires a human execute request for each remaining eligible step. Neither control fabricates a completed/failed result for an in-flight step.

Tests must exercise every permitted and rejected transition, concurrent approvals, policy/resource revision drift, partial completion, both controls during a plan, restart recovery, and ambiguous provider outcomes.

## Komodo Phase 2 connection and polling foundation

Komodo Core API compatibility is checked at runtime against the supported v2 contract, without pinning the integration to patch `2.3.3`. The client sends only explicitly allowlisted `POST /read/<method>` calls with TLS validation, bounded transport, pagination, and safe errors. Resource inventory is limited to Server, Stack, Deployment, and Swarm. Connection CRUD and credential rotation use protected storage and verify replacement values before saving. The poller is organization-scoped and uses Pulse's existing supplemental-record publication boundary. It creates no executor or action capability.

- Persist connection identity and operational status only as needed by the current connection model. Actor/source provenance, endpoint fingerprints, selected resource scopes, and versioned secret references remain future integration work.
- Store credentials in protected encrypted configuration. Never include credential material, configuration fields, raw Komodo responses, or other sensitive response data in resource records, errors, logs, AI context, or telemetry.
- A successful empty inventory replaces prior Komodo inventory with empty; failed collection keeps last-known records and marks them stale; intentional disablement removes that connection's published records. Organization IDs and connection UUIDs scope polling and resource identities.
- Connection setup and discovery do not scan networks, guess endpoints, request elevated scopes, add remediation allowlist entries, approve plans, or register action capabilities. The Phase 0 action-safety contract remains unchanged.

## Cross-provider identity and relationship foundation

Komodo records use stable identity scoped by Pulse connection UUID, resource kind, and Komodo native ID. Komodo does not infer identity from names, labels, or similar display fields. Pulse's existing org-scoped Resource Link API can associate a Komodo Server with a Proxmox VM; it persists the explicit identity link and canonicalizes source identities. Komodo-to-Komodo parent edges and Proxmox-to-Komodo linked identities are represented in the same central resource registry and relationship graph.

The registry already keeps source-specific identities and provider parent references. Operator links persist in the resource store and are reapplied when resource snapshots rebuild. Provider-discovered relationships are rebuilt from snapshots, and Docker network attachments are re-derived from Docker inventory. A canonical merge must rewrite relationship endpoints and preserve/deduplicate edges; it must not guess based on names. A manual identity link means the operator confirmed two records are the same resource; it is distinct from a dependency edge.

The central graph has typed, directed, evidence-bearing relationships. The operator relationship API persists `depends_on`, `runs_on`, `hosted_by`, and `routes_to` edges in `operator_resource_relationships`, a table in the existing organization-scoped resource store. `POST /api/resources/{sourceId}/relationships` accepts `{targetId,type,reason}` and idempotently creates or updates an edge; `GET` on that path returns the source's active persisted edges; `DELETE` with `{targetId,type}` retires one edge and records the remover/time. A soft-deleted row is retained as a minimal audit tombstone. `same_as` remains the separate `resource_links` identity decision and is not accepted by the relationship API.

Persisted endpoints remain the canonical IDs the operator selected. Registry projections resolve linked/folded endpoints to the current canonical row and keep the stored edge available if either endpoint temporarily disappears. Discovered edges expose `evidenceSource` and `evidenceFreshness`; provider staleness updates freshness without deleting last-known evidence. Unknown provenance remains unknown. Availability `checks` edges are independent and may coexist with multiple operator `depends_on` edges. Provider-discovered edges still refresh only from their owning provider's successful complete observation. No second database or registry is introduced.

Before adding an identity link, Pulse requires both resources to be present in the organization's registry and rejects a component merge that would include distinct native IDs from the same provider. The response identifies the conflicting source and IDs; a valid Proxmox-to-Komodo link remains allowed. A same-provider link is allowed only with `confirmProviderConflict: true` and a non-empty audit reason. There is no name-based matching. Operators must resolve an ambiguous source match or split an existing mistaken link before retrying. This audit adds bounded canonical topology context to the existing query tools and bidirectional relationship projection to resource details and facets; it does not add a second graph store or a full-page interactive topology view. Traefik configuration parsing, dependency-aware notification routing, and route-based alert behavior remain future work. See the Phase 2C addendum in the Komodo checkpoint for live-test requirements and full-suite diagnostics.

## Sindri behavior and test crosswalk

This is a behavior/test index, not a source-port list. The Sindri tests below identify scenarios to preserve; Pulse package tests and contracts are the starting point for equivalent coverage. Sindri's source was not copied.

| Sindri behavior and test anchors | Pulse equivalent or gap |
|---|---|
| Feature switches/capability contracts: `capabilities.py`, `config.py`; `tests/test_capabilities.py`, `test_configuration_boundaries.py`, `test_config.py`. | `internal/config`, `pkg/licensing/feature_map.go`, API entitlement and frontend session/license stores. Add central fork-profile resolution tests proving public availability is not a license claim, private/unshipped features fail closed, and direct backend routes enforce the resolved capability. |
| Codex-LB-only calls and provider failure handling: `ai.py`; `tests/test_ai.py`. | `internal/config/ai.go`, `internal/ai/providers/`, `internal/ai/chat/`; existing provider lifecycle/factory tests. Add tests proving all investigation/Patrol calls route through the configured gateway and cannot silently use another provider. |
| Read-only Komodo resource and workload synchronization: `komodo.py`; `tests/test_komodo.py`, `test_provider_native_checks.py`, `test_infrastructure_connections.py`. | No direct equivalent. Follow monitoring/provider integration tests, unified-resource adapter tests, config API tests, and least-privilege contract tests. Add guided setup, API-version/scope validation, pagination, credential-reference/rotation, provenance-redaction, and proof that connection creation cannot add action capability. |
| Rule-based localization, source freshness, topology and recovery: `triage_engine.py`, `capabilities.py`, `topology.py`, `resolution_loop.py`; `tests/test_triage_engine.py`, `test_failure_localization.py`, `test_incident_aggregation.py`, `test_incident_reconciliation.py`, `test_resolution_loop.py`. | `internal/alerts/` lifecycle and incident synthesis, `internal/ai/correlation/`, Patrol triage and resource context. Extend table-driven tests for stale/missing/conflicting evidence and alert ordering/recovery. |
| Policy-gated typed remediation and verification: `remediation.py`, `remediation_authorization.py`; `tests/test_remediation.py`, `test_remediation_authorization.py`, `test_remediation_policy_routes.py`, `test_remediation_routes.py`. | `internal/actionplanner/`, `actionlifecycle/`, `actionrunner/`, `agentcapabilities/`; preserve approval, identity, audit, dispatch reconciliation, target freshness, and recovery verification. Add durable exact allowlist/revision/budget/cooldown/concurrency/database-failure tests and both admission-control state matrices. |
| Evidence-backed investigations and incident workflow: `incident_investigations.py`, `incident_projection.py`, `storage_incident_workflow.py`; `tests/test_incident_investigations.py`, `test_incident_workflow.py`, `test_incident_evidence.py`, `test_incident_projection.py`. | Patrol findings and `internal/ai/investigation_records.go` plus Pulse alert incidents are the nearest pieces. Add durable case membership/state tests plus plan/step approval, partial completion, stop-on-failure, uncertain outcome, restart, split/merge, and recovery tests. |
| Historical metrics, rollups and cleanup: `storage.py`, `storage_metric_rollups.py`, `storage_retention.py`; `tests/test_metric_rollups.py`, `test_rollup_retention.py`, `test_storage_retention.py`, `test_retention_worker.py`. | `pkg/metrics/` and existing `pkg/licensing.FeatureLongTermMetrics`; validate tier boundaries, rollups, bounded maintenance, disk behavior, and backup/restore on Pulse's store. |
| Discord command/notification surface: `main.py`, `bot_commands/`, `notifications.py`; `tests/test_commands.py` and remediation tests. | Outbound Discord delivery exists in `internal/notifications/`; inbound conversations/approvals are a gap. New interaction tests must end at canonical action lifecycle APIs, bind an individual actor, honor both controls, and prove duplicate/replayed approvals cannot dispatch. |

## Dependency-ordered roadmap and validation gates

Use incremental implementation on `feature/sindri-integration`, one reviewable phase at a time. Keep each phase revertible; stop before action dispatch or release if its gate fails. These are proposed checks for future implementation, not tests run in this planning checkpoint.

| Phase | Scope and dependency | Gate before proceeding |
|---|---|---|
| 0. Baseline and contracts | Preserve and merge the reviewed upstream revision; define the public fork profile and entitlement boundary, Codex-LB and Komodo protocols, action identity/revision and approval model, default-empty operation allowlist, case correlation, numeric safety defaults, metrics budget/recovery, Discord identity, and release/cutover gate. | History-preserving merge and planning specification are reviewable; no secrets in Git; public/private boundary audited; relevant Go tests, frontend checks, and repository-boundary regression recorded. No Phase 1 feature implementation is authorized. |
| 1. Fork availability profile | Add one validated backend `ForkProfile` and central resolver, API exposure, and frontend presentation. Include existing public Pulse investigation and governed-action implementations where their `fork.*` capabilities are enabled. Keep license entitlement responses separate. | Go/API/frontend tests prove checked-in public implementations can be enabled as fork capabilities without creating entitlements, private/unshipped implementations remain unavailable, invalid profile values fail closed, and direct routes, service entry points, and canonical action admission all enforce the same resolver. Run Pulse's private-boundary audit. |
| 1.5. Fork-owned image and isolated Compose qualification | Preserve the root Dockerfile and upstream release workflows. Build its `runtime` target with the `release` Go tag for `linux/amd64`, label the full source SHA, smoke-test before publication, then publish only that qualified image to `ghcr.io/gonecrzy/pulse` under its exact SHA tag using the job-scoped `GITHUB_TOKEN`. Push builds run on `feature/sindri-integration` and the future `sindri` branch; manual dispatch remains restricted to the repository default branch. Add an isolated Compose project that requires an explicit SHA tag or digest, keeps `/data` in a project-scoped volume, disables in-app automatic updates for pinned images, and supports health, persistence, digest redeployment, and cross-revision update/rollback checks. | Build-and-smoke succeeds without commercial/signing secrets or Docker Hub credentials; credential-bearing publication has `packages:write` only and no remote build cache; published digest and source revision are recorded; pulled image and Compose health/restart/update checks pass; the image-switch verifier proves old-SHA → new-SHA → old-SHA identity, health, and volume persistence; missing `PULSE_IMAGE` fails and the Compose boundary audit passes. Cross-revision image switching does not qualify application database/schema downgrade compatibility. Keep `main` as the upstream baseline and `feature/sindri-integration` as active development; `sindri` is a proposed stable/default branch only after separate authorization and review. Do not detach the fork or create/promote `sindri` in this checkpoint. |
| 2. Native Komodo monitoring | Add the protocol-only client, encrypted connection CRUD, read-method scope checks, stable identity and relationship normalization, and an organization-scoped poller through the supplemental provider boundary. Preserve operator-confirmed cross-provider links. Read-only describes the current collector boundary; it does not rule out separately governed provider actions in a later phase. | Relevant Go tests and governance checks prove API/auth/version/scope validation, allowlisted read methods, pagination, bounded transport, credential rotation/redaction, duplicate-ID handling, organization isolation, empty/failure/stale/disable/removal behavior, registry ingestion, retained relationships after identity linking, and no implicitly granted action authority. |
| 2B. Cross-provider identity and relationship validation | Verify Proxmox VM ↔ Komodo Server linking, source-target and history ownership, canonical selection, parent/Swarm edge rewrites, availability associations, resource-store restart, source staleness/removal, and organization isolation. Use the central registry and existing operator links. | Go tests prove source polling order, explicit-pair-only folds, Proxmox history target preservation regardless of selected canonical provider, parent/relationship rewrites, persisted links and recovery, stale/missing-provider behavior, availability check compatibility, and no same-name merge. |
| 2C. Relationship hardening and live integration preparation | Reject unacknowledged same-provider identity collisions and unavailable link endpoints. Persist directed operator dependency/ownership/routing edges in the existing org resource store, preserve them across registry rebuilds and endpoint disappearance, and expose discovered-edge freshness. | Store/API/registry tests cover collision 409s, reasoned same-provider confirmation, legitimate cross-provider links, stable edge identity, endpoint rewrites, soft removal audit, stale discovered evidence, provider disappearance/recovery, org isolation, and availability-check coexistence. No live Komodo claim until the operator supplies a Core endpoint, actual version, and approved read-only credentials. |
| 2D. Komodo live qualification | Run the opt-in harness with the approved protected credential and operator attestation. Verify actual Core v2 version, auth, effective scope, pagination, resource fields, parent references, stable IDs, and safe read-only operation. | Passed against Core `2.3.4` through the exact operator-provided test-only LAN HTTP endpoint. The API verified all four list methods and populated-resource base `Read`; the operator attested that effective specific permissions are absent. The certificate-trusted HTTPS production path remains a separate deployment prerequisite. |
| 2E. Komodo monitoring integration and isolated image qualification | Connect Komodo connection health to the existing alert lifecycle, normalized provider health/incidents to canonical evaluation, alerts/recoveries to `NotificationManager`, sanitized status to existing UI, and safe read-only records to Patrol context. Use the existing explicit identity and dependency model. Build a source-SHA-tagged test image from the production Dockerfile and use a copied data directory; do not deploy it. | Fixture-backed integration and regression tests pass independently of AI. Live inventory and normalization qualification passed. The local runtime image was built and smoke-tested; isolated operator data acceptance and any copied-database schema/rollback exercise remain. Do not publish to stable/latest, use the live writable database, or deploy. |
| 2L. Codex-LB gateway integration | Add exclusive Sindri inference routing through Pulse's OpenAI-compatible adapter. Enforce `fork.codex_lb_gateway` in provider construction and shared AI services, configure the gateway in existing protected AI settings, and qualify it through Patrol's existing readiness and preflight paths. | Mock-backed tests prove endpoint policy, authentication, model selection, streaming/tool-call parsing, bounded continuation, secret masking, no alternate-provider fallback, and deterministic monitoring continuity. Run the applicable full Go and frontend regression suites plus repository governance checks. Live gateway qualification remains opt-in and requires separately supplied protected credentials. |
| 4. Deterministic triage | Add rules over Pulse alerts/resources, Komodo observations, topology, and metric freshness; Patrol may explain evidence but cannot supply authorization. | `go test ./internal/alerts ./internal/ai/correlation ./internal/ai` plus alert API tests. Replay ordered, duplicate, stale, contradictory, unknown, and recovery fixtures; decisions must be deterministic and unknown must not become healthy. |
| 5. Canonical action controls and strict policy | Add Disable Automation and Total Emergency Stop at canonical admission; enforce exact allowlists, immutable identities, revisions, durable budgets/cooldowns/concurrency, freshness, actor bindings, audit, failure behavior, and post-action verification. | `go test ./internal/actionplanner ./internal/actionlifecycle ./internal/actionrunner ./internal/agentcapabilities` plus store/API tests. Matrix tests prove manual-vs-automated semantics, pending and in-flight behavior, authenticated administrator emergency-stop clearing, restart persistence, database fail-closed behavior, denied/stale/ambiguous/concurrent/expired requests never dispatch, one approval authorizes the full immutable plan by default, configured higher-risk step approvals work, approved work dispatches once, and uncertain outcomes reconcile without blind retry. |
| 6. Long-term metrics | Configure retention/rollups against an explicit storage and freshness budget. Targets are 5-minute averages for 30 days, hourly averages for 365 days, and daily averages for 730 days, retaining existing short raw/minute tiers where appropriate. | `go test ./pkg/metrics ./pkg/licensing` plus API history tests and disposable load/restore qualification. Measure representative series cardinality, DB/WAL/index growth, query/write latency, recovery and backup headroom against an operator-approved finite budget. At quota, alert and make write gaps explicit; never silently truncate daily history. Destructive reductions require explicit operator confirmation and a verified recovery plan. |
| 7. Multi-alert cases and repair plans | Add the durable case aggregate and immutable plan/step state machines. Depends on phases 4 and 5; metrics evidence from phase 6 is used where required. | Alert/AI/API and SolidJS tests cover correlation, dedupe, membership audit, split/merge/reopen, partial recovery, permissions, evidence links, one plan approval by default, explicitly configured higher-risk step approvals, revision drift, stop-on-failure, partial execution, emergency/automation controls, uncertain outcomes, restart, and isolation of individual alert lifecycle. |
| 8. Discord operations and approvals | Add authenticated interaction transport and durable outbound/inbound state. Depends on phases 5 and 7. | Discord adapter and action lifecycle tests cover signature/actor/role checks, replay, duplicate interaction, expiry, role revocation, both controls, outbox retry, outage, and idempotent approval. No direct dispatch path. |
| 9. Isolated end-to-end acceptance | Exercise the integrated monitoring, triage, case, approval, action, recovery, credential-rotation, and rollback paths in a disposable isolated environment. No production configuration or provider credentials. | Reproducible end-to-end evidence, failure-injection results, audit review, and signed-off list of known limits. Any failed safety or recovery scenario returns to its owning phase; no production operation is authorized by passing this gate. |
| 10. Release readiness and rollback | After Phase 9 passes, produce release artifact/version, migration and compatibility review, backup/restore evidence, secrets/credential rotation steps, feature-profile defaults, health checks, monitoring, cutover checklist, rollback trigger thresholds, named operator/decision owner, and tested rollback instructions. | Readiness review confirms previous artifact/config restore is tested, no irreversible migration is unaccounted for, all credentials are scoped, applicable controls are verified, and rollback ownership is explicit. The agreed decision is one production cutover for the initial release; this roadmap does not authorize it. Any later forward retry or second cutover requires a new explicit approval. A rollback restores the prior known-good state and does not automatically trigger another cutover. |

Across implementation phases, run repository formatting/static checks and focused Go/SolidJS suites; run broader CI suites before merge. Live-provider checks use disposable/staging systems with read-only credentials until separately approved action qualification. Isolated end-to-end acceptance precedes release readiness. No production cutover or production configuration change is part of this planning checkpoint.

## Branch promotion and default-branch preparation

- Keep `main` at the upstream baseline and continue checkpoint commits on `feature/sindri-integration`. Do not rebase or force-push published checkpoint history.
- After a feature checkpoint receives independent review and explicit promotion approval, open a pull request from `feature/sindri-integration` into `sindri`. Require the selected CI, governance, boundary, and relevant targeted checks to pass before merging. Preserve the reviewed feature commits in the promotion history.
- The `sindri` push filter runs the fork image workflow, publishing only the tested exact-SHA artifact. Confirm the published digest and isolated deployment qualification before treating it as a stable candidate.
- Before changing GitHub's default branch, configure `sindri` protection to require the appropriate CI and governance checks and review who can merge. Confirm the workflow-dispatch gate follows the new default branch. Changing the default branch, creating/promoting `sindri`, detaching the fork, and merging into `main` require separate authorization.
- General code-validation workflows are extended to `sindri`. The scheduled dependency-advisory watch reads the repository's current default branch, so it continues to audit `main` now and follows `sindri` only after a separately approved default-branch change. Release-continuity and credential-bearing private-governance workflows remain tied to `main` because they audit upstream release state or access `pulse-pro` with `WORKFLOW_PAT`; they are not promoted as fork checks.

## Phase 1.5 configuration-test comparison

The three configuration tests reported during Phase 1.5 were compared on baseline `96bb8d46fff123651fc3c2b5c7bb47513a5e46be` and Phase 1.5 source `20b4e91d3fcfc524e6b4056ef692ec6c3476cbe7`:

- Under the same unprivileged environment (`uid=65534`, Go `1.26.10`, separate writable temporary Go caches), the three-test selection passed with `-count=3` on both revisions. The full `go test ./internal/config -count=1` suite also passed on both revisions.
- Under `uid=0`, the full config suite reproduced the same three failures on both revisions. Baseline reported 1,815 passed, 3 failed, and 2 skipped; Phase 1.5 reported 1,821 passed, 3 failed, and 2 skipped. A focused root run reproduced the current failures.
- `TestConfigurationMigrationArchiveScope` fails at `migration_scope_test.go:55` with `Should be false` (`fresh destinations must use a different key`). `TestLoadAIFindings_UndecryptableFilePreserved` fails at `persistence_corrupt_preserve_test.go:114` because zero preserved files are found instead of one. `TestLoadAIChatSessions_UndecryptableFilePreserved` fails at lines 134 and 137 because the session decrypts into `map[s1:...]` and zero corrupt copies are preserved instead of one.
- Root-run logs show the config's legacy-key migration finds a system-level key and copies it into each temporary config directory. That makes the supposedly fresh destination keys identical and lets the encrypted fixtures decrypt. The unprivileged run cannot use that root-readable legacy key and generates independent temporary keys, so the preservation assertions exercise their intended case.

These failures are inherited at the Phase 0/Phase 1 baseline and deterministic for the tested root environment; they are not introduced by Phase 1.5. Do not weaken encrypted-data preservation assertions. Run this package as an unprivileged user until the root-sensitive fixture setup receives a separately reviewed correction.

## Inherited browser regression disposition

The upstream `frontend-modern/browser-tests/backup-date-chronology.cjs` failure is a test-fixture mismatch: the current backup coverage row renders one valid latest-backup age, while the test still expects two obsolete per-source age labels. A test-only correction is recommended: assert the single valid replacement age, then measure whether the age cell's painted content is clipped by its actual cell bounds at desktop and narrow viewports. Use fractional CSS-pixel-aware bounds/tolerance so a subpixel glyph overhang is not misclassified as clipping. Do not change application alert/snapshot behavior based on this assertion. This recommendation is planning only; the `.cjs` test change requires separate operator authorization and is not included in this checkpoint.

## Public/private and license boundary

- Pulse's root `LICENSE` is MIT. Preserve its copyright/license notices in any copied or substantially reused Pulse source.
- `docs/REPO_BOUNDARY.md` marks Pulse core public and defines private enterprise/business ownership; private in-app behavior crossing that boundary must use the established `pkg/...` contracts rather than leaking private implementations into `internal/...`.
- No tracked root `LICENSE`, `COPYING`, or `NOTICE` was found in Sindri. Treat its implementation as reference behavior only until source provenance/license is resolved. Independently implement against Pulse contracts; do not copy Sindri code or architecture report details into the product.
- Keep personal deployment switches, gateway endpoints, tokens, infrastructure names, allowlists, and Discord identities out of public defaults, fixtures, telemetry, and checked-in docs. Keep user-specific values in protected runtime configuration.
- Fork availability is a separate public capability signal. Never emit or alter commercial entitlements, add code that assumes private implementation access, or scatter entitlement exceptions through feature handlers.
- Do not use a fork profile to authorize a provider action. Komodo connection setup grants read-only monitoring only. Remediation authority requires the exact separately reviewed action policy and canonical Pulse lifecycle.

## Open implementation blockers and decisions

1. **Historical contract review:** the original Phase 0 and Phase 1 approvals are recorded in their checkpoint documents. This steering update authorizes only the Komodo Phase 2 foundation described here; it does not authorize deployment, action authority, or the optional Codex-LB capability.
2. **Production transport:** live compatibility was qualified through the exact operator-provided HTTP LAN endpoint, only in the opted-in test harness. That route does not encrypt API credentials in transit. Production Komodo connection validation remains HTTPS-only; before using this Core in a Pulse deployment, restore DNS and a normally trusted certificate for `https://komodo.home.goneczy.com` (or provide another trusted HTTPS origin). The API returned level-only `Read`, so absence of specific permissions remains operator-attested rather than API-verified. No new credential or permission confirmation is required.
3. **Action scope:** integration action allowlist is empty. Any non-empty tuple requires a separately reviewed checkpoint with exact resource IDs, operation/version, canonical arguments, policy/resource revisions, actor authorization, and passing durable safety tests. Komodo repair and SSH parity remain out of scope.
4. **Storage and recovery:** operator-supplied metric storage budget and recovery destination must be qualified against representative data before enabling 30-day 5-minute, 365-day hourly, and 730-day daily retention. No budget policy may silently reduce daily history.
5. **Inherited browser test:** the test-only update to `frontend-modern/browser-tests/backup-date-chronology.cjs` remains pending separate operator authorization; it is not an application change in this planning checkpoint.
6. **Release:** name the artifact/release owner, operations owner, rollback operator, and cutover authority before Phase 10. The one-production-cutover decision remains in force; Phase 0 and isolated acceptance authorize no deployment.

These inputs block live compatibility claims and image readiness; they do not block fixture-backed monitoring integration. Future action authority remains a separate Pulse policy decision and checkpoint. The existing Sindri reports remain read-only and should be consulted only if a later phase needs a specific behavior reference.

### Phase 2K — Standalone fork policy and upstream service removal

**Status (2026-10-27):** Implemented locally from `c325f89bd` on
`feature/komodo-phase2-foundation`; pending review. No push, merge, deployment,
or image publication is part of this checkpoint. See the
[standalone egress inventory](2026-10-27-standalone-egress-inventory.md) and
the [Phase 2J architecture audit](2026-10-27-patrol-sindri-architecture-audit.md)
for the source matrix and compatibility boundaries.

- The fork profile is version 2 and explicitly reports implementation,
  supported/experimental/unavailable status, enabled state, and availability.
  Its fresh-install defaults enable supported deterministic triage, read-only
  Komodo, Sindri finding investigation, and the existing governed action
  lifecycle. Public agent-profile, RBAC, audit, reporting, and external-probe
  implementations also have explicit fork capabilities, disabled until an
  administrator enables each one. `fork.action_safety`, cases, Discord operations, and the
  Codex-LB-exclusive gateway remain unavailable or disabled. Capability
  availability does not grant a user role, action approval, or dispatch.
- An existing version-1 profile is migrated in memory while preserving its
  explicit enabled feature set; the next administrator save writes version 2.
  Corrupt/unknown policy remains fail-closed. A missing profile uses the new
  version-2 defaults. License, billing, provider, and infrastructure files are
  not rewritten.
- Standalone licensing, activation, grant refresh, installation-status polling,
  hosted entitlement refresh, trial, and checkout traffic are stopped by
  default. Old activation, license, billing, and refresh-token files remain on
  disk. `GET /api/license/entitlements` is retained for compatibility and does
  not synthesize Pro, hosted, or trial state; activation/checkout are retired
  with `410 Gone`. An explicit `PULSE_UPSTREAM_COMMERCIAL_SERVICES=true` keeps
  the legacy compatibility path available for operators who deliberately need
  it. Hosted service behavior is unchanged.
- Missing telemetry preferences now resolve off, and the disabled telemetry
  runner creates no sender. Existing explicit opt-in remains effective. Release
  checks and unattended updates default off; an operator can still run a manual
  check, and configured infrastructure, AI provider, notification, IdP, probe,
  and typed action traffic remains enabled under its existing controls.
- The local Sindri finding-reinvestigation route reuses Pulse's runner, finding
  records, evidence tools, budgets, and organization/resource admission. It does
  not change Pulse `ai_autofix` / `ai_alerts` entitlements or general Pulse
  automatic Patrol behavior. Automatic repair remains unavailable while
  `fork.action_safety` is unimplemented.

**Focused validation:** fork-feature policy tests passed (13 tests); focused
configuration and telemetry tests passed (2 and 3 tests); the focused API
regression selection passed (21 tests); licensing tests passed (1,019 tests);
frontend targeted tests passed (26 tests across 5 files), and the TypeScript
check passed. The standalone HTTP egress tests use local mock servers; no live
licensing, provider, or infrastructure endpoint was called. The broader
`go test ./internal/api -count=1` run completed with 6,419 passed, 45 failed,
and 11 skipped. The remaining failures include legacy commercial-route,
entitlement-response snapshot, and prior capability-gate assertions; they are
recorded for review and are not represented as passing validation. A
supplemental multi-package run also reported three failures in
`internal/config` (`TestConfigurationMigrationArchiveScope`,
`TestLoadAIFindings_UndecryptableFilePreserved`, and
`TestLoadAIChatSessions_UndecryptableFilePreserved`); the focused configuration
default tests passed separately.

This Phase 2K checkpoint supersedes the Phase 2J audit's as-of-audit statements
that the profile was default-empty and that standalone telemetry was enabled by
default. The audit remains the source-reference baseline; its historical
licensing and gateway recommendations are not evidence that those later phases
have been implemented.

### Phase 2L — Codex-LB exclusive gateway integration

**Status (2026-10-28):** Implemented locally on
`feature/komodo-phase2-foundation`; focused and full verification passed. No live
gateway request, push, merge, deployment, or image publication is part of this
checkpoint. Setup and operator-trust guidance is in
[`docs/CODEX_LB_GATEWAY.md`](../CODEX_LB_GATEWAY.md).

- `fork.codex_lb_gateway` is now registered as a supported, disabled-by-default
  local capability. Enabling it admits only the dedicated Codex-LB provider and
  explicitly selected gateway models. The generic OpenAI-compatible constructor
  cannot impersonate this provider; direct OpenAI, Claude, Ollama and subscription
  routes fail provider admission while the capability is active. Legacy provider
  settings remain encrypted and intact.
- The endpoint, credential and private-network opt-in use the existing encrypted
  AI settings file and authenticated settings API. The frontend receives endpoint,
  selected model, configured-state and readiness evidence, never the stored key.
  HTTPS is the default. Private-network and localhost routing requires explicit
  opt-in, and metadata-service destinations remain denied at configuration and
  request time.
- Patrol readiness exercises the existing OpenAI-compatible Chat Completions
  streaming, tool-call and multi-turn continuation path. The provider retains
  existing usage accounting, timeout and circuit-breaker integration; stream
  startup now also uses its single bounded retry for a transient 429. Gateway
  errors do not select an alternate model/provider, and deterministic monitoring
  remains active when AI is unavailable.
- No second model client, investigation runner, scheduler, findings store or repair
  engine was introduced. Existing Pulse commercial compatibility behavior,
  authentication, tenant boundaries, approval checks and automatic-repair policy
  remain separate from the fork capability.

**Validation (2026-10-28):**

- Focused gateway and policy Go tests passed: 20 top-level tests and 12 subtests
  across 6 packages. The full `internal/ai/providers` package also passed after
  its source-contract check was updated to require `ChatStream`'s selected-model
  validation call. Focused command:
  `go test ./internal/config ./internal/forkfeatures ./internal/ai/providers ./internal/ai/modelresolution ./internal/ai ./internal/api -run 'CodexLBGateway|TestContract_AISettings(UpdateProviderResolutionJSONSnapshot|BYOKOverrideDoesNotExposeQuickstartInventoryJSONSnapshot|ProviderRegistryMetadata|RetiredQuickstartAliasJSONSnapshot|OllamaAuthJSONSnapshot|HostedAISettingsDoesNotAutoBootstrapQuickstartJSONSnapshot|HostedTenantAISettingsDoesNotAutoBootstrapQuickstartJSONSnapshot)|TestForkFeatureHandlersRejectForgedAndStaleProfiles' -count=1`.
- `go test -json ./... -count=1` passed: 20,780 top-level test events and 31,868
  subtest events passed; 84 top-level and 29 subtest events skipped; zero failed.
  All 160 packages with tests passed; 13 packages had no test files.
- Frontend `node node_modules/vitest/vitest.mjs run` (from `frontend-modern`)
  passed: 1,269 test files passed, 1 skipped; 22,193 tests passed, 3 skipped.
  TypeScript checking (`node node_modules/typescript/lib/tsc.js --noEmit`), ESLint,
  frontend audits, 48 workflow trust tests, 15 mutation registry tests, 31 repoctl
  tests, one boundary regression test, and all six repository boundary enforcement
  scripts passed.
- Gateway qualification used local mock servers and fixtures only. No live
  Codex-LB account or credential was configured or used.

**Limits and follow-up:** The endpoint's identity is an administrator trust
assertion; OpenAI-compatible syntax alone cannot attest that a service is Codex-LB.
Pulse uses Chat Completions and does not preserve Codex-LB Responses API reasoning
state. Unknown model IDs use Pulse's default context estimate and may lack dollar
pricing metadata. A live test against the operator's pinned Codex-LB deployment,
with separately supplied protected credentials and explicit authorization, is a
separate qualification checkpoint.

## Git checkpoint discipline

- The current focused checkpoint is `feature/komodo-phase2-foundation`, based on current `origin/sindri`. Preserve the historical `feature/sindri-integration` branch and checkpoint history. Create descriptive, logically separated commits for approved implementation phases.
- Run the phase's focused tests and relevant broader checks before committing, and record exact commands/results with that checkpoint.
- Push this completed review checkpoint to `origin/feature/komodo-phase2-foundation` so the architecture reviewer can inspect it on GitHub. Do not merge it into `sindri`. Report branch, commit SHA, changed files, tests, and remaining issues.
- For policy, authentication, action execution, AI tools, or feature-entitlement changes, publish and review a checkpoint before starting dependent work.
- Never commit secrets, environment files, production credentials, or private Sindri artifacts. Do not push directly to `main`, force-push a reviewed checkpoint, or merge without explicit authorization.
