<script setup>
import { computed, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { TYPES_INFO } from '../../data/enneagramGame'
import { ensureLogin, getToken, clearToken } from '../../utils/auth'
import { hiddenCount, previewItems } from '../../utils/listPreview'
import { clearBookingSession } from '../../utils/bookingSession'
import { userErrorMessage } from '../../utils/userMessage'
import { createWechatPayTestOrderApi, getUserInfoApi, listTestRecordsApi, listBookingsApi } from '../../api'
import { requestWechatPayment } from '../../utils/payment'
import { normalizeMiniappPayment } from '../../utils/miniappPages'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import { previewImage } from '../../utils/imagePreview'
import { bookingKindLabel, bookingStatusLabel } from '../../utils/bookingDisplay'
import NxIcon from '../../components/NxIcon.vue'
import NxStudioHeader from '../../components/NxStudioHeader.vue'
import { UI_PREVIEW } from '../../utils/uiPreview'
import { useStudioNavigation } from '../../utils/studioNavigation'

// Keep session tests independent from the SFC-only navigation import while
// preserving the shared fixed header in the compiled app.
const studioNavigation = typeof useStudioNavigation === 'function'
  ? useStudioNavigation()
  : { pageStyle: computed(() => ({})), refreshNavigation: () => {} }
const { pageStyle, refreshNavigation } = studioNavigation

const logged = ref(false)
const user = ref(null)
const records = ref([])
const bookings = ref([])
const recordsError = ref('')
const bookingsError = ref('')
const logging = ref(false)
const paymentTesting = ref(false)
const paymentEnabled = ref(normalizeMiniappPayment(getStoredSiteConfig()).enabled)
const profileLoading = ref(false)
const userAvatarFailed = ref(false)
const historyExpanded = ref(false)
const visibleRecords = computed(() => historyExpanded.value ? records.value : previewItems(records.value))
const hiddenRecordCount = computed(() => hiddenCount(records.value))
const latestBooking = computed(() => bookings.value[0] || null)
const recordCount = computed(() => records.value.length)
const bookingCount = computed(() => bookings.value.length)
const recordCountLabel = computed(() => profileLoading.value || recordsError.value ? '—' : String(recordCount.value))
const bookingCountLabel = computed(() => profileLoading.value || bookingsError.value ? '—' : String(bookingCount.value))
let loadTicket = 0
let sessionGeneration = 0
let paymentRefreshTicket = 0

async function refreshPaymentAvailability() {
  const ticket = ++paymentRefreshTicket
  try {
    const config = await refreshSiteConfig()
    if (ticket !== paymentRefreshTicket) return
    paymentEnabled.value = normalizeMiniappPayment(config).enabled
  } catch {
    // Keep the last known value when a background refresh is unavailable.
  }
}

onShow(() => {
  refreshNavigation()
  paymentEnabled.value = normalizeMiniappPayment(getStoredSiteConfig()).enabled
  void refreshPaymentAvailability()
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

function previewUserAvatar() {
  if (!user.value?.avatar || userAvatarFailed.value) return
  previewImage(user.value.avatar)
}

function openProfileEdit() {
  uni.navigateTo({ url: '/pages/profile-edit/profile-edit' })
}

function openBookingRecords() {
  uni.navigateTo({ url: '/pages/booking-records/booking-records' })
}

function openOrders() {
  uni.navigateTo({ url: '/pages/orders/orders' })
}

function openLearn() {
  uni.switchTab({ url: '/pages/learn/learn' })
}

function openBooking() {
  uni.switchTab({ url: '/pages/booking/booking' })
}

function openTest() {
  uni.navigateTo({ url: '/pages/test/test' })
}

async function testWechatPayment() {
  if (paymentTesting.value || !paymentEnabled.value) return
  paymentTesting.value = true
  try {
    const order = await createWechatPayTestOrderApi()
    const pay = order?.payParams || {}
    if (pay.devMode) throw new Error('当前后端是开发模拟支付环境，请切换到已配置微信支付的生产环境')
    await requestWechatPayment(pay)
    uni.showToast({ title: '支付已完成，正在确认', icon: 'success' })
  } catch (error) {
    const message = String(error?.errMsg || error?.message || '')
    uni.showToast({
      title: message.toLowerCase().includes('cancel') ? '已取消支付' : userErrorMessage(error, '支付测试失败'),
      icon: 'none',
      duration: 2800,
    })
  } finally {
    paymentTesting.value = false
  }
}
</script>

<template>
  <view class="wrap profile page-stack ios-page ios-safe-bottom">
    <NxStudioHeader />
    <view class="profile-content" :style="pageStyle">
      <view class="profile-header">
        <view class="eyebrow-row"><text class="eyebrow">MY GROWTH</text><text v-if="UI_PREVIEW" class="preview-badge">演示体验</text></view>
        <text class="profile-header__title">我的成长手记</text>
        <text class="profile-header__lead">每一次看见，都是向自己走近一点。</text>
      </view>

      <template v-if="!logged">
        <view class="login-card">
          <view class="login-card__icon"><NxIcon name="user" :size="30" color="#A55C3B" /></view>
          <text class="login-card__title">让每一步成长，都有迹可循</text>
          <text class="login-card__lead">登录后，保存你的九型画像，查看报名安排，记录与自己相遇的时刻。</text>
          <!-- #ifdef H5 -->
          <button class="profile-login" disabled>请在微信小程序内登录</button>
          <text class="login__hint">你仍可以浏览老师日常，或开始一次九型探索。</text>
          <!-- #endif -->
          <!-- #ifndef H5 -->
          <button class="profile-login" :loading="logging" :disabled="logging" @click="login">{{ logging ? '正在登录…' : '微信一键登录' }}</button>
          <!-- #endif -->
        </view>
        <view class="guest-links">
          <button class="guest-link" @click="openLearn"><view class="guest-link__icon"><NxIcon name="video" :size="23" color="#A55C3B" /></view><view class="guest-link__body"><text>先听听老韩的日常</text><text class="guest-link__desc">从一段分享，开启新的看见</text></view><NxIcon name="chevron" :size="18" color="#77786F" /></button>
          <button class="guest-link" @click="openTest"><view class="guest-link__icon"><NxIcon name="spark" :size="23" color="#A55C3B" /></view><view class="guest-link__body"><text>做一次九型探索</text><text class="guest-link__desc">认识你的关注点与内在动力</text></view><NxIcon name="chevron" :size="18" color="#77786F" /></button>
        </view>
      </template>

      <template v-else>
        <view class="identity-card">
          <view class="profile-hero__identity">
            <button v-if="user && user.avatar && !userAvatarFailed" class="user-avatar-action" aria-label="预览个人头像" @click="previewUserAvatar"><image class="user__avatar" :src="user.avatar" mode="aspectFill" @error="onUserAvatarError" /></button>
            <view v-else class="user__avatar user__avatar--ph">{{ (user && user.nickname ? user.nickname : '我').slice(0, 1) }}</view>
            <button class="identity-edit" aria-label="编辑个人资料" @click="openProfileEdit"><view class="user__info"><text class="user__name">{{ (user && user.nickname) || '九型用户' }}</text><text class="user__type">{{ user && user.mainType ? typeName(user.mainType) : '保持好奇，继续认识自己' }}</text></view><NxIcon name="chevron" :size="18" color="#77786F" /></button>
          </view>
          <view class="profile-stats">
            <view class="profile-stat"><text class="profile-stat__value">{{ user && user.mainType ? `${user.mainType}` : '—' }}<text v-if="user && user.mainType" class="profile-stat__unit">号</text></text><text class="profile-stat__label">我的主型</text></view>
            <view class="profile-stat"><text class="profile-stat__value">{{ recordCountLabel }}</text><text class="profile-stat__label">探索记录</text></view>
            <view class="profile-stat"><text class="profile-stat__value">{{ bookingCountLabel }}</text><text class="profile-stat__label">报名与预约</text></view>
          </view>
        </view>

        <view class="profile-actions" aria-label="个人快捷操作">
          <button class="profile-action" @click="openOrders"><view class="profile-action__icon"><NxIcon name="book" :size="25" color="#A55C3B" /></view><text class="profile-action__title">我的订单</text><text class="profile-action__desc">支付与课程记录</text></button>
          <button class="profile-action" @click="openBookingRecords"><view class="profile-action__icon"><NxIcon name="calendar" :size="25" color="#A55C3B" /></view><text class="profile-action__title">报名记录</text><text class="profile-action__desc">查看我的安排</text></button>
          <button class="profile-action" @click="openTest"><view class="profile-action__icon"><NxIcon name="spark" :size="25" color="#A55C3B" /></view><text class="profile-action__title">九型探索</text><text class="profile-action__desc">更懂自己的内心</text></button>
          <button class="profile-action" @click="openLearn"><view class="profile-action__icon"><NxIcon name="video" :size="25" color="#A55C3B" /></view><text class="profile-action__title">老师日常</text><text class="profile-action__desc">把看见带回生活</text></button>
        </view>

        <view class="section-heading"><text class="section-title">下一次，与成长相约</text><NxIcon name="calendar" :size="19" color="#A55C3B" /></view>
        <view class="booking-summary">
          <view v-if="profileLoading" class="empty" role="status">正在同步报名与预约记录…</view>
          <view v-else-if="bookingsError" class="empty empty--error"><text>{{ bookingsError }}</text><button class="sync-retry" @click="loadAll">重新加载</button></view>
          <view v-else-if="!latestBooking" class="booking-empty"><text class="booking-empty__title">给自己，留一段成长的时间</text><text class="booking-empty__copy">选择适合你的课程或咨询，与老师聊聊此刻的你。</text><button class="booking-cta" @click="openBooking">看看课程与咨询<NxIcon name="arrow" :size="18" color="#FFFFFF" /></button></view>
          <template v-else>
            <view class="booking-summary__eyebrow"><text>{{ bookingKindLabel(latestBooking.kind) }}</text><text class="booking-status">{{ bookingStatusLabel(latestBooking.status) }}</text></view>
            <text class="booking-summary__title">{{ latestBooking.intent || '与老师一起，探索新的可能' }}</text>
            <view class="booking-summary__meta"><NxIcon name="clock" :size="15" color="#77786F" /><text>{{ latestBooking.createTime }} 提交</text></view>
            <button class="booking-summary__open" @click="openBookingRecords"><text>查看预约详情</text><NxIcon name="arrow" :size="18" color="#A55C3B" /></button>
          </template>
        </view>

        <view class="section-heading"><text class="section-title">认识自己的足迹</text><text class="section-note">{{ recordCountLabel }} 次探索</text></view>
        <view class="history-section">
          <view v-if="profileLoading" class="empty" role="status">正在同步探索记录…</view>
          <view v-else-if="recordsError" class="empty empty--error"><text>{{ recordsError }}</text><button class="sync-retry" @click="loadAll">重新加载</button></view>
          <view v-else-if="records.length === 0" class="empty"><text>你的第一份九型画像，从这里开始。</text><button class="history-start" @click="openTest">开始九型探索<NxIcon name="arrow" :size="18" color="#A55C3B" /></button></view>
          <view v-else class="history-timeline">
            <view v-for="(rec, index) in visibleRecords" :key="rec.id" class="history-item"><view class="history-item__rail"><view class="history-item__dot" :class="{ 'history-item__dot--latest': index === 0 }" /></view><view class="history-item__body"><view class="history-item__heading"><text class="history-item__main">{{ typeName(rec.resultType) }}</text><text v-if="index === 0" class="history-latest">最近一次</text></view><text class="history-item__meta">{{ rec.createTime }}</text></view></view>
            <button v-if="hiddenRecordCount" class="history-expand" @click="historyExpanded = !historyExpanded">{{ historyExpanded ? '收起历史记录' : `查看其余 ${hiddenRecordCount} 条记录` }}<NxIcon name="chevron" :size="16" color="#A55C3B" /></button>
          </view>
        </view>

        <view class="account-links">
          <button class="account-link" @click="openProfileEdit"><NxIcon name="user" :size="20" color="#77786F" /><text>个人资料</text><NxIcon name="chevron" :size="17" color="#77786F" /></button>
          <button class="account-link" @click="openBooking"><NxIcon name="message" :size="20" color="#77786F" /><text>与老师聊一聊</text><NxIcon name="chevron" :size="17" color="#77786F" /></button>
        </view>

        <view v-if="paymentEnabled" class="wechat-pay-test">
          <view class="section-heading"><text class="section-title">微信支付体验</text><text class="section-note">¥0.10</text></view>
          <text class="wechat-pay-test__copy">点击后将发起一笔 0.10 元的微信支付测试订单。</text>
          <button class="wechat-pay-test__button" :loading="paymentTesting" :disabled="paymentTesting" @click="testWechatPayment">{{ paymentTesting ? '正在调起支付…' : '测试微信支付 ¥0.10' }}</button>
        </view>
        <button class="logout" @click="logout">退出登录</button>
      </template>
      <view class="profile-footer"><view class="profile-footer__line" /><text>成长，是一场温柔的同行</text><view class="profile-footer__line" /></view>
    </view>
  </view>
</template>

<style scoped>
.profile { padding: 0 0 calc(24rpx + env(safe-area-inset-bottom) + var(--window-bottom, 0px)); overflow-x: hidden; background: var(--nx-page-bg, #F7F5F0); color: #282A27; }
.profile-content { width: 100%; max-width: 980rpx; margin: 0 auto; padding: 36rpx 36rpx 48rpx; box-sizing: border-box; }
button { margin: 0; padding: 0; border-radius: 0; background: transparent; color: inherit; font-size: inherit; line-height: 1.5; text-align: left; box-sizing: border-box; }
button::after { border: 0; }
button:active { opacity: .74; }
button:focus-visible { outline: 3rpx solid #A55C3B; outline-offset: 5rpx; }
.profile-header { padding: 8rpx 0 38rpx; }
.eyebrow-row { display: flex; align-items: center; justify-content: space-between; gap: 16rpx; }
.eyebrow { color: #A55C3B; font-size: 21rpx; font-weight: 600; letter-spacing: 4rpx; }
.preview-badge { padding: 5rpx 12rpx; border: 1rpx solid #D9C6B9; border-radius: 8rpx; color: #8D634D; font-size: 20rpx; }
.profile-header__title { display: block; margin-top: 24rpx; font-family: 'Songti SC', 'STSong', serif; font-size: 48rpx; font-weight: 600; letter-spacing: 2rpx; line-height: 1.45; }
.profile-header__lead { display: block; margin-top: 14rpx; font-size: 26rpx; line-height: 1.7; color: #77786F; }
.identity-card { padding: 34rpx 30rpx 30rpx; border: 1rpx solid #E6E1D8; border-radius: 24rpx; background: #FFFFFF; }
.profile-hero__identity { display: flex; align-items: center; gap: 24rpx; }
.user-avatar-action, .user__avatar { width: 116rpx; height: 116rpx; flex: none; border-radius: 50%; overflow: hidden; }
.user__avatar { display: block; }
.user__avatar--ph { display: flex; align-items: center; justify-content: center; background: #EBE5DA; color: #A55C3B; font-family: 'Songti SC', 'STSong', serif; font-size: 48rpx; }
.identity-edit { display: flex; flex: 1; min-width: 0; align-items: center; gap: 14rpx; min-height: 116rpx; }
.user__info { flex: 1; min-width: 0; }
.user__name { display: block; font-size: 36rpx; font-weight: 600; line-height: 1.5; }
.user__type { display: block; margin-top: 10rpx; color: #77786F; font-size: 24rpx; }
.profile-stats { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); margin-top: 34rpx; padding-top: 26rpx; border-top: 1rpx solid #E6E1D8; }
.profile-stat { position: relative; text-align: center; padding: 4rpx; }
.profile-stat + .profile-stat::before { content: ''; position: absolute; left: 0; top: 18rpx; bottom: 18rpx; width: 1rpx; background: #E6E1D8; }
.profile-stat__value { display: block; font-family: Georgia, serif; font-size: 44rpx; line-height: 1.2; font-variant-numeric: tabular-nums; }
.profile-stat__unit { padding-left: 4rpx; font-family: sans-serif; font-size: 21rpx; }
.profile-stat__label { display: block; margin-top: 12rpx; color: #77786F; font-size: 23rpx; }
.profile-actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); margin-top: 26rpx; padding: 20rpx 0; }
.profile-action { display: flex; min-height: 152rpx; flex-direction: column; align-items: center; justify-content: center; }
.profile-action__icon { display: flex; width: 80rpx; height: 74rpx; align-items: center; justify-content: center; }
.profile-action__title { display: block; margin-top: 9rpx; font-size: 27rpx; font-weight: 500; }
.profile-action__desc { display: block; margin-top: 7rpx; color: #77786F; font-size: 21rpx; }
.section-heading { display: flex; align-items: center; justify-content: space-between; gap: 16rpx; margin: 34rpx 0 24rpx; }
.section-title { font-family: 'Songti SC', 'STSong', serif; font-size: 34rpx; font-weight: 600; line-height: 1.5; }
.section-note { flex: none; color: #77786F; font-size: 22rpx; }
.booking-summary { padding: 28rpx 30rpx 0; border: 1rpx solid #DED7CA; border-radius: 24rpx; background: #EDE9DF; }
.booking-summary__eyebrow { display: flex; align-items: center; justify-content: space-between; gap: 16rpx; color: #8D634D; font-size: 22rpx; }
.booking-status { border: 1rpx solid #C8C7B4; padding: 5rpx 12rpx; border-radius: 8rpx; color: #687357; background: #E7E8DB; font-size: 21rpx; }
.booking-summary__title { display: block; margin-top: 22rpx; font-size: 32rpx; font-weight: 500; line-height: 1.6; }
.booking-summary__meta { display: flex; align-items: center; gap: 10rpx; margin-top: 18rpx; color: #77786F; font-size: 22rpx; line-height: 1.5; }
.booking-summary__open { display: flex; width: 100%; min-height: 96rpx; align-items: center; justify-content: space-between; gap: 16rpx; margin-top: 28rpx; border-top: 1rpx solid #DCD4C5; color: #A55C3B; font-size: 25rpx; }
.booking-empty { padding-bottom: 28rpx; }
.booking-empty__title { display: block; font-size: 30rpx; line-height: 1.6; }
.booking-empty__copy { display: block; margin-top: 14rpx; color: #77786F; font-size: 25rpx; line-height: 1.8; }
.booking-cta { display: flex; align-items: center; justify-content: space-between; min-height: 88rpx; width: 100%; margin-top: 28rpx; padding: 0 24rpx; border-radius: 14rpx; background: #A55C3B; color: #FFFFFF; font-size: 26rpx; }
.history-section { padding: 12rpx 26rpx; border: 1rpx solid #E6E1D8; border-radius: 24rpx; background: #FFFFFF; }
.history-item { display: flex; gap: 22rpx; min-height: 124rpx; }
.history-item__rail { position: relative; flex: none; display: flex; width: 20rpx; align-items: center; justify-content: center; }
.history-item__rail::before { position: absolute; content: ''; top: 0; bottom: 0; width: 1rpx; background: #E6E1D8; }
.history-item:first-child .history-item__rail::before { top: 50%; }
.history-item:last-child .history-item__rail::before { bottom: 50%; }
.history-item__dot { position: relative; z-index: 1; width: 11rpx; height: 11rpx; border: 4rpx solid #FFFFFF; border-radius: 50%; background: #CAC7BE; }
.history-item__dot--latest { background: #A55C3B; }
.history-item__body { display: flex; flex: 1; min-width: 0; flex-direction: column; justify-content: center; padding: 24rpx 0; }
.history-item + .history-item .history-item__body { border-top: 1rpx solid #F0EDE7; }
.history-item__heading { display: flex; align-items: center; justify-content: space-between; gap: 12rpx; }
.history-item__main { font-size: 27rpx; line-height: 1.5; }
.history-latest { color: #A55C3B; font-size: 21rpx; }
.history-item__meta { display: block; margin-top: 9rpx; color: #77786F; font-size: 22rpx; }
.history-expand { display: flex; align-items: center; justify-content: center; width: 100%; min-height: 88rpx; gap: 12rpx; border-top: 1rpx solid #E6E1D8; color: #A55C3B; font-size: 24rpx; }
.empty { display: flex; flex-direction: column; gap: 16rpx; align-items: center; justify-content: center; min-height: 128rpx; padding: 24rpx 8rpx; color: #77786F; font-size: 25rpx; text-align: center; line-height: 1.8; }
.empty--error { color: #964A32; }
.sync-retry { min-height: 88rpx; display: flex; align-items: center; justify-content: center; padding: 0 26rpx; color: #A55C3B; }
.history-start { min-height: 88rpx; display: flex; align-items: center; gap: 16rpx; color: #A55C3B; }
.account-links { margin-top: 32rpx; padding: 0 26rpx; border: 1rpx solid #E6E1D8; border-radius: 24rpx; background: #FFFFFF; }
.account-link { display: flex; align-items: center; width: 100%; min-height: 112rpx; gap: 22rpx; font-size: 27rpx; }
.account-link + .account-link { border-top: 1rpx solid #E6E1D8; }
.account-link > text { flex: 1; }
.wechat-pay-test { padding: 0 26rpx 26rpx; margin-top: 30rpx; border: 1rpx solid #E6E1D8; border-radius: 24rpx; }
.wechat-pay-test__copy { display: block; color: #77786F; font-size: 24rpx; line-height: 1.7; }
.wechat-pay-test__button { min-height: 88rpx; display: flex; align-items: center; justify-content: center; width: 100%; margin-top: 20rpx; background: #A55C3B; border-radius: 14rpx; color: #FFFFFF; font-size: 26rpx; }
.logout { display: flex; align-items: center; justify-content: center; width: 100%; min-height: 88rpx; margin-top: 28rpx; color: #77786F; font-size: 25rpx; }
.profile-footer { display: flex; align-items: center; justify-content: center; gap: 18rpx; margin-top: 38rpx; color: #99978D; font-size: 22rpx; }
.profile-footer__line { width: 44rpx; height: 1rpx; background: #DCD6CA; }
.login-card { padding: 36rpx 30rpx; border: 1rpx solid #E6E1D8; border-radius: 24rpx; background: #FFFFFF; }
.login-card__icon { display: flex; width: 100rpx; height: 100rpx; align-items: center; justify-content: center; border-radius: 50%; background: #F3EDE4; }
.login-card__title { display: block; margin-top: 24rpx; font-family: 'Songti SC', 'STSong', serif; font-size: 36rpx; line-height: 1.6; }
.login-card__lead { display: block; margin-top: 18rpx; font-size: 26rpx; line-height: 1.8; color: #77786F; }
.profile-login { display: flex; align-items: center; justify-content: center; width: 100%; min-height: 96rpx; margin-top: 30rpx; border-radius: 16rpx; background: #A55C3B; color: #FFFFFF; font-size: 28rpx; }
.profile-login[disabled] { background: #DED8CE; color: #6A695F; }
.login__hint { display: block; margin-top: 20rpx; color: #77786F; font-size: 22rpx; line-height: 1.7; }
.guest-links { margin-top: 34rpx; padding: 0 24rpx; border: 1rpx solid #E6E1D8; border-radius: 24rpx; background: #FFFFFF; }
.guest-link { display: flex; align-items: center; gap: 22rpx; width: 100%; min-height: 148rpx; padding: 24rpx 0; }
.guest-link + .guest-link { border-top: 1rpx solid #E6E1D8; }
.guest-link__icon { flex: none; display: flex; align-items: center; justify-content: center; width: 74rpx; height: 74rpx; border-radius: 14rpx; background: #F3EDE4; }
.guest-link__body { flex: 1; min-width: 0; font-size: 27rpx; }
.guest-link__desc { display: block; margin-top: 9rpx; color: #77786F; font-size: 22rpx; }
</style>
