<script setup>
import { computed, ref } from 'vue'
import { onLoad, onShow, onHide, onUnload } from '@dcloudio/uni-app'
import { isCourseRegistrationEnabled } from '../../utils/courseRegistration'
import { refreshSiteConfig, getStoredSiteConfig } from '../../utils/siteConfig'
import NxIcon from '../../components/NxIcon.vue'
import { getToken } from '../../utils/auth'
import { getCourseBookingOrderStatusApi } from '../../api'
import { navigateToOrders, navigateToMyCourse } from '../../utils/courseNavigation'

const registrationConfig = ref(getStoredSiteConfig() || {})
const courseRegistrationEnabled = computed(() => isCourseRegistrationEnabled(registrationConfig.value))
async function refreshRegistrationConfig() {
  try {
    const config = await refreshSiteConfig()
    if (!disposed) registrationConfig.value = config || {}
  } catch { /* Retain the last confirmed switch; checkout is also guarded by the server. */ }
}

const bookingId = ref('')
const order = ref(null)
const status = ref('pending')
const loading = ref(false)
const queryError = ref(false)
const loginNeeded = ref(false)
const invalidBooking = ref(false)
const attempts = ref(0)
const MAX_ATTEMPTS = 6
const QUERY_INTERVAL = 1200
const terminalStatuses = ['paid', 'closed', 'refunded', 'cancelled']
let sessionToken = ''
let queryTicket = 0
let timer = null
let active = false
let disposed = false
let redirecting = false

const isPaid = computed(() => status.value === 'paid' && !!order.value && !loginNeeded.value)
const isPending = computed(() => !loginNeeded.value && !invalidBooking.value && status.value === 'pending')
const headline = computed(() => {
  if (loginNeeded.value) return '请重新登录'
  if (invalidBooking.value) return '未找到订单'
  return { paid: '支付成功', closed: '订单已关闭', refunded: '订单已退款', cancelled: '订单已取消' }[status.value] || '支付结果确认中'
})
const description = computed(() => {
  if (loginNeeded.value) return '登录状态已更新，请登录后在我的订单中查看支付结果。'
  if (invalidBooking.value) return '订单信息不完整，请前往我的订单查看报名与支付记录。'
  if (isPaid.value) return '报名已确认，期待与你在课堂相遇。'
  if (status.value === 'refunded') return '此笔订单已退款，退款详情请在微信支付账单中查看。'
  if (status.value === 'closed') return '此笔订单已关闭，可前往我的订单查看记录。'
  if (status.value === 'cancelled') return '此笔订单已取消，可前往我的订单查看记录。'
  if (queryError.value || attempts.value >= MAX_ATTEMPTS) return '支付结果仍在确认中。如已扣款，请勿重复支付，可刷新结果或前往我的订单查看。'
  return '正在向微信确认支付结果，请稍候。'
})
const statusText = computed(() => ({ paid: '已支付', pending: '确认中', closed: '已关闭', refunded: '已退款', cancelled: '已取消' }[status.value] || '确认中'))
const amountText = computed(() => Number.isSafeInteger(order.value?.amount) && order.value.amount >= 0 ? (order.value.amount / 100).toFixed(2) : '—')
const statusIcon = computed(() => loginNeeded.value ? 'user' : isPaid.value ? 'check' : isPending.value ? 'clock' : 'book')

