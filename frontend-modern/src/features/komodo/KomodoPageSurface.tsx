import { A, useLocation, useSearchParams } from '@solidjs/router';
import { For, Show, createMemo } from 'solid-js';
import {
  createPlatformResourceDetailState,
  createPlatformResourceLabelResolver,
  getPlatformResourceDetailRowInteractionProps,
  PlatformResourceDetailTableRow,
  PlatformResourceDetailToggleButton,
} from '@/features/platformPage/PlatformResourceDetailTableRow';
import {
  PlatformErrorState,
  PlatformSectionTabs,
  PlatformTableEmptyState,
  PlatformTableLoadingState,
} from '@/features/platformPage/sharedPlatformPage';
import { useUnifiedResources } from '@/hooks/useUnifiedResources';
import { buildKomodoPath, KOMODO_QUERY_PARAMS } from '@/routing/resourceLinks';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/shared/Table';
import { TableCard } from '@/components/shared/TableCard';
import type { Resource, ResourceType } from '@/types/resource';
import { formatRelativeTime } from '@/utils/format';
import { getPreferredInfrastructureDisplayName } from '@/utils/resourceIdentity';
import { getSimpleStatusIndicator, getStatusIndicatorBadgeToneClasses } from '@/utils/status';

type KomodoTabId = 'all' | 'servers' | 'stacks' | 'deployments' | 'swarms';

const KOMODO_TABS: readonly { id: KomodoTabId; label: string; type?: ResourceType }[] = [
  { id: 'all', label: 'All resources' },
  { id: 'servers', label: 'Servers', type: 'komodo-server' },
  { id: 'stacks', label: 'Stacks', type: 'komodo-stack' },
  { id: 'deployments', label: 'Deployments', type: 'komodo-deployment' },
  { id: 'swarms', label: 'Swarms', type: 'komodo-swarm' },
];
const VALID_TABS = new Set<KomodoTabId>(KOMODO_TABS.map((tab) => tab.id));

const getKomodoStatus = (resource: Resource) => {
  switch (resource.health?.verdict) {
    case 'critical':
      return { variant: 'danger' as const, label: 'Critical' };
    case 'attention':
      return { variant: 'warning' as const, label: 'Attention' };
    case 'stale':
      return { variant: 'warning' as const, label: 'Stale' };
    case 'off':
      return { variant: 'muted' as const, label: 'Disabled' };
    case 'ok':
      return getSimpleStatusIndicator(resource.status);
    default:
      return getSimpleStatusIndicator(resource.status);
  }
};

const canonicalIdFor = (resource: Resource): string =>
  resource.canonicalIdentity?.primaryId?.trim() || resource.id;

const komodoObservationFor = (resource: Resource) => {
  const observation = resource.sourceStatus?.komodo;
  const observedAt = observation?.lastSeen ? Date.parse(observation.lastSeen) : Number.NaN;
  return {
    status: observation?.status?.trim() || 'unknown',
    observedAt: Number.isFinite(observedAt)
      ? formatRelativeTime(observedAt, { compact: true })
      : 'Unknown',
    title: Number.isFinite(observedAt)
      ? new Date(observedAt).toLocaleString()
      : 'No Komodo observation recorded',
  };
};

