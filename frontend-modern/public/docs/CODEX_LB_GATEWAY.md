# Codex-LB gateway for Sindri

Sindri uses Pulse's existing OpenAI-compatible Chat Completions adapter and
Patrol investigation runner. When `fork.codex_lb_gateway` is enabled, the
backend admits only the separately configured Codex-LB route and its explicitly
selected model. Other saved provider settings remain stored for compatibility,
but cannot be used for inference under this policy.

## Configure the gateway

1. Configure Codex-LB and its upstream account or source using the
   [Codex-LB client setup guide](https://github.com/Soju06/codex-lb/blob/main/docs/client-setup.md).
   Confirm the endpoint and gateway API key with the administrator responsible
   for that deployment.
2. In Pulse, open **Settings → Fork Capabilities**, enable
   `fork.codex_lb_gateway`, and save the profile. The fork profile only enables
   the local policy; it does not provide a gateway endpoint, credential, model,
   upstream account, or entitlement.
3. Open **Settings → Pulse Intelligence → Provider Configuration** and expand
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

Codex-LB's Chat Completions compatibility surface uses `/v1/chat/completions`,
streamed `chat.completion.chunk` events, and OpenAI-style tool-call and error
envelopes per its [compatibility specification](https://github.com/Soju06/codex-lb/blob/main/openspec/specs/chat-completions-compat/spec.md).
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

## Credential rotation and removal

Enter a replacement key and save to rotate it. The current value remains
masked. **Remove** clears this provider's endpoint, credential, private-network
option, and gateway-owned model selections; it does not delete other providers'
saved settings.

The added settings are optional fields in the existing encrypted AI settings
file. Existing installations require no file conversion or destructive
migration.
