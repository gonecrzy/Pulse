import type { AttentionSummary } from '@/api/patrolAttention';
import type { PatrolRunRecord, PatrolStatus } from '@/api/patrol';
import type { UnifiedFinding } from '@/stores/aiIntelligence';
import type { CodexLBGatewayStatus } from '@/types/ai';

export interface PatrolSindriOverviewInput {
  patrolEnabled: boolean;
  status: PatrolStatus | null | undefined;
  settingsLoaded: boolean;
  gateway: CodexLBGatewayStatus | null | undefined;
  findings: readonly UnifiedFinding[] | null | undefined;
  attentionSummary: AttentionSummary | null | undefined;
  pendingActionCount: number | null | undefined;
  lastCompletedRun?: PatrolRunRecord | null;
}

export interface PatrolSindriOverviewModel {
  monitoring: { label: string; operational: boolean };
  gateway: {
    label: string;
    ready: boolean;
    endpointConfigured: boolean;
    credentialConfigured: boolean;
    selectedModel: string;
    connectivity: string;
    toolCalling: string;
    streaming: string;
    lastSuccessfulQualification?: string;
  };
  activeFindingCount: number | null;
  activeInvestigationCount: number | null;
  activeIncidentCount: number | null;
  pendingActionCount: number | null;
  severityCounts: { critical: number; warning: number; watch: number; info: number } | null;
  lastCompletedAt?: string;
  nextPatrolAt?: string;
  showMonitoringWithoutAIMessage: boolean;
}

export function isCodexLBGatewayReady(gateway?: CodexLBGatewayStatus | null): boolean {
  return Boolean(
    gateway?.capability_available &&
    gateway.endpoint?.trim() &&
    gateway.credential_configured &&
    gateway.selected_model?.trim() &&
    gateway.connectivity_status === 'ready' &&
    gateway.tool_calling_readiness === 'ready' &&
    gateway.streaming_readiness === 'ready',
  );
}

function formatGatewayReadiness(value?: string): string {
  const normalized = value?.trim().toLowerCase();
  if (!normalized) return 'Unavailable';
  switch (normalized) {
    case 'ready':
      return 'Ready';
    case 'not_checked':
      return 'Not checked';
    case 'not_configured':
      return 'Not configured';
    case 'model_not_selected':
      return 'Model not selected';
    case 'unavailable':
    case 'failed':
      return 'Unavailable';
    case 'warning':
      return 'Warning';
    default:
      return normalized.replace(/[_-]+/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
  }
}

export function buildPatrolSindriOverviewModel(
  input: PatrolSindriOverviewInput,
): PatrolSindriOverviewModel {
  const status = input.status;
  const monitoringOperational = Boolean(
    status?.healthy &&
    input.patrolEnabled &&
    status.enabled &&
    status.runtime_state !== 'disabled' &&
    status.runtime_state !== 'unavailable',
  );
  let monitoringLabel = 'Status unavailable';
  if (status) {
    if (!input.patrolEnabled || !status.enabled || status.runtime_state === 'disabled') {
      monitoringLabel = 'Paused';
    } else if (status.runtime_state === 'unavailable') {
      monitoringLabel = 'Unavailable';
    } else if (status.runtime_state === 'blocked') {
      monitoringLabel = 'Blocked';
    } else if (monitoringOperational) {
      monitoringLabel = 'Operational';
    } else {
      monitoringLabel = 'Degraded';
    }
  }

  const ready = isCodexLBGatewayReady(input.gateway);
  const gatewayLabel = !input.settingsLoaded
    ? 'Status unavailable'
    : ready
      ? 'Ready'
      : input.gateway?.capability_available
        ? !input.gateway.endpoint?.trim() || !input.gateway.credential_configured
          ? 'Not configured'
          : !input.gateway.selected_model?.trim()
            ? 'Model not selected'
            : input.gateway.connectivity_status !== 'ready'
              ? formatGatewayReadiness(input.gateway.connectivity_status)
              : 'Not qualified'
        : 'Unavailable';

  const activeFindings = input.findings?.filter((finding) => finding.status === 'active');
  const severityCounts = activeFindings
    ? {
        critical: activeFindings.filter((finding) => finding.severity === 'critical').length,
        warning: activeFindings.filter((finding) => finding.severity === 'warning').length,
        watch: activeFindings.filter((finding) => finding.severity === 'watch').length,
        info: activeFindings.filter((finding) => finding.severity === 'info').length,
      }
    : null;

  return {
    monitoring: { label: monitoringLabel, operational: monitoringOperational },
    gateway: {
      label: gatewayLabel,
      ready,
      endpointConfigured: Boolean(input.gateway?.endpoint?.trim()),
      credentialConfigured: input.gateway?.credential_configured ?? false,
      selectedModel: input.gateway?.selected_model?.trim() ?? '',
      connectivity: formatGatewayReadiness(input.gateway?.connectivity_status),
      toolCalling: formatGatewayReadiness(input.gateway?.tool_calling_readiness),
      streaming: formatGatewayReadiness(input.gateway?.streaming_readiness),
      lastSuccessfulQualification: input.gateway?.last_successful_qualification,
    },
    activeFindingCount: activeFindings?.length ?? null,
    activeInvestigationCount:
      activeFindings?.filter(
        (finding) =>
          finding.investigationStatus === 'pending' || finding.investigationStatus === 'running',
      ).length ?? null,
    activeIncidentCount:
      input.attentionSummary?.coverageState === 'unavailable'
        ? null
        : (input.attentionSummary?.activeCount ?? null),
    pendingActionCount:
      typeof input.pendingActionCount === 'number' && Number.isFinite(input.pendingActionCount)
        ? Math.max(0, Math.trunc(input.pendingActionCount))
        : null,
    severityCounts,
    lastCompletedAt: input.lastCompletedRun?.completed_at || undefined,
    nextPatrolAt: status?.next_patrol_at,
    showMonitoringWithoutAIMessage: monitoringOperational && input.settingsLoaded && !ready,
  };
}
