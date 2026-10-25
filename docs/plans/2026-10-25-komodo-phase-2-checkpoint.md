# Pulse + Sindri — Komodo Phase 2 foundation checkpoint

**Status:** focused implementation for independent review on `feature/komodo-phase2-foundation`, based on current `origin/sindri`. This checkpoint is not merged or deployed. Codex-LB remains optional and deferred. Phase 0 read-only, least-privilege, and action-safety contracts remain in force.

## Contract and implementation boundary

The Komodo client uses the documented Core v2 HTTP shape: `POST /read/<method>` with `X-Api-Key` and `X-Api-Secret`. Its exact local allowlist is `GetVersion`, `ListPermissions`, `GetPermission`, `ListServers`, `ListStacks`, `ListDeployments`, and `ListSwarms`. It uses HTTPS with normal certificate validation, TLS 1.2 minimum, no redirects, bounded request/response sizes, a 10-second default timeout capped at 30 seconds, and bounded zero-based pagination. It validates Core's reported version as semantic major 2; it does not assume or pin patch `2.3.3`. Unsupported response shapes fail collection.

Credentials are stored through the protected encrypted `komodo.enc` configuration. Add and endpoint/credential changes verify the candidate before saving; a failed rotation leaves the stored values unchanged. Responses expose a fixed mask. The adapter retains only normalized ID, name, state, and supported parent fields. Raw API payloads and configuration/environment fields are not placed in records, errors, logs, AI context, or telemetry. The adapter registers no Komodo execute/write method, action capability, action runner, or repair authority.

Scope handling rejects direct permission grants above exact `Read` and rejects any specific grants such as `Logs`, `Inspect`, `Terminal`, `Processes`, or `Attach`. It checks effective `GetPermission` on one visible item per non-empty resource kind. A successful empty inventory requires a direct type-wide `Read` grant on target ID `all`, because an empty list alone cannot prove read access. Core permission inheritance and mixed per-resource permission behavior need qualification against an actual operator-selected Core release; see unresolved decisions below.

The org-scoped poller uses Pulse's supplemental-record provider boundary. Success with an empty inventory clears prior records. A failed poll retains the last-known records and marks them stale. A saved disabled connection publishes no records; deleting a connection removes its cached records after configuration sync. AI availability is not read by the poller and does not gate canonical alert evaluation.

## Existing provider architecture reviewed

- Proxmox uses Pulse's platform polling and registry paths in `internal/monitoring/platform_poller_shared.go`, `poll_providers.go`, and `monitor.go`. Its behavior is left unchanged.
- TrueNAS provides the closest supplemental-provider model: `internal/api/truenas_handlers.go` owns connection setup while `internal/monitoring/truenas_poller.go` owns org-scoped polling, cached status, failure retention, and normalized records. Komodo follows those boundaries without changing TrueNAS.
- VMware's existing poller and `internal/api/connections_aggregator.go` show how provider health is projected into shared connection status. Komodo status is added to that aggregator; canonical connection-alert conversion remains a separate follow-up.
- `internal/unifiedresources/monitor_adapter.go` already accepts supplemental records and `SnapshotOwnedSources` ownership declarations. The Komodo poller publishes through that API into the existing registry; it adds no scheduler or resource registry.
- `internal/unifiedresources/metrics_targets.go` is the canonical history-target map. Komodo has no native history mapping; an explicitly linked guest can reuse its existing Proxmox history target when that source identity is unambiguous.
- `internal/api/router.go` owns poller startup and shutdown, while existing infrastructure settings and `ConnectionEditor` components own connection configuration. The Komodo UI uses those surfaces and the common `ConnectionType` API.

## Resource mapping matrix

All Komodo source IDs are opaque and stable within the Pulse connection: `komodo:<base64url(connection UUID)>:<lowercase kind>:<base64url(native Komodo ID)>`. The registry then derives its canonical source-specific ID from `(Komodo resource type, SourceKomodo, source ID)`. Names and labels are display data only.

