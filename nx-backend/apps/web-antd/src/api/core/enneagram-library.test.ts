import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }));

vi.mock('#/api/request', () => ({ requestClient: mocks }));

describe('enneagram library admin api', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.put.mockReset();
  });

  it('uses one endpoint family for overview, detail and draft editing', async () => {
    const { getEnneagramTypesApi, getEnneagramTypeDetailApi, saveEnneagramDraftApi } = await import('./enneagram-library');
    await getEnneagramTypesApi();
    await getEnneagramTypeDetailApi(3);
    await saveEnneagramDraftApi(3, { title: '三号', sourceChapter: '第三章', items: [{ contentKey: 'x', text: '内容' }] });
    expect(mocks.get).toHaveBeenNthCalledWith(1, '/enneagram-library/types');
    expect(mocks.get).toHaveBeenNthCalledWith(2, '/enneagram-library/types/3');
    expect(mocks.put).toHaveBeenCalledWith('/enneagram-library/types/3/draft', expect.objectContaining({ title: '三号' }));
  });

  it('keeps review, publish, history and rollback actions separate', async () => {
    const { approveEnneagramTypeApi, getEnneagramVersionsApi, publishEnneagramTypeApi, rollbackEnneagramTypeApi, submitEnneagramReviewApi } = await import('./enneagram-library');
    await submitEnneagramReviewApi(4);
    await approveEnneagramTypeApi(4, 'checked');
    await publishEnneagramTypeApi(4);
    await getEnneagramVersionsApi(4);
    await rollbackEnneagramTypeApi(4, 2);
    expect(mocks.post).toHaveBeenNthCalledWith(1, '/enneagram-library/types/4/submit', {});
    expect(mocks.post).toHaveBeenNthCalledWith(2, '/enneagram-library/types/4/approve', { notes: 'checked' });
    expect(mocks.post).toHaveBeenNthCalledWith(3, '/enneagram-library/types/4/publish', {});
    expect(mocks.get).toHaveBeenCalledWith('/enneagram-library/types/4/versions');
    expect(mocks.post).toHaveBeenNthCalledWith(4, '/enneagram-library/types/4/rollback', { version: 2 });
  });
});
