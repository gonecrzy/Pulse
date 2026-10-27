import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@solidjs/testing-library';
import { STORAGE_KEYS } from '@/utils/localStorage';
import { SecurityStep } from '../steps/SecurityStep';
import type { WizardState } from '../SetupWizard';

const apiFetchJSONMock = vi.fn();
const setApiTokenMock = vi.fn();
const showErrorMock = vi.fn();

vi.mock('@/utils/apiClient', () => ({
  apiFetchJSON: (...args: unknown[]) => apiFetchJSONMock(...args),
  setApiToken: (...args: unknown[]) => setApiTokenMock(...args),
}));

vi.mock('@/utils/toast', () => ({
  showError: (...args: unknown[]) => showErrorMock(...args),
}));

const baseState: WizardState = {
  username: 'admin',
  password: '',
  apiToken: '',
};

const stubCryptoRandom = () => {
  const getRandomValuesMock = vi.fn((array: Uint8Array) => {
    for (let index = 0; index < array.length; index += 1) {
      array[index] = index + 1;
    }
    return array;
  });

  vi.stubGlobal('crypto', {
    getRandomValues: getRandomValuesMock,
  });

  return getRandomValuesMock;
};

describe('SecurityStep', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();
    // Drop queued one-off responses so a test cannot hand its leftovers on.
    apiFetchJSONMock.mockReset();
    apiFetchJSONMock.mockResolvedValue({ success: true });
    stubCryptoRandom();
  });

  afterEach(() => {
    cleanup();
    sessionStorage.clear();
    vi.unstubAllGlobals();
  });

  it('hands off username and token but never persists the plaintext password', async () => {
    const updateState = vi.fn();
    const onComplete = vi.fn();

    render(() => (
      <SecurityStep
        state={baseState}
        updateState={updateState}
        bootstrapToken="bootstrap-token"
        onComplete={onComplete}
        onBack={vi.fn()}
      />
    ));

    fireEvent.click(screen.getByRole('button', { name: /Create Account & Continue/i }));

    await waitFor(() => {
      expect(apiFetchJSONMock).toHaveBeenCalledWith(
        '/api/security/quick-setup',
        expect.objectContaining({ method: 'POST' }),
      );
    });

    const [, requestInit] = apiFetchJSONMock.mock.calls[0] as [string, RequestInit];
    const body = JSON.parse(String(requestInit.body)) as {
      username: string;
      password: string;
      apiToken: string;
      setupToken: string;
    };

    expect(body.username).toBe('admin');
    expect(body.password).toHaveLength(20);
    expect(body.apiToken).toHaveLength(48);
    expect(body.setupToken).toBe('bootstrap-token');
    expect(setApiTokenMock).toHaveBeenCalledWith(body.apiToken);
    // The password is kept in in-memory wizard state so the completion screen
    // can show it once for the user to save.
    expect(updateState).toHaveBeenCalledWith({
      username: 'admin',
      password: body.password,
      apiToken: body.apiToken,
    });

    const storedHandoff = JSON.parse(
      sessionStorage.getItem(STORAGE_KEYS.SETUP_HANDOFF) || '{}',
    ) as {
      username?: string;
      password?: string;
      apiToken?: string;
      createdAt?: string;
    };
    expect(storedHandoff).toMatchObject({
      username: 'admin',
      apiToken: body.apiToken,
    });
    // The plaintext admin password must never be written to browser storage
    // (code-scanning finding). It lives only in in-memory wizard state and is
    // shown once on the completion screen.
    expect(storedHandoff.password).toBeUndefined();
    expect(storedHandoff.createdAt).toEqual(expect.any(String));
    expect(onComplete).toHaveBeenCalledOnce();
    expect(showErrorMock).not.toHaveBeenCalled();
    // Usage statistics stay off by default, so setup makes no settings write.
    expect(apiFetchJSONMock).toHaveBeenCalledTimes(1);
  });

  it('offers an opt-in that is off by default and saves it after the account exists', async () => {
    const onComplete = vi.fn();

    render(() => (
      <SecurityStep
        state={baseState}
        updateState={vi.fn()}
        bootstrapToken="bootstrap-token"
        onComplete={onComplete}
        onBack={vi.fn()}
      />
    ));

    const toggle = screen.getByRole('button', { name: 'Usage statistics' });
    expect(toggle).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByText(/never hostnames, credentials, or IP addresses/)).toBeInTheDocument();

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-pressed', 'true');

    fireEvent.click(screen.getByRole('button', { name: /Create Account & Continue/i }));

    await waitFor(() => expect(onComplete).toHaveBeenCalledOnce());

    expect(apiFetchJSONMock).toHaveBeenCalledTimes(2);
    expect(apiFetchJSONMock.mock.calls[0][0]).toBe('/api/security/quick-setup');
    const [settingsUrl, settingsInit] = apiFetchJSONMock.mock.calls[1] as [string, RequestInit];
    expect(settingsUrl).toBe('/api/system/settings/update');
    expect(settingsInit.method).toBe('POST');
    expect(JSON.parse(String(settingsInit.body))).toEqual({ telemetryEnabled: true });
    expect(showErrorMock).not.toHaveBeenCalled();
  });

  // A server started with PULSE_TELEMETRY refuses the write (409 env_locked).
  it('reports when server policy keeps a requested opt-in off', async () => {
    const onComplete = vi.fn();
    apiFetchJSONMock
      .mockResolvedValueOnce({ success: true })
      .mockRejectedValueOnce(
        new Error('telemetryEnabled is locked by the PULSE_TELEMETRY environment variable'),
      )
      .mockResolvedValueOnce({ telemetryEnabled: false, envOverrides: { PULSE_TELEMETRY: true } });

    render(() => (
      <SecurityStep
        state={baseState}
        updateState={vi.fn()}
        bootstrapToken="bootstrap-token"
        onComplete={onComplete}
        onBack={vi.fn()}
      />
    ));

    fireEvent.click(screen.getByRole('button', { name: 'Usage statistics' }));
    fireEvent.click(screen.getByRole('button', { name: /Create Account & Continue/i }));

    await waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
    expect(apiFetchJSONMock.mock.calls[2][0]).toBe('/api/system/settings');
    expect(showErrorMock).toHaveBeenCalledWith(
      'Your admin account was created. Usage statistics remain off because this server sets PULSE_TELEMETRY=false. Remove that override and restart Pulse to enable them.',
    );
  });

  it('does not report a failed opt-in if the server already keeps usage statistics on', async () => {
    const onComplete = vi.fn();
    apiFetchJSONMock
      .mockResolvedValueOnce({ success: true })
      .mockRejectedValueOnce(
        new Error('telemetryEnabled is locked by the PULSE_TELEMETRY environment variable'),
      )
      .mockResolvedValueOnce({ telemetryEnabled: true, envOverrides: { PULSE_TELEMETRY: true } });

    render(() => (
      <SecurityStep
        state={baseState}
        updateState={vi.fn()}
        bootstrapToken="bootstrap-token"
        onComplete={onComplete}
        onBack={vi.fn()}
      />
    ));

    fireEvent.click(screen.getByRole('button', { name: 'Usage statistics' }));
    fireEvent.click(screen.getByRole('button', { name: /Create Account & Continue/i }));

    await waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
    expect(showErrorMock).not.toHaveBeenCalled();
  });

  it('keeps the account when the opt-in write fails and says so', async () => {
    const onComplete = vi.fn();
    apiFetchJSONMock
      .mockResolvedValueOnce({ success: true })
      .mockRejectedValueOnce(new Error('settings unavailable'));

    render(() => (
      <SecurityStep
        state={baseState}
        updateState={vi.fn()}
        bootstrapToken="bootstrap-token"
        onComplete={onComplete}
        onBack={vi.fn()}
      />
    ));

    fireEvent.click(screen.getByRole('button', { name: 'Usage statistics' }));
    fireEvent.click(screen.getByRole('button', { name: /Create Account & Continue/i }));

    await waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
    expect(showErrorMock).toHaveBeenCalledWith(
      'Your admin account was created, but usage statistics could not be enabled. You can enable them in Settings → System → General.',
    );
  });
});
