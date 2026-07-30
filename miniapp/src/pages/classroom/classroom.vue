<script setup>
import { computed, ref } from "vue";
import NxAsyncState from "../../components/NxAsyncState.vue";
import { onLoad, onShow, onUnload } from "@dcloudio/uni-app";
import {
  createClassroomOrderApi,
  devPayClassroomOrderApi,
  getClassroomContinueLearningApi,
  getClassroomOrderStatusApi,
  getClassroomSeriesApi,
  listClassroomSeriesApi,
  listClassroomStandaloneApi,
} from "../../api";
import {
  classroomAccessLabel,
  classroomCoverRatioClass,
  classroomContentRoute,
  classroomPurchaseAction,
  normalizeClassroomContent,
  normalizeClassroomSeries,
} from "../../utils/classroomDisplay";
import { createClassroomPurchaseController } from "../../utils/classroomProgress";
import { getToken } from "../../utils/auth";
import { userErrorMessage } from "../../utils/userMessage";

const activeTab = ref("standalone");
const seriesItems = ref([]);
const standaloneItems = ref([]);
const loadedTabs = ref({ series: false, standalone: false });
const loading = ref(false);
const loadError = ref("");
const expandedSeries = ref(null);
const selectedSeries = ref(null);
const seriesDetails = ref({});
const seriesLoading = ref(false);
const seriesError = ref("");
const continueItem = ref(null);
const continueLoading = ref(false);
const continueError = ref("");
const seriesPaymentTargetId = ref("");
const seriesPaymentState = ref("idle");
const seriesPaymentMessage = ref("");
const seriesPurchaseInFlight = ref(false);
const coverImageErrors = ref({});
let listTicket = 0;
let seriesTicket = 0;
let continueTicket = 0;
let skipNextShowRefresh = false;
let seriesPurchaseController = null;
let seriesPurchaseOperation = null;
let seriesPurchaseTicket = 0;
let disposed = false;

const activeItems = computed(() =>
  activeTab.value === "series" ? seriesItems.value : standaloneItems.value,
);
const emptyCopy = computed(() =>
  activeTab.value === "series" ? "系列课程正在准备中" : "独立课件正在准备中",
);
const emptyDescription = computed(() =>
  activeTab.value === "series"
    ? "系列课程会把相关主题串成完整路径；也可以先从独立课件开始学习。"
    : "老师的公开视频和音频课件会持续整理到这里，欢迎先浏览现有内容。",
);
const seriesPaymentBusy = computed(() => seriesPurchaseInFlight.value);

function responseItems(response) {
  return Array.isArray(response?.items) ? response.items : [];
}

function normalizeContinueItem(value = {}) {
  const item = normalizeClassroomContent(value);
  if (!item.id) return null;
  return {
    ...item,
    positionSeconds: Math.max(0, Math.floor(Number(value.positionSeconds) || 0)),
    completed: value.completed === true,
    lastPlayedAt: String(value.lastPlayedAt || ""),
  };
}

async function loadContinueLearning() {
  const ticket = ++continueTicket;
  if (!getToken()) {
    continueItem.value = null;
    continueLoading.value = false;
    continueError.value = "";
    return;
  }
  continueLoading.value = true;
  continueError.value = "";
  try {
    const response = await getClassroomContinueLearningApi();
    if (ticket !== continueTicket) return;
    continueItem.value = responseItems(response).map(normalizeContinueItem).find(Boolean) || null;
  } catch (error) {
    if (ticket === continueTicket) {
      continueItem.value = null;
      continueError.value = userErrorMessage(error, "学习记录加载失败，请稍后重试");
    }
  } finally {
    if (ticket === continueTicket) continueLoading.value = false;
  }
}

function continuePercent(item) {
  if (item?.completed) return 100;
  const duration = Math.max(0, Number(item?.durationSeconds) || 0);
  if (!duration) return 0;
  return Math.min(
    89,
    Math.max(0, Math.floor(((Number(item?.positionSeconds) || 0) / duration) * 100)),
  );
}

