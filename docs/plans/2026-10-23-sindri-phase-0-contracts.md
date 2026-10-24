# Pulse + Sindri — Phase 0 Contracts

**Status:** revised after independent review; this document defines proposed contracts and does not authorize Phase 1 implementation. The corrections in this revision require independent confirmation before Phase 1.

**Reviewed upstream baseline:** the first reviewed tip `9fb404604b006822ef3a007442bc5ac5d1a8a900` was integrated by merge `71ad40bda11140b7d798827cab584b9f44edd5b8`. Before final synchronization, upstream advanced to `ece25327105f8be262dc9639caa4ce08282c77cc`; its intervening changes were reviewed and integrated by merge `80b04593ee49a326f404ce98c5e50cf61439eba1`. That merge retains `71ad40b` as first parent and current `upstream/main` as second parent. The published fork checkpoint `63e1d72afd47fda39b37205b7fd3c7a4436dc696` remains an ancestor. Fork `main` was not changed.

This specification uses Pulse's MIT-licensed public interfaces as the implementation base. It neither copies Sindri source nor records deployment secrets, endpoint values, account names, or private infrastructure details. A value marked **operator input** must be supplied in protected deployment configuration and must not be guessed or committed.

## 1. Personal-fork public feature profile

Use one deployment-wide, backend-owned `ForkProfile` and one capability resolver. The profile is an availability list for this independent public fork. It is not a license, an entitlement, an authorization to run actions, or a claim about code in Pulse's private repositories.

The exact profile schema is version 1 with a finite `enabled_features` list. The only feature IDs this integration may register are:

| Feature ID | Public implementation and evidence | Status at Phase 0 |
|---|---|---|
| `fork.codex_lb_gateway` | Constrain Pulse's OpenAI-compatible provider path in `internal/ai/providers/openai.go`, `internal/config/ai.go`, and provider lifecycle tests. Pulse already accepts an OpenAI-compatible base URL; it does not yet enforce Codex-LB as the exclusive gateway. | Not complete; disabled |
| `fork.komodo_read_only` | New first-party, protocol-only adapter in `internal/monitoring/` and `internal/unifiedresources/`. No Komodo connector exists in Pulse. Do not add a Komodo SDK or copy Komodo source into Pulse. | Not implemented; disabled |
| `fork.deterministic_triage` | Extend public alert lifecycle and resource/correlation evidence in `internal/alerts/`, `internal/ai/correlation/`, and `internal/ai/`. | Existing foundations; integration additions incomplete; disabled |
| `fork.ai_investigation` | Reuse Pulse's MIT-licensed investigation implementation in `internal/ai/chat/service_investigation.go`, `internal/ai/investigation_records.go`, and the existing Patrol orchestration. This is a fork capability over code present in this repository; it does not assert that a commercial entitlement is present. | Public implementation exists; fork availability wiring is incomplete; disabled |
| `fork.governed_actions` | Reuse Pulse's MIT-licensed typed action pipeline in `internal/actionplanner/`, `internal/actionlifecycle/`, `internal/actionrunner/`, `internal/agentcapabilities/`, and the authenticated `/api/actions/...` routes. This exposes the governed public implementation only; it adds no action tuple, runner operation, or authority. | Public implementation exists; fork availability wiring is incomplete; disabled |
| `fork.action_safety` | Extend `internal/actionplanner/`, `internal/actionlifecycle/`, `internal/actionrunner/`, and `internal/agentcapabilities/`. This flag adds policy controls only; it does not register an executable action. | Existing foundations; integration additions incomplete; disabled |
| `fork.operational_cases` | New public case aggregate referencing Pulse alert occurrences and `internal/ai/investigation_records.go`; no durable cross-alert case aggregate currently exists. | Not implemented; disabled |
| `fork.long_term_metrics` | Use `pkg/metrics/` SQLite tiers, rollups, pruning, and recovery. Add a separate fork capability path; do not change the value or meaning of `pkg/licensing.FeatureLongTermMetrics`. | Existing storage; fork-specific access path incomplete; disabled |
| `fork.discord_ops` | Extend outbound-only Discord support in `internal/notifications/` with a first-party interaction receiver bound to Pulse identities and action APIs. | Not implemented; disabled |

