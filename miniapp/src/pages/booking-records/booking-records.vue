<script setup>
import { ref } from 'vue'
import { onShow, onUnload } from '@dcloudio/uni-app'
import { listBookingsApi } from '../../api'
import { clearToken, getToken } from '../../utils/auth'
import {
  bookingKindLabel,
  bookingStatusLabel,
  bookingValue,
  maskBookingPhone,
  normalizeBookingId,
} from '../../utils/bookingDisplay'
import { clearBookingSession, setBookingSession } from '../../utils/bookingSession'
import { userErrorMessage } from '../../utils/userMessage'

const bookings = ref([])
const loading = ref(false)
const loadError = ref('')

let loadTicket = 0
let loadedToken = ''
let redirecting = false

onShow(() => {
  redirecting = false
  clearBookingSession()
  loadBookings()
})

onUnload(() => {
  loadTicket += 1
  loadedToken = ''
  loading.value = false
  clearBookingSession()
})

function isAuthError(error) {
  const statusCode = Number(error?.statusCode)
  return statusCode === 401 || statusCode === 403
}

function isCurrentBookingSession(ticket, token, error) {
  if (ticket !== loadTicket) return false
  const currentToken = getToken()
  if (token === currentToken) return true
  return Boolean(
    !currentToken
    && error?.authExpired
    && error.requestToken === token,
  )
}

function invalidateStaleBookingSession(ticket = loadTicket) {
  if (ticket !== loadTicket) return
  loadTicket += 1
  loadedToken = ''
  bookings.value = []
  loadError.value = '登录状态已更新，请重新加载预约记录'
  loading.value = false
  clearBookingSession()
}

function handleAuthLoss(ticket = loadTicket) {
  if (ticket !== loadTicket) return

  loadTicket += 1
  clearToken()
  clearBookingSession()
  loadedToken = ''
  bookings.value = []
  loadError.value = ''
  loading.value = false

  if (redirecting) return
  redirecting = true
  uni.showToast({ title: '登录已过期，请重新登录', icon: 'none' })
  uni.switchTab({ url: '/pages/profile/profile' })
}

async function loadBookings() {
  if (loading.value || redirecting) return

  const requestToken = getToken()
  const ticket = ++loadTicket
  if (!requestToken) {
    handleAuthLoss(ticket)
    return
  }

  loading.value = true
  loadError.value = ''

  try {
    const response = await listBookingsApi()
    if (!isCurrentBookingSession(ticket, requestToken)) {
      invalidateStaleBookingSession(ticket)
      return
    }

    loadedToken = requestToken
    bookings.value = Array.isArray(response?.items) ? response.items : []
  } catch (error) {
    if (!isCurrentBookingSession(ticket, requestToken, error)) {
      invalidateStaleBookingSession(ticket)
      return
    }
    if (isAuthError(error)) {
      handleAuthLoss(ticket)
      return
    }
    loadError.value = userErrorMessage(error, '预约记录加载失败，请重试')
  } finally {
    if (isCurrentBookingSession(ticket, requestToken)) {
      loading.value = false
    }
  }
}

function retryLoad() {
  if (loading.value || redirecting) return
  loadBookings()
}

function bookingSummary(record) {
  const intent = bookingValue(record?.intent)
  return intent === '未填写' ? '老师会尽快与你确认安排' : intent
}

function openBooking(record) {
  const currentToken = getToken()
  if (!currentToken) {
    const ticket = ++loadTicket
    handleAuthLoss(ticket)
    return
  }
  if (currentToken !== loadedToken) {
    invalidateStaleBookingSession()
    return
  }

  const bookingId = normalizeBookingId(record?.id)
  if (!bookingId || !setBookingSession(currentToken, record)) {
    uni.showToast({ title: '该预约记录暂时无法打开', icon: 'none' })
    return
  }

  uni.navigateTo({
    url: `/pages/booking-detail/booking-detail?id=${encodeURIComponent(bookingId)}`,
    fail() {
      clearBookingSession()
    },
  })
}

function goBooking() {
  clearBookingSession()
  uni.switchTab({ url: '/pages/booking/booking' })
}
</script>