function continueLabel(item) {
  if (item?.completed) return "已完成，可再次学习";
  const position = formatDuration(item?.positionSeconds);
  return position ? `已学习至 ${position}` : "从头开始学习";
}

async function loadActiveList({ force = false } = {}) {
  const tab = activeTab.value;
  if (!force && loadedTabs.value[tab]) {
    listTicket += 1;
    loading.value = false;
    return;
  }
  const ticket = ++listTicket;
  loading.value = true;
  loadError.value = "";
  try {
    const response =
      tab === "series"
        ? await listClassroomSeriesApi({ limit: 50, offset: 0 })
        : await listClassroomStandaloneApi({ limit: 50, offset: 0 });
    if (ticket !== listTicket || tab !== activeTab.value) return;
    if (tab === "series")
      seriesItems.value = responseItems(response)
        .map(normalizeClassroomSeries)
        .filter((item) => item.id);
    else
      standaloneItems.value = responseItems(response)
        .map(normalizeClassroomContent)
        .filter((item) => item.id);
    loadedTabs.value = { ...loadedTabs.value, [tab]: true };
  } catch (error) {
    if (ticket === listTicket && tab === activeTab.value) {
      loadError.value = userErrorMessage(error, "课堂内容加载失败，请稍后重试");
    }
  } finally {
    if (ticket === listTicket && tab === activeTab.value) loading.value = false;
  }
}

function selectTab(tab) {
  if (tab !== "series" && tab !== "standalone") return;
  if (tab !== activeTab.value) {
    seriesTicket += 1;
    expandedSeries.value = null;
    selectedSeries.value = null;
    seriesLoading.value = false;
    seriesError.value = "";
  }
  activeTab.value = tab;
  loadError.value = "";
  loadActiveList();
}

function retryActiveList() {
  return loadActiveList({ force: true });
}

async function openSeries(item, { force = false } = {}) {
  if (!item?.id || activeTab.value !== "series") return;
  if (!force && selectedSeries.value?.id === item.id) {
    seriesTicket += 1;
    selectedSeries.value = null;
    expandedSeries.value = null;
    seriesLoading.value = false;
    seriesError.value = "";
    return;
  }
  const ticket = ++seriesTicket;
  selectedSeries.value = item;
  const cached = seriesDetails.value[item.id];
  if (!force && cached) {
    expandedSeries.value = cached;
    seriesLoading.value = false;
    seriesError.value = "";
    return;
  }
  seriesLoading.value = true;
  seriesError.value = "";
  try {
    const response = await getClassroomSeriesApi(item.id);
    if (
      ticket !== seriesTicket ||
      activeTab.value !== "series" ||
      selectedSeries.value?.id !== item.id
    )
      return;
    const detail = {
      series: normalizeClassroomSeries(response?.series || item),
      contents: (Array.isArray(response?.contents) ? response.contents : [])
        .map(normalizeClassroomContent)
        .filter((content) => content.id),
    };
    seriesDetails.value = { ...seriesDetails.value, [item.id]: detail };
    const existingIndex = seriesItems.value.findIndex((series) => series.id === item.id);
    if (existingIndex >= 0) {
      seriesItems.value = seriesItems.value.map((series, index) =>
        index === existingIndex ? detail.series : series,
      );
    } else {
      seriesItems.value = [detail.series, ...seriesItems.value];
    }
    selectedSeries.value = detail.series;
    expandedSeries.value = detail;
  } catch (error) {
    if (
      ticket === seriesTicket &&
      activeTab.value === "series" &&
      selectedSeries.value?.id === item.id
    ) {
      seriesError.value = userErrorMessage(error, "系列课件加载失败，请稍后重试");
    }
  } finally {
    if (
      ticket === seriesTicket &&
      activeTab.value === "series" &&
      selectedSeries.value?.id === item.id
    )
      seriesLoading.value = false;
  }
}

function retrySelectedSeries() {
  if (!selectedSeries.value) return;
  return openSeries(selectedSeries.value, { force: true });
}

function openContent(item) {
  const url = classroomContentRoute(item);
  if (url) uni.navigateTo({ url });
}

