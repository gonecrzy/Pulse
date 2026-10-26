import { apiFetchJSON } from '@/utils/apiClient';
import type {
  Resource,
  ResourceCapability,
  ResourceChange,
  ResourceChangeKind,
  ResourceChangeSourceAdapter,
  ResourceChangeSourceType,
  ResourceFacetCounts,
  ResourceRelationship,
  ResourceRelationshipType,
  OperatorResourceRelationship,
  ResourceIdentityLink,
  ResourcePolicyPostureSummary,
} from '@/types/resource';

export interface ResourceStatsResponse {
  total: number;
  byType: Record<string, number>;
  byStatus: Record<string, number>;
  bySource: Record<string, number>;
  policyPosture: ResourcePolicyPostureSummary;
}

export interface ResourceTimelineQueryOptions {
  since?: string | number | Date;
  limit?: number;
  kind?: ResourceChangeKind;
  sourceType?: ResourceChangeSourceType;
  sourceAdapter?: ResourceChangeSourceAdapter;
}

export interface ResourceTimelineResponse {
  resourceId: string;
  recentChanges: ResourceChange[];
  count: number;
}

export interface ResourceFacetBundle {
  capabilities?: ResourceCapability[];
  relationships?: ResourceRelationship[];
  recentChanges: ResourceChange[];
  counts: ResourceFacetCounts;
}

export interface ResourceIdentityLinksResponse {
  links: ResourceIdentityLink[];
}

const normalizeResourceId = (resourceId: string): string => resourceId.trim();

const buildFacetPath = (resourceId: string, suffix: string): string =>
  `/api/resources/${encodeURIComponent(normalizeResourceId(resourceId))}/${suffix}`;

const buildTimelineQuery = (options?: ResourceTimelineQueryOptions): string => {
  const params = new URLSearchParams();
  if (options?.since !== undefined && options.since !== null && `${options.since}`.trim()) {
    const date = options.since instanceof Date ? options.since : new Date(options.since);
    if (Number.isFinite(date.getTime())) {
      params.set('since', date.toISOString());
    }
  }
  if (Number.isFinite(options?.limit ?? NaN) && (options?.limit ?? 0) > 0) {
    params.set('limit', String(Math.trunc(options?.limit ?? 0)));
  }
  if (options?.kind) {
    params.set('kind', options.kind);
  }
  if (options?.sourceType) {
    params.set('sourceType', options.sourceType);
  }
  if (options?.sourceAdapter) {
    params.set('sourceAdapter', options.sourceAdapter);
  }
  const query = params.toString();
  return query ? `?${query}` : '';
};

const fetchFacet = async <T>(url: string): Promise<T> =>
  apiFetchJSON<T>(url, {
    cache: 'no-store',
  });

export class ResourceAPI {
  // The backend owns alias, succession and ambiguity resolution.
  static async getResource(
    reference: string,
    options?: { signal?: AbortSignal },
  ): Promise<Resource> {
    return apiFetchJSON<Resource>(
      `/api/resources/${encodeURIComponent(normalizeResourceId(reference))}`,
      { cache: 'no-store', signal: options?.signal },
    );
  }

  static async getStats(): Promise<ResourceStatsResponse> {
    return fetchFacet<ResourceStatsResponse>('/api/resources/stats');
  }

  static async getGlobalTimeline(
    options?: ResourceTimelineQueryOptions,
  ): Promise<ResourceTimelineResponse> {
    return fetchFacet<ResourceTimelineResponse>(
      `/api/resources/timeline${buildTimelineQuery(options)}`,
    );
  }

  static async getTimeline(
    resourceId: string,
    options?: ResourceTimelineQueryOptions,
  ): Promise<ResourceTimelineResponse> {
    return fetchFacet<ResourceTimelineResponse>(
      `${buildFacetPath(resourceId, 'timeline')}${buildTimelineQuery(options)}`,
    );
  }

  static async getFacetBundle(
    resourceId: string,
    options?: ResourceTimelineQueryOptions,
  ): Promise<ResourceFacetBundle> {
    return fetchFacet<ResourceFacetBundle>(
      `${buildFacetPath(resourceId, 'facets')}${buildTimelineQuery(options)}`,
    );
  }

  static async getIdentityLinks(): Promise<ResourceIdentityLinksResponse> {
    return fetchFacet<ResourceIdentityLinksResponse>('/api/resources/identity-links');
  }

  static async addRelationship(
    sourceId: string,
    input: { targetId: string; type: ResourceRelationshipType; reason?: string },
  ): Promise<{ relationship: OperatorResourceRelationship }> {
    return apiFetchJSON<{ relationship: OperatorResourceRelationship }>(
      buildFacetPath(sourceId, 'relationships'),
      { method: 'POST', body: JSON.stringify(input) },
    );
  }

  static async removeRelationship(
    sourceId: string,
    input: { targetId: string; type: ResourceRelationshipType },
  ): Promise<{ status: string }> {
    return apiFetchJSON<{ status: string }>(buildFacetPath(sourceId, 'relationships'), {
      method: 'DELETE',
      body: JSON.stringify(input),
    });
  }

  static async linkIdentity(
    primaryId: string,
    targetId: string,
    reason: string,
    confirmProviderConflict = false,
  ): Promise<{ status: string; message: string }> {
    return apiFetchJSON<{ status: string; message: string }>(buildFacetPath(primaryId, 'link'), {
      method: 'POST',
      body: JSON.stringify({
        targetId,
        reason,
        confirmIdentity: true,
        confirmProviderConflict,
      }),
    });
  }

  static async unlinkIdentity(
    resourceId: string,
    targetId: string,
    reason = '',
  ): Promise<{ status: string; message: string }> {
    return apiFetchJSON<{ status: string; message: string }>(buildFacetPath(resourceId, 'unlink'), {
      method: 'POST',
      body: JSON.stringify({ targetId, reason }),
    });
  }
}
