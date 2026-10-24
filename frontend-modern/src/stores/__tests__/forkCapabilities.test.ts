import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ForkCapabilitiesAPI } from '@/api/forkCapabilities';
import {
  forkCapabilitySnapshot,
  forkCapabilitiesError,
  hasForkCapability,
  loadForkCapabilities,
  loadForkProfile,
  updateForkProfile,
  forkProfile,
  resetForkCapabilities,
} from '@/stores/forkCapabilities';

vi.mock('@/api/forkCapabilities', async () => {
  const actual =
    await vi.importActual<typeof import('@/api/forkCapabilities')>('@/api/forkCapabilities');
  return {
    ...actual,
    ForkCapabilitiesAPI: {
      getCapabilities: vi.fn(),
      getProfile: vi.fn(),
      updateProfile: vi.fn(),
    },
  };
});

describe('fork capability store', () => {
  const api = vi.mocked(ForkCapabilitiesAPI);

  beforeEach(() => {
    vi.clearAllMocks();
    api.getCapabilities.mockReset();
    api.getProfile.mockReset();
    api.updateProfile.mockReset();
    resetForkCapabilities();
  });

  it('keeps fork capabilities separate and fail-closed when loading fails', async () => {
    api.getCapabilities.mockRejectedValueOnce(new Error('unavailable'));

    await loadForkCapabilities(true);

    expect(forkCapabilitySnapshot()).toBeNull();
    expect(hasForkCapability('fork.ai_investigation')).toBe(false);
    expect(forkCapabilitiesError()?.message).toBe('unavailable');
  });

  it('publishes resolved server state and versioned profile updates', async () => {
    api.getCapabilities.mockResolvedValueOnce({
      profile_version: 1,
      revision: 2,
      capabilities: [
        { id: 'fork.ai_investigation', available: true, implemented: true, source: 'fork_public' },
        { id: 'fork.governed_actions', available: false, implemented: true, source: 'unavailable' },
      ],
    });
    await loadForkCapabilities(true);
    expect(hasForkCapability('fork.ai_investigation')).toBe(true);
    expect(hasForkCapability('fork.governed_actions')).toBe(false);

    api.updateProfile.mockResolvedValueOnce({
      profile_version: 1,
      revision: 3,
      capabilities: [
        { id: 'fork.ai_investigation', available: true, implemented: true, source: 'fork_public' },
        { id: 'fork.governed_actions', available: true, implemented: true, source: 'fork_public' },
      ],
    });
    await updateForkProfile({
      version: 1,
      expected_revision: 2,
      enabled_features: ['fork.ai_investigation', 'fork.governed_actions'],
    });

    expect(hasForkCapability('fork.governed_actions')).toBe(true);
    expect(forkProfile()).toEqual({
      version: 1,
      revision: 3,
      enabled_features: ['fork.ai_investigation', 'fork.governed_actions'],
    });
  });

  it('loads the administrator profile separately from resolved capabilities', async () => {
    api.getProfile.mockResolvedValueOnce({
      version: 1,
      revision: 4,
      enabled_features: ['fork.ai_investigation'],
    });

    await loadForkProfile();

    expect(forkProfile()?.revision).toBe(4);
    expect(api.getProfile).toHaveBeenCalledTimes(1);
  });

  it('invalidates cached state and ignores capability responses from an older session', async () => {
    let resolveCapabilities:
      | ((value: Awaited<ReturnType<typeof ForkCapabilitiesAPI.getCapabilities>>) => void)
      | undefined;
    api.getCapabilities.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveCapabilities = resolve;
        }),
    );

    const loading = loadForkCapabilities(true);
    resetForkCapabilities();
    resolveCapabilities?.({
      profile_version: 1,
      revision: 2,
      capabilities: [
        { id: 'fork.ai_investigation', available: true, implemented: true, source: 'fork_public' },
      ],
    });
    await loading;

    expect(forkCapabilitySnapshot()).toBeNull();
    expect(forkProfile()).toBeNull();
    expect(hasForkCapability('fork.ai_investigation')).toBe(false);
  });

  it('reloads the server profile and resolved capabilities after a revision conflict', async () => {
    const conflict = Object.assign(new Error('stale'), { status: 409, code: 'stale_fork_profile' });
    api.updateProfile.mockRejectedValueOnce(conflict);
    api.getProfile.mockResolvedValueOnce({
      version: 1,
      revision: 5,
      enabled_features: ['fork.governed_actions'],
    });
    api.getCapabilities.mockResolvedValueOnce({
      profile_version: 1,
      revision: 5,
      capabilities: [
        { id: 'fork.governed_actions', available: true, implemented: true, source: 'fork_public' },
      ],
    });

    await expect(
      updateForkProfile({
        version: 1,
        expected_revision: 4,
        enabled_features: ['fork.ai_investigation'],
      }),
    ).rejects.toBe(conflict);

    expect(forkProfile()).toEqual({
      version: 1,
      revision: 5,
      enabled_features: ['fork.governed_actions'],
    });
    expect(hasForkCapability('fork.governed_actions')).toBe(true);
    expect(forkCapabilitiesError()).toBeNull();
    expect(api.getProfile).toHaveBeenCalledTimes(1);
    expect(api.getCapabilities).toHaveBeenCalledTimes(1);
  });
});
