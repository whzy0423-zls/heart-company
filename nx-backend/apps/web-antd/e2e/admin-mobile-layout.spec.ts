import type { Locator, Page } from 'playwright/test';
import { devices, expect, test } from 'playwright/test';

const routes = [
  ['/customer/orders', '/customer/app-orders', '订单管理'],
  ['/system/audit', '/system/audit/list', '操作审计'],
  ['/teacher/list', '/teacher/index', '老师管理'],
  ['/app/commissions', '/app/distribution-commissions', '佣金明细'],
  ['/system/menu', '/system/menu/list', '菜单权限'],
  ['/settings/model', '/settings/model', '模型配置'],
  ['/app/distribution', '/app/distribution-management', '代理管理'],
  ['/customer/users', '/customer/app-users', 'App客户'],
  ['/customer/insights', '/customer/user-insights', '用户画像'],
  ['/customer/miniapp', '/customer/miniapp-users', '小程序客户'],
];
const menus = [
  {
    name: 'FixtureLayout',
    path: '/fixture',
    component: 'BasicLayout',
    meta: { title: '布局测试', icon: 'lucide:settings' },
    children: routes.map(([path, component, title], index) => ({
      name: `FixturePage${index}`,
      path,
      component,
      meta: { title, icon: 'lucide:file' },
    })),
  },
];
const order = {
  id: 1,
  outTradeNo: 'MOBILE-LAYOUT-ORDER-20261007-0001',
  appUserId: 999,
  phone: '13000000000',
  nickname: '布局测试用户',
  productId: 'vip_month',
  title: '月卡会员',
  durationDays: 30,
  amount: 9900,
  status: 'pending_confirmation',
  paymentProvider: 'manual',
  memberLevel: 'free',
  createTime: '2026-10-07 10:00:00',
};
const audit = {
  id: 1,
  operatorName: '布局测试管理员',
  operatorId: 999,
  action: 'app_user.update',
  targetType: 'app_user',
  targetId: '999',
  summary: '布局测试：更新客户信息',
  createTime: '2026-10-07 10:00:00',
  ip: '127.0.0.1',
  userAgent: 'Mobile-Layout-Fixture/' + 'x'.repeat(180),
  before: { nickname: 'before' },
  after: { nickname: '布局测试用户', detail: 'long-json-value-'.repeat(30) },
};
const teacher = {
  key: 'fixture-teacher',
  name: '布局测试老师',
  title: '九型导师',
  enabled: true,
  appUserId: 999,
  sortOrder: 1,
  tags: ['心理成长', '亲密关系'],
  expertise: [],
  avatar: '',
  cover: '',
  bio: '布局测试老师资料',
  customerService: {},
  customerServiceConfig: {},
  offlineService: { enabled: false, types: [] },
  offlineServiceConfig: { enabled: false, types: [] },
};

