import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const pageUrl = new URL("./learn.vue", import.meta.url);
const source = await readFile(pageUrl, "utf8");
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1];
assert.ok(script, "learning page should expose a script setup block");
assert.match(source, /import NxAsyncState/, "learning page should share the async-state component");
assert.match(
  source,
  /<NxAsyncState[^>]*state="stale"[\s\S]*?@action="retryContentRefresh"/,
  "cached content refresh failures should use the shared stale state",
);
assert.match(
  source,
  /<NxAsyncState[^>]*state="loading"/,
  "classroom preview should use the shared loading state",
);
assert.match(
  source,
  /<NxAsyncState[^>]*state="empty"[\s\S]*?@action="openClassroom\('standalone'\)"/,
  "an empty classroom preview should keep a visitor-facing shared empty state",
);

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
  /function\s+classroomPreviewPresentation\s*\(/,
  "classroom preview should derive type-specific presentation from one helper",
);
assert.match(
  source,
  /v-for="item in classroomPreview"/,
  "classroom preview template should render the stable source list directly",
);
assert.doesNotMatch(
  source,
  /const\s+classroomPreviewCards\s*=\s*computed\s*\(/,
  "classroom preview should avoid the derived card wrapper that blanks the WeChat page runtime",
);
assert.match(
  source,
  /openClassroom\(classroomPreviewPresentation\(item\)\.tab\)/,
  "classroom preview navigation should use the shared presentation tab",
);
for (const field of ["title", "kind", "fallback", "action"]) {
  assert.match(
    source,
    new RegExp(`classroomPreviewPresentation\\(item\\)\\.${field}`),
    `classroom preview should render its ${field} presentation`,
  );
}
for (const forbidden of [
  /#0f766e/i,
  /#15803d/i,
  /#0f6b4f/i,
  /#ecfdf5/i,
  /#bbf7d0/i,
  /#86efac/i,
  /rgba\(\s*15,\s*118,\s*110/i,
]) {
  assert.doesNotMatch(
    source,
    forbidden,
    "learning page should not keep the old dominant green classroom palette",
  );
}
assert.match(
  source,
  /var\(--nx-brand-900\)|var\(--nx-brand-700\)|var\(--nx-accent-gold\)/,
  "learning page should use the shared graphite-blue and champagne-gold tokens",
);
assert.doesNotMatch(
  source,
  /#4338ca|#4f46e5|#7c3aed|#f59e0b|rgba\(\s*67,\s*56,\s*202/i,
  "learning page should remove the old purple and orange visual tokens",
);
assert.match(source, /learnCopy\.hero\.eyebrow/, "learning page should expose its normalized classroom title");
assert.match(
  source,
  /function\s+openClassroom\s*\(tab\s*=\s*"standalone"\)/,
  "learning page classroom entry should default to standalone courseware first",
);
assert.match(
  source,
  /@click="openClassroom\('standalone'\)"/,
  "learning page classroom entry should open standalone courseware first",
);
assert.match(
  source,
  /class="classroom-entry__intro"/,
  "learning page classroom area should use a compact editorial introduction",
);
assert.match(
  source,
  /class="classroom-entry__more"[\s\S]*:aria-label="learnCopy\.classroom\.ctaText"/,
  "learning page classroom section should expose one clear header action",
);
assert.doesNotMatch(
  source,
  /class="classroom-entry__hero"|class="classroom-entry__hero-cta"|classroom-entry__hero-wave/,
  "learning classroom section should remove the nested promotional hero and decorative waves",
);
const classroomIndex = source.indexOf('class="classroom-entry card ios-card learn-section nx-panel section"');
const teacherIndex = source.indexOf('class="card ios-card learn-section nx-panel section teacher-section"');
const coursewareIndex = source.indexOf('class="card ios-card learn-section nx-panel section courseware-section"');
const typeIndex = source.indexOf('class="card ios-card learn-section nx-panel section type-section"');
const quoteIndex = source.indexOf('class="card ios-card learn-section nx-panel section quote-section"');
assert.ok(
  classroomIndex < teacherIndex &&
    teacherIndex < coursewareIndex &&
    coursewareIndex < typeIndex &&
    typeIndex < quoteIndex,
  "learning page should order classroom, teacher, course direction, enneagram content, then quote",
);
assert.match(
  source,
  /class="classroom-entry__item"[\s\S]*role="button"[\s\S]*@click="openClassroom/,
  "learning classroom preview cards should be tappable entry points",
);
assert.doesNotMatch(
  source,
  /aria-role="button"/,
  "learning classroom cards should not emit the invalid aria-role attribute",
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
  /\.classroom-entry__cover\s*\{[^}]*width:\s*200rpx[^}]*height:\s*112rpx/s,
  "learning classroom cards should use a compact stable 200x112 thumbnail window",
);
assert.doesNotMatch(
  source,
  /\.classroom-entry__cover\.classroom-cover--(?:16x9|9x16|1x1)\s*\{[^}]*height:/s,
  "source cover ratios should not override the stable classroom thumbnail height",
);
assert.match(
  source,
  /\.classroom-entry__body\s*\{[^}]*align-self:\s*stretch[^}]*min-height:\s*112rpx/s,
  "classroom card content should stretch to the thumbnail height for aligned actions",
);
assert.match(
  source,
  /v-if="classroomWarning && classroomPreview\.length > 0"[\s\S]*?@click="retryClassroomPreview"/,
  "a partial classroom failure should expose a non-blocking retry warning",
);
assert.match(
  source,
  /v-else-if="classroomWarning && classroomPreview\.length === 0"[\s\S]*?state="error"/,
  "an empty classroom result with a failed request should use a blocking error state",
);
assert.match(
  source,
  /\.classroom-entry__more\s*\{[^}]*min-height:\s*88rpx/s,
  "the classroom header action should keep the project's 88rpx minimum touch target",
);
assert.match(
  source,
  /\.classroom-entry__intro\s*\{[^}]*border-left:\s*6rpx\s+solid\s+var\(--nx-accent-gold\)/s,
  "the classroom introduction should use one restrained gold editorial accent",
);
assert.match(
  source,
  /<view\s+v-else\s+class="courseware-list">[\s\S]*v-for="\(c, i\) in coursewareItems"/,
  "course directions should render inside an explicit WeChat-stable list container",
);
assert.match(
  source,
  /class="courseware-card__mark"[^>]*>[\s\S]*c\.badge\s*\|\|\s*i\s*\+\s*1/,
  "course directions should use lightweight text marks instead of native image layers",
);
assert.doesNotMatch(
  source,
  /courseware-card__cover|course-media__fallback|courseImageErrors|markCourseImageError/,
  "course direction cards should not keep image layers that create blank WeChat scroll regions",
);

