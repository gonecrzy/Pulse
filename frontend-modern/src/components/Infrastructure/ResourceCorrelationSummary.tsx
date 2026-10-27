import { For, Show, createMemo, createSignal, type Component } from 'solid-js';
import type { ResourceCorrelation } from '@/types/aiIntelligence';
import type {
  ResourceRelationship,
  ResourceRelationshipContextEdge,
  ResourceRelationshipContextResource,
} from '@/types/resource';
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
  selectedResourceId?: string;
  correlations?: readonly ResourceCorrelation[] | null;
  relationships?: readonly ResourceRelationship[] | null;
  relatedResources?: readonly ResourceRelationshipContextResource[] | null;
  contextResources?: readonly ResourceRelationshipContextResource[] | null;
  serviceMonitoringRelationships?: readonly ResourceRelationshipContextEdge[] | null;
  infrastructureRelationships?: readonly ResourceRelationshipContextEdge[] | null;
  crossBranchRelationships?: readonly ResourceRelationshipContextEdge[] | null;
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

interface ResourceTopologyNode {
  resource: ResourceRelationshipContextResource;
  children: ResourceTopologyNode[];
}

const resourceTopologyTypeLabel = (resource: ResourceRelationshipContextResource): string => {
  const sources = resource.sources ?? [];
  if (resource.type === 'agent' && sources.includes('proxmox')) return 'Proxmox physical host';
  if (resource.type === 'vm' && sources.includes('proxmox') && sources.includes('komodo')) {
    return 'VM · Proxmox + Komodo Server';
  }
  if (resource.type === 'vm' && sources.includes('proxmox')) return 'Proxmox VM';
  if (resource.type === 'network-endpoint') return 'availability check';
  return resource.type.replace(/[-_]/g, ' ');
};

const buildResourceTopologyTree = (
  selectedResourceId: string | undefined,
  resources: readonly ResourceRelationshipContextResource[],
  directRelationships: readonly ResourceRelationship[],
  serviceRelationships: readonly ResourceRelationshipContextEdge[],
): ResourceTopologyNode[] => {
  if (!selectedResourceId || resources.length === 0) return [];

  const resourcesById = new Map(resources.map((resource) => [resource.id, resource]));
  const parentById = new Map<string, string>();
  for (const resource of resources) {
    if (resource.parentId && resourcesById.has(resource.parentId)) {
      parentById.set(resource.id, resource.parentId);
    }
  }
  const monitoringEdges = [
    ...directRelationships,
    ...serviceRelationships.map((edge) => edge.relationship),
  ];
  for (const relationship of monitoringEdges) {
    if (relationship.type !== 'checks') continue;
    const source = resourcesById.get(relationship.sourceId);
    const target = resourcesById.get(relationship.targetId);
    const check = source?.type === 'network-endpoint' ? source : target;
    const service = check?.id === source?.id ? target : source;
    if (check && service && !parentById.has(check.id)) {
      parentById.set(check.id, service.id);
    }
  }

  const selected = resourcesById.get(selectedResourceId);
  if (!selected) return [];
  let rootId = selected.id;
  const ancestorIds = new Set<string>([rootId]);
  while (parentById.has(rootId)) {
    const parentId = parentById.get(rootId)!;
    if (ancestorIds.has(parentId)) break;
    ancestorIds.add(parentId);
    rootId = parentId;
  }

  const nodesById = new Map(
    resources.map((resource) => [
      resource.id,
      { resource, children: [] as ResourceTopologyNode[] },
    ]),
  );
  const roots: ResourceTopologyNode[] = [];
  for (const resource of resources) {
    const node = nodesById.get(resource.id)!;
    const parentId = parentById.get(resource.id);
    const parent = parentId ? nodesById.get(parentId) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }

  const sortTree = (node: ResourceTopologyNode) => {
    node.children.sort(
      (left, right) =>
        left.resource.name.localeCompare(right.resource.name) ||
        left.resource.id.localeCompare(right.resource.id),
    );
    node.children.forEach(sortTree);
  };
  roots.forEach(sortTree);
  const selectedHierarchyRoot = nodesById.get(rootId);
  return selectedHierarchyRoot
    ? [selectedHierarchyRoot]
    : roots.filter((node) => node.resource.id === selected.id);
};

interface ResourceTopologyTreeEntryProps {
  node: ResourceTopologyNode;
  buildResourceHref: (resourceId: string) => string | null | undefined;
  expandedNodeIds: () => Set<string>;
  setNodeExpanded: (resourceId: string, expanded: boolean) => void;
}