function openContinueLearning(item) {
  const url = classroomContentRoute(item);
  if (!url) return;
  const position = Math.max(0, Math.floor(Number(item?.positionSeconds) || 0));
  uni.navigateTo({ url: `${url}&position=${position}` });
}

function itemAction(item) {
  return classroomPurchaseAction(item);
}

function coverMediaKey(item) {
  return `${item?.contentType || "series"}:${item?.id || ""}`;
}

function markCoverImageError(key) {
  coverImageErrors.value = { ...coverImageErrors.value, [key]: true };
}

function requestSeriesPayment(pay = {}) {
  return new Promise((resolve, reject) => {
    uni.requestPayment({
      provider: "wxpay",
      timeStamp: pay.timeStamp,
      nonceStr: pay.nonceStr,
      package: pay.package,
      signType: pay.signType || "RSA",
      paySign: pay.paySign,
      success: resolve,
      fail: reject,
    });
  });
}

function createSeriesPurchase(item) {
  seriesPurchaseController?.stop();
  const purchaseTicket = ++seriesPurchaseTicket;
  seriesPaymentTargetId.value = item.id;
  seriesPurchaseController = createClassroomPurchaseController({
    create: () => createClassroomOrderApi("series", item.id),
    pay: async (order) => {
      if (order?.payParams?.devMode) return devPayClassroomOrderApi(order.outTradeNo);
      return requestSeriesPayment(order?.payParams);
    },
    status: () => getClassroomOrderStatusApi("series", item.id),
    onChange: (snapshot) => {
      if (disposed || purchaseTicket !== seriesPurchaseTicket) return;
      seriesPaymentState.value = snapshot.state;
      seriesPaymentMessage.value = snapshot.message;
    },
    onSuccess: async () => {
      if (disposed || purchaseTicket !== seriesPurchaseTicket) return;
      loadedTabs.value = { ...loadedTabs.value, series: false };
      await loadActiveList({ force: true });
      if (disposed || purchaseTicket !== seriesPurchaseTicket) return;
      if (selectedSeries.value?.id === item.id) {
        await openSeries(item, { force: true });
        if (disposed || purchaseTicket !== seriesPurchaseTicket) return;
      }
    },
  });
  return seriesPurchaseController;
}

function trackSeriesPurchase(run) {
  if (seriesPurchaseOperation) return;
  seriesPurchaseInFlight.value = true;
  let operation;
  try {
    operation = Promise.resolve(run());
  } catch (error) {
    seriesPurchaseInFlight.value = false;
    throw error;
  }
  let tracked;
  tracked = operation.finally(() => {
    if (seriesPurchaseOperation !== tracked) return;
    seriesPurchaseOperation = null;
    seriesPurchaseInFlight.value = false;
  });
  seriesPurchaseOperation = tracked;
  return tracked;
}

function startSeriesPurchase(item) {
  if (!item?.id || itemAction(item).type !== "purchase") return;
  if (seriesPurchaseOperation) return;
  if (!getToken()) {
    uni.switchTab({ url: "/pages/profile/profile" });
    return;
  }
  return trackSeriesPurchase(() => createSeriesPurchase(item).purchase());
}

function retrySeriesPurchase(item) {
  if (seriesPurchaseOperation) return;
  if (seriesPaymentTargetId.value !== item?.id) return startSeriesPurchase(item);
  if (!seriesPurchaseController) return;
  return trackSeriesPurchase(() => seriesPurchaseController.retry());
}

function cancelSeriesPurchase() {
  seriesPurchaseController?.reset();
  seriesPaymentState.value = "idle";
  seriesPaymentMessage.value = "";
}

function formatDuration(seconds) {
  const value = Math.max(0, Math.floor(Number(seconds) || 0));
  if (!value) return "";
  const minutes = Math.floor(value / 60);
  const remainder = String(value % 60).padStart(2, "0");
  return `${minutes}:${remainder}`;
}

