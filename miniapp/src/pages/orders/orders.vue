<script setup>
import { computed, ref } from 'vue'
import { onShow, onUnload, onPullDownRefresh, onReachBottom } from '@dcloudio/uni-app'
import NxIcon from '../../components/NxIcon.vue'
import { getToken, clearToken } from '../../utils/auth'
import { listMiniappOrdersApi, createCourseBookingOrderApi, devPayCourseBookingOrderApi, getCourseBookingOrderStatusApi } from '../../api'
import { payWechatOrder } from '../../utils/payment'
import { createCoursePaymentController, coursePaymentResultUrl } from '../../utils/coursePayment'
import { userErrorMessage } from '../../utils/userMessage'

const filters = [{ value: '', label: '全部订单' }, { value: 'pending', label: '待支付' }, { value: 'paid', label: '已支付' }]
const filter = ref('')
const orders = ref([])
const loading = ref(false)
const loadingMore = ref(false)
const loadError = ref('')
const moreError = ref('')
const page = ref(0)
const total = ref(0)
const payingId = ref('')
const paymentMessage = ref('')
const failedCovers = ref({})
const hasMore = computed(() => orders.value.length < total.value)
const PAGE_SIZE = 20
let sessionToken = ''
let loadTicket = 0
let paymentTicket = 0
let paymentController = null
let disposed = false
let redirecting = false

function resetSession() {
  loadTicket += 1
  paymentTicket += 1
  paymentController?.stop()
  paymentController = null
  sessionToken = ''
  orders.value = []
  total.value = 0
  page.value = 0
  loading.value = false
  loadingMore.value = false
  payingId.value = ''
  paymentMessage.value = ''
  loadError.value = ''
  moreError.value = ''
  failedCovers.value = {}
}
function loginRequired() {
  resetSession()
  if (disposed || redirecting) return
  redirecting = true
  uni.showToast({ title: '请先登录后查看订单', icon: 'none' })
  uni.switchTab({ url: '/pages/profile/profile' })
}
function isAuthError(error) {
  return error?.authExpired || error?.authRequired || [401, 403].includes(Number(error?.statusCode))
}
function currentLoad(ticket, token) {
  return !disposed && ticket === loadTicket && token === sessionToken && token === getToken()
}
function handleSessionChange(token, error) {
  if (disposed || sessionToken !== token) return
  const current = getToken()
  if (current !== token) {
    resetSession()
    if (!current) loginRequired()
    else loadError.value = '登录状态已更新，请重新加载订单'
  } else if (isAuthError(error)) {
    clearToken()
    loginRequired()
  }
}

async function loadPage(nextPage, replace = false) {
  if (disposed || payingId.value) return
  const token = getToken()
  if (!token) { loginRequired(); return }
  if (sessionToken && sessionToken !== token) resetSession()
  sessionToken = token
  const ticket = ++loadTicket
  if (replace) {
    orders.value = []
    total.value = 0
    page.value = 0
    loading.value = true
    loadingMore.value = false
    loadError.value = ''
    moreError.value = ''
  } else {
    loadingMore.value = true
    moreError.value = ''
  }
  try {
    const result = await listMiniappOrdersApi({ page: nextPage, pageSize: PAGE_SIZE, status: filter.value || undefined })
    if (!currentLoad(ticket, token)) {
      if (ticket === loadTicket) handleSessionChange(token)
      return
    }
    const items = Array.isArray(result?.items) ? result.items : []
    const seen = new Set(replace ? [] : orders.value.map((item) => String(item.id)))
    const unique = items.filter((item) => { const id = String(item.id); if (seen.has(id)) return false; seen.add(id); return true })
    orders.value = replace ? unique : [...orders.value, ...unique]
    page.value = Number(result?.page) || nextPage
    total.value = Math.max(orders.value.length, Number(result?.total) || 0)
  } catch (error) {
    if (!currentLoad(ticket, token)) {
      if (ticket === loadTicket) handleSessionChange(token, error)
      return
    }
    if (isAuthError(error)) { handleSessionChange(token, error); return }
    const message = [502, 503, 504].includes(Number(error?.statusCode)) ? '服务暂时繁忙，请稍后重新加载。已支付订单记录会保留。' : userErrorMessage(error, '订单加载失败，请重试')
    if (replace) loadError.value = message
    else moreError.value = message
  } finally {
    if (currentLoad(ticket, token)) {
      loading.value = false
      loadingMore.value = false
    }
  }
}
function refresh() { return loadPage(1, true) }
function loadMore() {
  if (loading.value || loadingMore.value || payingId.value || !hasMore.value) return
  return loadPage(page.value + 1)
}
function selectFilter(value) {
  if (payingId.value || filter.value === value) return
  filter.value = value
  return refresh()
}

