<script setup>
import { computed, ref } from "vue";
import { onHide, onLoad, onShow, onUnload } from "@dcloudio/uni-app";
import {
  createClassroomOrderApi,
  devPayClassroomOrderApi,
  getClassroomContentApi,
  getClassroomOrderStatusApi,
  getClassroomSeriesApi,
  updateClassroomProgressApi,
  withClassroomPlaybackRetry,
} from "../../api";
import {
  classroomAccessLabel,
  classroomCoverRatioClass,
  classroomPurchaseAction,
  normalizeClassroomContent,
  normalizeClassroomSeries,
} from "../../utils/classroomDisplay";
import {
  classroomCompletion,
  createClassroomProgressTracker,
  createClassroomPurchaseController,
  readAnonymousClassroomProgress,
} from "../../utils/classroomProgress";
import { getToken } from "../../utils/auth";
import { userErrorMessage } from "../../utils/userMessage";

const contentId = ref("");
const content = ref(normalizeClassroomContent());
const loading = ref(true);
const loadError = ref("");
const playbackUrl = ref("");
const playbackLoading = ref(false);
const playbackError = ref("");
const playbackRetryLabel = ref("重试播放");
const audioPlaying = ref(false);
const audioPosition = ref(0);
const audioDuration = ref(0);
const progressPosition = ref(0);
const progressCompleted = ref(false);
const progressSyncError = ref("");
const paymentState = ref("idle");
const paymentMessage = ref("");
const purchaseTarget = ref({ type: "content", id: "", ready: true });
const purchaseOffer = ref(null);
const purchaseTargetError = ref("");
let detailTicket = 0;
let playbackTicket = 0;
let audioContext = null;
let audioBindings = null;
let videoContext = null;
let playbackRecoveryUsed = false;
let disposed = false;
let pageVisible = true;
let progressTracker = null;
let purchaseController = null;
let requestedResumePosition = 0;

const accessAction = computed(() => classroomPurchaseAction(purchaseOffer.value || content.value));
const progressPercent = computed(() => {
  if (progressCompleted.value) return 100;
  return Math.min(
    89,
    Math.floor(
      classroomCompletion(progressPosition.value, content.value.durationSeconds).ratio * 100,
    ),
  );
});
const paymentBusy = computed(
  () => paymentState.value === "creating" || paymentState.value === "pending",
);

const progressStorage = {
  getItem(key) {
    return uni.getStorageSync(key);
  },
  setItem(key, value) {
    uni.setStorageSync(key, value);
  },
};

function applyProgress(position, completed = false) {
  progressPosition.value = Math.max(0, Math.floor(Number(position) || 0));
  progressCompleted.value = progressCompleted.value || completed === true;
}

function setupProgress() {
  progressTracker = null;
  progressSyncError.value = "";
  progressPosition.value = 0;
  progressCompleted.value = false;
  const loggedIn = Boolean(getToken());
  if (loggedIn && requestedResumePosition > 0) {
    const duration = Math.max(0, Number(content.value.durationSeconds) || 0);
    applyProgress(duration ? Math.min(requestedResumePosition, duration) : requestedResumePosition);
  } else if (!loggedIn) {
    const local = readAnonymousClassroomProgress(progressStorage, contentId.value);
    if (local) applyProgress(local.positionSeconds, local.completed);
  }
  progressTracker = createClassroomProgressTracker({
    contentId: contentId.value,
    durationSeconds: content.value.durationSeconds,
    loggedIn,
    storage: progressStorage,
    completed: progressCompleted.value,
    send: async (id, positionSeconds) => {
      const result = await updateClassroomProgressApi(id, positionSeconds);
      if (!disposed) applyProgress(result?.positionSeconds ?? positionSeconds, result?.completed);
      return result;
    },
  });
}

async function recordProgress(position, { force = false } = {}) {
  if (!progressTracker) return;
  try {
    const snapshot = await progressTracker.record(position, { force });
    if (!disposed) {
      applyProgress(snapshot.positionSeconds, snapshot.completed);
      progressSyncError.value = "";
    }
  } catch (error) {
    if (!disposed) progressSyncError.value = userErrorMessage(error, "学习进度将在网络恢复后重试");
  }
}

async function flushProgress() {
  if (!progressTracker) return;
  try {
    await progressTracker.flush();
    if (!disposed) progressSyncError.value = "";
  } catch (error) {
    if (!disposed) progressSyncError.value = userErrorMessage(error, "学习进度将在网络恢复后重试");
  }
}

