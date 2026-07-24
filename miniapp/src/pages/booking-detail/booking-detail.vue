<script setup>
import { ref } from 'vue'
import { onHide, onLoad, onShow, onUnload } from '@dcloudio/uni-app'
import { listBookingsApi } from '../../api'
import { clearToken, getToken } from '../../utils/auth'
import {
  bookingKindLabel,
  bookingStatusLabel,
  bookingValue,
  normalizeBookingId,
} from '../../utils/bookingDisplay'
import { clearBookingSession, readBookingSession } from '../../utils/bookingSession'
import { userErrorMessage } from '../../utils/userMessage'

const booking = ref(null)
const loading = ref(false)
const loadError = ref('')
const notFound = ref(false)

let loadTicket = 0
let routeBookingId = ''
let redirecting = false
let skipNextShow = false
let pageHidden = false

onLoad((query = {}) => {
  redirecting = false
  skipNextShow = true
  pageHidden = false
  const nextBookingId = normalizeBookingId(query?.id)
  if (routeBookingId && routeBookingId !== nextBookingId) {
    loadTicket += 1
    clearBookingSession()
  }
  routeBookingId = nextBookingId
  loadBookingDetail()
})

onShow(() => {
  if (skipNextShow) {
    skipNextShow = false
    return
  }
  if (!pageHidden) return

  pageHidden = false
  redirecting = false
  routeBookingId = normalizeBookingId(routeBookingId)
  loadBookingDetail()
})

onHide(() => {
  loadTicket += 1
  skipNextShow = false
  pageHidden = true
  booking.value = null
  loading.value = false
  loadError.value = ''
  notFound.value = false
  clearBookingSession()
})

onUnload(() => {
  loadTicket += 1
  skipNextShow = false
  pageHidden = false
  routeBookingId = ''
  booking.value = null
  loading.value = false
  loadError.value = ''
  notFound.value = false
  clearBookingSession()
})

function isAuthError(error) {
  const statusCode = Number(error?.statusCode)
  return statusCode === 401 || statusCode === 403
}

function isCurrentBookingContext(ticket, token, bookingId, error) {
  if (ticket !== loadTicket || bookingId !== routeBookingId) return false
  const currentToken = getToken()
  if (token === currentToken) return true
  return Boolean(
    !currentToken
    && error?.authExpired
    && error.requestToken === token,
  )
}

function invalidateStaleBookingContext(ticket = loadTicket) {
  if (ticket !== loadTicket) return
  loadTicket += 1
  booking.value = null
  loading.value = false
  notFound.value = false
  loadError.value = '登录状态已更新，请返回预约记录后重试'
  clearBookingSession()
}

function handleAuthLoss(ticket = loadTicket) {
  if (ticket !== loadTicket) return

  loadTicket += 1
  clearToken()
  clearBookingSession()
  booking.value = null
  loading.value = false
  loadError.value = ''
  notFound.value = false

  if (redirecting) return
  redirecting = true
  uni.showToast({ title: '登录已过期，请重新登录', icon: 'none' })
  uni.switchTab({ url: '/pages/profile/profile' })
}

async function loadBookingDetail() {
  const ticket = ++loadTicket
  const bookingId = routeBookingId

  booking.value = null
  loadError.value = ''
  notFound.value = false

  if (!bookingId) {
    loading.value = false
    notFound.value = true
    clearBookingSession()
    return
  }

  const requestToken = getToken()
  if (!requestToken) {
    handleAuthLoss(ticket)
    return
  }

  loading.value = true
  const cachedBooking = readBookingSession(requestToken, bookingId)
  if (!isCurrentBookingContext(ticket, requestToken, bookingId)) {
    invalidateStaleBookingContext(ticket)
    return
  }
  if (cachedBooking) {
    booking.value = cachedBooking
    loading.value = false
    return
  }

  try {
    const response = await listBookingsApi()
    if (!isCurrentBookingContext(ticket, requestToken, bookingId)) {
      invalidateStaleBookingContext(ticket)
      return
    }

    const matchedBooking = (Array.isArray(response?.items) ? response.items : [])
      .slice(0, 50)
      .find((item) => String(item?.id) === bookingId)

    if (!matchedBooking) {
      clearBookingSession()
      notFound.value = true
      return
    }

    booking.value = matchedBooking
  } catch (error) {
    if (!isCurrentBookingContext(ticket, requestToken, bookingId, error)) {
      invalidateStaleBookingContext(ticket)
      return
    }
    if (isAuthError(error)) {
      handleAuthLoss(ticket)
      return
    }
    loadError.value = userErrorMessage(error, '预约详情加载失败，请重试')
  } finally {
    if (isCurrentBookingContext(ticket, requestToken, bookingId)) {
      loading.value = false
    }
  }
}

