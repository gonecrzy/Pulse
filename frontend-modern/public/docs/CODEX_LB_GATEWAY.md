# Codex-LB gateway for Sindri

Sindri uses Pulse's existing OpenAI-compatible Chat Completions adapter and
Patrol investigation runner. When `fork.codex_lb_gateway` is enabled, the
backend admits only the separately configured Codex-LB route and its explicitly
selected model. Other saved provider settings remain stored for compatibility,
but cannot be used for inference under this policy.

## Configure the gateway

1. Configure Codex-LB and its upstream account or source using the
   [Codex-LB client setup guide](https://github.com/Soju06/codex-lb/blob/69f128afcbc616d9f8e924ca6583f7031d75cf82/docs/client-setup.md).
   Confirm the endpoint and gateway API key with the administrator responsible
   for that deployment.
2. In Pulse, open **Settings → Fork Capabilities**, enable
   `fork.codex_lb_gateway`, and save the profile. This enables the local policy
   only; it does not provide a gateway endpoint, credential, model, upstream
   account, or entitlement.
3. Open **Settings → Pulse Intelligence → Provider & Models** and expand
   **Sindri Codex-LB gateway**. Enter the gateway base endpoint, such as
   `https://codex-lb.example/v1`, and its API key. Pulse sends the key as a
   Bearer credential and stores it with the existing encrypted AI settings.
4. Save the settings, load the provider's model catalog, and explicitly select
   a gateway model. Any configured chat, Patrol, or fix-model override must
   also select a model from this gateway. Pulse does not invent a default model
   or replace an unavailable selection.
5. Run **Run Preflight**, then run the Patrol model-readiness advisor. The
   gateway card reports connectivity, tool-call readiness, streaming readiness,
   credential configured state, and the last successful qualification. The
   saved API key is never returned to the browser.

The endpoint is a base URL for OpenAI-compatible `GET /models` and
`POST /chat/completions` calls. Do not put credentials in URL user information,
query strings, or fragments. Pulse does not infer that a generic OpenAI-
compatible endpoint is Codex-LB: the administrator explicitly designates the
endpoint and key in the dedicated Codex-LB fields. That is an operator trust
assertion, not cryptographic product attestation. Verify the gateway host and
TLS certificate out of band.

## Activate a configured gateway

When Provider & Models reports that the gateway capability is disabled, use
**Settings → Fork Capabilities** to enable `fork.codex_lb_gateway`. This page
also reports each registered capability's implementation status, maturity,
profile state, effective availability, prerequisites, and restrictions. The
gateway connection test stays disabled until its capability becomes available.
After saving, return to Provider & Models; its settings and readiness state
reload when that page opens. No Pulse restart is required.

For an instance where the administrator needs to update the profile through the
API, the following commands preserve every existing enabled capability and add
only `fork.codex_lb_gateway` and `fork.ai_investigation` when missing. Set
`PULSE_URL` and `PULSE_API_TOKEN` through a protected environment or secret
manager. The token must be instance-wide and have `settings:read` and
`settings:write`; organization-bound tokens are refused by these instance
administration routes. Token-authenticated requests without a session cookie do
not require CSRF. Browser-session requests do require the session's CSRF token.

```sh
set -eu
: "${PULSE_URL:?Set PULSE_URL to the Pulse base URL}"
: "${PULSE_API_TOKEN:?Load an instance-wide settings token into PULSE_API_TOKEN}"

umask 077
auth_headers="$(mktemp)"
trap 'rm -f "$auth_headers"' EXIT
printf 'X-API-Token: %s\n' "$PULSE_API_TOKEN" > "$auth_headers"
pulse_curl() {
  curl --fail --silent --show-error --header "@$auth_headers" "$@"
}

base="${PULSE_URL%/}"
profile="$(pulse_curl "$base/api/fork/profile")"
if printf '%s' "$profile" | jq -e '
  [.enabled_features[]]
  | index("fork.codex_lb_gateway") != null
    and index("fork.ai_investigation") != null
' >/dev/null; then
  printf 'Both requested capabilities are already enabled.\n'
  exit 0
fi

update="$(printf '%s' "$profile" | jq -c '
  {
    version,
    expected_revision: .revision,
    enabled_features: (
      .enabled_features + ["fork.codex_lb_gateway", "fork.ai_investigation"]
      | unique
      | sort
    )
  }
')"
saved="$(pulse_curl --request PUT \
  --header 'Content-Type: application/json' \
  --data-binary "$update" \
  "$base/api/fork/profile")"

printf '%s' "$saved" | jq -e '
  [.capabilities[] | select(
    .id == "fork.codex_lb_gateway" or .id == "fork.ai_investigation"
  )] as $requested
  | ($requested | length) == 2
    and all($requested[]; .enabled and .available)
' >/dev/null
printf '%s' "$saved" | jq '{revision, capabilities: [.capabilities[] | select(
  .id == "fork.codex_lb_gateway" or .id == "fork.ai_investigation"
) | {id, enabled, available}]}'
```

The write uses the revision read with the profile. If the API returns HTTP 409,
stop; another administrator changed the profile. Re-read and review that
version before retrying. The commands do not read or write AI provider settings,
gateway credentials, action safety, Komodo mutations, or other capabilities.
The profile change takes effect immediately and survives restart.

## Network policy

HTTPS is required by default. Public HTTP endpoints are rejected. An explicit
**Allow HTTP, localhost, or private network destinations** setting permits
private and loopback HTTPS destinations and permits HTTP only for `localhost`
or loopback/private IP literals. HTTP hostnames other than localhost are
rejected. Metadata-service, link-local, multicast, unspecified, and unsafe
resolved destinations remain blocked; runtime DNS resolution is checked again
for every request.

Use the private-network option only when the gateway is intentionally hosted on
a trusted private network. Prefer TLS for private DNS names. Do not put a
private or production endpoint in source control.

## Qualification and failure behavior

Pulse qualifies the configured model using the existing Patrol readiness and
tool preflight paths. These checks use synthetic evidence and in-memory tool
results; they do not execute infrastructure changes. They exercise model
authentication, model discovery, streaming Chat Completions, tool-call argument
assembly, and the bounded multi-turn tool-result continuation. Ordinary
provider usage fields continue to feed Pulse's existing token and cost
accounting.

Codex-LB v1.25.0-beta.9's Chat Completions compatibility surface uses `/v1/chat/completions`,
streamed `chat.completion.chunk` events, and OpenAI-style tool-call and error
envelopes per its [compatibility specification](https://github.com/Soju06/codex-lb/blob/69f128afcbc616d9f8e924ca6583f7031d75cf82/openspec/specs/chat-completions-compat/spec.md).
Pulse currently speaks Chat Completions; it does not claim Codex-LB Responses
API reasoning-state preservation. Context sizing uses Pulse's existing model
limits and Patrol readiness fixtures, so unknown model IDs use Pulse's default
context estimate. Provider token usage is reported when the gateway returns
compatible usage fields; estimated dollar cost may be unavailable for models
without Pulse pricing metadata.

If the gateway is unavailable, unauthorized, malformed, or incompatible, AI
work reports the provider failure and does not switch to OpenAI, Claude,
Ollama, or a subscription agent. Deterministic monitoring and triage continue
without model success. Fix proposals remain proposals; this integration does
not dispatch infrastructure mutations or enable automatic repairs.

## Phase 2M live qualification status

Live qualification was not performed in this checkout on 2026-10-28. The
`PULSE_DATA_DIR` override is unset, and the default runtime directory has no
`ai.enc` settings file. No gateway endpoint, selected model, or credential is
available here, so no live request was sent. Phase 2L mock tests establish
adapter behavior against local fixtures; they are not live Codex-LB evidence.

To qualify a deployment, make its approved Pulse AI settings available, enable
`fork.codex_lb_gateway`, configure the gateway endpoint and credential in Pulse,
select the gateway model, and explicitly authorize the synthetic read-only
qualification requests. Do not reuse credentials from unrelated applications.

Phase 2N adds a fixture-backed Patrol status view for the server-reported
gateway fields. This UI integration does not qualify a live Codex-LB deployment
or replace the test-image acceptance procedure; live connectivity, streaming,
tool continuation and latency remain unverified until that checklist passes.

## Credential rotation and removal

Enter a replacement key and save to rotate it. The current value remains
masked. **Remove** clears this provider's endpoint, credential, private-network
option, and gateway-owned model selections; it does not delete other providers'
saved settings.

The added settings are optional fields in the existing encrypted AI settings
file. Existing installations require no file conversion or destructive
migration.