function validPlaybackUrl(value) {
  const url = String(value || "").trim();
  return /^https:\/\//i.test(url) ? url : "";
}

function detachAudio(context, bindings) {
  if (!context || !bindings) return;
  for (const [name, handler] of Object.entries(bindings)) {
    const off = context[`off${name}`];
    if (typeof off === "function") off.call(context, handler);
  }
}

function destroyAudio() {
  const context = audioContext;
  const bindings = audioBindings;
  audioContext = null;
  audioBindings = null;
  if (!context) return;
  detachAudio(context, bindings);
  context.stop();
  context.destroy();
  audioPlaying.value = false;
}

function prepareAudio(url) {
  if (disposed || !pageVisible) return;
  destroyAudio();
  const context = uni.createInnerAudioContext();
  const active = () => !disposed && pageVisible && audioContext === context;
  let resumed = false;
  const bindings = {
    Play: () => {
      if (active()) audioPlaying.value = true;
    },
    Pause: () => {
      if (!active()) return;
      audioPlaying.value = false;
      void recordProgress(context.currentTime, { force: true });
    },
    Stop: () => {
      if (active()) audioPlaying.value = false;
    },
    Ended: () => {
      if (!active()) return;
      audioPlaying.value = false;
      audioPosition.value = audioDuration.value;
      void recordProgress(audioDuration.value || context.duration, { force: true });
    },
    Canplay: () => {
      if (!active()) return;
      const duration = Number(context.duration);
      if (Number.isFinite(duration) && duration > 0) audioDuration.value = Math.floor(duration);
      if (!resumed && progressPosition.value > 0) {
        resumed = true;
        context.seek(progressPosition.value);
      }
    },
    TimeUpdate: () => {
      if (!active()) return;
      audioPosition.value = Math.max(0, Math.floor(Number(context.currentTime) || 0));
      const duration = Number(context.duration);
      if (Number.isFinite(duration) && duration > 0) audioDuration.value = Math.floor(duration);
      void recordProgress(audioPosition.value);
    },
    Error: (error) => {
      if (active()) handlePlaybackError(error);
    },
  };
  audioContext = context;
  audioBindings = bindings;
  context.autoplay = false;
  for (const [name, handler] of Object.entries(bindings)) context[`on${name}`](handler);
  context.src = url;
}

function signedPlaybackError(error) {
  const status = Number(error?.statusCode || error?.detail?.statusCode || 0);
  if (status === 401 || status === 403) return true;
  const code = String(error?.code || error?.errCode || error?.detail?.errCode || "").toLowerCase();
  const message = String(
    error?.message || error?.errMsg || error?.detail?.errMsg || "",
  ).toLowerCase();
  return /expired|signature|accessdenied|token.*过期|签名|凭证.*过期|url.*过期/.test(
    `${code} ${message}`,
  );
}

function pauseVisibleMedia() {
  if (audioContext) audioContext.pause();
  if (!videoContext) videoContext = uni.createVideoContext("classroom-video");
  videoContext?.pause();
  audioPlaying.value = false;
}

async function refreshPlayback({ recovery = false } = {}) {
  if (disposed || !pageVisible || !contentId.value || !content.value.canPlay) return;
  if (!recovery) playbackRecoveryUsed = false;
  const ticket = ++playbackTicket;
  playbackLoading.value = true;
  playbackError.value = "";
  playbackRetryLabel.value = "重试播放";
  playbackUrl.value = "";
  destroyAudio();
  try {
    await withClassroomPlaybackRetry(contentId.value, async (playback) => {
      if (disposed || !pageVisible || ticket !== playbackTicket) return;
      const url = validPlaybackUrl(playback?.url);
      if (!url) throw new Error("播放地址无效");
      playbackUrl.value = url;
      if (content.value.contentType === "audio") prepareAudio(url);
    });
  } catch (error) {
    if (!disposed && pageVisible && ticket === playbackTicket) {
      playbackError.value = userErrorMessage(error, "播放地址加载失败，请重试");
      playbackRetryLabel.value = "重试播放";
    }
  } finally {
    if (!disposed && ticket === playbackTicket) playbackLoading.value = false;
  }
}

