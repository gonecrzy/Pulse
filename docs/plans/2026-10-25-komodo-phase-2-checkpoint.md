# Pulse + Sindri — Komodo Phase 2 foundation checkpoint

**Status:** Phase 2C relationship foundation and Phase 2D fixture harness are complete on `feature/komodo-phase2-foundation`. The Phase 2E endpoint and owner-only credential file have been used to confirm authenticated Komodo Core `2.3.4` access. Inventory qualification stopped safely because the live `GetPermission` response is the level-only string `"Read"` and does not expose effective specific grants. A test-only, owner-only credential-file fallback and a narrowly scoped operator-attestation gate are implemented; the operator attestation is still pending. Live inventory qualification, gated integration work, and a Phase 2E image remain blocked. This checkpoint is not merged or deployed. Codex-LB remains optional and deferred. Phase 0 read-only, least-privilege, and action-safety contracts remain in force.

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
- The existing operator path is `POST /api/resources/{canonical-id}/link` with `targetId` and optional reason; `/unlink` splits a pair. The API delegates to the org-scoped `ResourceStore`. Its SQLite store persists `resource_links`, and registry generations reapply those decisions when snapshots rebuild. This is an identity merge, not a dependency edge. Phase 2C rejects distinct native IDs from a common provider with HTTP 409 unless the operator sets `confirmProviderConflict: true` and supplies a non-empty reason. Operators should only link when they have verified that both source records identify the same machine.
- Provider parent relationships use `ParentSourceID` and source-specific parent resolution; the registry materializes canonical parent IDs and can expose multiple `ResourceRelationship` edges. Edges carry type, confidence, evidence, discoverer, active state, timestamps, and safe metadata. This checkpoint updates central merge handling so operator identity folds rewrite relationship endpoints and deduplicate edges rather than lose them or leave edges pointing at a folded ID.
- Docker network attachment edges are derived again from Docker-native inventory. Komodo edges are likewise rebuilt from each successful inventory. At the Phase 2B checkpoint, generic dependency or route edges had no independent durable operator store; Phase 2C adds that store and API below. `routes_to` still has no automatic producer or Traefik configuration parser.
- `ResourceRelationshipsWithCanonicalParent` feeds resource API/detail projections. The generic Infrastructure resource detail drawer already has a “Relationship map”. Alert incident synthesis traverses active canonical relationships to build causal/correlated context; relationships do not independently close or suppress alerts. AI chat and handoff code can include generic relationship context, but Komodo-specific Patrol context still needs qualification and explicit product decisions. Polling does not depend on either.

The Phase 2B checkpoint called for cross-provider routing/dependency persistence in this central graph; Phase 2C implements the operator-authored edge store and API below. Reuse the existing operator link for verified resource identity. Do not convert name similarity into identity or treat a dependency edge as an identity merge.

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

### Phase 2B relationship semantics and persistence decision

| Meaning | Registry representation | Persistence and refresh |
|---|---|---|
| `same_as` | Existing operator `ResourceLink`, which folds source identities into one canonical row. It is deliberately not a self-edge in `ResourceRelationship`. | Existing org-scoped `resource_links` table in the SQLite `ResourceStore`; stable until operator unlink/split. |
| `runs_on` / `hosted_by` | Directed `ResourceRelationship` or the same meaning projected from source-specific `ParentSourceID`. | Source-owned edges are emitted from normalized inventory and refreshed with the owning provider's complete snapshot. A failed collection keeps last-known records and edges with stale source status; a successful removal/empty snapshot removes the provider's omitted resources and edges. |
| `routes_to` | New typed `RelRoutesTo` value on the existing edge model. | At Phase 2B, no producer or operator route API existed. Phase 2C adds the shared operator edge API; Traefik parsing and route editing remain deferred. |
| `depends_on` | Existing typed `RelDependsOn` value on the existing edge model. | At Phase 2B, no durable operator dependency API existed. Phase 2C stores these decisions separately from `resource_links`. |

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
| Durable dependencies and routes | Phase 2C adds org-scoped storage/API for operator-authored `depends_on`, `runs_on`, `hosted_by`, and `routes_to` edges, with actor/reason and soft-removal audit. | Add an editing UI, decide tombstone retention and any stronger verification/expiry policy, and add parsers only for explicitly supported sources such as Traefik if later approved. |

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
5. **Relationship audit and validation policy:** Phase 2C stores actor/reason, timestamps, and soft-removal tombstones. Decide whether tombstones need bounded retention or a stronger audit sink, and whether API creation should require currently present endpoints (today it accepts canonical IDs so operator intent can survive temporarily missing resources).
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

### Phase 2C follow-up decisions

1. **Canonical continuity while a selected primary is absent:** links reapply when both provider identities return. While the selected primary is absent entirely, the survivor remains visible under its own ID rather than inheriting a synthetic ID/type. Decide whether a durable canonical anchor and its source-history coordinates are required before enabling operator links on ephemeral provider identities.
2. **Stale edge interpretation:** Phase 2C adds provider evidence source/freshness to discovered edges. Alert synthesis still traverses active relationships without a freshness policy; decide how stale evidence may affect causal or notification decisions before enabling those consumers.
3. **Operator dependency edge policy:** Phase 2C persists operator `depends_on`, `runs_on`, `hosted_by`, and `routes_to` edges in the org resource store, with actor/reason, timestamps, idempotent upsert, and soft-removal tombstones. Retention duration and any stronger audit sink remain open. Missing endpoints keep their persisted edge but only render when the source exists.
4. **Route semantics:** edge direction is source to target and is persisted. What a Traefik route targets, how route evidence is verified/refreshed, and any alert/AI interpretation remain future decisions; no configuration parser was added.
5. **Proxmox non-guest ownership:** the Komodo-primary metrics fix intentionally maps linked Proxmox guest facets only. Decide whether Komodo Server links to a Proxmox node/agent should expose a node or agent history target and how to represent a resource with multiple metric owners.
6. **Incremental availability ordering:** full registry rebuilds process availability after infrastructure sources. A check processed before its referenced provider appears stays unresolved until its next poll; decide whether incremental provider publication should trigger a re-correlation pass.
7. **Previously stored conflicting links:** new link requests are checked against currently observed provider-native identities and return HTTP 409 when a proposed component contains distinct IDs from one provider, unless the operator explicitly confirms that collision and supplies a reason. The check also rejects either missing endpoint because its compatibility cannot be validated. Existing persisted same-provider link chains are not rewritten automatically; operators must review and unlink mistaken decisions. The Proxmox-primary history-target logic still fails closed when ownership is ambiguous.

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

The Phase 2B `rtk go test ./...` attempt returned exit 1 after reporting 4,335 passing tests across 51 packages. RTK's captured output was capped at 1 MiB and ended before the failure diagnostic, so that historical run was inconclusive. Phase 2C captured a full JSON run and diagnosed its failures below.

No live Komodo Core was available, so these results establish fixture and registry compatibility only; they do not claim compatibility with an operator's installed Core release.

## Phase 2C relationship hardening and live qualification preparation

### Identity-link findings and contract

