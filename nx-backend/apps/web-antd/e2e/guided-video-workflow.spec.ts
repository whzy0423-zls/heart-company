import { expect, test, type Page } from 'playwright/test';

type Scenario =
  | 'accepted'
  | 'compose-failure'
  | 'failed-selection'
  | 'legacy'
  | 'partial-import'
  | 'reconciled'
  | 'stale-final'
  | 'stale-selection'
  | 'timeout'
  | 'unknown';

const artifactRoot = '/Users/wohenzaiyi/Desktop/nine-xing/artifacts';
const ok = (data: unknown) => ({ code: 0, data, error: null, message: 'ok' });

function project() {
  return {
    characterCount: 0, completedShots: 0, composeStatus: 'pending', createTime: '2026/07/11 10:00:00',
    description: '', finalVideoAssetId: '', finalVideoInputHash: '', finalVideoUrl: '', id: '1', name: '浏览器验收项目',
    sceneCount: 0, scriptContent: '', scriptRevision: 0, status: 'draft', styleGuide: '', theme: '', totalShots: 0, updateTime: '2026/07/11 10:00:00',
  };
}

function shot(id: string, orderNum: number) {
  return {
    actionDescription: `镜头 ${orderNum} 动作`, aspectRatio: '16:9', cameraMovement: '', characterIds: [], createTime: '', duration: 15,
    dynamicDescription: '', endFrameUrl: '', errorMessage: '', generatedPrompt: '', generationId: '', generationRevision: 1,
    gridStoryboardPrompt: '', id, imageReferenceModes: [], name: `分镜 ${orderNum}`, orderNum, projectId: '1', sceneId: '',
    scriptOriginalContent: `段落 ${orderNum}`, selectedGenerationId: '', selectedGenerationRevision: 0, selectedGenerationStatus: '',
    shotAssets: [], soundAndPictureTogether: '', sourceKey: `source-${id}`, sourceScriptRevision: 1, status: 'draft', storyboardUrl: '',
    updateTime: '', usedAudios: [], usedImages: [], usedVideos: [], videoModel: '', videoReferenceMode: 'none', videoResolution: '', videoUrl: '',
  };
}

function version(id: string, current = false) {
  return { backupFlag: false, createTime: '', errorMessage: '', id, isCurrent: current, model: 'mock', shotId: '1', shotRevision: 1, status: 'completed', taskId: `task-${id}`, videoUrl: `https://example.test/${id}.mp4` };
}

function makeEntry(id: string, readiness: string) {
  const item: any = { readiness, shot: shot(id, Number(id)) };
  if (readiness === 'completed') {
    Object.assign(item.shot, { selectedGenerationId: `9${id}`, selectedGenerationRevision: 1, selectedGenerationStatus: 'completed', videoUrl: `https://example.test/${id}.mp4` });
  }
  return item;
}