async function login(page: Page, homePath: string, agent = false) {
  const unexpected: string[] = [];
  const writes: string[] = [];
  const errors: string[] = [];
  page.on('pageerror', (error) => {
    if (
      error.message !==
      'ResizeObserver loop completed with undelivered notifications.'
    )
      errors.push(error.message);
  });
  await page.addInitScript(() => {
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
        '/auth/login': { accessToken: 'admin-mobile-fixture-token' },
        '/auth/codes': [
          ...(agent
            ? ['Agent:Distribution:View', 'Agent:Distribution:Write']
            : []),
          'Customer:AppOrders:Write',
          'Miniapp:Teacher:Manage',
          'System:Menu:Create',
          'System:Menu:Update',
        ],
        '/user/info': {
          id: 999,
          userId: '999',
          username: 'layout-fixture',
          realName: '',
          roles: [agent ? 'agent' : 'admin'],
          homePath,
        },
        '/menu/all': menus,
        '/public/admin-branding': {
          name: '布局测试后台',
          logo: '',
          loadingText: '',
        },
        '/miniapp/users': { items: [], total: 0 },
        '/app-users/list': { items: [], total: 0 },
        '/app-users/insights': { items: [], total: 0 },
        '/app-orders/list': { items: [order], total: 1 },
        '/audit-logs/list': { items: [audit], total: 1 },
        '/admin/teachers': { items: [teacher], total: 1 },
        '/admin/teacher-reviews': { items: [], total: 0 },
        '/admin/distribution/commissions': {
          items: [
            {
              ID: 1,
              OrderID: 20261007,
              AgentID: 999,
              OrderAmount: 990000,
              Amount: 99000,
              RuleVersion: 1,
              Status: 'pending',
            },
          ],
        },
        '/system/menu/list': [
          {
            id: 1,
            name: 'FixtureMenu',
            path: '/fixture/menu',
            component: '/fixture/very-long-component-name',
            meta: { title: '布局测试菜单' },
            status: 1,
            type: 'menu',
            authCode: 'Fixture:Permission:Read',
          },
        ],
        '/admin/distribution/agents': { items: [], total: 0 },
        '/agent/distribution/agents': {
          current: {
            id: 999,
            agentCode: 'FIXTURE999',
            level: 2,
            status: 'active',
          },
          items: [],
          total: 0,
        },
        '/admin/distribution/analytics': {
          summary: {},
          trend: [],
          agentRankings: [],
        },
        '/agent/distribution/analytics': {
          summary: {},
          trend: [],
          agentRankings: [],
          users: [],
          orders: [],
        },
        '/distribution-poster-config': {
          id: 'fixture',
          name: '布局测试海报',
          templates: [],
          templateUrl: '',
          landingUrl: '',
          qrImageUrl: '',
        },
        '/model-config': {
          chat: {
            provider: 'openai-compatible',
            apiBase: 'https://fixture.invalid',
            model: 'fixture',
            timeoutSeconds: 30,
          },
          xinzhiliVoice: { enabled: true, asr: {}, tts: {}, interaction: {} },
        },
      };
      if (!(path in payloads)) {
        unexpected.push(`${request.method()} ${path}`);
        await route.fulfill({
          status: 404,
          json: { code: 404, message: `Unmocked fixture endpoint: ${path}` },
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
    const modulePath = '/src/store/auth.ts';
    const { useAuthStore } = await import(/* @vite-ignore */ modulePath);
    await useAuthStore().authLogin({
      username: 'layout-fixture',
      password: 'fixture',
    });
  });
  await expect(page).toHaveURL(new RegExp(`${homePath}$`));
  return { unexpected, writes, errors };
}

async function withinViewport(page: Page, target: Locator) {
  await expect(target).toBeVisible();
  await expect
    .poll(async () => {
      const box = await target.boundingBox();
      return (
        !!box &&
        box.x >= -1 &&
        box.x + box.width <= page.viewportSize()!.width + 1
      );
    })
    .toBe(true);
}

async function exerciseHorizontalTable(page: Page) {
  const scrollArea = page
    .locator('.ant-table-content, .ant-table-body')
    .first();
  await withinViewport(page, scrollArea);
  const geometry = await scrollArea.evaluate((node) => ({
    clientWidth: node.clientWidth,
    scrollWidth: node.scrollWidth,
  }));
  if (geometry.scrollWidth > geometry.clientWidth + 1) {
    await scrollArea.scrollIntoViewIfNeeded();
    const point = await scrollArea.evaluate((node) => {
      const box = node.getBoundingClientRect();
      for (
        let y = Math.max(box.top + 8, 8);
        y < Math.min(box.bottom - 8, innerHeight - 8);
        y += 20
      ) {
        const x = Math.max(box.left + 20, 20);
        const hit = document.elementFromPoint(x, y);
        if (hit && node.contains(hit)) return { x, y };
      }
      return null;
    });
    expect(
      point,
      'a table scroll target must remain visible below the fixed header',
    ).not.toBeNull();
    await page.mouse.move(point!.x, point!.y);
    if (process.env.ADMIN_LAYOUT_BROWSER === 'webkit') {
      // Mobile WebKit's automation API has no wheel support. Chromium above
      // covers real wheel input; this engine still checks the scroll container.
      await scrollArea.evaluate((node) => node.scrollBy(400, 0));
    } else {
      await page.mouse.wheel(400, 0);
    }
    await expect
      .poll(() => scrollArea.evaluate((node) => node.scrollLeft))
      .toBeGreaterThan(0);
    if (process.env.ADMIN_LAYOUT_BROWSER === 'webkit') {
      await scrollArea.evaluate((node) => node.scrollTo(0, 0));
    } else {
      await page.mouse.wheel(-10000, 0);
    }
    await expect
      .poll(() => scrollArea.evaluate((node) => node.scrollLeft))
      .toBe(0);
  }
}

for (const width of [320, 360, 390, 430, 844, 1440]) {
  test(`orders filters, detail and grant modal ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: width === 844 ? 390 : 900 });
    const api = await login(page, '/customer/orders');
    const search = page.getByPlaceholder('搜索订单号 / 手机号 / 昵称');
    await withinViewport(page, search);
    await search.fill('布局测试');
    await page.getByRole('button', { name: /查\s*询/ }).click();
    await exerciseHorizontalTable(page);
    const detail = page.getByRole('button', { name: /详\s*情/ }).first();
    await detail.click();
    const drawer = page.locator('.ant-drawer-content-wrapper');
    await withinViewport(page, drawer);
    const dimensions = await drawer.evaluate((node) => ({
      width: node.getBoundingClientRect().width,
      left: node.getBoundingClientRect().left,
      viewport: innerWidth,
    }));
    console.log(JSON.stringify({ page: 'orders', width, dimensions }));
    await testInfo.attach('drawer-dimensions', {
      body: JSON.stringify(dimensions),
      contentType: 'application/json',
    });
    await withinViewport(page, drawer);
    await withinViewport(page, page.locator('.ant-drawer-close'));
    await page.locator('.ant-drawer-close').click();
    await page.getByRole('button', { name: '确认开通', exact: true }).click();
    await withinViewport(page, page.locator('.ant-modal-content'));
    await page.getByRole('button', { name: /取\s*消/ }).click();
    expect(api.writes).toEqual([]);
    expect(api.unexpected).toEqual([]);
    expect(api.errors).toEqual([]);
  });
}

for (const [path, _component, title] of routes.slice(1, 6)) {
  test(`${title} controls remain reachable at 360px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 360, height: 900 });
    const api = await login(page, path!);
    await expect(
      page.locator('.ant-table, .editor-shell-card').first(),
    ).toBeVisible();
    const dimensions = await page.evaluate(() => ({
      documentWidth: document.documentElement.scrollWidth,
      viewport: innerWidth,
      tables: [
        ...document.querySelectorAll<HTMLElement>(
          '.ant-table-content,.ant-table-body',
        ),
      ].map((node) => ({
        clientWidth: node.clientWidth,
        scrollWidth: node.scrollWidth,
        left: node.getBoundingClientRect().left,
        right: node.getBoundingClientRect().right,
        overflow: getComputedStyle(node).overflowX,
      })),
      controls: [
        ...document.querySelectorAll<HTMLElement>(
          '.toolbar > *, .ant-card-extra',
        ),
      ].map((node) => ({
        text: node.textContent?.slice(0, 50),
        left: node.getBoundingClientRect().left,
        right: node.getBoundingClientRect().right,
      })),
    }));
    console.log(JSON.stringify({ title, dimensions }));
    await testInfo.attach('page-dimensions', {
      body: JSON.stringify(dimensions),
      contentType: 'application/json',
    });
    for (const table of dimensions.tables)
      expect(table.right).toBeLessThanOrEqual(360);
    for (const control of dimensions.controls)
      expect(control.right).toBeLessThanOrEqual(360);
    expect(dimensions.documentWidth).toBeLessThanOrEqual(360);
    if (path !== '/settings/model') await exerciseHorizontalTable(page);
    if (path === '/system/audit') {
      await page.getByRole('button', { name: /详\s*情/ }).click();
      await withinViewport(page, page.locator('.ant-drawer-content-wrapper'));
      await withinViewport(page, page.locator('.ant-drawer-close'));
      await page.locator('.ant-drawer-close').click();
    }
    if (path === '/teacher/list' || path === '/system/menu') {
      await page
        .getByRole('button', {
          name: path === '/teacher/list' ? '新增老师' : /新\s*增/,
        })
        .click();
      await withinViewport(page, page.locator('.ant-modal-content'));
      await page.getByRole('button', { name: /取\s*消/ }).click();
    }
    expect(api.writes).toEqual([]);
    expect(api.unexpected).toEqual([]);
    expect(api.errors).toEqual([]);
  });
}

