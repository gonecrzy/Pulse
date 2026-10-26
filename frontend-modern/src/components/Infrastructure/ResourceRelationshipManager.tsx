import { For, Show, createEffect, createMemo, createSignal } from 'solid-js';
import type { Component } from 'solid-js';
import GitBranchIcon from 'lucide-solid/icons/git-branch';
import LinkIcon from 'lucide-solid/icons/link';
import XIcon from 'lucide-solid/icons/x';
import { Dialog } from '@/components/shared/Dialog';
import { useUnifiedResources } from '@/hooks/useUnifiedResources';
import { ResourceAPI } from '@/api/resources';
import type {
  Resource,
  ResourceIdentityLink,
  ResourceRelationship,
  ResourceRelationshipType,
} from '@/types/resource';
import { showSuccess } from '@/utils/toast';

type ManagerMode = 'topology' | 'identity';
type OperatorType = 'depends_on' | 'runs_on' | 'hosted_by' | 'routes_to';

interface ResourceRelationshipManagerProps {
  initialResourceId?: string;
  inline?: boolean;
  onResourceActionSettled?: () => void | Promise<void>;
}

const RELATIONSHIP_OPTIONS: Array<{ value: OperatorType; label: string; help: string }> = [
  { value: 'depends_on', label: 'Depends on', help: 'Source depends on target.' },
  { value: 'runs_on', label: 'Runs on', help: 'Workload runs on an execution host.' },
  {
    value: 'hosted_by',
    label: 'Hosted by',
    help: 'Application, service, or stack is hosted by a host.',
  },
  {
    value: 'routes_to',
    label: 'Routes to',
    help: 'Kubernetes ingress routes to a Kubernetes service.',
  },
];

const labelFor = (resource: Resource | undefined, id: string) =>
  resource?.displayName?.trim() || resource?.name?.trim() || id;

const identityDetails = (resource: Resource) => {
  const nativeIds = [
    resource.platformId,
    resource.canonicalIdentity?.platformId,
    resource.proxmox?.sourceId,
    resource.canonicalIdentity?.primaryId,
  ].filter((value): value is string => Boolean(value?.trim()));
  return {
    nativeIds: [...new Set(nativeIds)],
    hostnames: [
      ...new Set(
        [resource.identity?.hostname, ...(resource.identity?.hostnames ?? [])].filter(
          (value): value is string => Boolean(value?.trim()),
        ),
      ),
    ],
    ips: [...new Set(resource.identity?.ips ?? [])],
  };
};

const HOST_TYPES = new Set([
  'agent',
  'vm',
  'system-container',
  'k8s-cluster',
  'k8s-node',
  'komodo-server',
  'komodo-swarm',
  'pbs',
  'pmg',
  'ceph',
]);
const WORKLOAD_TYPES = new Set([
  'vm',
  'system-container',
  'app-container',
  'docker-service',
  'pod',
  'k8s-deployment',
  'komodo-deployment',
]);
const HOSTED_TYPES = new Set([
  'komodo-stack',
  'komodo-deployment',
  'app-container',
  'docker-service',
  'pod',
  'k8s-deployment',
]);