The `enabled_features` list defaults to empty. An ID can resolve true only when its complete implementation is present in this fork, its entry is in the compiled registry, and the central resolver validates the profile. Unknown IDs, duplicate IDs, invalid schema versions, absent/corrupt profile state, and resolver errors fail closed. A profile entry cannot make absent code available. The frontend consumes the server's resolved fork capabilities; it cannot set them or treat them as entitlements. Profile edits are server-side, authenticated, audited, and versioned; changing the profile does not update or mint license claims.

Fork availability and commercial entitlement are two independent backend results. Keep `pkg/licensing` claims and license responses truthful and unchanged: `fork.ai_investigation`, `fork.governed_actions`, and other `fork.*` IDs must never be written as `ai_alerts`, `ai_autofix`, or any other commercial feature. The fork can make a checked-in MIT implementation available under its own `fork.*` capability even when a commercial entitlement is false; the response must report that availability separately and state its source as the public fork profile. Commercial/private implementations remain unavailable unless present and authorized by the existing commercial entitlement. In particular, do not infer or claim access to any private implementation.

Use one backend `ForkProfile` registry/resolver as the only fork-availability policy. It returns resolved availability and provenance, not a license claim. Its result distinguishes `commercial_license`, `fork_public`, and `unavailable`; the API reports fork capabilities separately from unchanged license features. Where a public implementation currently sits behind a commercial extension adapter or license check, Phase 1 must bind the fork capability once through this central availability service and route/service dependency wiring. It must not edit each handler with a local exception. Enforce the same resolved capability at three layers: (1) the relevant authenticated HTTP route groups in `internal/api/router_routes_ai_relay.go` (Patrol/investigation) and `internal/api/router_routes_monitoring.go` (`/api/actions/...`), including direct API calls; (2) service entry points and injected service dependencies, including `internal/ai/service.go` and the existing investigation runner in `internal/ai/chat/service_investigation.go`; and (3) the canonical `internal/actionlifecycle.Service` plan/decision/execute admission path before any typed runner dispatch. Route checks shape responses and reject direct access, service checks protect non-HTTP callers, and lifecycle admission is the final action authority boundary. All checks consume the same resolver result. Do not add per-handler license exceptions, change `HasLicenseFeature` or commercial license payloads, or copy the investigation engine into a parallel Sindri service. UI visibility is a presentation of backend results, never authorization.

The MIT root license and notices remain attached to Pulse-derived code. The Komodo adapter must use only its documented HTTP protocol and independently authored Pulse code; the official Komodo project identifies itself as GPL-3.0. No Komodo GPL client library or source is approved for this MIT repository.

## 2. Codex-LB provider contract

### Compatibility target

- **Gateway release baseline:** Codex-LB `v1.24.0`, the stable release reviewed on 2026-10-23. Deployment must record its actual version as operator input. A different release is unsupported until the compatibility suite passes against that exact release.
- **Wire protocol:** Codex-LB OpenAI-compatible `/v1` Chat Completions only, using `POST /v1/chat/completions`, `GET /v1/models`, and Bearer authentication when API-key auth is enabled. Select a model from the live model catalog; model IDs are operator input and are never hard-coded from an example.
- Pulse's current OpenAI-compatible provider supports chat completions, tool calls, and streaming. This contract does not claim Codex-native Responses API compatibility or preserved hidden reasoning state. Do not configure `/backend-api/codex` until Pulse has a dedicated Responses implementation and tests.
- Store the endpoint, selected model, and API key reference in Pulse's protected server configuration. Endpoint, API key/secret reference, TLS/egress policy, timeout override, and selected model are operator inputs. Do not include key material in a profile, log, export, fixture, telemetry, or planning file.
- The configured Codex-LB gateway is exclusive for every Pulse AI request in this integration. No direct OpenAI, Anthropic, Codex subscription, local model, or alternate-provider fallback is allowed. Patrol and monitoring can run without AI; provider failure never grants action authority.

