import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    component: BasicLayout,
    meta: {
      authority: ['App:EnneagramLibrary:View', 'App:StoryManagement:View'],
      icon: 'lucide:smartphone',
      order: 11,
      title: 'App 管理',
    },
    name: 'AppManage',
    path: '/app',
    children: [
      {
        component: () => import('#/views/app/enneagram-library.vue'),
        meta: {
          authority: ['App:EnneagramLibrary:View'],
          icon: 'lucide:brain-circuit',
          title: '九型人格库',
        },
        name: 'AppEnneagramLibrary',
        path: 'enneagram-library',
      },
      {
        component: () => import('#/views/app/story-management.vue'),
        meta: {
          authority: ['App:StoryManagement:View'],
          icon: 'lucide:book-open-text',
          title: '故事管理',
        },
        name: 'AppStoryManagement',
        path: 'story-management',
      },
    ],
  },
];

export default routes;
