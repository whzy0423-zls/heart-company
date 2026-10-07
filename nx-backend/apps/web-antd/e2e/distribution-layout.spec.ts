import { expect, test } from 'playwright/test';

const agents = Array.from({ length: 12 }, (_, index) => ({
  id: index + 1,
  appUserId: index + 100,
  appUserAccount: `layout-fixture-${index + 1}`,
  appUserNickname: `布局测试代理 ${index + 1}`,
  agentCode: `ABCD${index + 10}`,
  level: 1,
  parentAgentId: index === 1 ? 1 : 0,
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
    : process.env.DISTRIBUTION_TOUCH ? [390, 844] : [1024, 1440, 768, 390]) {
    test(`${role} actions and table remain reachable at ${width}px`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width, height: process.env.DISTRIBUTION_TOUCH && width === 844 ? 390 : 900 });
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
          ...grid.querySelectorAll<HTMLElement>('.ant-table-body, .ant-table-content'),
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

      const table = agentList.locator('.ant-table-body, .ant-table-content');
      if (process.env.DISTRIBUTION_TOUCH) {
        const client = await page.context().newCDPSession(page);
        const swipe = async (x: number, y: number, distance: number, verticalDistance = 0) => {
          await client.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y, id: 1 }] });
          for (let step = 1; step <= 12; step++) {
            await client.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: x - distance * step / 12, y: y - verticalDistance * step / 12, id: 1 }] });
            await page.waitForTimeout(18);
          }
          await client.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
        };
        // Every visible table, including empty customer/order tables, must pan
        // from the header. Fixed headers used to ignore this real touch gesture.
        for (const wrapper of await page.locator('.distribution-page .ant-table-wrapper').all()) {
          const scroll = wrapper.locator('.ant-table-body, .ant-table-content');
          const heading = wrapper.locator('thead');
          await heading.scrollIntoViewIfNeeded();
          await heading.evaluate((el) => el.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'instant' }));
          await scroll.evaluate((el) => { el.scrollLeft = 0; });
          const bounds = (await scroll.boundingBox())!;
          const headingBounds = (await heading.boundingBox())!;
          console.log('header-touch', await heading.evaluate((el, x) => { const rect = el.getBoundingClientRect(); const hit = document.elementFromPoint(x, rect.y + rect.height / 2); return { text: el.textContent, hit: hit?.tagName, inside: el.contains(hit) }; }, Math.min(bounds.x + bounds.width, width) - 30));
          await swipe(Math.min(bounds.x + bounds.width, width) - 30, headingBounds.y + headingBounds.height / 2, 210);
          const maxScroll = await scroll.evaluate((el) => el.scrollWidth - el.clientWidth);
          if (maxScroll > 0) {
            await expect.poll(() => scroll.evaluate((el) => el.scrollLeft)).toBeGreaterThan(Math.min(100, maxScroll - 1));
          } else {
            await expect(heading.locator('th').last()).toBeInViewport();
          }
        }
        const header = agentList.locator('thead');
        await header.scrollIntoViewIfNeeded();
          await header.evaluate((el) => el.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'instant' }));
        await table.evaluate((el) => { el.scrollLeft = 0; });
        const tableBox = (await table.boundingBox())!;
        const headerBox = (await header.boundingBox())!;
        const x = Math.min(tableBox.x + tableBox.width, width) - 30;
        const y = headerBox.y + headerBox.height / 2;
        await swipe(x, y, 210);
        await expect.poll(() => table.evaluate((el) => el.scrollLeft)).toBeGreaterThan(100);
        const afterHeader = await table.evaluate((el) => el.scrollLeft);
        const firstRow = agentList.locator('tr[data-row-key="1"]');
        await firstRow.scrollIntoViewIfNeeded();
        const rowBox = (await firstRow.boundingBox())!;
        await swipe(x, rowBox.y + 30, 190);
        await expect.poll(() => table.evaluate((el) => el.scrollLeft)).toBeGreaterThan(afterHeader);
        // Continue by touch until the final action column is actually reachable.
        for (let attempt = 0; attempt < 7; attempt++) {
          await swipe(x, rowBox.y + 30, 210);
        }
        await expect(firstRow.getByRole('button', { name: '暂停', exact: true })).toBeInViewport();
        const verticalStart = await page.evaluate(() => window.scrollY);
        await swipe(x, rowBox.y + 90, 0, 120);
        await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(verticalStart + 30);

        await firstRow.getByRole('button', { name: '查看下级', exact: true }).click();
        const childDialog = page.getByRole('dialog');
        const childTable = childDialog.locator('.ant-table-content');
        await childDialog.locator('thead').scrollIntoViewIfNeeded();
        await childDialog.locator('thead').evaluate((el) => el.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'instant' }));
        const childBounds = (await childTable.boundingBox())!;
        const childHeader = (await childDialog.locator('thead').boundingBox())!;
        await swipe(Math.min(childBounds.x + childBounds.width, width) - 30, childHeader.y + childHeader.height / 2, 210);
        const childMaxScroll = await childTable.evaluate((el) => el.scrollWidth - el.clientWidth);
        if (childMaxScroll > 0) {
          await expect.poll(() => childTable.evaluate((el) => el.scrollLeft)).toBeGreaterThan(Math.min(100, childMaxScroll - 1));
        } else {
          await expect(childDialog.locator('th').last()).toBeInViewport();
        }
        await childDialog.locator('button.ant-modal-close').click();
        await expect(childDialog).not.toBeVisible();
        await client.detach();
      } else if (dimensions.table.scrollWidth > dimensions.table.clientWidth) {
        await table.scrollIntoViewIfNeeded();
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
