<script setup>
import { computed, ref } from "vue";
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
  classroomContentRoute,
  classroomPurchaseAction,
  normalizeClassroomContent,
  normalizeClassroomSeries,
} from "../../utils/classroomDisplay";
import { createClassroomPurchaseController } from "../../utils/classroomProgress";
import { getToken } from "../../utils/auth";
import { userErrorMessage } from "../../utils/userMessage";

const activeTab = ref("series");
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
let listTicket = 0;
let seriesTicket = 0;
let continueTicket = 0;
let skipNextShowRefresh = false;
let seriesPurchaseController = null;

const activeItems = computed(() =>
  activeTab.value === "series" ? seriesItems.value : standaloneItems.value,
);
const emptyCopy = computed(() =>
  activeTab.value === "series" ? "系列课程正在准备中" : "独立课件正在准备中",
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
  seriesPaymentTargetId.value = item.id;
  seriesPurchaseController = createClassroomPurchaseController({
    create: () => createClassroomOrderApi("series", item.id),
    pay: async (order) => {
      if (order?.payParams?.devMode) return devPayClassroomOrderApi(order.outTradeNo);
      return requestSeriesPayment(order?.payParams);
    },
    status: () => getClassroomOrderStatusApi("series", item.id),
    onChange: (snapshot) => {
      seriesPaymentState.value = snapshot.state;
      seriesPaymentMessage.value = snapshot.message;
    },
    onSuccess: async () => {
      loadedTabs.value = { ...loadedTabs.value, series: false };
      await loadActiveList({ force: true });
      if (selectedSeries.value?.id === item.id) await openSeries(item, { force: true });
    },
  });
  return seriesPurchaseController;
}

function startSeriesPurchase(item) {
  if (!item?.id || itemAction(item).type !== "purchase") return;
  if (!getToken()) {
    uni.switchTab({ url: "/pages/profile/profile" });
    return;
  }
  if (
    seriesPaymentTargetId.value === item.id &&
    (seriesPaymentState.value === "creating" || seriesPaymentState.value === "pending")
  )
    return;
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
  skipNextShowRefresh = true;
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
  seriesPurchaseController?.stop();
  seriesPurchaseController = null;
});
</script>