### Failure behavior

The provider adapter returns a typed unavailable/configuration failure for authentication errors, model-not-listed, timeout/cancellation, connection/TLS/DNS failure, rate limit, upstream 5xx, invalid JSON, unsupported tool-call shape, malformed or truncated stream, missing terminal stream marker, and response-size violation. Diagnostics are redacted and contain no URL credentials, API key, prompt content, or upstream secret-bearing response.

Generation requests are not automatically retried: after a request may have reached Codex-LB, retry can duplicate cost or produce a different tool proposal. A failed or partial streamed generation is discarded as a whole. Model-catalog reads may retry once with bounded delay for a transient 429/5xx. All other retries are explicit operator requests. A tool call remains a proposal and must pass Pulse planning, policy, approval, and action-admission checks; provider output never dispatches an operation directly.

Qualification uses a mock HTTP server for adapter failure cases and a disposable Codex-LB instance at the configured supported release for end-to-end compatibility. It proves model discovery, bearer redaction, tools, streaming completion, no alternate-provider request, and normal alert/metric operation while the gateway is down.

## 3. Komodo API, resource identity, and credentials

### Version and exact read surface

- **API baseline:** Komodo Core v2 API, pinned for initial qualification to release `2.3.3` and its OpenAPI 3.1 contract. Record the deployed Core version as operator input. Reject unsupported major versions; accept a later v2 version only after its contract suite passes.
- Use documented HTTP `POST /read/<method>` calls. The adapter allowlist is exactly: `GetVersion`, `GetCoreInfo`, `ListServers`, `GetServer`, `GetServerState`, `GetServersSummary`, `ListStacks`, `GetStack`, `GetStackActionState`, `GetStacksSummary`, `ListDeployments`, `GetDeployment`, `GetDeploymentActionState`, `GetDeploymentsSummary`, `ListSwarms`, `GetSwarm`, `GetSwarmActionState`, and `GetSwarmsSummary`. Deny all other methods in Pulse before transmission, regardless of Komodo credential grants.
- The supported resource types are exactly `Server`, `Stack`, `Deployment`, and `Swarm`. Identity is `(Pulse Komodo connection UUID, Komodo resource type, Komodo resource ID)`. Names and labels are display data, never identity or retargeting keys. IDs from separate connections never collide.
- Require Komodo `Read` on only the selected resources. Do not grant or call `Execute`, `Write`, `Logs`, `Inspect`, `Terminal`, `Processes`, `Attach`, or any mutation method. `Inspect` and log/process data can expose environment values and other sensitive content. Normalize fields into Pulse resources and redact secret-like fields before persistence, API output, AI context, logging, and telemetry.

### Setup and credential provenance

Setup is initiated with an operator-supplied Core URL and a credential reference. It creates a Pulse-side provider connection only after TLS/URL validation, `GetVersion`, authenticated read probes, and an explicit read-scope check succeed. It then discovers only the four allowlisted resource types and creates the connection record and bounded polling schedule. No network scanning, endpoint guessing, Komodo API-key creation, or elevated OAuth/API scope request is permitted.

The credential is a Komodo API key and secret supplied by an authorized operator and stored through Pulse's protected secret/configuration path. Persist only the secret reference/version, source class (operator entry or approved secret store), Pulse actor who supplied it, creation/rotation timestamps, connection UUID, endpoint fingerprint, verified Komodo version, selected scope, and last validation result. Never persist raw key/secret in resource metadata. Rotation re-verifies the read surface before switching references; revoked/unknown credentials disable the provider and do not trigger fallback.