function viewBlockForClass(markup, className) {
  const classIndex = markup.indexOf(`class="${className}"`);
  assert.ok(classIndex >= 0, `expected a ${className} view`);
  const openingIndex = markup.lastIndexOf("<view", classIndex);
  assert.ok(openingIndex >= 0, `${className} should be a view`);

  const viewTag = /<\/?view\b[^>]*>/g;
  viewTag.lastIndex = openingIndex;
  let depth = 0;
  for (let match = viewTag.exec(markup); match; match = viewTag.exec(markup)) {
    if (match[0].startsWith("</")) {
      depth -= 1;
      if (depth === 0) return markup.slice(openingIndex, viewTag.lastIndex);
    } else {
      depth += 1;
    }
  }
  assert.fail(`${className} should have a closing view tag`);
}

const teacherCardTemplate = viewBlockForClass(source, "teacher-card");
const teacherCardHeader = viewBlockForClass(teacherCardTemplate, "teacher-card__header");
const teacherCardDetails = viewBlockForClass(teacherCardTemplate, "teacher-card__details");
assert.match(
  teacherCardHeader,
  /class="teacher-media teacher-card__avatar"/,
  "teacher-card header should contain the avatar image",
);
assert.match(
  teacherCardHeader,
  /class="teacher-media__fallback"/,
  "teacher-card header should retain the avatar fallback",
);
assert.match(
  teacherCardHeader,
  /class="teacher-card__identity"/,
  "teacher-card header should contain the compact identity",
);
assert.match(
  teacherCardDetails,
  /class="teacher-card__bio"/,
  "teacher-card details should contain the biography",
);
assert.match(
  teacherCardDetails,
  /class="teacher-card__tags"/,
  "teacher-card details should contain the tags",
);
assert.doesNotMatch(
  source,
  /class="teacher-card__body"/,
  "teacher cards should not keep a two-column body that traps biography beside the avatar",
);
assert.match(
  source,
  /\.teacher-card\s*\{[^}]*flex-direction:\s*column[^}]*\}/s,
  "teacher cards should stack the identity header and details vertically",
);
assert.match(
  source,
  /\.teacher-card__details\s*\{[^}]*width:\s*100%[^}]*\}/s,
  "teacher details should span the card width below the avatar header",
);
assert.match(
  source,
  /\.teacher-card__header\s*\{[^}]*flex-direction:\s*row[^}]*\}/s,
  "teacher card header should explicitly keep avatar and identity in a horizontal row",
);


