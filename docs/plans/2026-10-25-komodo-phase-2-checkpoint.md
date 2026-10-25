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
- `internal/unifiedresources/metrics_targets.go` is the canonical history-target map. Since it has no Komodo mapping, normalized Komodo resources enter the inventory with no implied historical series.
- `internal/api/router.go` owns poller startup and shutdown, while existing infrastructure settings and `ConnectionEditor` components own connection configuration. The Komodo UI uses those surfaces and the common `ConnectionType` API.

## Resource mapping matrix

All Komodo source IDs are opaque and stable within the Pulse connection: `komodo:<base64url(connection UUID)>:<lowercase kind>:<base64url(native Komodo ID)>`. The registry then derives its canonical source-specific ID from `(Komodo resource type, SourceKomodo, source ID)`. Names and labels are display data only.

| Komodo kind | Pulse resource type | Parent / relationship mapping | Metrics and history key | Facet need |
|---|---|---|---|---|
| Server | `komodo-server` | Each listed Swarm `server_ids` entry produces Server `member_of` Swarm. No singular parent is inferred. | None assigned. `BuildMetricsTarget` returns nil; no Komodo metric writer or history family exists. | No new facet for current inventory fields. |
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
- Docker network attachment edges are derived again from Docker-native inventory. Komodo edges are likewise rebuilt from each successful inventory. Generic dependency or route edges have no independent durable operator store, and there is no `routes_to` edge type or Traefik configuration parser.
- `ResourceRelationshipsWithCanonicalParent` feeds resource API/detail projections. The generic Infrastructure resource detail drawer already has a “Relationship map”. Alert incident synthesis traverses active canonical relationships to build causal/correlated context; relationships do not independently close or suppress alerts. AI chat and handoff code can include generic relationship context, but Komodo-specific Patrol context still needs qualification and explicit product decisions. Polling does not depend on either.

Future cross-provider routing or dependency work should extend this central graph and add explicit edge origin, verification, and persistence where operator-authored edges are required. Reuse the existing operator link for verified resource identity. Do not convert name similarity into identity or treat a dependency edge as an identity merge.

## Remaining integration points

| Area | Current foundation | Remaining work |
|---|---|---|
| Historical metrics | Generic targets exist for existing supported types; Komodo resource types are intentionally unmapped. | Pick a Komodo metric family/key, add a bounded metric writer and history API target, retention and migration behavior, charts, and tests. Decide whether each resource kind has useful metrics before adding a facet. |
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

## Validation and review status

Passed checks:

- `rtk go test ./internal/unifiedresources -count=1` — 3,028 tests, including registry ingestion and operator-confirmed cross-provider identity-link relationship preservation.
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

## References

- Komodo [API and clients](https://komo.do/docs/ecosystem/api), [permissioning](https://komo.do/docs/configuration/permissioning), [live API docs](https://demo.komo.do/docs), [ListServers pagination schema](https://docs.rs/komodo_client/latest/komodo_client/api/read/struct.ListServers.html), [Stack parent fields](https://docs.rs/komodo_client/latest/komodo_client/entities/stack/struct.StackListItemInfo.html), [Deployment parent fields](https://docs.rs/komodo_client/latest/komodo_client/entities/deployment/struct.DeploymentListItemInfo.html), [Swarm membership fields](https://docs.rs/komodo_client/latest/komodo_client/entities/swarm/struct.SwarmListItemInfo.html), [v2.3.2 client API reference](https://docs.rs/komodo_client/2.3.2/komodo_client/), and [release list](https://github.com/moghtech/komodo/releases). References inform the protocol shape; support is not pinned to `2.3.3`.
- Pulse resource API: `/api/resources/{id}/link` and `/unlink`; resource graph and supplemental record APIs are owned by `internal/unifiedresources`.