<template>
  <view class="classroom page-stack ios-page ios-safe-bottom">
    <view class="classroom-hero nx-page-hero">
      <text class="classroom-hero__eyebrow">老师课堂</text>
      <text class="classroom-hero__title">用声音与影像，陪你把觉察带进生活</text>
      <text class="classroom-hero__lead">既可以按系列循序学习，也可以从一节独立课件开始。</text>
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
        :class="{ 'classroom-tab--active': activeTab === 'series' }"
        role="tab"
        :aria-selected="activeTab === 'series'"
        @click="selectTab('series')"
      >
        系列课程
      </button>
      <button
        class="classroom-tab"
        :class="{ 'classroom-tab--active': activeTab === 'standalone' }"
        role="tab"
        :aria-selected="activeTab === 'standalone'"
        @click="selectTab('standalone')"
      >
        独立课件
      </button>
    </view>

    <view v-if="loading" class="classroom-state" aria-live="polite">课堂内容加载中…</view>
    <view v-else-if="loadError" class="classroom-state classroom-state--error" aria-live="polite">
      <text>{{ loadError }}</text>
      <button class="state-action" :disabled="loading" @click="retryActiveList">重新加载</button>
    </view>
    <view v-else-if="activeItems.length === 0" class="classroom-state" aria-live="polite">{{
      emptyCopy
    }}</view>

    <view v-else class="classroom-list">
      <view v-for="item in activeItems" :key="item.id" class="classroom-list__item">
        <view
          class="classroom-card ios-card"
          :class="{
            'classroom-card--selected': activeTab === 'series' && selectedSeries?.id === item.id,
            'classroom-card--loading':
              activeTab === 'series' && selectedSeries?.id === item.id && seriesLoading,
          }"
          role="button"
          aria-role="button"
          tabindex="0"
          @click="activeTab === 'series' ? openSeries(item) : openContent(item)"
          @keydown.enter="activeTab === 'series' ? openSeries(item) : openContent(item)"
          @keydown.space.prevent="activeTab === 'series' ? openSeries(item) : openContent(item)"
        >
          <image
            v-if="item.coverUrl"
            class="classroom-card__cover"
            :src="item.coverUrl"
            mode="aspectFill"
            lazy-load
          />
          <view
            v-else
            class="classroom-card__cover classroom-card__cover--fallback"
            aria-hidden="true"
            >课</view
          >
          <view class="classroom-card__body">
            <view class="classroom-card__meta">
              <text class="nx-tag">{{ classroomAccessLabel(item.effectiveAccess) }}</text>
              <text v-if="item.contentType" class="classroom-card__kind">{{
                item.contentType === "audio" ? "音频" : "视频"
              }}</text>
            </view>
            <text class="classroom-card__title">{{ item.title || "未命名课件" }}</text>
            <text v-if="item.summary || item.description" class="classroom-card__summary">{{
              item.summary || item.description
            }}</text>
            <view class="classroom-card__footer">
              <text>{{ item.teacherName || "九型老师" }}</text>
              <text v-if="formatDuration(item.durationSeconds)">{{
                formatDuration(item.durationSeconds)
              }}</text>
              <button
                v-if="activeTab === 'series' && itemAction(item).type === 'purchase'"
                class="series-buy"
                :disabled="
                  seriesPaymentTargetId === item.id &&
                  (seriesPaymentState === 'creating' || seriesPaymentState === 'pending')
                "
                @click.stop="startSeriesPurchase(item)"
              >
                {{
                  seriesPaymentTargetId === item.id && seriesPaymentState === "creating"
                    ? "创建订单中…"
                    : itemAction(item).label
                }}
              </button>
              <text v-else class="classroom-card__action">{{
                activeTab === "series"
                  ? selectedSeries?.id === item.id
                    ? "收起课件"
                    : "查看课件"
                  : itemAction(item).label
              }}</text>
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
          <view v-if="seriesLoading" class="classroom-state" aria-live="polite"
            >系列课件加载中…</view
          >
          <view
            v-else-if="seriesError"
            class="classroom-state classroom-state--error"
            aria-live="polite"
          >
            <text>{{ seriesError }}</text>
            <button class="state-action" @click="retrySelectedSeries">重试</button>
          </view>
          <block v-else-if="expandedSeries">
            <text class="series-panel__eyebrow">系列课件</text>
            <text class="series-panel__title">{{ expandedSeries.series.title }}</text>
            <view v-if="expandedSeries.contents.length === 0" class="classroom-state"
              >这个系列暂时没有可学习的课件</view
            >
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
              <text aria-hidden="true">›</text>
            </button>
          </block>
        </view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.classroom {
  min-height: 100vh;
  background: #f4f8f6;
}
.classroom-hero {
  padding: 38rpx 34rpx 40rpx;
  border-radius: 38rpx;
  color: #fff;
  background: linear-gradient(135deg, #0f766e, #15803d);
}
.classroom-hero__eyebrow,
.classroom-hero__title,
.classroom-hero__lead {
  display: block;
  color: #fff;
}
.classroom-hero__eyebrow {
  font-size: 24rpx;
  font-weight: 800;
}
.classroom-hero__title {
  margin-top: 14rpx;
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.3;
}
.classroom-hero__lead {
  margin-top: 16rpx;
  font-size: 26rpx;
  line-height: 1.65;
}
.classroom-tabs {
  display: flex;
  gap: 12rpx;
  padding: 8rpx;
  background: #e7f3ee;
  border-radius: 24rpx;
}
.continue-learning {
  display: block;
  width: 100%;
  min-height: 176rpx;
  padding: 28rpx;
  color: #173e32;
  text-align: left;
  background: linear-gradient(135deg, #ecfdf5, #eff6ff);
  border-radius: 28rpx;
}
.continue-learning::after {
  border: 0;
}
.continue-learning--loading,
.continue-learning--error {
  color: #64756e;
  font-size: 25rpx;
  text-align: center;
}
.continue-learning--error {
  color: #9f3a38;
}
.continue-learning__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20rpx;
}
.continue-learning__eyebrow,
.continue-learning__title,
.continue-learning__copy {
  display: block;
}
.continue-learning__eyebrow,
.continue-learning__action {
  color: #0f766e;
  font-size: 23rpx;
  font-weight: 800;
}
.continue-learning__title {
  margin-top: 6rpx;
  color: #173e32;
  font-size: 30rpx;
  font-weight: 900;
  line-height: 1.4;
}
.continue-learning__progress {
  height: 12rpx;
  margin-top: 22rpx;
  overflow: hidden;
  background: #cfe7dc;
  border-radius: 999rpx;
}
.continue-learning__progress-fill {
  height: 100%;
  background: #0f766e;
  border-radius: inherit;
}
.continue-learning__copy {
  margin-top: 12rpx;
  color: #587167;
  font-size: 23rpx;
}
.classroom-tab {
  flex: 1;
  min-height: 88rpx;
  color: #527066;
  font-size: 27rpx;
  font-weight: 800;
  line-height: 88rpx;
  background: transparent;
  border-radius: 18rpx;
}
.classroom-tab::after,
.state-action::after,
.lesson-row::after,
.series-buy::after {
  border: 0;
}
.series-buy {
  min-height: 64rpx;
  padding: 0 18rpx;
  color: #fff;
  font-size: 22rpx;
  font-weight: 800;
  line-height: 64rpx;
  background: #0f766e;
  border-radius: 16rpx;
}
.series-payment {
  padding: 24rpx;
  color: #52685f;
  font-size: 24rpx;
  background: #fff;
  border-radius: 24rpx;
}
.series-payment__actions {
  display: flex;
  gap: 12rpx;
}
.classroom-tab--active {
  color: #0f6b4f;
  background: #fff;
  box-shadow: 0 10rpx 26rpx rgba(15, 107, 79, 0.12);
}
.classroom-state {
  padding: 48rpx 30rpx;
  color: #64756e;
  font-size: 27rpx;
  line-height: 1.6;
  text-align: center;
  background: #fff;
  border-radius: 28rpx;
}
.classroom-state--error {
  color: #9f3a38;
}
.state-action {
  min-height: 88rpx;
  margin-top: 20rpx;
  padding: 0 32rpx;
  color: #0f6b4f;
  font-weight: 800;
  line-height: 88rpx;
  background: #ecfdf5;
  border-radius: 18rpx;
}
.classroom-list {
  display: grid;
  gap: 22rpx;
}
.classroom-list__item {
  display: grid;
  gap: 14rpx;
}
.classroom-card {
  display: flex;
  min-height: 220rpx;
  overflow: hidden;
  background: #fff;
  border-radius: 30rpx;
}
.classroom-card--selected {
  box-shadow:
    0 0 0 4rpx #34d399,
    0 16rpx 32rpx rgba(15, 118, 110, 0.12);
}
.classroom-card--loading {
  opacity: 0.82;
}
.classroom-card:focus {
  outline: 4rpx solid #2b7fff;
}
.classroom-card__cover {
  flex: 0 0 210rpx;
  width: 210rpx;
  min-height: 220rpx;
  background: #dbeee6;
}
.classroom-card__cover--fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #0f766e;
  font-size: 54rpx;
  font-weight: 900;
}
.classroom-card__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
  padding: 24rpx;
}
.classroom-card__meta,
.classroom-card__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12rpx;
}
.classroom-card__kind,
.classroom-card__footer {
  color: #708078;
  font-size: 22rpx;
}
.classroom-card__title {
  margin-top: 14rpx;
  color: #16221d;
  font-size: 30rpx;
  font-weight: 900;
  line-height: 1.4;
}
.classroom-card__summary {
  margin-top: 8rpx;
  color: #68776f;
  font-size: 23rpx;
  line-height: 1.5;
}
.classroom-card__footer {
  margin-top: auto;
  padding-top: 18rpx;
}
.classroom-card__action {
  color: #0f766e;
  font-weight: 800;
}
.series-panel {
  padding: 30rpx;
  background: #fff;
  border-radius: 30rpx;
}
.series-panel__eyebrow,
.series-panel__title {
  display: block;
}
.series-panel__eyebrow {
  color: #0f766e;
  font-size: 23rpx;
  font-weight: 800;
}
.series-panel__title {
  margin-top: 8rpx;
  color: #17241e;
  font-size: 34rpx;
  font-weight: 900;
}
.lesson-row {
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 104rpx;
  margin-top: 18rpx;
  padding: 16rpx 10rpx;
  text-align: left;
  background: #f5faf7;
  border-radius: 18rpx;
}
.lesson-row__index {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 52rpx;
  height: 52rpx;
  color: #fff;
  background: #0f766e;
  border-radius: 50%;
}
.lesson-row__body {
  flex: 1;
  min-width: 0;
  margin: 0 18rpx;
}
.lesson-row__title,
.lesson-row__meta {
  display: block;
}
.lesson-row__title {
  color: #1b2822;
  font-size: 26rpx;
  font-weight: 800;
}
.lesson-row__meta {
  margin-top: 6rpx;
  color: #718078;
  font-size: 22rpx;
}
</style>
