/**
 * InvestigationSection
 *
 * Inline investigation details within expanded finding.
 * Replaces the drawer. Shows status, summary, tools used, and
 * a collapsible investigation thread.
 */

import {
  Component,
  createSignal,
  createResource,
  createEffect,
  createMemo,
  onCleanup,
  Show,
  For,
} from 'solid-js';
import { getInvestigation, reinvestigateFinding, formatTimestamp } from '@/api/patrol';
import {
  getInvestigationConfidenceBadgeTone,
  getInvestigationOutcomeBadgeTone,
  getInvestigationOutcomeLabel,
  getInvestigationStatusLabel,
  getInvestigationStatusBadgeTone,
} from '@/utils/aiFindingPresentation';
import { getInvestigationSectionState } from '@/utils/patrolEmptyStatePresentation';
import {
  buildPatrolInvestigationRecordPresentation,
  labelDiagnosticInterpretation,
} from '@/features/patrol/patrolInvestigationContextModel';
import { LoadingSpinner } from '@/components/shared/LoadingSpinner';
import { MetadataBadge } from '@/components/shared/MetadataBadge';
import { InvestigationMessages } from './InvestigationMessages';
import { InvestigationResourceTopology } from './InvestigationResourceTopology';
import { renderMarkdown } from '@/components/AI/aiChatUtils';
import { notificationStore } from '@/stores/notifications';
import { aiIntelligenceStore } from '@/stores/aiIntelligence';
import type { InvestigationRecord } from '@/api/ai';
import RefreshCwIcon from 'lucide-solid/icons/refresh-cw';

const INVESTIGATION_BADGE_PROPS = {
  appearance: 'outline',
  size: 'xs',
  shape: 'rounded',
} as const;

const summaryClass =
  'text-sm prose prose-slate prose-sm dark:prose-invert max-w-none wrap-break-word prose-headings:my-2 prose-p:my-2 prose-pre:overflow-x-auto prose-code:break-all prose-code:before:content-none prose-code:after:content-none';

// A run cut short by a restart did not fail: Patrol runs it again. A run that
// stopped after planning a fix left that fix waiting for approval. Either way,
// its note reads as information rather than as an error.
const runNoteClass = (outcome: string | undefined): string =>
  outcome === 'interrupted' || outcome === 'fix_queued'
    ? 'border-border-subtle bg-surface text-muted'
    : 'border-red-200 bg-red-50 text-red-700 dark:border-red-800 dark:bg-red-900/25 dark:text-red-300';

interface InvestigationSectionProps {
  findingId: string;
  resourceId?: string;
  investigationStatus?: string;
  investigationOutcome?: string;
  investigationAttempts?: number;
  investigationRecord?: InvestigationRecord;
  canReinvestigate?: boolean;
  investigationReady?: boolean;
  investigationReadinessMessage?: string;
}

