import { createResource, For, Show, type Component } from 'solid-js';
import { ResourceAPI } from '@/api/resources';
import { ResourceCorrelationSummary } from '@/components/Infrastructure/ResourceCorrelationSummary';
import { buildResourcePagePath } from '@/features/platformNavigation/resourcePagePath';
import type { Resource } from '@/types/resource';

interface InvestigationResourceTopologyProps {
  resourceId: string;
}

type ResourceLoadResult =
  { resource: Resource; unavailable: false } | { resource: null; unavailable: true };

export const InvestigationResourceTopology: Component<InvestigationResourceTopologyProps> = (
  props,
) => {
  const [resourceResult] = createResource<ResourceLoadResult>(async () => {
    try {
      return {
        resource: await ResourceAPI.getResource(props.resourceId.trim()),
        unavailable: false,
      };
    } catch {
      return { resource: null, unavailable: true };
    }
  });
  const resource = () => resourceResult()?.resource ?? null;
  const context = () => resource()?.relationshipContext;
  const resources = () => context()?.resources ?? [];
  const relationships = () => context()?.relationships ?? [];
  const details = () => context()?.relationshipDetails ?? [];
  const staleSources = () =>
    Object.entries(resource()?.sourceStatus ?? {})
      .filter(([, observation]) => observation.status === 'stale')
      .map(([source]) => source);

  const buildResourceHref = (resourceId: string) => {
    const related = resources().find((candidate) => candidate.id === resourceId);
    return related
      ? buildResourcePagePath({ id: related.id, type: related.type, name: related.name })
      : null;
  };

  return (
    <div data-testid="patrol-investigation-resource-topology" class="mt-2 space-y-2">
      <Show when={resourceResult.loading}>
        <p class="text-xs text-muted">Loading canonical infrastructure context…</p>
      </Show>
      <Show when={resourceResult()?.unavailable}>
        <p role="status" class="text-xs text-amber-700 dark:text-amber-300">
          Canonical infrastructure context is unavailable.
        </p>
      </Show>
      <Show when={resource()}>
        {(current) => (
          <>
            <div class="rounded-sm border border-border-subtle bg-surface-alt px-2 py-1.5 text-xs">
              <p>
                <span class="font-medium text-base-content">Canonical resource:</span>{' '}
                {current().canonicalIdentity?.displayName?.trim() ||
                  current().displayName?.trim() ||
                  current().name}
              </p>
              <p class="mt-0.5 break-all text-muted">
                {current().canonicalIdentity?.primaryId || current().id}
              </p>
              <Show when={(current().sources?.length ?? 0) > 0}>
                <p class="mt-0.5 text-muted">Provenance: {current().sources?.join(', ')}</p>
              </Show>
              <Show when={staleSources().length > 0}>
                <p class="mt-0.5 text-amber-700 dark:text-amber-300">
                  Stale observations: {staleSources().join(', ')}
                </p>
              </Show>
              <Show when={current().lastSeen}>
                <p class="mt-0.5 text-muted">
                  Last observed: {new Date(current().lastSeen).toLocaleString()}
                </p>
              </Show>
            </div>

            <Show when={context()?.truncated}>
              <p class="text-xs text-amber-700 dark:text-amber-300">
                The server returned a bounded, truncated resource neighborhood.
              </p>
            </Show>

            <Show
              when={resources().length > 0 || relationships().length > 0}
              fallback={
                <p class="text-xs text-muted">
                  No linked topology evidence is available for this resource.
                </p>
              }
            >
              <ResourceCorrelationSummary
                dataTestId="patrol-investigation-relationship-map"
                title="Canonical infrastructure relationships"
                selectedResourceId={current().id}
                relationships={relationships()}
                contextResources={resources()}
                serviceMonitoringRelationships={details().filter(
                  (edge) =>
                    edge.classification === 'service_monitoring' && edge.scope === 'indirect',
                )}
                crossBranchRelationships={details().filter(
                  (edge) =>
                    edge.scope === 'indirect' &&
                    ['indirect_dependency', 'cross_branch_relationship'].includes(
                      edge.classification,
                    ),
                )}
                infrastructureRelationships={details().filter(
                  (edge) => edge.classification === 'shared_infrastructure',
                )}
                buildResourceHref={buildResourceHref}
                resolveResourceLabel={(resourceId) =>
                  resources().find((candidate) => candidate.id === resourceId)?.name || resourceId
                }
                showLastSeen
                maxCorrelations={12}
              />
            </Show>

            <Show when={details().length > 0}>
              <ul class="sr-only" aria-label="Relationship provenance summary">
                <For each={details()}>
                  {(edge) => (
                    <li>
                      {edge.scope} {edge.classification} relationship
                    </li>
                  )}
                </For>
              </ul>
            </Show>
          </>
        )}
      </Show>
    </div>
  );
};

export default InvestigationResourceTopology;