export function KomodoPageSurface() {
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedTab = createMemo<KomodoTabId>(() => {
    const segment = location.pathname.split('/').filter(Boolean)[1] as KomodoTabId | undefined;
    return segment && VALID_TABS.has(segment) ? segment : 'all';
  });
  const inventory = useUnifiedResources({
    query: 'source=komodo',
    cacheKey: 'komodo-inventory',
    initialHydration: 'prefer-ws-then-rest',
  });
  const activeTab = createMemo(() => requestedTab());
  const activeSpec = createMemo(
    () => KOMODO_TABS.find((tab) => tab.id === activeTab()) ?? KOMODO_TABS[0],
  );
  const resources = createMemo(() => inventory.resources());
  const counts = createMemo(() => {
    const facets = inventory.facets()?.byType;
    return Object.fromEntries(
      KOMODO_TABS.map((tab) => [
        tab.id,
        !tab.type
          ? inventory.resources().length
          : (facets?.[tab.type] ??
            inventory.resources().filter((resource) => resource.type === tab.type).length),
      ]),
    ) as Record<KomodoTabId, number>;
  });
  const tabs = createMemo(() =>
    KOMODO_TABS.map((tab) => ({
      id: tab.id,
      label: `${tab.label} (${counts()[tab.id]})`,
      path: buildKomodoPath(tab.id),
    })),
  );
  const search = createMemo(() => {
    const value = searchParams[KOMODO_QUERY_PARAMS.query];
    return typeof value === 'string' ? value.trim().toLowerCase() : '';
  });
  const filteredResources = createMemo(() => {
    const query = search();
    const selectedType = activeTab() === 'all' ? undefined : activeSpec().type;
    const tabResources = selectedType
      ? resources().filter((resource) => resource.type === selectedType)
      : resources();
    if (!query) return tabResources;
    return tabResources.filter((resource) => {
      const parent = resource.parentName || resource.parentId || '';
      return [
        resource.name,
        resource.displayName,
        canonicalIdFor(resource),
        resource.id,
        parent,
      ].some((value) => value.toLowerCase().includes(query));
    });
  });
  const parentNameFor = (resource: Resource): string => {
    if (resource.parentName?.trim()) return resource.parentName;
    const parent = inventory.resources().find((candidate) => candidate.id === resource.parentId);
    return parent ? getPreferredInfrastructureDisplayName(parent) : resource.parentId || '—';
  };
  const parentPathFor = (resource: Resource): string | undefined => {
    const parent = inventory.resources().find((candidate) => candidate.id === resource.parentId);
    if (!parent) return undefined;
    const tab =
      parent.type === 'komodo-server'
        ? 'servers'
        : parent.type === 'komodo-swarm'
          ? 'swarms'
          : 'all';
    return `${buildKomodoPath(tab)}?${new URLSearchParams({
      [KOMODO_QUERY_PARAMS.query]: parent.name,
    }).toString()}`;
  };
  const drawer = createPlatformResourceDetailState({ idPrefix: 'komodo-resource-detail' });
  const resolveResourceLabel = createPlatformResourceLabelResolver(() => resources());

  return (
    <div data-testid="komodo-page" class="pulse-wide-data-surface space-y-3">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold text-base-content">Komodo</h1>
          <p class="mt-1 text-sm text-muted">
            Read-only inventory from the canonical Pulse resource registry. All resources includes
            Komodo records folded into a cross-provider identity. Type tabs show their canonical
            Pulse type.
          </p>
        </div>
        <label class="flex flex-col gap-1 text-xs text-muted">
          <span>Search inventory</span>
          <input
            type="search"
            class="h-9 w-64 rounded-md border border-border bg-surface px-3 text-sm text-base-content"
            value={
              typeof searchParams[KOMODO_QUERY_PARAMS.query] === 'string'
                ? searchParams[KOMODO_QUERY_PARAMS.query]
                : ''
            }
            onInput={(event) =>
              setSearchParams(
                { [KOMODO_QUERY_PARAMS.query]: event.currentTarget.value || null },
                { replace: true },
              )
            }
            placeholder="Name or canonical ID"
          />
        </label>
      </div>

      <PlatformSectionTabs tabs={tabs()} active={activeTab()} ariaLabel="Komodo inventory" />

      <Show
        when={!inventory.loading() || inventory.resources().length > 0}
        fallback={
          <PlatformTableLoadingState
            title="Loading Komodo inventory"
            description="Pulse is loading Servers, Stacks, Deployments, and Swarms from the unified resource registry."
          />
        }
      >
        <Show
          when={!inventory.error()}
          fallback={
            <PlatformErrorState
              title="Could not load Komodo inventory"
              description="The canonical resource registry request failed. Review connection health separately in Infrastructure Settings."
              onRefresh={() => void inventory.refetch()}
            />
          }
        >
          <Show
            when={filteredResources().length > 0}
            fallback={
              <PlatformTableEmptyState
                title={
                  inventory.resources().length === 0
                    ? 'No Komodo resources in the canonical inventory'
                    : `No ${activeSpec().label.toLowerCase()} match`
                }
                description={
                  inventory.resources().length === 0
                    ? 'Review the Komodo connection and its latest poll in Infrastructure Settings. A successful empty inventory and a failed collection are reported separately there.'
                    : search()
                      ? 'Clear the search field to show all resources in this section.'
                      : activeTab() === 'all'
                        ? 'No Komodo resources appear in the canonical inventory for this organization.'
                        : `No canonical Komodo-sourced resources of type ${activeSpec().label.toLowerCase()} appear in this inventory.`
                }
                actions={
                  inventory.resources().length === 0 ? (
                    <A
                      class="text-sm font-medium text-blue-600 hover:underline dark:text-blue-300"
                      href="/settings/infrastructure"
                    >
                      Open Infrastructure Settings
                    </A>
                  ) : undefined
                }
              />
            }
          >
            <TableCard>
              <Table aria-label={`Komodo ${activeSpec().label}`}>
                <TableHeader>
                  <TableRow>
                    <TableHead class="px-3 py-2">Name</TableHead>
                    <TableHead class="px-3 py-2">Canonical identity</TableHead>
                    <TableHead class="px-3 py-2">Canonical type</TableHead>
                    <TableHead class="px-3 py-2">Health</TableHead>
                    <TableHead class="px-3 py-2">Parent</TableHead>
                    <TableHead class="px-3 py-2">Sources</TableHead>
                    <TableHead class="px-3 py-2">Komodo observation</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <For each={filteredResources()}>
                    {(resource) => {
                      const status = () => getKomodoStatus(resource);
                      const komodoObservation = () => komodoObservationFor(resource);
                      const expanded = () => drawer.isExpanded(resource);
                      const detailRowId = drawer.detailRowId(resource);
                      return (
                        <>
                          <TableRow
                            {...getPlatformResourceDetailRowInteractionProps({
                              expanded: expanded(),
                              onToggle: () => drawer.toggle(resource),
                            })}
                          >
                            <TableCell class="px-3 py-2 font-medium text-base-content">
                              <div class="flex items-center gap-2">
                                <PlatformResourceDetailToggleButton
                                  expanded={expanded()}
                                  resourceLabel={resource.name}
                                  controlsId={detailRowId}
                                  onToggle={() => drawer.toggle(resource)}
                                />
                                <span class="truncate">{resource.name}</span>
                              </div>
                            </TableCell>
                            <TableCell
                              class="max-w-72 truncate px-3 py-2 font-mono text-xs"
                              title={canonicalIdFor(resource)}
                            >
                              {canonicalIdFor(resource)}
                            </TableCell>
                            <TableCell class="px-3 py-2 text-xs text-muted">
                              {resource.type}
                            </TableCell>
                            <TableCell class="px-3 py-2">
                              <span
                                class={`inline-flex rounded-full px-2 py-0.5 text-xs ${getStatusIndicatorBadgeToneClasses(status().variant)}`}
                              >
                                {status().label}
                              </span>
                              <Show
                                when={
                                  (resource.incidentCount ?? resource.incidents?.length ?? 0) > 0
                                }
                              >
                                <span class="ml-2 text-xs text-muted">
                                  {resource.incidentCount ?? resource.incidents?.length} incident(s)
                                </span>
                              </Show>
                            </TableCell>
                            <TableCell class="px-3 py-2 text-sm">
                              <Show
                                when={resource.parentId}
                                fallback={<span class="text-muted">—</span>}
                              >
                                <Show
                                  when={parentPathFor(resource)}
                                  fallback={<span>{parentNameFor(resource)}</span>}
                                >
                                  {(path) => (
                                    <A
                                      class="text-blue-600 hover:underline dark:text-blue-300"
                                      href={path()}
                                    >
                                      {parentNameFor(resource)}
                                    </A>
                                  )}
                                </Show>
                              </Show>
                            </TableCell>
                            <TableCell class="px-3 py-2 text-xs text-muted">
                              {(resource.sources ?? []).join(', ') || '—'}
                            </TableCell>
                            <TableCell
                              class="px-3 py-2 text-sm text-muted"
                              title={komodoObservation().title}
                            >
                              <span class="capitalize">{komodoObservation().status}</span>
                              <span class="ml-2">{komodoObservation().observedAt}</span>
                            </TableCell>
                          </TableRow>
                          <PlatformResourceDetailTableRow
                            resource={resource}
                            open={expanded()}
                            detailRowId={detailRowId}
                            colSpan={7}
                            statusIndicator={status()}
                            resolveResourceLabel={resolveResourceLabel}
                            onClose={() => drawer.close(resource)}
                          />
                        </>
                      );
                    }}
                  </For>
                </TableBody>
              </Table>
            </TableCard>
          </Show>
        </Show>
      </Show>
    </div>
  );
}

export default KomodoPageSurface;
