import { createMemo, For, Show, type Accessor } from 'solid-js';
import { ButtonLink } from '@/components/shared/Button';
import { MetadataBadge } from '@/components/shared/MetadataBadge';
import { settingsTabPath } from '@/components/Settings/settingsNavigationModel';
import type { AttentionSummary } from '@/api/patrolAttention';
import type { PatrolRunRecord, PatrolStatus } from '@/api/patrol';
import type { UnifiedFinding } from '@/stores/aiIntelligence';
import type { CodexLBGatewayStatus } from '@/types/ai';
import { formatRelativeTime } from '@/utils/format';
import { buildPatrolSindriOverviewModel } from './patrolSindriOverviewModel';

interface PatrolSindriOverviewProps {
  patrolStatus: Accessor<PatrolStatus | null | undefined>;
  patrolEnabled: Accessor<boolean>;
  gatewayStatus: Accessor<CodexLBGatewayStatus | null | undefined>;
  settingsLoaded: Accessor<boolean>;
  findings: Accessor<readonly UnifiedFinding[] | null | undefined>;
  attentionSummary: Accessor<AttentionSummary | null | undefined>;
  pendingActionCount: Accessor<number | null | undefined>;
  lastCompletedRun: Accessor<PatrolRunRecord | null | undefined>;
}

const readinessTone = (ready: boolean): 'success' | 'warning' => (ready ? 'success' : 'warning');

const relativeTime = (value?: string, fallback = 'Not available'): string => {
  if (!value || !Number.isFinite(Date.parse(value))) return fallback;
  return formatRelativeTime(value, { compact: true });
};