| Komodo kind | Pulse resource type | Parent / relationship mapping | Metrics and history key | Facet need |
|---|---|---|---|---|
| Server | `komodo-server` | Each listed Swarm `server_ids` entry produces Server `member_of` Swarm. No singular parent is inferred. | No Komodo-native history series. If explicitly linked to one Proxmox guest, the existing Proxmox VM/LXC history target is retained even when the Komodo Server is canonical. If multiple Proxmox history IDs conflict and Komodo is primary, the target is omitted as ambiguous. | No new facet for current inventory fields. |
| Swarm | `komodo-swarm` | No singular parent. Membership is represented by `member_of` edges from known Servers. | None assigned; no historical series. | No new facet for current inventory fields. |
| Stack | `komodo-stack` | `hosted_by` the same-connection Swarm when `swarm_id` is present; otherwise the same-connection Server from `server_id`; otherwise no parent. | None assigned; no historical series. | No new facet for current inventory fields. |
| Deployment | `komodo-deployment` | `hosted_by` the same-connection Swarm when `swarm_id` is present; otherwise the same-connection Server from `server_id`; otherwise no parent. | None assigned; no historical series. | No new facet for current inventory fields. |

The existing generic `Resource`, source, status, timestamp, `ParentSourceID`, and relationship support is sufficient for this inventory foundation. Komodo types intentionally remain disjoint from Docker and Proxmox types; no name-based merge is attempted. An operator can explicitly confirm identity between a Komodo Server and a Proxmox VM with the existing resource link API. Registry merges preserve and rewrite graph endpoints, so a Stack hosted by that Server resolves to the linked canonical VM.

The Komodo schema documents both `server_id` and `swarm_id` for Stacks and Deployments, with `swarm_id` taking precedence when both are set. The mapping follows that rule. Swarms expose `server_ids`; only matching Servers present in the same authorized inventory are connected by `member_of` edges.

If Komodo metrics are added later, choose a stable metric family and key explicitly. A candidate is the connection-scoped source ID above, but no writer or reader currently uses it. Resource-specific details beyond the allowlisted inventory, performance counters, deployment logs, or configuration views would require an explicit Komodo facet or a separately designed generic detail contract. Do not retain raw configuration or environment data to populate a facet.

## Cross-provider relationship audit

Pulse already has a central relationship graph and an operator-confirmed identity-link store:

- `ResourceIdentity` and `bySource` preserve provider identities under the central registry. Komodo records carry empty inferred identity, so equal names do not cause identity matches. The new integration test also puts a same-named Proxmox resource and Komodo Server through the registry and asserts that they remain separate until linked.
- The existing operator path is `POST /api/resources/{canonical-id}/link` with `targetId` and optional reason; `/unlink` splits a pair. The API delegates to the org-scoped `ResourceStore`. Its SQLite store persists `resource_links`, and registry generations reapply those decisions when snapshots rebuild. This is an identity merge, not a dependency edge. Operators should only link when they have verified that both source records identify the same machine.
- Provider parent relationships use `ParentSourceID` and source-specific parent resolution; the registry materializes canonical parent IDs and can expose multiple `ResourceRelationship` edges. Edges carry type, confidence, evidence, discoverer, active state, timestamps, and safe metadata. This checkpoint updates central merge handling so operator identity folds rewrite relationship endpoints and deduplicate edges rather than lose them or leave edges pointing at a folded ID.
- Docker network attachment edges are derived again from Docker-native inventory. Komodo edges are likewise rebuilt from each successful inventory. Generic dependency or route edges have no independent durable operator store; `routes_to` is now a typed value, but has no producer or Traefik configuration parser.
- `ResourceRelationshipsWithCanonicalParent` feeds resource API/detail projections. The generic Infrastructure resource detail drawer already has a “Relationship map”. Alert incident synthesis traverses active canonical relationships to build causal/correlated context; relationships do not independently close or suppress alerts. AI chat and handoff code can include generic relationship context, but Komodo-specific Patrol context still needs qualification and explicit product decisions. Polling does not depend on either.

Future cross-provider routing or dependency work should extend this central graph and add explicit edge origin, verification, and persistence where operator-authored edges are required. Reuse the existing operator link for verified resource identity. Do not convert name similarity into identity or treat a dependency edge as an identity merge.