function retryLoad() {
  if (loading.value || redirecting) return
  loadBookingDetail()
}

function goBookingRecords() {
  clearBookingSession()
  uni.redirectTo({ url: '/pages/booking-records/booking-records' })
}
</script>

<template>
  <view class="booking-detail page-stack ios-page ios-safe-bottom">
    <view class="detail-state-wrap" aria-live="polite">
      <view v-if="loading" class="detail-state nx-state ios-card">
        <view class="loading-mark" aria-hidden="true"></view>
        <text class="detail-state__title">正在读取预约详情…</text>
        <text class="detail-state__lead">请稍候，我们正在确认这条预约记录。</text>
      </view>

      <view v-else-if="loadError" class="detail-state detail-state--error nx-state ios-card">
        <text class="detail-state__title">暂时无法加载</text>
        <text class="detail-state__lead">{{ loadError }}</text>
        <button class="detail-action ios-button" tabindex="0" @click="retryLoad">重试</button>
      </view>

      <view v-else-if="notFound" class="detail-state nx-state ios-card">
        <view class="not-found-mark" aria-hidden="true">约</view>
        <text class="detail-state__title">没有找到这条预约</text>
        <text class="detail-state__lead">记录可能已更新，返回预约列表查看最新安排。</text>
        <button class="detail-action ios-button" @click="goBookingRecords">返回预约列表</button>
      </view>

      <template v-else-if="booking">
        <view class="detail-hero nx-page-hero">
          <text class="detail-hero__eyebrow">预约详情</text>
          <view class="detail-hero__head">
            <text class="detail-hero__title">{{ bookingKindLabel(booking.kind) }}</text>
            <text class="detail-hero__status">{{ bookingStatusLabel(booking.status) }}</text>
          </view>
          <text class="detail-hero__lead">老师会根据你提交的信息联系确认具体安排。</text>
        </view>

        <view class="detail-panel nx-panel ios-card">
          <view class="detail-panel__head">
            <text class="detail-panel__kicker">预约信息</text>
            <text class="detail-panel__title">记录概览</text>
          </view>
          <view class="detail-list">
            <view class="detail-row">
              <text class="detail-row__label">预约编号</text>
              <text class="detail-row__value">{{ bookingValue(booking.id) }}</text>
            </view>
            <view class="detail-row">
              <text class="detail-row__label">预约类型</text>
              <text class="detail-row__value">{{ bookingKindLabel(booking.kind) }}</text>
            </view>
            <view class="detail-row">
              <text class="detail-row__label">当前状态</text>
              <text class="detail-row__value detail-row__value--status">{{ bookingStatusLabel(booking.status) }}</text>
            </view>
            <view class="detail-row">
              <text class="detail-row__label">创建时间</text>
              <text class="detail-row__value">{{ bookingValue(booking.createTime) }}</text>
            </view>
          </view>
        </view>

        <view class="detail-panel nx-panel ios-card">
          <view class="detail-panel__head">
            <text class="detail-panel__kicker">联系资料</text>
            <text class="detail-panel__title">联系信息</text>
          </view>
          <view class="detail-list">
            <view class="detail-row">
              <text class="detail-row__label">称呼</text>
              <text class="detail-row__value">{{ bookingValue(booking.contactName) }}</text>
            </view>
            <view class="detail-row">
              <text class="detail-row__label">手机号</text>
              <text class="detail-row__value detail-row__value--phone">{{ bookingValue(booking.phone) }}</text>
            </view>
          </view>
        </view>

        <view class="detail-panel nx-panel ios-card">
          <view class="detail-panel__head">
            <text class="detail-panel__kicker">需求备注</text>
            <text class="detail-panel__title">学习安排</text>
          </view>
          <view class="detail-list">
            <view class="detail-row detail-row--stacked">
              <text class="detail-row__label">学习意向</text>
              <text class="detail-row__value">{{ bookingValue(booking.intent) }}</text>
            </view>
            <view class="detail-row detail-row--stacked">
              <text class="detail-row__label">期望时间</text>
              <text class="detail-row__value">{{ bookingValue(booking.preferredTime) }}</text>
            </view>
            <view class="detail-row detail-row--stacked">
              <text class="detail-row__label">留言</text>
              <text class="detail-row__value detail-row__value--message">{{ bookingValue(booking.message) }}</text>
            </view>
          </view>
        </view>
      </template>
    </view>
  </view>
