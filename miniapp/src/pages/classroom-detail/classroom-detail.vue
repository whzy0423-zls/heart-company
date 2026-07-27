<script setup>
import { computed, ref } from 'vue'
import { onLoad, onUnload } from '@dcloudio/uni-app'
import { getClassroomContentApi, withClassroomPlaybackRetry } from '../../api'
import {
  classroomAccessLabel,
  classroomPurchaseAction,
  normalizeClassroomContent,
} from '../../utils/classroomDisplay'
import { userErrorMessage } from '../../utils/userMessage'

const contentId = ref('')
const content = ref(normalizeClassroomContent())
const loading = ref(true)
const loadError = ref('')
const playbackUrl = ref('')
const playbackLoading = ref(false)
const playbackError = ref('')
const audioPlaying = ref(false)
const audioPosition = ref(0)
const audioDuration = ref(0)
let detailTicket = 0
let playbackTicket = 0
let audioContext = null
let playbackRecoveryUsed = false

const accessAction = computed(() => classroomPurchaseAction(content.value))

function validPlaybackUrl(value) {
  const url = String(value || '').trim()
  return /^https:\/\//i.test(url) ? url : ''
}

function destroyAudio() {
  if (!audioContext) return
  audioContext.stop()
  audioContext.destroy()
  audioContext = null
  audioPlaying.value = false
}

function prepareAudio(url) {
  destroyAudio()
  audioContext = uni.createInnerAudioContext()
  audioContext.autoplay = false
  audioContext.src = url
  audioContext.onPlay(() => { audioPlaying.value = true })
  audioContext.onPause(() => { audioPlaying.value = false })
  audioContext.onStop(() => { audioPlaying.value = false })
  audioContext.onEnded(() => {
    audioPlaying.value = false
    audioPosition.value = audioDuration.value
  })
  audioContext.onCanplay(() => {
    const duration = Number(audioContext?.duration)
    if (Number.isFinite(duration) && duration > 0) audioDuration.value = Math.floor(duration)
  })
  audioContext.onTimeUpdate(() => {
    audioPosition.value = Math.max(0, Math.floor(Number(audioContext?.currentTime) || 0))
    const duration = Number(audioContext?.duration)
    if (Number.isFinite(duration) && duration > 0) audioDuration.value = Math.floor(duration)
  })
  audioContext.onError(handlePlaybackError)
}

async function refreshPlayback({ recovery = false } = {}) {
  if (!contentId.value || !content.value.canPlay || playbackLoading.value) return
  if (!recovery) playbackRecoveryUsed = false
  const ticket = ++playbackTicket
  playbackLoading.value = true
  playbackError.value = ''
  playbackUrl.value = ''
  destroyAudio()
  try {
    await withClassroomPlaybackRetry(contentId.value, async (playback) => {
      const url = validPlaybackUrl(playback?.url)
      if (!url) throw new Error('播放地址无效')
      if (ticket !== playbackTicket) return
      playbackUrl.value = url
      if (content.value.contentType === 'audio') prepareAudio(url)
    })
  } catch (error) {
    if (ticket === playbackTicket) playbackError.value = userErrorMessage(error, '播放地址加载失败，请重试')
  } finally {
    if (ticket === playbackTicket) playbackLoading.value = false
  }
}

async function loadDetail() {
  if (!contentId.value) {
    loading.value = false
    loadError.value = '课件参数无效'
    return
  }
  const ticket = ++detailTicket
  loading.value = true
  loadError.value = ''
  playbackError.value = ''
  playbackUrl.value = ''
  destroyAudio()
  try {
    const response = await getClassroomContentApi(contentId.value)
    if (ticket !== detailTicket) return
    const normalized = normalizeClassroomContent(response)
    if (!normalized.id) throw new Error('课件内容不存在')
    content.value = normalized
    if (normalized.canPlay) await refreshPlayback()
  } catch (error) {
    if (ticket === detailTicket) loadError.value = userErrorMessage(error, '课件详情加载失败，请重试')
  } finally {
    if (ticket === detailTicket) loading.value = false
  }
}

function handlePlaybackError() {
  playbackUrl.value = ''
  destroyAudio()
  if (!playbackRecoveryUsed) {
    playbackRecoveryUsed = true
    playbackError.value = '播放凭证已失效，正在刷新…'
    refreshPlayback({ recovery: true })
    return
  }
  playbackError.value = '播放凭证可能已过期，请刷新后重试'
}

function toggleAudio() {
  if (!audioContext || !playbackUrl.value) return
  if (audioPlaying.value) audioContext.pause()
  else audioContext.play()
}

function seekAudio(event) {
  if (!audioContext) return
  const seconds = Math.max(0, Number(event?.detail?.value) || 0)
  audioContext.seek(seconds)
  audioPosition.value = Math.floor(seconds)
}

function formatTime(seconds) {
  const value = Math.max(0, Math.floor(Number(seconds) || 0))
  return `${Math.floor(value / 60)}:${String(value % 60).padStart(2, '0')}`
}

function handleAccessAction() {
  if (accessAction.value.type === 'login' || accessAction.value.type === 'member') {
    uni.switchTab({ url: '/pages/profile/profile' })
    return
  }
  if (accessAction.value.type === 'purchase') {
    uni.showToast({ title: '购买功能准备中', icon: 'none' })
  }
}

onLoad((options = {}) => {
  contentId.value = String(options.id || '').trim()
  loadDetail()
})