onLoad(async (options = {}) => {
  disposed = false;
  skipNextShowRefresh = true;
  if (options.tab === "series") activeTab.value = "series";
  if (options.tab === "standalone") activeTab.value = "standalone";
  const requestedSeriesId = /^\d+$/.test(String(options.seriesId || "").trim())
    ? String(options.seriesId).trim()
    : "";
  loadContinueLearning();
  await loadActiveList();
  if (!disposed && activeTab.value === "series" && requestedSeriesId) {
    let item = seriesItems.value.find((series) => series.id === requestedSeriesId);
    if (!item) {
      item = normalizeClassroomSeries({ id: requestedSeriesId });
      seriesItems.value = [item, ...seriesItems.value];
    }
    await openSeries(item);
  }
});

onShow(() => {
  if (skipNextShowRefresh) {
    skipNextShowRefresh = false;
    return;
  }
  loadContinueLearning();
});

onUnload(() => {
  disposed = true;
  listTicket += 1;
  seriesTicket += 1;
  continueTicket += 1;
  seriesPurchaseTicket += 1;
  seriesPurchaseOperation = null;
  seriesPurchaseInFlight.value = false;
  seriesPurchaseController?.stop();
  seriesPurchaseController = null;
});
</script>

<template>
  <view class="wrap classroom page-stack ios-page ios-safe-bottom">
    <view class="classroom-hero nx-page-hero">
      <text class="classroom-hero__eyebrow">老师课堂</text>
      <text class="classroom-hero__title">视频与音频课件</text>
      <text class="classroom-hero__lead">按自己的节奏学习，也可以跟随系列课程持续进阶。</text>
      <view class="classroom-hero__meta" aria-hidden="true">
        <text>独立课件</text>
        <text>系列课程</text>
      </view>
    </view>

    <view
      v-if="continueLoading"
      class="continue-learning continue-learning--loading ios-card"
      aria-live="polite"
    >
      正在读取学习进度…
    </view>
    <view
      v-else-if="continueError"
      class="continue-learning continue-learning--error ios-card"
      aria-live="polite"
    >
      <text>{{ continueError }}</text>
      <button class="state-action" @click="loadContinueLearning">重试学习记录</button>
    </view>
    <button
      v-else-if="continueItem"
      class="continue-learning ios-card"
      :aria-label="`继续学习${continueItem.title}，${continueLabel(continueItem)}`"
      @click="openContinueLearning(continueItem)"
    >
      <view class="continue-learning__head">
        <view class="continue-learning__info">
          <text class="continue-learning__eyebrow">继续学习</text>
          <text class="continue-learning__title">{{ continueItem.title }}</text>
        </view>
        <text class="continue-learning__action">继续 ›</text>
      </view>
      <view
        class="continue-learning__progress"
        role="progressbar"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-valuenow="continuePercent(continueItem)"
        :aria-label="
          continueItem.completed ? '课程已完成' : `课程进度 ${continuePercent(continueItem)}%`
        "
      >
        <view
          class="continue-learning__progress-fill"
          :style="{ width: `${continuePercent(continueItem)}%` }"
        />
      </view>
      <text class="continue-learning__copy">{{ continueLabel(continueItem) }}</text>
    </button>

    <view class="classroom-tabs" role="tablist" aria-label="课堂内容分类">
      <button
        class="classroom-tab"
        :class="{ 'classroom-tab--active': activeTab === 'standalone' }"
        role="tab"
        :aria-selected="activeTab === 'standalone'"
        @click="selectTab('standalone')"
      >
        独立课件
      </button>
      <button
        class="classroom-tab"
        :class="{ 'classroom-tab--active': activeTab === 'series' }"
        role="tab"
        :aria-selected="activeTab === 'series'"
        @click="selectTab('series')"
      >
        系列课程
      </button>
    </view>

    <NxAsyncState
      v-if="loading"
      state="loading"
      title="课堂内容加载中"
      description="正在整理最新的视频与音频课件。"
    />
    <NxAsyncState
      v-else-if="loadError"
      state="error"
      title="课堂内容暂未加载"
      :description="loadError"
      action-text="重新加载"
      @action="retryActiveList"
    />
    <NxAsyncState
      v-else-if="activeItems.length === 0"
      state="empty"
      :title="emptyCopy"
      :description="emptyDescription"
      :action-text="activeTab === 'series' ? '查看独立课件' : ''"
      @action="selectTab('standalone')"
    />

    <view v-else class="classroom-list">
      <view v-for="item in activeItems" :key="item.id" class="classroom-list__item">
        <view
          class="classroom-card ios-card"
          :class="{
            'classroom-card--selected': activeTab === 'series' && selectedSeries?.id === item.id,
            'classroom-card--loading':
              activeTab === 'series' && selectedSeries?.id === item.id && seriesLoading,
          }"
        >
          <view class="classroom-card__media">
            <view class="classroom-card__cover-shell" :class="classroomCoverRatioClass(item)">
              <image
                v-if="item.coverUrl && !coverImageErrors[coverMediaKey(item)]"
                class="classroom-card__cover"
                :class="classroomCoverRatioClass(item)"
                :src="item.coverUrl"
                mode="aspectFill"
                lazy-load
                @error="markCoverImageError(coverMediaKey(item))"
              />
              <view
                v-else
                class="classroom-card__cover classroom-card__cover--fallback"
                :class="classroomCoverRatioClass(item)"
                aria-hidden="true"
                >课</view
              >
              <view class="classroom-card__cover-overlay">
                <view class="classroom-card__overlay-tags">
                  <text class="classroom-card__kind">{{
                    activeTab === "series" ? "系列" : item.contentType === "audio" ? "音频" : "视频"
                  }}</text>
                </view>
                <view class="classroom-card__play" aria-hidden="true">
                  <text class="classroom-card__play-icon">{{
                    activeTab === "series" && selectedSeries?.id === item.id ? "⌃" : "▶"
                  }}</text>
                </view>
              </view>
            </view>
          </view>
          <view class="classroom-card__body">
            <view class="classroom-card__eyebrow">
              <text class="classroom-card__access">{{
                classroomAccessLabel(item.effectiveAccess)
              }}</text>
              <text v-if="formatDuration(item.durationSeconds)" class="classroom-card__duration">{{
                formatDuration(item.durationSeconds)
              }}</text>
            </view>
            <text class="classroom-card__title">{{ item.title || "未命名课件" }}</text>
            <text v-if="item.summary || item.description" class="classroom-card__summary">{{
              item.summary || item.description
            }}</text>
            <view class="classroom-card__footer">
              <text class="classroom-card__teacher">{{ item.teacherName || "九型老师" }}</text>
              <button
                v-if="activeTab === 'series' && itemAction(item).type === 'purchase'"
                class="series-buy"
                :disabled="seriesPaymentBusy"
                @click.stop="startSeriesPurchase(item)"
              >
                {{
                  seriesPaymentTargetId === item.id && seriesPaymentState === "creating"
                    ? "创建订单中…"
                    : itemAction(item).label
                }}
              </button>
              <button
                v-else
                class="classroom-card__action"
                @click="activeTab === 'series' ? openSeries(item) : openContent(item)"
              >
                {{
                  activeTab === "series"
                    ? selectedSeries?.id === item.id
                      ? "收起课件"
                      : "查看课件"
                    : itemAction(item).label
                }}
              </button>
            </view>
          </view>
        </view>

        <view
          v-if="
            activeTab === 'series' &&
            seriesPaymentTargetId === item.id &&
            seriesPaymentState !== 'idle' &&
            seriesPaymentState !== 'success'
          "
          class="series-payment ios-card"
          aria-live="polite"
        >
          <text>{{ seriesPaymentMessage }}</text>
          <view
            v-if="seriesPaymentState === 'failure' || seriesPaymentState === 'cancelled'"
            class="series-payment__actions"
          >
            <button class="state-action" @click="retrySeriesPurchase(item)">重新支付</button>
            <button class="state-action" @click="cancelSeriesPurchase">暂不购买</button>
          </view>
        </view>

        <view
          v-if="activeTab === 'series' && selectedSeries?.id === item.id"
          class="series-panel ios-card"
        >
          <NxAsyncState
            v-if="seriesLoading"
            state="loading"
            title="系列课件加载中"
            description="正在整理本系列的章节。"
          />
          <NxAsyncState
            v-else-if="seriesError"
            state="error"
            title="系列课件暂未加载"
            :description="seriesError"
            action-text="重新加载"
            @action="retrySelectedSeries"
          />
          <block v-else-if="expandedSeries">
            <text class="series-panel__eyebrow">系列课件</text>
            <text class="series-panel__title">{{ expandedSeries.series.title }}</text>
            <NxAsyncState
              v-if="expandedSeries.contents.length === 0"
              state="empty"
              title="这个系列正在补充课件"
              description="可以先返回独立课件，选择一节视频或音频开始学习。"
            />
            <view v-else class="series-panel__chapters">
              <button
                v-for="(lesson, index) in expandedSeries.contents"
                :key="lesson.id"
                class="lesson-row"
                @click="openContent(lesson)"
              >
                <text class="lesson-row__index">{{ index + 1 }}</text>
                <view class="lesson-row__body">
                  <text class="lesson-row__title">{{ lesson.title }}</text>
                  <text class="lesson-row__meta"
                    >{{ lesson.contentType === "audio" ? "音频" : "视频" }} ·
                    {{ itemAction(lesson).label }}</text
                  >
                </view>
                <text class="lesson-row__arrow" aria-hidden="true">›</text>
              </button>
            </view>
          </block>
        </view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.classroom {
  width: 100%;
  max-width: 900rpx;
  margin: 0 auto;
  box-sizing: border-box;
  gap: 32rpx;
  min-height: 100vh;
  background:
    radial-gradient(circle at 0 0, rgba(223, 188, 127, 0.16), transparent 30%),
    linear-gradient(180deg, var(--nx-surface-soft), var(--nx-page-bg));
}
.classroom-hero {
  padding: 28rpx 28rpx 26rpx;
  color: var(--nx-surface);
  background:
    radial-gradient(circle at 88% 12%, rgba(223, 188, 127, 0.32), transparent 28%),
    linear-gradient(135deg, var(--nx-brand-900), var(--nx-brand-700));
  border-radius: 38rpx;
  box-shadow: 0 24rpx 54rpx -34rpx rgba(32, 42, 55, 0.64);
}
.classroom-hero__eyebrow,
.classroom-hero__title,
.classroom-hero__lead { display: block; }
.classroom-hero__eyebrow { color: var(--nx-accent-gold); font-size: 22rpx; font-weight: 900; letter-spacing: 3rpx; }
.classroom-hero__title { margin-top: 10rpx; color: var(--nx-surface); font-size: 36rpx; font-weight: 900; line-height: 1.28; }
.classroom-hero__lead { margin-top: 10rpx; color: rgba(255, 255, 255, 0.82); font-size: 24rpx; line-height: 1.55; }
.classroom-hero__meta { display: flex; flex-wrap: wrap; gap: 8rpx; margin-top: 16rpx; }
.classroom-hero__meta text { padding: 6rpx 14rpx; color: var(--nx-surface); font-size: 20rpx; font-weight: 800; background: rgba(255, 255, 255, 0.12); border: 2rpx solid rgba(255, 255, 255, 0.18); border-radius: 999rpx; }
.classroom-tabs { display: flex; gap: 8rpx; padding: 6rpx; background: var(--nx-surface-soft); border: 2rpx solid var(--nx-border); border-radius: 20rpx; }
.classroom-tab { flex: 1; min-height: 88rpx; color: var(--nx-text-muted); font-size: 27rpx; font-weight: 900; line-height: 88rpx; background: transparent; border-radius: 18rpx; }
.classroom-tab::after,
.state-action::after,
.lesson-row::after,
.series-buy::after,
.classroom-card__action::after,
.continue-learning::after { border: 0; }
.classroom-tab--active { color: var(--nx-brand-900); background: var(--nx-surface); box-shadow: 0 0 0 2rpx var(--nx-accent-gold), 0 8rpx 18rpx rgba(32, 42, 55, 0.16); }
.continue-learning { display: block; width: 100%; min-height: 152rpx; padding: 20rpx; color: var(--nx-text); text-align: left; background: linear-gradient(135deg, var(--nx-surface), var(--nx-surface-soft)); border-left: 6rpx solid var(--nx-accent-gold); border-radius: 24rpx; box-sizing: border-box; }
.continue-learning--loading,
.continue-learning--error { color: var(--nx-text-muted); font-size: 25rpx; text-align: center; }
.continue-learning--error { color: #a23b32; }
.continue-learning__head { display: flex; align-items: center; justify-content: space-between; gap: 16rpx; }
.continue-learning__info { display: flex; align-items: center; flex: 1; min-width: 0; gap: 12rpx; }
.continue-learning__eyebrow,
.continue-learning__title,
.continue-learning__copy { display: block; }
.continue-learning__eyebrow,
.continue-learning__action { color: var(--nx-brand-700); font-size: 23rpx; font-weight: 900; }
.continue-learning__eyebrow,
.continue-learning__action { flex-shrink: 0; }
.continue-learning__title { flex: 1; min-width: 0; margin-top: 0; overflow: hidden; color: var(--nx-text); font-size: 28rpx; font-weight: 900; line-height: 1.35; text-overflow: ellipsis; white-space: nowrap; }
.continue-learning__progress { height: 10rpx; margin-top: 14rpx; overflow: hidden; background: var(--nx-border); border-radius: 999rpx; }
.continue-learning__progress-fill { height: 100%; background: var(--nx-brand-700); border-radius: inherit; }
.continue-learning__copy { margin-top: 8rpx; color: var(--nx-text-muted); font-size: 22rpx; }
.state-action { min-height: 88rpx; margin-top: 20rpx; padding: 0 32rpx; color: var(--nx-brand-900); font-weight: 900; line-height: 88rpx; background: var(--nx-surface); border: 2rpx solid var(--nx-border); border-radius: 18rpx; }
.classroom-list { display: grid; gap: 22rpx; }
.classroom-list__item { display: grid; gap: 10rpx; }
.classroom-card { display: flex; align-items: flex-start; flex-direction: column; min-height: 220rpx; padding: 0; overflow: hidden; background: var(--nx-surface); border: 2rpx solid var(--nx-border); border-radius: 30rpx; }
.classroom-card--selected { box-shadow: 0 0 0 4rpx var(--nx-accent-gold), 0 16rpx 32rpx rgba(32, 42, 55, 0.14); }
.classroom-card--loading { opacity: 0.82; }
.classroom-card__media { width: 100%; }
.classroom-card__cover-shell { position: relative; width: 100%; overflow: hidden; background: linear-gradient(135deg, var(--nx-surface-soft), var(--nx-accent-gold)); }
.classroom-card__cover-shell::after { content: ""; position: absolute; inset: auto 0 0; height: 24%; background: linear-gradient(180deg, rgba(32, 42, 55, 0), rgba(32, 42, 55, 0.24)); pointer-events: none; }
.classroom-card__cover { display: block; width: 100%; height: 100%; background: var(--nx-border); }
.classroom-card__cover.classroom-cover--16x9 { height: 300rpx; }
.classroom-card__cover.classroom-cover--9x16 { height: 360rpx; }
.classroom-card__cover.classroom-cover--1x1 { height: 320rpx; }
.classroom-card__cover--fallback { display: flex; align-items: center; justify-content: center; color: var(--nx-brand-900); font-size: 58rpx; font-weight: 900; }
.classroom-card__cover-overlay { position: absolute; inset: 0; z-index: 1; padding: 16rpx; color: var(--nx-surface); background: linear-gradient(180deg, rgba(32, 42, 55, 0.04), rgba(32, 42, 55, 0.34)); }
.classroom-card__overlay-tags { display: flex; flex-wrap: wrap; gap: 8rpx; }
.classroom-card__overlay-tags .classroom-card__kind { color: var(--nx-surface); }
.classroom-card__kind { padding: 0 12rpx; line-height: 36rpx; background: rgba(32, 42, 55, 0.58); border: 1rpx solid rgba(255, 255, 255, 0.32); border-radius: 999rpx; }
.classroom-card__play { position: absolute; top: 50%; left: 50%; display: inline-flex; align-items: center; justify-content: center; width: 52rpx; height: 52rpx; color: var(--nx-brand-900); font-weight: 900; background: rgba(255, 255, 255, 0.9); border-radius: 50%; transform: translate(-50%, -50%); }
.classroom-card__play-icon { font-size: 22rpx; }
.classroom-card__body { display: flex; flex-direction: column; min-width: 0; width: 100%; padding: 20rpx 22rpx 22rpx; box-sizing: border-box; }
.classroom-card__eyebrow { display: flex; align-items: center; flex-wrap: wrap; gap: 10rpx; color: var(--nx-text-muted); font-size: 22rpx; font-weight: 800; }
.classroom-card__access { padding: 4rpx 12rpx; color: var(--nx-brand-900); line-height: 1.4; background: var(--nx-surface-soft); border: 1rpx solid var(--nx-border); border-radius: 999rpx; }
.classroom-card__duration { color: var(--nx-text-muted); }
.classroom-card__footer { display: flex; align-items: flex-end; justify-content: space-between; gap: 12rpx; margin-top: 14rpx; padding-top: 14rpx; border-top: 2rpx solid var(--nx-border); }
.classroom-card__teacher { flex: 1; min-width: 0; color: var(--nx-text-muted); font-size: 22rpx; line-height: 1.45; }
.classroom-card__title { margin-top: 10rpx; color: var(--nx-text); font-size: 30rpx; font-weight: 900; line-height: 1.4; }
.classroom-card__summary { margin-top: 6rpx; color: var(--nx-text-muted); font-size: 23rpx; line-height: 1.5; }
.classroom-card__action { flex-shrink: 0; min-height: 88rpx; padding: 0 24rpx; color: var(--nx-surface); font-size: 23rpx; font-weight: 900; line-height: 88rpx; background: var(--nx-brand-700); border-radius: 999rpx; }
.series-buy { flex-shrink: 0; min-height: 88rpx; padding: 0 24rpx; color: var(--nx-brand-900); font-size: 23rpx; font-weight: 900; line-height: 88rpx; background: var(--nx-accent-gold); border-radius: 999rpx; }
.series-payment { padding: 24rpx; color: var(--nx-text-muted); font-size: 24rpx; background: var(--nx-surface); border: 2rpx solid var(--nx-border); border-radius: 30rpx; }
.series-payment__actions { display: flex; gap: 12rpx; }
.series-panel { padding: 30rpx; background: var(--nx-surface); border: 2rpx solid var(--nx-border); border-radius: 30rpx; }
.series-panel__chapters { display: grid; gap: 14rpx; margin-top: 16rpx; }
.series-panel__eyebrow,
.series-panel__title { display: block; }
.series-panel__eyebrow { color: var(--nx-brand-700); font-size: 23rpx; font-weight: 900; }
.series-panel__title { margin-top: 8rpx; color: var(--nx-text); font-size: 34rpx; font-weight: 900; }
.lesson-row { display: flex; align-items: center; width: 100%; min-height: 104rpx; padding: 16rpx 10rpx; text-align: left; background: var(--nx-surface-soft); border: 2rpx solid var(--nx-border); border-radius: 18rpx; box-sizing: border-box; }
.lesson-row__index { display: flex; align-items: center; justify-content: center; width: 52rpx; height: 52rpx; color: var(--nx-brand-900); background: var(--nx-accent-gold); border-radius: 50%; }
.lesson-row__body { flex: 1; min-width: 0; margin: 0 18rpx; }
.lesson-row__title,
.lesson-row__meta { display: block; }
.lesson-row__title { color: var(--nx-text); font-size: 26rpx; font-weight: 800; }
.lesson-row__meta { margin-top: 6rpx; color: var(--nx-text-muted); font-size: 22rpx; }
.lesson-row__arrow { color: var(--nx-text-muted); font-size: 32rpx; font-weight: 700; }

@media (max-width: 380px) {
  .classroom-card__footer { flex-direction: column; align-items: stretch; }
  .classroom-card__teacher { width: 100%; }
  .classroom-card__action,
  .series-buy { width: 100%; }
}
</style>
