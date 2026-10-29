# Sindri Patrol test-image acceptance checklist

**Status:** Pending future test-image qualification. This checklist is a
read-only operator procedure; Phase 2N fixture tests are not live gateway or
infrastructure evidence.

Use only the approved test image and synthetic diagnostic resources. Do not
paste credentials into test notes or source control. Record the image digest,
Pulse version, gateway version/model where available, qualification time, and
redacted request/correlation identifiers.

## Finding triage and investigation

- [ ] An authorized operator sees **Investigate with Sindri** on an eligible
      active Patrol finding while Patrol is in monitor mode.
- [ ] The action is unavailable without the effective investigation
      capability or operator permission, and is disabled with a clear message
      until the configured gateway and model pass readiness checks.
- [ ] Starting a run returns a followable investigation ID; repeated clicks
      and retries open the same running session rather than starting duplicates.
- [ ] The finding row opens the current investigation, reports its running
      status and read-only evidence/tool progress, and remains followable after
      navigation or page reload.
- [ ] Completion and failure refresh the finding/activity views and preserve
      the current run's status, timestamps, tool names, evidence provenance,
      call counts, outcome, and safe error message.
- [ ] An investigation with no useful evidence or no diagnostic interpretation
      states that limitation explicitly. Model reasoning text is not displayed
      as observed evidence or a verified fact.
- [ ] Finding management controls are visually separate from investigation.
      Acknowledging, dismissing, suppressing, or resolving a finding does not
      imply that it was investigated or that an action executed.
- [ ] An existing investigation has a view link; any retry requires the
      current capability, authorization, gateway readiness, and server retry
      policy.
- [ ] An investigation run interrupted by navigation continues in Patrol;
      a restart recovers its current finding record without replacing the
      finding's newer occurrence.

## Monitoring and resource identity

- [ ] Existing deterministic monitoring remains healthy before, during, and
      after AI gateway tests.
- [ ] Critical or warning infrastructure findings affect infrastructure
      health counts without labeling a completed, unblocked Patrol runtime as
      degraded. Blocked or unavailable monitoring includes its runtime reason.
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
