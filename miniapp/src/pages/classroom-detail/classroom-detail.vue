<script setup>
import { computed, ref, watch } from "vue";
import { onHide, onLoad, onShow, onUnload, onShareAppMessage, onShareTimeline } from "@dcloudio/uni-app";
import NxShareActions from "../../components/NxShareActions.vue";
import { buildShareCard, showPublicShareMenu, isTimelinePreview, requireFullMiniapp } from "../../utils/share";
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
  readAnonymousClassroomProgress,
} from "../../utils/classroomProgress";
import { createWechatPaymentController } from "../../utils/payment";
import { normalizeMiniappPayment } from "../../utils/miniappPages";
import { getStoredSiteConfig, refreshSiteConfig } from "../../utils/siteConfig";
import { normalizeTeachers } from "../../utils/teacherCourseware";
import { STUDIO_TEACHER } from "../../data/teacherStudio";
import { getToken } from "../../utils/auth";
import { userErrorMessage } from "../../utils/userMessage";
import { clearBookingIntent, setBookingIntent } from "../../utils/bookingIntent";
import NxImagePreview from "../../components/NxImagePreview.vue";
import { previewImage } from "../../utils/imagePreview";
import { isWechatDevtools } from "../../utils/imagePreview";

const timelinePreview = isTimelinePreview();
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
const purchaseInFlight = ref(false);
const purchaseTarget = ref({ type: "content", id: "", ready: true });
const purchaseOffer = ref(null);
const purchaseTargetError = ref("");
const paymentEnabled = ref(normalizeMiniappPayment(getStoredSiteConfig()).enabled);
const coverImageFailed = ref(false);
const teacherAvatarFailed = ref(false);
const teacherAvatarPreviewVisible = ref(false);
const teacherAvatar = computed(() => {
  const configured = normalizeTeachers(getStoredSiteConfig() || {})[0]?.avatar || STUDIO_TEACHER.avatar;
  return /\/avatars\//i.test(configured) || /teacher-poster/i.test(configured)
    ? STUDIO_TEACHER.avatar
    : configured;
});
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
let purchaseOperation = null;
let requestedResumePosition = 0;
let paymentRefreshTicket = 0;
const contentShareable = computed(() => !loading.value && !loadError.value
  && /^[1-9]\d*$/.test(contentId.value) && content.value.id === contentId.value && !!content.value.title);
function contentShareCard() {
  if (!contentShareable.value) return buildShareCard({ kind: "home" });
  return buildShareCard({ kind: "content", id: content.value.id,
    title: content.value.title, imageUrl: content.value.coverUrl });
}
function syncContentShareMenu() {
  if (!disposed && pageVisible) showPublicShareMenu(contentShareable.value);
}
watch(contentShareable, syncContentShareMenu, { flush: "sync" });
onShareAppMessage(() => contentShareCard().appMessage);
onShareTimeline(() => contentShareCard().timeline);

async function refreshPaymentAvailability() {
  const previous = paymentEnabled.value;
  const ticket = ++paymentRefreshTicket;
  try {
    const config = await refreshSiteConfig();
    if (disposed || ticket !== paymentRefreshTicket) return;
    const next = normalizeMiniappPayment(config).enabled;
    paymentEnabled.value = next;
    // A detail opened while payment was offline skipped parent-series lookup.
    // Rebuild the purchase target when the switch comes back online instead of
    // accidentally falling back to a single-lesson order.
    if (
      !previous &&
      next &&
      contentId.value &&
      !content.value.canPlay &&
      content.value.effectiveAccess === "paid" &&
      content.value.accessLevel === "inherit" &&
      purchaseTarget.value.type !== "series"
    ) {
      await loadDetail();
    }
  } catch {
    // Keep the cached switch when a background refresh is unavailable.
  }
}

const accessAction = computed(() => {
  const action = classroomPurchaseAction(purchaseOffer.value || content.value);
  if (!paymentEnabled.value && action.type === "purchase") {
    return { type: "unavailable", label: "暂不可购买" };
  }
  return action;
});
const progressPercent = computed(() => {
  if (progressCompleted.value) return 100;
  return Math.min(
    89,
    Math.floor(
      classroomCompletion(progressPosition.value, content.value.durationSeconds).ratio * 100,
    ),
  );
});
const paymentBusy = computed(() => purchaseInFlight.value);