## Phase 2B identity and relationship validation

This follow-up validates the report above against the existing Proxmox, Docker, availability, and unified-resource paths. It makes one focused correction: when the operator selects a Komodo Server ID as the canonical ID for a Proxmox guest, the merged resource now keeps the Proxmox-owned metrics/history target. The root resource type and canonical ID still follow the operator's selected primary; its `Sources` and source-target mappings retain both provider identities.

### Identity and relationship findings

- **`same_as` is identity, not topology.** The existing `ResourceLink` / `resource_links` store is the durable operator decision. The chosen `PrimaryID` is the canonical resource ID while both linked members are present. Registry folding keeps both `SourceTargets`, rewrites parent and graph endpoints, and deduplicates edges. Names alone never create an identity link.
- **Explicit links are transitive.** Existing `ResourceLink` components are equivalence sets, not one-to-one provider pairings. The regression fixture links one Komodo Server to two Proxmox VM IDs and confirms the current deterministic fold into the earliest selected primary. This preserves the explicit operator decisions, but it can combine two native identities from the same provider; whether the link API should reject or surface that collision remains open.
- **Missing providers fail closed.** A stale source that still has a last-known record remains in the fold and keeps the primary ID. If a linked source is absent from a complete inventory, its remaining partner stays available under its own observed ID; the stored decision remains in the org's resource store and folds the records again when that source returns. The implementation does not synthesize a missing resource or guess that an ID has not been reused. Consequently, the canonical ID may temporarily change if the selected primary's record is removed, then returns to the selected primary ID on recovery.
- **Metric ownership follows the metric-producing source.** When the Komodo Server is canonical and a Proxmox guest facet is present, the resource still targets `vm/<Proxmox source ID>`; a Proxmox LXC facet targets `system-container/<source ID>`. Komodo-only Servers and linked Proxmox nodes without a guest facet do not invent a target. If the Proxmox record is absent, the Proxmox target is absent until that source returns.
- **Conflicting metric targets fail closed.** When the selected canonical ID is one of multiple Proxmox identities, metrics target selection now chooses the Proxmox candidate that generated that canonical ID. If Komodo is primary and multiple Proxmox history IDs are in the same explicit link component, no Proxmox history target is returned instead of choosing one by map iteration order.
- **Parent and child edges stay in the central graph.** Komodo Stack/Deployment parents resolve to the linked Proxmox VM when their parent is the linked Server. A Server's Swarm `member_of` edge is rewritten to start at the canonical VM and retain the canonical Swarm target. Docker container parent edges and network attachment edges are already materialized by the Docker adapter in this registry; a Docker host can be explicitly identity-linked to a Proxmox VM or Komodo Server and child endpoints follow the fold.
- **Availability association stays additive.** An explicit check configured against the Komodo Server ID resolves through the operator link after the infrastructure records are present. The check remains its own availability endpoint, projects its availability facet to the canonical VM, and emits a `checks` edge to that VM. Full snapshot rebuilds intentionally process availability records after infrastructure sources. An incremental check that arrives before its referenced resource is discovered remains unresolved until a later check poll; it never falls back to name matching.
- **Consumers use existing projections.** The resource API emits canonical relationships; the Infrastructure resource drawer renders its Relationship map; alert incident synthesis traverses active typed edges; and AI/correlation/context code consumes generic edges. Storage topology is a domain-specific projection over storage resources, not another general resource graph. `routes_to` is now part of the shared relationship vocabulary and frontend type, but no parser, route producer, route UI, or route-based alert behavior is implemented here.

### Relationship semantics and persistence decision

