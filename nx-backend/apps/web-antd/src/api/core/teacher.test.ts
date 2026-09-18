import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  delete: vi.fn(),
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}));

vi.mock('#/api/request', () => ({ requestClient: mocks }));

describe('teacher admin API contract', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset());
  });

  it('uses the teacher profile endpoint family for list, detail, create and update', async () => {
    const api = await import('./teacher');
    await api.getTeachersApi({ page: 2, pageSize: 10, enabled: true });
    await api.getTeacherApi('han');
    await api.createTeacherApi({ key: 'han', name: '老韩' });
    await api.updateTeacherApi('han', { name: '韩老师' });
    await api.setTeacherEnabledApi('han', false);

    expect(mocks.get).toHaveBeenNthCalledWith(1, '/admin/teachers', {
      params: { page: 2, pageSize: 10, enabled: true },
    });
    expect(mocks.get).toHaveBeenNthCalledWith(2, '/admin/teachers/han');
    expect(mocks.post).toHaveBeenCalledWith('/admin/teachers', {
      key: 'han',
      name: '老韩',
    });
    expect(mocks.put).toHaveBeenCalledWith('/admin/teachers/han', {
      name: '韩老师',
    });
    expect(mocks.put).toHaveBeenCalledWith('/admin/teachers/han/status', {
      enabled: false,
    });
  });

  it('keeps role binding and review actions explicit', async () => {
    const api = await import('./teacher');
    await api.bindTeacherUserApi('han', { appUserId: 42, enabled: true });
    await api.getTeacherReviewQueueApi({ status: 'pending_review', feedType: 'daily' });
    await api.approveTeacherReviewApi(8, { expectedUpdatedAt: '2026-09-18T00:00:00Z' });
    await api.rejectTeacherReviewApi(8, { reason: '请补充课程简介' });
    await api.offlineTeacherReviewApi(8, { reason: '内容调整' });

    expect(mocks.put).toHaveBeenCalledWith('/admin/teachers/han/binding', {
      appUserId: 42,
      enabled: true,
    });
    expect(mocks.get).toHaveBeenCalledWith('/admin/teacher-reviews', {
      params: { status: 'pending_review', feedType: 'daily' },
    });
    expect(mocks.post).toHaveBeenNthCalledWith(1, '/admin/teacher-reviews/8/approve', {
      expectedUpdatedAt: '2026-09-18T00:00:00Z',
    });
    expect(mocks.post).toHaveBeenNthCalledWith(2, '/admin/teacher-reviews/8/reject', {
      reason: '请补充课程简介',
    });
    expect(mocks.post).toHaveBeenNthCalledWith(3, '/admin/teacher-reviews/8/offline', {
      reason: '内容调整',
    });
  });
});
