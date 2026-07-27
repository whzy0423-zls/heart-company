import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    component: BasicLayout,
    meta: {
      authority: ['Miniapp:Classroom:List'],
      icon: 'lucide:presentation',
      order: 6,
      title: '老师课堂',
    },
    name: 'TeacherClassroom',
    path: '/classroom',
    children: [
      {
        component: () => import('#/views/classroom/index.vue'),
        meta: { title: '课堂管理' },
        name: 'TeacherClassroomManagement',
        path: '',
      },
    ],
  },
];

export default routes;