| Meaning | Registry representation | Persistence and refresh |
|---|---|---|
| `same_as` | Existing operator `ResourceLink`, which folds source identities into one canonical row. It is deliberately not a self-edge in `ResourceRelationship`. | Existing org-scoped `resource_links` table in the SQLite `ResourceStore`; stable until operator unlink/split. |
| `runs_on` / `hosted_by` | Directed `ResourceRelationship` or the same meaning projected from source-specific `ParentSourceID`. | Source-owned edges are emitted from normalized inventory and refreshed with the owning provider's complete snapshot. A failed collection keeps last-known records and edges with stale source status; a successful removal/empty snapshot removes the provider's omitted resources and edges. |
| `routes_to` | New typed `RelRoutesTo` value on the existing edge model. | No producer or operator route API yet. Traefik parsing and route editing remain deferred. |
| `depends_on` | Existing typed `RelDependsOn` value on the existing edge model. | No durable operator dependency API yet. Do not store these decisions in `resource_links`. |

When operator-authored `depends_on` or `routes_to` is approved for implementation, persist those edges through the existing per-organization `ResourceStore` and SQLite database, in a table separate from `resource_links`. Store immutable edge ID, source and target canonical IDs, type, actor, reason, creation/update times, and an explicit removal state. Reconcile endpoints through the registry's identity-reference resolution at read/materialization time. Provider polling must never delete operator-owned rows. Keep automatic edges in provider snapshots with a declared discoverer/owner, and replace only that provider's edges after a successful complete collection; collection failures retain last-known edges and stale status. Do not create a second database or relationship engine. Decide unresolved-endpoint presentation and audit/retention policy before adding the API.

For the requested example, the graph would fold each Proxmox/Komodo host pair only through its explicit `same_as` link, represent the Jellyfin and Traefik containers with `runs_on` edges to their canonical hosts, and eventually represent the proxy-to-backend path as `routes_to(Traefik, Jellyfin)`. The route's concrete target identity and refresh/verification policy remain open; this is a relationship model, not automatic Traefik parsing.

### Live Core compatibility status and operator action

No live Komodo endpoint or Komodo-named environment variable/approved credential was available in this workspace. No network call or live compatibility claim was made. Before live qualification, an operator must supply or select an authorized Komodo Core endpoint and existing read-only API key/secret, then verify: (1) `GetVersion` reports the installed Core v2 release; (2) the configured principal has only inventory `Read` scope for Server, Stack, Deployment, and Swarm and no specific Logs/Inspect/Terminal/Processes/Attach grants; (3) the listed native IDs are stable; and (4) Stack/Deployment `server_id`/`swarm_id` and Swarm `server_ids` match that release's returned items. Record the actual version and safe fixture shape, never the credentials or raw configuration. Do not create a privileged credential for this check.

## Remaining integration points

| Area | Current foundation | Remaining work |
|---|---|---|
| Historical metrics | Komodo has no native history family. A single linked Proxmox guest retains its existing VM/LXC target; ambiguous multiple Proxmox targets fail closed. | Pick any future Komodo-native metric family/key, add a bounded writer and history API target, retention and migration behavior, charts, and tests. Decide whether each resource kind has useful metrics before adding a facet. |
| Connection alerts | Komodo poll health appears in the connection aggregator and redacted status response. | `internal/api/connections_alerts.go` still maps only PVE/PBS/PMG/VMware/TrueNAS into `alerts.ConnectionSnapshot`; add Komodo connection state/category mapping and canonical alert lifecycle tests if these failures should raise connection alerts. |
| Notification routing | Pulse's existing notification engine handles canonical alert notifications. | Decide whether Komodo connection/resource alert classes need routing preferences. Relationship-based destination selection is not implemented; add it only through the existing alert/notification flow after edge persistence and policy semantics are defined. |
| Monitoring UI | Existing infrastructure settings workspace can add/test/edit/enable/disable/delete Komodo connections. The generic resource drawer can display relationships. | Add resource-kind presentation and meaningful detail/history views if operators need them; expose no Komodo action controls. Decide how stale/disabled/empty status appears in the resource inventory and connection view. |
| Patrol / AI context | Generic resource relationship context exists in AI service and handoff paths; Komodo polling has no AI dependency. | Qualify how generic Komodo types appear in Patrol's resource selection/context, add only safe normalized fields, preserve AI-unavailable monitoring/alerts/notifications behavior, and test that no configuration or environment data enters context. |
| Durable dependencies and routes | Central `ResourceRelationship` supports source-discovered directed edges. | Add explicit durable storage and API/UI for operator-authored `depends_on`/`routes_to` edges, provenance, verification, expiration/removal, and audit. Add parsers only for explicitly supported sources such as Traefik if later approved. |

