import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    component: BasicLayout,
    meta: {
      authority: ['Website:Write', 'Customer:Miniapp:List'],
      icon: 'lucide:smartphone',
      order: 12,
      title: '小程序管理',
    },
    name: 'MiniappManage',
    path: '/miniapp',
    children: [
      {
        component: () => import('#/views/miniapp/home.vue'),
        meta: { title: '首页管理' },
        name: 'MiniappHome',
        path: 'home',
      },
      {
        component: () => import('#/views/miniapp/learn.vue'),
        meta: { title: '学习页管理' },
        name: 'MiniappLearn',
        path: 'learn',
      },
      {
        component: () => import('#/views/miniapp/payment.vue'),
        meta: { title: '支付设置' },
        name: 'MiniappPayment',
        path: 'payment',
      },
      {
        component: () => import('#/views/miniapp/courses.vue'),
        meta: { title: '课程配置' },
        name: 'MiniappCourses',
        path: 'courses',
      },
      {
        component: () => import('#/views/customer/miniapp-users.vue'),
        meta: {
          authority: ['Customer:Miniapp:List'],
          title: '客户信息',
        },
        name: 'MiniappCustomers',
        path: 'customers',
      },
    ],
  },
];

export default routes;