The API allowlist and resource normalization remain read-only even if an operator accidentally supplies a credential with greater Komodo permissions. Connection creation and resource discovery never register a Pulse action capability, add an action allowlist entry, approve a plan, or create a repair executor. Komodo repair and SSH parity are excluded from the initial release.

## 4. Initial typed Pulse action scope and authorization

### Exact operation scope

The fork integration's initial new action allowlist is **empty**. It registers no Sindri or Komodo mutating operation. Discovery, triage, AI, cases, and Discord cannot add an action implicitly. Existing Pulse action runner v1 recognizes these baseline operation strings: `host.update`, `host.storage_cleanup`, `proxmox.guest.start`, `proxmox.guest.stop`, `proxmox.guest.shutdown`, `proxmox.guest.reboot`, `container.start`, `container.stop`, `container.restart`, and `container.update`. This document does not add, enable, or broaden those operations. They remain subject to Pulse's existing feature entitlements, typed schemas, target bindings, policy, and canonical lifecycle.

### Actor and approval contract

Only an authenticated human Pulse actor bound to the current organization and stable resource identity may request, approve, or execute a plan. Pulse AI, Patrol, a Komodo credential, and a Discord bot are not human actors. AI may submit a proposal to `internal/actionplanner`; it may not approve or execute. Discord approval, when implemented, is an authenticated interaction representing a mapped Pulse human actor and uses the same action lifecycle.

Every mutation must pass `ActionPlan -> ActionApprove -> ActionExecute` (or the equivalent existing canonical methods) in `internal/actionlifecycle`, and then the typed `internal/actionrunner`. Existing action feature gates and target capability checks remain in force. By default, one authorized approval binds the entire immutable ordered plan hash, including every step and its exact scope. No second plan-plus-step approval is required. A separate step approval is required only when an explicitly configured higher-risk policy names that step/action class and the policy revision is included in the plan hash. Approval evidence is bound to the human actor, organization, immutable plan hash (and step hash when a step approval is configured), provider/resource identity, resource revision, policy revision, control revision, evidence digest, method, and expiry. A single-administrator deployment may request and approve its own plan after normal authentication and any configured step-up authentication; requester/approver identity, timestamp, plan hash, and policy basis are audited. A distinct approver is required only by an explicit policy, never as a global default. Approval does not itself dispatch: execution remains an explicit authenticated lifecycle request, not a second approval. All requests, denials, decisions, dispatch receipts, reconciliations, and control changes are durably audited.

The central policy matches exact `(organization, provider, resource type, stable resource ID, operation ID, operation version, canonical argument schema)` tuples. No wildcards or name-only matching. The initial fork tuple set is empty. Change of target, argument, operation, resource version, policy revision, or plan ordering creates a new immutable action identity and invalidates all previous approvals. A database error or missing revision denies admission. After uncertain dispatch, keep budget/cooldown/concurrency reservations and reconcile durable runner evidence; never retry blindly.

### Defaults and action controls

All values below are proposed Phase 0 defaults, not production deployment values. Policy may be tightened without review; raising a limit requires a separately reviewed policy checkpoint before implementation.

| Limit | Proposed default |
|---|---:|
| Fork integration mutation allowlist | 0 operations |
| Maximum ordered steps in a future plan | 3 |
| Mutating operation concurrency across the deployment | 1 |
| Mutations per exact target/action tuple | 1 per 60 minutes; maximum 3 per target per rolling 24 hours |
| Evidence age at planning and again at each step admission | At most 60 seconds, with source-specific health and provenance present |
| Plan and approval expiry | 10 minutes; expiry requires a fresh plan and approval |
| Automatic mutation retry | 0 |
| Typed runner operation deadline | At most 30 minutes, matching the existing runner maximum |