## File-by-file implementation report

| File | Change |
|---|---|
| `docs/plans/2026-10-23-sindri-integration-roadmap.md` | Updated current order to Komodo Phase 2 and Codex-LB optional later; recorded cross-provider relationship foundation and preserved prior branch/order details as historical context. |
| `docs/plans/2026-10-23-sindri-phase-0-contracts.md` | Preserved read-only/action-safety terms and clarified runtime Core v2 compatibility, protected storage, API allowlist, and AI independence. |
| `docs/plans/2026-10-25-komodo-phase-2-checkpoint.md` | Added this contract, mapping matrix, relationship audit, integration gaps, and review handoff. |
| `internal/config/komodo.go` | Added Komodo connection schema, UUID/HTTPS validation, redaction, and masked-secret preservation. |
| `internal/config/komodo_test.go` | Added config validation, masking, and credential rotation coverage. |
| `internal/config/persistence.go` | Added encrypted Komodo config load/save at `komodo.enc`. |
| `internal/komodo/client.go` | Added bounded protocol-only client, allowlisted read calls, pagination, Core major-version validation, safe normalization, and permission checks. |
| `internal/komodo/client_test.go` | Added auth, version, scope, pagination, duplicate, timeout, transient failure, allowlist, and redaction fixtures. |
| `internal/komodo/normalize.go` | Added stable connection/kind/native identities, disjoint resource types, safe status, parent edges, and Swarm membership. |
| `internal/komodo/normalize_test.go` | Added identity isolation, no name merge, registry ingestion, parent/membership, and operator-linked Proxmox relationship tests. |
| `internal/unifiedresources/types.go` | Added `SourceKomodo` and the four disjoint Komodo resource types. |
| `internal/unifiedresources/platform_scopes.go` | Added Komodo to existing platform/source scoping. |
| `internal/unifiedresources/relationships.go` | Added relationship merge/rewrite helpers and Komodo hosted-by parent semantics. |
| `internal/unifiedresources/registry.go` | Preserved/deduplicated relationships during canonical resource merges and rewrote endpoints after identity folds. |
| `internal/monitoring/komodo_poller.go` | Added independent organization-scoped polling through supplemental record snapshots, including stale, empty, disabled, and removed states. |
| `internal/monitoring/komodo_poller_test.go` | Added organization separation and empty/failure/stale/disable/removal/snapshot coverage. |
| `internal/api/komodo_handlers.go` | Added protected connection list/add/update/delete/test handlers with pre-save candidate verification and redacted poll status. |
| `internal/api/komodo_handlers_test.go` | Added CRUD, credential preservation/rotation, candidate test, redaction, and denied action-route coverage. |
| `internal/api/connections_aggregator.go` | Added Komodo connection type, summaries, status, and health to existing aggregation. |
| `internal/api/connections_aggregator_test.go` | Added mapping tests for healthy-empty, stale, failed, disabled, pending, and credential-safe Komodo connection status. |
| `internal/api/connections_alerts.go` | Added Komodo poll-summary input; canonical alert snapshot mapping remains deferred and is documented above. |
| `internal/api/connections_handlers.go` | Wired Komodo poll summaries into the existing connection API. |
| `internal/api/connections_types.go` | Added the Komodo connection type. |
| `internal/api/platform_connection_shared.go` | Added candidate pre-save validation to the shared update flow for verified endpoint/credential rotation. |
| `internal/api/router.go` | Constructed and started/stopped the Komodo poller; registered its supplemental source in live and mock adapters. |
| `internal/api/router_routes_registration.go` | Added org-admin/settings-scoped Komodo connection routes. |
| `frontend-modern/src/api/komodo.ts` | Added typed Komodo connection API calls. |
| `frontend-modern/src/api/connections.ts` | Added Komodo to connection type and enable/disable/delete routing. |
| `frontend-modern/src/components/Settings/ConnectionEditor/CredentialSlots/KomodoCredentialSlot.tsx` | Added read-only connection editor with test, safe masked credentials, enable toggle, and rotation UI. |
| `frontend-modern/src/components/Settings/ConnectionEditor/CredentialSlots/__tests__/KomodoCredentialSlot.test.tsx` | Added editor credential and action-safety presentation tests. |
| `frontend-modern/src/components/Settings/ConnectionEditor/useConnectionEditor.ts` | Routed Komodo connection editing to its credential slot. |
| `frontend-modern/src/components/Settings/useConnectionsLedger.ts` | Added the Komodo connection label for the shared connection ledger. |
| `frontend-modern/src/components/Settings/InfrastructureSourcePicker.tsx` | Added Komodo as a selectable infrastructure source. |
| `frontend-modern/src/components/Settings/InfrastructureWorkspace.tsx` | Added Komodo connection configuration in the infrastructure workspace. |
| `frontend-modern/src/components/Settings/infrastructureWorkspaceModel.ts` | Added Komodo read-only source-model mapping. |
| `frontend-modern/src/utils/infrastructureOnboardingPresentation.ts` | Added Komodo labels and read-only setup support; Codex-LB remains deferred. |
| `frontend-modern/src/utils/__tests__/infrastructureOnboardingPresentation.branchcov2.test.ts` | Added Komodo's search aliases to the typed source-picker fixture. |

