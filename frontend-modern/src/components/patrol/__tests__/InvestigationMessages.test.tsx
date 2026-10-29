import { cleanup, fireEvent, render, screen } from '@solidjs/testing-library';
import { afterEach, describe, expect, it, vi } from 'vitest';
import InvestigationMessages from '../InvestigationMessages';

const getMessages = vi.hoisted(() => vi.fn());
vi.mock('@/api/patrol', () => ({
  getInvestigationMessages: getMessages,
  formatTimestamp: (value: string) => value,
}));

afterEach(cleanup);

describe('InvestigationMessages', () => {
  it('retains merged failed tool evidence and exposes it through the shared disclosure', async () => {
    getMessages.mockResolvedValue({
      messages: [
        {
          id: 'turn-1',
          role: 'assistant',
          content: '',
          timestamp: '2026-09-06',
          tool_calls: [
            {
              id: 'read-1',
              name: 'pulse_read',
              input: { resource_id: 'app-container-1' },
              output: '{"error":"NO_AGENT","message":"Command agent is not connected"}',
              success: false,
            },
          ],
        },
      ],
    });
    render(() => <InvestigationMessages findingId="finding-1" />);
    const disclosure = await screen.findByRole('button', { name: /failed/i });
    expect(disclosure).toHaveAttribute('aria-expanded', 'false');
    fireEvent.keyDown(disclosure, { key: 'Enter' });
    expect(disclosure).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText(/"error":\s*"NO_AGENT"/)).toBeVisible();
    fireEvent.keyDown(disclosure, { key: ' ' });
    expect(disclosure).toHaveAttribute('aria-expanded', 'false');
  });

  it('does not infer completion from historical calls that have no recorded result status', async () => {
    getMessages.mockResolvedValue({
      messages: [
        {
          id: 'turn-2',
          role: 'assistant',
          content: '',
          timestamp: '2026-09-06',
          tool_calls: [
            {
              id: 'query-1',
              name: 'pulse_query',
              input: { action: 'metrics' },
              output: 'Historical output',
            },
          ],
        },
      ],
    });
    render(() => <InvestigationMessages findingId="finding-2" />);
    expect(await screen.findByText('Historical output')).toBeVisible();
    expect(screen.queryByText('completed')).not.toBeInTheDocument();
    expect(screen.queryByText('failed')).not.toBeInTheDocument();
  });

  it('never renders model reasoning text as investigation evidence', async () => {
    getMessages.mockResolvedValue({
      messages: [
        {
          id: 'turn-3',
          role: 'assistant',
          content: 'The storage relationship is a hypothesis pending fresh metrics.',
          reasoning_content: 'PRIVATE MODEL REASONING',
          timestamp: '2026-09-06',
        },
        {
          id: 'turn-4',
          role: 'assistant',
          content: '',
          reasoning_content: 'REASONING ONLY',
          timestamp: '2026-09-06',
        },
      ],
    });

    render(() => <InvestigationMessages findingId="finding-3" />);
    expect(
      await screen.findByText('The storage relationship is a hypothesis pending fresh metrics.'),
    ).toBeVisible();
    expect(screen.queryByText('PRIVATE MODEL REASONING')).not.toBeInTheDocument();
    expect(screen.queryByText('REASONING ONLY')).not.toBeInTheDocument();
    expect(screen.queryByText('Show reasoning')).not.toBeInTheDocument();
  });
});
