import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    component: BasicLayout,
    meta: { authority: ['System:Payment:Config'], icon: 'lucide:wallet-cards', order: 22, title: '第三方支付' },
    name: 'ThirdPartyPayment',
    path: '/third-party-payment',
    children: [
      {
        component: () => import('#/views/third-party-payment/xzn.vue'),
        meta: { title: '星之柠' },
        name: 'XznPayment',
        path: 'xzn',
      },
      {
        component: () => import('#/views/third-party-payment/mode.vue'),
        meta: { title: '支付模式' },
        name: 'AppPaymentMode',
        path: 'mode',
      },
    ],
  },
];

export default routes;