async function installFixture(page: Page, scenario?: Scenario) {
  const state: any = {
    batchShotIds: [] as string[], characters: [] as any[], composeAttempts: 0, composePolls: 0, generationPosts: 0,
    mockedPaidPosts: 0, project: project(), reconcilePosts: 0, scenes: [] as any[], shots: [] as any[], versions: {} as Record<string, any[]>, workflowGets: 0,
  };
  if (scenario) {
    state.project.scriptContent = '已有剧本';
    state.project.scriptRevision = 1;
    state.shots = [makeEntry('1', 'completed')];
    state.versions['1'] = [version('91', true)];
  }
  if (scenario === 'legacy') {
    state.project.scriptContent = '';
    state.shots = [makeEntry('1', 'incomplete')];
  }
  if (scenario === 'accepted' || scenario === 'reconciled' || scenario === 'timeout') {
    state.shots = [makeEntry('1', 'generating')];
    state.shots[0].activeSubmission = { requestKey: '11111111-1111-4111-8111-111111111111', status: scenario === 'reconciled' ? 'reconciled' : 'accepted', submissionId: 11, taskId: 'task-11' };
  }
  if (scenario === 'unknown') {
    state.shots = [makeEntry('1', 'recovery')];
    state.shots[0].activeSubmission = { requestKey: '22222222-2222-4222-8222-222222222222', status: 'unknown_outcome', submissionId: 12, taskId: 'task-12' };
  }
  if (scenario === 'failed-selection') {
    state.shots[0].shot.status = 'failed';
  }
  if (scenario === 'stale-selection') state.shots = [makeEntry('1', 'stale')];
  if (scenario === 'stale-final') {
    state.project.finalVideoUrl = 'https://example.test/old-final.mp4';
    state.project.finalVideoInputHash = 'old-hash';
  }

  const errors: string[] = [];
  const failedRequests: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (entry) => { if (entry.type() === 'error') errors.push(entry.text()); });
  page.on('requestfailed', (request) => failedRequests.push(request.url()));

  await page.route('https://example.test/**', (route) => route.fulfill({ body: '', contentType: 'video/mp4', status: 200 }));

  await page.route('http://127.0.0.1:4317/api/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const body = request.postDataJSON?.() || {};
    let data: any = {};
    if (path === '/api/auth/login') data = { accessToken: 'e2e-token' };
    else if (path === '/api/user/info') data = { avatar: '', desc: '', homePath: '/video/production', realName: '验收用户', roles: ['admin'], token: 'e2e-token', userId: '1', username: 'e2e' };
    else if (path === '/api/auth/codes') data = ['Video:Project:Manage'];
    else if (path === '/api/menu/all') data = [
      { component: '/video/production/index', id: 1008, meta: { icon: 'lucide:clapperboard', title: '制片工作台' }, name: 'VideoProduction', path: '/video/production', pid: 0, status: 1, type: 'menu' },
      { component: '/video/projects/index', id: 1006, meta: { icon: 'lucide:folder-kanban', title: '项目列表' }, name: 'VideoProjects', path: '/video/projects', pid: 0, status: 1, type: 'menu' },
      { component: '/video/projects/workflow', id: 1007, meta: { activePath: '/video/projects', hideInMenu: true, title: '项目工作台详情' }, name: 'VideoProjectWorkbench', path: '/video/projects/:id/workbench', pid: 0, status: 1, type: 'menu' },
      { component: '/video/projects/workbench', id: 1010, meta: { activePath: '/video/projects', hideInMenu: true, title: '高级项目工作台' }, name: 'VideoProjectAdvancedWorkbench', path: '/video/projects/:id/workbench/advanced', pid: 0, status: 1, type: 'menu' },
    ];
    else if (path === '/api/site-config' || path === '/api/system/branding') data = {};
    else if (path === '/api/video/projects/list') data = { items: state.shots.length ? [state.project] : [], total: state.shots.length ? 1 : 0 };
    else if (path === '/api/video/projects' && request.method() === 'POST') data = state.project;
    else if (path === '/api/video/projects/1' && request.method() === 'PUT') { Object.assign(state.project, body); state.project.scriptRevision += 1; data = state.project; }
    else if (path === '/api/video/projects-workflow/1') {
      state.workflowGets += 1;
      if ((scenario === 'accepted' || scenario === 'reconciled') && state.workflowGets >= 2) {
        state.shots[0] = makeEntry('1', 'completed');
        state.versions['1'] = [version('91', true)];
      }
      if (scenario === 'unknown' && state.reconcilePosts && state.workflowGets >= 2) state.shots[0] = makeEntry('1', 'completed');
      state.project.totalShots = state.shots.length;
      state.project.completedShots = state.shots.filter((item: any) => item.readiness === 'completed').length;
      const recommendedStep = scenario === 'legacy' ? 'storyboard' : state.shots.length ? 'storyboard' : state.project.scriptContent ? 'assets' : 'brief';
      data = {
        project: state.project, recommendedStep,
        shots: state.shots.map((item: any) => ({ ...item, canGenerate: ['ready', 'stale', 'failed'].includes(item.readiness) })),
        steps: { brief: state.project.scriptContent ? 'complete' : state.shots.length ? 'skipped_existing' : 'blocked', assets: state.characters.length || state.scenes.length ? 'complete' : state.shots.length ? 'skipped_existing' : 'optional', storyboard: state.shots.length ? 'complete' : 'blocked', generate: state.shots.every((item: any) => item.readiness === 'completed') && state.shots.length ? 'complete' : state.shots.some((item: any) => item.readiness === 'stale') ? 'stale' : 'blocked', export: state.project.finalVideoUrl ? scenario === 'stale-final' ? 'stale' : 'complete' : 'blocked' },
      };
    } else if (path === '/api/video/projects-shots/from-script/1') {
      if (scenario === 'partial-import' && !state.partialReturned) {
        state.partialReturned = true;
        state.shots = [makeEntry('1', 'ready'), makeEntry('2', 'ready')];
        data = { created: [{ index: 0, shotId: '1', sourceKey: 'source-1', status: 'created' }], existing: [{ index: 1, shotId: '2', sourceKey: 'source-2', status: 'existing' }], failed: [{ error: 'mock failure', index: 2, sourceKey: 'source-3', status: 'failed' }], items: [] };
      } else {
        const offset = state.shots.length;
        const created = body.items.map((item: any, index: number) => makeEntry(String(offset + index + 1), 'ready'));
        state.shots.push(...created);
        data = { created: created.map((item: any, index: number) => ({ index, shotId: item.shot.id, sourceKey: `source-${item.shot.id}`, status: 'created' })), existing: [], failed: [], items: [] };
      }
    } else if (path.startsWith('/api/video/shots/') && request.method() === 'PUT') { const item = state.shots.find((entry: any) => path.endsWith(`/${entry.shot.id}`)); Object.assign(item.shot, body); data = item.shot; }
    else if (path === '/api/video/projects-characters/1' && request.method() === 'GET') data = state.characters;
    else if (path === '/api/video/projects-characters/1' && request.method() === 'POST') { const item = { ...body, id: 'c1' }; state.characters.push(item); data = item; }
    else if (path === '/api/video/projects-scenes/1' && request.method() === 'GET') data = state.scenes;
    else if (path === '/api/video/projects-scenes/1' && request.method() === 'POST') { const item = { ...body, id: 's1' }; state.scenes.push(item); data = item; }
    else if (path === '/api/video/projects-batch-generate-safe/1') {
      state.generationPosts += 1; state.mockedPaidPosts += 1; state.batchShotIds = body.items.map((item: any) => item.shotId);
      for (const item of state.shots.filter((entry: any) => state.batchShotIds.includes(entry.shot.id))) {
        item.readiness = 'ready'; item.shot.generationId = `9${item.shot.id}`; item.shot.status = 'completed';
        state.versions[item.shot.id] = [version(`9${item.shot.id}`)];
      }
      data = { failedCount: 0, projectId: '1', shotResults: state.batchShotIds.map((shotId: string) => ({ errorMessage: '', generationId: `9${shotId}`, orderNum: Number(shotId), shotId, shotName: `分镜 ${shotId}`, status: 'success' })), successCount: state.batchShotIds.length, totalShots: state.batchShotIds.length };
    } else if (path.startsWith('/api/video/shots-video-versions/list/')) data = state.versions[path.split('/').at(-1)!] || [];
    else if (path.startsWith('/api/video/shots-video-versions/set/')) {
      const [, , , , , shotId, generationId] = path.split('/');
      const item = state.shots.find((entry: any) => entry.shot.id === shotId);
      item.readiness = 'completed'; Object.assign(item.shot, { selectedGenerationId: generationId, selectedGenerationRevision: 1, selectedGenerationStatus: 'completed', videoUrl: `https://example.test/${generationId}.mp4` });
      state.versions[shotId] = (state.versions[shotId] || []).map((entry: any) => ({ ...entry, isCurrent: entry.id === generationId })); data = item.shot;
    } else if (path.startsWith('/api/video/generation-submissions/reconcile/')) {
      state.reconcilePosts += 1; state.shots[0].readiness = 'generating'; state.shots[0].activeSubmission.status = 'reconciled'; data = { generationId: '91', requestKey: path.split('/').at(-1), status: 'reconciled', taskId: body.taskId };
    } else if (path === '/api/video/projects-compose-safe/1') {
      state.composeAttempts += 1; state.composePolls = 0;
      data = { error: '', inputHash: 'hash', isCurrent: false, jobId: String(7 + state.composeAttempts), progress: 0, projectId: '1', status: 'queued', videoUrl: '' };
    } else if (path.startsWith('/api/video/projects-compose-safe-status/1/')) {
      state.composePolls += 1;
      const failed = scenario === 'compose-failure' && state.composeAttempts === 1;
      const complete = !failed && state.composePolls > 1;
      if (complete) { state.project.finalVideoUrl = 'https://example.test/final.mp4'; state.project.finalVideoInputHash = 'hash'; }
      data = { error: failed ? 'mock compose failure' : '', inputHash: 'hash', inputSnapshot: {}, isCurrent: complete, jobId: path.split('/').at(-1), progress: failed ? 35 : complete ? 100 : 70, projectId: '1', status: failed ? 'failed' : complete ? 'completed' : 'processing', videoUrl: complete ? state.project.finalVideoUrl : '' };
    }
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(ok(data)), status: 200 });
  });
  return { errors, failedRequests, state };
}

