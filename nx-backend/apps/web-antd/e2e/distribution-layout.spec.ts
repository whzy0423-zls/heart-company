import { expect, test } from 'playwright/test';

const agents = Array.from({ length: 12 }, (_, index) => ({
  id: index + 1,
  appUserId: index + 100,
  appUserAccount: `layout-fixture-${index + 1}`,
  appUserNickname: `布局测试代理 ${index + 1}`,
  agentCode: `ABCD${index + 10}`,
  level: 1,
  parentAgentId: 0,
  rootAgentId: index + 1,
  path: `/${index + 1}/`,
  status: 'active',
  directUserCount: 3,
  secondLevelAgentCount: 2,
  thirdLevelAgentCount: 1,
}));

const analytics = {
  summary: {
    totalAgents: agents.length,
    activeAgents: agents.length,
    pausedAgents: 0,
    totalOrderAmount: 120000,
    totalCommissionAmount: 12000,
    pendingCommissionAmount: 12000,
    settledCommissionAmount: 0,
    commissionRecordCount: 12,
  },
  trend: [
    {
      date: '2026-10-01',
      orderAmount: 120000,
      commissionAmount: 12000,
      commissionCount: 12,
    },
  ],
  agentRankings: agents.map((agent) => ({
    agentId: agent.id,
    agentCode: agent.agentCode,
    appUserId: agent.appUserId,
    status: agent.status,
    directUserCount: 3,
    childAgentCount: 3,
    orderCount: 1,
    orderAmount: 10000,
    commissionAmount: 1000,
    pendingCommissionAmount: 1000,
  })),
};
// A high-performing older agent can be outside the API's latest 200 agents.
analytics.agentRankings.push({
  ...analytics.agentRankings[0]!,
  agentId: 99,
  appUserId: 9999,
  agentCode: 'OLDER99',
});

const menus = [
  {
    name: 'AppManage',
    path: '/app',
    component: 'BasicLayout',
    meta: { title: 'App 管理', icon: 'lucide:smartphone' },
    children: [
      {
        name: 'AppDistributionManagement',
        path: '/app/distribution',
        component: '/app/distribution-management',
        meta: { title: '代理管理', icon: 'lucide:share-2' },
      },
    ],
  },
];

