import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const pageUrl = new URL("./learn.vue", import.meta.url);
const source = await readFile(pageUrl, "utf8");
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1];
assert.ok(script, "learning page should expose a script setup block");

assert.match(
  source,
  /class="content-refresh-notice"[^>]*aria-live="polite"/,
  "stale learning content should expose a polite, non-blocking refresh notice",
);
assert.match(
  source,
  /classroomCoverRatioClass/,
  "learning classroom preview should apply the returned cover aspect ratio",
);
assert.match(
  source,
  /class="classroom-entry__hero"/,
  "learning page classroom area should begin with a content-platform hero banner",
);
assert.match(
  source,
  /class="classroom-entry__hero-cta"/,
  "learning page classroom hero should expose a primary classroom CTA",
);
assert.match(
  source,
  /class="classroom-entry__item"[\s\S]*role="button"[\s\S]*@click="openClassroom/,
  "learning classroom preview cards should be tappable entry points",
);
assert.match(
  source,
  /<image[\s\S]*class="classroom-entry__cover"[\s\S]*:class="classroomCoverRatioClass\(item\)"[\s\S]*mode="aspectFill"/,
  "learning classroom cover image should use aspectFill inside a ratio-aware container",
);
assert.match(
  source,
  /@error="markClassroomPreviewCoverError\(classroomPreviewMediaKey\(item\)\)"/,
  "learning classroom cover images should fall back when loading fails",
);
assert.match(
  source,
  /class="classroom-entry__cover classroom-entry__cover--fallback"[\s\S]*:class="classroomCoverRatioClass\(item\)"/,
  "learning empty-cover placeholder should keep the same ratio container",
);
assert.match(
  source,
  /\.classroom-entry__cover\.classroom-cover--9x16/s,
  "learning classroom cards should define the portrait cover ratio",
);
assert.match(
  source,
  /v-if="classroomWarning"[\s\S]*?@click="retryClassroomPreview"/,
  "a partial classroom failure should expose a non-blocking retry warning",
);
assert.match(
  source,
  /v-else-if="classroomPreview\.length === 0"/,
  "an empty successful classroom result should remain distinct from partial failure feedback",
);
assert.match(
  source,
  /<button[^>]*class="refresh-retry"[^>]*:disabled="refreshing"[^>]*@click="retryContentRefresh"/,
  "the refresh notice should offer a retry action that is disabled while refreshing",
);
assert.match(
  source,
  /\.refresh-retry\s*\{[^}]*min-height:\s*88rpx/s,
  "the refresh retry action should keep the project's 88rpx minimum touch target",
);

const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, "");
const dir = await mkdtemp(join(tmpdir(), "nx-learn-content-state-"));
const modulePath = join(dir, "learn-content-state.mjs");

const harnessPrelude = `
const ref = (value) => ({ value })
const onMounted = (handler) => { globalThis.__learnHarness.onMounted = handler }
const TYPES_INFO = { 1: { name: '一号' } }
const getStoredSiteConfig = () => globalThis.__learnHarness.cached
const hasSiteConfigLearningSection = (cfg) => cfg?.hasLearning !== false
const refreshSiteConfig = () => globalThis.__learnHarness.refreshSiteConfig()
const userErrorMessage = (error, fallback) => error?.message || fallback
const normalizeTeachers = (cfg) => cfg?.teachers ? [...cfg.teachers] : ['默认老师']
const normalizeCoursewareItems = (cfg) => cfg?.courses ? [...cfg.courses] : ['默认课程']
const listClassroomSeriesApi = (...args) => globalThis.__learnHarness.listSeries(...args)
const listClassroomStandaloneApi = (...args) => globalThis.__learnHarness.listStandalone(...args)
const normalizeClassroomSeries = (value = {}) => ({ ...value, id: String(value.id || '') })
const normalizeClassroomContent = (value = {}) => ({ ...value, id: String(value.id || '') })
`;

await writeFile(
  modulePath,
  `${harnessPrelude}\n${executableScript}\nexport { teachers, coursewareItems, quotes, loading, loadError, refreshError, refreshing, classroomPreview, classroomLoading, classroomWarning, showStoredContent, loadContent, retryContentRefresh, loadClassroomPreview, retryClassroomPreview }\n`,
);

let moduleCounter = 0;

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

async function createHarness() {
  const state = {
    cached: null,
    refreshSiteConfig: async () => ({ teachers: [], courses: [], home: { quotes: { items: [] } } }),
    listSeries: async () => ({ items: [] }),
    listStandalone: async () => ({ items: [] }),
  };
  globalThis.__learnHarness = state;
  globalThis.uni = { switchTab() {}, navigateTo() {} };
  moduleCounter += 1;
  const page = await import(`${pathToFileURL(modulePath).href}?case=${moduleCounter}`);
  return { page, state };
}