Budgets, cooldowns, active slots, policy revision, and action audit are durable and reserved transactionally before dispatch. Unknown outcomes retain all reservations until reconciled or explicitly adjudicated under a separately reviewed policy. Any database read/write failure blocks the action; there is no in-memory fallback. Exact organization/target IDs and any later operation tuple are deployment policy inputs and are not checked into Git.

**Disable Automation** is durable and defaults enabled (automation disabled). It denies automated-origin admission and autonomous continuation. Human-authored Pulse actions remain eligible under ordinary checks. An automation-origin plan may be manually reviewed, approved, and executed only through an explicit human request and the full canonical lifecycle; preserving its automated provenance is mandatory. An already-admitted step may finish and must be reconciled, but the next step cannot start automatically. Restart never resumes automation. Re-enabling is an explicit authorized and audited human policy change and does not drain queued work.

**Total Emergency Stop** is a separate durable latch. It denies every new executable action admission, manual or automated, and prevents pending approvals from becoming executable and approved-but-undispatched actions from dispatching. Pending records remain inspectable; clearing the stop never releases a queue. Re-plan and approve each intended operation against current evidence and policy. For in-flight work, persist the stop request, request cancellation only when the typed executor explicitly supports safe cancellation, stop every later plan step, and reconcile receipts. Unknown outcomes remain uncertain; never infer rollback or retry. The latch survives restart. Missing/corrupt/unreadable policy state means stopped for admission purposes. Any authenticated human may invoke the stop. Clearing requires explicit confirmation by an authenticated Pulse administrator with step-up authentication and a durable audit record. A deployment may configure a second distinct administrator as an additional control, but two-person authorization is not mandatory. Clearing never turns Disable Automation off and never resumes pending work.

Both controls are checked at the same canonical action-admission boundary before creating any dispatch attempt. The stop action is confirmed to its caller only after durable commit. If the database is unavailable, deny actions and report the stop-state write as unconfirmed until an operator verifies it after recovery.

## 5. Operational cases and repair-plan state machines

### Case correlation and lifecycle

Cases do not replace Pulse alert occurrences. A case key includes organization ID, provider/connection UUID, canonical resource identity, alert rule/family ID, and any explicit topology relationship used to join a second resource. Correlate only within a 15-minute event-time window. Join cross-resource alerts only through an explicit Pulse topology edge; never correlate across organizations or by display name/text similarity alone. Repeated observations of one occurrence dedupe; separate alert occurrences remain separate members.

Membership, detach, split, merge, assignment, reopen, and resolution are append-only audited events. An alert retains its own Pulse lifecycle. A closed case is terminal; a later recurrence creates a new episode/case linked to the prior case rather than rewriting its resolution. Case states are `open -> triaging -> awaiting_plan -> awaiting_approval -> executing -> verifying -> resolved -> closed`; `blocked`, `partially_recovered`, and `uncertain` are held states with a recorded reason. Resolve only when all required occurrences have fresh recovery evidence, every action outcome is known, and post-action verification is complete. Closing is a separate audited decision.

### Repair plan and step lifecycle

The immutable plan hash covers the case ID, ordered step IDs, actor/origin, organization, provider and stable target IDs, typed operation/version, canonical argument digests, dependencies, evidence digests, resource revisions, policy revision, control revision, and expiry. A changed target, argument, order, dependency, evidence, policy, resource, or control revision makes a new plan and invalidates old approvals.

