import { Component, For, Show, createEffect, createSignal, onMount } from 'solid-js';
import { Button } from '@/components/shared/Button';
import SettingsPanel from '@/components/shared/SettingsPanel';
import type { ForkFeatureID } from '@/api/forkCapabilities';
import {
  forkCapabilitySnapshot,
  forkCapabilitiesError,
  forkCapabilitiesLoading,
  forkProfile,
  forkProfileError,
  forkProfileLoading,
  loadForkCapabilities,
  loadForkProfile,
  updateForkProfile,
} from '@/stores/forkCapabilities';

export interface ForkCapabilitiesPanelProps {
  canManage: () => boolean;
}

const featureLabel = (id: ForkFeatureID): string => id.replace(/^fork\./, '').replaceAll('_', ' ');

const isRevisionConflict = (error: unknown): boolean => {
  if (!error || typeof error !== 'object') return false;
  const apiError = error as { status?: number; code?: string };
  return apiError.status === 409 || apiError.code === 'stale_fork_profile';
};

export const ForkCapabilitiesPanel: Component<ForkCapabilitiesPanelProps> = (props) => {
  const [draft, setDraft] = createSignal<ForkFeatureID[]>([]);
  const [draftRevision, setDraftRevision] = createSignal<number | null>(null);
  const [saving, setSaving] = createSignal(false);
  const [message, setMessage] = createSignal('');

  createEffect(() => {
    const current = forkProfile();
    if (current && current.revision !== draftRevision()) {
      setDraft([...current.enabled_features]);
      setDraftRevision(current.revision);
    }
  });

  createEffect(() => {
    if (props.canManage()) void loadForkProfile();
  });

  onMount(() => {
    void loadForkCapabilities(true);
  });

  const refresh = async () => {
    setMessage('');
    await loadForkCapabilities(true);
    if (props.canManage()) await loadForkProfile();
  };

  const setFeatureEnabled = (id: ForkFeatureID, enabled: boolean) => {
    setDraft((current) =>
      enabled
        ? current.includes(id)
          ? current
          : [...current, id]
        : current.filter((feature) => feature !== id),
    );
    setMessage('');
  };

  const save = async () => {
    const currentProfile = forkProfile();
    const currentSnapshot = forkCapabilitySnapshot();
    if (!props.canManage() || !currentProfile || !currentSnapshot || saving()) return;

    const implemented = new Set(
      currentSnapshot.capabilities
        .filter((capability) => capability.implemented)
        .map((capability) => capability.id),
    );
    setSaving(true);
    setMessage('');
    try {
      await updateForkProfile({
        version: currentProfile.version,
        expected_revision: currentProfile.revision,
        enabled_features: draft().filter((feature) => implemented.has(feature)),
      });
      setMessage('Fork profile saved.');
    } catch (error) {
      if (isRevisionConflict(error)) {
        setMessage(
          'The profile changed on the server. The latest version is loaded. Review it before saving again.',
        );
      } else {
        setMessage(error instanceof Error ? error.message : 'The fork profile could not be saved.');
      }
    } finally {
      setSaving(false);
    }
  };

  const isDirty = () => {
    const current = forkProfile()?.enabled_features ?? [];
    const selected = draft();
    return (
      current.length !== selected.length || current.some((feature) => !selected.includes(feature))
    );
  };

  return (
    <SettingsPanel
      title="Personal fork capabilities"
      description="Fork availability is separate from Pulse commercial entitlements."
      action={
        <Button
          size="sm"
          variant="secondary"
          onClick={() => void refresh()}
          disabled={forkCapabilitiesLoading()}
        >
          Refresh
        </Button>
      }
      bodyClass="space-y-3 p-3 sm:space-y-4 sm:p-5"
    >
      <p class="text-sm text-muted">
        Supported local capabilities can be enabled here without Pulse activation or subscription
        state. Availability does not grant action approval or execution authority.
      </p>

      <Show when={forkCapabilitiesLoading() && !forkCapabilitySnapshot()}>
        <p role="status" class="text-sm text-muted">
          Loading fork capabilities…
        </p>
      </Show>
      <Show when={!forkCapabilitySnapshot() && !forkCapabilitiesLoading()}>
        <p role="alert" class="text-sm text-danger">
          Fork capabilities are unavailable. No capability is treated as enabled.
          <Show when={forkCapabilitiesError()}>
            <span class="block">{forkCapabilitiesError()?.message}</span>
          </Show>
        </p>
      </Show>

      <Show when={forkCapabilitySnapshot()}>
        {(snapshot) => (
          <div class="divide-y divide-border rounded-md border border-border">
            <For each={snapshot().capabilities}>
              {(capability) => {
                const id = `fork-capability-${capability.id.replaceAll('.', '-')}`;
                const selected = () => draft().includes(capability.id);
                const source = () =>
                  capability.source === 'fork_public' ? 'Public fork' : 'Unavailable';
                return (
                  <div class="flex flex-col gap-2 px-3 py-3 sm:flex-row sm:items-center sm:justify-between">
                    <div class="min-w-0">
                      <p class="font-medium capitalize text-base-content">
                        {featureLabel(capability.id)}
                      </p>
                      <p class="break-all font-mono text-xs text-muted">{capability.id}</p>
                      <p class="text-xs text-muted">
                        {capability.status} ·{' '}
                        {capability.implemented ? 'implemented' : 'not implemented'} ·{' '}
                        {capability.enabled ? 'enabled' : 'disabled'} · {source()}
                      </p>
                    </div>
                    <Show when={capability.implemented}>
                      <label for={id} class="inline-flex shrink-0 items-center gap-2 text-sm">
                        <input
                          id={id}
                          type="checkbox"
                          checked={selected()}
                          disabled={!props.canManage() || !forkProfile() || saving()}
                          onChange={(event) =>
                            setFeatureEnabled(capability.id, event.currentTarget.checked)
                          }
                        />
                        <span>
                          {selected() ? 'Enabled in fork profile' : 'Disabled in fork profile'}
                        </span>
                      </label>
                    </Show>
                  </div>
                );
              }}
            </For>
          </div>
        )}
      </Show>

      <Show when={props.canManage()}>
        <Show
          when={forkProfile()}
          fallback={
            <p role="status" class="text-sm text-muted">
              {forkProfileLoading()
                ? 'Loading editable profile…'
                : 'The editable profile is unavailable.'}
              <Show when={forkProfileError()}>
                <span class="block">{forkProfileError()?.message}</span>
              </Show>
            </p>
          }
        >
          <div class="flex flex-wrap items-center gap-3">
            <Button
              onClick={() => void save()}
              disabled={!isDirty() || saving() || !forkCapabilitySnapshot()}
              isLoading={saving()}
            >
              Save fork profile
            </Button>
            <Show when={message()}>
              <span role="status" aria-live="polite" class="text-sm text-muted">
                {message()}
              </span>
            </Show>
          </div>
        </Show>
      </Show>
      <Show when={!props.canManage()}>
        <p class="text-xs text-muted">
          A settings administrator can manage the validated fork profile.
        </p>
      </Show>
    </SettingsPanel>
  );
};
