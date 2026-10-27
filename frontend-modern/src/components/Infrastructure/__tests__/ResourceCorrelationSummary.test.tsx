import { cleanup, fireEvent, render, screen, within } from '@solidjs/testing-library';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ResourceCorrelationSummary } from '../ResourceCorrelationSummary';

describe('ResourceCorrelationSummary', () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it('ages relationship and correlation last-seen labels on the shared clock', async () => {
    const start = Date.parse('2026-10-16T22:00:00Z');
    vi.useFakeTimers({ now: start });
    const minuteAgo = new Date(start - 60_000).toISOString();
    render(() => (
      <ResourceCorrelationSummary
        relationships={[
          {
            sourceId: 'node:pve-1',
            targetId: 'vm-child',
            type: 'runs_on',
            confidence: 1,
            active: true,
            discoverer: 'proxmox_adapter',
            observedAt: minuteAgo,
            lastSeenAt: minuteAgo,
          },
        ]}
        correlations={[
          {
            source_id: 'storage-1',
            source_name: 'Storage 1',
            source_type: 'storage',
            target_id: 'host-1',
            target_name: 'Host 1',
            target_type: 'vm',
            event_pattern: 'disk_full -> restart',
            occurrences: 2,
            avg_delay: 125000000000,
            confidence: 0.875,
            last_seen: minuteAgo,
            description: 'Disk pressure often precedes restarts',
          },
        ]}
        showLastSeen
      />
    ));

    expect(screen.getAllByText(/last seen 1m ago/)).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(2 * 3_600_000);
    expect(screen.queryAllByText(/last seen 1m ago/)).toHaveLength(0);
    expect(screen.getAllByText(/last seen 2h ago/)).toHaveLength(2);
  });

  it('renders learned correlations with canonical labels and totals', () => {
    render(() => (
      <ResourceCorrelationSummary
        title="Learned correlations"
        correlations={[
          {
            source_id: 'storage-1',
            source_name: 'Storage 1',
            source_type: 'storage',
            target_id: 'host-1',
            target_name: 'Host 1',
            target_type: 'vm',
            event_pattern: 'disk_full -> restart',
            occurrences: 2,
            avg_delay: 125000000000,
            confidence: 0.875,
            last_seen: '2026-03-01T00:15:00Z',
            description: 'Disk pressure often precedes restarts',
          },
          {
            source_id: 'storage-2',
            source_name: 'Storage 2',
            source_type: 'storage',
            target_id: 'host-2',
            target_name: 'Host 2',
            target_type: 'vm',
            event_pattern: 'ALERT → ALERT',
            occurrences: 1,
            avg_delay: 125000000000,
            confidence: 0.5,
            last_seen: '2026-03-01T00:10:00Z',
            description: 'Alerts often cluster together',
          },
        ]}
        summaryText="5 total"
      />
    ));

    expect(screen.getByText('Learned correlations')).toBeInTheDocument();
    expect(screen.getByText('5 total')).toBeInTheDocument();
    // Cross-jump links to the legacy /infrastructure surface were retired;
    // labels render as plain text by default in the platform-first layout.
    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(screen.getByText('Storage 1')).toBeInTheDocument();
    expect(screen.getByText('Host 1')).toBeInTheDocument();
    expect(screen.getByText('Disk Full → Restart')).toBeInTheDocument();
    const alertPattern = screen.getByText('Alert → Alert');
    expect(alertPattern).toBeInTheDocument();
    expect(alertPattern.className).not.toContain('uppercase');
    expect(screen.getByText(/2 occurrences · avg delay 2m · 88% confidence/)).toBeInTheDocument();
    expect(screen.getByText('Disk pressure often precedes restarts')).toBeInTheDocument();
    expect(screen.queryByText(/last seen/i)).toBeNull();
  });

  it('renders correlation context with dependency and dependent links', () => {
    render(() => (
      <ResourceCorrelationSummary
        title="Correlation context"
        relationships={[
          {
            sourceId: 'node:pve-1',
            targetId: 'vm-child',
            type: 'runs_on',
            confidence: 1,
            active: true,
            discoverer: 'proxmox_adapter',
            observedAt: '2026-03-18T12:00:00Z',
            lastSeenAt: '2026-03-18T12:05:00Z',
          },
        ]}
        dependencies={['storage-1']}
        dependents={['vm-child']}
        resolveResourceLabel={(resourceId) =>
          resourceId === 'node:pve-1'
            ? 'PVE 1'
            : resourceId === 'storage-1'
              ? 'Storage 1 alias'
              : resourceId === 'vm-child'
                ? 'VM Child'
                : resourceId
        }
        correlations={[
          {
            source_id: 'storage-1',
            source_name: 'Storage 1',
            source_type: 'storage',
            target_id: 'host-1',
            target_name: 'Host 1',
            target_type: 'vm',
            event_pattern: 'disk_full -> restart',
            occurrences: 2,
            avg_delay: 125000000000,
            confidence: 0.875,
            last_seen: '2026-03-01T00:15:00Z',
            description: 'Disk pressure often precedes restarts',
          },
        ]}
        showLastSeen
      />
    ));

    expect(screen.getByText('Correlation context')).toBeInTheDocument();
    expect(
      screen.getByText('1 canonical relationship · 1 dependency · 1 dependent · 1 correlation'),
    ).toBeInTheDocument();
    expect(screen.getByText('Direct relationships')).toBeInTheDocument();
    expect(screen.getByText('Correlations')).toBeInTheDocument();
    const suggestions = screen.getByText('AI relationship suggestions (2)');
    expect(suggestions).toBeInTheDocument();
    const suggestionContext = screen.getByTestId('resource-ai-relationship-suggestions');
    expect(within(suggestionContext).getByText('Suggested dependencies')).not.toBeVisible();
    fireEvent.click(suggestions);
    expect(
      within(suggestionContext).getByText(
        'These analysis results are candidate paths. They are not provider-verified dependencies or confirmed impact.',
      ),
    ).toBeVisible();
    expect(within(suggestionContext).getByText('Suggested dependencies')).toBeVisible();
    expect(within(suggestionContext).getByText('Potentially used by')).toBeVisible();
    // Cross-jump links to /infrastructure were retired; labels render as
    // plain text by default in the platform-first layout.
    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(screen.getByText('PVE 1')).toBeInTheDocument();
    expect(within(suggestionContext).getByText('VM Child')).toBeVisible();
    expect(within(suggestionContext).getByText('Storage 1 alias')).toBeVisible();
    const relationshipType = screen.getByText('Runs On');
    expect(relationshipType).toBeInTheDocument();
    expect(relationshipType.className).not.toContain('uppercase');
    expect(screen.getByText(/100% confidence · Proxmox Adapter · last seen/)).toBeInTheDocument();
    expect(screen.getAllByText(/last seen/i).length).toBeGreaterThanOrEqual(2);
  });

  it('labels stale evidence on a direct provider relationship', () => {
    render(() => (
      <ResourceCorrelationSummary
        selectedResourceId="vm-www-dmz"
        relationships={[
          {
            sourceId: 'vm-www-dmz',
            targetId: 'storage-local-zfs',
            type: 'stores_on',
            confidence: 1,
            active: true,
            evidenceSource: 'proxmox',
            evidenceFreshness: 'stale',
            discoverer: 'proxmox_vm_config',
            observedAt: '2026-10-27T00:00:00Z',
            lastSeenAt: '2026-10-27T00:00:00Z',
          },
        ]}
      />
    ));

    expect(screen.getByText('Stale provider evidence')).toBeVisible();
  });

  it('shows related provider state, incidents, and navigable resource details', () => {
    render(() => (
      <ResourceCorrelationSummary
        title="Relationship map"
        relationships={[
          {
            sourceId: 'availability-check',
            targetId: 'komodo-stack',
            type: 'checks',
            confidence: 1,
            evidenceSource: 'availability',
            evidenceFreshness: 'current',
            active: true,
            discoverer: 'availability_adapter',
            observedAt: '2026-10-26T10:00:00Z',
            lastSeenAt: '2026-10-26T10:00:00Z',
          },
        ]}
        relatedResources={[
          {
            id: 'availability-check',
            type: 'network-endpoint',
            name: 'kristi-www-site',
            status: 'offline',
            lastSeen: '2026-10-26T10:00:00Z',
            sources: ['availability'],
            sourceStatus: {
              availability: { status: 'online', lastSeen: '2026-10-26T10:00:00Z' },
            },
            canonicalIdentity: {
              primaryId: 'availability:kristi-www-site',
              aliases: ['kristi-www-site'],
            },
            availability: {
              targetId: 'kristi-www-site',
              available: false,
              lastChecked: '2026-10-26T10:00:00Z',
              applicationStatusCode: 503,
            },
            incidents: [
              {
                code: 'availability_unreachable',
                severity: 'critical',
                summary: 'HTTPS returned 503',
              },
            ],
            incidentCount: 1,
          },
        ]}
        buildResourceHref={(id) => `/standalone/availability?resource=${id}`}
        showLastSeen
      />
    ));

    expect(
      screen.getByRole('link', { name: 'Open related resource kristi-www-site' }),
    ).toHaveAttribute('href', '/standalone/availability?resource=availability-check');
    expect(screen.getByText('Providers: availability')).toBeInTheDocument();
    expect(screen.getByText(/availability: online/)).toBeInTheDocument();
    expect(screen.getByText(/HTTP 503/)).toBeInTheDocument();
    expect(screen.getByText(/HTTPS returned 503/)).toBeInTheDocument();
    expect(screen.getByTestId('resource-related-resource')).toHaveTextContent(
      'Provider identity links: kristi-www-site',
    );
    expect(screen.getByText(/Availability: failed/)).toBeInTheDocument();
  });

  it('keeps shared host disks and sibling VMs collapsed while showing Stack monitoring context', () => {
    const vmId = 'vm-www';
    const hostId = 'pve-host';
    const stackId = 'stack-kristi';
    const checkId = 'check-kristi';
    const diskId = 'disk-samsung-a';
    const siblingId = 'vm-alfheim';
    const proxyId = 'service-reverse-proxy';
    const edge = (sourceId: string, targetId: string, type: string) => ({
      sourceId,
      targetId,
      type,
      confidence: 1,
      active: true,
      discoverer: 'proxmox_adapter',
      observedAt: '2026-10-27T10:00:00Z',
      lastSeenAt: '2026-10-27T10:00:00Z',
    });
    render(() => (
      <ResourceCorrelationSummary
        title="Relationship map"
        selectedResourceId={vmId}
        relationships={[edge(vmId, hostId, 'runs_on'), edge(stackId, vmId, 'hosted_by')]}
        relatedResources={[
          { id: hostId, type: 'agent', name: 'pve1', status: 'online' },
          { id: stackId, type: 'komodo-stack', name: 'kristi-www', status: 'online' },
        ]}
        contextResources={[
          {
            id: vmId,
            type: 'vm',
            name: 'www-DMZ',
            status: 'online',
            parentId: hostId,
            sources: ['proxmox', 'komodo'],
            sourceStatus: { proxmox: { status: 'stale' } },
          },
          { id: hostId, type: 'agent', name: 'pve1', status: 'online', sources: ['proxmox'] },
          {
            id: stackId,
            type: 'komodo-stack',
            name: 'kristi-www',
            status: 'online',
            parentId: vmId,
          },
          {
            id: checkId,
            type: 'network-endpoint',
            name: 'kristi-www HTTPS',
            status: 'offline',
            parentId: stackId,
            incidentCount: 1,
            incidentSummary: 'HTTPS returned 503',
          },
          {
            id: diskId,
            type: 'physical_disk',
            name: 'Samsung SSD',
            status: 'online',
            parentId: hostId,
          },
          { id: siblingId, type: 'vm', name: 'alfheim', status: 'online', parentId: hostId },
          { id: proxyId, type: 'komodo-deployment', name: 'edge-proxy', status: 'online' },
        ]}
        serviceMonitoringRelationships={[
          {
            relationship: edge(checkId, stackId, 'checks'),
            scope: 'indirect',
            classification: 'service_monitoring',
            distance: 2,
            traversalPath: [vmId, stackId, checkId],
            impactAssessment: 'service_observation',
          },
        ]}
        crossBranchRelationships={[
          {
            relationship: edge(stackId, proxyId, 'routes_to'),
            scope: 'indirect',
            classification: 'cross_branch_relationship',
            distance: 2,
            traversalPath: [vmId, stackId, proxyId],
            impactAssessment: 'potential_only',
          },
        ]}
        infrastructureRelationships={[
          {
            relationship: edge(diskId, hostId, 'mounted_to'),
            scope: 'indirect',
            classification: 'shared_infrastructure',
            distance: 2,
            traversalPath: [vmId, hostId, diskId],
            impactAssessment: 'shared_failure_domain_candidate',
          },
          {
            relationship: edge(siblingId, hostId, 'runs_on'),
            scope: 'indirect',
            classification: 'shared_infrastructure',
            distance: 2,
            traversalPath: [vmId, hostId, siblingId],
            impactAssessment: 'shared_failure_domain_candidate',
          },
        ]}
        buildResourceHref={(id) => `/resources/${id}`}
        showLastSeen
      />
    ));

    expect(screen.getByText('Direct relationships')).toBeInTheDocument();
    expect(screen.getByText('Monitoring attached to hosted services')).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'Open monitored service resource kristi-www HTTPS' }),
    ).toHaveAttribute('href', `/resources/${checkId}`);
    expect(screen.getByText(`Path: www-DMZ → kristi-www → kristi-www HTTPS`)).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /Samsung SSD/ })).toBeNull();
    expect(screen.queryByRole('link', { name: /alfheim/ })).toBeNull();
    expect(
      screen.queryByRole('link', { name: 'Open cross-branch resource edge-proxy' }),
    ).toBeNull();
    expect(screen.getByText('Infrastructure hierarchy')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open topology resource pve1' })).toBeInTheDocument();
    expect(screen.getByTestId(`resource-topology-tree-node-${hostId}`)).toHaveTextContent(
      'Proxmox physical host',
    );
    expect(screen.queryByRole('link', { name: 'Open topology resource alfheim' })).toBeNull();

    const expandNode = (id: string) => {
      const details = screen.getByTestId(`resource-topology-tree-node-${id}`);
      fireEvent.click(details.querySelector('summary')!);
      // jsdom toggles <details>.open on click but does not emit its native toggle event.
      fireEvent(details, new Event('toggle'));
      return details;
    };
    expandNode(hostId);
    expect(
      screen.getByRole('link', { name: 'Open topology resource alfheim' }),
    ).toBeInTheDocument();
    expect(screen.getByTestId(`resource-topology-tree-node-${vmId}`)).toBeInTheDocument();
    expect(screen.getByTestId(`resource-topology-tree-node-${vmId}`)).toHaveTextContent(
      'Stale observations: proxmox',
    );
    expect(screen.getByTestId(`resource-topology-tree-node-${vmId}`)).toHaveTextContent(
      'VM · Proxmox + Komodo Server',
    );
    expandNode(vmId);
    expect(screen.getByTestId(`resource-topology-tree-node-${stackId}`)).toBeInTheDocument();
    expandNode(stackId);
    expect(screen.getByTestId(`resource-topology-tree-entry-${checkId}`)).toHaveTextContent(
      /offline1 active incident/,
    );

    const crossBranchDisclosure = screen.getByTestId('resource-cross-branch-context');
    fireEvent.click(crossBranchDisclosure.querySelector('summary')!);
    fireEvent(crossBranchDisclosure, new Event('toggle'));
    expect(
      screen.getByRole('link', { name: 'Open cross-branch resource edge-proxy' }),
    ).toBeInTheDocument();
    expect(screen.getByText('Path: www-DMZ → kristi-www → edge-proxy')).toBeInTheDocument();

    const infrastructureDisclosure = screen.getByTestId('resource-shared-infrastructure-context');
    fireEvent.click(screen.getByText(/Shared infrastructure context/));
    // jsdom toggles <details>.open on click but does not emit its native toggle event.
    fireEvent(infrastructureDisclosure, new Event('toggle'));
    expect(
      screen.getByRole('link', {
        name: /Open shared infrastructure resource Samsung SSD · physical_disk/,
      }),
    ).toHaveAttribute('href', `/resources/${diskId}`);
    expect(
      screen.getByRole('link', { name: /Open shared infrastructure resource alfheim · vm/ }),
    ).toHaveAttribute('href', `/resources/${siblingId}`);
    expect(
      screen.getAllByText('This path may share a failure domain. Impact is unconfirmed.'),
    ).toHaveLength(2);
  });
});