export function isOperatorRelationshipCompatible(
  sourceType: string,
  targetType: string,
  relation: OperatorType,
): boolean {
  if (relation === 'depends_on') return true;
  if (relation === 'runs_on') return WORKLOAD_TYPES.has(sourceType) && HOST_TYPES.has(targetType);
  if (relation === 'hosted_by') return HOSTED_TYPES.has(sourceType) && HOST_TYPES.has(targetType);
  return sourceType === 'k8s-ingress' && targetType === 'k8s-service';
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

const ResourceRelationshipManagerPanel: Component<{
  initialResourceId?: string;
  inline?: boolean;
  onClose?: () => void;
  onResourceActionSettled?: () => void | Promise<void>;
  enabled: () => boolean;
}> = (props) => {
  const inventory = useUnifiedResources({
    query: '',
    cacheKey: 'relationship-manager-inventory',
    initialHydration: 'prefer-ws-then-rest',
    enabled: props.enabled,
  });
  const [mode, setMode] = createSignal<ManagerMode>('topology');
  const [search, setSearch] = createSignal('');
  const [providerFilter, setProviderFilter] = createSignal('all');
  const [typeFilter, setTypeFilter] = createSignal('all');
  const [sourceId, setSourceId] = createSignal('');
  const [targetId, setTargetId] = createSignal('');
  const [relationshipType, setRelationshipType] = createSignal<OperatorType>('depends_on');
  const [confirmRelationship, setConfirmRelationship] = createSignal(false);
  const [reason, setReason] = createSignal('');
  const [identityPrimaryId, setIdentityPrimaryId] = createSignal('');
  const [identityTargetId, setIdentityTargetId] = createSignal('');
  const [identityReason, setIdentityReason] = createSignal('');
  const [confirmIdentity, setConfirmIdentity] = createSignal(false);
  const [confirmProviderConflict, setConfirmProviderConflict] = createSignal(false);
  const [identityLinks, setIdentityLinks] = createSignal<ResourceIdentityLink[]>([]);
  const [identityLoading, setIdentityLoading] = createSignal(false);
  const [identityError, setIdentityError] = createSignal('');
  const [saving, setSaving] = createSignal(false);
  const [mutationError, setMutationError] = createSignal('');

  const resources = createMemo(() => inventory.resources());
  const resourceById = createMemo(
    () => new Map(resources().map((resource) => [resource.id, resource])),
  );
  const providers = createMemo(() =>
    [
      ...new Set(
        resources()
          .flatMap((resource) => [resource.platformType, ...(resource.sources ?? [])])
          .filter(Boolean),
      ),
    ].sort(),
  );
  const resourceTypes = createMemo(() =>
    [...new Set(resources().map((resource) => resource.type))].sort(),
  );
  const filteredResources = createMemo(() => {
    const query = search().trim().toLowerCase();
    return resources().filter((resource) => {
      const sources = [resource.platformType, ...(resource.sources ?? [])];
      const matchesProvider = providerFilter() === 'all' || sources.includes(providerFilter());
      const matchesType = typeFilter() === 'all' || resource.type === typeFilter();
      const matchesSearch =
        !query ||
        [
          resource.id,
          resource.name,
          resource.displayName,
          resource.type,
          resource.platformId,
          resource.identity?.hostname,
          ...(resource.identity?.hostnames ?? []),
        ].some((value) => value?.toLowerCase().includes(query));
      return matchesProvider && matchesType && matchesSearch;
    });
  });
  const relationships = createMemo(() => {
    const unique = new Map<string, ResourceRelationship>();
    for (const resource of resources()) {
      for (const relationship of resource.relationships ?? []) {
        if (!relationship.active) continue;
        const key = `${relationship.sourceId}\u0000${relationship.targetId}\u0000${relationship.type}`;
        const previous = unique.get(key);
        if (!previous || relationship.discoverer === 'operator_store')
          unique.set(key, relationship);
      }
    }
    return [...unique.values()].sort((a, b) =>
      `${a.type}:${a.sourceId}:${a.targetId}`.localeCompare(
        `${b.type}:${b.sourceId}:${b.targetId}`,
      ),
    );
  });
  const matchingOperatorEdge = createMemo(() =>
    relationships().find(
      (relationship) =>
        relationship.sourceId === sourceId() &&
        relationship.targetId === targetId() &&
        relationship.type === relationshipType() &&
        relationship.discoverer === 'operator_store',
    ),
  );
  const matchingExistingEdge = createMemo(() =>
    relationships().find(
      (relationship) =>
        relationship.sourceId === sourceId() &&
        relationship.targetId === targetId() &&
        relationship.type === relationshipType(),
    ),
  );
  const matchingIdentityLink = createMemo(() =>
    identityLinks().find(
      (link) =>
        (link.resourceA === identityPrimaryId() && link.resourceB === identityTargetId()) ||
        (link.resourceB === identityPrimaryId() && link.resourceA === identityTargetId()),
    ),
  );
  const compatibleTargets = createMemo(() =>
    resources().filter(
      (resource) =>
        resource.id !== sourceId() &&
        isOperatorRelationshipCompatible(
          resourceById().get(sourceId())?.type ?? '',
          resource.type,
          relationshipType(),
        ),
    ),
  );
  const compatibleIdentityTargets = createMemo(() => {
    const primary = resourceById().get(identityPrimaryId());
    return resources().filter(
      (resource) =>
        resource.id !== identityPrimaryId() &&
        primary &&
        (resource.type === primary.type ||
          (HOST_TYPES.has(resource.type) && HOST_TYPES.has(primary.type))),
    );
  });

  createEffect(() => {
    const initial = props.initialResourceId?.trim();
    if (initial && resources().some((resource) => resource.id === initial)) {
      setSourceId(initial);
      setIdentityPrimaryId(initial);
    }
  });

  const loadIdentityLinks = async () => {
    setIdentityLoading(true);
    setIdentityError('');
    try {
      const response = await ResourceAPI.getIdentityLinks();
      setIdentityLinks(response.links ?? []);
    } catch (error) {
      setIdentityError(errorMessage(error));
    } finally {
      setIdentityLoading(false);
    }
  };

  createEffect(() => {
    if (props.enabled()) void loadIdentityLinks();
  });

  const settled = async (message: string) => {
    showSuccess(message);
    setMutationError('');
    await inventory.refetch();
    await loadIdentityLinks();
    await props.onResourceActionSettled?.();
  };

  const saveRelationship = async (event: SubmitEvent) => {
    event.preventDefault();
    if (saving() || !sourceId() || !targetId() || !confirmRelationship()) return;
    const source = resourceById().get(sourceId());
    const target = resourceById().get(targetId());
    if (
      !source ||
      !target ||
      !isOperatorRelationshipCompatible(source.type, target.type, relationshipType())
    ) {
      setMutationError('Choose resources that support this relationship direction.');
      return;
    }
    if (matchingExistingEdge() && matchingExistingEdge()?.discoverer !== 'operator_store') {
      setMutationError('An equivalent provider-observed relationship already exists.');
      return;
    }
    setSaving(true);
    setMutationError('');
    try {
      await ResourceAPI.addRelationship(sourceId(), {
        targetId: targetId(),
        type: relationshipType(),
        reason: reason().trim() || undefined,
      });
      await settled(matchingOperatorEdge() ? 'Relationship updated' : 'Relationship added');
    } catch (error) {
      setMutationError(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const removeRelationship = async (relationship: ResourceRelationship) => {
    setSaving(true);
    setMutationError('');
    try {
      await ResourceAPI.removeRelationship(relationship.sourceId, {
        targetId: relationship.targetId,
        type: relationship.type as ResourceRelationshipType,
      });
      await settled('Relationship removed');
    } catch (error) {
      setMutationError(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const linkIdentity = async (event: SubmitEvent) => {
    event.preventDefault();
    if (
      saving() ||
      !confirmIdentity() ||
      !identityPrimaryId() ||
      !identityTargetId() ||
      matchingIdentityLink()
    )
      return;
    setSaving(true);
    setMutationError('');
    try {
      await ResourceAPI.linkIdentity(
        identityPrimaryId(),
        identityTargetId(),
        identityReason().trim(),
        confirmProviderConflict(),
      );
      setConfirmIdentity(false);
      setConfirmProviderConflict(false);
      setIdentityReason('');
      await settled('Resource identities linked');
    } catch (error) {
      setMutationError(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const unlinkIdentity = async (link: ResourceIdentityLink) => {
    setSaving(true);
    setMutationError('');
    try {
      const otherId = link.primaryId === link.resourceA ? link.resourceB : link.resourceA;
      await ResourceAPI.unlinkIdentity(link.primaryId, otherId, 'Operator removed identity link');
      await settled('Identity link removed');
    } catch (error) {
      setMutationError(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const selectedIdentityResources = createMemo(() => ({
    primary: resourceById().get(identityPrimaryId()),
    target: resourceById().get(identityTargetId()),
  }));
  const pageTitleId = () =>
    props.inline ? 'relationship-manager-title' : 'relationship-manager-dialog-title';

  return (
    <div class={props.inline ? 'space-y-5' : 'flex h-full min-h-0 flex-col'}>
      <div class="flex items-start justify-between gap-4 border-b border-border pb-4">
        <div>
          <h2 id={pageTitleId()} class="text-lg font-semibold text-base-content">
            Infrastructure relationships
          </h2>
          <p class="mt-1 max-w-3xl text-sm text-muted">
            Record directed dependencies and routes, or explicitly confirm two records describe the
            same infrastructure identity.
          </p>
        </div>
        <Show when={props.onClose}>
          {(onClose) => (
            <button
              type="button"
              onClick={onClose()}
              class="rounded-md p-1.5 text-muted hover:bg-surface-hover"
              aria-label="Close relationships"
            >
              <XIcon class="h-4 w-4" />
            </button>
          )}
        </Show>
      </div>

      <div class="flex flex-wrap gap-2" role="tablist" aria-label="Relationship manager modes">
        <button
          type="button"
          role="tab"
          aria-selected={mode() === 'topology'}
          onClick={() => setMode('topology')}
          class={`rounded-md px-3 py-2 text-sm ${mode() === 'topology' ? 'bg-blue-600 text-white' : 'bg-surface-alt text-base-content'}`}
        >
          <GitBranchIcon class="mr-1 inline h-4 w-4" />
          Topology &amp; dependencies
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode() === 'identity'}
          onClick={() => setMode('identity')}
          class={`rounded-md px-3 py-2 text-sm ${mode() === 'identity' ? 'bg-blue-600 text-white' : 'bg-surface-alt text-base-content'}`}
        >
          <LinkIcon class="mr-1 inline h-4 w-4" />
          Identity (same_as)
        </button>
      </div>

      <div class="grid gap-3 md:grid-cols-[minmax(12rem,1fr)_12rem_12rem]">
        <label class="text-xs font-medium text-muted">
          Search resources
          <input
            value={search()}
            onInput={(event) => setSearch(event.currentTarget.value)}
            placeholder="Name, ID, hostname"
            class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
          />
        </label>
        <label class="text-xs font-medium text-muted">
          Provider
          <select
            value={providerFilter()}
            onChange={(event) => setProviderFilter(event.currentTarget.value)}
            class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
          >
            <option value="all">All providers</option>
            <For each={providers()}>
              {(provider) => <option value={provider}>{provider}</option>}
            </For>
          </select>
        </label>
        <label class="text-xs font-medium text-muted">
          Resource type
          <select
            value={typeFilter()}
            onChange={(event) => setTypeFilter(event.currentTarget.value)}
            class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
          >
            <option value="all">All types</option>
            <For each={resourceTypes()}>{(type) => <option value={type}>{type}</option>}</For>
          </select>
        </label>
      </div>

      <Show when={mutationError()}>
        <div
          role="alert"
          class="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200"
        >
          {mutationError()}
        </div>
      </Show>
      <Show when={inventory.error()}>
        <div role="alert" class="rounded-md border border-red-300 px-3 py-2 text-sm text-red-700">
          Unable to load resources: {errorMessage(inventory.error())}
        </div>
      </Show>

      <Show when={mode() === 'topology'}>
        <div class="grid min-h-0 gap-5 xl:grid-cols-[minmax(18rem,0.8fr)_minmax(0,1.2fr)]">
          <form onSubmit={saveRelationship} class="space-y-3 rounded-lg border border-border p-4">
            <h3 class="font-semibold text-base-content">Add or update a relationship</h3>
            <label class="block text-xs font-medium text-muted">
              Relationship type
              <select
                value={relationshipType()}
                onChange={(event) => {
                  setRelationshipType(event.currentTarget.value as OperatorType);
                  setTargetId('');
                  setConfirmRelationship(false);
                }}
                class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
              >
                <For each={RELATIONSHIP_OPTIONS}>
                  {(option) => <option value={option.value}>{option.label}</option>}
                </For>
              </select>
            </label>
            <p class="text-xs text-muted">
              {RELATIONSHIP_OPTIONS.find((item) => item.value === relationshipType())?.help}
            </p>
            <label class="block text-xs font-medium text-muted">
              Source
              <select
                value={sourceId()}
                onChange={(event) => {
                  setSourceId(event.currentTarget.value);
                  setTargetId('');
                  setConfirmRelationship(false);
                }}
                required
                class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
              >
                <option value="">Choose source</option>
                <For each={filteredResources()}>
                  {(resource) => (
                    <option value={resource.id}>
                      {labelFor(resource, resource.id)} · {resource.type}
                    </option>
                  )}
                </For>
              </select>
            </label>
            <label class="block text-xs font-medium text-muted">
              Target
              <select
                value={targetId()}
                onChange={(event) => {
                  setTargetId(event.currentTarget.value);
                  setConfirmRelationship(false);
                }}
                required
                class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
              >
                <option value="">Choose compatible target</option>
                <For each={compatibleTargets()}>
                  {(resource) => (
                    <option value={resource.id}>
                      {labelFor(resource, resource.id)} · {resource.type}
                    </option>
                  )}
                </For>
              </select>
            </label>
            <label class="block text-xs font-medium text-muted">
              Reason (optional)
              <textarea
                value={reason()}
                onInput={(event) => setReason(event.currentTarget.value)}
                rows="2"
                placeholder="Why this relationship exists"
                class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
              />
            </label>
            <Show when={matchingExistingEdge()}>
              {(relationship) => (
                <p
                  role="status"
                  class={`rounded-md px-3 py-2 text-xs ${relationship().discoverer === 'operator_store' ? 'bg-blue-50 text-blue-800 dark:bg-blue-950/30 dark:text-blue-200' : 'bg-amber-50 text-amber-900 dark:bg-amber-950/30 dark:text-amber-100'}`}
                >
                  {relationship().discoverer === 'operator_store'
                    ? 'An operator-confirmed relationship already exists. Saving updates its reason.'
                    : `An equivalent relationship is already reported by ${relationship().discoverer}. It is read-only here.`}
                </p>
              )}
            </Show>
            <Show when={sourceId() && targetId()}>
              <p
                class="rounded-md border border-border px-3 py-2 text-xs text-base-content"
                data-testid="relationship-review"
              >
                Review: <strong>{labelFor(resourceById().get(sourceId()), sourceId())}</strong>{' '}
                {relationshipType().replaceAll('_', ' ')}{' '}
                <strong>{labelFor(resourceById().get(targetId()), targetId())}</strong>
              </p>
              <label class="flex items-start gap-2 text-xs leading-relaxed text-base-content">
                <input
                  type="checkbox"
                  checked={confirmRelationship()}
                  onChange={(event) => setConfirmRelationship(event.currentTarget.checked)}
                  class="mt-0.5"
                />
                I reviewed this direction and confirm the relationship.
              </label>
            </Show>
            <button
              type="submit"
              disabled={
                saving() ||
                !sourceId() ||
                !targetId() ||
                !confirmRelationship() ||
                Boolean(
                  matchingExistingEdge() && matchingExistingEdge()?.discoverer !== 'operator_store',
                )
              }
              class="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50"
            >
              {saving()
                ? 'Saving…'
                : matchingOperatorEdge()
                  ? 'Update relationship'
                  : 'Add relationship'}
            </button>
          </form>
          <section class="min-h-0 space-y-2" aria-label="Current topology relationships">
            <div class="flex items-center justify-between">
              <h3 class="font-semibold text-base-content">Current relationships</h3>
              <span class="text-xs text-muted">{relationships().length} active</span>
            </div>
            <Show when={!relationships().length}>
              <p class="rounded-lg border border-dashed border-border px-4 py-8 text-center text-sm text-muted">
                No active relationships have been recorded.
              </p>
            </Show>
            <div class="max-h-[26rem] space-y-2 overflow-y-auto">
              <For each={relationships()}>
                {(relationship) => {
                  const operatorOwned = relationship.discoverer === 'operator_store';
                  const source = () => resourceById().get(relationship.sourceId);
                  const target = () => resourceById().get(relationship.targetId);
                  return (
                    <article
                      class="rounded-lg border border-border px-3 py-2 text-sm"
                      data-testid="relationship-row"
                    >
                      <div class="flex flex-wrap items-center justify-between gap-2">
                        <div class="min-w-0">
                          <span class="font-medium">
                            {labelFor(source(), relationship.sourceId)}
                          </span>
                          <span class="mx-2 text-muted">
                            {relationship.type.replaceAll('_', ' ')}
                          </span>
                          <span class="font-medium">
                            {labelFor(target(), relationship.targetId)}
                          </span>
                        </div>
                        <div class="flex items-center gap-2">
                          <span
                            class={`rounded px-2 py-0.5 text-[11px] ${operatorOwned ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-200' : 'bg-surface-alt text-muted'}`}
                          >
                            {operatorOwned
                              ? 'Operator confirmed'
                              : `${relationship.evidenceSource ? 'Provider observed' : 'Automatically discovered'} · ${relationship.discoverer || 'unknown source'}`}
                          </span>
                          <Show when={operatorOwned}>
                            <button
                              type="button"
                              disabled={saving()}
                              onClick={() => {
                                setSourceId(relationship.sourceId);
                                setTargetId(relationship.targetId);
                                setRelationshipType(relationship.type as OperatorType);
                                setReason(relationship.reason ?? '');
                              }}
                              class="text-xs text-blue-700 hover:underline dark:text-blue-300"
                            >
                              Edit
                            </button>
                            <button
                              type="button"
                              disabled={saving()}
                              onClick={() => void removeRelationship(relationship)}
                              class="text-xs text-red-700 hover:underline dark:text-red-300"
                            >
                              Remove
                            </button>
                          </Show>
                        </div>
                      </div>
                      <div class="mt-1 text-xs text-muted">
                        {relationship.evidenceFreshness
                          ? `Freshness: ${relationship.evidenceFreshness}`
                          : 'Freshness: unknown'}
                        {relationship.observedAt
                          ? ` · observed ${new Date(relationship.observedAt).toLocaleString()}`
                          : ''}
                        {relationship.createdBy ? ` · owner ${relationship.createdBy}` : ''}
                        {relationship.reason ? ` · ${relationship.reason}` : ''}
                      </div>
                    </article>
                  );
                }}
              </For>
            </div>
          </section>
        </div>
      </Show>

      <Show when={mode() === 'identity'}>
        <div class="grid gap-5 xl:grid-cols-[minmax(18rem,0.8fr)_minmax(0,1.2fr)]">
          <form onSubmit={linkIdentity} class="space-y-3 rounded-lg border border-border p-4">
            <h3 class="font-semibold text-base-content">Confirm two records share an identity</h3>
            <p class="text-xs leading-relaxed text-muted">
              Provider or name/IP similarity is not proof. Review both records and choose the
              primary record explicitly.
            </p>
            <label class="block text-xs font-medium text-muted">
              Primary resource
              <select
                value={identityPrimaryId()}
                onChange={(event) => {
                  setIdentityPrimaryId(event.currentTarget.value);
                  setIdentityTargetId('');
                  setConfirmIdentity(false);
                }}
                required
                class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
              >
                <option value="">Choose primary resource</option>
                <For each={filteredResources()}>
                  {(resource) => (
                    <option value={resource.id}>
                      {labelFor(resource, resource.id)} · {resource.type}
                    </option>
                  )}
                </For>
              </select>
            </label>
            <label class="block text-xs font-medium text-muted">
              Other resource to merge
              <select
                value={identityTargetId()}
                onChange={(event) => {
                  setIdentityTargetId(event.currentTarget.value);
                  setConfirmIdentity(false);
                }}
                required
                class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
              >
                <option value="">Choose compatible resource</option>
                <For each={compatibleIdentityTargets()}>
                  {(resource) => (
                    <option value={resource.id}>
                      {labelFor(resource, resource.id)} · {resource.type}
                    </option>
                  )}
                </For>
              </select>
            </label>
            <Show when={selectedIdentityResources().primary && selectedIdentityResources().target}>
              <div class="rounded-md bg-surface-alt p-3 text-xs text-base-content">
                <For
                  each={[selectedIdentityResources().primary!, selectedIdentityResources().target!]}
                >
                  {(resource) => {
                    const details = identityDetails(resource);
                    return (
                      <div class="mb-2 last:mb-0">
                        <div class="font-semibold">
                          {labelFor(resource, resource.id)}{' '}
                          <span class="font-normal text-muted">({resource.type})</span>
                        </div>
                        <div class="text-muted">
                          Provider:{' '}
                          {(resource.sources ?? [resource.platformType]).join(', ') || 'unknown'} ·
                          native IDs: {details.nativeIds.join(', ') || resource.id}
                        </div>
                        <div class="text-muted">
                          Hostnames: {details.hostnames.join(', ') || 'none'} · observed IPs:{' '}
                          {details.ips.join(', ') || 'none'}
                        </div>
                      </div>
                    );
                  }}
                </For>
              </div>
            </Show>
            <Show when={matchingIdentityLink()}>
              <p
                role="status"
                class="rounded-md bg-blue-50 px-3 py-2 text-xs text-blue-800 dark:bg-blue-950/30 dark:text-blue-200"
              >
                These records already have a confirmed identity link.
              </p>
            </Show>
            <label class="block text-xs font-medium text-muted">
              Audit reason
              <textarea
                value={identityReason()}
                onInput={(event) => setIdentityReason(event.currentTarget.value)}
                rows="2"
                required
                placeholder="Explain the evidence for this identity decision"
                class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-base-content"
              />
            </label>
            <label class="flex items-start gap-2 text-xs leading-relaxed text-base-content">
              <input
                type="checkbox"
                checked={confirmIdentity()}
                onChange={(event) => setConfirmIdentity(event.currentTarget.checked)}
                class="mt-0.5"
              />
              I reviewed both records and confirm they are the same infrastructure identity. The
              primary selection affects canonical resolution and metric history.
            </label>
            <label class="flex items-start gap-2 text-xs leading-relaxed text-muted">
              <input
                type="checkbox"
                checked={confirmProviderConflict()}
                onChange={(event) => setConfirmProviderConflict(event.currentTarget.checked)}
                class="mt-0.5"
              />
              I understand these records collide within a provider and explicitly authorize that
              exceptional merge.
            </label>
            <p class="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950/30 dark:text-amber-100">
              Availability checks and dependency references resolve through the primary identity.
              Primary selection also affects metric history. Select the PVE record as primary to
              preserve its Proxmox history.
            </p>
            <button
              type="submit"
              disabled={
                saving() ||
                !identityPrimaryId() ||
                !identityTargetId() ||
                !identityReason().trim() ||
                !confirmIdentity() ||
                Boolean(matchingIdentityLink())
              }
              class="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-50"
            >
              {saving() ? 'Saving…' : 'Confirm identity link'}
            </button>
          </form>
          <section class="space-y-2" aria-label="Current identity links">
            <div class="flex items-center justify-between">
              <h3 class="font-semibold text-base-content">Confirmed identity links</h3>
              <span class="text-xs text-muted">{identityLinks().length} active</span>
            </div>
            <Show when={identityLoading()}>
              <p class="text-sm text-muted">Loading identity links…</p>
            </Show>
            <Show when={identityError()}>
              <p role="alert" class="text-sm text-red-700">
                Unable to load identity links: {identityError()}
              </p>
            </Show>
            <Show when={!identityLinks().length && !identityLoading()}>
              <p class="rounded-lg border border-dashed border-border px-4 py-8 text-center text-sm text-muted">
                No operator-confirmed identity links.
              </p>
            </Show>
            <div class="max-h-[26rem] space-y-2 overflow-y-auto">
              <For each={identityLinks()}>
                {(link) => (
                  <article
                    class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border px-3 py-2 text-sm"
                    data-testid="identity-link-row"
                  >
                    <div>
                      <div>
                        <span class="font-medium">
                          {labelFor(resourceById().get(link.resourceA), link.resourceA)}
                        </span>
                        <span class="mx-2 text-muted">same as</span>
                        <span class="font-medium">
                          {labelFor(resourceById().get(link.resourceB), link.resourceB)}
                        </span>
                      </div>
                      <div class="mt-1 text-xs text-muted">
                        Primary: {labelFor(resourceById().get(link.primaryId), link.primaryId)} ·{' '}
                        {link.reason || 'No reason recorded'}
                        {link.createdAt ? ` · ${new Date(link.createdAt).toLocaleString()}` : ''}
                      </div>
                      <div class="text-xs text-muted">
                        Confirmed by {link.createdBy || 'operator'}
                      </div>
                    </div>
                    <button
                      type="button"
                      disabled={saving()}
                      onClick={() => void unlinkIdentity(link)}
                      class="text-xs text-red-700 hover:underline dark:text-red-300"
                    >
                      Remove link
                    </button>
                  </article>
                )}
              </For>
            </div>
          </section>
        </div>
      </Show>

      <Show when={inventory.loading() && !resources().length}>
        <p class="text-sm text-muted">Loading resource inventory…</p>
      </Show>
    </div>
  );
};

export const ResourceRelationshipManager: Component<ResourceRelationshipManagerProps> = (props) => {
  const [open, setOpen] = createSignal(Boolean(props.inline));
  createEffect(() => {
    if (props.inline) setOpen(true);
  });
  const enabled = () => Boolean(props.inline || open());
  const close = () => setOpen(false);
  return (
    <>
      <Show when={!props.inline}>
        <button
          type="button"
          onClick={() => setOpen(true)}
          class="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs font-medium text-base-content hover:bg-surface-hover"
          aria-label="Add or manage infrastructure relationships"
        >
          <GitBranchIcon class="h-3.5 w-3.5" aria-hidden="true" />
          <span>Add relationship</span>
        </button>
      </Show>
      <Show when={props.inline}>
        <ResourceRelationshipManagerPanel
          inline
          initialResourceId={props.initialResourceId}
          enabled={enabled}
          onResourceActionSettled={props.onResourceActionSettled}
        />
      </Show>
      <Show when={!props.inline}>
        <Dialog
          isOpen={open()}
          onClose={close}
          panelClass="max-h-[92vh] max-w-7xl"
          ariaLabel="Infrastructure relationships"
        >
          <div class="flex max-h-[92vh] flex-col overflow-y-auto p-5">
            <ResourceRelationshipManagerPanel
              initialResourceId={props.initialResourceId}
              enabled={enabled}
              onClose={close}
              onResourceActionSettled={props.onResourceActionSettled}
            />
          </div>
        </Dialog>
      </Show>
    </>
  );
};

export default ResourceRelationshipManager;