for (const width of [320, 390, 768, 1440]) {
  test(`navigation and header controls ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const api = await login(page, '/customer/orders');
    const header = page.getByRole('banner');
    await withinViewport(page, header);
    for (const button of await header.getByRole('button').all()) {
      if (await button.isVisible()) await withinViewport(page, button);
    }
    if (width < 768) {
      await header.getByRole('button').first().click();
      const sidebar = page.getByRole('complementary');
      await withinViewport(page, sidebar);
      if (!(await sidebar.getByText('老师管理', { exact: true }).isVisible())) {
        await sidebar.getByText('布局测试', { exact: true }).click();
      }
      await sidebar.getByText('老师管理', { exact: true }).click();
      await expect(page).toHaveURL(/\/teacher\/list$/);
      await withinViewport(
        page,
        page.getByRole('button', { name: '新增老师', exact: true }),
      );
      await expect
        .poll(async () => {
          const box = await sidebar.boundingBox();
          return box === null || box.x + box.width <= 1;
        })
        .toBe(true);
    }
    await testInfo.attach('header-layout', {
      body: JSON.stringify(
        await header.evaluate((node) =>
          [...node.querySelectorAll('button')].map((button) => ({
            left: button.getBoundingClientRect().left,
            right: button.getBoundingClientRect().right,
          })),
        ),
      ),
      contentType: 'application/json',
    });
    expect(api.writes).toEqual([]);
    expect(api.unexpected).toEqual([]);
    expect(api.errors).toEqual([]);
  });
}

test('model form at enlarged text size', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const api = await login(page, '/settings/model');
  await page.evaluate(() => {
    document.documentElement.style.fontSize = '20px';
  });
  await withinViewport(
    page,
    page.getByRole('button', { name: '保存配置', exact: true }),
  );
  const speed = page.getByPlaceholder('如 1', { exact: true });
  const format = page.getByPlaceholder('mp3', { exact: true });
  await speed.scrollIntoViewIfNeeded();
  await withinViewport(page, speed);
  await speed.fill('1.1');
  await format.scrollIntoViewIfNeeded();
  await withinViewport(page, format);
  const speedBox = await speed.boundingBox();
  const formatBox = await format.boundingBox();
  expect(formatBox!.y).toBeGreaterThan(speedBox!.y);
  expect(speedBox!.width).toBeGreaterThan(200);
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
    .toBeLessThanOrEqual(390);
  expect(api.writes).toEqual([]);
  expect(api.unexpected).toEqual([]);
  expect(api.errors).toEqual([]);
});

for (const width of [320, 390]) {
  test(`agent date range popup ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 });
    await page.clock.setFixedTime(new Date('2026-10-07T10:00:00+08:00'));
    const api = await login(page, '/app/distribution', true);
    await page.getByPlaceholder('开始日期').click();
    const popup = page.locator(
      '.ant-picker-dropdown:not(.ant-picker-dropdown-hidden)',
    );
    await expect(popup).toBeVisible();
    console.log(
      JSON.stringify({
        page: 'date-range',
        width,
        box: await popup.boundingBox(),
      }),
    );
    await withinViewport(page, popup);
    const firstPanel = popup.locator('.ant-picker-panel').first();
    await firstPanel.getByTitle('2026-10-01', { exact: true }).click();
    const secondMonthDate = popup
      .locator('.ant-picker-panel')
      .nth(1)
      .getByTitle('2026-11-05', { exact: true });
    await secondMonthDate.scrollIntoViewIfNeeded();
    await secondMonthDate.click();
    await expect(page.getByPlaceholder('开始日期')).toHaveValue('2026-10-01');
    await expect(page.getByPlaceholder('结束日期')).toHaveValue('2026-11-05');
    expect(api.writes).toEqual([]);
    expect(api.unexpected).toEqual([]);
    expect(api.errors).toEqual([]);
  });
}