async function loadDetail() {
  if (disposed) return;
  if (!contentId.value) {
    loading.value = false;
    loadError.value = "课件参数无效";
    return;
  }
  const ticket = ++detailTicket;
  loading.value = true;
  loadError.value = "";
  playbackError.value = "";
  playbackUrl.value = "";
  destroyAudio();
  try {
    const response = await getClassroomContentApi(contentId.value);
    if (disposed || ticket !== detailTicket) return;
    const normalized = normalizeClassroomContent(response);
    if (!normalized.id) throw new Error("课件内容不存在");
    content.value = normalized;
    purchaseController?.stop();
    purchaseController = null;
    paymentState.value = "idle";
    paymentMessage.value = "";
    purchaseOffer.value = null;
    purchaseTargetError.value = "";
    purchaseTarget.value = { type: "content", id: normalized.id, ready: true };
    if (
      !normalized.canPlay &&
      normalized.effectiveAccess === "paid" &&
      normalized.accessLevel === "inherit"
    ) {
      purchaseTarget.value = { type: "series", id: normalized.seriesId, ready: false };
      try {
        if (!normalized.seriesId) throw new Error("系列购买信息缺失");
        const response = await getClassroomSeriesApi(normalized.seriesId);
        if (disposed || ticket !== detailTicket) return;
        const series = normalizeClassroomSeries(response?.series);
        const inheritedLesson = (Array.isArray(response?.contents) ? response.contents : [])
          .map(normalizeClassroomContent)
          .find((item) => item.id === normalized.id && item.accessLevel === "inherit");
        if (
          !series.id ||
          !inheritedLesson ||
          series.effectiveAccess !== "paid" ||
          series.purchaseState !== "purchase_required"
        )
          throw new Error("系列当前不可购买");
        purchaseOffer.value = series;
        purchaseTarget.value = { type: "series", id: series.id, ready: true };
      } catch (error) {
        if (disposed || ticket !== detailTicket) return;
        purchaseTargetError.value = userErrorMessage(error, "系列购买信息加载失败，请重试");
      }
    }
    setupProgress();
    if (normalized.canPlay && pageVisible) await refreshPlayback();
  } catch (error) {
    if (!disposed && ticket === detailTicket) {
      loadError.value = userErrorMessage(error, "课件详情加载失败，请重试");
    }
  } finally {
    if (!disposed && ticket === detailTicket) loading.value = false;
  }
}

function handlePlaybackError(error) {
  if (disposed || !pageVisible) return;
  playbackTicket += 1;
  playbackLoading.value = false;
  playbackUrl.value = "";
  destroyAudio();
  if (signedPlaybackError(error) && !playbackRecoveryUsed) {
    playbackRecoveryUsed = true;
    playbackError.value = "播放凭证已失效，正在刷新…";
    refreshPlayback({ recovery: true });
    return;
  }
  const signedError = signedPlaybackError(error);
  playbackError.value = signedError
    ? "播放凭证仍然无效，请点击刷新后重试"
    : "媒体播放失败，请检查网络或文件格式后重试";
  playbackRetryLabel.value = signedError ? "刷新播放凭证" : "重新加载播放";
}

function toggleAudio() {
  if (disposed || !pageVisible || !audioContext || !playbackUrl.value) return;
  if (audioPlaying.value) audioContext.pause();
  else audioContext.play();
}

function seekAudio(event) {
  if (disposed || !pageVisible || !audioContext) return;
  const seconds = Math.max(0, Number(event?.detail?.value) || 0);
  audioContext.seek(seconds);
  audioPosition.value = Math.floor(seconds);
  void recordProgress(seconds);
}

function handleVideoTimeUpdate(event) {
  const current = Math.max(0, Number(event?.detail?.currentTime) || 0);
  applyProgress(current);
  void recordProgress(current);
}

function handleVideoPause(event) {
  const current = Math.max(0, Number(event?.detail?.currentTime) || progressPosition.value);
  applyProgress(current);
  void recordProgress(current, { force: true });
}

function handleVideoEnded() {
  const duration = Math.max(0, Number(content.value.durationSeconds) || progressPosition.value);
  applyProgress(duration);
  void recordProgress(duration, { force: true });
}

function formatTime(seconds) {
  const value = Math.max(0, Math.floor(Number(seconds) || 0));
  return `${Math.floor(value / 60)}:${String(value % 60).padStart(2, "0")}`;
}

