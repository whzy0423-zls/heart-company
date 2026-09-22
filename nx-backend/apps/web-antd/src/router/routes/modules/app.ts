import type { RouteRecordRaw } from 'vue-router';

import { BasicLayout } from '#/layouts';

const routes: RouteRecordRaw[] = [
  {
    component: BasicLayout,
    meta: {
      authority: [
        'Analytics:App:Overview',
        'App:EnneagramLibrary:View',
        'Customer:App:List',
        'Customer:AppOrders:List',
        'Customer:AppChat:List',
        'Customer:AppMemory:List',
        'Customer:UserInsights:List',
        'App:SkillLibrary:View',
        'App:StoryManagement:View',
        'App:PlanManagement:View',
        'App:VoiceBroadcast:Manage',
        'Website:AppReleases:List',
        'Website:Write',
        'Agent:Distribution:View',
        'agent',
      ],
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
        component: () => import('#/views/app/skill-library-management.vue'),
        meta: {
          authority: ['App:SkillLibrary:View'],
          icon: 'lucide:library-big',
          title: '成长技能库',
        },
        name: 'AppSkillLibrary',
        path: 'skill-library',
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
      { component: () => import('#/views/app/distribution-commissions.vue'), meta: { authority: ['Customer:App:List'], icon: 'lucide:coins', title: '佣金明细' }, name: 'AppDistributionCommissions', path: 'distribution-commissions' },
      { component: () => import('#/views/app/distribution-rules.vue'), meta: { authority: ['Customer:App:List'], icon: 'lucide:percent', title: '佣金规则' }, name: 'AppDistributionRules', path: 'distribution-rules' },
      { component: () => import('#/views/app/distribution-settlements.vue'), meta: { authority: ['Customer:App:List'], icon: 'lucide:wallet-cards', title: '分销结算' }, name: 'AppDistributionSettlements', path: 'distribution-settlements' },
      {
        component: () => import('#/views/app/distribution-management.vue'),
        meta: {
          authority: ['Customer:App:List', 'Agent:Distribution:View', 'Agent:Distribution:Write', 'agent'],
          hideInMenu: true,
          icon: 'lucide:share-2',
          title: '代理管理',
        },
        name: 'AppDistributionManagement',
        path: 'distribution',
      },
      {
        component: () => import('#/views/app/distribution-poster-management.vue'),
        meta: {
          authority: ['Customer:App:List', 'Agent:Distribution:View', 'agent'],
          icon: 'lucide:image',
          title: '海报管理',
        },
        name: 'AppDistributionPosterManagement',
        path: 'distribution-poster',
      },
      {
        component: () => import('#/views/app/voice-broadcast.vue'),
        meta: {
          authority: ['App:VoiceBroadcast:Manage'],
          icon: 'lucide:volume-2',
          title: '语音播报配置',
        },
        name: 'AppVoiceBroadcastConfig',
        path: 'voice-broadcast-config',
      },
      {
        component: () => import('#/views/app/plan-management.vue'),
        meta: {
          authority: ['App:PlanManagement:View'],
          icon: 'lucide:badge-dollar-sign',
          title: '套餐管理',
        },
        name: 'AppPlanManagement',
        path: 'plan-management',
      },
    ],
  },
];

export default routes;