async function login(page: Page) {
  await page.goto('/auth/login');
  await page.getByPlaceholder(/用户名|username/i).fill('e2e');
  await page.getByPlaceholder(/密码|password/i).fill('password');
  const slider = page.locator('[name="captcha-action"]');
  const track = slider.locator('..');
  const sliderBox = await slider.boundingBox();
  const trackBox = await track.boundingBox();
  expect(sliderBox).not.toBeNull(); expect(trackBox).not.toBeNull();
  await page.mouse.move(sliderBox!.x + sliderBox!.width / 2, sliderBox!.y + sliderBox!.height / 2);
  await page.mouse.down(); await page.mouse.move(trackBox!.x + trackBox!.width - 2, sliderBox!.y + sliderBox!.height / 2, { steps: 10 }); await page.mouse.up();
  await expect(page.getByText(/验证通过|验证成功|verification succeeded/i)).toBeVisible();
  await page.getByRole('button', { name: /登录|login/i }).click();
  await page.waitForURL('**/video/production');
}

async function openWorkflow(page: Page, step?: string) {
  await page.goto(`/video/projects/1/workbench${step ? `?step=${step}` : ''}`);
  await expect(page.getByRole('navigation', { name: '视频制作步骤' })).toBeVisible();
}

async function selectEveryVersion(page: Page, count: number) {
  for (let index = 0; index < count; index += 1) {
    if (index > 0) await page.getByRole('tablist', { name: '分镜生成状态' }).getByRole('button', { name: /可生成/ }).click();
    await page.getByRole('button', { name: '查看版本' }).first().click();
    await page.getByRole('button', { name: '设为当前' }).click();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('button', { name: '查看版本' }).last()).toBeFocused();
  }
}

