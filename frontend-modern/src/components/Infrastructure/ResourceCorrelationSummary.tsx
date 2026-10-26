import { For, Show, createMemo, type Component } from 'solid-js';
import type { ResourceCorrelation } from '@/types/aiIntelligence';
import type { ResourceRelationship, ResourceRelationshipContextResource } from '@/types/resource';
import { formatRelativeTime } from '@/utils/format';
import { useRelativeTimeNow } from '@/utils/relativeTimeClock';
import {
  formatResourceCorrelationEndpoint,
  formatResourceCorrelationHeadline,
  formatResourceCorrelationPattern,
  formatResourceCorrelationSummary,
  formatResourceCorrelationSummaryText,
  formatResourceRelationshipSummary,
  formatResourceRelationshipType,
  sortResourceCorrelations,
  sortResourceRelationships,
} from '@/utils/resourceCorrelationPresentation';

interface ResourceCorrelationSummaryProps {
  correlations?: readonly ResourceCorrelation[] | null;
  relationships?: readonly ResourceRelationship[] | null;
  relatedResources?: readonly ResourceRelationshipContextResource[] | null;
  dependencies?: readonly string[] | null;
  dependents?: readonly string[] | null;
  title?: string;
  summaryText?: string;
  dataTestId?: string;
  buildResourceHref?: (resourceId: string) => string | null | undefined;
  resolveResourceLabel?: (resourceId: string) => string | null | undefined;
  showLastSeen?: boolean;
  class?: string;
  maxCorrelations?: number;
}