function productLabel(product) {
  return { course_booking: '课程报名', classroom_series: '课堂系列', classroom_content: '课堂课件', report: '深度报告', member: '会员服务', wechat_pay_test: '微信支付测试' }[product] || '小程序订单'
}
function statusLabel(status) { return { pending: '待支付', paid: '已支付', closed: '已关闭', refunded: '已退款', cancelled: '已取消', failed: '未完成' }[status] || '状态确认中' }
function amountYuan(amount) { return Number.isFinite(Number(amount)) ? (Number(amount) / 100).toFixed(2) : '—' }
function canContinue(order) { return order.product === 'course_booking' && order.status === 'pending' && /^\d+$/.test(String(order.bookingId || '')) }
function openCourse(order) {
  if (disposed || !sessionToken || sessionToken !== getToken()) { handleSessionChange(sessionToken); return }
  if (order.product !== 'course_booking') return
  if (order.status === 'paid' && /^[1-9]\d*$/.test(String(order.bookingId || ''))) {
    uni.navigateTo({ url: `/pages/my-course/my-course?bookingId=${encodeURIComponent(order.bookingId)}` })
    return
  }
  if (order.courseId) uni.navigateTo({ url: `/pages/course-detail/course-detail?id=${encodeURIComponent(order.courseId)}` })
}
function browseCourses() { uni.switchTab({ url: '/pages/booking/booking' }) }
function viewPaymentResult(order) {
  if (disposed || !sessionToken || sessionToken !== getToken()) { handleSessionChange(sessionToken); return }
  const url = coursePaymentResultUrl(order)
  if (url) uni.navigateTo({ url })
}
function paymentCurrent(ticket, token) { return !disposed && ticket === paymentTicket && sessionToken === token && getToken() === token }
function paid(status) { return status?.status === 'paid' || status?.paymentStatus === 'paid' }
function updateOrder(order, status) {
  orders.value = orders.value.map((item) => String(item.id) === String(order.id)
    ? { ...item, status: paid(status) ? 'paid' : status?.status || item.status, paidAt: status?.paidAt || item.paidAt }
    : item)
}
async function continuePayment(order) {
  if (payingId.value || disposed || !canContinue(order)) return
  if (typeof window !== 'undefined') {
    uni.showToast({ title: '请在微信小程序内完成支付', icon: 'none' })
    return
  }
  const token = getToken()
  if (!token) { loginRequired(); return }
  if (token !== sessionToken) { handleSessionChange(sessionToken); return }
  const ticket = ++paymentTicket
  payingId.value = String(order.id)
  paymentMessage.value = '正在确认订单状态'
  const ensureSession = () => {
    if (!paymentCurrent(ticket, token)) throw new Error('登录状态已更新，请重新查看订单')
  }
  try {
    const status = await getCourseBookingOrderStatusApi(order.bookingId)
    if (!paymentCurrent(ticket, token)) { handleSessionChange(token); return }
    if (paid(status) || status?.syncStatus === 'retrying') {
      updateOrder(order, status)
      viewPaymentResult(order)
      return
    }
    if (['closed', 'refunded', 'cancelled', 'failed'].includes(status?.status)) {
      updateOrder(order, status)
      viewPaymentResult(order)
      return
    }
    paymentController = createCoursePaymentController({
      isCurrent: () => paymentCurrent(ticket, token),
      create: () => { ensureSession(); return createCourseBookingOrderApi(order.bookingId) },
      pay: (created) => { ensureSession(); return payWechatOrder(created, { devPay: (pending) => devPayCourseBookingOrderApi(pending.outTradeNo) }) },
      status: () => { ensureSession(); return getCourseBookingOrderStatusApi(order.bookingId) },
      onChange: (snapshot) => { if (paymentCurrent(ticket, token)) paymentMessage.value = snapshot.message || '' },
    })
    const result = await paymentController.purchase()
    if (!paymentCurrent(ticket, token)) { handleSessionChange(token); return }
    if (result?.state === 'success') {
      updateOrder(order, result.status || { status: 'paid' })
    }
    if (result?.order) viewPaymentResult(result.order)
  } catch (error) {
    if (!paymentCurrent(ticket, token)) { handleSessionChange(token, error); return }
    if (isAuthError(error)) { handleSessionChange(token, error); return }
    viewPaymentResult(order)
  } finally {
    if (paymentCurrent(ticket, token)) {
      paymentController = null
      payingId.value = ''
      paymentMessage.value = ''
    }
  }
}