## Unresolved compatibility decisions

1. **Core v2 minor compatibility:** the live OpenAPI page currently identifies itself as Core v2.3.4 and the latest published Rust reference reviewed is v2.3.2; the client accepts any semantic v2 version and relies on response validation. Only mock fixtures have been run. Run a disposable compatibility suite against the actual operator-selected Core release and record its `GetVersion` result. Decide whether the minimum supported minor can be stated from tested evidence.
2. **Permission inheritance and granularity:** official permission docs say `ListPermissions` excludes inherited UserGroup grants while `GetPermission` includes them. Empty inventories currently fail closed unless the direct `all` grant proves scope. Non-empty inventory checks `GetPermission` for one visible item per kind; determine from the actual Core contract whether list results are already filtered and whether mixed per-item scope must be checked individually. Specific grants are rejected.
3. **Resource relationship fields:** the reviewed v2.3.2 client schema documents Stack and Deployment `server_id`/`swarm_id` fields, Swarm `server_ids`, and Swarm precedence over Server. Qualify those response fields and permission-filtered parent visibility against the actual operator-selected Core release. Current parent choice follows that precedence and does not invent a second singular parent.
4. **Metric families:** no Komodo history key or writer is selected. Define whether Komodo exposes metrics useful enough to retain before extending generic history or adding facets.
5. **Generic dependency persistence:** decide edge ownership, confirmation, versioning, expiry, and notifications before adding operator-maintained `depends_on` or `routes_to` edges.
6. **Connection alert policy:** decide which Komodo failures and disabled states should produce canonical connection alerts and whether a successful empty inventory is healthy for all four resource kinds.
7. **Operational settings:** resource-type selection and per-connection poll interval are not implemented; current polling interval is fixed and bounded. Actor/source credential provenance and endpoint verification fingerprints also remain future work.

## Phase 2B implementation delta