function cancelQueries() {
  queryTicket += 1
  if (timer !== null) clearTimeout(timer)
  timer = null
  loading.value = false
}
function clearDetails() {
  order.value = null
  status.value = 'pending'
  queryError.value = false
  attempts.value = 0
}
function goLogin() {
  if (disposed || redirecting) return
  redirecting = true
  uni.switchTab({ url: '/pages/profile/profile', fail: () => { redirecting = false } })
}
function requireLogin() {
  cancelQueries()
  clearDetails()
  bookingId.value = ''
  sessionToken = ''
  loginNeeded.value = true
  goLogin()
}
function currentSession() {
  if (loginNeeded.value) return false
  if (!sessionToken || getToken() !== sessionToken) {
    requireLogin()
    return false
  }
  return true
}
function currentQuery(ticket) {
  return active && !disposed && ticket === queryTicket
}
function normalizeOrder(value) {
  const confirmedStatus = value?.syncStatus === 'retrying' && value.status !== 'paid' ? 'pending' : value?.status
  if (!value || String(value.bookingId) !== bookingId.value || !['pending', ...terminalStatuses].includes(confirmedStatus)) {
    throw new Error('订单状态尚未确认')
  }
  return {
    bookingId: String(value.bookingId),
    status: confirmedStatus,
    title: typeof value.title === 'string' ? value.title : '',
    amount: value.amount,
    outTradeNo: typeof value.outTradeNo === 'string' ? value.outTradeNo : '',
    courseId: typeof value.courseId === 'string' || typeof value.courseId === 'number' ? String(value.courseId) : '',
    paidAt: typeof value.paidAt === 'string' ? value.paidAt : '',
  }
}
async function queryStatus(ticket) {
  if (!currentQuery(ticket) || !currentSession()) return
  loading.value = true
  attempts.value += 1
  try {
    const result = await getCourseBookingOrderStatusApi(bookingId.value)
    if (!currentQuery(ticket) || !currentSession()) return
    const confirmedOrder = normalizeOrder(result)
    order.value = confirmedOrder
    status.value = confirmedOrder.status
    queryError.value = false
  } catch (error) {
    if (!currentQuery(ticket) || !currentSession()) return
    if (error?.authExpired || error?.authRequired || [401, 403].includes(Number(error?.statusCode))) {
      requireLogin()
      return
    }
    status.value = 'pending'
    queryError.value = true
  } finally {
    if (currentQuery(ticket)) loading.value = false
  }
  if (currentQuery(ticket) && currentSession() && status.value === 'pending' && attempts.value < MAX_ATTEMPTS) {
    timer = setTimeout(() => {
      timer = null
      return queryStatus(ticket)
    }, QUERY_INTERVAL)
  }
}
function startPolling() {
  if (disposed || !active || !currentSession() || invalidBooking.value) return
  cancelQueries()
  clearDetails()
  return queryStatus(queryTicket)
}
function refreshResult() {
  if (loading.value) return
  return startPolling()
}
function goOrders() {
  if (disposed || !currentSession()) return
  navigateToOrders()
}
function openCourse() {
  if (disposed || !currentSession() || !order.value) return
  if (isPaid.value && /^[1-9]\d*$/.test(order.value.bookingId)) {
    navigateToMyCourse(order.value.bookingId)
  } else if (!courseRegistrationEnabled.value) {
    browseCourses()
  } else if (order.value.courseId) {
    uni.navigateTo({ url: `/pages/course-detail/course-detail?id=${encodeURIComponent(order.value.courseId)}` })
  }
}
function browseCourses() {
  if (!disposed) uni.switchTab({ url: '/pages/booking/booking' })
}

onLoad((query) => {
  const id = String(query?.bookingId ?? '')
  invalidBooking.value = !/^\d+$/.test(id) || /^0+$/.test(id)
  bookingId.value = invalidBooking.value ? '' : id
  sessionToken = getToken()
  if (!sessionToken) requireLogin()
})
onShow(() => {
  if (disposed) return
  // #ifdef MP-WEIXIN
  if (typeof uni !== 'undefined' && typeof uni.hideShareMenu === 'function') {
    uni.hideShareMenu({ menus: ['shareAppMessage', 'shareTimeline'] })
  }
  // #endif
  active = true
  redirecting = false
  return Promise.all([startPolling(), refreshRegistrationConfig()])
})
onHide(() => {
  active = false
  cancelQueries()
  clearDetails()
})
onUnload(() => {
  disposed = true
  active = false
  cancelQueries()
  clearDetails()
  bookingId.value = ''
  sessionToken = ''
})
</script>

<template>
  <view class="payment-result-page">
    <view class="result-header">
      <text class="eyebrow">PAYMENT RESULT</text>
      <text class="page-label">课程报名 · 支付结果</text>
    </view>

    <view class="result-card" aria-live="polite">
      <view :class="['status-symbol', { 'status-symbol--paid': isPaid }]">
        <NxIcon :name="statusIcon" :size="38" :color="isPaid ? '#FFFFFF' : '#A55C3B'" />
      </view>
      <text class="result-title">{{ headline }}</text>
      <text class="result-description">{{ description }}</text>
      <view v-if="isPending && loading" class="confirmation-note"><view class="confirmation-dot" /><text>正在查询订单状态</text></view>

      <view v-if="order && !loginNeeded" class="order-summary">
        <view class="course-heading"><text class="summary-eyebrow">你的课程报名</text><text class="course-title">{{ order.title || '课程报名' }}</text></view>
        <view class="amount-row"><text>订单金额</text><view><text class="currency">¥</text><text class="amount">{{ amountText }}</text></view></view>
        <view class="detail-row"><text class="detail-label">支付状态</text><text class="status-value">{{ statusText }}</text></view>
        <view v-if="order.outTradeNo" class="detail-row"><text class="detail-label">订单编号</text><text class="detail-value">{{ order.outTradeNo }}</text></view>
        <view v-if="isPaid && order.paidAt" class="detail-row"><text class="detail-label">确认时间</text><text class="detail-value">{{ order.paidAt }}</text></view>
      </view>
    </view>

    <view class="result-actions">
      <button v-if="loginNeeded" class="primary-button" @click="goLogin">登录后查看</button>
      <block v-else>
        <button v-if="isPending" class="primary-button" :loading="loading" :disabled="loading" @click="refreshResult">{{ loading ? '正在确认' : '刷新结果' }}</button>
        <button v-if="isPaid" class="primary-button" @click="openCourse">查看我的课程 <NxIcon name="arrow" :size="18" color="#FFFFFF" /></button>
        <button :class="isPending || isPaid ? 'secondary-button' : 'primary-button'" @click="goOrders">查看我的订单 <NxIcon name="arrow" :size="18" :color="isPending || isPaid ? '#A55C3B' : '#FFFFFF'" /></button>
        <button v-if="courseRegistrationEnabled && !isPaid && order?.courseId" class="text-button" @click="openCourse">查看课程介绍 <NxIcon name="chevron" :size="16" /></button>
      </block>
      <button class="text-button browse-button" @click="browseCourses">{{ courseRegistrationEnabled ? '返回课程列表' : '返回报名页' }}</button>
    </view>
    <view class="page-ending"><view /><text>让理解发生，让成长继续</text><view /></view>
  </view>
