import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const source = await readFile(new URL("./classroom-detail.vue", import.meta.url), "utf8");
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1];
assert.ok(script, "classroom detail should expose executable page state");

assert.match(source, /onHide/, "detail should pause page media when hidden");
assert.match(source, /onShow/, "detail should restore visibility without autoplay");
assert.match(source, /id="classroom-video"/, "video should have a stable page context id");
assert.match(
  source,
  /classroomCoverRatioClass/,
  "detail cover should apply the returned cover aspect ratio",
);
assert.match(
  source,
  /class="detail-head__media"/,
  "detail page should use a media-first hero shell",
);
assert.match(
  source,
  /class="detail-head__cover-shell"[\s\S]*:class="classroomCoverRatioClass\(content\)"/,
  "detail cover should apply aspect ratio on the same full-width shell as list cards",
);
assert.match(
  source,
  /class="detail-head__play"/,
  "detail head should expose a clear media type/play affordance",
);
assert.match(
  source,
  /class="player-panel__body"/,
  "detail player panel should share the refreshed platform-card content body",
);
assert.match(
  source,
  /<image[\s\S]*class="detail-head__cover"[\s\S]*:class="classroomCoverRatioClass\(content\)"[\s\S]*mode="aspectFill"/,
  "detail cover image should use aspectFill inside a ratio-aware container",
);
assert.match(
  source,
  /@error="markCoverImageError"/,
  "detail cover image should fall back when loading fails",
);
assert.match(
  source,
  /class="detail-head__cover detail-head__cover--fallback"[\s\S]*:class="classroomCoverRatioClass\(content\)"/,
  "detail empty-cover placeholder should keep the same ratio container",
);
assert.match(
  source,
  /\.detail-head__cover\.classroom-cover--1x1/s,
  "detail cover should define the square cover ratio",
);
assert.match(source, /<slider\b[^>]*@change="seekAudio"/, "audio player should expose seeking");
assert.match(
  source,
  /v-if="!content\.canPlay"[^>]*class="access-panel/,
  "locked content should render its effective permission CTA",
);
assert.match(
  source,
  /createClassroomOrderApi|requestPayment|getClassroomOrderStatusApi/,
  "detail should expose classroom purchase and payment status behavior",
);
assert.match(
  source,
  /createClassroomProgressTracker/,
  "detail should track classroom learning progress",
);
assert.match(
  source,
  /updateClassroomProgressApi/,
  "logged-in progress should use the server endpoint",
);
assert.match(
  source,
  /@timeupdate="handleVideoTimeUpdate"/,
  "video playback should report progress",
);
assert.match(source, /@pause="handleVideoPause"/, "video pause should flush progress");
assert.match(source, /@ended="handleVideoEnded"/, "video completion should flush progress");
assert.doesNotMatch(
  source,
  /applyProgress\(duration,\s*true\)/,
  "video ended must not mark logged-in completion before the server responds",
);
assert.match(
  source,
  /void flushProgress\(\)/,
  "page hide/unload should safely flush queued progress",
);
assert.doesNotMatch(
  source,
  /void progressTracker\?\.flush\(\)/,
  "lifecycle flush failures must not become unhandled rejections",
);
assert.match(source, /paymentState/, "detail should render explicit payment states");
assert.match(source, /retryPurchase/, "failed or cancelled payment should expose retry");
assert.match(source, /cancelPurchase/, "pending payment feedback should be dismissible");
assert.match(
  source,
  /createClassroomPurchaseController/,
  "purchase polling should use the bounded controller",
);
assert.match(
  source,
  /getClassroomSeriesApi/,
  "series lessons should resolve their parent purchase target",
);
assert.match(
  source,
  /createClassroomOrderApi\(target\.type,\s*target\.id\)/,
  "detail checkout should use the resolved purchase target type",
);
assert.doesNotMatch(
  source,
  /createClassroomOrderApi\("content",\s*contentId\.value\)/,
  "detail must not hard-code inherit lessons to content orders",
);
assert.match(source, /role="progressbar"/, "learning progress should be exposed accessibly");
assert.match(
  source,
  /options\.position/,
  "detail should accept the server-provided continue-learning resume position",
);
assert.doesNotMatch(
  source,
  /<slider\b[^>]*:value="audioPosition"[^>]*:value="audioPosition"/,
  "audio seek value must not be bound twice",
);
for (const forbidden of [/objectKey/i, /mediaUrl/i, /aliyuncs\.com/i, /oss-[a-z0-9-]+\./i]) {
  assert.doesNotMatch(
    source,
    forbidden,
    "detail must not read or render permanent media locations",
  );
}

