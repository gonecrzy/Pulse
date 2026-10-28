import type { Component } from 'solid-js';
import { ButtonLink } from '@/components/shared/Button';
import { settingsTabPath } from './settingsNavigationModel';

export interface CodexLBGatewayCapabilityNoticeProps {
  capabilityAvailable: boolean;
}

export const CodexLBGatewayCapabilityNotice: Component<CodexLBGatewayCapabilityNoticeProps> = (
  props,
) =>
  !props.capabilityAvailable ? (
    <div
      role="status"
      class="space-y-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-800 dark:bg-amber-950/20 dark:text-amber-100"
    >
      <p>
        Codex-LB gateway capability is disabled. Enable it under Settings → Fork Capabilities before
        testing the connection.
      </p>
      <ButtonLink href={settingsTabPath('system-fork-capabilities')} size="sm" variant="secondary">
        Open Fork Capabilities
      </ButtonLink>
    </div>
  ) : null;