test.describe('mobile touch browser', () => {
  const { defaultBrowserType: _browser, ...mobileDevice } = devices['Pixel 7']!;
  test.use(mobileDevice);
  test('teacher creation and navigation respond to taps', async ({ page }) => {
    const api = await login(page, '/teacher/list');
    expect(await page.evaluate(() => navigator.maxTouchPoints)).toBeGreaterThan(
      0,
    );
    await page.getByRole('button', { name: '新增老师', exact: true }).tap();
    await withinViewport(page, page.locator('.ant-modal-content'));
    await page.getByPlaceholder('请输入老师姓名').fill('触摸布局草稿');
    await page.getByRole('button', { name: /取\s*消/ }).tap();
    await expect(page.getByRole('dialog')).not.toBeVisible();
    await page.getByRole('banner').getByRole('button').first().tap();
    const sidebar = page.getByRole('complementary');
    if (!(await sidebar.getByText('菜单权限', { exact: true }).isVisible())) {
      await sidebar.getByText('布局测试', { exact: true }).tap();
    }
    await sidebar.getByText('菜单权限', { exact: true }).tap();
    await expect(page).toHaveURL(/\/system\/menu$/);
    await page.getByRole('button', { name: /新\s*增/ }).tap();
    await withinViewport(page, page.locator('.ant-modal-content'));
    await page.getByRole('button', { name: /取\s*消/ }).tap();
    // Re-selecting the active route must also dismiss the mobile navigation.
    await page.getByRole('banner').getByRole('button').first().tap();
    if (!(await sidebar.getByText('菜单权限', { exact: true }).isVisible())) {
      await sidebar.getByText('布局测试', { exact: true }).tap();
    }
    await sidebar.getByText('菜单权限', { exact: true }).tap();
    await expect
      .poll(async () => {
        const box = await sidebar.boundingBox();
        return box === null || box.x + box.width <= 1;
      })
      .toBe(true);
    await page.getByRole('button', { name: /新\s*增/ }).tap();
    await withinViewport(page, page.locator('.ant-modal-content'));
    await page.getByRole('button', { name: /取\s*消/ }).tap();
    expect(api.writes).toEqual([]);
    expect(api.unexpected).toEqual([]);
    expect(api.errors).toEqual([]);
  });
});

