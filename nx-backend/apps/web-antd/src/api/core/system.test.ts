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

describe('system api save endpoints', () => {
  beforeEach(() => {
    mocks.del.mockReset();
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.put.mockReset();
  });

  it('uses create endpoint when saving a new system user', async () => {
    const { saveSystemUserApi } = await import('./system');

    await saveSystemUserApi({
      nickname: '新用户',
      roleIds: [],
      status: 1,
      username: 'new-user',
    });

    expect(mocks.post).toHaveBeenCalledWith('/system/user', {
      nickname: '新用户',
      roleIds: [],
      status: 1,
      username: 'new-user',
    });
    expect(mocks.put).not.toHaveBeenCalled();
  });

  it('uses update endpoint when saving an existing system user', async () => {
    const { saveSystemUserApi } = await import('./system');

    await saveSystemUserApi({
      id: '42',
      nickname: '老用户',
      roleIds: [],
      status: 1,
      username: 'old-user',
    });

    expect(mocks.put).toHaveBeenCalledWith('/system/user/42', {
      id: '42',
      nickname: '老用户',
      roleIds: [],
      status: 1,
      username: 'old-user',
    });
    expect(mocks.post).not.toHaveBeenCalled();
  });

  it('uses update endpoints for existing roles and menus', async () => {
    const { saveSystemMenuApi, saveSystemRoleApi } = await import('./system');

    await saveSystemRoleApi({
      code: 'ops',
      id: '7',
      menuIds: [],
      name: '运营',
      status: 1,
    });
    await saveSystemMenuApi({
      id: 9,
      name: 'OpsMenu',
      path: '/ops',
      status: 1,
      type: 'menu',
    });

    expect(mocks.put).toHaveBeenCalledWith('/system/role/7', {
      code: 'ops',
      id: '7',
      menuIds: [],
      name: '运营',
      status: 1,
    });
    expect(mocks.put).toHaveBeenCalledWith('/system/menu/9', {
      id: 9,
      name: 'OpsMenu',
      path: '/ops',
      status: 1,
      type: 'menu',
    });
  });

  it('uses create endpoints for new roles and menus', async () => {
    const { saveSystemMenuApi, saveSystemRoleApi } = await import('./system');

    await saveSystemRoleApi({
      code: 'new-role',
      menuIds: [],
      name: '新角色',
      status: 1,
    });
    await saveSystemMenuApi({
      id: 0,
      name: 'NewMenu',
      path: '/new-menu',
      status: 1,
      type: 'menu',
    });

    expect(mocks.post).toHaveBeenCalledWith('/system/role', {
      code: 'new-role',
      menuIds: [],
      name: '新角色',
      status: 1,
    });
    expect(mocks.post).toHaveBeenCalledWith('/system/menu', {
      id: 0,
      name: 'NewMenu',
      path: '/new-menu',
      status: 1,
      type: 'menu',
    });
  });
});