function consultTeacher() {
  if (!requireFullMiniapp()) return;
  setBookingIntent({ kind: "consult", intentText: content.value.title });
  uni.switchTab({
    url: "/pages/booking/booking",
    fail() { clearBookingIntent(); },
  });
}

function openTeacherDetail() {
  if (!requireFullMiniapp()) return;
  uni.navigateTo({ url: "/pages/teacher/teacher" });
}

function previewTeacherAvatar() {
  if (teacherAvatarFailed.value) return;
  if (isWechatDevtools()) {
    teacherAvatarPreviewVisible.value = true;
    return;
  }
  previewImage(teacherAvatar.value);
}

function closeTeacherAvatarPreview() {
  teacherAvatarPreviewVisible.value = false;
}

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
  if (timelinePreview) return;
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
      try {
        const result = await updateClassroomProgressApi(id, positionSeconds);
        if (!disposed) applyProgress(result?.positionSeconds ?? positionSeconds, result?.completed);
        return result;
      } catch (_) {
        // Progress is best-effort. Keep playback and the local progress bar
        // usable when an expired session or platform request fails.
        return { positionSeconds, completed: progressCompleted.value };
      }
    },
  });
}

async function recordProgress(position, { force = false } = {}) {
  if (timelinePreview || !progressTracker) return;
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
  if (timelinePreview || !progressTracker) return;
  try {
    await progressTracker.flush();
    if (!disposed) progressSyncError.value = "";
  } catch (error) {
    if (!disposed) progressSyncError.value = userErrorMessage(error, "学习进度将在网络恢复后重试");
  }
}

