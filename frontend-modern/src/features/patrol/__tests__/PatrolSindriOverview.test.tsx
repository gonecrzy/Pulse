import { cleanup, render, screen, within } from '@solidjs/testing-library';
import { Route, Router } from '@solidjs/router';
import { afterEach, describe, expect, it } from 'vitest';
import type { AttentionSummary } from '@/api/patrolAttention';
import type { PatrolStatus } from '@/api/patrol';
import type { UnifiedFinding } from '@/stores/aiIntelligence';
import type { CodexLBGatewayStatus } from '@/types/ai';
import { settingsTabPath } from '@/components/Settings/settingsNavigationModel';
import { PatrolSindriOverview } from '../PatrolSindriOverview';

const status = {
  runtime_state: 'active',
  running: false,
  enabled: true,
  next_patrol_at: '2026-10-28T10:00:00Z',
  last_duration_ms: 1000,
  resources_checked: 1,
  findings_count: 1,
  error_count: 0,
  healthy: true,
  interval_ms: 600000,
  fixed_count: 0,
  summary: { critical: 0, warning: 0, watch: 0, info: 0 },
} as PatrolStatus;

const gateway: CodexLBGatewayStatus = {
  capability_available: true,
  endpoint: 'https://gateway.example/v1',
  selected_model: 'codex/model-a',
  credential_configured: true,
  allow_private_url: false,
  connectivity_status: 'ready',
  tool_calling_readiness: 'ready',
  streaming_readiness: 'ready',
  last_successful_qualification: '2026-10-28T09:00:00Z',
};

const finding: UnifiedFinding = {
  id: 'finding-1',
  source: 'ai-patrol',
  resourceId: 'vm-1',
  resourceName: 'Database VM',
  resourceType: 'vm',
  category: 'reliability',
  severity: 'critical',
  title: 'Health check failed',
  description: 'Health check returned an error.',
  detectedAt: '2026-10-28T09:00:00Z',
  status: 'active',
  investigationStatus: 'running',
};

const attentionSummary: AttentionSummary = {
  activeCount: 2,
  openCount: 2,
  acknowledgedCount: 0,
  suppressedCount: 0,
  uncertainCount: 0,
  resolvedCount: 0,
  calm: false,
  coverageState: 'current',
  evaluatedAt: '2026-10-28T09:00:00Z',
};

const renderOverview = (options: {
  patrolStatus?: PatrolStatus | null;
  gatewayStatus?: CodexLBGatewayStatus;
  settingsLoaded?: boolean;
  patrolEnabled?: boolean;
  findings?: UnifiedFinding[];
  summary?: AttentionSummary | null;
  pendingActionCount?: number;
}) => {
  const props = {
    patrolStatus: () => ('patrolStatus' in options ? options.patrolStatus : status),
    patrolEnabled: () => options.patrolEnabled ?? true,
    gatewayStatus: () => ('gatewayStatus' in options ? options.gatewayStatus : gateway),
    settingsLoaded: () => options.settingsLoaded ?? true,
    findings: () => ('findings' in options ? options.findings : [finding]),
    attentionSummary: () => ('summary' in options ? options.summary : attentionSummary),
    pendingActionCount: () => ('pendingActionCount' in options ? options.pendingActionCount : 1),
    lastCompletedRun: () => null,
  };
  return render(() => (
    <Router>
      <Route path="*" component={() => <PatrolSindriOverview {...props} />} />
    </Router>
  ));
};

describe('PatrolSindriOverview', () => {
  afterEach(() => cleanup());

  it('separates deterministic monitoring health from AI gateway readiness', () => {
    renderOverview({ gatewayStatus: { ...gateway, connectivity_status: 'failed' } });

    expect(screen.getByText('Deterministic monitoring')).toBeInTheDocument();
    expect(screen.getByText('Operational')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent(
      'Monitoring operational — AI investigation unavailable.',
    );
    expect(
      screen.getByRole('heading', { name: 'AI investigation · Codex-LB' }),
    ).toBeInTheDocument();
    expect(screen.getAllByText('Unavailable').length).toBeGreaterThan(0);
  });

  it('shows selected model, masked credential state, qualification dimensions and settings link', () => {
    renderOverview({});

    expect(
      screen.getByText((_, element) => element?.textContent === 'Selected model: codex/model-a'),
    ).toBeInTheDocument();
    expect(screen.getByText('Credential configured')).toBeInTheDocument();
    expect(screen.getByText('Connectivity')).toBeInTheDocument();
    expect(screen.getByText('Tool calling')).toBeInTheDocument();
    expect(screen.getByText('Streaming')).toBeInTheDocument();
    expect(screen.getByText(/Last successful qualification/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'AI gateway settings' })).toHaveAttribute(
      'href',
      settingsTabPath('system-ai'),
    );
    expect(document.body.textContent).not.toContain('gateway-secret');
  });

  it('summarizes current findings, investigations, incident counts and pending actions', () => {
    renderOverview({});

    expect(screen.getByText('1 active finding')).toBeInTheDocument();
    expect(screen.getByText('1 active investigation')).toBeInTheDocument();
    const incidentCard = screen.getByRole('heading', { name: 'Active incidents' }).parentElement!;
    const actionCard = screen.getByRole('heading', { name: 'Action approvals' }).parentElement!;
    expect(within(incidentCard).getByText('2')).toBeInTheDocument();
    expect(within(actionCard).getByText('1')).toBeInTheDocument();
    expect(screen.getByLabelText('Active finding severity')).toHaveTextContent('Critical 1');
  });

  it('shows unavailable data without fabricating zero counts or gateway readiness', () => {
    renderOverview({
      patrolStatus: null,
      gatewayStatus: undefined,
      settingsLoaded: false,
      findings: undefined,
      summary: null,
      pendingActionCount: undefined,
    });

    expect(screen.getAllByText('Status unavailable').length).toBeGreaterThan(0);
    expect(screen.getByText('Findings not loaded')).toBeInTheDocument();
    expect(screen.getByText('Incident count unavailable')).toBeInTheDocument();
    expect(screen.getByText('Action count unavailable')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