</template>

<style scoped>
.booking-detail {
  gap: 24rpx;
}

.detail-state-wrap {
  display: flex;
  flex-direction: column;
  gap: 24rpx;
}

.detail-state {
  min-height: 420rpx;
  padding: 52rpx 34rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  text-align: center;
}

.detail-state__title {
  color: #0f172a;
  font-size: 32rpx;
  font-weight: 900;
}

.detail-state__lead {
  display: block;
  max-width: 520rpx;
  margin-top: 14rpx;
  color: #64748b;
  font-size: 25rpx;
  line-height: 1.65;
}

.loading-mark {
  width: 52rpx;
  height: 52rpx;
  margin-bottom: 24rpx;
  border: 6rpx solid rgba(43, 127, 255, .18);
  border-top-color: #2b7fff;
  border-radius: 50%;
  animation: spin .9s linear infinite;
}

.not-found-mark {
  width: 92rpx;
  height: 92rpx;
  margin-bottom: 22rpx;
  border-radius: 30rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #60a5fa, #7c3aed);
  color: #fff;
  font-size: 38rpx;
  font-weight: 900;
}

.detail-action {
  min-width: 240rpx;
  min-height: 88rpx;
  margin-top: 28rpx;
  border: 0;
  border-radius: 24rpx;
  color: #fff;
  background: linear-gradient(135deg, #2b7fff, #6d5dfc);
  font-size: 27rpx;
  font-weight: 800;
}

.detail-hero {
  box-sizing: border-box;
  width: 100%;
  padding: 38rpx 34rpx;
  border-radius: 38rpx;
  background: linear-gradient(145deg, #172554, #4338ca 56%, #7c3aed);
  color: #fff;
  box-shadow: 0 26rpx 54rpx -34rpx rgba(49, 46, 129, .78);
}

.detail-hero__eyebrow {
  display: block;
  color: #ddd6fe;
  font-size: 24rpx;
  font-weight: 800;
}

.detail-hero__head {
  margin-top: 12rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
}

.detail-hero__title {
  color: #fff;
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.25;
}

.detail-hero__status {
  flex: none;
  padding: 9rpx 17rpx;
  border: 2rpx solid rgba(255, 255, 255, .28);
  border-radius: 999rpx;
  color: #fff;
  background: rgba(255, 255, 255, .14);
  font-size: 23rpx;
  font-weight: 800;
}

.detail-hero__lead {
  display: block;
  margin-top: 16rpx;
  color: #ede9fe;
  font-size: 25rpx;
  line-height: 1.65;
}

.detail-panel {
  box-sizing: border-box;
  width: 100%;
  margin-top: 24rpx;
  padding: 30rpx;
}

.detail-panel__kicker {
  display: block;
  color: #4f46e5;
  font-size: 24rpx;
  font-weight: 800;
}

.detail-panel__title {
  display: block;
  margin-top: 6rpx;
  color: #0f172a;
  font-size: 32rpx;
  font-weight: 900;
}

.detail-list {
  margin-top: 24rpx;
  border-top: 1rpx solid rgba(148, 163, 184, .24);
}

.detail-row {
  min-height: 88rpx;
  padding: 22rpx 0;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 28rpx;
  border-bottom: 1rpx solid rgba(148, 163, 184, .2);
  box-sizing: border-box;
}

.detail-row--stacked {
  flex-direction: column;
  gap: 10rpx;
}

.detail-row__label {
  flex: none;
  color: #64748b;
  font-size: 24rpx;
  line-height: 1.55;
}

.detail-row__value {
  min-width: 0;
  color: #172033;
  font-size: 26rpx;
  font-weight: 700;
  line-height: 1.55;
  text-align: right;
  word-break: break-word;
}

.detail-row--stacked .detail-row__value {
  width: 100%;
  text-align: left;
  font-weight: 600;
}

.detail-row__value--status {
  color: #295fbd;
}

.detail-row__value--phone {
  letter-spacing: .5rpx;
}

.detail-row__value--message {
  white-space: pre-wrap;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 360px) {
  .detail-hero,
  .detail-panel {
    padding-left: 26rpx;
    padding-right: 26rpx;
  }

  .detail-hero__head {
    align-items: flex-start;
  }
}

@media (prefers-reduced-motion: reduce) {
  .loading-mark {
    animation: none;
  }
}
</style>
