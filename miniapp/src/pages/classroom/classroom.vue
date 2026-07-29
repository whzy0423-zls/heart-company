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
const coverImageErrors = ref({});
let listTicket = 0;
let seriesTicket = 0;
let continueTicket = 0;
let skipNextShowRefresh = false;
let seriesPurchaseController = null;
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
const seriesPaymentBusy = computed(
  () => seriesPaymentState.value === "creating" || seriesPaymentState.value === "pending",
);

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

function startSeriesPurchase(item) {
  if (!item?.id || itemAction(item).type !== "purchase") return;
  if (seriesPaymentBusy.value) return;
  if (!getToken()) {
    uni.switchTab({ url: "/pages/profile/profile" });
    return;
  }
  return createSeriesPurchase(item).purchase();
}

function retrySeriesPurchase(item) {
  if (seriesPaymentTargetId.value !== item?.id) return startSeriesPurchase(item);
  return seriesPurchaseController?.retry();
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

onLoad((options = {}) => {
  disposed = false;
  skipNextShowRefresh = true;
  if (options.tab === "series") activeTab.value = "series";
  if (options.tab === "standalone") activeTab.value = "standalone";
  loadActiveList();
  loadContinueLearning();
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
  seriesPurchaseController?.stop();
  seriesPurchaseController = null;
});
</script>

<template>
  <view class="classroom page-stack ios-page ios-safe-bottom">
    <view class="classroom-hero nx-page-hero">
      <text class="classroom-hero__eyebrow">老师课堂</text>
      <text class="classroom-hero__title">用声音与影像，陪你把觉察带进工作与生活</text>
      <text class="classroom-hero__lead">独立课件先行，系列课程随后；视频和音频都可以按自己的节奏反复学习。</text>
      <view class="classroom-hero__meta" aria-hidden="true">
        <text>视频课件</text>
        <text>音频精讲</text>
        <text>按需学习</text>
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
        <view>
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
                  <text class="nx-tag">{{ classroomAccessLabel(item.effectiveAccess) }}</text>
                  <text class="classroom-card__kind">{{
                    activeTab === "series" ? "系列" : item.contentType === "audio" ? "音频" : "视频"
                  }}</text>
                </view>
                <view class="classroom-card__play" aria-hidden="true">
                  <text class="classroom-card__play-icon">{{
                    activeTab === "series" && selectedSeries?.id === item.id ? "⌃" : "▶"
                  }}</text>
                  <text class="classroom-card__play-text">{{
                    activeTab === "series"
                      ? selectedSeries?.id === item.id
                        ? "收起"
                        : "展开"
                      : itemAction(item).label
                  }}</text>
                </view>
              </view>
            </view>
          </view>
          <view class="classroom-card__body">
            <view class="classroom-card__meta">
              <text>{{ activeTab === "series" ? "系列沉淀" : "独立学习" }}</text>
              <text>{{ classroomAccessLabel(item.effectiveAccess) }}</text>
            </view>
            <text class="classroom-card__title">{{ item.title || "未命名课件" }}</text>
            <text v-if="item.summary || item.description" class="classroom-card__summary">{{
              item.summary || item.description
            }}</text>
            <view class="classroom-card__footer">
              <view class="classroom-card__facts">
                <text>{{ item.teacherName || "九型老师" }}</text>
                <text v-if="formatDuration(item.durationSeconds)">{{
                  formatDuration(item.durationSeconds)
                }}</text>
              </view>
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
  min-height: 100vh;
  background:
    radial-gradient(circle at 0 0, rgba(223, 188, 127, 0.16), transparent 30%),
    linear-gradient(180deg, var(--nx-surface-soft), var(--nx-page-bg));
}
.classroom-hero {
  padding: 40rpx 34rpx 36rpx;
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
.classroom-hero__eyebrow { color: var(--nx-accent-gold); font-size: 24rpx; font-weight: 900; letter-spacing: 4rpx; }
.classroom-hero__title { margin-top: 14rpx; color: var(--nx-surface); font-size: 42rpx; font-weight: 900; line-height: 1.3; }
.classroom-hero__lead { margin-top: 16rpx; color: rgba(255, 255, 255, 0.82); font-size: 25rpx; line-height: 1.65; }
.classroom-hero__meta { display: flex; flex-wrap: wrap; gap: 12rpx; margin-top: 24rpx; }
.classroom-hero__meta text { padding: 8rpx 16rpx; color: var(--nx-surface); font-size: 21rpx; font-weight: 800; background: rgba(255, 255, 255, 0.12); border: 2rpx solid rgba(255, 255, 255, 0.18); border-radius: 999rpx; }
.classroom-tabs { display: flex; gap: 12rpx; padding: 8rpx; background: var(--nx-surface-soft); border: 2rpx solid var(--nx-border); border-radius: 24rpx; }
.classroom-tab { flex: 1; min-height: 88rpx; color: var(--nx-text-muted); font-size: 27rpx; font-weight: 900; line-height: 88rpx; background: transparent; border-radius: 18rpx; }
.classroom-tab::after,
.state-action::after,
.lesson-row::after,
.series-buy::after,
.classroom-card__action::after,
.continue-learning::after { border: 0; }
.classroom-tab--active { color: var(--nx-brand-900); background: var(--nx-surface); box-shadow: 0 10rpx 26rpx rgba(32, 42, 55, 0.12); }
.continue-learning { display: block; width: 100%; min-height: 176rpx; padding: 28rpx; color: var(--nx-text); text-align: left; background: linear-gradient(135deg, var(--nx-surface-soft), var(--nx-accent-gold)); border-radius: 28rpx; box-sizing: border-box; }
.continue-learning--loading,
.continue-learning--error { color: var(--nx-text-muted); font-size: 25rpx; text-align: center; }
.continue-learning--error { color: #a23b32; }
.continue-learning__head { display: flex; align-items: flex-start; justify-content: space-between; gap: 20rpx; }
.continue-learning__eyebrow,
.continue-learning__title,
.continue-learning__copy { display: block; }
.continue-learning__eyebrow,
.continue-learning__action { color: var(--nx-brand-700); font-size: 23rpx; font-weight: 900; }
.continue-learning__title { margin-top: 6rpx; color: var(--nx-text); font-size: 30rpx; font-weight: 900; line-height: 1.4; }
.continue-learning__progress { height: 12rpx; margin-top: 22rpx; overflow: hidden; background: var(--nx-border); border-radius: 999rpx; }
.continue-learning__progress-fill { height: 100%; background: var(--nx-brand-700); border-radius: inherit; }
.continue-learning__copy { margin-top: 12rpx; color: var(--nx-text-muted); font-size: 23rpx; }
.state-action { min-height: 88rpx; margin-top: 20rpx; padding: 0 32rpx; color: var(--nx-brand-900); font-weight: 900; line-height: 88rpx; background: var(--nx-surface); border: 2rpx solid var(--nx-border); border-radius: 18rpx; }
.classroom-list { display: grid; gap: 22rpx; }
.classroom-list__item { display: grid; gap: 14rpx; }
.classroom-card { display: flex; align-items: flex-start; flex-direction: column; min-height: 220rpx; overflow: hidden; background: var(--nx-surface); border: 2rpx solid var(--nx-border); border-radius: 30rpx; }
.classroom-card--selected { box-shadow: 0 0 0 4rpx var(--nx-accent-gold), 0 16rpx 32rpx rgba(32, 42, 55, 0.14); }
.classroom-card--loading { opacity: 0.82; }
.classroom-card__media { width: 100%; }
.classroom-card__cover-shell { position: relative; width: 100%; overflow: hidden; background: linear-gradient(135deg, var(--nx-surface-soft), var(--nx-accent-gold)); }
.classroom-card__cover-shell::after { content: ""; position: absolute; inset: auto 0 0; height: 30%; background: linear-gradient(180deg, rgba(32, 42, 55, 0), rgba(32, 42, 55, 0.42)); pointer-events: none; }
.classroom-card__cover { display: block; width: 100%; height: 100%; background: var(--nx-border); }
.classroom-card__cover.classroom-cover--16x9 { height: 376rpx; }
.classroom-card__cover.classroom-cover--9x16 { height: 472rpx; }
.classroom-card__cover.classroom-cover--1x1 { height: 360rpx; }
.classroom-card__cover--fallback { display: flex; align-items: center; justify-content: center; color: var(--nx-brand-900); font-size: 58rpx; font-weight: 900; }
.classroom-card__cover-overlay { position: absolute; inset: 0; z-index: 1; display: flex; flex-direction: column; justify-content: space-between; padding: 22rpx; color: var(--nx-surface); background: linear-gradient(180deg, rgba(32, 42, 55, 0.08), rgba(32, 42, 55, 0.58)); }
.classroom-card__overlay-tags { display: flex; flex-wrap: wrap; gap: 10rpx; }
.classroom-card__overlay-tags .nx-tag,
.classroom-card__overlay-tags .classroom-card__kind { color: var(--nx-surface); }
.classroom-card__kind { padding: 0 14rpx; line-height: 42rpx; background: rgba(255, 255, 255, 0.18); border-radius: 999rpx; }
.classroom-card__play { display: inline-flex; align-items: center; align-self: flex-end; gap: 10rpx; min-height: 60rpx; padding: 0 18rpx; color: var(--nx-brand-900); font-size: 22rpx; font-weight: 900; background: rgba(255, 255, 255, 0.94); border-radius: 999rpx; }
.classroom-card__play-icon { font-size: 20rpx; }
.classroom-card__body { display: flex; flex-direction: column; min-width: 0; width: 100%; padding: 24rpx 24rpx 26rpx; box-sizing: border-box; }
.classroom-card__meta,
.classroom-card__footer,
.classroom-card__facts { display: flex; align-items: center; justify-content: space-between; gap: 12rpx; }
.classroom-card__meta,
.classroom-card__facts { color: var(--nx-text-muted); font-size: 22rpx; }
.classroom-card__title { margin-top: 14rpx; color: var(--nx-text); font-size: 30rpx; font-weight: 900; line-height: 1.4; }
.classroom-card__summary { margin-top: 8rpx; color: var(--nx-text-muted); font-size: 23rpx; line-height: 1.5; }
.classroom-card__footer { align-items: flex-end; margin-top: 18rpx; padding-top: 18rpx; border-top: 2rpx solid var(--nx-border); }
.classroom-card__facts { align-items: flex-start; flex-direction: column; min-width: 0; }
.classroom-card__action { flex-shrink: 0; min-height: 88rpx; padding: 0 24rpx; color: var(--nx-surface); font-size: 23rpx; font-weight: 900; line-height: 88rpx; background: var(--nx-brand-700); border-radius: 999rpx; }
.series-buy { flex-shrink: 0; min-height: 88rpx; padding: 0 24rpx; color: var(--nx-brand-900); font-size: 23rpx; font-weight: 900; line-height: 88rpx; background: var(--nx-accent-gold); border-radius: 999rpx; }
.series-payment { padding: 24rpx; color: var(--nx-text-muted); font-size: 24rpx; background: var(--nx-surface); border: 2rpx solid var(--nx-border); border-radius: 24rpx; }
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
</style>