- New link requests resolve both endpoints in the current organization registry and inspect each endpoint's full existing explicit-link component. If folding the proposed pair would place more than one distinct native ID from any shared provider in a component, the API returns HTTP 409 with the provider and sorted native IDs. This prevents an accidental second Proxmox VM or Komodo Server from joining an existing cross-provider identity.
- Intentional same-provider folds remain possible only when the request includes `confirmProviderConflict: true` and a non-empty `reason`. That reason is saved in the existing `ResourceLink` audit field. Cross-provider Proxmox VM ↔ Komodo Server links do not need the override. Names are never considered by this validation.
- The identity check relies on provider-native source mappings. Komodo IDs are opaque and private to the in-memory unified record; the resource API therefore replays Komodo supplemental records when rebuilding from a serialized unified seed, restoring those mappings before validating links. Proxmox retains its source ID in the Proxmox facet. No Komodo secret/configuration fields are copied into the source identity map.
- Existing persisted link chains are not automatically rewritten. An operator should inspect and unlink any legacy chain whose provider component already contains distinct native IDs. If a linked provider is temporarily absent, its link and operator relationship remain in the store; the visible survivor uses its own ID until both linked records are observed again. No synthetic canonical anchor is introduced.

### Operator relationship model and API

Operator-authored directed edges use the optional `OperatorRelationshipStore` capability on Pulse's existing org-scoped `ResourceStore`. SQLite stores rows in `operator_resource_relationships` in the same per-organization resource database. The stable edge ID is a deterministic digest of `(type, canonical source ID, canonical target ID)`. Upsert is idempotent; removal stores a tombstone with actor and time; active-list reads exclude removed rows. Provider polling never deletes these rows. `same_as` is rejected here and remains exclusively a `ResourceLink` identity decision.

| Request | Contract |
|---|---|
| `GET /api/resources/{sourceId}/relationships` | Returns active operator-authored edges whose source is `sourceId`. |
| `POST /api/resources/{sourceId}/relationships` | JSON `{ "targetId": "…", "type": "depends_on", "reason": "…" }`; creates or updates `depends_on`, `runs_on`, `hosted_by`, or `routes_to`; responds `201` with the stored edge. |
| `DELETE /api/resources/{sourceId}/relationships` | JSON `{ "targetId": "…", "type": "depends_on" }`; records a soft-removal tombstone and responds `200`. |

The API rejects self-edges (including endpoints that resolve to the same linked identity), missing IDs, unknown relationship types, and unknown JSON fields. It accepts canonical IDs for endpoints that are temporarily not present, allowing an operator's relationship to survive provider removal or an unavailable poll. A stored edge is projected only when its source resource is present; a target may remain unresolved until it returns. When either endpoint is folded under another identity, GET rewrites the response to the current canonical row, POST updates the existing stored edge rather than creating a second identity, and DELETE retires the matching stored row. A resulting self-edge is suppressed without modifying stored operator intent. POST and DELETE return HTTP 409 when multiple stored rows resolve to the same canonical edge. Availability `checks` and multiple `depends_on` edges are additive and are not replaced by this projection.

Provider-discovered relationships carry `evidenceSource` and `evidenceFreshness` (`current`, `stale`, or unset/unknown). Registry stale transitions update evidence freshness while preserving the last-known edge. Operator-authored edges are independent of provider freshness and remain current until removed. This is provenance, not a new alert policy; alert synthesis still needs an explicit rule before stale topology can alter alert/notification outcomes.

### Phase 2C file-by-file implementation report

| File | Change |
|---|---|
| `internal/unifiedresources/operator_relationships.go` | Add directed-edge allowlist, stable IDs, store projection through identity folds, provider-evidence freshness, and same-provider identity collision validation with clear conflict types. |
| `internal/unifiedresources/operator_relationship_store.go` | Implement SQLite and in-memory upsert/list/soft-removal capabilities over existing stores. |
| `internal/unifiedresources/store.go` | Add org-store SQLite table/indexes and initialize the MemoryStore edge map. |
| `internal/unifiedresources/registry.go` | Load/carry operator rows across registry generations, project relationships into list/get reads, refresh discovered evidence freshness, and retain provider-native IDs across folds and detached snapshots. |
| `internal/unifiedresources/types.go`, `internal/unifiedresources/clone.go` | Preserve private provider-native source ID mappings on detached in-memory resources without adding them to resource JSON. |
| `internal/unifiedresources/relationships.go` | Add relationship evidence source/freshness and safe operator reason/actor projection fields; annotate canonical-parent evidence. |
| `internal/api/resourceapi/resources.go` | Add relationship GET/POST/DELETE dispatch and handler; resolve folded endpoints through the registry for reads and idempotent writes; return HTTP 409 for ambiguous edges, unacknowledged provider collisions, or missing identity-link endpoints; require an explicit reasoned override for intentional same-provider linking; replay Komodo supplemental identities during seed rebuilds. |
| `internal/api/resourceapi/operator_relationships_test.go` | Cover API edge lifecycle/projection, cross-provider canonical endpoint rewrites on GET/POST/DELETE, source-ID rehydration, conflict response, and explicit same-provider confirmation. |
| `internal/api/resourceapi/resources_report_merge_test.go` | Preserve a previously deliberate same-provider agent link by making its test send the new explicit confirmation and audit reason. |
| `internal/api/route_inventory_test.go` | Restore the three Komodo connection routes omitted from the preceding checkpoint's route inventory allowlists. |
| `internal/unifiedresources/operator_relationships_test.go` | Cover SQLite restart, organization isolation, removal audit, rebuilds, folded endpoint rewriting, stale/current discovery, endpoint disappearance/recovery, availability coexistence, and identity collisions. |
| `internal/unifiedresources/registry_materialization_test.go`, `internal/unifiedresources/registry_materialization_integration_test.go` | Keep the independent metadata oracle aligned with the list read's relationship-freshness projection and report field-level mismatches. |
| `internal/unifiedresources/code_standards_test.go` | Update the source-contract assertion for unconditional Komodo identity rehydration from serialized resource seeds. |
| `frontend-modern/src/types/resource.ts` | Type the optional relationship provenance, freshness, reason, actor, and stable ID fields. |
| `docs/plans/2026-10-23-sindri-integration-roadmap.md` | Record Phase 2C's completed relationship contract, collision confirmation, and live qualification gate. |
| `docs/plans/2026-10-25-komodo-phase-2-checkpoint.md` | Record the identity audit, persistence/API contract, implementation report, exact test outcome, and operator live-test inputs. |

### Live Komodo qualification requirements

There was no authorized live Komodo endpoint or approved existing credential available in this workspace; no live request was made. An operator must supply or select the base URL and an existing read-only API key/secret, then provide the observed `GetVersion` result and sanitized sample IDs/responses for Server, Stack, Deployment, and Swarm. Confirm that the principal has only the required inventory `Read` permissions, that IDs remain stable across polls, and whether the actual Stack/Deployment records contain `server_id`, `swarm_id`, both, or neither and what Swarm `server_ids` returns. Verify list visibility under the real permission model. Do not send credentials or raw environment/configuration values, and do not create a privileged key for qualification.

The historical metrics, connection-alert conversion, notification routing, broad monitoring UI, and Patrol integration remain deferred. No action capability or AI dependency was added.

### Phase 2C verification and complete Go-suite diagnosis

