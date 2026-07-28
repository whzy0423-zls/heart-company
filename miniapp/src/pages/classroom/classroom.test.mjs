import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const source = await readFile(new URL("./classroom.vue", import.meta.url), "utf8").catch(() => "");

assert.ok(source, "classroom list page should exist");
assert.match(source, /listClassroomSeriesApi/, "classroom list should load published series");
assert.match(
  source,
  /listClassroomStandaloneApi/,
  "classroom list should load standalone courseware",
);
assert.match(
  source,
  /getClassroomSeriesApi/,
  "classroom list should expand a series into its lessons",
);
assert.match(
  source,
  /normalizeClassroomSeries/,
  "series metadata should be normalized before rendering",
);
assert.match(
  source,
  /normalizeClassroomContent/,
  "courseware metadata should be normalized before rendering",
);
assert.match(
  source,
  /classroomCoverRatioClass/,
  "classroom cover cards should apply the returned cover aspect ratio",
);
assert.match(
  source,
  /<image[\s\S]*class="classroom-card__cover"[\s\S]*:class="classroomCoverRatioClass\(item\)"[\s\S]*mode="aspectFill"/,
  "classroom cover images should use aspectFill inside a ratio-aware container",
);
assert.match(
  source,
  /@error="markCoverImageError\(coverMediaKey\(item\)\)"/,
  "classroom cover images should fall back when loading fails",
);
assert.match(
  source,
  /class="classroom-card__cover classroom-card__cover--fallback"[\s\S]*:class="classroomCoverRatioClass\(item\)"/,
  "classroom empty-cover placeholder should keep the same ratio container",
);
assert.match(
  source,
  /\.classroom-card__cover\.classroom-cover--9x16/s,
  "classroom cards should define the portrait cover ratio",
);
assert.match(
  source,
  /class="classroom-tabs"[^>]*role="tablist"/,
  "classroom should expose a two-entry tab list",
);
assert.match(source, /activeTab === 'series'/, "classroom should expose the series entry");
assert.match(source, /activeTab === 'standalone'/, "classroom should expose the standalone entry");
assert.match(source, />\s*系列课程\s*</, "series tab should have a clear label");
assert.match(source, />\s*独立课件\s*</, "standalone tab should have a clear label");
assert.match(
  source,
  /v-if="loading"[^>]*class="classroom-state/,
  "classroom should render a safe loading state",
);
assert.match(
  source,
  /v-else-if="loadError"[^>]*class="classroom-state classroom-state--error/,
  "classroom should render a safe error state",
);
assert.match(source, /@click="retryActiveList"/, "list errors should provide retry");
assert.match(
  source,
  /v-else-if="activeItems\.length === 0"[^>]*class="classroom-state/,
  "classroom should render an empty state",
);
assert.match(source, /aria-live="polite"/, "async classroom feedback should be announced politely");
assert.match(source, /function\s+openSeries\s*\(/, "series cards should open their lesson list");
assert.match(
  source,
  /v-for="item in activeItems"[^>]*class="classroom-list__item"/,
  "each series should own an inline detail region in the long list",
);
assert.match(
  source,
  /v-if="activeTab === 'series' && selectedSeries\?\.id === item\.id"[^>]*class="series-panel/,
  "series lessons should render immediately after the selected card",
);
assert.match(
  source,
  /classroom-card--selected/,
  "the selected series card should expose immediate visual feedback",
);
assert.match(
  source,
  /classroom-card--loading/,
  "the selected series card should expose request progress",
);
assert.match(
  source,
  /selectedSeries\.value\s*=\s*item/,
  "series retry should retain the series the user selected",
);
assert.match(
  source,
  /@click="retrySelectedSeries"/,
  "series load retry should retry the selected series",
);
assert.match(
  source,
  /if\s*\(!force\s*&&\s*loadedTabs\.value\[tab\]\)\s*\{[\s\S]*?loading\.value\s*=\s*false/,
  "switching back to a loaded tab should settle an older loading state",
);
assert.match(source, /function\s+openContent\s*\(/, "courseware cards should open content detail");
assert.match(
  source,
  /classroomContentRoute/,
  "content navigation should use the safe route helper",
);
assert.match(source, /classroomAccessLabel/, "list cards should explain effective access");
assert.match(
  source,
  /classroomPurchaseAction/,
  "list cards should expose the effective access action",
);
assert.match(source, /startSeriesPurchase/, "paid series cards should expose direct purchase");
assert.match(
  source,
  /createClassroomOrderApi\("series",\s*item\.id\)/,
  "series checkout must create a series order",
);
assert.match(
  source,
  /getClassroomOrderStatusApi\("series"/,
  "series payment polling must query series status",
);
assert.match(
  source,
  /@click\.stop="startSeriesPurchase\(item\)"/,
  "series purchase CTA must not toggle the card",
);
assert.match(source, /seriesPaymentState/, "series cards should render payment state feedback");
assert.match(
  source,
  /getClassroomContinueLearningApi/,
  "classroom list should load the signed-in learner's recent progress",
);
assert.match(
  source,
  /class="continue-learning/,
  "classroom list should render a continue-learning card",
);
assert.match(
  source,
  /positionSeconds/,
  "continue-learning should expose the saved resume position",
);
assert.match(
  source,
  /openContinueLearning/,
  "continue-learning should preserve its resume position when opening detail",
);
assert.match(source, /completed/, "continue-learning should expose completion state");
assert.match(source, /role="progressbar"/, "continue-learning progress should be accessible");
for (const forbidden of [/objectKey/i, /mediaUrl/i, /aliyuncs\.com/i, /oss-[a-z0-9-]+\./i]) {
  assert.doesNotMatch(
    source,
    forbidden,
    "classroom list source must not read or render permanent media locations",
  );
}

const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1];
assert.ok(script, "classroom list should expose executable page state");
const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, "");
const dir = await mkdtemp(join(tmpdir(), "nx-classroom-page-state-"));
const modulePath = join(dir, "classroom-state.mjs");
const prelude = `
const ref = (value) => ({ value })
const computed = (getter) => ({ get value() { return getter() } })
const onLoad = (handler) => { globalThis.__classroomHarness.onLoad = handler }
const onShow = (handler) => { globalThis.__classroomHarness.onShow = handler }
const onUnload = (handler) => { globalThis.__classroomHarness.onUnload = handler }
const listClassroomSeriesApi = (...args) => globalThis.__classroomHarness.listSeries(...args)
const listClassroomStandaloneApi = (...args) => globalThis.__classroomHarness.listStandalone(...args)
const getClassroomSeriesApi = (...args) => globalThis.__classroomHarness.getSeries(...args)
const getClassroomContinueLearningApi = (...args) => globalThis.__classroomHarness.getContinue(...args)
const createClassroomOrderApi = (...args) => globalThis.__classroomHarness.createOrder(...args)
const getClassroomOrderStatusApi = (...args) => globalThis.__classroomHarness.getOrderStatus(...args)
const devPayClassroomOrderApi = (...args) => globalThis.__classroomHarness.devPay(...args)
const getToken = () => globalThis.__classroomHarness.token
const normalizeClassroomSeries = (value = {}) => ({ ...value, id: String(value.id || '') })
const normalizeClassroomContent = (value = {}) => ({ ...value, id: String(value.id || '') })
const classroomAccessLabel = (value) => value
const classroomContentRoute = (item) => item?.id ? '/detail/' + item.id : ''
const classroomPurchaseAction = (item = {}) => item.purchaseState === 'purchase_required'
  ? { type: 'purchase', label: '立即购买' }
  : { type: 'play', label: '立即学习' }
const createClassroomPurchaseController = (options) => {
  let stopped = false
  const controller = {
    async purchase() {
      options.onChange?.({ state: 'creating', message: 'creating' })
      const order = await options.create()
      if (stopped) return
      options.onChange?.({ state: 'pending', message: 'pending' })
      await options.pay(order)
      if (stopped) return
      const status = await options.status(order)
      if (stopped) return
      if (status?.owned || status?.status === 'paid') {
        options.onChange?.({ state: 'success', message: 'success' })
        await options.onSuccess?.(status)
      }
    },
    retry() { return this.purchase() },
    stop() { stopped = true; globalThis.__classroomHarness.stopCalls += 1 },
    reset() { stopped = true; options.onChange?.({ state: 'idle', message: '' }) },
  }
  globalThis.__classroomHarness.controllers.push(controller)
  return controller
}
const userErrorMessage = (error, fallback) => error?.message || fallback
`;
await writeFile(
  modulePath,
  `${prelude}\n${executableScript}\nexport { activeTab, seriesItems, expandedSeries, selectedSeries, seriesLoading, seriesError, seriesDetails, seriesPaymentTargetId, seriesPaymentState, selectTab, openSeries, retrySelectedSeries, startSeriesPurchase }\n`,
);

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

let harnessCounter = 0;
async function createHarness() {
  const state = {
    listSeries: async () => ({ items: [] }),
    listStandalone: async () => ({ items: [] }),
    getSeries: async () => ({ series: { id: 1 }, contents: [] }),
    getContinue: async () => ({ items: [] }),
    token: "",
    orderCalls: [],
    statusCalls: [],
    stopCalls: 0,
    controllers: [],
    createOrder(targetType, refId) {
      this.orderCalls.push({ targetType, refId });
      return { outTradeNo: `series-${refId}`, payParams: { devMode: true } };
    },
    getOrderStatus(targetType, refId) {
      this.statusCalls.push({ targetType, refId });
      return { status: "paid", owned: true };
    },
    devPay: async () => ({ paid: true }),
  };
  globalThis.__classroomHarness = state;
  globalThis.uni = { navigateTo() {}, switchTab() {}, requestPayment() {} };
  harnessCounter += 1;
  const page = await import(`${pathToFileURL(modulePath).href}?case=${harnessCounter}`);
  return { page, state };
}

try {
  for (const outcome of ["resolve", "reject"]) {
    const { page, state } = await createHarness();
    const staleSeries = deferred();
    state.getSeries = () => staleSeries.promise;
    const oldRequest = page.openSeries({ id: "12", title: "旧系列" });
    assert.equal(page.seriesLoading.value, true);

    page.selectTab("standalone");
    await Promise.resolve();
    assert.equal(page.activeTab.value, "standalone");
    assert.equal(
      page.seriesLoading.value,
      false,
      "tab switch should settle the abandoned series loading state",
    );
    assert.equal(
      page.selectedSeries.value,
      null,
      "tab switch should clear the abandoned series selection",
    );
    assert.equal(page.seriesError.value, "", "tab switch should clear series feedback");

    if (outcome === "resolve")
      staleSeries.resolve({ series: { id: 12, title: "迟到系列" }, contents: [{ id: 21 }] });
    else staleSeries.reject(new Error("迟到失败"));
    await oldRequest;
    assert.equal(
      page.expandedSeries.value,
      null,
      `stale series ${outcome} must not write into standalone state`,
    );
    assert.equal(
      page.seriesError.value,
      "",
      `stale series ${outcome} must not write feedback into standalone state`,
    );

    let reloads = 0;
    state.listSeries = async () => {
      reloads += 1;
      return { items: [] };
    };
    page.selectTab("series");
    await Promise.resolve();
    await Promise.resolve();
    assert.equal(reloads, 1, "switching back to series should load its current list normally");
  }

  {
    const { page, state } = await createHarness();
    let requests = 0;
    state.getSeries = async () => {
      requests += 1;
      return { series: { id: 12, title: "缓存系列" }, contents: [{ id: 21, title: "第一课" }] };
    };
    await page.openSeries({ id: "12", title: "缓存系列" });
    assert.equal(page.selectedSeries.value.id, "12");
    assert.equal(page.expandedSeries.value.contents[0].id, "21");
    assert.equal(requests, 1);

    await page.openSeries({ id: "12", title: "缓存系列" });
    assert.equal(
      page.selectedSeries.value,
      null,
      "clicking the selected series should collapse it",
    );

    await page.openSeries({ id: "12", title: "缓存系列" });
    assert.equal(page.expandedSeries.value.contents[0].id, "21");
    assert.equal(requests, 1, "reopening an already loaded series should reuse its cached detail");
  }

  {
    const { page, state } = await createHarness();
    let requests = 0;
    state.getSeries = async () => {
      requests += 1;
      if (requests === 1) throw new Error("系列加载失败");
      return { series: { id: 13, title: "恢复系列" }, contents: [] };
    };
    await page.openSeries({ id: "13", title: "恢复系列" });
    assert.match(page.seriesError.value, /系列加载失败/);
    assert.equal(
      page.selectedSeries.value.id,
      "13",
      "failed inline series should remain selected for retry",
    );
    await page.retrySelectedSeries();
    assert.equal(page.seriesError.value, "");
    assert.equal(page.expandedSeries.value.series.title, "恢复系列");
    assert.equal(requests, 2, "retry should issue exactly one replacement request");
  }

  for (const outcome of ["resolve", "reject"]) {
    const { page, state } = await createHarness();
    state.getSeries = async () => ({
      series: { id: 12, title: "缓存 A" },
      contents: [{ id: 21, title: "A 第一课" }],
    });
    await page.openSeries({ id: "12", title: "缓存 A" });
    await page.openSeries({ id: "12", title: "缓存 A" });

    const pendingB = deferred();
    state.getSeries = () => pendingB.promise;
    const oldB = page.openSeries({ id: "13", title: "未缓存 B" });
    assert.equal(page.seriesLoading.value, true);
    await page.openSeries({ id: "12", title: "缓存 A" });
    assert.equal(page.selectedSeries.value.id, "12");
    assert.equal(page.expandedSeries.value.series.title, "缓存 A");
    assert.equal(page.seriesLoading.value, false);

    if (outcome === "resolve") {
      pendingB.resolve({ series: { id: 13, title: "迟到 B" }, contents: [] });
    } else {
      pendingB.reject(new Error("迟到 B 失败"));
    }
    await oldB;
    assert.equal(
      page.selectedSeries.value.id,
      "12",
      `late B ${outcome} must keep cached A selected`,
    );
    assert.equal(
      page.expandedSeries.value.series.title,
      "缓存 A",
      `late B ${outcome} must not replace cached A detail`,
    );
    assert.equal(page.seriesLoading.value, false, `late B ${outcome} must not revive loading`);
    assert.equal(page.seriesError.value, "", `late B ${outcome} must not publish stale feedback`);
  }

  {
    const { page, state } = await createHarness();
    state.token = "jwt";
    const payment = deferred();
    state.devPay = () => payment.promise;
    const seriesA = { id: "12", purchaseState: "purchase_required" };
    const seriesB = { id: "13", purchaseState: "purchase_required" };
    const first = page.startSeriesPurchase(seriesA);
    await Promise.resolve();
    await Promise.resolve();
    assert.equal(page.seriesPaymentState.value, "pending");
    const second = page.startSeriesPurchase(seriesB);
    assert.equal(
      second,
      undefined,
      "a different series click must be ignored while payment is active",
    );
    assert.deepEqual(state.orderCalls, [{ targetType: "series", refId: "12" }]);
    assert.equal(
      state.stopCalls,
      0,
      "a different series click must not interrupt the active checkout",
    );
    payment.resolve({ paid: true });
    await first;
  }

  {
    const { page, state } = await createHarness();
    state.token = "jwt";
    let detailCalls = 0;
    state.getSeries = async () => {
      detailCalls += 1;
      return {
        series: { id: 12, title: detailCalls === 1 ? "购买前" : "购买后" },
        contents: [],
      };
    };
    await page.openSeries({ id: "12", title: "购买前" });
    state.listSeries = async () => ({ items: [{ id: 12, title: "购买后" }] });
    await page.startSeriesPurchase({ id: "12", purchaseState: "purchase_required" });
    assert.equal(page.seriesItems.value[0].title, "购买后");
    assert.equal(page.expandedSeries.value.series.title, "购买后");
    assert.equal(detailCalls, 2, "successful payment should refresh the expanded series detail");
  }

  {
    const { page, state } = await createHarness();
    state.token = "jwt";
    page.seriesItems.value = [{ id: "12", title: "购买前" }];
    let detailCalls = 0;
    state.getSeries = async () => {
      detailCalls += 1;
      return { series: { id: 12, title: "购买前" }, contents: [] };
    };
    await page.openSeries({ id: "12", title: "购买前" });
    const list = deferred();
    let listCalls = 0;
    state.listSeries = () => {
      listCalls += 1;
      return list.promise;
    };
    const purchase = page.startSeriesPurchase({ id: "12", purchaseState: "purchase_required" });
    for (let count = 0; count < 10 && listCalls === 0; count += 1) await Promise.resolve();
    assert.equal(listCalls, 1, "successful payment should begin one list refresh");
    state.onUnload();
    list.resolve({ items: [{ id: 12, title: "卸载后迟到" }] });
    await purchase;
    assert.deepEqual(page.seriesItems.value, [{ id: "12", title: "购买前" }]);
    assert.equal(
      detailCalls,
      1,
      "unload must prevent a late success callback from reopening series detail",
    );
  }
} finally {
  await rm(dir, { force: true, recursive: true });
}

console.log("miniapp classroom list page contract tests passed");
