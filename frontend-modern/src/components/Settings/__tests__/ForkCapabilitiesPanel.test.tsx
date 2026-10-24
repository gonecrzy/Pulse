import { fireEvent, render, screen, waitFor } from '@solidjs/testing-library';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ForkCapabilitiesAPI } from '@/api/forkCapabilities';
import { ForkCapabilitiesPanel } from '../ForkCapabilitiesPanel';
import { resetForkCapabilities } from '@/stores/forkCapabilities';

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

describe('ForkCapabilitiesPanel', () => {
  const api = vi.mocked(ForkCapabilitiesAPI);

  beforeEach(() => {
    vi.clearAllMocks();
    api.getCapabilities.mockReset();
    api.getProfile.mockReset();
    api.updateProfile.mockReset();
    resetForkCapabilities();
  });

  it('shows capabilities as unavailable after the authenticated read fails', async () => {
    api.getCapabilities.mockRejectedValueOnce(new Error('request denied'));
    api.getProfile.mockRejectedValueOnce(new Error('request denied'));

    render(() => <ForkCapabilitiesPanel canManage={() => true} />);

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'No capability is treated as enabled',
    );
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  });

  it('shows server capability facts and reports stale profile conflicts', async () => {
    api.getCapabilities
      .mockResolvedValueOnce({
        profile_version: 1,
        revision: 3,
        capabilities: [
          {
            id: 'fork.ai_investigation',
            available: true,
            implemented: true,
            source: 'fork_public',
          },
          {
            id: 'fork.governed_actions',
            available: false,
            implemented: true,
            source: 'unavailable',
          },
          { id: 'fork.action_safety', available: false, implemented: false, source: 'unavailable' },
        ],
      })
      .mockResolvedValueOnce({
        profile_version: 1,
        revision: 4,
        capabilities: [
          {
            id: 'fork.ai_investigation',
            available: false,
            implemented: true,
            source: 'unavailable',
          },
          {
            id: 'fork.governed_actions',
            available: true,
            implemented: true,
            source: 'fork_public',
          },
          { id: 'fork.action_safety', available: false, implemented: false, source: 'unavailable' },
        ],
      });
    api.getProfile
      .mockResolvedValueOnce({
        version: 1,
        revision: 3,
        enabled_features: ['fork.ai_investigation'],
      })
      .mockResolvedValueOnce({
        version: 1,
        revision: 4,
        enabled_features: ['fork.governed_actions'],
      });
    api.updateProfile.mockRejectedValueOnce(
      Object.assign(new Error('Fork profile changed; reload it before saving'), {
        status: 409,
        code: 'stale_fork_profile',
      }),
    );

    render(() => <ForkCapabilitiesPanel canManage={() => true} />);

    expect(await screen.findByText('Personal fork capabilities')).toBeInTheDocument();
    expect(screen.getByText(/Public fork/)).toBeInTheDocument();
    expect(screen.getByText(/Not implemented/)).toBeInTheDocument();
    expect(screen.getAllByRole('checkbox')).toHaveLength(2);

    fireEvent.click(screen.getByLabelText('Disabled in fork profile'));
    fireEvent.click(screen.getByRole('button', { name: 'Save fork profile' }));

    await waitFor(() =>
      expect(api.updateProfile).toHaveBeenCalledWith({
        version: 1,
        expected_revision: 3,
        enabled_features: ['fork.ai_investigation', 'fork.governed_actions'],
      }),
    );
    expect(await screen.findByText(/profile changed on the server/i)).toBeInTheDocument();
    await waitFor(() => expect(api.getProfile).toHaveBeenCalledTimes(2));
    expect(screen.getByLabelText('Enabled in fork profile')).toBeChecked();
  });

  it('does not expose profile editing to a non-administrator', async () => {
    api.getCapabilities.mockResolvedValueOnce({
      profile_version: 1,
      revision: 1,
      capabilities: [
        { id: 'fork.ai_investigation', available: true, implemented: true, source: 'fork_public' },
      ],
    });

    render(() => <ForkCapabilitiesPanel canManage={() => false} />);

    expect(
      await screen.findByText('A settings administrator can manage the validated fork profile.'),
    ).toBeInTheDocument();
    expect(api.getProfile).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Save fork profile' })).not.toBeInTheDocument();
  });
});