assert.match(
  source,
  /import\s*\{\s*normalizeMiniappLearn\s*\}\s*from\s*["']\.\.\/\.\.\/utils\/miniappPages["']/,
  "learning page should import the dedicated learn-copy view model",
);
assert.match(
  script,
  /const\s+learnCopy\s*=\s*ref\(normalizeMiniappLearn\(\)\)/,
  "learning page should keep a default-backed learn-copy view model",
);
assert.match(
  script,
  /function\s+applyContent\s*\(cfg\)\s*\{[\s\S]*?learnCopy\.value\s*=\s*normalizeMiniappLearn\(cfg\)/,
  "cached and refreshed learning content should both normalize configured copy",
);
assert.match(
  source,
  /v-for="\(item, index\) in learnCopy\.hero\.meta"[^>]*:key="`\$\{item\}-\$\{index\}`"/,
  "learning hero meta should use value plus index keys when configured labels repeat",
);
for (const binding of [
  "learnCopy.hero.eyebrow",
  "learnCopy.hero.title",
  "learnCopy.hero.lead",
  "learnCopy.hero.meta",
  "learnCopy.classroom.eyebrow",
  "learnCopy.classroom.title",
  "learnCopy.classroom.moreText",
  "learnCopy.classroom.heroEyebrow",
  "learnCopy.classroom.heroTitle",
  "learnCopy.classroom.heroLead",
  "learnCopy.classroom.ctaText",
  "learnCopy.classroom.emptyTitle",
  "learnCopy.classroom.emptyDescription",
  "learnCopy.classroom.emptyActionText",
  "learnCopy.sections.teacher.eyebrow",
  "learnCopy.sections.teacher.title",
  "learnCopy.sections.courses.eyebrow",
  "learnCopy.sections.courses.title",
  "learnCopy.sections.types.eyebrow",
  "learnCopy.sections.types.title",
  "learnCopy.sections.quotes.eyebrow",
  "learnCopy.sections.quotes.title",
  "learnCopy.sections.courses.emptyTitle",
  "learnCopy.sections.courses.emptyDescription",
  "learnCopy.sections.quotes.emptyTitle",
  "learnCopy.bottomCtaText",
]) {
  assert.match(
    source,
    new RegExp(binding.replaceAll(".", "\\.")),
    `learning page should render ${binding} from normalized configuration`,
  );
}

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
const normalizeMiniappLearn = (cfg) => globalThis.__learnHarness.normalizeMiniappLearn(cfg)
const listClassroomSeriesApi = (...args) => globalThis.__learnHarness.listSeries(...args)
const listClassroomStandaloneApi = (...args) => globalThis.__learnHarness.listStandalone(...args)
const normalizeClassroomSeries = (value = {}) => ({ ...value, id: String(value.id || '') })
const normalizeClassroomContent = (value = {}) => ({ ...value, id: String(value.id || '') })
`;

await writeFile(
  modulePath,
  `${harnessPrelude}\n${executableScript}\nexport { teachers, coursewareItems, quotes, learnCopy, loading, loadError, refreshError, refreshing, classroomPreview, classroomLoading, classroomWarning, showStoredContent, loadContent, retryContentRefresh, loadClassroomPreview, retryClassroomPreview, classroomPreviewPresentation }\n`,
);

let moduleCounter = 0;

function defaultLearnCopy() {
  return {
    hero: { eyebrow: '老师课堂', title: '跟着老师，把九型真正用进工作与生活', lead: '从视频与音频课件开始，理解自己、改善关系，也为团队协作建立更清晰的共同语言。', meta: ['视频课程', '音频精讲', '九型实践'] },
    classroom: { eyebrow: '课堂精选', title: '视频与音频课件', moreText: '查看全部', heroEyebrow: '随时回看 · 反复练习', heroTitle: '把老师以往开课内容，整理成可以持续学习的专业课件', heroLead: '支持视频和音频；先看独立课件，也可以进入系列课程循序学习。', ctaText: '进入老师课堂', emptyTitle: '老师课堂正在准备中', emptyDescription: '可以先浏览老师介绍和课程方向，新的视频与音频课件会在这里持续更新。', emptyActionText: '进入课堂看看' },
    sections: {
      teacher: { eyebrow: '老师简介', title: '认识你的学习向导' },
      courses: { eyebrow: '课程方向', title: '循序建立九型视角', emptyTitle: '课程方向正在整理中', emptyDescription: '更多面向个人成长、关系沟通与企业团队的学习主题会持续补充。' },
      types: { eyebrow: '九型内容', title: '九种性格，九条成长路径' },
      quotes: { eyebrow: '课堂一念', title: '把觉察带回当下', emptyTitle: '课堂语录即将上线' },
    },
    bottomCtaText: '先完成测试，建立你的学习地图',
  };
}

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
    normalizeMiniappLearn: (cfg) => cfg?.home?.miniappLearn || defaultLearnCopy(),
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
    const { page } = await createHarness();
    for (const [item, expected] of [
      [
        { contentType: "video", title: "" },
        {
          kind: "视频",
          fallback: "视",
          action: "查看课件 ›",
          tab: "standalone",
          title: "未命名课件",
        },
      ],
      [
        { contentType: "audio", title: "音频课" },
        {
          kind: "音频",
          fallback: "音",
          action: "查看课件 ›",
          tab: "standalone",
          title: "音频课",
        },
      ],
      [
        { title: "" },
        {
          kind: "系列",
          fallback: "系",
          action: "查看系列 ›",
          tab: "series",
          title: "未命名系列",
        },
      ],
    ]) {
      assert.deepEqual(page.classroomPreviewPresentation(item), expected);
    }
  }

  {
    const { page, state } = await createHarness();
    state.cached = { home: { miniappLearn: { hero: { title: "缓存课堂标题" } } } };
    assert.equal(page.showStoredContent(), true);
    assert.equal(
      page.learnCopy.value.hero.title,
      "缓存课堂标题",
      "cached learning-page configuration should be applied through the normalized copy view model",
    );

    state.refreshSiteConfig = async () => ({
      home: { miniappLearn: { hero: { title: "更新后的课堂标题" } } },
    });
    await page.loadContent({ silent: true });
    assert.equal(
      page.learnCopy.value.hero.title,
      "更新后的课堂标题",
      "network refresh should replace cached learning-page copy through the same view model",
    );
  }

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
    state.cached = { home: { miniappLearn: { hero: { title: "缓存课堂标题" } } } };
    assert.equal(page.showStoredContent(), true);
    assert.equal(
      page.learnCopy.value.hero.title,
      "缓存课堂标题",
      "cached learning-page configuration should be applied through the normalized copy view model",
    );

    state.refreshSiteConfig = async () => ({
      home: { miniappLearn: { hero: { title: "更新后的课堂标题" } } },
    });
    await page.loadContent({ silent: true });
    assert.equal(
      page.learnCopy.value.hero.title,
      "更新后的课堂标题",
      "network refresh should replace cached learning-page copy through the same view model",
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
      ["恢复课件", "恢复系列"],
      "standalone video and audio courseware should appear before series",
    );
    assert.deepEqual(
      page.coursewareItems.value,
      ["缓存课程"],
      "classroom retry must not replace legacy fallback courses",
    );
  }

  {
    const { page, state } = await createHarness();
    state.cached = { home: { miniappLearn: { hero: { title: "缓存课堂标题" } } } };
    assert.equal(page.showStoredContent(), true);
    assert.equal(
      page.learnCopy.value.hero.title,
      "缓存课堂标题",
      "cached learning-page configuration should be applied through the normalized copy view model",
    );

    state.refreshSiteConfig = async () => ({
      home: { miniappLearn: { hero: { title: "更新后的课堂标题" } } },
    });
    await page.loadContent({ silent: true });
    assert.equal(
      page.learnCopy.value.hero.title,
      "更新后的课堂标题",
      "network refresh should replace cached learning-page copy through the same view model",
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
    assert.deepEqual(
      page.learnCopy.value,
      defaultLearnCopy(),
      'an initial failure should retain the complete normalized learning-page fallback copy',
    );
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
