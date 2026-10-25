import { cleanup, fireEvent, render, screen, waitFor } from '@solidjs/testing-library';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { KomodoAPI } from '@/api/komodo';
import { KomodoCredentialSlot } from '../KomodoCredentialSlot';

describe('KomodoCredentialSlot', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('tests and saves a read-only Komodo connection', async () => {
    const test = vi.spyOn(KomodoAPI, 'test').mockResolvedValue({ success: true });
    const add = vi.spyOn(KomodoAPI, 'add').mockResolvedValue({
      id: 'komodo:87ef53c3-9a5a-4d7b-a159-997ccca19811',
      name: 'lab',
      endpoint: 'https://komodo.example.test',
      apiKey: '********',
      apiSecret: '********',
      enabled: true,
    });
    const onSaved = vi.fn();
    render(() => <KomodoCredentialSlot onCancel={vi.fn()} onSaved={onSaved} />);

    fireEvent.input(screen.getByLabelText('Connection name'), { target: { value: 'lab' } });
    fireEvent.input(screen.getByLabelText(/^Core URL/), {
      target: { value: 'https://komodo.example.test' },
    });
    fireEvent.input(screen.getByLabelText('API key'), { target: { value: 'key-1' } });
    fireEvent.input(screen.getByLabelText('API secret'), { target: { value: 'secret-1' } });

    fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));
    await waitFor(() =>
      expect(test).toHaveBeenCalledWith({
        name: 'lab',
        endpoint: 'https://komodo.example.test',
        apiKey: 'key-1',
        apiSecret: 'secret-1',
        enabled: true,
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Add Komodo' }));
    await waitFor(() => expect(add).toHaveBeenCalled());
    expect(onSaved).toHaveBeenCalledOnce();
    expect(screen.getByText(/does not run deployments/)).toBeInTheDocument();
  });
});
