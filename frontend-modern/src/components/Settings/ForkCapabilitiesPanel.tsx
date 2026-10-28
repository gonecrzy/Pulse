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

const CAPABILITY_DETAILS: Record<
  ForkFeatureID,
  { name: string; description: string; restrictions: string }
> = {
  'fork.codex_lb_gateway': {
    name: 'Codex-LB gateway',
    description:
      'Routes Pulse Intelligence exclusively through the configured Sindri Codex-LB OpenAI-compatible gateway.',
    restrictions:
      'Requires a configured gateway endpoint and credential. Private destinations require the separate private-network opt-in. Direct OpenAI routes are blocked while this capability is active.',
  },
  'fork.ai_investigation': {
    name: 'AI investigation',
    description: 'Runs Pulse chat investigations and stores their findings.',
    restrictions:
      'Requires Pulse Intelligence to be enabled with a configured provider and selected model. Commercial Patrol features remain governed by their own entitlement checks.',
  },
  'fork.governed_actions': {
    name: 'Governed actions',
    description: 'Provides Pulse’s typed action planner and canonical action lifecycle.',
    restrictions:
      'Availability does not grant action approval or execution authority. Existing authorization, approval, and safety checks still apply.',
  },
  'fork.komodo_read_only': {
    name: 'Komodo read-only',
    description: 'Enables the read-only Komodo monitoring adapter.',
    restrictions:
      'Requires a configured Komodo connection. This capability grants read access only and does not permit Komodo mutations.',
  },
  'fork.deterministic_triage': {
    name: 'Deterministic triage',
    description: 'Adds deterministic alert triage extensions to Patrol.',
    restrictions: 'Uses Pulse monitoring data and does not require an AI provider.',
  },
  'fork.agent_profiles': {
    name: 'Agent profiles',
    description: 'Provides public agent profile management and versioned configuration.',
    restrictions:
      'Profile changes remain subject to the existing agent and settings authorization checks.',
  },
  'fork.rbac': {
    name: 'Role-based access control',
    description: 'Provides organization-scoped roles and permission checks.',
    restrictions:
      'Does not grant instance administrator privileges or bypass existing authorization.',
  },
  'fork.audit_logging': {
    name: 'Audit logging',
    description: 'Stores local append-only audit records and exposes read APIs.',
    restrictions: 'Enabling this capability does not change audit retention or delivery settings.',
  },
  'fork.advanced_reporting': {
    name: 'Advanced reporting',
    description: 'Provides local report generation and scheduling.',
    restrictions:
      'Report access remains subject to existing settings and organization permissions.',
  },
  'fork.external_probe': {
    name: 'External probes',
    description: 'Provides operator-configured external availability probes.',
    restrictions: 'Each probe still requires an explicit operator configuration.',
  },
  'fork.action_safety': {
    name: 'Action safety',
    description: 'Planned strict remediation policy and safety controls.',
    restrictions: 'Not implemented. It cannot be enabled from this page.',
  },
  'fork.operational_cases': {
    name: 'Operational cases',
    description: 'Planned multi-alert operational cases and repair plans.',
    restrictions: 'Experimental and not implemented. It cannot be enabled from this page.',
  },
  'fork.long_term_metrics': {
    name: 'Long-term metrics',
    description: 'Planned extended Pulse metric retention tiers.',
    restrictions: 'Unavailable in this build. It cannot be enabled from this page.',
  },
  'fork.discord_ops': {
    name: 'Discord operations',
    description: 'Experimental Discord conversational operations.',
    restrictions: 'Experimental and not implemented. It cannot be enabled from this page.',
  },
};

const formatStatus = (status: string): string => status.charAt(0).toUpperCase() + status.slice(1);

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
        .filter((capability) => capability.implemented && capability.status === 'supported')
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
      title="Fork Capabilities"
      description="Manage capabilities implemented by this Pulse fork. Availability is separate from commercial entitlements and action authorization."
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
                const details = CAPABILITY_DETAILS[capability.id];
                const selected = () => draft().includes(capability.id);
                const source = () =>
                  capability.source === 'fork_public' ? 'Public fork' : 'Unavailable';
                const canEnable = capability.implemented && capability.status === 'supported';
                return (
                  <div class="flex flex-col gap-2 px-3 py-3 sm:flex-row sm:items-center sm:justify-between">
                    <div class="min-w-0">
                      <p class="font-medium text-base-content">{details.name}</p>
                      <p class="break-all font-mono text-xs text-muted">{capability.id}</p>
                      <p class="mt-1 text-sm text-muted">{details.description}</p>
                      <p class="text-xs text-muted">
                        Status: {formatStatus(capability.status)} · Implemented:{' '}
                        {capability.implemented ? 'Yes' : 'No'} · Profile state:{' '}
                        {capability.enabled ? 'Enabled' : 'Disabled'} · {source()}
                      </p>
                      <p class="text-xs text-muted">
                        Effective availability: {capability.available ? 'Available' : 'Unavailable'}
                      </p>
                      <p class="text-xs text-muted">
                        Prerequisites or restrictions: {details.restrictions}
                      </p>
                    </div>
                    <Show when={canEnable}>
                      <label for={id} class="inline-flex shrink-0 items-center gap-2 text-sm">
                        <input
                          id={id}
                          aria-label={capability.id}
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
                    <Show when={!canEnable}>
                      <span class="shrink-0 text-xs text-muted">
                        {capability.implemented
                          ? 'Enablement unavailable for this maturity level'
                          : 'Not available to enable'}
                      </span>
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
          An instance settings administrator can manage the validated fork profile.
        </p>
      </Show>
    </SettingsPanel>
  );
};
