# Pulse + Sindri — Phase 1 Checkpoint

**Status:** Implementation complete for independent review. Phase 2 is not started.

**Approved baseline:** `df5799d9b0dff69dac481d4ae4b062b8288eea7a`  
**Branch:** `feature/sindri-integration`

## Delivered

- Added `internal/forkfeatures`, a deployment-wide, versioned `ForkProfile`, registry, persistent manager, and central access resolver. The approved identifiers are registered; only `fork.ai_investigation` and `fork.governed_actions` have implementations in this checkpoint. Every feature is disabled in the default profile, and unimplemented, unknown, duplicate, stale, or invalid profile values fail closed.
- Persisted the profile as `fork_profile.json` under Pulse configuration storage with strict JSON decoding, atomic writes, and owner-only file permissions. Administrators can read and replace it through authenticated settings routes with revision checks and audit events.
- Exposed fork availability through `/api/fork/capabilities`, independently from commercial license payloads. Added typed SolidJS API and store state in `frontend-modern/src/api/forkCapabilities.ts` and `frontend-modern/src/stores/forkCapabilities.ts`; a failed or missing response does not mark a capability available.
- Routed public finding investigation through Pulse's existing Patrol findings store and `internal/ai/chat` investigation runner. The authenticated endpoint still requires `ai:execute`; the service entry point independently checks the same resolver. Existing `ai_autofix` commercial checks and entitlement reporting remain unchanged. Fork availability does not unlock private reapproval, remediation, or approval endpoints.
- Gated governed-action HTTP planning, refresh, and approvals and wired the same resolver into the canonical action lifecycle for plan, approval, human execution admission, and policy execution admission. Missing admission wiring now fails closed. Existing action authorization, target binding, policies, approvals, durable receipts, rejection, and recovery behavior remain in the lifecycle. A fork capability does not grant action permission.
- Added the stable action error code to the capability manifest and regenerated `cmd/pulse-mcp/README.md`.

No action tuples, executor operations, autonomous execution paths, or Sindri implementation code were added. Automatic remediation remains controlled by its existing Pulse policy and license paths. No production configuration or deployment was changed.

## Commercial and source boundaries

- Public fork investigation is served by the checked-in Pulse chat/Patrol implementation when `fork.ai_investigation` is enabled; the route test verifies this reaches the public handler without requiring a commercial entitlement.
- Commercial entitlement checks and response fields were not redefined. The resolver may report `commercial_license` for the existing commercial investigation path, but fork capability responses contain only fork-profile availability.
- Existing `ai_autofix` checks remain on private investigation reapproval/fix execution, Patrol auto-fix settings, and remediation handlers. The public typed action lifecycle has no separate commercial entitlement check; fork availability gates entry while the existing actor, target, capability, approval, and policy checks continue to govern each action. No commercial gate was removed or added.
- Enabling both implemented fork capabilities does not unlock the private investigation fix endpoint. No references to unavailable private implementation packages were added.
- Pulse's existing MIT notices remain intact. No Sindri source or private infrastructure details were copied.

## Validation results

- Passed: `PULSE_LEGACY_KEY_PATH=/tmp/pulse-phase1-no-legacy-key go test ./internal/forkfeatures ./internal/config ./internal/actionlifecycle ./internal/ai/chat ./internal/agentcapabilities ./pkg/licensing -count=1 -timeout=15m` (all listed packages passed).
- The full API package run completed with two failures: the generated MCP README projection was stale after adding the shared action error, and a file-permission test ran as root. The README was regenerated with `go run ./scripts/generate-pulse-intelligence-docs.go`; `TestContract_PulseMCPReadmeManifestProjectionStaysGenerated` then passed. `TestTelemetryUpdate_NoMutationOnPersistFailure` passed when its compiled API test binary ran as `nobody`; the root-run failure is an environment limitation because root bypasses the test's chmod-based write denial.
- Passed focused API/security/lifecycle tests: `go test ./internal/api -run 'Test(MissingAdmissionAvailabilityFailsClosed|AdmissionAvailabilityBlocksNewTypedPlans|DisabledAdmissionStillAllowsPendingActionRejection|PatrolActionBrokerBarrierReplayAdmitsExecutorExactlyOnce|APTUpdateDetectorProposalApprovalDispatchAuditAndFindingReconciliation|AssistantPlansRestartForUnlockedGuest|AttentionActionPlanBindsCanonicalRecordEvidenceAndReplaysIdempotently|PatrolFullModeRunsStorageCleanupThroughCanonicalLifecycle|PatrolFullModeRunsHostUpdateThroughCanonicalLifecycle|HostUpdateCompatibleProposalRefusesLiveAgentDowngradeBeforeExecuting|Contract_ActionDryRunOnlyExecutionErrorJSONSnapshot|Contract_AgentSurfaceErrorCodesMatchManifestDeclarations|Fork.*|AuthenticatedActionPlanRouteRequiresForkAvailability)' -count=1 -timeout=3m`. This includes invalid profile and stale revision rejection, authenticated route enforcement, unauthorized action denial, public investigation without a commercial entitlement, private-fix denial, direct chat-service admission, action lifecycle admission and missing-resolver failure, Patrol proposal replay, and the shared error-manifest contract.
- Passed the root-sensitive API permission test as an unprivileged user: `runuser -u nobody -- sh -c 'cd /tmp && PULSE_LEGACY_KEY_PATH=/tmp/pulse-phase1-no-legacy-key ./pulse-api-phase1.test -test.run "^TestTelemetryUpdate_NoMutationOnPersistFailure$" -test.count=1'`.
- Passed: `npm --prefix frontend-modern test -- src/api/__tests__/forkCapabilities.test.ts src/stores/__tests__/forkCapabilities.test.ts` (5 tests).
- Passed: `npm --prefix frontend-modern run type-check`.
- Passed: `npm --prefix frontend-modern run build`.
- Passed: `scripts/audit-private-boundary.sh --enforce`; it reported zero new private implementation leakage and no API imports outside the established licensing bridge.
- Passed: `git diff --check`.

## Remaining limits

- `fork.codex_lb_gateway`, `fork.komodo_read_only`, `fork.deterministic_triage`, `fork.action_safety`, `fork.operational_cases`, `fork.long_term_metrics`, and `fork.discord_ops` remain registered but unavailable because their Phase 2+ implementations are absent.
- `fork.governed_actions` exposes the existing typed action system only. The configured action allowlist and existing per-action authorization/policy controls remain authoritative; this checkpoint grants no action tuple or execution permission.
- The full API suite should be run as a non-root user in CI to avoid the known chmod-based test limitation. Its root-only failure and corrected generated README test are documented above.
- No production capability has been enabled; the persisted profile defaults empty.
