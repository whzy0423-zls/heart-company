import { beforeEach, describe, expect, it, vi } from 'vitest';

const client = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
vi.mock('#/api/request', () => ({ requestClient: client }));

describe('public knowledge source API', () => {
  beforeEach(() => vi.clearAllMocks());

  it('passes catalog filters and pagination to the source endpoint', async () => {
    const api = await import('./rag');
    expect(api).toHaveProperty('getPublicKnowledgeSourcesApi');
    const filters = {
      category: '心理学',
      enabled: false,
      page: 2,
      pageSize: 20,
    };
    await api.getPublicKnowledgeSourcesApi(filters);
    expect(client.get).toHaveBeenCalledWith('/rag/sources', {
      params: filters,
    });
  });

  it('updates selection atomically and URL encodes namespaced preview IDs', async () => {
    const api = await import('./rag');
    expect(api).toHaveProperty('updatePublicKnowledgeSourcesStatusApi');
    expect(api).toHaveProperty('getPublicKnowledgeSourceChunksApi');
    await api.updatePublicKnowledgeSourcesStatusApi({
      enabled: true,
      ids: ['dataset:book:1'],
    });
    await api.getPublicKnowledgeSourceChunksApi('dataset/book:1');
    expect(client.post).toHaveBeenCalledWith('/rag/sources/status', {
      enabled: true,
      ids: ['dataset:book:1'],
    });
    expect(client.get).toHaveBeenCalledWith(
      '/rag/sources/dataset%2Fbook%3A1/chunks',
    );
  });
});
