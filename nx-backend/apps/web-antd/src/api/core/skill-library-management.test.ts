import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ get: vi.fn(), request: vi.fn() }));

vi.mock('#/api/request', () => ({ requestClient: mocks }));

describe('skill library management api', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.request.mockReset();
  });

  it('loads the catalog and updates each editable resource', async () => {
    const {
      getSkillLibraryManagementApi,
      updateSkillLibraryApi,
      updateSkillLibraryCategoryApi,
      updateSkillLibrarySkillApi,
      publishSkillLibrarySkillApi,
      unpublishSkillLibrarySkillApi,
      enableSkillLibrarySkillApi,
      disableSkillLibrarySkillApi,
    } = await import('./skill-library-management');

    const metadata = {
      name: '名称',
      description: '说明',
      iconKey: 'book-open',
      colorToken: 'green',
      sortOrder: 1,
      status: 'enabled' as const,
    };
    await getSkillLibraryManagementApi();
    await updateSkillLibraryApi(1, metadata);
    await updateSkillLibraryCategoryApi(2, metadata);
    await updateSkillLibrarySkillApi(3, {
      ...metadata,
      categoryId: 2,
      summary: '简介',
    });
    await publishSkillLibrarySkillApi(3);
    await unpublishSkillLibrarySkillApi(3);
    await enableSkillLibrarySkillApi(3);
    await disableSkillLibrarySkillApi(3);

    expect(mocks.get).toHaveBeenCalledWith('/skill-library-management');
    expect(mocks.request).toHaveBeenNthCalledWith(1, '/skill-library-management/library/1', {
      data: metadata,
      method: 'PATCH',
    });
    expect(mocks.request).toHaveBeenNthCalledWith(2, '/skill-library-management/categories/2', {
      data: metadata,
      method: 'PATCH',
    });
    expect(mocks.request).toHaveBeenNthCalledWith(3, '/skill-library-management/skills/3', {
      data: expect.objectContaining({ categoryId: 2, summary: '简介' }),
      method: 'PATCH',
    });
    expect(mocks.request).toHaveBeenNthCalledWith(4, '/skill-library-management/skills/3/publish', { data: {}, method: 'POST' });
    expect(mocks.request).toHaveBeenNthCalledWith(5, '/skill-library-management/skills/3/unpublish', { data: {}, method: 'POST' });
    expect(mocks.request).toHaveBeenNthCalledWith(6, '/skill-library-management/skills/3/enable', { data: {}, method: 'POST' });
    expect(mocks.request).toHaveBeenNthCalledWith(7, '/skill-library-management/skills/3/disable', { data: {}, method: 'POST' });
  });
});
