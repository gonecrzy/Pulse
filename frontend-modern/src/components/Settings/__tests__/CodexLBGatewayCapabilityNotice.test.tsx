import { render, screen } from '@solidjs/testing-library';
import { Route, Router } from '@solidjs/router';
import { describe, expect, it } from 'vitest';
import { CodexLBGatewayCapabilityNotice } from '../CodexLBGatewayCapabilityNotice';

describe('CodexLBGatewayCapabilityNotice', () => {
  it('explains how an administrator can enable a disabled gateway capability', () => {
    render(() => (
      <Router>
        <Route
          path="/"
          component={() => <CodexLBGatewayCapabilityNotice capabilityAvailable={false} />}
        />
      </Router>
    ));

    expect(
      screen.getByText(
        'Codex-LB gateway capability is disabled. Enable it under Settings → Fork Capabilities before testing the connection.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Fork Capabilities' })).toHaveAttribute(
      'href',
      '/settings/pulse-intelligence/fork-capabilities',
    );
  });

  it('does not show the disabled guidance after activation', () => {
    render(() => (
      <Router>
        <Route
          path="/"
          component={() => <CodexLBGatewayCapabilityNotice capabilityAvailable={true} />}
        />
      </Router>
    ));

    expect(screen.queryByText(/capability is disabled/)).not.toBeInTheDocument();
  });
});