</template>

<style scoped>
.payment-result-page{min-height:100vh;padding:42rpx 36rpx calc(44rpx + env(safe-area-inset-bottom));box-sizing:border-box;background:var(--nx-page-bg,#F7F4ED);color:var(--nx-text,#38372F)}
button{box-sizing:border-box;border:0;margin:0;line-height:1.5}button::after{border:0}.result-header{padding:0 4rpx 32rpx}.eyebrow{display:block;color:#A55C3B;font-size:18rpx;letter-spacing:3rpx}.page-label{display:block;margin-top:12rpx;font-size:23rpx;color:var(--nx-text-muted,#858073);letter-spacing:1rpx}.result-card{padding:44rpx 32rpx 34rpx;border:1rpx solid var(--nx-border,#E4DED1);border-radius:28rpx;background:var(--nx-surface,#FFFFFF);text-align:center}.status-symbol{display:flex;align-items:center;justify-content:center;width:118rpx;height:118rpx;margin:0 auto 27rpx;border:1rpx solid #EADACE;border-radius:50%;background:#F4EAE2}.status-symbol--paid{border-color:#A55C3B;background:#A55C3B}.result-title{display:block;font-family:'Songti SC','STSong',serif;font-size:43rpx;font-weight:600;line-height:1.4}.result-description{display:block;margin:19rpx auto 0;max-width:540rpx;font-size:25rpx;line-height:1.85;color:var(--nx-text-muted,#858073)}.confirmation-note{display:flex;justify-content:center;align-items:center;gap:12rpx;margin-top:24rpx;color:#A55C3B;font-size:22rpx}.confirmation-dot{height:9rpx;width:9rpx;border-radius:50%;background:#A55C3B}.order-summary{margin-top:38rpx;padding-top:30rpx;border-top:1rpx solid var(--nx-border,#E4DED1);text-align:left}.summary-eyebrow{display:block;font-size:20rpx;color:var(--nx-text-muted,#858073);letter-spacing:1rpx}.course-title{display:block;margin-top:12rpx;font-family:'Songti SC','STSong',serif;font-size:31rpx;line-height:1.6;overflow-wrap:anywhere}.amount-row{display:flex;justify-content:space-between;align-items:center;gap:22rpx;padding:27rpx 0;margin-bottom:5rpx;border-bottom:1rpx solid var(--nx-border,#E4DED1);font-size:24rpx;color:var(--nx-text-muted,#858073)}.currency{font-size:26rpx;color:#A55C3B;margin-right:7rpx}.amount{font-family:Georgia,serif;font-size:46rpx;color:#A55C3B}.detail-row{display:flex;justify-content:space-between;align-items:flex-start;gap:26rpx;padding-top:21rpx;font-size:23rpx;line-height:1.7}.detail-label{flex-shrink:0;color:var(--nx-text-muted,#858073)}.detail-value{min-width:0;text-align:right;overflow-wrap:anywhere;word-break:break-all}.status-value{color:#A55C3B}.result-actions{display:flex;flex-direction:column;gap:19rpx;margin-top:30rpx}.primary-button,.secondary-button{display:flex;align-items:center;justify-content:center;gap:14rpx;width:100%;min-height:96rpx;padding:23rpx;border-radius:15rpx;font-size:27rpx}.primary-button{background:#A55C3B;color:#FFFFFF}.primary-button[disabled]{background:#B87A5D;color:#FFFFFF}.secondary-button{background:transparent;border:1rpx solid #C9A28B;color:#A55C3B}.text-button{display:flex;align-items:center;justify-content:center;gap:7rpx;min-height:72rpx;padding:12rpx;background:transparent;color:#A55C3B;font-size:24rpx}.browse-button{color:var(--nx-text-muted,#858073)}.page-ending{display:flex;align-items:center;gap:18rpx;padding:40rpx 5rpx 0;color:#999183;font-size:20rpx;letter-spacing:1rpx}.page-ending view{flex:1;height:1rpx;background:var(--nx-border,#E4DED1)}
@media(min-width:600px){.payment-result-page{max-width:800rpx;margin:auto}}
</style>
