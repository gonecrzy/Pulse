# Standalone Pulse / Sindri egress inventory

**Audited:** 2026-10-27

**Runtime baseline:** commit `c325f89bd` plus the Phase 2K local changes described in the Sindri roadmap.
**Scope:** Pulse-owned licensing, entitlement, billing, activation, telemetry, updates, and normal configured outbound integrations. This is a source-level inventory; no production endpoints or credentials were contacted.

## Default policy

The standalone process starts without a Pulse license credential and does not
load persisted Pulse activation or hosted entitlement data as current state by
default. It preserves those files for rollback. Activation and checkout APIs
return `410 Gone`; `/api/license/entitlements` remains for client compatibility
and reports no hosted subscription or trial. The compatibility exception is an
explicit `PULSE_UPSTREAM_COMMERCIAL_SERVICES=true` operator setting. Hosted
deployments retain their server-managed entitlement behavior.

Telemetry defaults off. `telemetry.Start` returns before creating a runner or
sender when disabled. A saved explicit `telemetryEnabled: true`, the first-run
or Settings opt-in, or `PULSE_TELEMETRY=true` enables the startup ping and
daily heartbeat. A missing preference in an older `system.json` resolves to
off. Automatic release checks and unattended updates default off; when
`autoUpdateEnabled` is true on the stable channel, the startup/hourly check and
the systemd update timer may contact the selected update source.

These controls retire implicit Pulse-owned commercial and telemetry traffic;
they do not disable operator-configured monitoring, AI, notifications,
identity providers, probes, or action executors.

## Outbound inventory

| Path and source anchors | Destination/classification | When it sends | Standalone behavior |
|---|---|---|---|
| License activation and legacy exchange: `internal/api/licensing_handlers.go`, `pkg/licensing/license_server_client.go`, `pkg/licensing/activation_types.go` | Pulse licensing service (`license.pulserelay.pro`, overridable for tests/configuration); **upstream Pulse service to retire** | Operator submits an activation key; legacy compatibility can retry exchange after startup. | Activation returns 410 unless `PULSE_UPSTREAM_COMMERCIAL_SERVICES=true`. Legacy grant/license files are preserved but not loaded for current entitlement by default, and no exchange retry is started. |
| Grant refresh and installation status: `pkg/licensing/grant_refresh.go`, `pkg/licensing/installation_status_poll.go`, `internal/api/licensing_handlers.go` | Pulse licensing service; **upstream Pulse service to retire** | Periodic refresh/status polling after a grant is loaded. | No client is wired and no loops start by default. Existing activation state remains on disk. |
| Hosted entitlement/trial refresh: `internal/api/hosted_entitlement_refresh.go`, `pkg/licensing/entitlement_lease.go`, `pkg/licensing/database_source.go` | Configured hosted entitlement service; **upstream Pulse service to retire for standalone** | Hosted lease refresh and trial lifecycle; retry/backoff is hosted-service behavior. | Standalone refresh loops are stopped and billing snapshots are not used as current entitlement state by default. Hosted mode is unchanged. |
| Checkout, purchase handoff, and trial: `internal/api/licensing_handlers.go`, `pkg/licensing/upgrade.go`, `pkg/licensing/trial_activation.go` | Pulse account, checkout, and trial services; **upstream Pulse service to retire** | Operator clicks purchase/trial/activation controls or follows a handoff. | Checkout and activation routes return 410 by default. The standalone UI does not offer a trial or purchase activation path. Historical billing/trial files are retained. |
| Stripe subscription webhook: `internal/api/payments_webhook_handlers.go`, `pkg/licensing/stripe_subscription.go` | Pulse-hosted billing ingress; **inbound only** | Pulse-hosted billing provider posts a signed event. | Not an outbound runtime request and not used for standalone capability admission. Hosted signature validation and deduplication are unchanged. |
| Usage and service-health telemetry: `internal/telemetry/telemetry.go`, `pkg/server/server.go`, `internal/config/config.go` | `https://license.pulserelay.pro/v1/telemetry/ping`; **upstream Pulse service to retire by default** | Startup and daily heartbeat after an explicit opt-in; listener-failure telemetry uses the same opt-in. | Disabled by default. No sender goroutine is created and no request is sent while disabled. Explicit opt-in retains the existing content-bounded payload and 30-day rotating install ID. |
| Background release checks and auto-update: `internal/api/router.go`, `internal/updates/`, `internal/config/auto_update_environment_test.go` | GitHub Releases / configured `PULSE_UPDATE_SERVER`; **necessary package/update infrastructure** | Startup and hourly checks, and the systemd timer, only when unattended updates are enabled on stable. | Default off. The existing manual **Check for updates** control remains operator-triggered. `PULSE_UPDATE_SERVER` selects the source; it is not silently substituted for AI-provider traffic. |
| Pro-brokered update artifacts and agent helper downloads: `internal/api/`, `internal/updates/`, agent installation handlers | Pulse download broker or GitHub release asset; **upstream Pulse service / necessary update infrastructure** | Operator requests a Pro artifact or agent install/update. | Pro broker traffic requires the explicit commercial compatibility setting. Public agent/helper release downloads remain operator-triggered. |
| Local health probe used only by telemetry: `pkg/server/service_health.go`, `pkg/server/server.go` | Pulse's own bound listener; **local loopback/self-call** | Included in an enabled telemetry snapshot or bounded startup-health event. | No telemetry snapshot or probe sender is scheduled while telemetry is disabled. It does not report the listener URL or response body. |
| Proxmox, PBS, PMG, VMware, TrueNAS, Docker, Kubernetes, and host-agent collectors: provider packages under `internal/` and `pkg/`, configured by `internal/config/` | Operator-configured infrastructure endpoints; **operator-configured infrastructure service** | Automatic polling at the configured intervals; connection tests are operator-triggered. | Unchanged. Credentials and destination URLs come from the existing protected connection configuration. |
| Komodo read-only collector: `internal/komodo/`, resource registration in `internal/api/` | Operator-configured Komodo Core endpoint; **operator-configured infrastructure service** | Automatic inventory polling while a Komodo connection is enabled; setup/validation is operator-triggered. | Unchanged. Calls remain limited to allowlisted read methods; no mutation endpoint is registered. |
| AI chat, Patrol analysis, investigation, model discovery, and readiness preflight: `internal/ai/providers/`, `internal/ai/chat/`, `internal/api/ai_handlers.go` | Selected provider, custom OpenAI-compatible URL, or local Ollama; **operator-configured AI service / AI gateway** | User chat, an enabled scheduled/event Patrol run, explicit investigation, model discovery, or operator-triggered preflight. | Unchanged. Existing provider selection and secret storage apply. This checkpoint does not impose Codex-LB-only routing. |
| Alert and status notifications: `internal/notifications/`, `internal/alerts/` | Configured SMTP, Apprise, webhook, Discord, or other notification destination; **optional third-party integration** | Automatically on configured alert/finding events; test-send is operator-triggered. | Unchanged and inactive without an enabled destination. No fallback destination is introduced. |
| OIDC/SAML login and provider tests: `internal/api/router_routes_auth_security.go`, SSO packages | Operator-configured identity provider; **optional third-party integration** | Login redirects/token exchange, metadata/key retrieval, or an operator-triggered connection test. | Unchanged where the local runtime exposes the feature. The IdP is not a Pulse entitlement service. |
| Availability probes and discovery: `internal/alerts/external_probe.go`, availability/probe agent packages, `pkg/discovery/` | Operator-configured targets or the local operator-selected subnet; **operator-configured infrastructure service** | Enabled recurring probes/discovery; initial discovery and connection tests are operator-triggered. | Unchanged. No broad public Internet scan or implicit target is added. |
| Action plan execution: `internal/actionlifecycle/`, `internal/actionrunner/`, typed provider executors | Operator-configured infrastructure/provider endpoints; **operator-configured infrastructure service** | Only after authenticated plan/approval and executor admission; each provider performs its own request. | Existing actor, organization, permission, approval, revision, budget, cooldown, readiness, and audit checks remain. `fork.governed_actions` does not grant approval or dispatch authority. |
| Build, test, image pull, and deployment tooling | Go/npm registries, GitHub, container registry; **necessary package/update infrastructure** | Build/test/deployment operator or CI workflow. | Not initiated by the standalone Pulse runtime. This checkpoint performs no publish or deployment. |