onUnload(() => {
  detailTicket += 1
  playbackTicket += 1
  playbackUrl.value = ''
  destroyAudio()
})
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
        <image v-if="content.coverUrl" class="detail-head__cover" :src="content.coverUrl" mode="aspectFill" lazy-load />
        <view v-else class="detail-head__cover detail-head__cover--fallback" aria-hidden="true">{{ content.contentType === 'audio' ? '音' : '课' }}</view>
        <view class="detail-head__body">
          <view class="detail-head__meta">
            <text class="nx-tag">{{ classroomAccessLabel(content.effectiveAccess) }}</text>
            <text>{{ content.contentType === 'audio' ? '音频课件' : '视频课件' }}</text>
          </view>
          <text class="detail-head__title">{{ content.title }}</text>
          <text class="detail-head__teacher">{{ content.teacherName || '九型老师' }}</text>
        </view>
      </view>

      <view v-if="!content.canPlay" class="access-panel ios-card" aria-live="polite">
        <text class="access-panel__title">{{ accessAction.label }}</text>
        <text class="access-panel__copy">完成对应权限后，即可播放本课件。</text>
        <button
          v-if="accessAction.type !== 'blocked' && accessAction.type !== 'unavailable'"
          class="primary-action"
          @click="handleAccessAction"
        >{{ accessAction.label }}</button>
      </view>

      <view v-else class="player-panel ios-card">
        <view v-if="playbackLoading" class="detail-state" aria-live="polite">正在获取安全播放地址…</view>
        <view v-else-if="playbackError" class="detail-state detail-state--error" aria-live="polite">
          <text>{{ playbackError }}</text>
          <button class="detail-action" :disabled="playbackLoading" @click="refreshPlayback">刷新播放地址</button>
        </view>
        <block v-else-if="playbackUrl">
          <video
            v-if="content.contentType === 'video'"
            class="video-player"
            :src="playbackUrl"
            controls
            object-fit="contain"
            @error="handlePlaybackError"
          />
          <view v-else class="audio-player">
          <view class="audio-player__disc" :class="{ 'audio-player__disc--playing': audioPlaying }" aria-hidden="true">♫</view>
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
          <button class="primary-action" :aria-label="audioPlaying ? '暂停音频' : '播放音频'" @click="toggleAudio">
            {{ audioPlaying ? '暂停音频' : '播放音频' }}
          </button>
          </view>
        </block>
        <button v-else class="detail-action" @click="refreshPlayback">加载播放内容</button>
      </view>

      <view class="description-panel ios-card">
        <text class="description-panel__title">课件介绍</text>
        <text class="description-panel__copy">{{ content.description || '老师正在完善本课件介绍。' }}</text>
      </view>
    </block>
  </view>
</template>

<style scoped>
.classroom-detail { display: flex; flex-direction: column; gap: 24rpx; min-height: 100vh; padding: 28rpx; background: #f4f8f6; box-sizing: border-box; }
.detail-state { padding: 48rpx 30rpx; color: #64756e; font-size: 27rpx; line-height: 1.6; text-align: center; background: #fff; border-radius: 28rpx; }
.detail-state--error { color: #9f3a38; }
.detail-action, .primary-action { min-height: 88rpx; margin-top: 22rpx; padding: 0 32rpx; font-size: 27rpx; font-weight: 800; line-height: 88rpx; border-radius: 20rpx; }
.detail-action { color: #0f6b4f; background: #ecfdf5; }
.primary-action { width: 100%; color: #fff; background: #0f766e; }
.detail-action::after, .primary-action::after { border: 0; }
.detail-head { display: flex; overflow: hidden; background: #fff; border-radius: 30rpx; }
.detail-head__cover { width: 240rpx; min-height: 240rpx; background: #dbeee6; }
.detail-head__cover--fallback { display: flex; align-items: center; justify-content: center; color: #0f766e; font-size: 58rpx; font-weight: 900; }
.detail-head__body { display: flex; flex: 1; flex-direction: column; padding: 28rpx 24rpx; }
.detail-head__meta { display: flex; align-items: center; justify-content: space-between; color: #6b7b73; font-size: 22rpx; }
.detail-head__title { margin-top: 22rpx; color: #17241e; font-size: 34rpx; font-weight: 900; line-height: 1.4; }
.detail-head__teacher { margin-top: auto; padding-top: 18rpx; color: #617169; font-size: 24rpx; }
.access-panel, .player-panel, .description-panel { padding: 30rpx; background: #fff; border-radius: 30rpx; }
.access-panel__title, .access-panel__copy, .description-panel__title, .description-panel__copy { display: block; }
.access-panel__title, .description-panel__title { color: #17241e; font-size: 31rpx; font-weight: 900; }
.access-panel__copy, .description-panel__copy { margin-top: 14rpx; color: #68776f; font-size: 25rpx; line-height: 1.7; }
.video-player { width: 100%; min-height: 390rpx; background: #0b1511; border-radius: 24rpx; }
.audio-player { display: flex; flex-direction: column; align-items: center; padding: 24rpx 6rpx 4rpx; }
.audio-player__disc { display: flex; align-items: center; justify-content: center; width: 160rpx; height: 160rpx; color: #fff; font-size: 58rpx; background: linear-gradient(135deg, #0f766e, #22c55e); border: 16rpx solid #d9f3e7; border-radius: 50%; }
.audio-player__disc--playing { box-shadow: 0 0 0 12rpx rgba(34, 197, 94, .12); }
.audio-player__title { margin-top: 26rpx; color: #17241e; font-size: 29rpx; font-weight: 900; text-align: center; }
.audio-player__slider { width: 100%; margin-top: 24rpx; }
.audio-player__time { display: flex; justify-content: space-between; width: 100%; color: #718078; font-size: 22rpx; }
</style>
