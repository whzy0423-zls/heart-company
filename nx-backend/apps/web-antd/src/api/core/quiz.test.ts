import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  del: vi.fn(),
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}));

vi.mock('#/api/request', () => ({
  requestClient: {
    delete: mocks.del,
    get: mocks.get,
    post: mocks.post,
    put: mocks.put,
  },
}));

describe('quiz admin api', () => {
  beforeEach(() => {
    mocks.del.mockReset();
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.put.mockReset();
  });

  it('wraps question bank CRUD endpoints', async () => {
    const {
      createQuizQuestionApi,
      deleteQuizQuestionApi,
      getQuizQuestionsApi,
      updateQuizQuestionApi,
    } = await import('./quiz');

    await getQuizQuestionsApi();
    await createQuizQuestionApi({
      body: '题目',
      dimension: 'core',
      options: [],
      sort: 1,
      status: 'enabled',
    });
    await updateQuizQuestionApi(9, {
      body: '题目2',
      dimension: 'core',
      options: [],
      sort: 2,
      status: 'disabled',
    });
    await deleteQuizQuestionApi(9);

    expect(mocks.get).toHaveBeenCalledWith('/quiz/questions');
    expect(mocks.post).toHaveBeenCalledWith('/quiz/questions', expect.objectContaining({ body: '题目' }));
    expect(mocks.put).toHaveBeenCalledWith('/quiz/questions/9', expect.objectContaining({ body: '题目2' }));
    expect(mocks.del).toHaveBeenCalledWith('/quiz/questions/9');
  });

  it('loads app user cards by appUserId', async () => {
    const { getQuizCardsApi } = await import('./quiz');

    await getQuizCardsApi(42);

    expect(mocks.get).toHaveBeenCalledWith('/quiz/cards', {
      params: { appUserId: 42 },
    });
  });
});