`rtk proxy go test -json ./... > /tmp/pulse-phase2c-final2-go-test.jsonl 2>&1` completed in 8m37.68s with exit 1. Its complete stream contains 160 package summaries: 153 passed and 7 failed, with 52,489 test-pass events and 25 test-fail events (Go's JSON stream includes both subtest and parent events). The complete captured stream is available at that path in the implementation workspace. `TestRouterRouteInventory` passed; the missing Komodo routes found during Phase 2C's initial review were added to the route inventory allowlists.

The remaining failures are unrelated environment-sensitive fixtures; no failures were skipped or shimmed:

| Package and failing test(s) | Exact failure and cause |
|---|---|
| `internal/cloudcp/docker` — `TestPrepareTenantRuntimeMountSourcesAlignsOwnershipAndPermissions` | `manager_test.go:473`: temp directory uid was `1000`, expected `0`. The test runs as root and passes `uid=0` into a manager config, where zero is treated as “unset” and deliberately falls back to tenant runtime uid `1000`; the later assertion still expects root ownership. |
| `internal/hostmetrics` — `TestCollectSplitsReclaimableCache`; `TestCollectDerivesMemoryPressureFromAvailableMemory` | Live host memory-derived `UsedBytes` differed from fixture constants by 13,440 bytes: `6442437504` vs `6442450944`, and `8589921152` vs `8589934592`. |
| `internal/sensors` — `TestCollectLocalSensorsOutput`; `TestCollectLocalSensorsOutputWithNonZeroExit`; `TestCollectLocalAcceptsNonZeroWithOutput` | Fixture output included the host's real `cpu_thermal-virtual-0` readings (`56`–`57°C`) in addition to the mocked sensor values; these tests are not isolated from host hardware. |
| `internal/config` — `TestConfigurationMigrationArchiveScope`; `TestLoadAIFindings_UndecryptableFilePreserved`; `TestLoadAIChatSessions_UndecryptableFilePreserved` | Under root, the legacy system encryption key is readable and migrates into temporary config directories. That makes supposedly fresh keys equal and lets the encrypted fixtures decrypt, so the tests' expected key separation and preserved-undecipherable copies are not observed. |
| `internal/alerts` — `TestLoadHistory_PermissionError` | The test expected an unreadable backup to fail, but root can read the file despite its permission bits. |
| `internal/api` — `TestTelemetryUpdate_NoMutationOnPersistFailure` | `system_settings_telemetry_test.go:400`: expected HTTP 500 after a write-permission failure, but the root-run process persisted successfully and returned `200 {"success":true}`. |
| `scripts/installtests` — `TestInstallSHUninstallRetainsCredentialsWhenSavedURLIsMissing`; `TestNativeInstallerFixtureToolsAreRealAndStepLocal` and its 12 subcases | The refusal proof requires the ordinary unprivileged proof user, but this run is `uid=0`; the fixture reports `root-refusal control requires the ordinary proof user`. Each native-tool subcase also fails because `gawk` is not installed (`exec: "gawk": executable file not found in $PATH`). |

The earlier `TestRouterRouteInventory` failure came from its repository test allowlists not being updated for three already-registered Komodo connection routes. Updating both allowlists fixes that checkpoint governance/test gap; the final route inventory test passes, and no runtime route defect was found. The filesystem/host/tool failures above should be rerun in the repository's expected unprivileged test image with GNU `gawk` and isolated fixtures before treating `go test ./...` as green.

Additional Phase 2C checks:

- `rtk proxy go test ./internal/unifiedresources -count=1` — passed (full package; 43.9s).
- `rtk proxy go test ./internal/api/resourceapi -count=1` — passed (full package; 68.2s).
- Focused identity/relationship/API selection across `internal/unifiedresources`, `internal/api/resourceapi`, and `internal/api` — passed, including same-provider conflict/confirmation, source-ID rehydration, route inventory, dependency persistence, stale evidence, and check-edge coexistence.
- `rtk go vet ./internal/unifiedresources ./internal/api/resourceapi ./internal/api` — passed.
- `rtk npm run type-check`, `rtk npm run lint`, and the fresh `rtk npm run lint:planning-docs` in `frontend-modern` — passed.
- `rtk python3 scripts/repo-boundary-regression.py`, `rtk bash scripts/audit-private-boundary.sh --enforce`, `rtk python3 scripts/check_workflow_trust.py .github/workflows`, `rtk python3 scripts/tests/test_workflow_trust.py`, and the final `rtk git diff --check` — passed.

## References

- Komodo [API and clients](https://komo.do/docs/ecosystem/api), [permissioning](https://komo.do/docs/configuration/permissioning), [live API docs](https://demo.komo.do/docs), [ListServers pagination schema](https://docs.rs/komodo_client/latest/komodo_client/api/read/struct.ListServers.html), [Stack parent fields](https://docs.rs/komodo_client/latest/komodo_client/entities/stack/struct.StackListItemInfo.html), [Deployment parent fields](https://docs.rs/komodo_client/latest/komodo_client/entities/deployment/struct.DeploymentListItemInfo.html), [Swarm membership fields](https://docs.rs/komodo_client/latest/komodo_client/entities/swarm/struct.SwarmListItemInfo.html), [v2.3.2 client API reference](https://docs.rs/komodo_client/2.3.2/komodo_client/), and [release list](https://github.com/moghtech/komodo/releases). References inform the protocol shape; support is not pinned to `2.3.3`.
- Pulse resource API: `/api/resources/{id}/link` and `/unlink`; resource graph and supplemental record APIs are owned by `internal/unifiedresources`.

## Phase 2D — Komodo live qualification preparation

### Environment result

No Komodo environment variables were configured, `PULSE_DATA_DIR` was unset, and the default protected connection file `/etc/pulse/komodo.enc` was absent. No configuration was decrypted, no credential value was printed or exported, and no network request was attempted. Live compatibility, the installed Core version, effective permissions, and actual resource identifiers/parents remain unverified.

### API compatibility matrix

The following is the contract implemented and covered by local fixtures, checked against Komodo's published client/schema references. It is not a live-server result. The documentation available during this checkpoint identifies `komodo_client` 2.3.2; the adapter continues to query `GetVersion` at runtime and accepts a syntactically valid Core v2 version rather than pinning a patch release.

| Surface | Implemented contract | Fixture result / live status |
|---|---|---|
| Transport and auth | HTTPS origin only, default certificate verification, TLS 1.2 minimum, redirects disabled. `POST /read/{method}` with `X-Api-Key` and `X-Api-Secret`; only `GetVersion`, `ListPermissions`, `GetPermission`, `ListServers`, `ListStacks`, `ListDeployments`, and `ListSwarms` can be called. | Fixture checks request method/path/headers and the allowlist; invalid auth produces a generic error without echoing the server response. Live unavailable. |
| Core version | `GetVersion` must return a semantic version with major `2`; inventory is not requested for a different or malformed major version. | Fixture accepts `2.3.4` and rejects `1.9.0` before listing resources. No installed version observed. |
| Permission/scope | Direct grants must be exact `Read` with no specific permissions. For a non-empty kind, the client calls effective `GetPermission` for one visible resource; empty kinds require a direct type-wide `Read` grant whose target ID is `all`. | Fixtures cover missing, elevated, specific, and type-wide empty grants. This does not fully attest group-inherited or global user capabilities: Komodo documents that `ListPermissions` omits UserGroup grants, while `GetPermission` includes them for a specific resource. The current client samples one visible item per populated kind. The operator must additionally confirm the selected principal is non-admin, has no create-server/create-build permission, and receives no group grant above read. Live unavailable. |
| List and pagination | `ListServers`, `ListStacks`, `ListDeployments`, `ListSwarms`; request carries `query`, zero-based `page`, and bounded `limit`. Pagination is capped at 1,000 pages, each response at 8 MiB. Duplicate or missing IDs fail the collection. | Fixtures exercise pagination on all four kinds, cross-page duplicate detection, malformed IDs, transient errors, timeouts, and response redaction. Live unavailable. |
| Resource fields | List item identity/name are `id`/`name`; state is read from `info.state`. Stack and Deployment parents are `info.server_id` or `info.swarm_id`; Komodo's schema says `swarm_id` takes precedence when both are populated. Swarm membership is `info.server_ids`. | Normalization fixtures verify each parent and membership shape, precedence, stable source identities, redaction, and registry ingestion. Live field presence and values unavailable. |
| Empty/failure/recovery | A successful, permission-verified empty inventory is distinct from a failed collection. Poll failure retains stale last-known records; success recovers freshness; intentional disablement removes published records. | Client and poller fixtures cover these transitions. No live empty inventory, failure, or recovery was induced. |
| Identity and dependency | Identity is `(Pulse connection UUID, Komodo kind, Komodo native ID)` and never inferred from names. Cross-provider identity requires an explicit Pulse operator link. Durable directed dependencies remain in the existing org ResourceStore. | Registry, link, and relationship tests cover identity stability, canonical link persistence, dependency persistence, and endpoint rewrite. No live Proxmox/Komodo pair was available. |
| Authority and AI | The Komodo client exposes read methods only. Polling and canonical monitoring do not call Sindri AI; no write, execute, terminal, mutation, action runner, or remediation capability is registered. | Client allowlist and connection-action-authority tests pass; polling is independent of AI state. Live server-side credentials still require operator permission verification. |

Schema references checked: Komodo [API and clients](https://komo.do/docs/ecosystem/api), [permissioning](https://komo.do/docs/configuration/permissioning), the published [Core API client contract](https://docs.rs/komodo_client/2.3.2/komodo_client/api/index.html), [ListPermissions semantics](https://docs.rs/komodo_client/2.3.2/komodo_client/api/read/struct.ListPermissions.html), [effective GetPermission semantics](https://docs.rs/komodo_client/2.3.2/komodo_client/api/read/struct.GetPermission.html), [Stack parent fields and precedence](https://github.com/moghtech/komodo/blob/main/client/core/rs/src/entities/stack.rs), [Deployment parent fields and precedence](https://github.com/moghtech/komodo/blob/main/client/core/rs/src/entities/deployment.rs), and [Swarm membership field](https://docs.rs/komodo_client/2.3.2/komodo_client/entities/swarm/struct.SwarmListItemInfo.html). Komodo's client documents `ListPermissions` as direct permissions only (excluding UserGroups); `GetPermission` accounts for UserGroup permissions for its specified resource. The current API does not establish that a principal lacks global create rights, so credential qualification must inspect those settings as an operator.

### Reproducible qualification harness and operator procedure

`internal/komodo/live_qualification_test.go` adds an opt-in read-only test. It prefers an explicitly selected enabled connection from the encrypted Pulse store when `PULSE_DATA_DIR` and the Pulse connection UUID are supplied. Otherwise it can read a test-only JSON credential file when the endpoint and file path are explicitly supplied. The file loader exists only in `_test.go`, requires `PULSE_KOMODO_LIVE_QUALIFICATION=1`, rejects symlinks and non-regular files, requires a file owned by the test operator with no group/other access, caps size at 4 KiB, rejects unknown or duplicate fields, and emits only generic errors. It does not write credentials. File-backed identity checks use a stable endpoint-derived test UUID; no records are persisted. Both modes perform two bounded `CollectInventory` observations and report Core version, verified scope count, per-kind inventory counts, count of resources with a reported state, parent-field counts, unresolved parent references, identity count, and whether the identity sets match. Neither mode prints endpoint, connection ID, resource names/IDs, API key, API secret, or state values. With opt-in unset it skips, so ordinary fixture tests never contact a server.

Operator steps once an approved existing Core is available:

1. Use an isolated Pulse organization and an existing Komodo non-admin/service principal. Through Komodo's UI, confirm it is enabled, has only the inventory `Read` grants, has no specific permissions, no admin/create-server/create-build capability, and no UserGroup membership that raises access. Use only its already-approved API key/secret; do not create a privileged credential for this test.
2. Prefer Pulse's encrypted connection store when an isolated Pulse test organization is available: save the HTTPS Core origin and existing approved key/secret in Infrastructure settings, run its connection test, and record the non-secret Pulse connection UUID. For initial qualification without an existing Pulse connection, create a JSON file using a local secure editor or secret manager, outside the repository and shell history. Its shape is `{"apiKey":"<approved-key>","apiSecret":"<approved-secret>"}`. The file must be owned by the user running the test and have mode `0600` or stricter. Do not place credentials in command arguments or environment variables.
3. Run from the repository as the normal unprivileged test operator. For the preferred encrypted-connection path, supply the isolated `PULSE_DATA_DIR`, opt-in flag, and Pulse connection UUID. For file mode, explicitly unset those connection selectors and supply only the non-secret endpoint and credential-file path:

   ```sh
   rtk env -u PULSE_DATA_DIR -u PULSE_KOMODO_QUALIFICATION_CONNECTION_ID \
   PULSE_KOMODO_LIVE_QUALIFICATION=1 \
   PULSE_KOMODO_QUALIFICATION_ENDPOINT=https://komodo.home.gonecrzy.com \
   PULSE_KOMODO_QUALIFICATION_CREDENTIAL_FILE=/secure/test-only/komodo-qualification.json \
   go test ./internal/komodo -run '^TestLiveCoreV2Qualification$' -count=1 -v
   ```

   The test reads credentials in process memory and sends them only to the validated HTTPS endpoint through the existing read-only client. TLS verification and the API allowlist are unchanged. A changing identity set between the two reads fails without printing IDs; rerun during a stable inventory window.
4. Review the reported version, all four verified scopes, counts, parent-field counts, unresolved parent references, and stable-set result. A zero count is a valid empty inventory only when all four scope checks pass. If any parent references are unresolved, compare sanitized field presence/values in Komodo's UI against the Pulse record without copying configuration/environment data.
5. In that isolated Pulse organization, choose one verified Proxmox VM and its matching Komodo Server from inventory based on operator knowledge, explicitly link the pair in Pulse, add a directed dependency between the canonical resource IDs, and verify the link/edge after multiple poll cycles and a Pulse restart. Use an explicitly selected non-production pair; do not infer identity from equal names. Do not intentionally interrupt a production Core to test failure recovery: mock fixtures already cover unsuccessful collection, stale retention, and recovery.

Fixture-only qualification remains reproducible with `go test ./internal/komodo ./internal/monitoring -run 'Test(Client|Normalize|ProxmoxKomodoLink|SameNamedKomodo|ConflictingProxmox|KomodoPoller)' -count=1`. It covers bad authentication, version rejection, read scope, pagination, duplicate IDs, timeout/transient errors, safe error handling, normalized fields, cross-provider linking, empty inventory, stale retention, recovery, disablement, and resource removal.

### Previously failing package reproduction

The seven packages previously identified were rerun under UID 65534 (`nobody`) with Go 1.26.10, a private writable Go module/build cache, and GNU `gawk` 5.2.1 installed. Source was copied to `/tmp` without `.git` to keep the test tree accessible to the unprivileged user; package runtimes overlap in `go test`, and the longest package took 520.561s.

| Package | Result | Finding |
|---|---|---|
| `internal/cloudcp/docker` | Pass (0.017s) | Root-only UID-zero fallback failure is absent for a normal unprivileged UID. |
| `internal/config` | Pass (28.740s) | The undecryptable/migration assertions pass without root's readable legacy key. |
| `internal/alerts` | Pass (46.321s) | Permission-denied fixture behaves as intended without root read bypass. |
| `scripts/installtests` | Pass (520.561s) | The unprivileged refusal proof and native tool fixtures pass with GNU `gawk`. |
| `internal/hostmetrics` | Fail (0.305s): `TestCollectSplitsReclaimableCache` got `UsedBytes=6442437504`, wanted `6442450944`; `TestCollectDerivesMemoryPressureFromAvailableMemory` got `8589921152`, wanted `8589934592`. | The `virtualMemory` test stub does not override `readARCSize`; this host exposes `/proc/spl/kstat/zfs/arcstats`, and the live 13,440-byte ARC value is subtracted from both fixture expectations. Environmental fixture leakage, not a Phase 2 regression. |
| `internal/sensors` | Fail (0.174s): the three `TestCollectLocal...` assertions received `cpu_thermal-virtual-0` with 52–53°C alongside the mocked `chip` values. | These three tests replace the `sensors` executable but do not replace the default host sysfs roots. Host sensor fallback adds a real device. Environmental fixture leakage, not a Phase 2 regression. |
| `internal/api` | Initial package run failed only `TestServerInfoEndpointReportsDevelopment`. | The temporary source copy omitted `.git`, so version detection used the non-development fallback. After seeding minimal local Git metadata in the temporary copy, `go test -run '^TestServerInfoEndpointReportsDevelopment$' -count=1 ./internal/api` passed (2.274s). This was a reproduction-harness artifact, not an API regression. The earlier root-only telemetry persistence failure is likewise not present in the unprivileged run. |

No failure was ignored or rewritten. The hostmetrics and sensors fixture-isolation gaps remain outside this Komodo checkpoint; the full seven-package matrix is not green until those fixtures stop observing live host state. The prior root-only failures in Docker ownership, config encryption migration, alerts file permissions, API write-denial, installer root refusal, and missing `gawk` are resolved by running the expected unprivileged/dependency-complete test environment.

### Smallest next integration plan (gated on successful live qualification)

1. **Connection health and alerts:** feed `KomodoConnectionPollStatus` already projected by `internal/api/connections_aggregator.go` into the existing `alerts.ConnectionSnapshot`/`CheckConnection` boundary in `internal/alerts/connection.go`. Decide activation grace, stale thresholds, and org policy mapping; do not add an alert manager.
2. **Canonical resource health/incidents:** map only validated Komodo state and stale-source evidence into existing unified-resource health and incident synthesis. Keep connection failure distinct from a workload's reported state. Do not add Komodo metric/history series until a separate live metrics contract is approved.
3. **Notifications:** route the existing canonical connection/resource alerts through `internal/notifications.NotificationManager` and its existing queue/rules. Decide event type/severity and recovery routing; add no delivery engine.
4. **Monitoring UI:** extend the existing Infrastructure settings connection status and current unified-resource surfaces (`ConnectionEditor` and resource API types/views) with sanitized poll/scope status and validated parent evidence. No topology redesign or Traefik parser is included.
5. **Patrol context:** only after deterministic resource/relationship contracts are live-qualified, add a read-only canonical resource snapshot to the existing Patrol scope/context builder in `internal/api/router.go` / `internal/ai/patrol_*`. Include freshness/provenance and safe resource fields; exclude Komodo configuration, API credentials, raw responses, and execution tools. Polling, alert evaluation, and notifications remain operational when Sindri AI is unavailable.

No connection-alert, resource-alert, notification, monitoring UI, Patrol, metrics, or live Komodo behavior was expanded in Phase 2D. Review this checkpoint and the qualification procedure before continuing.

## Phase 2E — live integration attempt and current blocker

### Live access discovery

The current Codex workspace is on `feature/komodo-phase2-foundation` at the Phase 2D commit. No environment variable names for Komodo or `PULSE_DATA_DIR` are set. The default protected connection file `/etc/pulse/komodo.enc` and candidate `/var/lib/pulse/komodo.enc` are absent; no `komodo.enc` was found in `/run/secrets`, `/root/.config/pulse`, `/srv/pulse`, or the repository. `/etc/hosts` has no Komodo alias, and no running Docker container exposes an existing Pulse/Komodo connection. No credential file was opened or decrypted, and no API request or guessed-host network request was made.

Therefore the assertion that Komodo Core is accessible from the Codex environment could not be tied to a specific authorized endpoint or approved read-only credential in this checkout. **No live compatibility result is claimed.** The installed Core version, effective server-side permissions, actual resource fields/parents, pagination behavior, and live resource identity stability remain unverified. Do not continue with Komodo alert/incident integration or label an image as Phase 2E until the operator makes an existing approved connection available through the protected Pulse store and supplies its non-secret connection UUID and data-directory path.

### Existing integration audit

| Surface | Present foundation | Remaining Phase 2E work after live qualification |
|---|---|---|
| Polling and registry | The org-scoped Komodo poller publishes safe Server/Stack/Deployment/Swarm records through the supplemental provider boundary. Stable identities, stale/empty/disable semantics, explicit Proxmox links, parent edges, and operator dependency persistence are fixture-covered. | Verify the live contract and real source data. The concrete Jellyfin/Traefik chain cannot be asserted from fixture names. |
| Connection health and alerts | `buildKomodoConnection` maps poll state into the existing connection row and current Infrastructure UI. | `internal/alerts.ConnectionType` and `internal/api` connection-alert conversion do not yet include Komodo, so Komodo connection failure/recovery is not in the existing alert lifecycle. Add it there after scope and activation policy are validated. |
| Resource health and notifications | Unified records preserve sanitized reported status; the existing `NotificationManager` supports canonical alert/recovery delivery. | Komodo resource health has no validated canonical incident mapping or producer-to-NotificationManager route. Decide state semantics and recovery routing using live evidence, then reuse these existing systems. |
| Monitoring UI | Komodo connection create/test/status surfaces are present in Infrastructure settings. | Confirm live status/scope behavior and expose only validated resource health/freshness in the existing unified-resource surface. No topology redesign is needed. |
| Patrol | The router already injects the generic unified-resource provider; Komodo records can enter that registry. Polling and canonical monitoring do not depend on Sindri AI. | No Komodo-specific Patrol freshness/provenance qualification has been done. Verify safe canonical fields and read-only scope using live records before claiming this complete. |
| Relationships | Fixture tests cover Proxmox/Komodo explicit identity folding, persistent edges, availability-check associations, and endpoint rewrites. | No live Proxmox VM, Komodo Server, Docker host/container, Traefik route, or availability target pair was available here. Add only operator-confirmed identity/dependency edges; omit `routes_to` unless the operator supplies reliable route evidence. |

### Phase 2E focused validation

The following Go tests ran as UID 65534 (`nobody`) from an isolated source archive, with private writable Go build/module caches:

| Area | Result |
|---|---|
| `internal/komodo` | Package tests passed, including auth/version/pagination, normalization, explicit Proxmox linking, and safe-error cases. |
| `internal/monitoring` | Komodo poller org isolation, empty/failure/stale/recovery/disable/removal test passed. |
| `internal/unifiedresources` | Operator relationship persistence/isolation/rebuild and availability association, multiple-service, and Komodo-to-Proxmox-link tests passed. |
| `internal/alerts` | Existing connection lifecycle/degraded/recovery tests passed. These do not yet exercise Komodo because its connection type is not mapped. |
| `internal/notifications` | Existing recovery and destination-routing contract tests passed; no Komodo-specific producer is wired yet. |
| `internal/api` | `TestBuildKomodoConnectionMapsPollStatus` and `TestKomodoConnectionRoutesDoNotExposeActionAuthority` passed. The initial archive lacked the git-ignored embedded `internal/api/frontend-modern/dist`; after copying the already-built assets into the isolated source tree, both tests passed. |
| `internal/ai` | Existing canonical resource-scope resolution test passed. It is generic and does not establish a Komodo-specific Patrol context contract. |

The existing Komodo CredentialSlot frontend test passed (1 file, 1 test). `go vet ./internal/komodo ./internal/monitoring ./internal/api` and the planning-doc status audit passed in the Phase 2D/2E workspace. No full `go test ./...` run or Phase 2E Docker image build was performed. The focused Go run tests the existing fixture foundation only; it is not evidence of live compatibility or completed alert/notification/Patrol integration.

The selected Go test names were `TestKomodoPollerScopesOrganizationsAndDistinguishesEmptyFailureStaleDisableAndRemoval`; `TestSQLiteOperatorRelationshipsPersistAndIsolateOrganizations`, `TestOperatorRelationshipRejectsIdentityAndUnsupportedEdges`, `TestOperatorRelationshipsSurviveRebuildsAndRewriteLinkedEndpoints`, `TestOperatorDependenciesCoexistWithAvailabilityCheckRelationship`, `TestDiscoveredRelationshipFreshnessFollowsProviderStatus`, `TestAvailabilityExplicitLinkRetainsCheckAndProjectsFacetToKnownResource`, `TestAvailabilityKeepsEveryConfiguredCheckWithMultipleServicesOnOneHost`, and `TestAvailabilityLinkToKomodoServerFollowsOperatorFoldIntoProxmoxVM`; alert tests `TestCheckConnection`, `TestConnectionDegraded*`, and `TestClearConnectionDegradedAlert`; notification tests `TestOrdinaryNotificationRecoveryUsesDeliveredReceipt`, `TestQueuedNtfyRecoveryAfterProviderOutageAndRestart`, and `TestBuildNotificationDeliveryJobsRoutesAlertsByDestinationTags`; API tests `TestBuildKomodoConnectionMapsPollStatus` and `TestKomodoConnectionRoutesDoNotExposeActionAuthority`; and AI test `TestResolvePatrolScopeExpandsCanonicalUnifiedIDToSourceResource`. The whole `internal/komodo` package was run. These focused runs were executed as `nobody`; the frontend test ran in the checked-out workspace.

### Image and test-instance safety

No Phase 2E monitoring integration or database migration was applied, and no Phase 2E image was built or pushed. There is no image reference or digest to test. No production runtime environment variables were introduced. The test-only harness uses `PULSE_KOMODO_LIVE_QUALIFICATION`, `PULSE_KOMODO_QUALIFICATION_ENDPOINT`, and `PULSE_KOMODO_QUALIFICATION_CREDENTIAL_FILE`; encrypted-store mode additionally uses `PULSE_DATA_DIR` and the non-secret connection UUID. Do not use the Phase 2D image/source as though it contains Phase 2E integration.

Once the live gate and implementation tests pass, build from the existing production `Dockerfile` with the full source SHA in a dedicated test tag. Keep it local or publish it only under a unique non-stable tag; never overwrite `latest` or the stable tag. Use a copied data directory and a separate test container/Compose project, not the live writable Pulse database. Before testing a schema-bearing revision, create and verify a backup of the copied database; test upgrade, restart, and rollback against that copy. Rollback means stopping the new test container and starting the previous SHA against its pristine pre-upgrade data copy; do not assume a database upgraded by a newer image is backward-compatible. If only the migrated copy remains, restore the verified backup. No schema change was introduced here. Preserve the existing encryption key/config pairing in the copy; do not share it in chat or logs. Keep existing runtime environment/configuration unchanged except for the isolated data-directory path and the opt-in qualification variables from the Phase 2D procedure.

### Operator checklist to unblock live qualification

1. Make the existing approved read-only API key and secret available locally through either an isolated encrypted Pulse connection or an owner-only JSON file outside the repository. Do not send a secret in chat or create a privileged credential.
2. For file mode, set the non-secret credential-file path with `PULSE_KOMODO_QUALIFICATION_CREDENTIAL_FILE` and the supplied HTTPS origin with `PULSE_KOMODO_QUALIFICATION_ENDPOINT`; leave `PULSE_DATA_DIR` and `PULSE_KOMODO_QUALIFICATION_CONNECTION_ID` unset. Confirm the principal is non-admin and has no group-derived permissions above inventory read.
3. Run `TestLiveCoreV2Qualification` from the Phase 2D procedure. Review Core version, all four verified scopes, inventory counts, parent evidence, unresolved selected-parent references, and stable IDs. Capture no credential, raw API response, configuration, or sensitive resource payload in the report.
4. In an isolated Pulse organization, explicitly link the selected Proxmox VM and Komodo Server based on operator knowledge; validate the persisted link and dependency edge across polling, restart, and provider staleness. Separately identify the Docker host, Traefik host, Jellyfin check, and authoritative route evidence before authoring any additional relationships.
5. Only after those checks, implement and test the existing alert, incident, notification, UI, and Patrol integration seams; then build and smoke-test the SHA-tagged image against a copied data directory. Record schema/backup/rollback behavior and the local image reference or immutable digest before operator review.

No production deployment, stable-image update, Komodo mutation, action authority, execution capability, or remediation was performed.

## Phase 2E resumed endpoint qualification — 2026-10-25

The operator supplied the Komodo HTTPS endpoint. DNS resolved it to a private address, TLS 1.3 negotiated with the expected wildcard certificate and certificate verification succeeded. An unauthenticated `POST /read/GetVersion` reached the API and returned HTTP 401. This confirms DNS, HTTPS/TLS, and API route reachability only; it does not verify a Komodo Core version or authenticated API compatibility.

No Komodo-related environment variables or `PULSE_DATA_DIR` are configured in this workspace. The default `/etc/pulse/komodo.enc` is absent (the encryption key file is present but was not read); `/var/lib/pulse/komodo.enc` is also absent. No credential was available to the harness. No secret was requested, displayed, exported, or logged.

The previous protected-store-only instruction is superseded by the test-only file fallback below. The live gate remains blocked because no `PULSE_KOMODO_QUALIFICATION_CREDENTIAL_FILE` is configured in this workspace. The endpoint check does not establish authenticated compatibility.

Until that run succeeds and the operator confirms effective non-admin/read-only permissions, do not claim live compatibility, begin the conditional Phase 2E monitoring integration, or build a Phase 2E test image. No Komodo mutation, deployment, or merge occurred.

## Phase 2E test-only credential file update — 2026-10-25

The qualification harness now prefers an explicitly selected encrypted Pulse connection and offers a test-binary-only fallback for initial qualification. File mode reads a strict JSON object with `apiKey` and `apiSecret` from a regular non-symlink file owned by the test operator and readable only by that owner. It requires the explicit live-test opt-in, caps input size, rejects duplicate/unknown fields and unsafe permissions, and returns generic errors. The credential file path and endpoint use non-secret environment variables; credentials are never accepted through CLI arguments or environment variables. The existing Komodo client still enforces HTTPS, normal certificate verification, bounded transport, and its read-method allowlist. Production configuration storage and authentication were not changed.

The current workspace has no configured credential file path or protected connection selection. No credential contents were read. `go test ./internal/komodo -count=1` passed (43 tests), including file permission, symlink, parser, opt-in, protected-store precedence, and stable test identity coverage. `go vet ./internal/komodo` passed. The live test was not run against the endpoint because no approved local credential file was available. The operator action is to place an already-approved read-only key and secret in the documented owner-only file, set its path and the endpoint for the opted-in test, then provide only non-secret test output for review. Do not start Phase 2E monitoring integration or build an image until live qualification succeeds.

## Phase 2E credential-backed qualification attempt — 2026-10-25

The operator supplied an owner-only qualification credential file. Metadata was checked without displaying or exporting its contents: it is a regular non-symlink file, owned by the test user, with owner-only access. The opt-in live harness loaded the credentials from that file; no credential value was logged, persisted elsewhere, or passed on a command line. TLS verification remained enabled, and the client made only its explicitly allowlisted read calls.

The authenticated `GetVersion` response reported Core `2.3.4`. The response decoded successfully, confirming API-key authentication. `ListPermissions` also decoded and passed the client's direct-grant checks. Collection then stopped safely when `GetPermission` returned a top-level JSON string; the supported Pulse response model expects an object containing `level` and `specific`. The client rejected that mismatch before completing inventory qualification. The diagnostic reports only the allowlisted method, JSON field path, and JSON value kind; it never includes the response body. A unit regression test confirms malformed response material is not echoed.

| Check | Result |
|---|---|
| DNS, HTTPS, TLS validation, and API route | Previously verified; no TLS bypass was used. |
| API-key authentication | Passed: authenticated `GetVersion` returned Core `2.3.4`. |
| Core v2 version gate | Passed: `2.3.4` is accepted by the v2 version check. |
| Direct permission response | Decoded; the client found no direct grant above `Read` or with specific grants before reaching the per-resource check. |
| Effective resource permission | **Blocked:** `GetPermission` returned a JSON string, so effective level and specific permissions could not be verified under the current contract. |
| Complete Server/Stack/Deployment/Swarm inventory, pagination, state, parent fields, stable identities, empty/failure/recovery behavior | **Not qualified:** collection failed closed before it returned an inventory. No resource names, IDs, states, or response payloads were logged. |
| Proxmox identity linking and stored dependencies | Fixture-covered only; no live pair was evaluated. |
| Monitoring alerts, notifications, UI resource status, Patrol, and image | Not started because live qualification did not pass. |

At this checkpoint the `GetPermission` handler, serializer, and effective-permission resolver had not yet been reviewed. That investigation and the follow-up live result are recorded below; the earlier request to supply the endpoint schema is superseded by that source review.

The harness now logs the verified Core version before inventory collection, and client contract errors identify only the allowlisted method and safe JSON shape. `rtk go test ./internal/komodo -run 'TestClientIdentifiesMalformedReadContractWithoutEchoingResponse' -count=1` passed. No mutation, merge, deployment, integration expansion, or image build occurred.

## Phase 2E GetPermission compatibility review — 2026-10-25

The v2.3.4 tagged source was reviewed at the handler, response serialization, and permission resolver layers:

- [`GetPermission::resolve`](https://github.com/moghtech/komodo/blob/v2.3.4/bin/core/src/api/read/permission.rs) returns `get_user_permission_on_target`, whose response alias is `PermissionLevelAndSpecifics` in [`client/core/rs/src/api/read/permission.rs`](https://github.com/moghtech/komodo/blob/v2.3.4/client/core/rs/src/api/read/permission.rs). The alias has `level` and `specific` fields. The API comment still describes `[PermissionLevel]`, so the tagged API description itself is inconsistent.
- The [`/read` handler](https://github.com/moghtech/komodo/blob/v2.3.4/bin/core/src/api/read/mod.rs) resolves the request and returns the generated response wrapper. The wrapper's [`From<T: Serialize>` implementation](https://docs.rs/crate/mogh_error/1.0.4/source/src/axum.rs) serializes the resolved value with `serde_json::to_string` as the JSON response body. The tagged implementation therefore describes an object response.
- The [`permission resolver`](https://github.com/moghtech/komodo/blob/v2.3.4/bin/core/src/permission.rs) combines admin status, resource base permissions, user-wide permissions, all applicable groups, per-resource user/group grants, and inherited target specifics. The group query includes both explicit membership and `everyone: true` groups in [`helpers/query.rs`](https://github.com/moghtech/komodo/blob/v2.3.4/bin/core/src/helpers/query.rs).
- [`ListPermissions`](https://github.com/moghtech/komodo/blob/v2.3.4/bin/core/src/api/read/permission.rs) returns only records directly assigned to the user. `ListUserGroups` only lists groups whose `users` field contains the user and therefore omits Everyone groups; `ListUserTargetPermissions` is admin-only. These APIs cannot reconstruct the complete effective specific-permission set. `GetPermission` is the only non-admin endpoint in the supported read allowlist that resolves that combined scope.

The live `2.3.4` instance nevertheless returned a top-level JSON string for `GetPermission`. Pulse now recognizes both observed wire shapes while keeping the evidence distinct:

| Wire response | Decoder behavior | Qualification result |
|---|---|---|
| `{"level":"Read","specific":[]}` | Accept only with exactly the required non-null fields and an array of strings; duplicate or extra keys are rejected. | Verifies Read with no specific permission for that target. |
| `"Read"` | Recognize as the observed level-only response. | Does not establish that specific grants are empty; collection fails closed and requests operator verification. |
| `None`, `Execute`, `Write`, unknown level, malformed object, null/missing specifics, or non-empty specifics | Reject. | Cannot pass read-only scope validation. |

Scope validation now checks each returned resource because Komodo can grant different effective permission levels and specifics per target. Errors and qualification output contain no response body, native IDs, names, or credentials.

### Requalification result

The opt-in harness was rerun using the provided owner-only test credential file and `https://komodo.home.gonecrzy.com`. TLS verification remained enabled and only the existing allowlisted `/read` methods were used.

| Check | Result |
|---|---|
| Authentication and version | Passed; authenticated `GetVersion` reported Core `2.3.4`. |
| Direct permissions | `ListPermissions` decoded and passed Pulse's direct-grant rejection checks. This response does not include group grants. |
| Effective Server permission level | The live level-only response was recognized as `Read`. |
| Effective specific permissions | **Unverified.** The response omitted specific-permission data, so Pulse stopped before returning inventory. |
| Inventory counts, resource states, parent relationships, and identity stability | **Not qualified.** The harness stopped on the first Server effective-scope check; no complete inventory was returned and no raw resource data was logged. |
| Monitoring integration and Docker image | Not started because effective read-only scope remains unqualified. |

### Operator attestation still required

An authorized Komodo administrator must verify that the API-key principal is non-admin and that, for every Server, Stack, Deployment, and Swarm that this key can expose, the effective permission is `Read` with no specific grants. That verification must include direct user grants, every applicable user group including Everyone groups, resource base permissions, and inherited target permissions. The live endpoint's level-only response cannot provide this attestation, and the existing non-admin read endpoints cannot independently reconstruct it. Do not treat a key described as “read-only” as sufficient evidence. No additional credential or raw API schema is required; rerun live qualification after the effective-permission representation is corrected or the operator verification gate is explicitly resolved in a reviewed change.

The focused fixture suite passes (`go test ./internal/komodo -count=1`: 67 tests), and `go vet ./internal/komodo` passes. The live qualification test exits with the expected safe permission-verification gate. No monitoring, alert, notification, UI, Patrol, metrics, or Docker-image work was started. No Komodo mutation, merge, deployment, or execution authority was introduced.

## Phase 2E operator permission attestation gate — 2026-10-25

The live `GetPermission` string response cannot establish the absence of effective specific permissions. The non-admin read APIs previously reviewed cannot reconstruct the resolver's complete permission set, including direct grants, UserGroups (including Everyone), resource defaults, and inherited permissions. Production validation remains unchanged: it accepts only supported structured `Read` with an empty `specific` list and rejects unknown/elevated levels, malformed responses, and non-empty specifics.

The opt-in test harness now accepts the observed top-level JSON string `"Read"` only when an operator attestation is loaded from an explicitly selected, owner-only file. The gate is test-only and cannot be reached by production credential validation or the monitoring poller. Once loaded, the harness adapts only successful `/read/GetPermission` bodies that are exactly the JSON string `"Read"` into the existing structured `{ "level":"Read", "specific":[] }` decoder input. It passes structured responses unchanged, so elevated specific permissions remain rejected, and it passes `None`, `Execute`, `Write`, unknown levels, malformed bodies, error responses, and other methods unchanged. The test output labels adapted evidence `operator_attested`; it does not label it API-verified.

Attestation is bound to the canonical HTTPS Core origin and an HMAC-SHA256 fingerprint derived from the credential's API key and secret. The secret is only used as the in-memory HMAC key; it is not printed or persisted by this mechanism. A credential rotation or endpoint change invalidates the binding. The owner-only attestation file must be no larger than 4 KiB, must be a regular non-symlink file owned by the test operator, and must grant no group/other permissions. Claims expire after 24 hours; timestamps more than five minutes in the future are rejected. This freshness window is a qualification safeguard, not API evidence.

The accepted file has this exact schema; all claims must be `true`, the version must be `1`, and `verifiedAt` must be the UTC RFC3339 time at which an authorized operator completed the check:

```json
{
  "version": 1,
  "endpoint": "https://komodo.home.goneczy.com",
  "credentialFingerprint": "<64-character fingerprint printed by the opted-in harness>",
  "verifiedAt": "<UTC RFC3339 timestamp>",
  "principalNonAdmin": true,
  "allExposedResourcesRead": true,
  "noEffectiveSpecificPermissions": true,
  "noElevatedDirectGrants": true,
  "noElevatedUserGroupGrants": true,
  "noElevatedEveryoneGroupGrants": true,
  "noElevatedResourceDefaults": true,
  "noElevatedInheritedPermissions": true
}
```

`credentialFingerprint` is never a credential and does not reveal the API secret. When no attestation path is selected, the opt-in harness prints the fingerprint before contacting Core so an operator can prepare a bound assertion even if the endpoint is temporarily unavailable. The operator must set `PULSE_KOMODO_QUALIFICATION_ATTESTATION_FILE` to the file path and rerun only the opt-in live test (`PULSE_KOMODO_LIVE_QUALIFICATION=1`). The file is an operator assertion and must not be described as verified by Komodo's API.

Focused tests cover missing and non-opted-in use, endpoint and credential mismatch, stale and future timestamps, incomplete permission claims, owner-only file enforcement, endpoint/key/secret fingerprint binding, the exact level-only adaptation, and pass-through of structured, elevated, non-Read, error, and unrelated responses. Existing client tests continue to assert rejection of elevated structured specifics and unsafe permission levels with no test adapter. No production auth or permission validation changed.

### File-by-file changes in this checkpoint

| File | Change |
|---|---|
| `internal/komodo/live_qualification_credentials_test.go` | Share a single owner-only, non-symlink qualification-file reader between the approved credential file and operator attestation. Existing opt-in and file-safety rules remain in force. |
| `internal/komodo/live_qualification_attestation_impl_test.go` | Add test-binary-only attestation parsing, owner-only file validation, endpoint/credential HMAC fingerprint binding, 24-hour freshness checks, strict all-claims validation, and the attested exact-`"Read"` response adapter. No production package code changes. |
| `internal/komodo/live_qualification_attestation_test.go` | Cover opt-in, missing/mismatched/stale/future/invalid/incomplete attestations, file permissions, fingerprint binding, narrow response adaptation, elevated-response rejection, and end-to-end fixture inventory collection. |
| `internal/komodo/live_qualification_test.go` | Load the explicitly selected owner-only assertion for the live harness, provide a separate network-free fingerprint-only mode, print only a non-secret fingerprint when no assertion path is selected, and label permission evidence as API-verified or operator-attested. |
| `docs/plans/2026-10-23-sindri-integration-roadmap.md` | Update Phase 2E status and the remaining permission-attestation gate. |
| `docs/plans/2026-10-25-komodo-phase-2-checkpoint.md` | Record the attestation contract, freshness and file rules, test evidence, and current live blocker. |

### Current environment rerun

The opt-in test loaded the already-approved owner-only credential file and emitted its HMAC credential fingerprint without displaying credential material. The subsequent authenticated API request could not begin because this Codex environment currently failed DNS resolution for `komodo.home.goneczy.com` (`getent` exit 2; curl error 6, “Could not resolve host”). This is a current environment/network blocker, distinct from the earlier successful authenticated `GetVersion` result reporting Core `2.3.4`. This rerun produced no new authentication, permission, inventory, resource-state, parent, or identity-stability result. The operator attestation has not been recorded in this checkpoint, and Phase 2E integration and image preparation remain gated on both accepted attestation and a reachable endpoint.

Validation for this focused change: `rtk go test ./internal/komodo -count=1` passed (102 tests); `rtk go vet ./internal/komodo`, `rtk git diff --check`, and `rtk npm run lint:planning-docs` passed. The live qualification command was attempted separately and is not counted as a successful live test.

## Resume Phase 2E operator evidence review

The operator supplied additional Core `2.3.4` observations: DNS resolved to `10.1.10.201`; HTTPS certificate validation and authentication succeeded; `GetVersion` returned `2.3.4`; `ListPermissions` returned an empty array; five Servers and five Stacks returned level-only effective `GetPermission: "Read"`; no Deployments or Swarms were accessible in the sampled inventory; and the UI showed `sindri-api` as non-admin, with Create Server/Create Build disabled, base Read permissions, and no specific grants displayed. These are operator-reported observations for this resumed checkpoint, not live requests reproduced in this run.

The fingerprint-only mode safely loaded the existing protected file, checked its owner-only file restrictions, and recomputed the same non-secret HMAC fingerprint as the preceding checkpoint. It made no HTTP request. The fingerprint is intentionally not stored in this repository.

The supplied evidence does not attest to effective permission resolution through UserGroups, Everyone groups, resource-default permissions, or inherited target permissions. The UI's “no specific grants displayed,” the empty direct `ListPermissions` array, and level-only per-resource `"Read"` responses do not establish that no effective specific permissions exist across those sources. Therefore no attestation file was created and no authenticated API request, inventory qualification, identity-stability comparison, parent validation, integration work, or image build was performed in this turn. Resume qualification only after an authorized operator explicitly verifies those remaining claims and provides the owner-only assertion bound to the verified fingerprint.

Validation after adding the network-free fingerprint mode passed: `rtk go test ./internal/komodo -count=1` (102 tests), `rtk go vet ./internal/komodo`, `rtk git diff --check`, and `rtk npm run lint:planning-docs`.