- **Plan:** `draft -> awaiting_plan_approval -> approved -> executing -> verifying -> succeeded`; held/terminal outcomes are `rejected`, `expired`, `cancelled`, `failed`, `partially_completed`, `blocked`, and `uncertain`.
- **Step:** `blocked -> ready -> admitted -> executing -> succeeded`; allowed outcomes are `failed`, `uncertain`, `cancelled`, and `blocked`. A step enters `awaiting_step_approval` only when an explicitly configured higher-risk policy requires it.
- One authorized plan-level approval binds the exact ordered plan hash and covers every step. It does not dispatch or pre-admit later work. Additional step-level approval is required only for steps identified by a versioned higher-risk policy, and binds that exact step hash and target. A configured distinct-approver rule applies only to the affected approval. Single-administrator installations remain operable under authenticated, audited default approval. Any plan or approval-relevant policy change invalidates affected approval evidence and requires a new immutable plan/approval.
- Recheck actor, controls, policy/resource revisions, evidence freshness, budgets, cooldown, concurrency, and executor capability atomically before every step. Stop before admitting any later step after failure, block, cancellation, or uncertainty. A completed earlier step remains recorded; no automatic rollback/compensation or blind retry occurs. A compensating operation is a new typed plan and receives its own admission and approvals.
- Emergency stop prevents all later steps. Disable Automation prevents autonomous continuation and requires a fresh human execute request and complete revalidation for each remaining step. A restart recovers state from durable records only; it never invents completion or resumes a plan.

## 6. Discord operator identity and authorization

Use Discord application interactions (slash commands); do not scrape messages or require privileged message-content access. For webhook-delivered interactions, validate Discord's request signature and timestamp before parsing/acting; use the immutable interaction ID as a one-time replay key. For gateway delivery, require the authenticated Discord application/bot session and dedupe by interaction ID.

Map Discord's immutable user ID to one Pulse actor ID. Deployment inputs are the Discord application ID, allowed server/guild IDs, channel IDs, role IDs, and the explicit Discord-user-to-Pulse-actor mapping. Usernames, nicknames, display names, message text, and channel membership alone never identify an operator. Resolve the user's current server and role membership on each command, approval, and execution request; revocation or mismatch denies access. Commands are read-only by default. Any approval must bind the mapped human actor to Pulse approval evidence and current role, and still obey requester separation, both action controls, expiry, and the canonical action lifecycle.

The bot token and Discord public key are operator secrets stored via protected configuration. The token is never exposed to the browser or stored in planning documents. Use least-privilege application scopes and ephemeral responses for sensitive case/action details. If Discord is unavailable, approval through Pulse's authenticated UI remains available; no queued Discord interaction executes after recovery without fresh actor, role, expiry, control, and policy checks.

## 7. Metric retention, storage budget, and recovery

The fork metric target reuses existing Pulse rollups and exact target tiers: raw samples for 2 hours and minute averages for 24 hours where those short-duration tiers are supported; 5-minute averages for 30 days; hourly averages for 365 days; and daily averages for 730 days. Do not introduce a second store or independent rollup engine. These are fork capability settings; commercial license feature reports remain unchanged. `fork.long_term_metrics` is unavailable until the central profile includes it and a positive disk budget is supplied.

`metrics_storage_budget_bytes`, metrics data path, backup destination, and restore target are operator inputs. Qualify the bounded storage budget against representative series cardinality, sample rates, tag/index overhead, SQLite database plus WAL growth, rollup and retention work, backup space, and query/write latency. Record measured bytes per series/tier, projected 730-day storage, safety headroom, and recovery time; enable the capability only when the measured projection fits the operator-approved budget. At 80% of budget, emit a persistent warning. At 90%, block further retention increases and historical backfill, and raise a critical storage alert. At the hard budget, preserve configured retention horizons and existing history; never silently truncate daily retention to seven days or delete older daily points to meet a quota. Make any data loss/gap explicit and alert operators if new writes must be rejected. Any destructive retention reduction requires explicit authenticated operator configuration, a preview of affected time ranges/series, confirmation, audit, and a verified backup/restore plan before pruning. Preserve separate alert/action databases and never use a storage failure to authorize action admission.