onShow(() => {
  if (disposed) return
  redirecting = false
  if (sessionToken && sessionToken !== getToken()) resetSession()
  if (payingId.value && sessionToken === getToken()) { paymentController?.resume(); return }
  return refresh()
})
onPullDownRefresh(async () => { try { await refresh() } finally { uni.stopPullDownRefresh() } })
onReachBottom(loadMore)
onUnload(() => { disposed = true; resetSession() })
</script>

<template>
  <view class="orders-page">
    <view class="orders-header"><text class="eyebrow">MY LEARNING ORDERS</text><text class="page-title">我的订单</text><text class="page-description">每一份投入，都通向新的成长。</text></view>
    <view class="order-tabs" role="tablist" aria-label="订单状态"><button v-for="item in filters" :key="item.value" :class="['order-tab', { active: filter === item.value }]" :disabled="!!payingId" role="tab" :aria-selected="filter === item.value" @click="selectFilter(item.value)">{{ item.label }}</button></view>
    <view v-if="loading" class="state-panel"><NxIcon name="clock" :size="30" /><text>正在同步订单…</text></view>
    <view v-else-if="loadError" class="state-panel"><text class="state-title">订单暂未加载</text><text class="muted">{{ loadError }}</text><button class="outline-button" @click="refresh">重新加载</button></view>
    <view v-else-if="!orders.length" class="state-panel"><NxIcon name="book" :size="34" /><text class="state-title">{{ filter ? '暂时没有这类订单' : '你的学习旅程，从这里开始' }}</text><text class="muted">{{ filter ? '可以切换到全部订单查看。' : '购买课程后，可在这里查看支付与报名信息。' }}</text><button v-if="!filter" class="outline-button" @click="browseCourses">看看老师的课程</button></view>
    <block v-else>
      <view v-for="order in orders" :key="order.id" class="order-card">
        <view class="order-top"><text>{{ productLabel(order.product) }}</text><text :class="['order-status', `order-status--${order.status}`]">{{ statusLabel(order.status) }}</text></view>
        <view class="order-body"><image v-if="order.cover && !failedCovers[order.id]" class="order-cover" :src="order.cover" mode="aspectFill" @error="failedCovers[order.id] = true" /><view v-else class="order-cover order-cover--empty"><NxIcon name="book" :size="28" /></view><view class="order-copy"><text class="order-title">{{ order.title || productLabel(order.product) }}</text><text class="order-date">{{ order.createTime }} 下单</text><text v-if="order.status === 'paid' && order.paidAt" class="order-date">{{ order.paidAt }} 确认</text></view></view>
        <text class="order-number">订单号 {{ order.outTradeNo }}</text>
        <view class="order-bottom"><view><text class="amount-label">{{ order.status === 'paid' ? '实付' : '订单金额' }}</text><text class="order-amount"><text>¥</text>{{ amountYuan(order.amount) }}</text></view><view class="order-actions"><button v-if="order.product === 'course_booking' && (order.courseId || (order.status === 'paid' && order.bookingId))" :class="['small-button', { 'small-button--primary': order.status === 'paid' }]" :disabled="!!payingId" @click="openCourse(order)">{{ order.status === 'paid' ? '查看课程' : '课程介绍' }}</button><button v-if="order.product === 'course_booking' && order.bookingId" class="small-button" :disabled="!!payingId" @click="viewPaymentResult(order)">{{ order.status === 'paid' ? '付款记录' : '支付结果' }}</button><button v-if="canContinue(order)" class="small-button small-button--primary" :loading="payingId === String(order.id)" :disabled="!!payingId || loadingMore" @click="continuePayment(order)">{{ payingId === String(order.id) ? '确认中' : '继续支付' }}</button></view></view>
        <text v-if="payingId === String(order.id) && paymentMessage" class="payment-message" role="status">{{ paymentMessage }}</text>
      </view>
      <view v-if="moreError" class="load-more"><text class="muted">{{ moreError }}</text><button :disabled="!!payingId" class="text-button" @click="loadMore">重试加载</button></view>
      <button v-else-if="hasMore" class="text-button load-more" :loading="loadingMore" :disabled="loadingMore || !!payingId" @click="loadMore">{{ loadingMore ? '正在加载…' : '加载更多订单' }}</button>
      <text v-else class="list-ending">已显示全部订单</text>
    </block>
  </view>