test('complete five-step workflow selects versions explicitly and never reaches a real paid gateway', async ({ page }) => {
  const { errors, failedRequests, state } = await installFixture(page);
  await login(page);
  await page.getByRole('button', { name: '新建项目' }).click();
  await page.waitForURL('**/video/projects/1/workbench**');
  await page.getByLabel('完整剧本').fill('第一镜\n\n第二镜');
  await page.getByRole('button', { name: '保存并继续' }).click();
  await page.getByRole('button', { name: '添加角色' }).click();
  let dialog = page.getByRole('dialog');
  await dialog.getByRole('textbox', { name: '名称' }).fill('主角');
  await dialog.getByRole('textbox', { name: '参考图 URL' }).fill('https://example.test/character.png');
  await dialog.getByRole('button', { name: '确 定' }).click();
  await page.getByRole('button', { name: '添加场景' }).click();
  dialog = page.getByRole('dialog');
  await dialog.getByRole('textbox', { name: '名称' }).fill('街道');
  await dialog.getByRole('textbox', { name: '参考图 URL' }).fill('https://example.test/scene.png');
  await dialog.getByRole('button', { name: '确 定' }).click();
  await page.getByRole('navigation', { name: '视频制作步骤' }).getByRole('button', { name: /分镜/ }).click();
  await page.getByRole('button', { name: '从剧本创建分镜' }).click();
  await expect(page.getByText(/created 2/)).toBeVisible();
  await page.getByLabel('动作描述').fill('修改后的动作');
  await page.getByRole('button', { name: '保存并继续' }).click();
  await page.getByRole('button', { name: '生成可用分镜' }).click();
  await expect.poll(() => state.generationPosts).toBe(1);
  expect(state.batchShotIds).toEqual(['1', '2']);
  expect(state.shots.every((item: any) => !item.shot.selectedGenerationId)).toBe(true);
  await selectEveryVersion(page, 2);
  await page.getByRole('navigation', { name: '视频制作步骤' }).getByRole('button', { name: /导出/ }).click();
  await page.getByRole('button', { name: '开始合成' }).click();
  await expect(page.getByText('当前成片')).toBeVisible({ timeout: 15_000 });
  expect(errors).toEqual([]); expect(failedRequests).toEqual([]); expect(state.mockedPaidPosts).toBe(1);
});