const ResourceTopologyTreeEntry: Component<ResourceTopologyTreeEntryProps> = (props) => {
  const resource = () => props.node.resource;
  const label = () =>
    resource().canonicalIdentity?.displayName?.trim() || resource().name.trim() || resource().id;
  const health = () => resource().health?.verdict ?? resource().status ?? 'unknown';
  const incidentCount = () => resource().incidentCount ?? resource().incidents?.length ?? 0;
  const staleSources = () =>
    Object.entries(resource().sourceStatus ?? {})
      .filter(([, observation]) => observation.status === 'stale')
      .map(([source]) => source);
  const incidentSummary = () =>
    resource().incidentSummary ||
    resource()
      .incidents?.map((incident) => incident.summary)
      .join(' · ');
  const href = () => props.buildResourceHref(resource().id);

  return (
    <li class="min-w-0" data-testid={`resource-topology-tree-entry-${resource().id}`}>
      <Show
        when={props.node.children.length > 0}
        fallback={
          <div class="flex flex-wrap items-center gap-1.5 rounded-sm bg-surface px-2 py-1.5 text-[10px]">
            <Show
              when={href()}
              fallback={<span class="font-medium text-base-content">{label()}</span>}
            >
              {(resourceHref) => (
                <a
                  class="font-medium text-blue-700 hover:underline dark:text-blue-300"
                  href={resourceHref()}
                  aria-label={`Open topology resource ${label()}`}
                >
                  {label()}
                </a>
              )}
            </Show>
            <span class="text-muted">{resourceTopologyTypeLabel(resource())}</span>
            <span class="rounded-sm bg-surface-alt px-1.5 py-0.5">{health()}</span>
            <span class="text-muted">
              {incidentCount()} active {incidentCount() === 1 ? 'incident' : 'incidents'}
            </span>
            <Show when={staleSources().length > 0}>
              <span class="text-amber-700 dark:text-amber-300">
                Stale observations: {staleSources().join(', ')}
              </span>
            </Show>
            <Show when={incidentSummary()}>
              {(summary) => (
                <span class="text-amber-700 dark:text-amber-300">Observed: {summary()}</span>
              )}
            </Show>
          </div>
        }
      >
        <details
          data-testid={`resource-topology-tree-node-${resource().id}`}
          onToggle={(event) => props.setNodeExpanded(resource().id, event.currentTarget.open)}
        >
          <summary class="flex cursor-pointer flex-wrap items-center gap-1.5 rounded-sm bg-surface px-2 py-1.5 text-[10px]">
            <Show
              when={href()}
              fallback={<span class="font-medium text-base-content">{label()}</span>}
            >
              {(resourceHref) => (
                <a
                  class="font-medium text-blue-700 hover:underline dark:text-blue-300"
                  href={resourceHref()}
                  aria-label={`Open topology resource ${label()}`}
                  onClick={(event) => event.stopPropagation()}
                >
                  {label()}
                </a>
              )}
            </Show>
            <span class="text-muted">{resourceTopologyTypeLabel(resource())}</span>
            <span class="rounded-sm bg-surface-alt px-1.5 py-0.5">{health()}</span>
            <span class="text-muted">{props.node.children.length} children</span>
            <span class="text-muted">
              {incidentCount()} active {incidentCount() === 1 ? 'incident' : 'incidents'}
            </span>
            <Show when={staleSources().length > 0}>
              <span class="text-amber-700 dark:text-amber-300">
                Stale observations: {staleSources().join(', ')}
              </span>
            </Show>
          </summary>
          <Show when={props.expandedNodeIds().has(resource().id)}>
            <ul class="ml-3 mt-1 space-y-1 border-l border-border-subtle pl-2">
              <For each={props.node.children}>
                {(child) => (
                  <ResourceTopologyTreeEntry
                    node={child}
                    buildResourceHref={props.buildResourceHref}
                    expandedNodeIds={props.expandedNodeIds}
                    setNodeExpanded={props.setNodeExpanded}
                  />
                )}
              </For>
            </ul>
          </Show>
        </details>
      </Show>
    </li>
  );
};

