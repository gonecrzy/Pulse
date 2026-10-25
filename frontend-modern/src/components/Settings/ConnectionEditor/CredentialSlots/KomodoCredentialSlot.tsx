import { Component, Show, createSignal } from 'solid-js';
import { Button } from '@/components/shared/Button';
import { CalloutCard } from '@/components/shared/CalloutCard';
import { formControl, formField, formHelpText, formLabel } from '@/components/shared/Form';
import { KomodoAPI, type KomodoConnectionInput } from '@/api/komodo';
import type { Connection } from '@/api/connections';

const SECRET_MASK = '********';

export interface KomodoCredentialSlotProps {
  editingConnection?: Connection;
  onCancel: () => void;
  onSaved: () => void;
  onToggleEnabled?: () => void;
  togglePending?: boolean;
  connectionEnabled?: boolean;
  onDelete?: () => void;
  deletePending?: boolean;
  deleteConfirming?: boolean;
  deleteError?: string | null;
}

export const KomodoCredentialSlot: Component<KomodoCredentialSlotProps> = (props) => {
  const [name, setName] = createSignal(props.editingConnection?.name ?? '');
  const [endpoint, setEndpoint] = createSignal(props.editingConnection?.address ?? 'https://');
  const [apiKey, setApiKey] = createSignal(props.editingConnection ? SECRET_MASK : '');
  const [apiSecret, setApiSecret] = createSignal(props.editingConnection ? SECRET_MASK : '');
  const [enabled, setEnabled] = createSignal(props.editingConnection?.enabled ?? true);
  const [busy, setBusy] = createSignal(false);
  const [testing, setTesting] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [notice, setNotice] = createSignal<string | null>(null);

  const savedID = () => (props.editingConnection?.id ?? '').replace(/^komodo:/, '');
  const input = (): KomodoConnectionInput => ({
    name: name().trim(),
    endpoint: endpoint().trim(),
    apiKey: apiKey(),
    apiSecret: apiSecret(),
    enabled: enabled(),
  });

  const runTest = async () => {
    setTesting(true);
    setError(null);
    setNotice(null);
    try {
      if (props.editingConnection) await KomodoAPI.testSaved(savedID(), input());
      else await KomodoAPI.test(input());
      setNotice('Komodo Core connection and read scope verified.');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Komodo connection test failed.');
    } finally {
      setTesting(false);
    }
  };

  const save = async (event: SubmitEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      if (props.editingConnection) await KomodoAPI.update(savedID(), input());
      else await KomodoAPI.add(input());
      props.onSaved();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not save Komodo connection.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form class="space-y-5" onSubmit={save}>
      <CalloutCard
        tone="info"
        scale="compact"
        padding="sm"
        title="Read-only monitoring"
        description="Pulse reads Komodo inventory and health. It does not run deployments, change resources, or grant action capabilities."
      />

      <div class="grid gap-4 sm:grid-cols-2">
        <label class={formField}>
          <span class={formLabel}>Connection name</span>
          <input
            class={formControl}
            required
            value={name()}
            onInput={(event) => setName(event.currentTarget.value)}
            placeholder="Komodo Core"
          />
        </label>
        <label class={formField}>
          <span class={formLabel}>Core URL</span>
          <input
            class={formControl}
            type="url"
            required
            value={endpoint()}
            onInput={(event) => setEndpoint(event.currentTarget.value)}
            placeholder="https://komodo.example.com"
          />
          <span class={formHelpText}>Use the HTTPS origin for Komodo Core.</span>
        </label>
        <label class={formField}>
          <span class={formLabel}>API key</span>
          <input
            class={formControl}
            type="password"
            autocomplete="new-password"
            value={apiKey()}
            onInput={(event) => setApiKey(event.currentTarget.value)}
            placeholder={props.editingConnection ? 'Saved key retained unless replaced' : ''}
          />
          <Show when={props.editingConnection}>
            <span class={formHelpText}>Leave the mask unchanged to keep the saved key.</span>
          </Show>
        </label>
        <label class={formField}>
          <span class={formLabel}>API secret</span>
          <input
            class={formControl}
            type="password"
            autocomplete="new-password"
            value={apiSecret()}
            onInput={(event) => setApiSecret(event.currentTarget.value)}
            placeholder={props.editingConnection ? 'Saved secret retained unless replaced' : ''}
          />
          <Show when={props.editingConnection}>
            <span class={formHelpText}>Leave the mask unchanged to keep the saved secret.</span>
          </Show>
        </label>
      </div>

      <label class="flex items-start gap-3 text-sm text-base-content">
        <input
          type="checkbox"
          class="mt-0.5"
          checked={enabled()}
          onChange={(event) => setEnabled(event.currentTarget.checked)}
        />
        <span>Enable polling for this connection</span>
      </label>

      <Show when={error()}>
        {(message) => (
          <div
            role="alert"
            class="rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-800 dark:border-rose-900/25 dark:bg-rose-950/25 dark:text-rose-200"
          >
            {message()}
          </div>
        )}
      </Show>
      <Show when={notice()}>
        {(message) => (
          <div role="status" class="text-sm text-emerald-700 dark:text-emerald-300">
            {message()}
          </div>
        )}
      </Show>
      <Show when={props.deleteError}>
        {(message) => (
          <div role="alert" class="text-sm text-rose-700 dark:text-rose-300">
            {message()}
          </div>
        )}
      </Show>

      <div class="flex flex-col-reverse gap-2 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex flex-wrap gap-2">
          <Show when={props.editingConnection && props.onToggleEnabled}>
            <Button
              type="button"
              variant="outline"
              size="settingsAction"
              onClick={props.onToggleEnabled}
              disabled={props.togglePending}
            >
              {props.togglePending
                ? 'Updating…'
                : props.connectionEnabled
                  ? 'Disable polling'
                  : 'Enable polling'}
            </Button>
          </Show>
          <Show when={props.editingConnection && props.onDelete}>
            <Button
              type="button"
              variant={props.deleteConfirming ? 'danger' : 'dangerOutline'}
              size="settingsAction"
              onClick={props.onDelete}
              disabled={props.deletePending}
            >
              {props.deletePending
                ? 'Removing…'
                : props.deleteConfirming
                  ? 'Click again to remove'
                  : 'Remove connection'}
            </Button>
          </Show>
        </div>
        <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button type="button" variant="outline" size="settingsAction" onClick={props.onCancel}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="outline"
            size="settingsAction"
            onClick={() => void runTest()}
            disabled={busy() || testing()}
          >
            {testing() ? 'Testing…' : 'Test connection'}
          </Button>
          <Button type="submit" size="settingsAction" disabled={busy() || testing()}>
            {busy() ? 'Saving…' : props.editingConnection ? 'Save changes' : 'Add Komodo'}
          </Button>
        </div>
      </div>
    </form>
  );
};