</template>

<style scoped>
.orders-page{box-sizing:border-box;min-height:100vh;padding:36rpx 32rpx calc(48rpx + env(safe-area-inset-bottom));background:var(--nx-page-bg);color:var(--nx-text)}button{margin:0;border:0;line-height:1.5;box-sizing:border-box}button::after{border:0}button[disabled]{opacity:.5}.orders-header{padding:10rpx 4rpx 32rpx}.eyebrow{display:block;color:var(--nx-brand-700);font-size:19rpx;letter-spacing:3rpx}.page-title{display:block;margin-top:14rpx;font-family:'Songti SC','STSong',serif;font-size:46rpx;line-height:1.4}.page-description{display:block;margin-top:14rpx;color:var(--nx-text-muted);font-size:24rpx;line-height:1.7}.order-tabs{display:flex;gap:20rpx;margin-bottom:28rpx;border-bottom:1rpx solid var(--nx-border)}.order-tab{flex:1;min-height:88rpx;padding:18rpx 0;background:transparent;color:var(--nx-text-muted);font-size:25rpx;border-radius:0}.order-tab.active{border-bottom:4rpx solid var(--nx-brand-700);color:var(--nx-brand-700);font-weight:600}.order-card{padding:28rpx;margin-bottom:24rpx;background:var(--nx-surface);border:1rpx solid var(--nx-border);border-radius:22rpx}.order-top{display:flex;justify-content:space-between;align-items:center;gap:16rpx;font-size:21rpx;color:var(--nx-text-muted)}.order-status{color:var(--nx-text-muted)}.order-status--pending{color:var(--nx-brand-700)}.order-status--paid{color:#63744B}.order-body{display:flex;gap:22rpx;margin:28rpx 0}.order-cover{flex:0 0 136rpx;width:136rpx;height:136rpx;background:#EFEADF;border-radius:12rpx}.order-cover--empty{display:flex;align-items:center;justify-content:center}.order-copy{min-width:0;flex:1}.order-title{display:block;font-size:28rpx;font-weight:500;line-height:1.55;word-break:break-word}.order-date{display:block;margin-top:11rpx;color:var(--nx-text-muted);font-size:20rpx;line-height:1.6}.order-number{display:block;font-size:19rpx;line-height:1.6;color:var(--nx-text-muted);word-break:break-all}.order-bottom{display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:14rpx;padding-top:22rpx;margin-top:18rpx;border-top:1rpx solid var(--nx-border)}.amount-label{font-size:20rpx;color:var(--nx-text-muted);margin-right:12rpx}.order-amount{font-size:33rpx;color:var(--nx-brand-700);font-family:Georgia,serif}.order-amount>text{font-size:22rpx;margin-right:4rpx}.order-actions{display:flex;flex-wrap:wrap;justify-content:flex-end;gap:12rpx;margin-left:auto}.small-button{display:flex;align-items:center;justify-content:center;min-height:76rpx;padding:14rpx 19rpx;border:1rpx solid var(--nx-border);border-radius:12rpx;background:transparent;font-size:22rpx;color:var(--nx-text)}.small-button--primary{background:var(--nx-brand-700);border-color:var(--nx-brand-700);color:#fff}.payment-message{display:block;margin-top:18rpx;color:var(--nx-brand-700);font-size:22rpx;line-height:1.6}.state-panel{display:flex;flex-direction:column;align-items:center;gap:22rpx;padding:70rpx 28rpx;text-align:center;background:var(--nx-surface);border-radius:22rpx;font-size:26rpx}.state-title{font-size:30rpx;line-height:1.6}.muted{color:var(--nx-text-muted);font-size:23rpx;line-height:1.8}.outline-button{min-height:88rpx;padding:20rpx 30rpx;background:transparent;border:1rpx solid var(--nx-border);border-radius:12rpx;font-size:25rpx;color:var(--nx-brand-700)}.text-button{display:flex;align-items:center;justify-content:center;min-height:88rpx;background:transparent;color:var(--nx-brand-700);font-size:24rpx}.load-more{width:100%;text-align:center}.list-ending{display:block;text-align:center;font-size:21rpx;color:var(--nx-text-muted);padding:20rpx 0 0}@media(min-width:600px){.orders-page{max-width:800rpx;margin:auto}}@media(prefers-reduced-motion:reduce){.orders-page{scroll-behavior:auto!important}}
</style>