function requestWechatPayment(pay = {}) {
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

function ensurePurchaseController() {
  if (purchaseController) return purchaseController;
  const target = { ...purchaseTarget.value };
  if (!target.ready || !target.id) return null;
  purchaseController = createClassroomPurchaseController({
    create: () => createClassroomOrderApi(target.type, target.id),
    pay: async (order) => {
      if (order?.payParams?.devMode) return devPayClassroomOrderApi(order.outTradeNo);
      return requestWechatPayment(order?.payParams);
    },
    status: () => getClassroomOrderStatusApi(target.type, target.id),
    onChange: (snapshot) => {
      if (disposed) return;
      paymentState.value = snapshot.state;
      paymentMessage.value = snapshot.message;
    },
    onSuccess: async () => {
      if (!disposed) await loadDetail();
    },
  });
  return purchaseController;
}

function startPurchase() {
  if (disposed || paymentBusy.value) return;
  if (!getToken()) {
    uni.switchTab({ url: "/pages/profile/profile" });
    return;
  }
  if (!purchaseTarget.value.ready) return;
  return ensurePurchaseController()?.purchase();
}

function retryPurchase() {
  if (disposed || paymentBusy.value) return;
  return ensurePurchaseController()?.retry();
}

function cancelPurchase() {
  purchaseController?.reset();
  paymentState.value = "idle";
  paymentMessage.value = "";
}

function handleAccessAction() {
  if (disposed) return;
  if (accessAction.value.type === "login" || accessAction.value.type === "member") {
    uni.switchTab({ url: "/pages/profile/profile" });
    return;
  }
  if (accessAction.value.type === "purchase") {
    startPurchase();
  }
}

onLoad((options = {}) => {
  disposed = false;
  pageVisible = true;
  contentId.value = String(options.id || "").trim();
  requestedResumePosition = Math.max(0, Math.floor(Number(options.position) || 0));
  loadDetail();
});

onHide(() => {
  if (disposed) return;
  void flushProgress();
  pageVisible = false;
  playbackTicket += 1;
  playbackLoading.value = false;
  pauseVisibleMedia();
});

onShow(() => {
  if (!disposed) pageVisible = true;
});

onUnload(() => {
  if (disposed) return;
  disposed = true;
  void flushProgress();
  purchaseController?.stop();
  purchaseController = null;
  progressTracker = null;
  pageVisible = false;
  detailTicket += 1;
  playbackTicket += 1;
  loading.value = false;
  playbackLoading.value = false;
  playbackUrl.value = "";
  pauseVisibleMedia();
  destroyAudio();
  videoContext = null;
});
</script>

<template>
  <view class="classroom-detail ios-page ios-safe-bottom">
    <view v-if="loading" class="detail-state" aria-live="polite">课件详情加载中…</view>
    <view v-else-if="loadError" class="detail-state detail-state--error" aria-live="polite">
      <text>{{ loadError }}</text>
      <button class="detail-action" :disabled="loading" @click="loadDetail">重新加载</button>
    </view>

    <block v-else>
      <view class="detail-head ios-card">
        <image
          v-if="content.coverUrl"
          class="detail-head__cover"
          :class="classroomCoverRatioClass(content)"
          :src="content.coverUrl"
          mode="aspectFill"
          lazy-load
        />
        <view
          v-else
          class="detail-head__cover detail-head__cover--fallback"
          :class="classroomCoverRatioClass(content)"
          aria-hidden="true"
          >{{
          content.contentType === "audio" ? "音" : "课"
        }}</view>
        <view class="detail-head__body">
          <view class="detail-head__meta">
            <text class="nx-tag">{{ classroomAccessLabel(content.effectiveAccess) }}</text>
            <text>{{ content.contentType === "audio" ? "音频课件" : "视频课件" }}</text>
          </view>
          <text class="detail-head__title">{{ content.title }}</text>
          <text class="detail-head__teacher">{{ content.teacherName || "九型老师" }}</text>
        </view>
      </view>

      <view v-if="!content.canPlay" class="access-panel ios-card" aria-live="polite">
        <text class="access-panel__title">{{ accessAction.label }}</text>
        <text class="access-panel__copy">完成对应权限后，即可播放本课件。</text>
        <text v-if="purchaseTargetError" class="access-panel__error">{{
          purchaseTargetError
        }}</text>
        <button v-if="purchaseTargetError" class="detail-action" @click="loadDetail">
          重新加载购买信息
        </button>
        <button
          v-if="
            !purchaseTargetError &&
            accessAction.type !== 'blocked' &&
            accessAction.type !== 'unavailable'
          "
          class="primary-action"
          @click="handleAccessAction"
        >
          {{ accessAction.label }}
        </button>
      </view>

      <view v-else class="player-panel ios-card">
        <view v-if="playbackLoading" class="detail-state" aria-live="polite"
          >正在获取安全播放地址…</view
        >
        <view v-else-if="playbackError" class="detail-state detail-state--error" aria-live="polite">
          <text>{{ playbackError }}</text>
          <button class="detail-action" :disabled="playbackLoading" @click="refreshPlayback">
            {{ playbackRetryLabel }}
          </button>
        </view>
        <block v-else-if="playbackUrl">
          <video
            v-if="content.contentType === 'video'"
            id="classroom-video"
            class="video-player"
            :src="playbackUrl"
            :initial-time="progressPosition"
            controls
            object-fit="contain"
            @timeupdate="handleVideoTimeUpdate"
            @pause="handleVideoPause"
            @ended="handleVideoEnded"
            @error="handlePlaybackError"
          />
          <view v-else class="audio-player">
            <view
              class="audio-player__disc"
              :class="{ 'audio-player__disc--playing': audioPlaying }"
              aria-hidden="true"
              >♫</view
            >
            <text class="audio-player__title">{{ content.title }}</text>
            <slider
              class="audio-player__slider"
              :value="audioPosition"
              :max="audioDuration || content.durationSeconds || 1"
              active-color="#0f766e"
              block-size="18"
              @change="seekAudio"
            />
            <view class="audio-player__time">
              <text>{{ formatTime(audioPosition) }}</text>
              <text>{{ formatTime(audioDuration || content.durationSeconds) }}</text>
            </view>
            <button
              class="primary-action"
              :aria-label="audioPlaying ? '暂停音频' : '播放音频'"
              @click="toggleAudio"
            >
              {{ audioPlaying ? "暂停音频" : "播放音频" }}
            </button>
          </view>
        </block>
        <button v-else class="detail-action" @click="refreshPlayback">加载播放内容</button>
      </view>

      <view v-if="content.canPlay" class="progress-panel ios-card">
        <view class="progress-panel__head">
          <text class="progress-panel__title">学习进度</text>
          <text>{{ progressCompleted ? "已完成" : `${progressPercent}%` }}</text>
        </view>
        <view
          class="progress-panel__bar"
          role="progressbar"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-valuenow="progressPercent"
          :aria-label="progressCompleted ? '课件已完成' : `课件学习进度 ${progressPercent}%`"
        >
          <view class="progress-panel__fill" :style="{ width: `${progressPercent}%` }" />
        </view>
        <text class="progress-panel__copy">
          {{
            progressCompleted ? "已达到 90% 完成标准" : `已学习至 ${formatTime(progressPosition)}`
          }}
        </text>
        <text v-if="progressSyncError" class="progress-panel__error" aria-live="polite">{{
          progressSyncError
        }}</text>
      </view>

      <view
        v-if="!content.canPlay && accessAction.type === 'purchase' && paymentState !== 'idle'"
        class="payment-panel ios-card"
        aria-live="polite"
      >
        <text class="payment-panel__title">
          {{
            paymentState === "success"
              ? "购买成功"
              : paymentState === "pending"
                ? "等待支付确认"
                : paymentState === "creating"
                  ? "正在创建订单"
                  : paymentState === "cancelled"
                    ? "支付已取消"
                    : "支付未完成"
          }}
        </text>
        <text class="payment-panel__copy">{{ paymentMessage }}</text>
        <button
          v-if="paymentState === 'failure' || paymentState === 'cancelled'"
          class="primary-action"
          :disabled="paymentBusy"
          @click="retryPurchase"
        >
          重新支付
        </button>
        <button
          v-if="paymentState !== 'success' && !paymentBusy"
          class="detail-action"
          @click="cancelPurchase"
        >
          暂不购买
        </button>
      </view>

      <view class="description-panel ios-card">
        <text class="description-panel__title">课件介绍</text>
        <text class="description-panel__copy">{{
          content.description || "老师正在完善本课件介绍。"
        }}</text>
      </view>
    </block>
  </view>
</template>

<style scoped>
.classroom-detail {
  display: flex;
  flex-direction: column;
  gap: 24rpx;
  min-height: 100vh;
  padding: 28rpx;
  background: #f4f8f6;
  box-sizing: border-box;
}
.detail-state {
  padding: 48rpx 30rpx;
  color: #64756e;
  font-size: 27rpx;
  line-height: 1.6;
  text-align: center;
  background: #fff;
  border-radius: 28rpx;
}
.detail-state--error {
  color: #9f3a38;
}
.detail-action,
.primary-action {
  min-height: 88rpx;
  margin-top: 22rpx;
  padding: 0 32rpx;
  font-size: 27rpx;
  font-weight: 800;
  line-height: 88rpx;
  border-radius: 20rpx;
}
.progress-panel,
.payment-panel {
  padding: 30rpx;
  background: #fff;
  border-radius: 30rpx;
}
.progress-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #385349;
  font-size: 24rpx;
}
.progress-panel__title,
.payment-panel__title {
  color: #19342b;
  font-size: 29rpx;
  font-weight: 900;
}
.progress-panel__bar {
  height: 14rpx;
  margin-top: 20rpx;
  overflow: hidden;
  background: #dcebe5;
  border-radius: 999rpx;
}
.progress-panel__fill {
  height: 100%;
  background: #0f766e;
  border-radius: inherit;
}
.progress-panel__copy,
.progress-panel__error,
.payment-panel__copy {
  display: block;
  margin-top: 14rpx;
  color: #667970;
  font-size: 24rpx;
  line-height: 1.55;
}
.progress-panel__error {
  color: #9f3a38;
}
.detail-action {
  color: #0f6b4f;
  background: #ecfdf5;
}
.primary-action {
  width: 100%;
  color: #fff;
  background: #0f766e;
}
.detail-action::after,
.primary-action::after {
  border: 0;
}
.detail-head {
  display: flex;
  align-items: flex-start;
  overflow: hidden;
  background: #fff;
  border-radius: 30rpx;
}
.detail-head__cover {
  width: 240rpx;
  background: #dbeee6;
}
.detail-head__cover.classroom-cover--16x9 {
  height: 135rpx;
}
.detail-head__cover.classroom-cover--9x16 {
  height: 427rpx;
}
.detail-head__cover.classroom-cover--1x1 {
  height: 240rpx;
}
.detail-head__cover--fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #0f766e;
  font-size: 58rpx;
  font-weight: 900;
}
.detail-head__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  padding: 28rpx 24rpx;
}
.detail-head__meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #6b7b73;
  font-size: 22rpx;
}
.detail-head__title {
  margin-top: 22rpx;
  color: #17241e;
  font-size: 34rpx;
  font-weight: 900;
  line-height: 1.4;
}
.detail-head__teacher {
  margin-top: auto;
  padding-top: 18rpx;
  color: #617169;
  font-size: 24rpx;
}
.access-panel,
.player-panel,
.description-panel {
  padding: 30rpx;
  background: #fff;
  border-radius: 30rpx;
}
.access-panel__title,
.access-panel__copy,
.description-panel__title,
.description-panel__copy {
  display: block;
}
.access-panel__title,
.description-panel__title {
  color: #17241e;
  font-size: 31rpx;
  font-weight: 900;
}
.access-panel__copy,
.description-panel__copy {
  margin-top: 14rpx;
  color: #68776f;
  font-size: 25rpx;
  line-height: 1.7;
}
.video-player {
  width: 100%;
  min-height: 390rpx;
  background: #0b1511;
  border-radius: 24rpx;
}
.audio-player {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 24rpx 6rpx 4rpx;
}
.audio-player__disc {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 160rpx;
  height: 160rpx;
  color: #fff;
  font-size: 58rpx;
  background: linear-gradient(135deg, #0f766e, #22c55e);
  border: 16rpx solid #d9f3e7;
  border-radius: 50%;
}
.audio-player__disc--playing {
  box-shadow: 0 0 0 12rpx rgba(34, 197, 94, 0.12);
}
.audio-player__title {
  margin-top: 26rpx;
  color: #17241e;
  font-size: 29rpx;
  font-weight: 900;
  text-align: center;
}
.audio-player__slider {
  width: 100%;
  margin-top: 24rpx;
}
.audio-player__time {
  display: flex;
  justify-content: space-between;
  width: 100%;
  color: #718078;
  font-size: 22rpx;
}
</style>