try {
  {
    const { page, state } = await createHarness();
    state.cached = {
      teachers: ["缓存老师"],
      courses: ["缓存课程"],
      home: { quotes: { items: ["缓存语录"] } },
    };
    assert.equal(page.showStoredContent(), true);

    state.refreshSiteConfig = async () => {
      throw new Error("暂时无法更新");
    };
    await page.loadContent({ silent: true });

    assert.deepEqual(
      page.teachers.value,
      ["缓存老师"],
      "a silent refresh failure must preserve cached teachers",
    );
    assert.deepEqual(
      page.coursewareItems.value,
      ["缓存课程"],
      "a silent refresh failure must preserve cached courses",
    );
    assert.deepEqual(
      page.quotes.value,
      ["缓存语录"],
      "a silent refresh failure must preserve cached quotes",
    );
    assert.equal(
      page.loadError.value,
      "",
      "a silent refresh failure must not replace content with a blocking error",
    );
    assert.equal(
      page.refreshError.value,
      "暂时无法更新",
      "a silent refresh failure should expose lightweight feedback",
    );
  }

  {
    const { page, state } = await createHarness();
    state.listSeries = async () => {
      throw new Error("系列暂时失败");
    };
    state.listStandalone = async () => ({ items: [] });
    await page.loadClassroomPreview();
    assert.deepEqual(page.classroomPreview.value, []);
    assert.match(page.classroomWarning.value, /系列暂时失败/);
  }

  {
    const { page, state } = await createHarness();
    state.listSeries = async () => ({ items: [] });
    state.listStandalone = async () => {
      throw new Error("独立课件暂时失败");
    };
    await page.loadClassroomPreview();
    assert.deepEqual(page.classroomPreview.value, []);
    assert.match(page.classroomWarning.value, /独立课件暂时失败/);
  }

  {
    const { page, state } = await createHarness();
    state.cached = {
      teachers: ["缓存老师"],
      courses: ["缓存课程"],
      home: { quotes: { items: ["缓存语录"] } },
    };
    page.showStoredContent();
    state.listSeries = async () => ({ items: [{ id: 12, title: "可用系列" }] });
    state.listStandalone = async () => {
      throw new Error("独立课件失败");
    };
    await page.loadClassroomPreview();
    assert.equal(
      page.classroomPreview.value[0].title,
      "可用系列",
      "successful partial data should remain visible",
    );
    assert.match(
      page.classroomWarning.value,
      /独立课件失败/,
      "partial failure should remain retryable beside successful data",
    );
    assert.deepEqual(page.teachers.value, ["缓存老师"]);
    assert.deepEqual(
      page.coursewareItems.value,
      ["缓存课程"],
      "classroom failure must preserve legacy home courses",
    );
    assert.deepEqual(page.quotes.value, ["缓存语录"]);

    state.listSeries = async () => ({ items: [{ id: 13, title: "恢复系列" }] });
    state.listStandalone = async () => ({ items: [{ id: 23, title: "恢复课件" }] });
    await page.retryClassroomPreview();
    assert.equal(
      page.classroomWarning.value,
      "",
      "successful retry should clear partial failure feedback",
    );
    assert.deepEqual(
      page.classroomPreview.value.map((item) => item.title),
      ["恢复系列", "恢复课件"],
    );
    assert.deepEqual(
      page.coursewareItems.value,
      ["缓存课程"],
      "classroom retry must not replace legacy fallback courses",
    );
  }

  {
    const { page, state } = await createHarness();
    state.cached = {
      teachers: ["缓存老师"],
      courses: ["缓存课程"],
      home: { quotes: { items: ["缓存语录"] } },
    };
    page.showStoredContent();
    page.refreshError.value = "上次更新失败";
    const pending = deferred();
    state.refreshSiteConfig = () => pending.promise;

    const retryPromise = page.retryContentRefresh();
    assert.equal(
      page.refreshing.value,
      true,
      "retry should expose its in-flight state so the action can be disabled",
    );
    pending.resolve({
      teachers: ["新老师"],
      courses: ["新课程"],
      home: { quotes: { items: ["新语录"] } },
    });
    await retryPromise;

    assert.equal(page.refreshing.value, false);
    assert.equal(
      page.refreshError.value,
      "",
      "a successful retry should clear stale refresh feedback",
    );
    assert.deepEqual(page.teachers.value, ["新老师"]);
    assert.deepEqual(page.coursewareItems.value, ["新课程"]);
    assert.deepEqual(page.quotes.value, ["新语录"]);
  }

  {
    const { page, state } = await createHarness();
    state.refreshSiteConfig = async () => {
      throw new Error("首次加载失败");
    };

    await page.loadContent();

    assert.equal(
      page.loadError.value,
      "首次加载失败",
      "an initial failure without cache should remain blocking",
    );
    assert.equal(
      page.refreshError.value,
      "",
      "an initial failure should not use stale-content feedback",
    );
    assert.deepEqual(page.teachers.value, ["默认老师"]);
    assert.deepEqual(page.coursewareItems.value, ["默认课程"]);
    assert.deepEqual(page.quotes.value, []);
  }

  {
    const { page, state } = await createHarness();
    state.cached = {
      hasLearning: false,
      home: { hero: { title: "只有首页配置" } },
    };
    state.refreshSiteConfig = async () => {
      throw new Error("学习内容首次加载失败");
    };

    const hasCachedContent = page.showStoredContent();
    assert.equal(
      hasCachedContent,
      false,
      "an unrelated site-config cache must not count as cached learning content",
    );
    await page.loadContent({ silent: hasCachedContent });

    assert.equal(
      page.loadError.value,
      "学习内容首次加载失败",
      "an unrelated cache must keep refresh failure blocking",
    );
    assert.equal(
      page.refreshError.value,
      "",
      "an unrelated cache must not expose stale-learning refresh feedback",
    );
  }

  console.log("miniapp learn content state tests passed");
} finally {
  await rm(dir, { force: true, recursive: true });
}