<template>
  <view class="wrap booking-records page-stack ios-page ios-safe-bottom">
    <view class="records-hero card ios-card">
      <text class="eyebrow">我的预约</text>
      <text class="records-hero__title gradient-title">预约记录</text>
      <text class="records-hero__lead">查看老师与你确认中的咨询、课程和企业服务安排。</text>
    </view>

    <view class="records-panel" aria-live="polite">
      <view v-if="loading" class="records-state card ios-card">
        <view class="loading-mark" aria-hidden="true"></view>
        <text class="records-state__title">正在同步预约记录…</text>
        <text class="records-state__lead">请稍候，我们正在读取最新安排。</text>
      </view>

      <view v-else-if="loadError" class="records-state records-state--error card ios-card">
        <text class="records-state__title">暂时无法加载</text>
        <text class="records-state__lead">{{ loadError }}</text>
        <button class="retry-button ios-button" tabindex="0" @click.stop="retryLoad">重试</button>
      </view>

      <view v-else-if="bookings.length === 0" class="records-state card ios-card">
        <view class="empty-mark" aria-hidden="true">约</view>
        <text class="records-state__title">还没有预约记录</text>
        <text class="records-state__lead">提交需求后，老师会尽快与你联系并确认安排。</text>
        <button class="empty-action ios-button" @click="goBooking">去预约</button>
      </view>

      <view v-else class="booking-list">
        <view v-for="record in bookings" :key="record.id" class="booking-record card ios-card">
          <view
            class="booking-record__open"
            role="button"
            aria-role="button"
            tabindex="0"
            :aria-label="`查看${bookingKindLabel(record.kind)}预约详情`"
            hover-class="booking-record__open--pressed"
            @click="openBooking(record)"
            @keydown.enter="openBooking(record)"
            @keydown.space.prevent="openBooking(record)"
          >
            <view class="booking-record__head">
              <text class="booking-record__kind">{{ bookingKindLabel(record.kind) }}</text>
              <text class="booking-record__status">{{ bookingStatusLabel(record.status) }}</text>
            </view>
            <text class="booking-record__summary">{{ bookingSummary(record) }}</text>
            <view class="booking-record__meta">
              <text>{{ bookingValue(record.createTime) }}</text>
              <text>{{ maskBookingPhone(record.phone) }}</text>
            </view>
            <view class="booking-record__footer">
              <text>查看预约详情</text>
              <text class="booking-record__arrow" aria-hidden="true">›</text>
            </view>
          </view>
        </view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.booking-records {
  gap: 24rpx;
}

.records-hero {
  padding: 38rpx 34rpx;
  background:
    linear-gradient(145deg, rgba(255, 255, 255, .95), rgba(239, 246, 255, .78)),
    radial-gradient(circle at 92% 4%, rgba(124, 58, 237, .17), transparent 44%);
}

.records-hero__title {
  display: block;
  margin-top: 14rpx;
  font-size: 48rpx;
  font-weight: 900;
  letter-spacing: -1rpx;
}

.records-hero__lead {
  display: block;
  margin-top: 14rpx;
  color: #475569;
  font-size: 27rpx;
  line-height: 1.7;
}

.records-panel,
.booking-list {
  display: flex;
  flex-direction: column;
  gap: 20rpx;
}

.records-state {
  min-height: 360rpx;
  padding: 52rpx 34rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.records-state__title {
  color: #0f172a;
  font-size: 32rpx;
  font-weight: 900;
}

.records-state__lead {
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

.empty-mark {
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
  box-shadow: 0 18rpx 38rpx -24rpx rgba(43, 127, 255, .78);
}

.retry-button,
.empty-action {
  min-width: 220rpx;
  min-height: 88rpx;
  margin-top: 28rpx;
  border: 0;
  border-radius: 24rpx;
  color: #fff;
  background: linear-gradient(135deg, #2b7fff, #6d5dfc);
  font-size: 27rpx;
  font-weight: 800;
}

.booking-record {
  padding: 0;
  overflow: hidden;
}

.booking-record__open {
  min-height: 88rpx;
  padding: 30rpx 32rpx 26rpx;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  cursor: pointer;
}

.booking-record__open--pressed {
  opacity: .76;
  transform: scale(.992);
}

.booking-record__open:focus-visible {
  outline: 4rpx solid rgba(43, 127, 255, .72);
  outline-offset: -4rpx;
}

.booking-record__head,
.booking-record__meta,
.booking-record__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
}

.booking-record__kind {
  color: #0f172a;
  font-size: 31rpx;
  font-weight: 900;
}

.booking-record__status {
  flex: none;
  padding: 8rpx 16rpx;
  border-radius: 999rpx;
  color: #295fbd;
  background: #e8f1ff;
  font-size: 24rpx;
  font-weight: 800;
}

.booking-record__summary {
  display: block;
  margin-top: 22rpx;
  color: #334155;
  font-size: 27rpx;
  line-height: 1.6;
}

.booking-record__meta {
  margin-top: 18rpx;
  color: #64748b;
  font-size: 24rpx;
}

.booking-record__footer {
  min-height: 56rpx;
  margin-top: 24rpx;
  padding-top: 20rpx;
  border-top: 1rpx solid rgba(148, 163, 184, .22);
  color: #2563eb;
  font-size: 24rpx;
  font-weight: 800;
}

.booking-record__arrow {
  font-size: 38rpx;
  line-height: 1;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .loading-mark {
    animation: none;
  }

  .booking-record__open {
    transition: none;
  }
}
</style>