Use SQLite's existing transactional writes, bounded rollup/retention work, WAL checkpointing, and incremental reclaim. Require a daily consistent backup (destination is operator input), startup integrity validation, and a documented restore procedure to an isolated copy before replacing the live metrics file. Recovery must preserve timestamps, resource identities, rollup tiers, and entitlement/profile state; any recovery gap is surfaced as unknown evidence, never healthy. Phase 6 qualification measures series-volume growth, DB+WAL size against the configured budget, write/query latency, retention boundaries, checkpoint/reclaim behavior, full-disk behavior, crash restart, corruption handling, backup freshness, and restore correctness.

## 8. Release ownership, isolated acceptance, and rollback

The release owner, operations owner, approval authority, artifact location, deployment endpoint values, storage path/budget, backup destination, alert thresholds, and rollback operator are deployment inputs. No such values belong in this repository. Phase 9 must pass reproducible end-to-end acceptance in a disposable isolated environment with non-production accounts and no production credentials. It covers provider outages, Komodo read-only permission rejection, credential rotation/provenance, alert/case recovery, action denial and approval state machines, emergency stop, restart, metric budget pressure, backup/restore, Discord identity revocation, and rollback.

Phase 10 occurs only after Phase 9. Produce and verify the release artifact, schema/config compatibility, backup/restore evidence, secret rotation, feature profile defaults, health checks, rollback triggers, named owner, and tested rollback procedure. Preserve the agreed **one production cutover** decision for the initial release. This Phase 0 work authorizes no deployment, production configuration change, or second cutover. Rollback restores the prior known-good artifact/config/data state; another forward cutover requires new explicit approval.

## 9. Phase gates and remaining inputs

The dependency order and validation gates in the roadmap remain authoritative. Phase 0 baseline tests and audits are recorded with the Git checkpoint. Future implementation gates remain:

1. Fork profile: resolver/API/frontend tests prove default-empty, unknown-ID fail-closed behavior, separate entitlement fields, and enforcement across every protected route.
2. Codex-LB: provider/config/AI tests plus live isolated v1.24.0 compatibility; prove no fallback and redaction under every listed failure.
3. Komodo: adapter/config/unified-resource tests plus isolated v2.3.3 API; prove exact methods/types/IDs, pagination/freshness, secret rotation/redaction, and zero action authority from setup/discovery.
4. Triage: deterministic replay tests for duplicate, ordering, stale, missing, contradictory, recovery, and topology cases.
5. Action policy: action planner/lifecycle/runner/capability and persistence tests for both control matrices, exact allowlists, identity/revision drift, durable reservations, concurrency, stale evidence, DB failures, default single plan approval, configured higher-risk step approvals, single-administrator operation, emergency-stop clearing, uncertain outcomes, and restart.
6. Metrics: store/licensing/API tests plus bounded load, full-disk, backup, and isolated restore qualification.
7. Cases/plans: API/UI/store tests for correlation, membership events, immutable hashes, one plan approval by default, configured step approvals, partial execution, stop-on-failure, uncertainty, restart, and alert-lifecycle isolation.
8. Discord: signature, interaction replay, stable identity mapping, role revocation, control/approval binding, outage, and no-direct-dispatch tests.
9. Isolated end-to-end acceptance, followed by Phase 10 release readiness and rollback review.

Operator inputs still required before isolated live-provider qualification are the Codex-LB endpoint, secret reference and model; Komodo Core URL, credential reference, resource selection and poll interval; Discord application/guild/channel/role/user mappings; metrics budget/path/backup destination/restore target; and named release/operations owner. No values are filled in here. The initial typed action allowlist remains empty. These inputs and the proposed numerical policy defaults require independent review; no Phase 1 application development begins until that review approves the contracts.

### Inherited browser-test recommendation (planning only)

For `frontend-modern/browser-tests/backup-date-chronology.cjs`, update the fixture assertion to check one valid replacement age label, matching the current single latest-backup age rendered by the row. Measure clipping against the age cell's actual bounds at desktop and narrow widths, with fractional CSS-pixel handling so subpixel glyph overhang is not treated as clipped content. The known two-label expectation is obsolete; targeted investigation found the same replacement-gap failure on Node 20 and 22, while the current `Unknown` to valid-age behavior passes the focused unit tests. This is a test-only recommendation. Do not edit the `.cjs` file until the operator separately authorizes test-file changes.

