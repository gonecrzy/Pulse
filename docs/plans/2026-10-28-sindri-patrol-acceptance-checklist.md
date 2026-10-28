# Sindri Patrol test-image acceptance checklist

**Status:** Pending future test-image qualification. This checklist is a
read-only operator procedure; Phase 2N fixture tests are not live gateway or
infrastructure evidence.

Use only the approved test image and synthetic diagnostic resources. Do not
paste credentials into test notes or source control. Record the image digest,
Pulse version, gateway version/model where available, qualification time, and
redacted request/correlation identifiers.

## Monitoring and resource identity

- [ ] Existing deterministic monitoring remains healthy before, during, and
      after AI gateway tests.
- [ ] Patrol scheduling, run history, findings, and notifications continue to
      update while Codex-LB is unavailable.
- [ ] Proxmox and Komodo observations resolve to the expected canonical VM
      identity; ambiguous or conflicting identities remain separate.
- [ ] Investigation topology shows verified host/VM/Stack relationships,
      availability checks, and storage dependencies with provenance and freshness.
- [ ] Direct dependencies are distinguishable from shared infrastructure and
      incidental graph neighbors. Stale or missing observations are visible.

## Investigation and gateway

- [ ] Existing findings render with evidence separated from diagnostic
      interpretation; inferred causes are presented as hypotheses.
- [ ] An authorized operator can start a manual Sindri re-investigation, and
      organization/resource isolation is preserved.
- [ ] Codex-LB authentication succeeds and the explicitly configured model is
      discovered or selected without substituting a model.
- [ ] Streaming Chat Completions, tool-call parsing, sequential tool
      continuation, cancellation, bounded context, timeout handling, and usage
      reporting complete against the approved gateway.
- [ ] Gateway authentication, model, streaming, or tool failures do not switch
      to another provider; Patrol reports the failure while deterministic
      monitoring and notifications continue.
- [ ] The Patrol UI reports endpoint configured state, selected model,
      credential configured state, connectivity, tool-calling and streaming
      readiness, and the last successful qualification without exposing the key.

## Governed actions

- [ ] Repair proposals show their target, rationale, supporting evidence,
      approval requirement, action status, execution readiness, and verification
      requirements.
- [ ] Each proposal links to the exact existing `/actions` identity and its
      canonical audit trail.
- [ ] No infrastructure mutation occurs without the current Pulse action
      permission, approval, emergency-stop, and execution lifecycle checks.
- [ ] Operation receipts are not presented as proof of recovery unless the
      expected postcondition is independently verified.