| File | Change |
|---|---|
| `internal/unifiedresources/metrics_targets.go` | Preserve Proxmox-owned VM/LXC history targets when an operator selects a linked Komodo Server as canonical; for multiple Proxmox IDs in one link component, use the canonical candidate or fail closed if ownership is ambiguous. |
| `internal/unifiedresources/metrics_targets_test.go` | Cover linked Komodo Server targets for Proxmox VM, LXC, non-guest, and ambiguous same-source cases. |
| `internal/unifiedresources/relationships.go` | Add the typed `routes_to` relationship vocabulary while keeping `same_as` represented by identity links. |
| `internal/unifiedresources/relationship_presentation.go` | Add the `Routes to` presentation label. |
| `internal/unifiedresources/relationship_presentation_test.go` | Cover `routes_to`, `hosted_by`, and graph vocabulary registration. |
| `internal/unifiedresources/store.go` | Clarify `ResourceLink` as durable operator-confirmed same-as identity state, separate from dependency/topology edges. |
| `internal/unifiedresources/availability_link_test.go` | Prove an availability check linked to a folded Komodo Server projects to the canonical Proxmox VM and keeps its `checks` edge. |
| `internal/komodo/normalize_test.go` | Prove the Komodo-primary canonical choice still retains the Proxmox VM history target and both provider sources. |
| `internal/komodo/cross_provider_test.go` | Add SQLite restart, exact source identity, parent/membership rewrite, availability association, source polling order, stale/empty-snapshot removal/recovery, ambiguous same-name and transitive-link conflict coverage, history-target selection, and org isolation. |
| `frontend-modern/src/types/resource.ts` | Type the shared `hosted_by`, `routes_to`, `member_of`, `stores_on`, and `protected_by` values for the existing relationship projection. |
| `docs/plans/2026-10-23-sindri-integration-roadmap.md` | Add Phase 2B gates and the relationship persistence direction; preserve metrics/alerts/notifications/Patrol as later work. |
| `docs/plans/2026-10-25-komodo-phase-2-checkpoint.md` | Add the Phase 2B audit, missing-source semantics, relationship persistence decision, live-test operator steps, and delta report. |

### Phase 2B decisions still open

1. **Canonical continuity while a selected primary is absent:** links reapply when both provider identities return. While the selected primary is absent entirely, the survivor remains visible under its own ID rather than inheriting a synthetic ID/type. Decide whether a durable canonical anchor and its source-history coordinates are required before enabling operator links on ephemeral provider identities.
2. **Stale edge interpretation:** source poll failure retains last-known edges and the provider resource's stale `SourceStatus`; the edge's `Active` flag alone does not encode observation freshness, and current alert synthesis traverses active edges. Define edge freshness semantics before routing/dependency edges influence causal or notification decisions.
3. **Operator dependency edge ownership:** approve actor/reason fields, edge removal/audit/retention policy, unresolved endpoint projection, and store-table migration before exposing `depends_on`/`routes_to` mutations. Reuse the existing org `ResourceStore`; do not reuse `resource_links` or add a separate database.
4. **Route semantics:** define the direction, target identity, and refresh behavior for reverse-proxy routes; `routes_to` currently has no producer and does not change alert/AI traversal policy.
5. **Proxmox non-guest ownership:** the Komodo-primary metrics fix intentionally maps linked Proxmox guest facets only. Decide whether Komodo Server links to a Proxmox node/agent should expose a node or agent history target and how to represent a resource with multiple metric owners.
6. **Incremental availability ordering:** full registry rebuilds process availability after infrastructure sources. A check processed before its referenced provider appears stays unresolved until its next poll; decide whether incremental provider publication should trigger a re-correlation pass.
7. **Conflicting explicit identity links:** `ResourceLink` components are transitive. Linking one Komodo Server to two Proxmox VM IDs currently folds both IDs into one identity component, with the earliest selected primary as canonical. Decide whether the identity-link API should reject same-provider collisions or treat operator-authorized transitive components as authoritative and surface the conflict. A Komodo-primary component with multiple Proxmox history IDs now has no metrics target until that ownership is unambiguous.

## Validation and review status

Passed checks:

