import { describe, expect, it } from 'vitest';

import { toMenuTreeNodes } from './menu-tree';

describe('toMenuTreeNodes', () => {
  it('maps nested menu meta titles to ant design tree node titles', () => {
    const nodes = toMenuTreeNodes([
      {
        children: [
          {
            id: 401,
            meta: { title: '用户管理' },
            name: 'SystemUser',
            path: '/system/user',
            status: 1,
            type: 'menu',
          },
        ],
        id: 400,
        meta: { title: '系统管理' },
        name: 'SystemManage',
        path: '/system',
        status: 1,
        type: 'catalog',
      },
    ]);

    expect(nodes).toEqual([
      {
        children: [
          {
            children: undefined,
            key: 401,
            title: '用户管理',
          },
        ],
        key: 400,
        title: '系统管理',
      },
    ]);
  });

  it('falls back to menu name when meta title is missing', () => {
    const nodes = toMenuTreeNodes([
      {
        id: 403,
        name: 'SystemMenu',
        path: '/system/menu',
        status: 1,
        type: 'menu',
      },
    ]);

    expect(nodes[0]).toMatchObject({
      key: 403,
      title: 'SystemMenu',
    });
  });
});
