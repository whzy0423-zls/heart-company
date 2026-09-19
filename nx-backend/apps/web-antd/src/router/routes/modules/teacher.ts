import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    component: BasicLayout,
    meta: {
      authority: ['Teacher:List', 'Miniapp:Teacher:Manage', 'Miniapp:Classroom:Write'],
      icon: 'lucide:graduation-cap',
      order: 7,
      title: '老师管理',
    },
    name: 'TeacherManagement',
    path: '/teachers',
    children: [
      {
        component: () => import('#/views/teacher/index.vue'),
        meta: { title: '老师资料与审核' },
        name: 'TeacherManagementIndex',
        path: '',
      },
    ],
  },
];

export default routes;
