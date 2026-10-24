import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiFetchJSON } from '@/utils/apiClient';
import { ForkCapabilitiesAPI, type ForkProfileUpdate } from '../forkCapabilities';

vi.mock('@/utils/apiClient', () => ({ apiFetchJSON: vi.fn() }));

describe('ForkCapabilitiesAPI', () => {
  const mock = vi.mocked(apiFetchJSON);

  beforeEach(() => vi.clearAllMocks());

  it('loads resolved fork capabilities from their separate endpoint', async () => {
    const snapshot = {
      profile_version: 1,
      revision: 2,
      capabilities: [
        { id: 'fork.ai_investigation', available: true, implemented: true, source: 'fork_public' },
      ],
    };
    mock.mockResolvedValueOnce(snapshot);

    await expect(ForkCapabilitiesAPI.getCapabilities()).resolves.toStrictEqual(snapshot);
    expect(mock).toHaveBeenCalledWith('/api/fork/capabilities');
  });

  it('updates the versioned profile without sending commercial entitlement fields', async () => {
    mock.mockResolvedValueOnce({ profile_version: 1, revision: 3, capabilities: [] });
    const update: ForkProfileUpdate = {
      version: 1,
      expected_revision: 2,
      enabled_features: ['fork.ai_investigation'],
    };

    await ForkCapabilitiesAPI.updateProfile(update);

    expect(mock).toHaveBeenCalledWith('/api/fork/profile', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(update),
    });
  });
});