function validPlaybackUrl(value) {
  const url = String(value || "").trim();
  if (import.meta.env?.DEV === true && import.meta.env?.VITE_UI_PREVIEW === 'true'
    && /^http:\/\/127\.0\.0\.1:\d+\/__studio-media\/laohan-\d+\.mp4$/.test(url)) return url;
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
  if (!/^[1-9]\d*$/.test(contentId.value)) {
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
    if (!normalized.id || normalized.id !== contentId.value) throw new Error("课件内容不存在");
    content.value = normalized;
    coverImageFailed.value = false;
    purchaseController?.stop();
    purchaseController = null;
    paymentState.value = "idle";
    paymentMessage.value = "";
    purchaseOffer.value = null;
    purchaseTargetError.value = "";
    purchaseTarget.value = { type: "content", id: normalized.id, ready: true };
    if (
      paymentEnabled.value &&
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
    // Progress sync is auxiliary. A platform-specific storage/runtime issue
    // must not prevent the lesson metadata and media from rendering.
    try {
      setupProgress();
    } catch (error) {
      progressTracker = null;
      progressSyncError.value = userErrorMessage(error, "学习进度暂不可用");
    }
    if (normalized.canPlay && pageVisible) await refreshPlayback();
  } catch (error) {
    if (!disposed && ticket === detailTicket) {
      loadError.value = userErrorMessage(error, "课件详情加载失败，请重试");
    }
  } finally {
    if (!disposed && ticket === detailTicket) loading.value = false;
  }
}

function markCoverImageError() {
  coverImageFailed.value = true;
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

function ensurePurchaseController() {
  if (!paymentEnabled.value) return null;
  if (purchaseController) return purchaseController;
  const target = { ...purchaseTarget.value };
  if (!target.ready || !target.id) return null;
  purchaseController = createWechatPaymentController({
    create: () => createClassroomOrderApi(target.type, target.id),
    devPay: (order) => devPayClassroomOrderApi(order.outTradeNo),
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

function trackPurchase(run) {
  if (purchaseOperation) return;
  purchaseInFlight.value = true;
  let operation;
  try {
    operation = Promise.resolve(run());
  } catch (error) {
    purchaseInFlight.value = false;
    throw error;
  }
  let tracked;
  tracked = operation.finally(() => {
    if (purchaseOperation !== tracked) return;
    purchaseOperation = null;
    purchaseInFlight.value = false;
  });
  purchaseOperation = tracked;
  return tracked;
}

function startPurchase() {
  if (!requireFullMiniapp()) return;
  if (disposed || purchaseOperation || !paymentEnabled.value) return;
  if (!getToken()) {
    uni.switchTab({ url: "/pages/profile/profile" });
    return;
  }
  if (!purchaseTarget.value.ready) return;
  const controller = ensurePurchaseController();
  if (!controller) return;
  return trackPurchase(() => controller.purchase());
}

function retryPurchase() {
  if (!requireFullMiniapp()) return;
  if (disposed || purchaseOperation || !paymentEnabled.value) return;
  const controller = ensurePurchaseController();
  if (!controller) return;
  return trackPurchase(() => controller.retry());
}

function cancelPurchase() {
  purchaseController?.reset();
  paymentState.value = "idle";
  paymentMessage.value = "";
}

function handleAccessAction() {
  if (!requireFullMiniapp()) return;
  if (disposed || !paymentEnabled.value) return;
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
  paymentEnabled.value = normalizeMiniappPayment(getStoredSiteConfig()).enabled;
  contentId.value = String(options.id || "").trim();
  requestedResumePosition = timelinePreview ? 0 : Math.max(0, Math.floor(Number(options.position) || 0));
  syncContentShareMenu();
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
  if (disposed) return;
  paymentEnabled.value = normalizeMiniappPayment(getStoredSiteConfig()).enabled;
  pageVisible = true;
  syncContentShareMenu();
  void refreshPaymentAvailability();
  if (content.value.canPlay && !playbackUrl.value && !playbackLoading.value && !playbackError.value) {
    refreshPlayback();
  }
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
  purchaseOperation = null;
  purchaseInFlight.value = false;
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
    <NxImagePreview
      v-if="teacherAvatar && !teacherAvatarFailed"
      :visible="teacherAvatarPreviewVisible"
      :src="teacherAvatar"
      :alt="`${content.teacherName || '授课老师'}头像`"
      @close="closeTeacherAvatarPreview"
    />
    <view class="detail-shell__context">
      <text class="detail-shell__context-label">老师课堂</text>
      <text class="detail-shell__context-separator">/</text>
      <text>把理解，带回生活</text>
    </view>

    <view v-if="loading" class="detail-state" aria-live="polite">
      <text class="detail-state__title">正在准备课件</text>
      <text class="detail-state__copy">稍等片刻，一起进入老师的课堂。</text>
    </view>
    <view v-else-if="loadError" class="detail-state detail-state--error" aria-live="polite">
      <text class="detail-state__title">课件加载未完成</text>
      <text class="detail-state__copy">{{ loadError }}</text>
      <view class="detail-actions">
        <button class="detail-action" :disabled="loading" @click="loadDetail">重新加载</button>
      </view>
    </view>

    <block v-else>
      <view class="media-hero ios-card">
        <view class="detail-head__media">
          <view v-if="content.canPlay" class="player-panel">
            <view class="player-panel__head">
              <text class="player-panel__eyebrow">{{ content.contentType === "audio" ? "听老师说" : "老师短讲" }}</text>
              <text class="player-panel__badge">{{ classroomAccessLabel(content.effectiveAccess) }}</text>
            </view>
            <view class="player-panel__body">
              <view v-if="playbackLoading" class="detail-state detail-state--embedded" aria-live="polite">
                <text class="detail-state__title">正在准备播放</text>
                <text class="detail-state__copy">即将进入课堂，请稍候…</text>
              </view>
              <view v-else-if="playbackError" class="detail-state detail-state--embedded detail-state--error" aria-live="polite">
                <text class="detail-state__title">播放内容加载未完成</text>
                <text class="detail-state__copy">{{ playbackError }}</text>
                <view class="detail-actions">
                  <button class="detail-action" :disabled="playbackLoading" @click="refreshPlayback">{{ playbackRetryLabel }}</button>
                </view>
              </view>
              <block v-else-if="playbackUrl">
                <video
                  v-if="content.contentType === 'video'"
                  id="classroom-video"
                  class="video-player"
                  :class="classroomCoverRatioClass(content)"
                  :src="playbackUrl"
                  :poster="coverImageFailed ? '' : content.coverUrl"
                  :initial-time="progressPosition"
                  controls
                  object-fit="contain"
                  @timeupdate="handleVideoTimeUpdate"
                  @pause="handleVideoPause"
                  @ended="handleVideoEnded"
                  @error="handlePlaybackError"
                />
                <view v-else class="audio-player">
                  <view class="audio-player__disc" :class="{ 'audio-player__disc--playing': audioPlaying }" aria-hidden="true">
                    <text class="audio-player__disc-label">听</text>
                  </view>
                  <text class="audio-player__title">留一点时间，听见新的视角</text>
                  <slider
                    class="audio-player__slider"
                    :value="audioPosition"
                    :max="audioDuration || content.durationSeconds || 1"
                    active-color="#A55C3B"
                    background-color="#E9E4DA"
                    block-color="#A55C3B"
                    block-size="18"
                    @change="seekAudio"
                  />
                  <view class="audio-player__time">
                    <text>{{ formatTime(audioPosition) }}</text>
                    <text>{{ formatTime(audioDuration || content.durationSeconds) }}</text>
                  </view>
                  <view class="detail-actions detail-actions--audio">
                    <button class="primary-action" :aria-label="audioPlaying ? '暂停音频' : '播放音频'" @click="toggleAudio">{{ audioPlaying ? "暂停音频" : "播放音频" }}</button>
                  </view>
                </view>
              </block>
              <view v-else class="detail-actions player-panel__retry">
                <button class="detail-action" @click="refreshPlayback">加载播放内容</button>
              </view>
            </view>
          </view>
          <view v-else class="detail-head__cover-shell" :class="classroomCoverRatioClass(content)">
            <image
              v-if="content.coverUrl && !coverImageFailed"
              class="detail-head__cover"
              :class="classroomCoverRatioClass(content)"
              :src="content.coverUrl"
              mode="aspectFill"
              lazy-load
              @error="markCoverImageError"
            />
            <view v-else class="detail-head__cover detail-head__cover--fallback" :class="classroomCoverRatioClass(content)" aria-hidden="true">{{ content.contentType === "audio" ? "听" : "课" }}</view>
            <view class="detail-head__shade" aria-hidden="true" />
            <view class="detail-head__media-meta">
              <text class="detail-head__pill detail-head__pill--access">{{ classroomAccessLabel(content.effectiveAccess) }}</text>
            </view>
          </view>
        </view>
        <view class="detail-head__body">
          <view class="content-summary">
            <view class="detail-head__meta">
              <text>{{ content.contentType === "audio" ? "音频课堂" : content.effectiveAccess === "public" ? "免费短讲" : "视频课堂" }}</text>
              <text v-if="content.durationSeconds">{{ formatTime(content.durationSeconds) }} · {{ content.contentType === "audio" ? "随时收听" : "随时回看" }}</text>
            </view>
            <text class="detail-head__title">{{ content.title }}</text>
            <NxShareActions :disabled="!contentShareable" />
            <view class="content-summary__teacher" hover-class="teacher-link--pressed" aria-label="查看授课老师介绍" role="button" @click="openTeacherDetail">
              <button v-if="teacherAvatar && !teacherAvatarFailed" class="content-summary__avatar-action" aria-label="预览授课老师头像" @click.stop="previewTeacherAvatar">
                <image class="content-summary__avatar" :src="teacherAvatar" mode="aspectFill" :aria-label="`${content.teacherName || '九型老师'}头像`" @error="teacherAvatarFailed = true" />
              </button>
              <view v-else class="content-summary__avatar content-summary__avatar--fallback" aria-hidden="true">{{ (content.teacherName || "九型老师").slice(0, 1) }}</view>
              <view class="content-summary__teacher-copy">
                <text class="detail-head__teacher">{{ content.teacherName || "九型老师" }}</text>
                <text class="content-summary__teacher-label">授课老师</text>
              </view>
              <text class="content-summary__teacher-link">了解老师</text>
              <text class="content-summary__arrow" aria-hidden="true">›</text>
            </view>
          </view>
        </view>
      </view>

      <view v-if="!timelinePreview && !content.canPlay" class="access-panel ios-card" aria-live="polite">
        <text class="panel-eyebrow">学习方式</text>
        <text class="access-panel__title">{{ accessAction.label }}</text>
        <text class="access-panel__copy">{{ accessAction.type === "unavailable" ? "该课件暂未开放购买。" : "完成对应访问步骤后，即可进入本课件学习。" }}</text>
        <text v-if="purchaseTargetError" class="access-panel__error">{{ purchaseTargetError }}</text>
        <view class="detail-actions detail-actions--stacked">
          <button v-if="purchaseTargetError" class="detail-action" @click="loadDetail">重新加载购买信息</button>
          <button
            v-if="!purchaseTargetError && accessAction.type !== 'blocked' && accessAction.type !== 'unavailable'"
            class="primary-action"
            :disabled="paymentBusy"
            @click="handleAccessAction"
          >{{ paymentBusy ? "正在处理…" : accessAction.label }}</button>
        </view>
      </view>

      <view
        v-if="paymentEnabled && !content.canPlay && accessAction.type === 'purchase' && paymentState !== 'idle'"
        class="payment-panel ios-card"
        aria-live="polite"
      >
        <text class="panel-eyebrow">订单状态</text>
        <text class="payment-panel__title">{{
          paymentState === "success" ? "购买成功"
            : paymentState === "pending" ? "等待支付确认"
              : paymentState === "creating" ? "正在创建订单"
                : paymentState === "cancelled" ? "支付已取消" : "支付未完成"
        }}</text>
        <text class="payment-panel__copy">{{ paymentMessage }}</text>
        <view class="detail-actions detail-actions--stacked">
          <button v-if="paymentState === 'failure' || paymentState === 'cancelled'" class="primary-action" :disabled="paymentBusy" @click="retryPurchase">重新支付</button>
          <button v-if="paymentState !== 'success' && !paymentBusy" class="detail-action" @click="cancelPurchase">暂不购买</button>
        </view>
      </view>

      <view class="description-panel ios-card">
        <view class="section-heading">
          <text class="description-panel__title">这一讲，聊什么</text>
          <text class="panel-eyebrow">ABOUT</text>
        </view>
        <text class="description-panel__copy">{{ content.description || "老师正在完善本课件介绍。" }}</text>
      </view>

      <view v-if="!timelinePreview && content.canPlay" class="progress-panel ios-card">
        <view class="progress-panel__head">
          <text class="progress-panel__title">我的学习进度</text>
          <text class="progress-panel__value">{{ progressCompleted ? "已完成" : `${progressPercent}%` }}</text>
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
        <text class="progress-panel__copy">{{ progressCompleted ? "这一讲已学完，试着把新的理解用在生活里。" : progressPosition > 0 ? `已学习至 ${formatTime(progressPosition)}，下次从这里继续。` : "从这一讲开始，慢慢积累自己的觉察。" }}</text>
        <text v-if="progressSyncError" class="progress-panel__error" aria-live="polite">{{ progressSyncError }}</text>
      </view>

      <view v-if="!timelinePreview" class="detail-consult">
        <view class="detail-consult__copy">
          <text class="detail-consult__title">想和老师再聊一聊？</text>
          <text class="detail-consult__lead">带着你的问题，找到适合的学习方向。</text>
        </view>
        <button class="detail-consult__action" hover-class="teacher-link--pressed" @click="consultTeacher">预约咨询 <text aria-hidden="true">↗</text></button>
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
  padding: 24rpx 32rpx 48rpx;
  color: var(--nx-text);
  background: var(--nx-page-bg);
  box-sizing: border-box;
}
.detail-shell__context { display: flex; align-items: center; gap: 16rpx; padding: 4rpx 0 10rpx; color: var(--nx-text-muted); font-size: 22rpx; }
.detail-shell__context-label { color: var(--nx-brand-700); font-weight: 600; }
.detail-shell__context-separator { color: #C7BFB2; }
.detail-state__title, .detail-state__copy, .panel-eyebrow { display: block; }
.panel-eyebrow { color: var(--nx-text-muted); font-size: 19rpx; letter-spacing: 2rpx; }
.detail-state { padding: 64rpx 32rpx; color: var(--nx-text-muted); font-size: 26rpx; line-height: 1.65; text-align: center; background: var(--nx-surface); border: 1rpx solid var(--nx-border); border-radius: 24rpx; }
.detail-state--embedded { margin: 24rpx; padding: 40rpx 24rpx; background: var(--nx-surface-soft); border-radius: 16rpx; }
.detail-state--error { color: #A54937; }
.detail-state__title { color: var(--nx-text); font-size: 30rpx; font-weight: 600; }
.detail-state--error .detail-state__title { color: #8F3F32; }
.detail-state__copy { margin-top: 12rpx; color: inherit; font-size: 24rpx; }
.detail-actions { display: flex; flex-direction: column; gap: 16rpx; width: 100%; margin-top: 24rpx; }
.detail-action,
.primary-action {
  width: 100%;
  min-height: 88rpx;
  margin: 0;
  padding: 0 28rpx;
  font-size: 26rpx;
  font-weight: 600;
  line-height: 88rpx;
  border-radius: 16rpx;
  box-sizing: border-box;
  touch-action: manipulation;
}
.detail-action { color: var(--nx-brand-900); background: var(--nx-surface-soft); border: 1rpx solid var(--nx-border); }
.primary-action { color: #FFFFFF; background: #A55C3B; }
.detail-action[disabled], .primary-action[disabled] { color: var(--nx-text-muted); background: var(--nx-border); opacity: .72; }
.detail-action::after, .primary-action::after, .detail-consult__action::after { border: 0; }
.media-hero { overflow: hidden; background: var(--nx-surface); border: 1rpx solid var(--nx-border); border-radius: 24rpx; box-shadow: none; }
.detail-head__media { width: 100%; background: #282A27; }
.detail-head__cover-shell { position: relative; width: 100%; overflow: hidden; background: #282A27; }
.detail-head__cover-shell.classroom-cover--16x9 { height: 340rpx; }
.detail-head__cover-shell.classroom-cover--9x16 { height: 720rpx; max-height: 64vh; }
.detail-head__cover-shell.classroom-cover--1x1 { height: 560rpx; }
.detail-head__cover { position: absolute; top: 0; right: 0; bottom: 0; left: 0; width: 100%; height: 100%; background: #282A27; }
.detail-head__cover.classroom-cover--16x9, .detail-head__cover.classroom-cover--9x16, .detail-head__cover.classroom-cover--1x1 { height: 100%; }
.detail-head__cover--fallback { display: flex; align-items: center; justify-content: center; color: var(--nx-accent-gold); font-size: 88rpx; font-family: Georgia, "Songti SC", serif; background: #343931; }
.detail-head__shade { position: absolute; top: 0; right: 0; bottom: 0; left: 0; background: linear-gradient(180deg, rgba(40, 42, 39, .18), transparent 50%, rgba(40, 42, 39, .32)); }
.detail-head__media-meta { position: absolute; top: 24rpx; right: 24rpx; display: flex; align-items: center; }
.detail-head__pill { padding: 8rpx 16rpx; color: #FFFFFF; font-size: 21rpx; line-height: 1.5; border-radius: 8rpx; }
.detail-head__pill--access { background: rgba(40, 42, 39, .72); }
.detail-head__body { padding: 30rpx 28rpx 0; }
.content-summary { display: flex; flex-direction: column; }
.detail-head__meta { display: flex; align-items: center; justify-content: space-between; gap: 16rpx; color: #A55C3B; font-size: 22rpx; line-height: 1.5; }
.detail-head__title { display: block; margin-top: 18rpx; color: var(--nx-text); font-size: 38rpx; font-weight: 600; line-height: 1.45; letter-spacing: -.5rpx; }
.content-summary__teacher { display: flex; align-items: center; gap: 18rpx; min-height: 104rpx; margin-top: 28rpx; padding: 20rpx 0; border-top: 1rpx solid var(--nx-border); box-sizing: border-box; }
.content-summary__avatar-action { display: block; flex: 0 0 68rpx; width: 68rpx; height: 68rpx; margin: 0; padding: 0; border: 0; overflow: hidden; border-radius: 50%; background: transparent; line-height: 0; }
.content-summary__avatar-action::after { border: 0; }
.content-summary__avatar { display: block; flex: 0 0 68rpx; width: 68rpx; height: 68rpx; object-fit: cover; background: #EDE6D9; border-radius: 50%; }
.content-summary__avatar--fallback { display: flex; align-items: center; justify-content: center; color: #7C664C; font-family: Georgia, "Songti SC", serif; font-size: 30rpx; }
.content-summary__teacher-copy { display: flex; flex: 1; flex-direction: column; gap: 2rpx; }
.detail-head__teacher { color: var(--nx-text); font-size: 26rpx; font-weight: 600; }
.content-summary__teacher-label { color: var(--nx-text-muted); font-size: 20rpx; }
.content-summary__teacher-link { color: var(--nx-text-muted); font-size: 22rpx; }
.content-summary__arrow { color: #A39886; font-size: 36rpx; }
.teacher-link--pressed { opacity: .65; }
.player-panel { overflow: hidden; background: var(--nx-surface); }
.player-panel__head { display: flex; align-items: center; justify-content: space-between; gap: 20rpx; padding: 18rpx 28rpx; color: var(--nx-text); background: var(--nx-surface); }
.player-panel__eyebrow { font-size: 22rpx; font-weight: 600; letter-spacing: 2rpx; }
.player-panel__badge { padding: 4rpx 12rpx; color: #8C6545; font-size: 20rpx; background: #F4EFE6; border-radius: 6rpx; }
.player-panel__body { overflow: hidden; background: #282A27; }
.player-panel__retry { width: auto; margin: 0; padding: 24rpx; }
.video-player { display: block; width: 100%; height: 386rpx; background: #282A27; }
.video-player.classroom-cover--9x16 { height: 720rpx; max-height: 64vh; }
.video-player.classroom-cover--1x1 { height: 620rpx; max-height: 64vh; }
.audio-player { display: flex; flex-direction: column; align-items: center; padding: 42rpx 32rpx 32rpx; background: #F1EDE3; }
.audio-player__disc { display: flex; align-items: center; justify-content: center; width: 156rpx; height: 156rpx; color: #E7D9BA; font-family: Georgia, "Songti SC", serif; background: #3D443B; border: 14rpx solid #D9D2C1; border-radius: 50%; box-sizing: border-box; }
.audio-player__disc--playing { box-shadow: 0 0 0 10rpx rgba(165, 92, 59, .10); }
.audio-player__disc-label { font-size: 40rpx; }
.audio-player__title { margin-top: 30rpx; color: var(--nx-text); font-size: 24rpx; line-height: 1.55; text-align: center; }
.audio-player__slider { width: 100%; margin-top: 28rpx; }
.audio-player__time { display: flex; justify-content: space-between; width: 100%; color: var(--nx-text-muted); font-size: 21rpx; }
.detail-actions--audio { margin-top: 24rpx; }
.access-panel, .description-panel, .progress-panel, .payment-panel { padding: 30rpx; background: var(--nx-surface); border: 1rpx solid var(--nx-border); border-radius: 24rpx; box-shadow: none; }
.access-panel__title, .access-panel__copy, .access-panel__error, .description-panel__title, .description-panel__copy, .progress-panel__title, .progress-panel__copy, .progress-panel__error, .payment-panel__title, .payment-panel__copy { display: block; }
.access-panel__title, .description-panel__title, .progress-panel__title, .payment-panel__title { color: var(--nx-text); font-size: 29rpx; font-weight: 600; line-height: 1.5; }
.access-panel__title, .payment-panel__title { margin-top: 10rpx; }
.access-panel__copy, .description-panel__copy, .progress-panel__copy, .payment-panel__copy { margin-top: 20rpx; color: var(--nx-text-muted); font-size: 25rpx; line-height: 1.9; }
.description-panel__copy { color: #686B64; }
.access-panel__error, .progress-panel__error { margin-top: 14rpx; color: #A54937; font-size: 23rpx; line-height: 1.6; }
.section-heading, .progress-panel__head { display: flex; align-items: center; justify-content: space-between; gap: 20rpx; }
.progress-panel__value { color: #A55C3B; font-size: 25rpx; font-weight: 600; }
.progress-panel__bar { height: 8rpx; margin-top: 24rpx; overflow: hidden; background: #EDE8DE; border-radius: 999rpx; }
.progress-panel__fill { height: 100%; background: #A55C3B; border-radius: inherit; }
.progress-panel__copy { margin-top: 14rpx; font-size: 22rpx; line-height: 1.65; }
.detail-consult { display: flex; align-items: center; flex-wrap: wrap; gap: 20rpx; padding: 16rpx 0; }
.detail-consult__copy { flex: 1; min-width: 280rpx; }
.detail-consult__title { display: block; color: var(--nx-text); font-size: 27rpx; font-weight: 600; }
.detail-consult__lead { display: block; margin-top: 8rpx; color: var(--nx-text-muted); font-size: 22rpx; line-height: 1.6; }
.detail-consult__action { display: flex; align-items: center; justify-content: center; gap: 10rpx; min-width: 196rpx; min-height: 88rpx; margin: 0; padding: 0 22rpx; color: #A55C3B; background: #F0E8DC; font-size: 24rpx; line-height: 88rpx; border-radius: 16rpx; }
</style>
