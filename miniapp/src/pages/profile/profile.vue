<script setup>
import { computed, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { TYPES_INFO } from '../../data/enneagramGame'
import { ensureLogin, getToken, clearToken } from '../../utils/auth'
import { hiddenCount, previewItems } from '../../utils/listPreview'
import { clearBookingSession } from '../../utils/bookingSession'
import { userErrorMessage } from '../../utils/userMessage'
import { getUserInfoApi, listTestRecordsApi, listBookingsApi } from '../../api'

const logged = ref(false)
const user = ref(null)
const records = ref([])
const bookings = ref([])
const recordsError = ref('')
const bookingsError = ref('')
const logging = ref(false)
const profileLoading = ref(false)
const userAvatarFailed = ref(false)
const visibleRecords = computed(() => previewItems(records.value))
const hiddenRecordCount = computed(() => hiddenCount(records.value))
const latestBooking = computed(() => bookings.value[0] || null)
const recordCount = computed(() => records.value.length)
const bookingCount = computed(() => bookings.value.length)
const recordCountLabel = computed(() => profileLoading.value || recordsError.value ? '—' : String(recordCount.value))
const bookingCountLabel = computed(() => profileLoading.value || bookingsError.value ? '—' : String(bookingCount.value))
let loadTicket = 0
let sessionGeneration = 0

onShow(() => {
  logged.value = !!getToken()
  if (logged.value) loadAll()
})

async function login() {
  if (logging.value) return
  let generation = sessionGeneration
  logging.value = true
  try {
    await ensureLogin()
    sessionGeneration += 1
    generation = sessionGeneration
    logged.value = true
    await loadAll()
    if (!logged.value || generation !== sessionGeneration) return
    uni.showToast({ title: '登录成功', icon: 'success' })
  } catch (e) {
    if (generation !== sessionGeneration) return
    uni.showToast({ title: userErrorMessage(e, '登录失败'), icon: 'none' })
  } finally {
    if (generation === sessionGeneration) logging.value = false
  }
}

function isAuthError(error) {
  const statusCode = Number(error?.statusCode)
  return Boolean(error?.authExpired || error?.authRequired || statusCode === 401 || statusCode === 403)
}

function isCurrentProfileLoad(ticket, token, error) {
  if (ticket !== loadTicket) return false
  const currentToken = getToken()
  if (token === currentToken) return true
  return Boolean(
    !currentToken
    && error?.authExpired
    && error.requestToken === token,
  )
}

function invalidateStaleProfileLoad(ticket = loadTicket) {
  if (ticket !== loadTicket) return
  sessionGeneration += 1
  loadTicket += 1
  user.value = null
  records.value = []
  bookings.value = []
  recordsError.value = ''
  bookingsError.value = ''
  userAvatarFailed.value = false
  profileLoading.value = false
  logging.value = false
  clearBookingSession()
}

function handleAuthLoss(ticket = loadTicket) {
  if (ticket !== loadTicket) return
  resetLogin()
  uni.showToast({ title: '登录已过期，请重新登录', icon: 'none' })
}

async function loadAll() {
  const requestToken = getToken()
  const ticket = ++loadTicket
  if (!requestToken) {
    handleAuthLoss(ticket)
    return
  }

  profileLoading.value = true
  recordsError.value = ''
  bookingsError.value = ''
  try {
    const loadedUser = await getUserInfoApi()
    if (!isCurrentProfileLoad(ticket, requestToken)) {
      invalidateStaleProfileLoad(ticket)
      return
    }
    user.value = loadedUser
    userAvatarFailed.value = false
  } catch (e) {
    if (!isCurrentProfileLoad(ticket, requestToken, e)) {
      invalidateStaleProfileLoad(ticket)
      return
    }
    if (isAuthError(e)) {
      handleAuthLoss(ticket)
      return
    }
    const message = userErrorMessage(e, '同步失败，重试')
    recordsError.value = message
    bookingsError.value = message
    profileLoading.value = false
    return
  }

  const [rec, bk] = await Promise.allSettled([
    listTestRecordsApi(),
    listBookingsApi(),
  ])

  const historyAuthError = [rec, bk]
    .find((result) => result.status === 'rejected' && isAuthError(result.reason))
    ?.reason
  if (!isCurrentProfileLoad(ticket, requestToken, historyAuthError)) {
    invalidateStaleProfileLoad(ticket)
    return
  }
  if (historyAuthError) {
    handleAuthLoss(ticket)
    return
  }
  if (rec.status === 'fulfilled') {
    records.value = rec.value.items || []
  } else {
    recordsError.value = userErrorMessage(rec.reason, '同步失败，重试')
  }
  if (bk.status === 'fulfilled') {
    bookings.value = bk.value.items || []
  } else {
    bookingsError.value = userErrorMessage(bk.reason, '同步失败，重试')
  }
  if (isCurrentProfileLoad(ticket, requestToken)) profileLoading.value = false
}

function typeName(id) {
  return TYPES_INFO[id] ? `${id} 号 · ${TYPES_INFO[id].name}` : '—'
}

function resetLogin() {
  sessionGeneration += 1
  loadTicket += 1
  clearToken()
  clearBookingSession()
  logged.value = false
  user.value = null
  records.value = []
  bookings.value = []
  recordsError.value = ''
  bookingsError.value = ''
  userAvatarFailed.value = false
  profileLoading.value = false
  logging.value = false
}

function logout() {
  resetLogin()
}

function onUserAvatarError() {
  userAvatarFailed.value = true
}

function openProfileEdit() {
  uni.navigateTo({ url: '/pages/profile-edit/profile-edit' })
}

function openBookingRecords() {
  uni.navigateTo({ url: '/pages/booking-records/booking-records' })
}
</script>

<template>
  <view class="wrap profile page-stack ios-page ios-safe-bottom">
    <view v-if="!logged" class="profile-hero nx-page-hero login">
      <view class="profile-hero__mark">九</view>
      <text class="profile-hero__eyebrow">个人档案</text>
      <text class="profile-hero__title">记录每一次自我看见</text>
      <text class="profile-hero__lead">登录后沉淀你的九型档案、测试历史和预约记录。</text>
      <!-- #ifdef H5 -->
      <button class="profile-login ios-button" disabled>请在微信小程序内登录</button>
      <text class="login__hint">H5 可浏览公开内容；保存档案和预约记录请打开微信小程序。</text>
      <!-- #endif -->
      <!-- #ifndef H5 -->
      <button class="profile-login ios-button" :loading="logging" :disabled="logging" @click="login">微信一键登录</button>
      <!-- #endif -->
    </view>

    <template v-else>
      <view class="profile-hero nx-page-hero user">
        <view
          class="profile-hero__identity-action"
          role="button"
          aria-role="button"
          aria-label="编辑个人资料"
          tabindex="0"
          hover-class="profile-hero__identity-action--pressed"
          @click="openProfileEdit"
          @keydown.enter="openProfileEdit"
          @keydown.space.prevent="openProfileEdit"
        >
          <image v-if="user && user.avatar && !userAvatarFailed" class="user__avatar" :src="user.avatar" mode="aspectFill" lazy-load @error="onUserAvatarError" />
          <view v-else class="user__avatar user__avatar--ph">{{ (user && user.mainType) || '九' }}</view>
          <view class="user__info">
            <text class="profile-hero__eyebrow">个人档案</text>
            <text class="user__name">{{ (user && user.nickname) || '九型用户' }}</text>
            <text class="user__type" v-if="user && user.mainType">{{ typeName(user.mainType) }}</text>
            <text class="user__type" v-else>已通过微信登录</text>
          </view>
          <text class="profile-hero__identity-arrow" aria-hidden="true">›</text>
        </view>
        <text class="profile-hero__title">记录每一次自我看见</text>
        <text class="profile-hero__lead">你的成长轨迹，正在每一次探索中变得更清晰。</text>
        <view class="profile-stats">
          <view class="profile-stat">
            <text class="profile-stat__value">{{ user && user.mainType ? `${user.mainType}号` : '—' }}</text>
            <text class="profile-stat__label">主型</text>
          </view>
          <view class="profile-stat">
            <text class="profile-stat__value">{{ recordCountLabel }}</text>
            <text class="profile-stat__label">测试</text>
          </view>
          <view class="profile-stat">
            <text class="profile-stat__value">{{ bookingCountLabel }}</text>
            <text class="profile-stat__label">预约</text>
          </view>
        </view>
      </view>

      <view class="history-section nx-panel ios-card">
        <view class="section-head">
          <view>
            <text class="section-kicker">自我探索</text>
            <text class="sec-title">我的测试历史</text>
          </view>
          <text class="section-count">{{ recordCountLabel }}</text>
        </view>
        <view v-if="profileLoading" class="empty">正在同步测试历史…</view>
        <view v-else-if="recordsError" class="empty empty--error">
          <text>{{ recordsError }}</text>
          <button class="sync-retry" @click="loadAll">重试</button>
        </view>
        <view v-else-if="records.length === 0" class="empty">还没有记录，去测一测吧</view>
        <view v-else class="history-timeline">
          <view v-for="rec in visibleRecords" :key="rec.id" class="history-item">
            <view class="history-item__rail"><view class="history-item__dot" /></view>
            <view class="history-item__body">
              <text class="history-item__main">{{ typeName(rec.resultType) }}</text>
              <text class="history-item__meta">{{ rec.createTime }}</text>
            </view>
          </view>
          <text v-if="hiddenRecordCount" class="more-tip">还有 {{ hiddenRecordCount }} 条记录已收起</text>
        </view>
      </view>

      <view class="booking-summary nx-panel ios-card">
        <view class="section-head">
          <view>
            <text class="section-kicker">持续陪伴</text>
            <text class="sec-title">我的预约</text>
          </view>
          <text class="section-count">{{ bookingCountLabel }}</text>
        </view>
        <view
          class="booking-summary__open"
          role="button"
          aria-role="button"
          aria-label="查看全部预约记录"
          tabindex="0"
          hover-class="booking-summary__open--pressed"
          @click="openBookingRecords"
          @keydown.enter="openBookingRecords"
          @keydown.space.prevent="openBookingRecords"
        >
          <view class="booking-summary__status" aria-live="polite">
            <view v-if="profileLoading" class="booking-summary__state">
              <text class="booking-summary__main">正在同步预约记录…</text>
              <text class="booking-summary__meta">进入列表查看完整安排</text>
            </view>
            <view v-else-if="bookingsError" class="booking-summary__state">
              <text class="booking-summary__main">预约记录暂时无法同步</text>
              <text class="booking-summary__meta">{{ bookingsError }}</text>
            </view>
            <view v-else-if="!latestBooking" class="booking-summary__state">
              <text class="booking-summary__main">暂无预约</text>
              <text class="booking-summary__meta">进入列表页可以去提交新预约</text>
            </view>
            <view v-else class="booking-summary__state">
              <text class="booking-summary__main">{{ latestBooking.intent || latestBooking.kind }}</text>
              <text class="booking-summary__meta">{{ latestBooking.status }} · {{ latestBooking.createTime }}</text>
            </view>
          </view>
          <view class="booking-summary__footer">
            <text>查看全部预约</text>
            <text class="booking-summary__arrow" aria-hidden="true">›</text>
          </view>
        </view>
        <button v-if="bookingsError" class="booking-summary__retry ios-button" tabindex="0" @click.stop="loadAll">重试</button>
      </view>

      <button class="logout ios-button" @click="logout">退出登录</button>
    </template>
  </view>
</template>

<style scoped>
.profile { gap: 24rpx; overflow-x: hidden; }
.profile-hero { box-sizing: border-box; width: 100%; padding: 36rpx; border-radius: 38rpx; background: linear-gradient(145deg, #172554, #4338ca 56%, #7c3aed); color: #ffffff; box-shadow: 0 26rpx 54rpx -34rpx rgba(49, 46, 129, .78); overflow: hidden; }
.login { display: flex; flex-direction: column; align-items: center; text-align: center; gap: 18rpx; padding-top: 54rpx; padding-bottom: 50rpx; }
.profile-hero__mark { width: 116rpx; height: 116rpx; border-radius: 34rpx; background: rgba(255, 255, 255, .16); border: 2rpx solid rgba(255, 255, 255, .32); color: #ffffff; font-size: 50rpx; font-weight: 900; display: flex; align-items: center; justify-content: center; }
.profile-hero__eyebrow { color: #ddd6fe; font-size: 24rpx; font-weight: 800; line-height: 1.45; }
.profile-hero__title { display: block; margin-top: 10rpx; color: #ffffff; font-size: 42rpx; font-weight: 900; line-height: 1.25; }
.profile-hero__lead { display: block; margin-top: 14rpx; color: #ede9fe; font-size: 25rpx; line-height: 1.65; }
.profile-login { width: 100%; min-height: 88rpx; margin-top: 16rpx; border-radius: 24rpx; background: #ffffff; color: #312e81; font-size: 28rpx; font-weight: 900; }
.profile-login::after { border: none; }
.login__hint { color: #ddd6fe; font-size: 24rpx; line-height: 1.55; }
.profile-hero__identity-action { min-height: 88rpx; display: flex; align-items: center; gap: 22rpx; border-radius: 24rpx; cursor: pointer; }
.profile-hero__identity-action--pressed { opacity: .76; transform: scale(.992); }
.profile-hero__identity-action:focus-visible { outline: 4rpx solid rgba(255, 255, 255, .82); outline-offset: 6rpx; }
.profile-hero__identity-arrow { flex: none; color: #ddd6fe; font-size: 40rpx; line-height: 1; }
.profile-stats { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12rpx; margin-top: 30rpx; }
.profile-stat { min-width: 0; padding: 20rpx 10rpx; border-radius: 22rpx; background: rgba(255, 255, 255, .14); border: 2rpx solid rgba(255, 255, 255, .16); text-align: center; }
.profile-stat__value { display: block; color: #ffffff; font-size: 34rpx; font-weight: 900; line-height: 1.2; font-variant-numeric: tabular-nums; }
.profile-stat__label { display: block; margin-top: 8rpx; color: #ddd6fe; font-size: 24rpx; line-height: 1.35; }
.user__info { flex: 1; min-width: 0; }
.user__avatar { width: 116rpx; height: 116rpx; flex: 0 0 116rpx; border-radius: 50%; border: 4rpx solid rgba(255, 255, 255, .72); box-sizing: border-box; box-shadow: 0 16rpx 34rpx -26rpx rgba(15, 23, 42, .62); }
.user__avatar--ph { background: rgba(255, 255, 255, .16); color: #ffffff; font-size: 44rpx; font-weight: 900; display: flex; align-items: center; justify-content: center; }
.user__name { color: #ffffff; font-size: 35rpx; font-weight: 900; display: block; line-height: 1.28; }
.user__type { color: #ddd6fe; font-size: 25rpx; display: block; margin-top: 7rpx; }
.history-section, .booking-summary { box-sizing: border-box; width: 100%; padding: 30rpx; }
.section-kicker { display: block; margin-bottom: 6rpx; color: #4f46e5; font-size: 24rpx; font-weight: 800; line-height: 1.35; }
.sec-title { display: block; color: #0f172a; font-size: 32rpx; font-weight: 900; line-height: 1.3; }
.section-head { display: flex; align-items: center; justify-content: space-between; gap: 20rpx; margin-bottom: 22rpx; }
.section-count { min-width: 64rpx; color: #4338ca; font-size: 30rpx; font-weight: 900; text-align: right; font-variant-numeric: tabular-nums; }
.empty { color: #475569; font-size: 25rpx; padding: 28rpx 20rpx; text-align: center; border-radius: 22rpx; background: #f8fafc; }
.empty--error { display: flex; align-items: center; justify-content: space-between; gap: 18rpx; padding: 18rpx 20rpx; text-align: left; }
.sync-retry { flex-shrink: 0; min-width: 112rpx; min-height: 88rpx; padding: 0 20rpx; border-radius: 999rpx; background: #eef2ff; color: #3730a3; font-size: 24rpx; font-weight: 900; display: flex; align-items: center; justify-content: center; }
.sync-retry::after { border: none; }
.history-timeline { width: 100%; }
.history-item { display: flex; align-items: stretch; gap: 20rpx; min-height: 88rpx; border-bottom: 2rpx solid #e2e8f0; }
.history-item:last-of-type { border-bottom: none; }
.history-item__rail { width: 20rpx; flex: 0 0 20rpx; display: flex; justify-content: center; padding-top: 30rpx; }
.history-item__dot { width: 16rpx; height: 16rpx; border-radius: 50%; background: #6366f1; box-shadow: 0 0 0 7rpx #eef2ff; }
.history-item__body { min-width: 0; flex: 1; display: flex; flex-direction: column; justify-content: center; padding: 18rpx 0; }
.history-item__main { color: #0f172a; font-size: 28rpx; font-weight: 900; line-height: 1.35; }
.history-item__meta { display: block; margin-top: 7rpx; color: #475569; font-size: 24rpx; line-height: 1.45; }
.more-tip { display: block; margin-top: 16rpx; color: #475569; font-size: 24rpx; line-height: 1.5; }
.booking-summary__open { min-height: 88rpx; padding: 24rpx; border-radius: 24rpx; background: #f8fafc; box-sizing: border-box; cursor: pointer; }
.booking-summary__open--pressed { opacity: .76; transform: scale(.992); }
.booking-summary__open:focus-visible { outline: 4rpx solid rgba(79, 70, 229, .62); outline-offset: 4rpx; }
.booking-summary__state { min-height: 90rpx; display: flex; flex-direction: column; justify-content: center; }
.booking-summary__main { color: #0f172a; font-size: 28rpx; font-weight: 900; line-height: 1.4; }
.booking-summary__meta { display: block; margin-top: 8rpx; color: #475569; font-size: 24rpx; line-height: 1.5; }
.booking-summary__footer { min-height: 56rpx; margin-top: 20rpx; padding-top: 18rpx; border-top: 2rpx solid #e2e8f0; color: #4338ca; font-size: 24rpx; font-weight: 900; display: flex; align-items: center; justify-content: space-between; gap: 20rpx; }
.booking-summary__arrow { font-size: 38rpx; line-height: 1; }
.booking-summary__retry { width: 100%; min-height: 88rpx; margin-top: 16rpx; border-radius: 22rpx; background: #eef2ff; color: #3730a3; font-size: 24rpx; font-weight: 900; }
.booking-summary__retry::after { border: none; }
.logout { width: 100%; min-height: 88rpx; border-radius: 24rpx; background: transparent; border: 2rpx solid #fecaca; color: #b91c1c; font-size: 26rpx; font-weight: 800; }
.logout::after { border: none; }
@media (max-width: 360px) {
  .profile-hero, .history-section, .booking-summary { padding-left: 26rpx; padding-right: 26rpx; }
}
</style>