export const ResourceCorrelationSummary: Component<ResourceCorrelationSummaryProps> = (props) => {
  const now = useRelativeTimeNow();
  const [showInfrastructureContext, setShowInfrastructureContext] = createSignal(false);
  const [showCrossBranchContext, setShowCrossBranchContext] = createSignal(false);
  const [expandedTopologyNodeIds, setExpandedTopologyNodeIds] = createSignal(new Set<string>());
  const className = () => props.class?.trim() ?? '';
  const correlations = createMemo(() => sortResourceCorrelations(props.correlations ?? []));
  const relationships = createMemo(() =>
    sortResourceRelationships(props.relationships ?? [], props.selectedResourceId),
  );
  const relatedResources = () => props.relatedResources ?? [];
  const contextResources = () => props.contextResources ?? relatedResources();
  const serviceMonitoringRelationships = () => props.serviceMonitoringRelationships ?? [];
  const infrastructureRelationships = () => props.infrastructureRelationships ?? [];
  const crossBranchRelationships = () => props.crossBranchRelationships ?? [];
  const topologyTree = createMemo(() =>
    buildResourceTopologyTree(
      props.selectedResourceId,
      contextResources(),
      relationships(),
      serviceMonitoringRelationships(),
    ),
  );
  const setTopologyNodeExpanded = (resourceId: string, expanded: boolean) => {
    setExpandedTopologyNodeIds((current) => {
      const next = new Set(current);
      if (expanded) next.add(resourceId);
      else next.delete(resourceId);
      return next;
    });
  };
  const dependencies = () => props.dependencies ?? [];
  const dependents = () => props.dependents ?? [];
  const buildResourceHref = props.buildResourceHref ?? (() => null);
  const resolveResourceLabel = props.resolveResourceLabel;
  const maxCorrelations = () => props.maxCorrelations ?? 3;
  const hasContent = () =>
    relationships().length > 0 ||
    topologyTree().length > 0 ||
    relatedResources().length > 0 ||
    serviceMonitoringRelationships().length > 0 ||
    infrastructureRelationships().length > 0 ||
    crossBranchRelationships().length > 0 ||
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
  const contextResource = (resourceId: string) =>
    contextResources().find((resource) => resource.id === resourceId);
  const relationshipResourceLabel = (resourceId: string) => {
    const resource = contextResource(resourceId);
    return (
      resource?.canonicalIdentity?.displayName?.trim() ||
      resource?.name.trim() ||
      resolveResourceLabel?.(resourceId)?.trim() ||
      resourceId
    );
  };
  const relationshipPathLabel = (edge: ResourceRelationshipContextEdge) =>
    edge.traversalPath.map(relationshipResourceLabel).join(' → ');

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
            <div class="text-[9px] uppercase tracking-wide text-muted">Direct relationships</div>
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
                        <Show when={relationship.evidenceFreshness === 'stale'}>
                          <span class="inline-flex items-center rounded-sm bg-amber-100 px-1.5 py-0.5 text-[9px] text-amber-800 dark:bg-amber-900/30 dark:text-amber-200">
                            Stale provider evidence
                          </span>
                        </Show>
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

        <Show when={topologyTree().length > 0}>
          <section class="mt-3" data-testid="resource-topology-tree">
            <div class="text-[9px] uppercase tracking-wide text-muted">
              Infrastructure hierarchy
            </div>
            <ul class="mt-1 space-y-1">
              <For each={topologyTree()}>
                {(node) => (
                  <ResourceTopologyTreeEntry
                    node={node}
                    buildResourceHref={buildResourceHref}
                    expandedNodeIds={expandedTopologyNodeIds}
                    setNodeExpanded={setTopologyNodeExpanded}
                  />
                )}
              </For>
            </ul>
          </section>
        </Show>

        <Show when={serviceMonitoringRelationships().length > 0}>
          <div class="mt-3" data-testid="resource-service-monitoring-context">
            <div class="text-[9px] uppercase tracking-wide text-muted">
              Monitoring attached to hosted services
            </div>
            <div class="mt-1 space-y-1.5">
              <For each={serviceMonitoringRelationships()}>
                {(edge) => {
                  const relationship = edge.relationship;
                  const monitoredId =
                    contextResource(relationship.sourceId)?.type === 'network-endpoint'
                      ? relationship.sourceId
                      : relationship.targetId;
                  const monitored = contextResource(monitoredId);
                  const href = buildResourceHref(monitoredId);
                  const label = relationshipResourceLabel(monitoredId);
                  return (
                    <div
                      class="rounded-sm bg-surface px-2 py-1.5"
                      data-testid="resource-service-monitoring-edge"
                    >
                      <div class="flex flex-wrap items-center gap-1.5 text-[10px] text-base-content">
                        {href ? (
                          <a
                            class="inline-flex rounded-sm bg-surface-alt px-1.5 py-0.5 text-blue-700 hover:underline dark:text-blue-300"
                            href={href}
                            aria-label={`Open monitored service resource ${label}`}
                          >
                            {label}
                          </a>
                        ) : (
                          <span class="inline-flex rounded-sm bg-surface-alt px-1.5 py-0.5">
                            {label}
                          </span>
                        )}
                        <span class="text-muted">
                          Availability check · indirect service context
                        </span>
                        <Show when={monitored}>
                          {(resource) => (
                            <span class="rounded-sm bg-surface-alt px-1.5 py-0.5">
                              {resource().health?.verdict ?? resource().status}
                            </span>
                          )}
                        </Show>
                      </div>
                      <div class="mt-1 text-[10px] text-muted">
                        Path: {relationshipPathLabel(edge)}
                      </div>
                      <Show
                        when={(monitored?.incidentCount ?? monitored?.incidents?.length ?? 0) > 0}
                      >
                        <div class="mt-1 text-[10px] text-amber-700 dark:text-amber-300">
                          Observed incident:{' '}
                          {monitored?.incidentSummary ||
                            monitored?.incidents?.map((incident) => incident.summary).join(' · ')}
                        </div>
                      </Show>
                    </div>
                  );
                }}
              </For>
            </div>
          </div>
        </Show>

        <Show when={crossBranchRelationships().length > 0}>
          <details
            class="mt-3 rounded-sm border border-border-subtle bg-surface"
            data-testid="resource-cross-branch-context"
            onToggle={(event) => setShowCrossBranchContext(event.currentTarget.open)}
          >
            <summary class="cursor-pointer px-2 py-1.5 text-[10px] font-medium text-base-content">
              Cross-branch relationships ({crossBranchRelationships().length})
            </summary>
            <Show when={showCrossBranchContext()}>
              <div class="space-y-1.5 border-t border-border-subtle p-2">
                <p class="text-[10px] text-muted">
                  These explicit graph edges connect separate hierarchy branches. They indicate
                  possible relevance, not confirmed impact.
                </p>
                <For each={crossBranchRelationships()}>
                  {(edge) => {
                    const relationship = edge.relationship;
                    const sourceName = relationshipResourceLabel(relationship.sourceId);
                    const targetName = relationshipResourceLabel(relationship.targetId);
                    const sourceHref = buildResourceHref(relationship.sourceId);
                    const targetHref = buildResourceHref(relationship.targetId);
                    return (
                      <div
                        class="rounded-sm bg-surface-alt px-2 py-1.5"
                        data-testid="resource-cross-branch-edge"
                      >
                        <div class="flex flex-wrap items-center gap-1 text-[10px] text-base-content">
                          {sourceHref ? (
                            <a
                              class="rounded-sm bg-surface px-1.5 py-0.5 text-blue-700 hover:underline dark:text-blue-300"
                              href={sourceHref}
                              aria-label={`Open cross-branch resource ${sourceName}`}
                            >
                              {sourceName}
                            </a>
                          ) : (
                            <span class="rounded-sm bg-surface px-1.5 py-0.5">{sourceName}</span>
                          )}
                          <span class="text-muted">→</span>
                          {targetHref ? (
                            <a
                              class="rounded-sm bg-surface px-1.5 py-0.5 text-blue-700 hover:underline dark:text-blue-300"
                              href={targetHref}
                              aria-label={`Open cross-branch resource ${targetName}`}
                            >
                              {targetName}
                            </a>
                          ) : (
                            <span class="rounded-sm bg-surface px-1.5 py-0.5">{targetName}</span>
                          )}
                          <span class="rounded-sm bg-surface px-1.5 py-0.5 text-muted">
                            {formatResourceRelationshipType(relationship)}
                          </span>
                        </div>
                        <div class="mt-1 text-[10px] text-muted">
                          Path: {relationshipPathLabel(edge)}
                        </div>
                        <div class="mt-0.5 text-[10px] text-muted">
                          This path may be relevant. Impact is unconfirmed.
                        </div>
                      </div>
                    );
                  }}
                </For>
              </div>
            </Show>
          </details>
        </Show>

        <Show when={infrastructureRelationships().length > 0}>
          <details
            class="mt-3 rounded-sm border border-border-subtle bg-surface"
            data-testid="resource-shared-infrastructure-context"
            onToggle={(event) => setShowInfrastructureContext(event.currentTarget.open)}
          >
            <summary class="cursor-pointer px-2 py-1.5 text-[10px] font-medium text-base-content">
              Shared infrastructure context ({infrastructureRelationships().length})
            </summary>
            <Show when={showInfrastructureContext()}>
              <div class="space-y-1.5 border-t border-border-subtle p-2">
                <p class="text-[10px] text-muted">
                  These resources share an infrastructure path. Their presence does not establish a
                  direct dependency or confirmed impact.
                </p>
                <For each={infrastructureRelationships()}>
                  {(edge) => {
                    const relationship = edge.relationship;
                    const source = contextResource(relationship.sourceId);
                    const target = contextResource(relationship.targetId);
                    const sourceName = relationshipResourceLabel(relationship.sourceId);
                    const targetName = relationshipResourceLabel(relationship.targetId);
                    const sourceHref = buildResourceHref(relationship.sourceId);
                    const targetHref = buildResourceHref(relationship.targetId);
                    const sourceLabel = `${sourceName}${source?.type ? ` · ${source.type}` : ''} · ${relationship.sourceId.slice(-8)}`;
                    const targetLabel = `${targetName}${target?.type ? ` · ${target.type}` : ''} · ${relationship.targetId.slice(-8)}`;
                    return (
                      <div
                        class="rounded-sm bg-surface-alt px-2 py-1.5"
                        data-testid="resource-shared-infrastructure-edge"
                      >
                        <div class="flex flex-wrap items-center gap-1 text-[10px] text-base-content">
                          {sourceHref ? (
                            <a
                              class="rounded-sm bg-surface px-1.5 py-0.5 text-blue-700 hover:underline dark:text-blue-300"
                              href={sourceHref}
                              aria-label={`Open shared infrastructure resource ${sourceLabel}`}
                            >
                              {sourceLabel}
                            </a>
                          ) : (
                            <span class="rounded-sm bg-surface px-1.5 py-0.5">{sourceLabel}</span>
                          )}
                          <span class="text-muted">→</span>
                          {targetHref ? (
                            <a
                              class="rounded-sm bg-surface px-1.5 py-0.5 text-blue-700 hover:underline dark:text-blue-300"
                              href={targetHref}
                              aria-label={`Open shared infrastructure resource ${targetLabel}`}
                            >
                              {targetLabel}
                            </a>
                          ) : (
                            <span class="rounded-sm bg-surface px-1.5 py-0.5">{targetLabel}</span>
                          )}
                          <span class="rounded-sm bg-surface px-1.5 py-0.5 text-muted">
                            {formatResourceRelationshipType(relationship)}
                          </span>
                        </div>
                        <div class="mt-1 text-[10px] text-muted">
                          Path: {relationshipPathLabel(edge)}
                        </div>
                        <div class="mt-0.5 text-[10px] text-muted">
                          This path may share a failure domain. Impact is unconfirmed.
                        </div>
                      </div>
                    );
                  }}
                </For>
              </div>
            </Show>
          </details>
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

        <Show when={dependencies().length > 0 || dependents().length > 0}>
          <details
            class="mt-3 rounded-sm border border-border-subtle bg-surface"
            data-testid="resource-ai-relationship-suggestions"
          >
            <summary class="cursor-pointer px-2 py-1.5 text-[10px] font-medium text-base-content">
              AI relationship suggestions ({dependencies().length + dependents().length})
            </summary>
            <div class="space-y-2 border-t border-border-subtle p-2">
              <p class="text-[10px] text-muted">
                These analysis results are candidate paths. They are not provider-verified
                dependencies or confirmed impact.
              </p>
              <Show when={dependencies().length > 0}>
                <div>
                  <div class="text-[9px] uppercase tracking-wide text-muted">
                    Suggested dependencies
                  </div>
                  <div class="mt-1 flex flex-wrap gap-1">
                    <For each={dependencies().slice(0, maxCorrelations())}>
                      {(dependency) => {
                        const label = resolveResourceLabel?.(dependency)?.trim() || dependency;
                        const href = buildResourceHref(dependency);
                        return href ? (
                          <a
                            class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-blue-700 hover:underline dark:text-blue-300"
                            href={href}
                            aria-label={`Open suggested dependency ${label} in Infrastructure`}
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
                <div>
                  <div class="text-[9px] uppercase tracking-wide text-muted">
                    Potentially used by
                  </div>
                  <div class="mt-1 flex flex-wrap gap-1">
                    <For each={dependents().slice(0, maxCorrelations())}>
                      {(dependent) => {
                        const label = resolveResourceLabel?.(dependent)?.trim() || dependent;
                        const href = buildResourceHref(dependent);
                        return href ? (
                          <a
                            class="inline-flex items-center rounded-sm bg-surface-alt px-1.5 py-0.5 text-[10px] text-blue-700 hover:underline dark:text-blue-300"
                            href={href}
                            aria-label={`Open potentially affected resource ${label} in Infrastructure`}
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
            </div>
          </details>
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