export const InvestigationSection: Component<InvestigationSectionProps> = (props) => {
  const [showThread, setShowThread] = createSignal(false);
  const [showInfrastructureContext, setShowInfrastructureContext] = createSignal(false);
  const [reinvestigating, setReinvestigating] = createSignal(false);
  const [cooldownUntil, setCooldownUntil] = createSignal(0);
  let lastTerminalRefreshKey = '';
  const investigationRecord = createMemo(() =>
    buildPatrolInvestigationRecordPresentation(props.investigationRecord),
  );
  const investigationResourceId = createMemo(
    () => props.investigationRecord?.subject.resource_id?.trim() || props.resourceId?.trim() || '',
  );

  const [investigation, { refetch }] = createResource(
    () => props.findingId,
    async (findingId) => {
      try {
        return await getInvestigation(findingId);
      } catch {
        return null;
      }
    },
  );
  const hasInvestigationContext = createMemo(
    () => Boolean(investigation()) || investigationRecord().hasRecord,
  );
  const investigationSectionState = createMemo(() =>
    getInvestigationSectionState(investigation.loading, hasInvestigationContext()),
  );

  createEffect(() => {
    const current = investigation();
    if (!current || current.status === 'running' || current.status === 'pending') return;
    const refreshKey = `${current.session_id}:${current.status}:${current.outcome ?? ''}`;
    if (refreshKey === lastTerminalRefreshKey) return;
    lastTerminalRefreshKey = refreshKey;
    void Promise.all([
      aiIntelligenceStore.loadFindings(),
      aiIntelligenceStore.loadPatrolFindings(),
    ]);
  });

  // Auto-poll while investigation is active
  createEffect(() => {
    const inv = investigation();
    const isActive =
      inv?.status === 'running' ||
      inv?.status === 'pending' ||
      props.investigationStatus === 'running';
    if (!isActive) return;

    const interval = setInterval(() => refetch(), 5000);
    onCleanup(() => clearInterval(interval));
  });

  const canReinvestigate = () => {
    if (props.canReinvestigate === false) return false;
    if (Date.now() < cooldownUntil()) return false;
    const inv = investigation();
    if (!inv) return true; // No investigation yet
    if (inv.status === 'running') return false;
    // A queued fix is still open, even after a run that failed later.
    // Investigating again would plan a second action for the same problem.
    if (inv.outcome === 'fix_queued') return false;
    return (
      inv.status === 'failed' ||
      inv.status === 'needs_attention' ||
      inv.outcome === 'cannot_fix' ||
      inv.outcome === 'timed_out' ||
      inv.outcome === 'unclassified' ||
      inv.outcome === 'interrupted' ||
      inv.outcome === 'fix_rejected' ||
      inv.outcome === 'fix_verification_failed' ||
      inv.outcome === 'fix_verification_unknown' ||
      inv.outcome === 'fix_failed'
    );
  };

  const handleReinvestigate = async (e: Event) => {
    e.stopPropagation();
    setReinvestigating(true);
    try {
      await reinvestigateFinding(props.findingId);
      notificationStore.success('Re-investigation started');
      setCooldownUntil(Date.now() + 60000);
      refetch();
      aiIntelligenceStore.loadFindings();
    } catch (err) {
      notificationStore.error((err as Error).message || 'Failed to start re-investigation');
    } finally {
      setReinvestigating(false);
    }
  };

  return (
    <div class="mt-3 pt-3 border-t border-border-subtle">
      {/* Header; wraps so a long outcome label never squeezes the action */}
      <div class="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 mb-2">
        <div class="flex items-center gap-2">
          <span class="text-sm font-medium text-base-content">Investigation</span>
          {/* Show outcome badge when available, otherwise show status badge */}
          <Show
            when={investigation()?.outcome}
            fallback={
              <Show when={investigation()?.status}>
                <MetadataBadge
                  {...INVESTIGATION_BADGE_PROPS}
                  tone={getInvestigationStatusBadgeTone(investigation()!.status)}
                >
                  {getInvestigationStatusLabel(investigation()!.status)}
                </MetadataBadge>
              </Show>
            }
          >
            <MetadataBadge
              {...INVESTIGATION_BADGE_PROPS}
              tone={getInvestigationOutcomeBadgeTone(investigation()!.outcome!)}
            >
              {getInvestigationOutcomeLabel(investigation()!.outcome!)}
            </MetadataBadge>
          </Show>
          <Show when={props.investigationAttempts && props.investigationAttempts > 1}>
            <span class="text-[10px] text-muted">attempt {props.investigationAttempts}</span>
          </Show>
        </div>
        <Show when={canReinvestigate()}>
          <button
            type="button"
            onClick={handleReinvestigate}
            disabled={reinvestigating() || props.investigationReady === false}
            class="flex items-center gap-1 whitespace-nowrap px-2 py-1 text-xs font-medium text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/25 rounded-sm transition-colors disabled:opacity-50"
            title={
              props.investigationReady === false
                ? (props.investigationReadinessMessage ??
                  'Verify Sindri gateway and model readiness before investigating')
                : 'Reinvestigate this finding with Sindri'
            }
          >
            <RefreshCwIcon class={`w-3 h-3 ${reinvestigating() ? 'animate-spin' : ''}`} />
            Reinvestigate with Sindri
          </button>
        </Show>
        <Show when={!canReinvestigate() && Date.now() < cooldownUntil()}>
          <span class="text-xs text-muted">Re-investigation started</span>
        </Show>
      </div>

      {/* Loading */}
      <Show when={!investigationSectionState().empty && investigationSectionState().text}>
        <div class="flex items-center gap-2 text-xs text-muted py-2">
          <LoadingSpinner size="sm" />
          {investigationSectionState().text}
        </div>
      </Show>

      {/* No investigation data */}
      <Show when={investigationSectionState().empty}>
        <p class="text-xs text-muted py-1">{investigationSectionState().text}</p>
      </Show>

      <Show when={investigationRecord().hasRecord}>
        <div class="mb-2 rounded-sm border border-border bg-surface-alt p-2">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-xs font-medium text-base-content">Patrol record</span>
            {/* As in the header, the outcome replaces the run status once there is
                one: an interrupted run's status is failed, which it is not. */}
            <Show
              when={investigationRecord().outcomeLabel}
              fallback={
                <MetadataBadge
                  {...INVESTIGATION_BADGE_PROPS}
                  tone={getInvestigationStatusBadgeTone(props.investigationRecord!.status)}
                >
                  {investigationRecord().statusLabel}
                </MetadataBadge>
              }
            >
              <MetadataBadge
                {...INVESTIGATION_BADGE_PROPS}
                tone={getInvestigationOutcomeBadgeTone(props.investigationRecord!.outcome ?? '')}
              >
                {investigationRecord().outcomeLabel}
              </MetadataBadge>
            </Show>
            <Show when={investigationRecord().confidenceLabel}>
              <MetadataBadge
                {...INVESTIGATION_BADGE_PROPS}
                tone={getInvestigationConfidenceBadgeTone(
                  props.investigationRecord!.confidence ?? '',
                )}
              >
                {investigationRecord().confidenceLabel}
              </MetadataBadge>
            </Show>
          </div>

          <Show
            when={
              props.investigationRecord?.trigger.title ||
              props.investigationRecord?.trigger.detected_at
            }
          >
            <p class="mt-2 text-xs text-muted">
              <span class="font-medium text-base-content">Trigger:</span>{' '}
              {props.investigationRecord?.trigger.title || 'Finding investigation'}
              <Show when={props.investigationRecord?.trigger.detected_at}>
                {' · detected '}
                {formatTimestamp(props.investigationRecord!.trigger.detected_at)}
              </Show>
            </p>
          </Show>

          <Show when={investigationRecord().conclusion}>
            <div class="mt-2 text-[10px] font-medium uppercase text-muted">
              Diagnostic interpretation
            </div>
            <p class="mt-1 text-[11px] text-muted">
              AI-authored hypothesis. Compare it with observed evidence before treating it as a
              cause.
            </p>
            <div
              class={`mt-2 ${summaryClass}`}
              // eslint-disable-next-line solid/no-innerhtml
              innerHTML={renderMarkdown(
                labelDiagnosticInterpretation(investigationRecord().conclusion),
              )}
            />
          </Show>
          <Show
            when={
              !investigationRecord().conclusion &&
              props.investigationRecord?.status !== 'running' &&
              props.investigationRecord?.status !== 'pending'
            }
          >
            <p class="mt-2 text-xs italic text-muted" role="status">
              No diagnostic interpretation was produced for this run.
            </p>
          </Show>
          <Show when={investigationRecord().recommendedAction}>
            <p class="mt-1 text-xs text-muted">
              <span class="font-medium text-base-content">Recommended action:</span>{' '}
              {investigationRecord().recommendedAction}
            </p>
          </Show>

          <Show when={investigationRecord().proposedFix}>
            {(fix) => (
              <div class="mt-2 rounded-sm border border-border-subtle bg-surface p-2 text-xs">
                <div class="mb-1 text-[10px] font-medium uppercase text-muted">Proposed action</div>
                <div class="font-medium text-base-content">{fix().description}</div>
                <div class="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-muted">
                  <Show when={fix().riskLabel}>
                    <span>{fix().riskLabel} risk</span>
                  </Show>
                  <Show when={fix().targetHost}>
                    <span>target {fix().targetHost}</span>
                  </Show>
                  <Show when={fix().commandSummary}>
                    <span>{fix().commandSummary}</span>
                  </Show>
                  <Show when={fix().destructive}>
                    <span class="text-amber-700 dark:text-amber-300">destructive</span>
                  </Show>
                </div>
                <Show when={fix().rationale}>
                  <p class="mt-1 text-muted">{fix().rationale}</p>
                </Show>
              </div>
            )}
          </Show>

          <Show when={investigationRecord().hasRecord}>
            <div class="mt-2">
              <div class="text-[10px] font-medium uppercase text-muted">Impact</div>
              <p
                class="mt-1 text-xs"
                classList={{
                  'text-muted': Boolean(investigationRecord().impact),
                  'italic ': !investigationRecord().impact,
                }}
              >
                {investigationRecord().impact || 'Impact not assessed'}
              </p>
            </div>
          </Show>

          <Show when={investigationRecord().hasRecord}>
            <div class="mt-2">
              <div class="text-[10px] font-medium uppercase text-muted">Observed evidence</div>
              <Show
                when={investigationRecord().evidenceSummaries.length > 0}
                fallback={
                  <p class="mt-1 text-xs italic text-muted">
                    {props.investigationRecord?.status === 'running' ||
                    props.investigationRecord?.status === 'pending'
                      ? 'No evidence has been recorded yet.'
                      : 'No useful evidence was recorded during this run.'}
                  </p>
                }
              >
                <ul class="mt-1 space-y-1 text-xs text-muted">
                  <For each={investigationRecord().evidenceSummaries}>
                    {(item) => <li>{item}</li>}
                  </For>
                </ul>
              </Show>
            </div>
          </Show>

          <Show when={investigationRecord().verificationSummaries.length > 0}>
            <div class="mt-2">
              <div class="text-[10px] font-medium uppercase text-muted">Verification</div>
              <ul class="mt-1 space-y-1 text-xs text-muted">
                <For each={investigationRecord().verificationSummaries}>
                  {(item) => <li>{item}</li>}
                </For>
              </ul>
            </div>
          </Show>

          <Show when={investigationRecord().hasRecord}>
            <div class="mt-2">
              <div class="text-[10px] font-medium uppercase text-muted">Rollback</div>
              <Show
                when={investigationRecord().rollbackSummaries.length > 0}
                fallback={<p class="mt-1 text-xs italic">Rollback not specified</p>}
              >
                <ul class="mt-1 space-y-1 text-xs text-muted">
                  <For each={investigationRecord().rollbackSummaries}>
                    {(item) => <li>{item}</li>}
                  </For>
                </ul>
              </Show>
            </div>
          </Show>

          <Show when={investigationRecord().toolsUsed.length > 0}>
            <div class="mt-2 flex flex-wrap gap-1">
              <For each={investigationRecord().toolsUsed}>
                {(tool) => (
                  <MetadataBadge tone="muted" size="xs" shape="rounded">
                    {tool}
                  </MetadataBadge>
                )}
              </For>
            </div>
          </Show>

          <Show when={investigationRecord().error}>
            <div
              class={`mt-2 rounded-sm border p-2 text-xs ${runNoteClass(props.investigationRecord?.outcome)}`}
            >
              {investigationRecord().error}
            </div>
          </Show>
        </div>
      </Show>

      <Show when={investigationResourceId()}>
        <details
          class="rounded-sm border border-border-subtle bg-surface-alt px-2 py-1.5"
          onToggle={(event) => setShowInfrastructureContext(event.currentTarget.open)}
        >
          <summary class="cursor-pointer text-xs font-medium text-base-content">
            Infrastructure context
          </summary>
          <Show when={showInfrastructureContext()}>
            <InvestigationResourceTopology resourceId={investigationResourceId()} />
          </Show>
        </details>
      </Show>

      {/* Investigation details */}
      <Show when={!investigation.loading && investigation()}>
        {(inv) => (
          <div class="space-y-2">
            <Show when={inv().status === 'running' || inv().status === 'pending'}>
              <p class="text-xs text-muted" role="status" aria-live="polite">
                Collecting read-only observations · {inv().evidence_call_count ?? 0} evidence reads
                · {inv().tool_call_count ?? 0} tool calls
              </p>
            </Show>
            {/* Error message for failed investigations */}
            <Show
              when={
                inv().error &&
                inv().error?.trim() !== investigationRecord().error &&
                (inv().status === 'failed' ||
                  inv().outcome === 'timed_out' ||
                  inv().outcome === 'interrupted' ||
                  inv().outcome === 'unclassified' ||
                  inv().outcome === 'fix_failed' ||
                  inv().outcome === 'fix_rejected' ||
                  inv().outcome === 'fix_verification_failed' ||
                  inv().outcome === 'fix_verification_unknown' ||
                  inv().outcome === 'needs_attention' ||
                  inv().outcome === 'cannot_fix')
              }
            >
              <div class={`rounded-sm border p-2 text-xs ${runNoteClass(inv().outcome)}`}>
                {inv().error}
              </div>
            </Show>

            {/* Summary */}
            <Show
              when={
                inv().summary?.trim() && inv().summary?.trim() !== investigationRecord().conclusion
              }
            >
              <div class="rounded-sm bg-surface-alt p-2">
                <div class="mb-1 text-[10px] font-medium uppercase text-muted">
                  Diagnostic interpretation
                </div>
                <p class="mb-2 text-[11px] text-muted">
                  AI-authored hypothesis. Compare it with observed evidence before treating it as a
                  cause.
                </p>
                <div
                  class={summaryClass}
                  // eslint-disable-next-line solid/no-innerhtml
                  innerHTML={renderMarkdown(labelDiagnosticInterpretation(inv().summary))}
                />
              </div>
            </Show>

            {/* Tools used + turn count */}
            <div class="flex items-center gap-2 flex-wrap">
              <Show when={inv().tools_used && inv().tools_used!.length > 0}>
                <div class="flex items-center gap-1 flex-wrap">
                  <For each={inv().tools_used}>
                    {(tool) => (
                      <MetadataBadge tone="neutral" size="xs" shape="rounded">
                        {tool}
                      </MetadataBadge>
                    )}
                  </For>
                </div>
              </Show>
              <span class="text-[10px] text-muted">
                {inv().turn_count} turn{inv().turn_count === 1 ? '' : 's'}
              </span>
              <Show when={inv().started_at}>
                <span class="text-[10px] text-muted">
                  started {formatTimestamp(inv().started_at)}
                </span>
              </Show>
            </div>

            {/* Show thread toggle */}
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setShowThread(!showThread());
              }}
              class="text-xs text-blue-600 dark:text-blue-400 hover:underline flex items-center gap-1"
            >
              {showThread() ? 'Hide investigation thread' : 'Show investigation thread'}
              <svg
                class={`w-3 h-3 transition-transform ${showThread() ? 'rotate-90' : ''}`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 5l7 7-7 7"
                />
              </svg>
            </button>

            {/* Investigation messages (lazy loaded) */}
            <Show when={showThread()}>
              <InvestigationMessages findingId={props.findingId} />
            </Show>
          </div>
        )}
      </Show>
    </div>
  );
};

export default InvestigationSection;