for (const role of ['admin', 'agent-level2', 'agent-level3'] as const) {
  for (const width of role === 'agent-level3'
    ? [390]
    : [1024, 1440, 768, 390]) {
    test(`${role} actions and table remain reachable at ${width}px`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width, height: 900 });
      await page.addInitScript(() => {
        // ECharts resizing can trigger this browser delivery notification.
        // Keep Vite's development overlay from covering the real page controls.
        window.addEventListener(
          'error',
          (event) => {
            if (
              event.message ===
              'ResizeObserver loop completed with undelivered notifications.'
            ) {
              event.preventDefault();
              event.stopImmediatePropagation();
            }
          },
          true,
        );
      });
      const unexpectedRequests: string[] = [];
      const writes: string[] = [];

      await page.route(
        (url) => url.pathname.startsWith('/api/'),
        async (route) => {
          const request = route.request();
          const path = new URL(request.url()).pathname.replace(/^\/api/, '');
          if (request.method() !== 'GET' && path !== '/auth/login') {
            writes.push(`${request.method()} ${path}`);
            await route.abort();
            return;
          }
          const payloads: Record<string, unknown> = {
            '/auth/login': { accessToken: 'distribution-layout-fixture-token' },
            '/auth/codes':
              role === 'admin'
                ? ['Customer:App:List', 'Customer:App:Write']
                : ['Agent:Distribution:View', 'Agent:Distribution:Write'],
            '/user/info': {
              id: 999,
              userId: '999',
              username: 'layout-fixture',
              realName: '',
              roles: [role === 'admin' ? 'admin' : 'agent'],
              homePath: '/app/distribution',
            },
            '/menu/all': menus,
            '/admin/distribution/agents': {
              items: agents,
              total: agents.length,
            },
            '/admin/distribution/analytics': analytics,
            '/app-users/list': { items: [], total: 0 },
            '/app-users/9999': { id: 9999, nickname: '早期代理', account: 'older-fixture' },
            '/public/admin-branding': {
              name: '布局测试后台',
              logo: '',
              loadingText: '',
            },
            '/agent/distribution/agents': {
              current: { ...agents[0], level: role === 'agent-level3' ? 3 : 2 },
              items: agents,
              total: agents.length,
            },
            '/agent/distribution/analytics': {
              ...analytics,
              users: [],
              orders: [],
            },
            '/distribution-poster-config': {
              id: 'layout-poster',
              name: '布局测试海报',
              templates: [],
              templateUrl: '',
              landingUrl: '',
              qrImageUrl: '',
            },
          };
          if (!(path in payloads)) {
            unexpectedRequests.push(`${request.method()} ${path}`);
            await route.fulfill({
              status: 404,
              json: {
                code: 404,
                message: `Unmocked fixture endpoint: ${path}`,
              },
            });
            return;
          }
          await route.fulfill({
            json: { code: 0, data: payloads[path], message: 'ok' },
          });
        },
      );

      await page.goto('/auth/login');
      await page.locator('input').first().waitFor();
      await page.evaluate(async () => {
        // Exercise the real auth store and route guard; all APIs above are fixtures.
        const modulePath = '/src/store/auth.ts';
        const { useAuthStore } = await import(/* @vite-ignore */ modulePath);
        await useAuthStore().authLogin({
          username: 'layout-fixture',
          password: 'fixture',
        });
      });
      await expect(page).toHaveURL(/\/app\/distribution$/);
      const addButton = page.getByRole('button', {
        name: role === 'admin' ? '新增代理' : '新增三级代理',
        exact: true,
      });
      if (role === 'agent-level3') await expect(addButton).toBeDisabled();
      else await expect(addButton).toBeEnabled();
      await expect(
        page.getByText('layout-fixture-1', { exact: true }),
      ).toBeVisible();

      if (role === 'admin') {
        const ranking = page.locator('.ant-card').filter({ has: page.getByText('代理经营排行', { exact: true }) });
        await expect(ranking.getByRole('columnheader', { name: '用户名称', exact: true })).toBeAttached();
        await expect(ranking.locator('tr[data-row-key="1"] td').nth(1)).toHaveText('布局测试代理 1');
        await expect(ranking.locator('tr[data-row-key="99"] td').nth(1)).toHaveText('早期代理');
        await expect(ranking.getByRole('columnheader', { name: 'App 用户 ID', exact: true })).toHaveCount(0);
      }
      const agentList = page.locator('.ant-card').filter({ has: page.getByText('代理列表', { exact: true }) });
      await expect(agentList.getByRole('columnheader', { name: '用户名称', exact: true })).toBeAttached();
      await expect(agentList.locator('tr[data-row-key="1"] td').nth(3)).toHaveText('布局测试代理 1');

      const dimensions = await page.evaluate(() => {
        const grid = document.querySelector('.distribution-page')!;
        const button = [...grid.querySelectorAll('button')].find((item) =>
          ['新增代理', '新增三级代理'].includes(item.textContent?.trim() || ''),
        )!;
        const body = [
          ...grid.querySelectorAll<HTMLElement>('.ant-table-body'),
        ].at(-1)!;
        const rect = (element: Element) => {
          const { left, right, width } = element.getBoundingClientRect();
          return { left, right, width };
        };
        return {
          viewport: window.innerWidth,
          documentWidth: document.documentElement.scrollWidth,
          grid: rect(grid),
          addButton: rect(button),
          table: {
            ...rect(body),
            clientWidth: body.clientWidth,
            scrollWidth: body.scrollWidth,
            overflowX: getComputedStyle(body).overflowX,
          },
          analyticsHeader: [
            ...grid.querySelectorAll(
              '.analytics-card .ant-card-head .ant-btn, .analytics-card .ant-picker',
            ),
          ].map(rect),
        };
      });
      console.log(JSON.stringify({ role, width, dimensions }));
      await testInfo.attach('layout-dimensions', {
        body: JSON.stringify(dimensions, null, 2),
        contentType: 'application/json',
      });
      expect(dimensions.addButton.left).toBeGreaterThanOrEqual(0);
      expect(dimensions.addButton.right).toBeLessThanOrEqual(width);
      expect(dimensions.grid.right).toBeLessThanOrEqual(width);
      expect(dimensions.documentWidth).toBeLessThanOrEqual(width);
      for (const control of dimensions.analyticsHeader) {
        expect(control.left).toBeGreaterThanOrEqual(0);
        expect(control.right).toBeLessThanOrEqual(width);
      }

      if (role !== 'agent-level3') {
        await addButton.click();
        await expect(page.getByRole('dialog')).toBeVisible();
        await expect(
          page.getByText('搜索并选择 App 客户', { exact: true }),
        ).toBeVisible();
        await page
          .getByRole('dialog')
          .getByRole('button', { name: /取\s*消/ })
          .click();
        await expect(page.getByRole('dialog')).not.toBeVisible();
      }

      const table = page.locator('.distribution-page .ant-table-body').last();
      await table.scrollIntoViewIfNeeded();
      if (dimensions.table.scrollWidth > dimensions.table.clientWidth) {
        expect(dimensions.table.overflowX).toBe('auto');
        await table.hover();
        await page.mouse.wheel(420, 0);
        await expect
          .poll(() => table.evaluate((element) => element.scrollLeft))
          .toBeGreaterThan(0);
      }
      expect(writes).toEqual([]);
      expect(unexpectedRequests).toEqual([]);
      console.log(JSON.stringify({ role, width, actionsPassed: true }));
    });
  }
}
