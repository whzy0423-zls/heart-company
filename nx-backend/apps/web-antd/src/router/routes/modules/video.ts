import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    component: BasicLayout,
    meta: {
      icon: 'lucide:video',
      order: 3,
      title: '视频生成',
    },
    name: 'VideoProjects',
    path: '/video',
    children: [
      {
        name: 'InfiniteCanvas',
        path: 'infinite-canvas',
        component: () => import('#/views/video/infinite-canvas.vue'),
        meta: {
          icon: 'lucide:workflow',
          title: '无限画布',
        },
      },
    ],
  },
];

export default routes;