for (const path of [
  '/customer/users',
  '/customer/insights',
  '/customer/miniapp',
]) {
  for (const width of path === '/customer/miniapp'
    ? [844, 390]
    : [844, 1024, 390]) {
    test(`customer filters ${path} fit content at ${width}px`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width, height: width === 844 ? 390 : 900 });
      const api = await login(page, path);
      const filterBar = page.locator(
        path === '/customer/miniapp' ? '.filters' : '.filter-bar',
      );
      await expect(filterBar).toBeVisible();
      const dimensions = await filterBar.evaluate((node) => ({
        width: innerWidth,
        container: {
          left: node.getBoundingClientRect().left,
          right: node.getBoundingClientRect().right,
        },
        controls: [...node.children].map((child) => ({
          text: child.textContent?.trim(),
          left: child.getBoundingClientRect().left,
          right: child.getBoundingClientRect().right,
        })),
      }));
      console.log(JSON.stringify({ path, width, dimensions }));
      await testInfo.attach('customer-filters', {
        body: JSON.stringify(dimensions),
        contentType: 'application/json',
      });
      for (const control of dimensions.controls) {
        expect(control.left).toBeGreaterThanOrEqual(
          dimensions.container.left - 1,
        );
        expect(control.right).toBeLessThanOrEqual(
          dimensions.container.right + 1,
        );
      }
      for (const control of await filterBar.locator(':scope > *').all()) {
        await withinViewport(page, control);
      }
      await page
        .getByPlaceholder(
          path === '/customer/miniapp'
            ? '搜索昵称 / 手机号 / 渠道 / 场景'
            : '搜索手机号 / 昵称',
        )
        .fill('布局测试');
      const endpoint =
        path === '/customer/miniapp'
          ? '/api/miniapp/users'
          : path === '/customer/users'
            ? '/api/app-users/list'
            : '/api/app-users/insights';
      const searched = page.waitForRequest((request) => {
        const url = new URL(request.url());
        return (
          url.pathname === endpoint &&
          url.searchParams.get('keyword') === '布局测试'
        );
      });
      await filterBar.getByRole('button', { name: /查\s*询/ }).click();
      await searched;
      expect(api.writes).toEqual([]);
      expect(api.unexpected).toEqual([]);
      expect(api.errors).toEqual([]);
    });
  }
}