## Checkpoint verification

Checks were run against the merged upstream baseline on 2026-10-23. Full command output and failure notes are reported with the Git checkpoint.

| Check | Result |
|---|---|
| `go test ./internal/alerts` | One environment-specific failure: `TestLoadHistory_PermissionError` expects mode `000` to be unreadable, but this container runs as root and can read it. The same suite passed when only that test was skipped; current upstream alert suite reported 2,784 passing tests. |
| `go test ./internal/actionplanner ./internal/actionlifecycle ./internal/actionrunner ./internal/agentcapabilities` | Passed; 835 tests across 4 packages. |
| Frontend `npm run type-check` | Passed. npm reported Node 20.19.4 while `eslint-plugin-solid` declares Node >=22; the type check still exited 0. |
| Frontend Vitest suite | Passed: 1,264 files and 22,163 tests; 1 file and 3 tests skipped. jsdom printed navigation/scroll warnings after completion; command exited 0. |
| `InvestigationSection.test.tsx` after latest upstream merge | Passed: 11 tests. |
| `build-backup-date-observation.mjs` and `build-snapshot-date-observation.mjs` | Both fixtures built successfully. |
| `backup-date-chronology.cjs` | Failed at the replacement-age gap assertion (`replacementGap` was `null`) under both Node 20 and Node 22. The test waited for one `1m` value but the fixture expected two direct child age labels. A debug rerun also timed out waiting for the coverage row; no application or test code was changed. This remains an upstream browser-test issue for review. |
| `scripts/check-snapshot-date-observation.cjs` | Passed in Chromium and WebKit at desktop (1280×900) and narrow (320×812) viewports. The manager produced identical active alert evidence for stale/missing/future dates and an empty result after trusted recovery; 16 local API reads, 8 screenshots, no page errors or horizontal overflow. |
| `scripts/repo-boundary-regression.py` | Passed. |
| `scripts/audit-private-boundary.sh --enforce` | Passed. Seven private-domain files remain baselined, zero new private implementation leakage; import boundaries passed. |
| `npm ci` audits | Passed with zero reported vulnerabilities. Browser runner dependencies were installed from the existing integration lockfile; no package or lockfile change is part of this checkpoint. |
| Planning-document scan and Git whitespace check | No credential/private-infrastructure pattern matches; no Sindri workspace files are included. |

## Evidence reviewed

- Pulse `LICENSE`, `docs/REPO_BOUNDARY.md`, `pkg/licensing/features.go`, `pkg/licensing/feature_map.go`, `internal/config/ai.go`, `internal/ai/providers/openai.go`, `internal/actionrunner/types.go`, `internal/actionplanner/`, `internal/actionlifecycle/`, `internal/monitoring/`, `internal/unifiedresources/`, and `pkg/metrics/store.go`.
- Codex-LB official [client setup](https://soju06.github.io/codex-lb/client-setup/), [API key documentation](https://soju06.github.io/codex-lb/api-keys/), and [release list](https://github.com/Soju06/codex-lb/releases). The live model catalog is authoritative; `/v1` is OpenAI-compatible and does not preserve the native Responses reasoning state expected by some clients.
- Komodo official [API and clients](https://komo.do/docs/ecosystem/api), [permissioning](https://komo.do/docs/configuration/permissioning), [API schema for v2.3.3](https://demo.komo.do/docs), [release list](https://github.com/moghtech/komodo/releases), and [project license](https://github.com/moghtech/komodo). Komodo identifies the project/client schema as GPL-3.0; this plan approves protocol interoperability only.
- Discord official [application commands](https://discord.com/developers/docs/interactions/slash-commands) and [interaction security documentation](https://discord.com/developers/docs/interactions/overview#security-and-authorization).