## Feature admission and route compatibility

| Existing path | Standalone treatment |
|---|---|
| `fork.ai_investigation` and `POST /api/ai/findings/{id}/reinvestigate` | Supported locally through the existing investigation runner and finding store. It is separately checked against the local versioned fork profile and request organization/resource. It does not set `ai_autofix`, `ai_alerts`, Pro, or hosted entitlement state. |
| `fork.governed_actions` and `/api/actions/*` | Supported locally as availability for the existing typed action lifecycle. Authentication, scope, actor/organization checks, approval, readiness, revision binding, and audit still decide whether an operation can proceed. |
| `fork.deterministic_triage`, `fork.komodo_read_only` | Supported local monitoring capabilities; neither depends on AI success or permits Komodo mutations. |
| Agent profiles, RBAC, audit-log read/export, advanced reporting, and external probes | Their public implementations are represented by `fork.agent_profiles`, `fork.rbac`, `fork.audit_logging`, `fork.advanced_reporting`, and `fork.external_probe`. Their existing middleware/manager admission uses the local profile in standalone mode; each is explicitly supported but disabled until an administrator enables it. They remain separate from entitlement response fields. |
| `fork.action_safety`, `fork.operational_cases`, `fork.discord_ops`, `fork.codex_lb_gateway` | Not enabled by the local profile. Their required implementation or qualification is incomplete. `fork.long_term_metrics` is unavailable in this runtime. |
| General Pulse `ai_alerts` / `ai_autofix` commercial flows | Existing Pulse entitlement decisions are not forged or rewritten. The current standalone Sindri entry is the explicit finding re-investigation route; alert-autonomous analysis and automatic repair remain unavailable unless separately implemented under fork policy. |
| Relay, mobile/push, long-term metrics, Kubernetes AI, multi-tenant, multi-user, white-label, and other unmapped licensed routes | Remain unavailable unless their unchanged free-tier policy already allows them. They are not granted by `fork.governed_actions` or an unrelated local capability. Relay-dependent features are not enabled through an upstream Pulse service. A route whose public runtime implementation is absent remains unavailable; this checkpoint does not turn on a private-runtime hook by changing its label. See the Phase 2J source matrix for exact source ownership. |
| `GET /api/license/entitlements` | Kept for older clients. It returns locally resolved/free state and explicit `commercial_services_enabled: false`; it does not synthesize paid claims, a trial, or hosted status. |

## Remaining outbound requests

Automatic outbound requests can still occur to operator-configured infrastructure,
AI providers, identity providers, enabled probe targets, and notification
destinations. The update source can be reached automatically only after
unattended updates are enabled. Pulse telemetry and standalone commercial
activation/refresh/checkouts are off by default. Operator-triggered update,
provider, connection, notification, SSO, activation-compatibility, and action
operations retain their separately documented destinations and checks.

The code search found no other standalone Pulse-owned recurring entitlement or
usage sender outside the source paths listed above. This is not a firewall
policy: operator-selected URLs remain the operator's responsibility and are
validated by their existing connection/provider-specific rules.
