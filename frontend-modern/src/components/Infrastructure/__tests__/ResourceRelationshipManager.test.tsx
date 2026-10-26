import { cleanup, fireEvent, render, screen, waitFor } from '@solidjs/testing-library';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Resource } from '@/types/resource';

const { api, resources, refetch } = vi.hoisted(() => ({
  api: {
    getIdentityLinks: vi.fn(),
    addRelationship: vi.fn(),
    removeRelationship: vi.fn(),
    linkIdentity: vi.fn(),
    unlinkIdentity: vi.fn(),
  },
  resources: vi.fn(),
  refetch: vi.fn(),
}));

vi.mock('@/api/resources', () => ({ ResourceAPI: api }));
vi.mock('@/utils/toast', () => ({ showSuccess: vi.fn() }));
vi.mock('@/hooks/useUnifiedResources', () => ({
  useUnifiedResources: () => ({
    resources: () => resources(),
    loading: () => false,
    error: () => undefined,
    refetch,
  }),
}));

import {
  ResourceRelationshipManager,
  isOperatorRelationshipCompatible,
} from '../ResourceRelationshipManager';

const fixture = (): Resource[] => [
  {
    id: 'pve-vm',
    type: 'vm',
    name: 'app-vm',
    displayName: 'app-vm',
    platformId: '101',
    platformType: 'proxmox-pve',
    sourceType: 'api',
    sources: ['proxmox'],
    status: 'running',
    lastSeen: 1,
    proxmox: { sourceId: 'vmid:101' },
    identity: { hostname: 'app.internal', ips: ['10.0.0.4'] },
    relationships: [
      {
        id: 'operator-edge',
        sourceId: 'pve-vm',
        targetId: 'agent-host',
        type: 'runs_on',
        confidence: 1,
        active: true,
        discoverer: 'operator_store',
        observedAt: '2026-10-26T00:00:00Z',
        lastSeenAt: '2026-10-26T00:00:00Z',
        reason: 'Confirmed placement',
      },
    ],
  },
  {
    id: 'agent-host',
    type: 'agent',
    name: 'node-1',
    displayName: 'node-1',
    platformId: 'node-1',
    platformType: 'agent',
    sourceType: 'agent',
    sources: ['agent'],
    status: 'online',
    lastSeen: 1,
  },
  {
    id: 'k8s-ingress',
    type: 'k8s-ingress',
    name: 'web',
    displayName: 'web ingress',
    platformId: 'web',
    platformType: 'kubernetes',
    sourceType: 'api',
    sources: ['kubernetes'],
    status: 'online',
    lastSeen: 1,
    relationships: [
      {
        id: 'provider-edge',
        sourceId: 'k8s-ingress',
        targetId: 'k8s-service',
        type: 'routes_to',
        confidence: 1,
        active: true,
        discoverer: 'kubernetes_adapter',
        evidenceSource: 'kubernetes',
        observedAt: '2026-10-26T00:00:00Z',
        lastSeenAt: '2026-10-26T00:00:00Z',
        evidenceFreshness: 'current',
      },
    ],
  },
  {
    id: 'k8s-service',
    type: 'k8s-service',
    name: 'api',
    displayName: 'api service',
    platformId: 'api',
    platformType: 'kubernetes',
    sourceType: 'api',
    sources: ['kubernetes'],
    status: 'online',
    lastSeen: 1,
  },
];

beforeEach(() => {
  resources.mockReturnValue(fixture());
  api.getIdentityLinks.mockResolvedValue({ links: [] });
  api.addRelationship.mockResolvedValue({ relationship: { id: 'new-edge' } });
  api.linkIdentity.mockResolvedValue({ status: 'linked', message: 'ok' });
  refetch.mockResolvedValue(resources());
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('ResourceRelationshipManager', () => {
  it('enforces directed relationship compatibility', () => {
    expect(isOperatorRelationshipCompatible('vm', 'agent', 'runs_on')).toBe(true);
    expect(isOperatorRelationshipCompatible('agent', 'vm', 'runs_on')).toBe(false);
    expect(isOperatorRelationshipCompatible('k8s-ingress', 'k8s-service', 'routes_to')).toBe(true);
    expect(isOperatorRelationshipCompatible('vm', 'k8s-service', 'routes_to')).toBe(false);
  });

  it('shows provider edges as read-only and operator edges as editable', async () => {
    render(() => <ResourceRelationshipManager />);
    fireEvent.click(
      screen.getByRole('button', { name: 'Add or manage infrastructure relationships' }),
    );

    await waitFor(() => expect(screen.getAllByTestId('relationship-row')).toHaveLength(2));
    expect(screen.getByText('Operator confirmed')).toBeInTheDocument();
    expect(screen.getByText(/Provider observed · kubernetes_adapter/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Remove' })).toHaveLength(1);
  });

  it('requires explicit same_as confirmation and preserves primary choice in the request', async () => {
    render(() => <ResourceRelationshipManager />);
    fireEvent.click(
      screen.getByRole('button', { name: 'Add or manage infrastructure relationships' }),
    );
    fireEvent.click(screen.getByRole('tab', { name: 'Identity (same_as)' }));

    fireEvent.change(screen.getByLabelText('Primary resource'), { target: { value: 'pve-vm' } });
    fireEvent.change(screen.getByLabelText('Other resource to merge'), {
      target: { value: 'agent-host' },
    });
    fireEvent.input(screen.getByLabelText('Audit reason'), {
      target: { value: 'Same machine confirmed' },
    });
    expect(screen.getByText(/native IDs: 101, vmid:101/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm identity link' })).toBeDisabled();

    fireEvent.click(screen.getByLabelText(/I reviewed both records/));
    fireEvent.click(screen.getByRole('button', { name: 'Confirm identity link' }));

    await waitFor(() =>
      expect(api.linkIdentity).toHaveBeenCalledWith(
        'pve-vm',
        'agent-host',
        'Same machine confirmed',
        false,
      ),
    );
    expect(screen.getByText(/PVE record as primary/)).toBeInTheDocument();
    expect(refetch).toHaveBeenCalled();
    expect(api.addRelationship).not.toHaveBeenCalled();
  });

  it('filters resource-picker options by search, provider, and resource type', () => {
    render(() => <ResourceRelationshipManager />);
    fireEvent.click(
      screen.getByRole('button', { name: 'Add or manage infrastructure relationships' }),
    );
    const sourcePicker = screen.getByLabelText('Source') as HTMLSelectElement;
    const search = screen.getByLabelText('Search resources');

    fireEvent.input(search, { target: { value: 'node-1' } });
    expect(sourcePicker.querySelector('option[value="agent-host"]')).toBeInTheDocument();
    expect(sourcePicker.querySelector('option[value="pve-vm"]')).not.toBeInTheDocument();

    fireEvent.input(search, { target: { value: '' } });
    fireEvent.change(screen.getByLabelText('Provider'), { target: { value: 'kubernetes' } });
    fireEvent.change(screen.getByLabelText('Resource type'), { target: { value: 'k8s-ingress' } });
    expect(sourcePicker.querySelector('option[value="k8s-ingress"]')).toBeInTheDocument();
    expect(sourcePicker.querySelector('option[value="k8s-service"]')).not.toBeInTheDocument();
  });
});