- `rtk go test ./internal/unifiedresources -count=1` — 3,036 tests in the Phase 2B validation run, including registry ingestion and operator-confirmed cross-provider identity-link relationship preservation.
- `rtk go test ./internal/komodo -count=1` — client fixtures for authentication, Core v2 validation, scope, pagination, duplicates, timeouts, transient failures, specific permission rejection, and redaction.
- `rtk go test ./internal/config -run '^TestKomodo' -count=1` — encrypted connection config validation, masking, and credential rotation.
- `rtk go test ./internal/monitoring -run '^TestKomodoPoller' -count=1` — org isolation and successful-empty, failed, stale, disabled, removal, and supplemental snapshot cases.
- `rtk go test ./internal/monitoring -count=1` — full provider regression, 4,083 tests passed.
- `rtk go test ./internal/api -run '^(TestBuildConnections|TestDeriveConnectionState|TestConnectionStaleThreshold|TestKomodo|TestBuildKomodoConnectionMapsPollStatus)' -count=1` — 55 connection aggregation and Komodo handler/status tests, including denied action authority and failed rotation preservation.
- `rtk npm run type-check` and focused Komodo editor/onboarding Vitest run — passed; editor and source selection remain read-only.
- `rtk npm run lint` and `rtk npm run lint:planning-docs` — passed, including repository frontend governance and planning status audit.
- Canonical action/mutation governance Go tests — 15 passed; `rtk go test ./internal/repoctl -count=1` — 31 passed.
- `rtk python3 scripts/check_workflow_trust.py .github/workflows`, `rtk python3 scripts/tests/test_workflow_trust.py` — 44 tests passed, `rtk python3 scripts/repo-boundary-regression.py`, and `rtk bash scripts/audit-private-boundary.sh --enforce` — passed with zero new private-boundary findings.
- `rtk git diff --check` — passed.

Live Core compatibility, production settings, deployment, and action authority are outside this checkpoint.

### Phase 2B validation results

- `rtk go test ./internal/komodo -count=1` — 30 tests passed, including canonical choice/source preservation and cross-provider SQLite restart, polling order, relationship rewrites, stale/empty-snapshot removal and recovery, same-name ambiguity, transitive explicit-link conflict behavior, and organization isolation.
- `rtk go test ./internal/unifiedresources -count=1` — 3,036 tests passed; the availability-link regression confirms an explicit Komodo association follows the operator fold to the Proxmox VM. Metrics tests cover canonical Proxmox history ownership and fail-closed handling for ambiguous Proxmox targets.
- `rtk go test ./internal/api/resourceapi -count=1` — 205 tests passed.
- `rtk go test ./internal/monitoring -count=1` — 4,094 tests passed, including the existing Proxmox, TrueNAS, and supplemental Komodo polling paths.
- `rtk go vet ./internal/unifiedresources ./internal/komodo ./internal/api/resourceapi` — passed with no issues.
- `rtk npm run type-check` and `rtk npm run lint` in `frontend-modern` — passed; lint includes the planning-doc status audit.
- `rtk python3 scripts/check_workflow_trust.py .github/workflows`, `rtk python3 scripts/repo-boundary-regression.py`, and `rtk bash scripts/audit-private-boundary.sh --enforce` — passed; no new private-boundary findings.
- `rtk git diff --check` — passed.

The broader `rtk go test ./...` attempt returned exit 1 after reporting 4,335 passing tests across 51 packages. RTK's captured output was capped at 1 MiB and ended before the failure diagnostic, so that run is inconclusive; the affected packages are run separately above.

No live Komodo Core was available, so these results establish fixture and registry compatibility only; they do not claim compatibility with an operator's installed Core release.

## References

- Komodo [API and clients](https://komo.do/docs/ecosystem/api), [permissioning](https://komo.do/docs/configuration/permissioning), [live API docs](https://demo.komo.do/docs), [ListServers pagination schema](https://docs.rs/komodo_client/latest/komodo_client/api/read/struct.ListServers.html), [Stack parent fields](https://docs.rs/komodo_client/latest/komodo_client/entities/stack/struct.StackListItemInfo.html), [Deployment parent fields](https://docs.rs/komodo_client/latest/komodo_client/entities/deployment/struct.DeploymentListItemInfo.html), [Swarm membership fields](https://docs.rs/komodo_client/latest/komodo_client/entities/swarm/struct.SwarmListItemInfo.html), [v2.3.2 client API reference](https://docs.rs/komodo_client/2.3.2/komodo_client/), and [release list](https://github.com/moghtech/komodo/releases). References inform the protocol shape; support is not pinned to `2.3.3`.
- Pulse resource API: `/api/resources/{id}/link` and `/unlink`; resource graph and supplemental record APIs are owned by `internal/unifiedresources`.
