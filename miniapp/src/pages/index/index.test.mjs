import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const pageUrl = new URL("./index.vue", import.meta.url);
const source = await readFile(pageUrl, "utf8");
const template = source.match(/<template>([\s\S]*?)<\/template>/)?.[1] || "";
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1] || "";
const style = source.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] || "";
const theme = await readFile(new URL("../../styles/apple-mobile.css", import.meta.url), "utf8");

assert.ok(template && script && style, "home page should expose template, executable page state, and scoped styles");

const requiredOrder = [
  "home-nav",
  "expert-hero",
  "proof-stats",
  "carousel",
  "enterprise-services",
  "test-game",
  "classroom-preview",
  "secondary-entries",
  "enterprise-final-cta",
];
let previousIndex = -1;
for (const className of requiredOrder) {
  const index = template.indexOf(`class="${className}`);
  assert.ok(index > previousIndex, `${className} should appear in the required home information order`);
  previousIndex = index;
}
assert.ok(
  template.indexOf('class="carousel') > template.indexOf('class="proof-stats'),
  "carousel should support the expert story instead of leading the home page",
);
assert.match(template, /v-if="view\.proofStats\.length"[^>]*class="proof-stats/, "proof stats should render only with configured data");
assert.match(template, /v-if="carousel\.items\.length"[^>]*class="carousel/, "carousel should render only when images remain available");
assert.match(template, /v-if="view\.game\.enabled"[^>]*class="test-game/, "test game should honor its enabled flag");
assert.match(
  template,
  /class="expert-hero__secondary"[\s\S]{0,220}@click="goClassroom"/,
  "the hero classroom CTA should open standalone classroom courseware rather than the generic learning tab",
);
assert.match(template, /:autoplay="carousel\.items\.length > 1 && carousel\.autoplay && !carouselPaused"/, "carousel autoplay should respect pause state");
assert.match(template, /class="carousel__image"[\s\S]{0,240}lazy-load[\s\S]{0,240}:aria-label=[\s\S]{0,180}@error="removeCarouselItem\(item\.image\)"/, "carousel images should keep lazy loading, accessible labels, and failure isolation");
assert.match(template, /class="carousel__toggle"[\s\S]{0,260}@click="toggleCarouselPaused"/, "carousel should expose an accessible pause control");
assert.match(template, /老师正在整理更多视频与音频内容，稍后再来看看。/, "empty classroom preview should use visitor-facing copy");
const secondaryTemplate = template.slice(
  template.indexOf('class="secondary-entries"'),
  template.indexOf('class="enterprise-final-cta"'),
);
assert.equal(
  (secondaryTemplate.match(/class="section-heading"/g) || []).length,
  1,
  "secondary entries should have one section heading rather than a nested duplicate",
);
assert.match(theme, /--nx-home-gold-halo:\s*rgba\(/, "the root theme should centralize translucent home colors in semantic CSS variables");
assert.doesNotMatch(style, /rgba\(/, "home style rules should consume root semantic variables instead of scattered rgba literals");

for (const state of ["loading", "stale", "empty", "error"]) {
  assert.match(source, new RegExp(`NxAsyncState[\\s\\S]{0,360}state=["']${state}["']`), `home should connect NxAsyncState ${state}`);
}
assert.match(source, /@action="retrySiteConfig"/, "stale config should expose a retry action");
assert.match(source, /@action="retryClassroomPreview"/, "classroom async states should expose retry");
assert.match(source, /:busy="siteRefreshing"/, "site retry should disable duplicate work while busy");
assert.match(source, /:busy="classroomLoading"/, "classroom retry should disable duplicate work while busy");
assert.doesNotMatch(source, /发布后的独立视频与音频课件/, "classroom empty copy should speak to visitors instead of backend publishing workflow");

assert.match(source, /listClassroomStandaloneApi\(\{\s*limit:\s*2,\s*offset:\s*0\s*\}\)/, "home should request two standalone classroom items");
assert.match(source, /\.map\(normalizeClassroomContent\)[\s\S]*?\.filter\(\(item\)\s*=>\s*item\.id\)[\s\S]*?\.slice\(0,\s*2\)/, "classroom preview should normalize, reject missing ids, preserve order, and cap at two");
assert.match(source, /classroomContentRoute\(item\)/, "classroom cards should use the shared detail route helper");
assert.match(source, /\/pages\/classroom\/classroom\?tab=standalone/, "view-all should default to standalone classroom content");
assert.match(source, /classroomAccessLabel/, "classroom cards should explain access permission");
assert.match(source, /formatDuration\(item\.durationSeconds\)/, "classroom cards should expose useful duration metadata");

assert.match(source, /expertHero\.image/, "expert hero should render the configured teacher image");
assert.match(
  template,
  /<image\b(?=[^>]*class="expert-hero__image")(?=[^>]*:src="view\.expertHero\.image")(?=[^>]*:key="view\.expertHero\.image")(?=[^>]*:data-image="view\.expertHero\.image")[^>]*>/,
  "teacher portrait should bind its source, render key, and error dataset to the current image identity",
);
assert.match(source, /teacherImageFailed/, "teacher image should own an isolated failure state");
assert.match(source, /failedCarouselImages/, "carousel images should keep an isolated failed-image Set");
assert.match(source, /courseCoverErrors/, "course covers should keep isolated fallback state");
assert.match(template, /class="classroom-card__cover-fallback"/, "missing classroom covers should use a CSS-only placeholder");
assert.doesNotMatch(template, /classroom-card__cover-fallback"[^>]*>[\s\S]{0,80}[😀-🙏]/u, "course fallback should not use emoji");

assert.match(source, /setBookingIntent\(\{\s*kind:\s*["']enterprise["'],\s*intentText:\s*["']["']\s*\}\)/, "primary enterprise CTA should store an empty enterprise intent");
assert.match(source, /intentText:\s*service\.title/, "enterprise service cards should store their title as intent text");
assert.match(source, /switchTab\(\{\s*url:\s*["']\/pages\/booking\/booking["']\s*\}\)/, "enterprise actions should switch to booking tab");
assert.match(source, /navigateTo\(\{\s*url:\s*["']\/pages\/test\/test["']\s*\}\)/, "game should always navigate to the test page");
assert.match(template, /18道生活情境题/, "test section should explain the fixed question count");
assert.match(template, /约3分钟/, "test section should explain the approximate completion time");
assert.doesNotMatch(template, /secondaryEntries[\s\S]{0,180}test/i, "test should not be duplicated in secondary navigation");
assert.match(source, /MINIAPP_HOME_ENTRY_BEHAVIORS\[entry\.key\]/, "secondary entries should use fixed route behavior instead of configured URLs");

for (const fictionalProof of [/80\+/, /96\s*%/, /虚构客户案例/, /客户案例/]) {
  assert.doesNotMatch(source, fictionalProof, "home should not include unconfigured demonstration proof");
}
assert.doesNotMatch(template, /view\.cases|case-card/, "empty cases should not create a decorative case section");
assert.doesNotMatch(template, /role="button"/, "interactive regions should use native buttons without nested controls");
for (const tag of template.match(/<[^>]+@click[^>]*>/g) || []) {
  assert.match(tag, /^<button\b/, `click interaction should have one native button region: ${tag}`);
}
for (const className of [
  "home-nav__profile",
  "expert-hero__primary",
  "expert-hero__secondary",
  "enterprise-service",
  "test-game__cta",
  "classroom-card",
  "secondary-entry",
  "enterprise-final-cta__button",
]) {
  assert.match(source, new RegExp(`\\.${className}\\s*\\{[^}]*min-height:\\s*88rpx`, "s"), `${className} should meet the 88rpx touch target`);
}
for (const token of ["--nx-brand-900", "--nx-brand-700", "--nx-accent-gold", "--nx-page-bg", "--nx-surface", "--nx-text", "--nx-text-muted", "--nx-border"]) {
  assert.match(source, new RegExp(`var\\(${token}\\)`), `home should use semantic token ${token}`);
}
assert.doesNotMatch(source, /purple|#7229ad|#6338c7|#7b3bc7/i, "home should not retain the old purple entertainment palette");
const nonSemanticRgbaLines = source
  .split("\n")
  .map((line, index) => ({ index: index + 1, line: line.trim() }))
  .filter(({ line }) => line.includes("rgba(") && !/^--nx-home-[a-z0-9-]+:\s*[^;]*rgba\(/.test(line));
assert.deepEqual(nonSemanticRgbaLines, [], "home rgba values should be centralized as --nx-home-* semantic CSS variables");

const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, "");
const dir = await mkdtemp(join(tmpdir(), "nx-home-page-state-"));
const modulePath = join(dir, "index-state.mjs");
const prelude = `
const ref = (value) => ({ value })
const computed = (getter) => ({ get value() { return getter() } })
const onMounted = (handler) => { globalThis.__homeHarness.onMounted = handler }
const getStoredSiteConfig = () => globalThis.__homeHarness.cached
const refreshSiteConfig = () => {
  globalThis.__homeHarness.siteCalls += 1
  return globalThis.__homeHarness.refreshSiteConfig()
}
const normalizePersonalExpertHome = (cfg = {}) => ({
  brand: { enabled: true, name: cfg.name || '默认品牌', tagline: '默认标语' },
  expertHero: { eyebrow: '导师', title: cfg.teacher || '默认老师', lead: '默认介绍', image: cfg.teacherImage || '', monogram: '九' },
  proofStats: cfg.stats || [],
  enterprise: { eyebrow: '企业', title: '团队服务', lead: '服务介绍', buttonText: '预约沟通', modules: [], services: cfg.services || [{ title: '团队共学', description: '共学介绍' }] },
  game: { enabled: cfg.gameEnabled !== false, eyebrow: '探索', title: '人格测试', lead: '了解自己', buttonText: '开始测试' },
  secondaryEntries: cfg.entries || [
    { key: 'test', enabled: true, title: '不应重复的测试', description: '', icon: 'compass' },
    { key: 'relation', enabled: true, title: '关系', description: '', icon: 'relation', url: '/configured/evil' },
    { key: 'learn', enabled: false, title: '课程', description: '', icon: 'book' },
    { key: 'profile', enabled: true, title: '档案', description: '', icon: 'growth' },
  ],
  cases: [],
})
const normalizeHomeCarousel = (cfg = {}) => ({ autoplay: true, interval: 4000, items: (cfg.images || []).map((image) => ({ image })) })
const filterFailedCarouselItems = (carousel, failed) => ({ ...carousel, items: carousel.items.filter((item) => !failed.has(item.image)) })
const MINIAPP_HOME_ENTRY_BEHAVIORS = {
  relation: { method: 'navigateTo', url: '/pages/relation/relation', ariaLabel: '关系' },
  learn: { method: 'switchTab', url: '/pages/learn/learn', ariaLabel: '课程' },
  profile: { method: 'switchTab', url: '/pages/profile/profile', ariaLabel: '档案' },
}
const setBookingIntent = (intent) => { globalThis.__homeHarness.intents.push(intent); return true }
const listClassroomStandaloneApi = (query) => {
  globalThis.__homeHarness.classroomCalls.push(query)
  return globalThis.__homeHarness.listStandalone(query)
}
const normalizeClassroomContent = (item = {}) => ({ ...item, id: String(item.id || '') })
const classroomContentRoute = (item) => item?.id ? '/detail/' + item.id : ''
const classroomAccessLabel = (value) => value === 'paid' ? '付费课件' : '免费'
`;

await writeFile(
  modulePath,
  `${prelude}\n${executableScript}\nexport { view, carousel, secondaryEntries, siteStale, siteRefreshing, teacherImageFailed, failedCarouselImages, classroomItems, classroomLoading, classroomError, classroomState, courseCoverErrors, initializeHome, retrySiteConfig, loadClassroomPreview, retryClassroomPreview, markTeacherImageError, removeCarouselItem, markCourseCoverError, bookEnterprise, bookEnterpriseService, startTest, activateSecondaryEntry, openClassroomItem, goClassroom, formatDuration }\n`,
);

let caseId = 0;
function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

async function createHarness() {
  const state = {
    cached: null,
    siteCalls: 0,
    classroomCalls: [],
    intents: [],
    navigation: [],
    refreshSiteConfig: async () => ({}),
    listStandalone: async () => ({ items: [] }),
  };
  globalThis.__homeHarness = state;
  globalThis.uni = {
    navigateTo(options) { state.navigation.push({ method: "navigateTo", ...options }); },
    switchTab(options) { state.navigation.push({ method: "switchTab", ...options }); },
  };
  caseId += 1;
  const page = await import(`${pathToFileURL(modulePath).href}?case=${caseId}`);
  return { page, state };
}

try {
  {
    const { page, state } = await createHarness();
    state.cached = { name: "缓存品牌", teacher: "缓存老师", teacherImage: "/teacher-a.png", images: ["/bad.png", "/good.png"] };
    state.refreshSiteConfig = async () => { throw new Error("刷新失败"); };
    await page.initializeHome();
    assert.equal(page.view.value.brand.name, "缓存品牌", "cached expert content should render immediately and survive refresh failure");
    assert.equal(page.siteStale.value, true, "cached content should enter stale state when silent refresh fails");
    assert.equal(page.siteRefreshing.value, false);
  }

  {
    const { page, state } = await createHarness();
    const pending = deferred();
    state.refreshSiteConfig = () => pending.promise;
    const first = page.retrySiteConfig();
    const second = page.retrySiteConfig();
    assert.equal(page.siteRefreshing.value, true, "site retry should expose busy state");
    assert.equal(state.siteCalls, 1, "busy site retry should suppress duplicate refresh calls");
    pending.resolve({ teacher: "新老师", teacherImage: "/teacher-b.png" });
    await Promise.all([first, second]);
    assert.equal(page.siteRefreshing.value, false);
    assert.equal(page.siteStale.value, false);
  }

  {
    const { page, state } = await createHarness();
    state.cached = { teacher: "缓存老师", teacherImage: "/teacher-a.png", images: ["/bad.png", "/good.png"] };
    state.refreshSiteConfig = async () => ({ teacher: "刷新老师", teacherImage: "/teacher-b.png", images: ["/bad.png", "/good.png"] });
    page.markTeacherImageError();
    page.markCourseCoverError("course:1");
    page.removeCarouselItem("/bad.png");
    assert.equal(page.teacherImageFailed.value, true);
    assert.equal(page.courseCoverErrors.value["course:1"], true);
    assert.deepEqual(page.carousel.value.items, [], "carousel failure should not depend on teacher or course cover state before config is applied");
    await page.initializeHome();
    assert.equal(page.teacherImageFailed.value, false, "fresh config should reset teacher image failure");
    assert.deepEqual(page.carousel.value.items.map((item) => item.image), ["/good.png"], "failed carousel URLs should stay filtered after refresh");
    assert.equal(page.courseCoverErrors.value["course:1"], true, "site refresh should not mutate independent course cover failures");
    page.markTeacherImageError({ currentTarget: { dataset: { image: "/teacher-a.png" } } });
    assert.equal(page.teacherImageFailed.value, false, "a late error from the replaced teacher image must not hide the refreshed image");
    page.markTeacherImageError({ currentTarget: { dataset: { image: "/teacher-b.png" } } });
    assert.equal(page.teacherImageFailed.value, true, "the current teacher image should still fall back after its own error");
  }

  {
    const { page, state } = await createHarness();
    const pending = deferred();
    state.listStandalone = () => pending.promise;
    const first = page.loadClassroomPreview();
    const second = page.retryClassroomPreview();
    assert.equal(page.classroomState.value, "loading");
    assert.equal(state.classroomCalls.length, 1, "busy classroom retry should suppress duplicate requests");
    pending.resolve({ items: [{ id: 0 }, { id: 2, title: "第一项" }, { id: 3, title: "第二项" }, { id: 4, title: "第三项" }] });
    await Promise.all([first, second]);
    assert.deepEqual(page.classroomItems.value.map((item) => item.id), ["2", "3"], "classroom preview should filter ids, preserve API order, and cap at two");
    assert.deepEqual(state.classroomCalls[0], { limit: 2, offset: 0 });
    assert.equal(page.classroomState.value, "ready");
  }

  {
    const { page, state } = await createHarness();
    const brandBefore = page.view.value.brand.name;
    state.listStandalone = async () => { throw new Error("课堂失败"); };
    await page.loadClassroomPreview();
    assert.equal(page.classroomState.value, "error");
    assert.equal(page.classroomError.value, "课堂失败");
    assert.equal(page.view.value.brand.name, brandBefore, "classroom failure must not block or replace other home modules");
    state.listStandalone = async () => ({ items: [] });
    await page.retryClassroomPreview();
    assert.equal(page.classroomState.value, "empty");
  }

  {
    const { page, state } = await createHarness();
    state.cached = { home: { miniappHome: { entriesSection: { enabled: false } } } };
    state.refreshSiteConfig = async () => { throw new Error("刷新失败"); };
    await page.initializeHome();
    assert.deepEqual(page.secondaryEntries.value, [], "disabled miniapp home entry sections should suppress every secondary entry");
  }

  {
    const { page, state } = await createHarness();
    const malformedConfig = Object.defineProperty({ name: "异常配置品牌" }, "home", {
      get() {
        throw new Error("bad home getter");
      },
    });
    state.cached = malformedConfig;
    state.refreshSiteConfig = async () => ({});
    await assert.doesNotReject(
      () => page.initializeHome(),
      "malformed entriesSection access should not crash the home page",
    );
    assert.deepEqual(page.secondaryEntries.value.map((entry) => entry.key), ["relation", "profile"]);
  }

  {
    const { page, state } = await createHarness();
    assert.deepEqual(page.secondaryEntries.value.map((entry) => entry.key), ["relation", "profile"], "secondary navigation should include enabled relation/learn/profile entries only");
    page.bookEnterprise();
    page.bookEnterpriseService({ title: "领导力工作坊" });
    page.startTest();
    page.activateSecondaryEntry({ key: "relation", url: "/configured/evil" });
    page.openClassroomItem({ id: "9", contentType: "audio" });
    page.goClassroom();
    assert.deepEqual(state.intents, [
      { kind: "enterprise", intentText: "" },
      { kind: "enterprise", intentText: "领导力工作坊" },
    ]);
    assert.deepEqual(state.navigation, [
      { method: "switchTab", url: "/pages/booking/booking" },
      { method: "switchTab", url: "/pages/booking/booking" },
      { method: "navigateTo", url: "/pages/test/test" },
      { method: "navigateTo", url: "/pages/relation/relation" },
      { method: "navigateTo", url: "/detail/9" },
      { method: "navigateTo", url: "/pages/classroom/classroom?tab=standalone" },
    ], "home actions should use fixed booking, test, secondary, and classroom routes");
    assert.equal(page.formatDuration(185), "03:05");
  }

  {
    const { page, state } = await createHarness();
    const disabledEntriesConfig = {
      home: {
        miniappHome: {
          entriesSection: { enabled: false },
        },
      },
    };
    state.cached = disabledEntriesConfig;
    state.refreshSiteConfig = async () => disabledEntriesConfig;
    await page.initializeHome();
    assert.deepEqual(page.secondaryEntries.value, [], "disabled miniappHome entriesSection should hide secondary navigation even when entries are enabled");
  }

  console.log("personal expert home page tests passed");
} finally {
  await rm(dir, { force: true, recursive: true });
}
