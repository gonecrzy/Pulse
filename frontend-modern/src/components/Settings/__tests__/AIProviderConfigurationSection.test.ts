import { describe, expect, it } from 'vitest';
import type { AISettings } from '@/types/ai';
import { isProviderConnectionTestBlocked } from '../AIProviderConfigurationSection';

const settingsWithGatewayState = (capabilityAvailable: boolean): AISettings =>
  ({
    codex_lb_gateway: {
      capability_available: capabilityAvailable,
      credential_configured: true,
      allow_private_url: true,
      connectivity_status: 'unavailable',
      tool_calling_readiness: 'unavailable',
      streaming_readiness: 'unavailable',
    },
  }) as AISettings;

describe('isProviderConnectionTestBlocked', () => {
  it('blocks a Codex-LB connection test while its fork capability is disabled', () => {
    expect(
      isProviderConnectionTestBlocked('codex_lb_gateway', settingsWithGatewayState(false)),
    ).toBe(true);
  });

  it('allows gateway tests after activation and leaves other providers unaffected', () => {
    expect(
      isProviderConnectionTestBlocked('codex_lb_gateway', settingsWithGatewayState(true)),
    ).toBe(false);
    expect(isProviderConnectionTestBlocked('openai', settingsWithGatewayState(false))).toBe(false);
  });
});
