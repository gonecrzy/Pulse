import { apiFetchJSON } from '@/utils/apiClient';

export const FORK_FEATURE_IDS = [
  'fork.codex_lb_gateway',
  'fork.komodo_read_only',
  'fork.deterministic_triage',
  'fork.ai_investigation',
  'fork.governed_actions',
  'fork.agent_profiles',
  'fork.rbac',
  'fork.audit_logging',
  'fork.advanced_reporting',
  'fork.external_probe',
  'fork.action_safety',
  'fork.operational_cases',
  'fork.long_term_metrics',
  'fork.discord_ops',
] as const;

export type ForkFeatureID = (typeof FORK_FEATURE_IDS)[number];

export interface ForkCapability {
  id: ForkFeatureID;
  available: boolean;
  implemented: boolean;
  enabled: boolean;
  status: 'supported' | 'experimental' | 'unavailable';
  source: 'fork_public' | 'unavailable';
}

export interface ForkCapabilitySnapshot {
  profile_version: number;
  revision: number;
  capabilities: ForkCapability[];
}

export interface ForkProfile {
  version: number;
  revision: number;
  enabled_features: ForkFeatureID[];
}

export interface ForkProfileUpdate {
  version: number;
  expected_revision: number;
  enabled_features: ForkFeatureID[];
}

export class ForkCapabilitiesAPI {
  private static readonly capabilitiesPath = '/api/fork/capabilities';
  private static readonly profilePath = '/api/fork/profile';

  static getCapabilities(): Promise<ForkCapabilitySnapshot> {
    return apiFetchJSON<ForkCapabilitySnapshot>(this.capabilitiesPath);
  }

  static getProfile(): Promise<ForkProfile> {
    return apiFetchJSON<ForkProfile>(this.profilePath);
  }

  static updateProfile(update: ForkProfileUpdate): Promise<ForkCapabilitySnapshot> {
    return apiFetchJSON<ForkCapabilitySnapshot>(this.profilePath, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(update),
    });
  }
}