const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, "");
const dir = await mkdtemp(join(tmpdir(), "nx-classroom-detail-state-"));
const modulePath = join(dir, "detail-state.mjs");
const prelude = `
const ref = (value) => ({ value })
const computed = (getter) => ({ get value() { return getter() } })
const onLoad = (handler) => { globalThis.__detailHarness.lifecycle.load = handler }
const onHide = (handler) => { globalThis.__detailHarness.lifecycle.hide = handler }
const onShow = (handler) => { globalThis.__detailHarness.lifecycle.show = handler }
const onUnload = (handler) => { globalThis.__detailHarness.lifecycle.unload = handler }
const getClassroomContentApi = (...args) => globalThis.__detailHarness.getContent(...args)
const getClassroomSeriesApi = (...args) => globalThis.__detailHarness.getSeries(...args)
const createClassroomOrderApi = (...args) => globalThis.__detailHarness.createOrder(...args)
const getClassroomOrderStatusApi = (...args) => globalThis.__detailHarness.getOrderStatus(...args)
const devPayClassroomOrderApi = (...args) => globalThis.__detailHarness.devPay(...args)
const updateClassroomProgressApi = async (_id, positionSeconds) => ({ positionSeconds, completed: false })
const withClassroomPlaybackRetry = async (id, consume) => {
  globalThis.__detailHarness.playbackCalls += 1
  return consume(await globalThis.__detailHarness.playback(id))
}
const normalizeClassroomContent = (value = {}) => ({
  id: String(value.id || ''), seriesId: String(value.seriesId || ''), title: value.title || '', description: value.description || '',
  teacherName: value.teacherName || '', coverUrl: value.coverUrl || '',
  contentType: value.contentType === 'audio' ? 'audio' : 'video',
  durationSeconds: Number(value.durationSeconds) || 0, accessLevel: value.accessLevel || '', canPlay: value.canPlay === true,
  effectiveAccess: value.effectiveAccess || 'public', priceCents: Number(value.priceCents) || 0, purchaseState: value.purchaseState || 'available',
})
const normalizeClassroomSeries = (value = {}) => ({ ...value, id: String(value.id || ''), priceCents: Number(value.priceCents) || 0 })
const classroomAccessLabel = (value) => value
const classroomPurchaseAction = (item) => item.canPlay ? { type: 'play', label: '立即学习' } : { type: 'login', label: '登录后学习' }
const classroomCompletion = (position, duration) => ({ ratio: duration > 0 ? Math.min(1, position / duration) : 0, completed: duration > 0 && position / duration >= 0.9 })
const getToken = () => globalThis.__detailHarness.token
const readAnonymousClassroomProgress = () => null
const createClassroomProgressTracker = () => ({ record: async (positionSeconds) => ({ positionSeconds, completed: false }), flush: async () => {} })
const createClassroomPurchaseController = (options) => {
  const purchase = async () => {
    options.onChange?.({ state: 'creating', message: 'creating' })
    const order = await options.create()
    options.onChange?.({ state: 'pending', message: 'pending' })
    await options.pay(order)
    return options.status(order)
  }
  return { purchase, retry: purchase, stop() {}, reset() { options.onChange?.({ state: 'idle', message: '' }) } }
}
const userErrorMessage = (error, fallback) => error?.message || fallback
`;
await writeFile(
  modulePath,
  `${prelude}\n${executableScript}\nexport { contentId, content, loading, loadError, playbackUrl, playbackLoading, playbackError, playbackRetryLabel, audioPlaying, audioPosition, purchaseTarget, purchaseTargetError, loadDetail, refreshPlayback, handlePlaybackError, toggleAudio, seekAudio, startPurchase }\n`,
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

function createAudioMock() {
  const callbacks = {};
  const removed = [];
  const audio = {
    autoplay: false,
    src: "",
    duration: 180,
    currentTime: 0,
    playCalls: 0,
    pauseCalls: 0,
    seekCalls: [],
    stopCalls: 0,
    destroyCalls: 0,
    callbacks,
    removed,
    play() {
      this.playCalls += 1;
      callbacks.play?.();
    },
    pause() {
      this.pauseCalls += 1;
      callbacks.pause?.();
    },
    seek(value) {
      this.seekCalls.push(value);
      this.currentTime = value;
    },
    stop() {
      this.stopCalls += 1;
      callbacks.stop?.();
    },
    destroy() {
      this.destroyCalls += 1;
    },
  };
  for (const name of ["Play", "Pause", "Stop", "Ended", "Canplay", "TimeUpdate", "Error"]) {
    const key = name[0].toLowerCase() + name.slice(1);
    audio[`on${name}`] = (callback) => {
      callbacks[key] = callback;
    };
    audio[`off${name}`] = (callback) => {
      removed.push(key);
      if (!callback || callbacks[key] === callback) delete callbacks[key];
    };
  }
  return audio;
}

let moduleCounter = 0;
async function createHarness({ type = "audio" } = {}) {
  const state = {
    lifecycle: {},
    playbackCalls: 0,
    audios: [],
    token: "jwt",
    getContent: async () => ({ id: 21, title: "课件", contentType: type, canPlay: true }),
    getSeries: async () => ({ series: {}, contents: [] }),
    orderCalls: [],
    statusCalls: [],
    createOrder(targetType, refId) {
      this.orderCalls.push({ targetType, refId });
      return { outTradeNo: "cls", payParams: { devMode: true } };
    },
    getOrderStatus(targetType, refId) {
      this.statusCalls.push({ targetType, refId });
      return { status: "pending", owned: false };
    },
    devPay: async () => ({ paid: true }),
    playback: async () => ({ url: `https://signed.example/${type}` }),
    video: {
      pauseCalls: 0,
      pause() {
        this.pauseCalls += 1;
      },
    },
  };
  globalThis.__detailHarness = state;
  globalThis.uni = {
    getStorageSync() {
      return "";
    },
    setStorageSync() {},
    createInnerAudioContext() {
      const audio = createAudioMock();
      state.audios.push(audio);
      return audio;
    },
    createVideoContext() {
      return state.video;
    },
    switchTab() {},
    showToast() {},
  };
  moduleCounter += 1;
  const page = await import(`${pathToFileURL(modulePath).href}?case=${moduleCounter}`);
  page.contentId.value = "21";
  page.content.value = normalizeContent({ id: 21, contentType: type, canPlay: true });
  return { page, state };
}

function normalizeContent(value) {
  return {
    id: String(value.id),
    title: "课件",
    description: "",
    teacherName: "",
    coverUrl: "",
    contentType: value.contentType,
    durationSeconds: 180,
    canPlay: value.canPlay,
    effectiveAccess: "public",
    purchaseState: "available",
  };
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

try {
  {
    const { page, state } = await createHarness();
    const pending = deferred();
    state.playback = () => pending.promise;
    const operation = page.refreshPlayback();
    state.lifecycle.unload();
    pending.resolve({ url: "https://signed.example/late" });
    await operation;
    assert.equal(
      page.playbackUrl.value,
      "",
      "playback resolved after unload must not revive its URL",
    );
    assert.equal(
      state.audios.length,
      0,
      "playback resolved after unload must not create audio resources",
    );
  }

  {
    const { page, state } = await createHarness();
    const pending = deferred();
    state.playback = () => pending.promise;
    const operation = page.refreshPlayback();
    state.lifecycle.unload();
    pending.reject(new Error("late playback failure"));
    await operation;
    assert.equal(
      page.playbackError.value,
      "",
      "playback rejected after unload must not publish stale feedback",
    );
    assert.equal(
      state.audios.length,
      0,
      "playback rejected after unload must not create media resources",
    );
  }

  {
    const { page, state } = await createHarness();
    const pending = deferred();
    state.playback = () => pending.promise;
    const operation = page.refreshPlayback();
    state.lifecycle.hide();
    pending.resolve({ url: "https://signed.example/hidden-late" });
    await operation;
    assert.equal(
      page.playbackUrl.value,
      "",
      "playback resolved after hide must wait for an explicit user retry",
    );
    assert.equal(state.audios.length, 0);
    const calls = state.playbackCalls;
    state.lifecycle.show();
    await flush();
    assert.equal(
      state.playbackCalls,
      calls,
      "showing the page must not autoplay or automatically reacquire playback",
    );
  }

  {
    const { page, state } = await createHarness();
    await page.refreshPlayback();
    const audio = state.audios[0];
    page.toggleAudio();
    assert.equal(audio.playCalls, 1);
    assert.equal(page.audioPlaying.value, true);
    page.seekAudio({ detail: { value: 47 } });
    assert.deepEqual(audio.seekCalls, [47]);
    state.lifecycle.hide();
    assert.equal(audio.pauseCalls, 1, "hide should pause audio");
    assert.equal(state.video.pauseCalls, 1, "hide should pause the page video context");
    const lateError = audio.callbacks.error;
    const calls = state.playbackCalls;
    state.lifecycle.unload();
    assert.equal(audio.destroyCalls, 1, "unload should destroy the page audio context");
    for (const callback of ["play", "pause", "stop", "ended", "canplay", "timeUpdate", "error"]) {
      assert.ok(audio.removed.includes(callback), `unload should detach ${callback}`);
    }
    lateError?.({ statusCode: 403 });
    await flush();
    assert.equal(
      state.playbackCalls,
      calls,
      "late media errors after unload must not issue playback requests",
    );
  }

  {
    const { page, state } = await createHarness({ type: "video" });
    const urls = ["https://signed.example/old", "https://signed.example/fresh"];
    state.playback = async () => ({ url: urls.shift() });
    await page.refreshPlayback();
    assert.equal(state.playbackCalls, 1);
    page.handlePlaybackError({ statusCode: 403 });
    await flush();
    assert.equal(
      state.playbackCalls,
      2,
      "an explicit signed-url authorization error should refresh once",
    );
    assert.equal(page.playbackUrl.value, "https://signed.example/fresh");
    page.handlePlaybackError({ detail: { errMsg: "signature expired" } });
    await flush();
    assert.equal(
      state.playbackCalls,
      2,
      "the same playback lifecycle should only auto-refresh once",
    );
    assert.match(page.playbackError.value, /播放凭证|刷新/);
    assert.match(
      page.playbackRetryLabel.value,
      /凭证|刷新/,
      "signed playback errors should label credential refresh accurately",
    );
  }

  {
    const { page, state } = await createHarness({ type: "video" });
    await page.refreshPlayback();
    const calls = state.playbackCalls;
    page.handlePlaybackError({ detail: { errMsg: "MEDIA_ERR_DECODE unsupported format" } });
    await flush();
    assert.equal(
      state.playbackCalls,
      calls,
      "decode and format errors must not reacquire a signed URL automatically",
    );
    assert.match(page.playbackError.value, /格式|网络|播放失败/);
    assert.match(
      page.playbackRetryLabel.value,
      /重新加载播放/,
      "generic media errors should not be mislabeled as credential expiry",
    );
  }

  {
    const { page, state } = await createHarness({ type: "video" });
    const first = deferred();
    const second = deferred();
    let request = 0;
    state.playback = () => (request++ === 0 ? first.promise : second.promise);
    const older = page.refreshPlayback();
    const newer = page.refreshPlayback();
    assert.equal(
      state.playbackCalls,
      2,
      "a replacement refresh should supersede an older in-flight request",
    );
    second.resolve({ url: "https://signed.example/newest" });
    await newer;
    first.resolve({ url: "https://signed.example/stale" });
    await older;
    assert.equal(
      page.playbackUrl.value,
      "https://signed.example/newest",
      "an older playback response must not replace the latest URL",
    );
  }

  {
    const { page, state } = await createHarness();
    const contentPending = deferred();
    state.getContent = () => contentPending.promise;
    page.content.value = normalizeContent({ id: "", contentType: "audio", canPlay: false });
    const operation = page.loadDetail();
    state.lifecycle.unload();
    contentPending.resolve({ id: 21, contentType: "audio", canPlay: true });
    await operation;
    assert.equal(
      page.content.value.id,
      "",
      "detail resolved after unload must not update page content",
    );
    assert.equal(
      state.playbackCalls,
      0,
      "detail resolved after unload must not start playback acquisition",
    );
  }

  {
    const { page, state } = await createHarness();
    state.getContent = async () => ({
      id: 21,
      seriesId: 12,
      contentType: "audio",
      accessLevel: "inherit",
      effectiveAccess: "paid",
      priceCents: 2990,
      purchaseState: "purchase_required",
      canPlay: false,
    });
    state.getSeries = async () => ({
      series: {
        id: 12,
        effectiveAccess: "paid",
        priceCents: 2990,
        purchaseState: "purchase_required",
        canPlay: false,
      },
      contents: [{ id: 21, seriesId: 12, accessLevel: "inherit" }],
    });
    await page.loadDetail();
    assert.deepEqual(page.purchaseTarget.value, { type: "series", id: "12", ready: true });
    await page.startPurchase();
    assert.deepEqual(state.orderCalls, [{ targetType: "series", refId: "12" }]);
    assert.equal(
      state.orderCalls.some((call) => call.targetType === "content"),
      false,
      "inherit lesson checkout must never create a content order",
    );
  }

  {
    const { page, state } = await createHarness();
    let seriesCalls = 0;
    state.getContent = async () => ({
      id: 22,
      seriesId: 12,
      contentType: "audio",
      accessLevel: "paid",
      effectiveAccess: "paid",
      priceCents: 990,
      purchaseState: "purchase_required",
      canPlay: false,
    });
    state.getSeries = async () => {
      seriesCalls += 1;
      return { series: {}, contents: [] };
    };
    page.contentId.value = "22";
    await page.loadDetail();
    assert.deepEqual(page.purchaseTarget.value, { type: "content", id: "22", ready: true });
    assert.equal(seriesCalls, 0, "explicit paid content must not resolve a parent purchase target");
    await page.startPurchase();
    assert.deepEqual(state.orderCalls, [{ targetType: "content", refId: "22" }]);
  }

  {
    const { page, state } = await createHarness();
    state.getContent = async () => ({
      id: 23,
      seriesId: 13,
      contentType: "audio",
      accessLevel: "inherit",
      effectiveAccess: "paid",
      purchaseState: "purchase_required",
      canPlay: false,
    });
    state.getSeries = async () => {
      throw new Error("系列加载失败");
    };
    page.contentId.value = "23";
    await page.loadDetail();
    assert.deepEqual(page.purchaseTarget.value, { type: "series", id: "13", ready: false });
    assert.match(page.purchaseTargetError.value, /系列加载失败/);
    await page.startPurchase();
    assert.deepEqual(
      state.orderCalls,
      [],
      "failed parent resolution must never fall back to a content order",
    );
  }

  console.log("miniapp classroom detail state tests passed");
} finally {
  await rm(dir, { force: true, recursive: true });
}