test('legacy project recommendation and partial import remain resumable', async ({ page }) => {
  await installFixture(page, 'legacy'); await login(page); await openWorkflow(page);
  await expect(page.locator('[data-step="storyboard"]')).toBeVisible();
  const fixture = await installFixture(page, 'partial-import');
  fixture.state.project.scriptContent = '一\n\n二\n\n三';
  await page.reload(); await page.getByRole('button', { name: '从剧本创建分镜' }).click();
  await expect(page.getByText('created 1')).toBeVisible(); await expect(page.getByText('existing 1')).toBeVisible(); await expect(page.getByText('failed 1')).toBeVisible();
  await page.getByRole('button', { name: '重试失败项' }).click(); await expect(page.getByText('failed 0')).toBeVisible();
});

for (const scenario of ['accepted', 'reconciled'] as const) {
  test(`${scenario} submission resumes polling after reload`, async ({ page }) => {
    const { state } = await installFixture(page, scenario); await login(page); await openWorkflow(page, 'generate');
    await expect(page.getByRole('button', { name: /已完成 1/ })).toBeVisible({ timeout: 10_000 });
    await page.getByRole('tablist', { name: '分镜生成状态' }).getByRole('button', { name: /已完成/ }).click();
    await expect(page.getByText('completed')).toBeVisible();
    expect(state.generationPosts).toBe(0);
  });
}

test('unknown outcome exposes reconcile-only recovery without a paid repost', async ({ page }) => {
  const { state } = await installFixture(page, 'unknown'); await login(page); await openWorkflow(page, 'generate');
  await page.getByRole('button', { name: /待完善/ }).click();
  await expect(page.getByRole('button', { name: '检查结果' })).toBeVisible();
  await expect(page.getByPlaceholder('上游 task ID')).toHaveValue('task-12');
  await page.getByRole('button', { name: /对\s*账/ }).click();
  await expect.poll(() => state.reconcilePosts).toBe(1);
  expect(state.generationPosts).toBe(0);
  await page.screenshot({ path: `${artifactRoot}/guided-workflow-recovery.png`, fullPage: true });
});

test('failed latest attempt with valid selection stays completed and stale selection is generatable', async ({ page }) => {
  const { state } = await installFixture(page, 'failed-selection'); await login(page); await openWorkflow(page, 'generate');
  await page.getByRole('tablist', { name: '分镜生成状态' }).getByRole('button', { name: /已完成/ }).click(); await expect(page.getByText('completed')).toBeVisible();
  state.shots = [makeEntry('1', 'stale')]; await page.reload();
  await expect(page.getByText(/内容变化 1/)).toBeVisible(); await expect(page.locator('.generation-list article').getByRole('button', { name: /生\s*成/ })).toBeVisible();
});