export const ResourceCorrelationSummary: Component<ResourceCorrelationSummaryProps> = (props) => {
  const now = useRelativeTimeNow();
  const className = () => props.class?.trim() ?? '';
  const correlations = createMemo(() => sortResourceCorrelations(props.correlations ?? []));
  const relationships = createMemo(() => sortResourceRelationships(props.relationships ?? []));
  const relatedResources = () => props.relatedResources ?? [];
  const dependencies = () => props.dependencies ?? [];
  const dependents = () => props.dependents ?? [];
  const buildResourceHref = props.buildResourceHref ?? (() => null);
  const resolveResourceLabel = props.resolveResourceLabel;
  const maxCorrelations = () => props.maxCorrelations ?? 3;
  const hasContent = () =>
    relationships().length > 0 ||
    relatedResources().length > 0 ||
    dependencies().length > 0 ||
    dependents().length > 0 ||
    correlations().length > 0;
  const summaryText = () =>
    formatResourceCorrelationSummaryText({
      relationshipsCount: relationships().length,
      dependenciesCount: dependencies().length,
      dependentsCount: dependents().length,
      correlationsCount: correlations().length,
      summaryText: props.summaryText,
    });

  return (
    <Show when={hasContent()}>
      <div
        data-testid={props.dataTestId}
        class={`rounded-md border border-border-subtle bg-surface p-4 ${className()}`.trim()}
      >
        <div class="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h3 class="text-sm font-semibold text-base-content">
              {props.title ?? 'Correlation context'}
            </h3>
            <Show when={summaryText()}>
              {(summary) => <p class="mt-1 text-xs text-muted">{summary()}</p>}
            </Show>
          </div>
        </div>

        <Show when={relationships().length > 0}>
          <div class="mt-3" data-testid="resource-canonical-relationships">
            <div class="text-[9px] uppercase tracking-wide text-muted">Canonical relationships</div>
            <div class="mt-1 space-y-1.5">
              <For each={relationships().slice(0, maxCorrelations())}>
                {(relationship) => {
                  const sourceLabel =
                    resolveResourceLabel?.(relationship.sourceId)?.trim() || relationship.sourceId;
                  const targetLabel =
                    resolveResourceLabel?.(relationship.targetId)?.trim() || relationship.targetId;
                  const sourceHref = buildResourceHref(relationship.sourceId);
                  const targetHref = buildResourceHref(relationship.targetId);
                  const typeLabel = formatResourceRelationshipType(relationship);
                  const summary = formatResourceRelationshipSummary(relationship);
                  const lastSeenLabel = () =>
                    props.showLastSeen && relationship.lastSeenAt
                      ? formatRelativeTime(relationship.lastSeenAt, {
                          compact: true,
                          emptyText: 'just now',
                          now: now(),
                        })
                      : '';

                  return (
                    <div class="rounded-sm bg-surface px-2 py-1">
                      <div class="flex flex-wrap items-center gap-1 text-[10px] text-base-content">
                        {sourceHref ? (
                          <a
                            class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-blue-700 hover:underline dark:text-blue-300"
                            href={sourceHref}
                            aria-label={`Open source resource ${sourceLabel} in Infrastructure`}
                          >
                            {sourceLabel}
                          </a>
                        ) : (
                          <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px]">
                            {sourceLabel}
                          </span>
                        )}
                        <span class="text-muted">→</span>
                        {targetHref ? (
                          <a
                            class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-blue-700 hover:underline dark:text-blue-300"
                            href={targetHref}
                            aria-label={`Open target resource ${targetLabel} in Infrastructure`}
                          >
                            {targetLabel}
                          </a>
                        ) : (
                          <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px]">
                            {targetLabel}
                          </span>
                        )}
                        <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[9px] text-muted">
                          {typeLabel}
                        </span>
                      </div>
                      <Show when={summary || lastSeenLabel()}>
                        <div class="mt-0.5 text-[10px] text-muted">
                          {summary}
                          <Show when={lastSeenLabel()}>
                            <>
                              {summary ? ' · ' : ''}last seen {lastSeenLabel()}
                            </>
                          </Show>
                        </div>
                      </Show>
                    </div>
                  );
                }}
              </For>
            </div>
          </div>
        </Show>

        <Show when={relatedResources().length > 0}>
          <div class="mt-3" data-testid="resource-related-resources">
            <div class="text-[9px] uppercase tracking-wide text-muted">Related resources</div>
            <div class="mt-1 space-y-1.5">
              <For each={relatedResources()}>
                {(resource) => {
                  const label =
                    resource.canonicalIdentity?.displayName?.trim() ||
                    resource.name.trim() ||
                    resource.id;
                  const href = buildResourceHref(resource.id);
                  const lastSeen = resource.lastSeen ? Date.parse(String(resource.lastSeen)) : NaN;
                  const lastSeenLabel = () =>
                    Number.isFinite(lastSeen)
                      ? formatRelativeTime(lastSeen, {
                          compact: true,
                          emptyText: 'unknown',
                          now: now(),
                        })
                      : 'unknown';
                  const availability = resource.availability;
                  const checkState =
                    availability?.available === true
                      ? 'reachable'
                      : availability?.available === false
                        ? 'failed'
                        : 'unknown';
                  const canonical = resource.canonicalIdentity;
                  const aliases = (canonical?.aliases ?? [])
                    .filter((alias) => alias && alias !== canonical?.primaryId)
                    .slice(0, 4);

                  return (
                    <div
                      class="rounded-sm bg-surface px-2 py-1.5"
                      data-testid="resource-related-resource"
                    >
                      <div class="flex flex-wrap items-center gap-1.5 text-[10px] text-base-content">
                        {href ? (
                          <a
                            class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-blue-700 hover:underline dark:text-blue-300"
                            href={href}
                            aria-label={`Open related resource ${label}`}
                          >
                            {label}
                          </a>
                        ) : (
                          <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5">
                            {label}
                          </span>
                        )}
                        <span class="text-muted">{resource.type}</span>
                        <span class="rounded-sm bg-surface-alt px-1.5 py-0.5">
                          {resource.health?.verdict ?? resource.status}
                        </span>
                        <span class="text-muted">seen {lastSeenLabel()}</span>
                      </div>
                      <Show when={(resource.sources?.length ?? 0) > 0}>
                        <div class="mt-1 text-[10px] text-muted">
                          Providers: {(resource.sources ?? []).join(', ')}
                        </div>
                      </Show>
                      <Show when={canonical?.primaryId || aliases.length > 0}>
                        <div class="mt-1 break-all text-[10px] text-muted">
                          <Show when={canonical?.primaryId}>
                            Primary identity: {canonical?.primaryId}
                          </Show>
                          <Show when={aliases.length > 0}>
                            <Show when={canonical?.primaryId}> · </Show>
                            Provider identity links: {aliases.join(', ')}
                          </Show>
                        </div>
                      </Show>
                      <Show when={Object.keys(resource.sourceStatus ?? {}).length > 0}>
                        <div class="mt-1 flex flex-wrap gap-x-2 gap-y-0.5 text-[10px] text-muted">
                          <For each={Object.entries(resource.sourceStatus ?? {})}>
                            {([source, observation]) => (
                              <span>
                                {source}: {observation.status || 'unknown'}
                                {observation.lastSeen
                                  ? ` · seen ${formatRelativeTime(observation.lastSeen, { compact: true, emptyText: 'unknown', now: now() })}`
                                  : ''}
                              </span>
                            )}
                          </For>
                        </div>
                      </Show>
                      <Show when={availability}>
                        <div class="mt-1 text-[10px] text-muted">
                          Availability: {checkState}
                          {availability?.lastChecked
                            ? ` · checked ${formatRelativeTime(availability.lastChecked, { compact: true, emptyText: 'unknown', now: now() })}`
                            : ''}
                          {availability?.applicationStatusCode
                            ? ` · HTTP ${availability.applicationStatusCode}`
                            : ''}
                          {availability?.lastError ? ` · ${availability.lastError}` : ''}
                        </div>
                      </Show>
                      <Show when={(resource.incidentCount ?? resource.incidents?.length ?? 0) > 0}>
                        <div class="mt-1 text-[10px] text-amber-700 dark:text-amber-300">
                          Active incidents:{' '}
                          {resource.incidentSummary ||
                            resource.incidents?.map((incident) => incident.summary).join(' · ') ||
                            resource.incidentCount}
                        </div>
                      </Show>
                    </div>
                  );
                }}
              </For>
            </div>
          </div>
        </Show>

        <Show when={dependencies().length > 0}>
          <div class="mt-3">
            <div class="text-[9px] uppercase tracking-wide text-muted">Depends on</div>
            <div class="mt-1 flex flex-wrap gap-1">
              <For each={dependencies().slice(0, maxCorrelations())}>
                {(dependency) => {
                  const label = resolveResourceLabel?.(dependency)?.trim() || dependency;
                  const href = buildResourceHref(dependency);
                  return href ? (
                    <a
                      class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-blue-700 hover:underline dark:text-blue-300"
                      href={href}
                      aria-label={`Open dependency resource ${label} in Infrastructure`}
                    >
                      {label}
                    </a>
                  ) : (
                    <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px]">
                      {label}
                    </span>
                  );
                }}
              </For>
            </div>
          </div>
        </Show>

        <Show when={dependents().length > 0}>
          <div class="mt-3">
            <div class="text-[9px] uppercase tracking-wide text-muted">Used by</div>
            <div class="mt-1 flex flex-wrap gap-1">
              <For each={dependents().slice(0, maxCorrelations())}>
                {(dependent) => {
                  const label = resolveResourceLabel?.(dependent)?.trim() || dependent;
                  const href = buildResourceHref(dependent);
                  return href ? (
                    <a
                      class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-blue-700 hover:underline dark:text-blue-300"
                      href={href}
                      aria-label={`Open dependent resource ${label} in Infrastructure`}
                    >
                      {label}
                    </a>
                  ) : (
                    <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px]">
                      {label}
                    </span>
                  );
                }}
              </For>
            </div>
          </div>
        </Show>

        <Show when={correlations().length > 0}>
          <div class="mt-3">
            <div class="text-[9px] uppercase tracking-wide text-muted">Correlations</div>
            <div class="mt-1 space-y-1.5">
              <For each={correlations().slice(0, maxCorrelations())}>
                {(correlation) => {
                  const sourceHref = buildResourceHref(correlation.source_id);
                  const targetHref = buildResourceHref(correlation.target_id);
                  const headline = formatResourceCorrelationHeadline(correlation);
                  const summary = formatResourceCorrelationSummary(correlation);
                  const sourceLabel = formatResourceCorrelationEndpoint(correlation, 'source');
                  const targetLabel = formatResourceCorrelationEndpoint(correlation, 'target');
                  const patternLabel = formatResourceCorrelationPattern(correlation);
                  const lastSeenLabel = () =>
                    props.showLastSeen && correlation.last_seen
                      ? formatRelativeTime(correlation.last_seen, {
                          compact: true,
                          emptyText: 'just now',
                          now: now(),
                        })
                      : '';
                  return (
                    <div class="rounded-sm bg-surface px-2 py-1" title={headline}>
                      <div class="flex flex-wrap items-center gap-1 text-[10px] text-base-content">
                        {sourceHref ? (
                          <a
                            class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-blue-700 hover:underline dark:text-blue-300"
                            href={sourceHref}
                            aria-label={`Open source resource ${sourceLabel} in Infrastructure`}
                          >
                            {sourceLabel}
                          </a>
                        ) : (
                          <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px]">
                            {sourceLabel}
                          </span>
                        )}
                        <span class="text-muted">→</span>
                        {targetHref ? (
                          <a
                            class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-blue-700 hover:underline dark:text-blue-300"
                            href={targetHref}
                            aria-label={`Open target resource ${targetLabel} in Infrastructure`}
                          >
                            {targetLabel}
                          </a>
                        ) : (
                          <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px]">
                            {targetLabel}
                          </span>
                        )}
                        <span class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[9px] text-muted">
                          {patternLabel}
                        </span>
                      </div>
                      <div class="mt-0.5 text-[10px] text-muted">
                        {summary}
                        <Show when={lastSeenLabel()}>
                          <> · last seen {lastSeenLabel()}</>
                        </Show>
                      </div>
                      <Show when={correlation.description}>
                        {(description) => (
                          <div class="mt-0.5 text-[10px] text-muted">{description()}</div>
                        )}
                      </Show>
                    </div>
                  );
                }}
              </For>
            </div>
          </div>
        </Show>
      </div>
    </Show>
  );
};

export default ResourceCorrelationSummary;
