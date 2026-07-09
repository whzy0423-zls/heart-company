import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    component: BasicLayout,
    meta: {
      icon: 'lucide:video',
      order: 3,
      title: '视频项目',
    },
    name: 'VideoProjects',
    path: '/video',
    children: [
      {
        name: 'VideoProduction',
        path: 'production',
        component: () => import('#/views/video/production/index.vue'),
        meta: {
          icon: 'lucide:clapperboard',
          title: '制片工作台',
        },
      },
      {
        name: 'VideoProductionShort',
        path: 'production/short',
        component: () => import('#/views/video/production/short.vue'),
        meta: {
          hideInMenu: true,
          title: '短片制工作台',
          activePath: '/video/production',
        },
      },
      {
        name: 'VideoProjectsList',
        path: 'projects',
        component: () => import('#/views/video/projects/index.vue'),
        meta: {
          icon: 'lucide:folder-video',
          title: '项目列表',
        },
      },
      {
        name: 'VideoProjectWorkbenchAlias',
        path: 'projects/:id',
        redirect: (to) => `/video/projects/${to.params.id}/workbench`,
        meta: {
          hideInMenu: true,
          title: '项目工作台',
          activePath: '/video/projects',
        },
      },
      {
        name: 'VideoProjectWorkbench',
        path: 'projects/:id/workbench',
        component: () => import('#/views/video/projects/workbench.vue'),
        meta: {
          hideInMenu: true,
          title: '项目工作台',
          activePath: '/video/projects',
        },
      },
    ],
  },
];

export default routes;