export function PatrolSindriOverview(props: PatrolSindriOverviewProps) {
  const model = createMemo(() =>
    buildPatrolSindriOverviewModel({
      patrolEnabled: props.patrolEnabled(),
      status: props.patrolStatus(),
      settingsLoaded: props.settingsLoaded(),
      gateway: props.gatewayStatus(),
      findings: props.findings(),
      attentionSummary: props.attentionSummary(),
      pendingActionCount: props.pendingActionCount(),
      lastCompletedRun: props.lastCompletedRun(),
    }),
  );
  const status = () => props.patrolStatus();

  return (
    <section
      aria-labelledby="patrol-sindri-overview-title"
      data-testid="patrol-sindri-overview"
      class="overflow-hidden rounded-lg border border-border bg-surface"
    >
      <div class="border-b border-border px-4 py-4 sm:px-5">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 id="patrol-sindri-overview-title" class="text-base font-semibold text-base-content">
              Sindri operations overview
            </h2>
            <p class="mt-1 text-sm leading-5 text-muted">
              Deterministic monitoring and AI investigation readiness are reported separately.
            </p>
          </div>
          <ButtonLink href="/patrol/activity" variant="secondary" size="sm">
            Recent activity
          </ButtonLink>
        </div>
      </div>

      <div class="space-y-4 p-4 sm:p-5">
        <Show when={model().showMonitoringWithoutAIMessage}>
          <p
            role="status"
            class="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm font-medium text-amber-900 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200"
          >
            Monitoring operational — AI investigation unavailable.
          </p>
        </Show>

        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <article class="rounded-md border border-border-subtle p-3">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <h3 class="text-sm font-semibold text-base-content">Deterministic monitoring</h3>
              <MetadataBadge
                tone={model().monitoring.operational ? 'success' : 'warning'}
                size="xs"
                shape="rounded"
              >
                {model().monitoring.label}
              </MetadataBadge>
            </div>
            <p class="mt-2 text-xs leading-5 text-muted">
              {status()?.running
                ? 'A Patrol run is in progress.'
                : !status()?.enabled || !props.patrolEnabled()
                  ? 'Patrol scheduling is paused.'
                  : (status()?.interval_ms ?? 0) > 0
                    ? `Scheduled every ${Math.round((status()?.interval_ms ?? 0) / 60000)} minutes.`
                    : 'No recurring Patrol interval is scheduled.'}
            </p>
            <p class="mt-1 text-xs leading-5 text-muted">
              Next run: {relativeTime(model().nextPatrolAt, 'Not scheduled')}
            </p>
            <p class="mt-1 text-xs leading-5 text-muted">
              Last completed run: {relativeTime(model().lastCompletedAt, 'No completed run yet')}
            </p>
            <Show when={model().monitoring.detail}>
              <p class="mt-1 text-xs leading-5 text-amber-700 dark:text-amber-300">
                {model().monitoring.detail}
              </p>
            </Show>
            <div class="mt-3 border-t border-border-subtle pt-2">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <h4 class="text-xs font-semibold text-base-content">Infrastructure health</h4>
                <MetadataBadge
                  tone={model().infrastructure.healthy ? 'success' : 'warning'}
                  size="xs"
                  shape="rounded"
                >
                  {model().infrastructure.label}
                </MetadataBadge>
              </div>
              <p class="mt-1 text-xs leading-5 text-muted">{model().infrastructure.detail}</p>
              <Show when={status() && !status()!.healthy}>
                <p class="mt-1 text-xs leading-5 text-muted">
                  Infrastructure findings do not mean the Patrol monitoring engine is degraded.
                </p>
              </Show>
            </div>
          </article>

          <article class="rounded-md border border-border-subtle p-3">
            <h3 class="text-sm font-semibold text-base-content">Current work</h3>
            <p class="mt-2 text-sm font-medium text-base-content">
              {model().activeFindingCount === null
                ? 'Findings not loaded'
                : `${model().activeFindingCount} active ${model().activeFindingCount === 1 ? 'finding' : 'findings'}`}
            </p>
            <p class="mt-1 text-xs leading-5 text-muted">
              {model().activeInvestigationCount === null
                ? 'Investigation count unavailable'
                : `${model().activeInvestigationCount} active ${model().activeInvestigationCount === 1 ? 'investigation' : 'investigations'}`}
            </p>
            <div class="mt-2 flex flex-wrap gap-1.5" aria-label="Active finding severity">
              <Show when={model().severityCounts}>
                {(counts) => (
                  <For each={Object.entries(counts())}>
                    {([severity, count]) => (
                      <span class="rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-muted">
                        {severity[0].toUpperCase() + severity.slice(1)} {count}
                      </span>
                    )}
                  </For>
                )}
              </Show>
            </div>
          </article>

          <article class="rounded-md border border-border-subtle p-3">
            <h3 class="text-sm font-semibold text-base-content">Active incidents</h3>
            <p class="mt-2 text-xl font-semibold tabular-nums text-base-content">
              {model().activeIncidentCount === null ? '—' : model().activeIncidentCount}
            </p>
            <p class="mt-1 text-xs leading-5 text-muted">
              {model().activeIncidentCount === null
                ? 'Incident count unavailable'
                : 'From the current Patrol attention summary.'}
            </p>
            <Show when={props.attentionSummary()?.coverageState === 'partial'}>
              <p class="mt-1 text-xs text-amber-700 dark:text-amber-300">
                Incident coverage is partial.
              </p>
            </Show>
          </article>

          <article class="rounded-md border border-border-subtle p-3">
            <h3 class="text-sm font-semibold text-base-content">Action approvals</h3>
            <p class="mt-2 text-xl font-semibold tabular-nums text-base-content">
              {model().pendingActionCount === null ? '—' : model().pendingActionCount}
            </p>
            <p class="mt-1 text-xs leading-5 text-muted">
              {model().pendingActionCount === null
                ? 'Action count unavailable'
                : "Actions waiting in Pulse's shared review inbox."}
            </p>
            <ButtonLink href="/actions" variant="ghost" size="sm" class="mt-2 px-0">
              Review actions
            </ButtonLink>
          </article>
        </div>

        <section
          aria-labelledby="patrol-sindri-gateway-title"
          class="rounded-md border border-border-subtle bg-surface-alt p-3 sm:p-4"
        >
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div class="flex flex-wrap items-center gap-2">
                <h3
                  id="patrol-sindri-gateway-title"
                  class="text-sm font-semibold text-base-content"
                >
                  AI investigation · Codex-LB
                </h3>
                <MetadataBadge
                  tone={readinessTone(model().gateway.ready)}
                  size="xs"
                  shape="rounded"
                >
                  {model().gateway.label}
                </MetadataBadge>
              </div>
              <p class="mt-1 text-xs text-muted">
                Selected model: {model().gateway.selectedModel || 'No model selected'}
              </p>
            </div>
            <ButtonLink href={settingsTabPath('system-ai')} variant="secondary" size="sm">
              AI gateway settings
            </ButtonLink>
          </div>

          <dl class="mt-3 grid gap-x-4 gap-y-2 text-xs sm:grid-cols-2 xl:grid-cols-4">
            <div>
              <dt class="text-muted">Endpoint</dt>
              <dd class="mt-0.5 font-medium text-base-content">
                {model().gateway.endpointConfigured ? 'Configured' : 'Not configured'}
              </dd>
            </div>
            <div>
              <dt class="text-muted">Credential</dt>
              <dd class="mt-0.5 font-medium text-base-content">
                {model().gateway.credentialConfigured ? 'Credential configured' : 'Not configured'}
              </dd>
            </div>
            <div>
              <dt class="text-muted">Connectivity</dt>
              <dd class="mt-0.5 font-medium text-base-content">{model().gateway.connectivity}</dd>
            </div>
            <div>
              <dt class="text-muted">Tool calling</dt>
              <dd class="mt-0.5 font-medium text-base-content">{model().gateway.toolCalling}</dd>
            </div>
            <div>
              <dt class="text-muted">Streaming</dt>
              <dd class="mt-0.5 font-medium text-base-content">{model().gateway.streaming}</dd>
            </div>
            <div>
              <dt class="text-muted">Last successful qualification</dt>
              <dd class="mt-0.5 font-medium text-base-content">
                {relativeTime(model().gateway.lastSuccessfulQualification, 'Not qualified')}
              </dd>
            </div>
          </dl>
        </section>
      </div>
    </section>
  );
}

export default PatrolSindriOverview;
