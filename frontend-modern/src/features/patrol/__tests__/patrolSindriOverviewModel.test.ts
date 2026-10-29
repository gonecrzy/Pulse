import { describe, expect, it } from 'vitest';
import type { AttentionSummary } from '@/api/patrolAttention';
import type { PatrolStatus } from '@/api/patrol';
import type { UnifiedFinding } from '@/stores/aiIntelligence';
import type { CodexLBGatewayStatus } from '@/types/ai';
import {
  buildPatrolSindriOverviewModel,
  isCodexLBGatewayReady,
} from '../patrolSindriOverviewModel';

const healthyStatus = {
  runtime_state: 'active',
  running: false,
  enabled: true,
  last_patrol_at: '2026-10-28T09:00:00Z',
  last_duration_ms: 1200,
  resources_checked: 10,
  findings_count: 2,
  error_count: 0,
  healthy: true,
  interval_ms: 600000,
  fixed_count: 0,
  summary: { critical: 0, warning: 0, watch: 0, info: 0 },
} as PatrolStatus;

const readyGateway: CodexLBGatewayStatus = {
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

const finding = (overrides: Partial<UnifiedFinding> = {}): UnifiedFinding => ({
  id: 'finding-1',
  source: 'ai-patrol',
  resourceId: 'vm-1',
  resourceName: 'database',
  resourceType: 'vm',
  category: 'reliability',
  severity: 'critical',
  title: 'Health check failed',
  description: 'The service returned an error.',
  detectedAt: '2026-10-28T09:00:00Z',
  status: 'active',
  ...overrides,
});

const attentionSummary: AttentionSummary = {
  activeCount: 3,
  openCount: 3,
  acknowledgedCount: 0,
  suppressedCount: 0,
  uncertainCount: 0,
  resolvedCount: 0,
  calm: false,
  coverageState: 'current',
  evaluatedAt: '2026-10-28T09:00:00Z',
};

describe('patrolSindriOverviewModel', () => {
  it('keeps deterministic monitoring operational when the AI gateway is unavailable', () => {
    const model = buildPatrolSindriOverviewModel({
      patrolEnabled: true,
      status: healthyStatus,
      settingsLoaded: true,
      gateway: { ...readyGateway, connectivity_status: 'failed' },
      findings: [finding()],
      attentionSummary,
      pendingActionCount: 1,
    });

    expect(model.monitoring).toMatchObject({ label: 'Operational', operational: true });
    expect(model.gateway).toMatchObject({ ready: false, label: 'Unavailable' });
    expect(model.showMonitoringWithoutAIMessage).toBe(true);
  });

  it('does not classify a completed Patrol run as degraded because infrastructure findings exist', () => {
    const model = buildPatrolSindriOverviewModel({
      patrolEnabled: true,
      status: {
        ...healthyStatus,
        healthy: false,
        summary: { critical: 1, warning: 2, watch: 0, info: 1 },
      },
      settingsLoaded: true,
      gateway: readyGateway,
      findings: [finding()],
      attentionSummary,
      pendingActionCount: 0,
    });

    expect(model.monitoring).toMatchObject({ label: 'Operational', operational: true });
    expect(model.infrastructure).toMatchObject({
      label: 'Issues detected',
      healthy: false,
      detail: '1 critical, 2 warning, 0 watch, 1 informational',
    });
  });

  it('requires the fork capability, endpoint, credential, explicit model, streaming and tools', () => {
    expect(isCodexLBGatewayReady(readyGateway)).toBe(true);
    for (const gateway of [
      { ...readyGateway, capability_available: false },
      { ...readyGateway, endpoint: '' },
      { ...readyGateway, credential_configured: false },
      { ...readyGateway, selected_model: '' },
      { ...readyGateway, tool_calling_readiness: 'not_checked' },
      { ...readyGateway, streaming_readiness: 'failed' },
    ]) {
      expect(isCodexLBGatewayReady(gateway)).toBe(false);
    }
  });

  it('summarizes active findings, running investigations, incident severity and proposals', () => {
    const model = buildPatrolSindriOverviewModel({
      patrolEnabled: true,
      status: healthyStatus,
      settingsLoaded: true,
      gateway: readyGateway,
      findings: [
        finding(),
        finding({ id: 'finding-2', severity: 'warning', investigationStatus: 'running' }),
        finding({ id: 'finding-3', severity: 'info', investigationStatus: 'pending' }),
        finding({ id: 'finding-4', status: 'resolved', severity: 'critical' }),
      ],
      attentionSummary,
      pendingActionCount: 2,
    });

    expect(model).toMatchObject({
      activeFindingCount: 3,
      activeInvestigationCount: 2,
      activeIncidentCount: 3,
      pendingActionCount: 2,
      severityCounts: { critical: 1, warning: 1, watch: 0, info: 1 },
    });
  });

  it('does not infer readiness or healthy monitoring from missing API data', () => {
    const model = buildPatrolSindriOverviewModel({
      patrolEnabled: true,
      status: null,
      settingsLoaded: false,
      gateway: undefined,
      findings: undefined,
      attentionSummary: null,
      pendingActionCount: undefined,
    });

    expect(model.monitoring).toMatchObject({ label: 'Status unavailable', operational: false });
    expect(model.gateway).toMatchObject({ label: 'Status unavailable', ready: false });
    expect(model.activeFindingCount).toBeNull();
    expect(model.activeIncidentCount).toBeNull();
    expect(model.pendingActionCount).toBeNull();
    expect(model.showMonitoringWithoutAIMessage).toBe(false);
  });

  it('does not present an unavailable incident projection as a zero count', () => {
    const model = buildPatrolSindriOverviewModel({
      patrolEnabled: true,
      status: healthyStatus,
      settingsLoaded: true,
      gateway: readyGateway,
      findings: [],
      attentionSummary: { ...attentionSummary, activeCount: 0, coverageState: 'unavailable' },
      pendingActionCount: 0,
    });

    expect(model.activeIncidentCount).toBeNull();
  });
});
