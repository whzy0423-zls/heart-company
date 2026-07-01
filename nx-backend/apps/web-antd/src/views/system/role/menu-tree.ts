import type { DataNode } from 'ant-design-vue/es/tree';

import type { SystemMenu } from '#/api';

export function toMenuTreeNodes(menus: SystemMenu[]): DataNode[] {
  return menus.map((menu) => ({
    children: menu.children?.length
      ? toMenuTreeNodes(menu.children)
      : undefined,
    key: menu.id,
    title: menu.meta?.title || menu.name || menu.path || String(menu.id),
  }));
}