test('polling timeout offers manual refresh and compose failure can retry', async ({ page }) => {
  const { state } = await installFixture(page, 'timeout'); await login(page); await page.clock.install(); await openWorkflow(page, 'generate');
  for (let attempt = 0; attempt < 32; attempt += 1) {
    const previousGets = state.workflowGets;
    await page.clock.runFor(4_001);
    await expect.poll(() => state.workflowGets).toBeGreaterThan(previousGets);
  }
  await page.getByRole('button', { name: /生成中/ }).click();
  await expect(page.getByRole('button', { name: '手动刷新' })).toBeVisible();

  await installFixture(page, 'compose-failure'); await openWorkflow(page, 'export');
  await page.getByRole('button', { name: '开始合成' }).click(); await expect(page.getByRole('alert')).toHaveText('mock compose failure');
  await page.getByRole('button', { name: '重试合成' }).click(); await expect(page.getByText('当前成片')).toBeVisible({ timeout: 10_000 });
});

test('stale final result preserves old preview and requests recomposition', async ({ page }) => {
  await installFixture(page, 'stale-final'); await login(page); await openWorkflow(page, 'export');
  await expect(page.getByText('内容已变化，需要重新合成')).toBeVisible();
  await expect(page.getByText('旧成片仍可预览和下载。')).toBeVisible();
});

for (const viewport of [{ width: 1440, height: 900 }, { width: 768, height: 1024 }, { width: 375, height: 812 }]) {
  test(`responsive and accessible shell ${viewport.width}x${viewport.height}`, async ({ page }) => {
    await page.setViewportSize(viewport); await page.emulateMedia({ reducedMotion: 'reduce' });
    await installFixture(page, 'failed-selection'); await login(page); await openWorkflow(page, 'storyboard');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    const primary = page.locator('[data-primary-action]'); expect((await primary.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    const main = page.locator('.workflow-main');
    const footer = page.locator('.workflow-footer');
    const [paddingBottom, footerBox] = await Promise.all([
      main.evaluate((element) => Number.parseFloat(getComputedStyle(element).paddingBottom)),
      footer.boundingBox(),
    ]);
    expect(paddingBottom).toBeGreaterThanOrEqual(footerBox!.height);
    expect(await footer.evaluate((element) => getComputedStyle(element).position)).toBe('sticky');
    if (viewport.width <= 720) await expect(page.getByLabel('选择分镜')).toBeVisible();
    const currentStep = page.locator('[aria-current="step"]'); await expect(currentStep).toHaveCount(1);
    const transitionDurations = await page.locator('.workflow-page *').evaluateAll((nodes) => nodes.slice(0, 30).map((node) => getComputedStyle(node).transitionDuration));
    expect(transitionDurations.every((duration) => duration === '0s')).toBe(true);
    const suffix = viewport.width === 1440 ? 'desktop' : viewport.width === 768 ? 'tablet' : 'mobile';
    await page.screenshot({ path: `${artifactRoot}/guided-workflow-${suffix}.png`, fullPage: true });
  });
}

test('dirty navigation dialog owns focus and Escape cancels navigation', async ({ page }) => {
  await installFixture(page); await login(page); await openWorkflow(page, 'brief');
  await page.getByLabel('完整剧本').fill('尚未保存的内容');
  await page.getByRole('navigation', { name: '视频制作步骤' }).getByRole('button', { name: /角色与场景/ }).click();
  const dialog = page.getByRole('dialog', { name: '保存当前修改？' }); await expect(dialog).toBeVisible();
  await expect(dialog.locator(':focus')).toHaveCount(1);
  await page.keyboard.press('Escape'); await expect(dialog).toBeHidden(); await expect(page.locator('[data-step="brief"]')).toBeVisible();
});
