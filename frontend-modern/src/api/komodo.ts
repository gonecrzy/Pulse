import { apiFetchJSON } from '@/utils/apiClient';

export interface KomodoPollError {
  at?: string;
  category: string;
  message: string;
}

export interface KomodoPollStatus {
  enabled: boolean;
  state: 'disabled' | 'failed' | 'healthy' | 'pending' | 'stale' | string;
  resourceCount: number;
  lastAttemptAt?: string;
  lastSuccessAt?: string;
  consecutiveFailures?: number;
  lastError?: KomodoPollError;
}

export interface KomodoConnection {
  id: string;
  name: string;
  endpoint: string;
  apiKey: string;
  apiSecret: string;
  enabled: boolean;
  poll?: KomodoPollStatus;
}

export type KomodoConnectionInput = Pick<
  KomodoConnection,
  'name' | 'endpoint' | 'apiKey' | 'apiSecret' | 'enabled'
>;

export class KomodoAPI {
  private static readonly baseUrl = '/api/komodo/connections';

  static async list(): Promise<KomodoConnection[]> {
    return apiFetchJSON(this.baseUrl);
  }

  static async add(input: KomodoConnectionInput): Promise<KomodoConnection> {
    return apiFetchJSON(this.baseUrl, {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  static async update(id: string, input: KomodoConnectionInput): Promise<KomodoConnection> {
    return apiFetchJSON(`${this.baseUrl}/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    });
  }

  static async remove(id: string): Promise<void> {
    await apiFetchJSON(`${this.baseUrl}/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  static async setEnabled(id: string, enabled: boolean): Promise<void> {
    await apiFetchJSON(`${this.baseUrl}/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    });
  }

  static async test(input: KomodoConnectionInput): Promise<{ success: boolean }> {
    return apiFetchJSON(`${this.baseUrl}/test`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  static async testSaved(id: string, input?: KomodoConnectionInput): Promise<{ success: boolean }> {
    return apiFetchJSON(`${this.baseUrl}/${encodeURIComponent(id)}/test`, {
      method: 'POST',
      ...(input ? { body: JSON.stringify(input) } : {}),
    });
  }
}
