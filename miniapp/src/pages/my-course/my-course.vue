<script setup>
import { computed, ref } from 'vue'
import { onLoad, onShow, onHide, onUnload, onPullDownRefresh } from '@dcloudio/uni-app'
import NxIcon from '../../components/NxIcon.vue'
import NxImagePreview from '../../components/NxImagePreview.vue'
import { getToken } from '../../utils/auth'
import { getCourseEnrollmentApi } from '../../api'
import { previewImage, isWechatDevtools } from '../../utils/imagePreview'
import { navigateToOrders, navigateToPaymentResult } from '../../utils/courseNavigation'

const enrollment = ref(null)
const viewState = ref('loading')
const loading = ref(false)
const coverFailed = ref(false)
const previewVisible = ref(false)
const previewSrc = ref('')
const previewAlt = ref('图片预览')
let query = null
let sessionToken = ''
let ticket = 0
let active = false
let disposed = false
let loginRedirecting = false
let resultNavigating = false

const text = value => typeof value === 'string' ? value.trim() : ''
const isOwned = computed(() => viewState.value === 'owned' && enrollment.value?.owned === true && enrollment.value?.order?.status === 'paid')
const course = computed(() => enrollment.value?.course || {})
const order = computed(() => enrollment.value?.order || {})
const title = computed(() => text(order.value.title) || text(course.value.title) || '我的课程')
const amountText = computed(() => Number.isSafeInteger(order.value.amount) && order.value.amount >= 0 ? (order.value.amount / 100).toFixed(2) : '—')
const schedule = computed(() => text(course.value.schedule) || '待通知')
const location = computed(() => text(course.value.location) || '待通知')
const duration = computed(() => text(course.value.duration) || '待通知')
const outline = computed(() => Array.isArray(course.value.outline) ? course.value.outline.map(text).filter(Boolean) : [])
const highlights = computed(() => Array.isArray(course.value.bullets) ? course.value.bullets.map(text).filter(Boolean) : [])
const notice = computed(() => text(course.value.notice))
const contactQr = computed(() => text(enrollment.value?.customerServiceQr))
const stateTitle = computed(() => ({ loading: '正在确认课程报名', login: '请登录后查看', unavailable: '暂未找到课程报名', error: '课程信息暂未加载' }[viewState.value] || ({ closed: '订单已关闭', refunded: '订单已退款', cancelled: '订单已取消' }[order.value.status] || '报名状态确认中')))
const stateCopy = computed(() => ({ loading: '正在读取你的报名凭证与课程安排。', login: '登录状态已更新，请登录后从我的订单进入。', unavailable: '请在我的订单中查看当前账号的课程报名记录。', error: '网络暂时中断，请刷新后查看课程安排。' }[viewState.value] || '请查看支付结果，报名状态以后台确认为准。如已扣款，请勿重复支付。'))

function clearDetails() {
  enrollment.value = null
  coverFailed.value = false
  previewVisible.value = false
  previewSrc.value = ''
}
function invalidate() { ticket++; loading.value = false; clearDetails() }
function goLogin() {
  if (disposed || loginRedirecting) return
  loginRedirecting = true
  uni.switchTab({ url: '/pages/profile/profile', fail: () => { loginRedirecting = false } })
}
function currentSession() {
  if (sessionToken && getToken() === sessionToken) return true
  invalidate()
  sessionToken = ''
  query = null
  viewState.value = 'login'
  goLogin()
  return false
}
function currentRequest(run) { return active && !disposed && run === ticket }
function validatedEnrollment(value) {
  if (!value || typeof value !== 'object') throw new Error('Missing enrollment')
  if (query.bookingId && String(value.bookingId) !== query.bookingId) throw new Error('Booking mismatch')
  if (query.courseId && String(value.courseId) !== query.courseId) throw new Error('Course mismatch')
  if (value.order && !/^[1-9]\d*$/.test(String(value.bookingId))) throw new Error('Missing booking identity')
  if (value.order && (String(value.order.bookingId) !== String(value.bookingId) || String(value.order.courseId) !== String(value.courseId))) throw new Error('Order mismatch')
  return value
}
async function loadEnrollment() {
  if (disposed || !active || !currentSession()) return
  if (!query) { viewState.value = 'unavailable'; return }
  const run = ++ticket
  clearDetails()
  viewState.value = 'loading'
  loading.value = true
  try {
    const result = await getCourseEnrollmentApi(query)
    if (!currentRequest(run) || !currentSession()) return
    enrollment.value = validatedEnrollment(result)
    if (result.owned === true && result.order?.status === 'paid') {
      viewState.value = 'owned'
    } else if (result.order && /^[1-9]\d*$/.test(String(result.bookingId))) {
      viewState.value = 'pending'
      goPaymentResult(true)
    } else {
      viewState.value = 'unavailable'
    }
  } catch (error) {
    if (!currentRequest(run) || !currentSession()) return
    clearDetails()
    if (error?.authExpired || error?.authRequired || Number(error?.statusCode) === 401) {
      sessionToken = ''
      currentSession()
    } else {
      viewState.value = [403, 404].includes(Number(error?.statusCode)) ? 'unavailable' : 'error'
    }
  } finally {
    if (currentRequest(run)) loading.value = false
  }
}
function goOrders() {
  if (!disposed && currentSession()) navigateToOrders()
}
function goPaymentResult(replace = false) {
  if (disposed || !currentSession() || resultNavigating) return
  const id = String(enrollment.value?.bookingId || '')
  if (!/^[1-9]\d*$/.test(id)) return
  resultNavigating = true
  navigateToPaymentResult(id, { replace: replace === true, fail: () => { resultNavigating = false } })
}
function showImage(src, alt) {
  if (disposed || !currentSession() || !isOwned.value || !text(src)) return
  if (isWechatDevtools()) {
    previewSrc.value = src
    previewAlt.value = alt
    previewVisible.value = true
  } else previewImage(src)
}
function previewCover() {
  if (!coverFailed.value) showImage(course.value.cover, `${title.value}封面`)
}
function previewContact() { showImage(contactQr.value, '工作室客服二维码') }
function closePreview() { previewVisible.value = false }
function openTeacher() {
  if (!disposed && currentSession() && isOwned.value) uni.navigateTo({ url: '/pages/teacher/teacher' })
}

onLoad(params => {
  const bookingId = String(params?.bookingId || '').trim()
  const courseId = String(params?.courseId || '').trim()
  if (bookingId && !courseId && /^[1-9]\d*$/.test(bookingId)) query = { bookingId }
  else if (courseId && !bookingId && /^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$/.test(courseId)) query = { courseId }
  sessionToken = getToken()
})
onShow(() => {
  if (disposed) return
  // #ifdef MP-WEIXIN
  if (typeof uni !== 'undefined' && typeof uni.hideShareMenu === 'function') uni.hideShareMenu({ menus: ['shareAppMessage', 'shareTimeline'] })
  // #endif
  active = true
  resultNavigating = false
  return loadEnrollment()
})
onHide(() => { active = false; invalidate(); viewState.value = 'loading' })
onUnload(() => { disposed = true; active = false; invalidate(); query = null; sessionToken = '' })
onPullDownRefresh(async () => { try { await loadEnrollment() } finally { uni.stopPullDownRefresh() } })
</script>

<template>
  <view class="my-course-page">
    <view class="page-heading"><text class="eyebrow">MY COURSE</text><text class="page-label">我的课程</text></view>
    <view v-if="!isOwned" class="state-card" aria-live="polite">
      <NxIcon :name="viewState === 'loading' ? 'clock' : 'book'" :size="36" />
      <text class="state-title">{{ stateTitle }}</text><text class="body-copy">{{ stateCopy }}</text>
      <button v-if="viewState === 'login'" class="primary-button" @click="goLogin">登录后查看</button>
      <block v-else-if="viewState !== 'loading'">
        <button v-if="viewState === 'error'" class="primary-button" :loading="loading" :disabled="loading" @click="loadEnrollment">刷新课程信息</button>
        <button v-else-if="viewState === 'pending'" class="primary-button" @click="goPaymentResult()">查看支付结果</button>
        <button class="secondary-button" @click="goOrders">查看我的订单</button>
      </block>
    </view>
    <block v-else>
      <view class="course-card">
        <button v-if="course.cover && !coverFailed" class="cover-action" aria-label="预览课程封面" @click="previewCover"><image class="course-cover" :src="course.cover" mode="widthFix" :aria-label="title" @error="coverFailed = true" /><view class="cover-caption"><text>查看原图</text><NxIcon name="arrow" :size="15" color="#FFFFFF" /></view></button>
        <view class="course-intro">
          <view class="enrolled-badge"><NxIcon name="check" :size="14" color="#687553" /><text>已报名</text></view>
          <text class="course-title">{{ title }}</text>
          <text v-if="course.subtitle" class="course-subtitle">{{ course.subtitle }}</text>
          <text v-if="course.format" class="course-format">{{ course.format }}</text>
        </view>
      </view>

      <view class="section-heading"><text class="section-title">相遇的安排</text><text class="section-kicker">COURSE ARRANGEMENTS</text></view>
      <view class="schedule-card">
        <view class="schedule-row"><text class="schedule-label">开课时间</text><text class="schedule-value">{{ schedule }}</text></view>
        <view class="schedule-row"><text class="schedule-label">上课地点</text><text class="schedule-value">{{ location }}</text></view>
        <view class="schedule-row"><text class="schedule-label">课程时长</text><text class="schedule-value">{{ duration }}</text></view>
      </view>
      <text v-if="enrollment.catalogAvailable === false" class="catalog-note">课程介绍正在更新，你的报名凭证已保留。最新安排请留意工作室通知。</text>

      <view class="content-section">
        <text class="section-title">这一次，我们一起学习</text>
        <text v-if="course.description" class="body-copy section-copy">{{ course.description }}</text>
        <view v-if="outline.length" class="outline-list"><view v-for="(item, index) in outline" :key="index" class="outline-row"><text class="outline-number">{{ String(index + 1).padStart(2, '0') }}</text><text class="outline-text">{{ item }}</text></view></view>
        <text v-else class="empty-copy">详细学习内容待工作室更新。</text>
        <view v-if="highlights.length" class="highlights"><view v-for="(item, index) in highlights" :key="index" class="highlight"><NxIcon name="check" :size="15" /><text>{{ item }}</text></view></view>
      </view>

      <view class="notice-card"><view class="notice-heading"><NxIcon name="book" :size="20" /><text class="notice-title">课前须知</text></view><text class="body-copy">{{ notice || '课前须知待通知。' }}</text></view>

      <view class="section-heading receipt-heading"><text class="section-title">你的报名凭证</text><text class="section-kicker">ENROLLMENT RECORD</text></view>
      <view class="receipt-card">
        <view class="receipt-amount"><text class="receipt-label">实付金额</text><view><text class="currency">¥</text><text class="amount">{{ amountText }}</text></view></view>
        <view class="receipt-row"><text class="receipt-label">报名状态</text><text class="receipt-value confirmed">已报名 · 已支付</text></view>
        <view v-if="order.outTradeNo" class="receipt-row"><text class="receipt-label">订单编号</text><text class="receipt-value">{{ order.outTradeNo }}</text></view>
        <view v-if="order.paidAt" class="receipt-row"><text class="receipt-label">确认时间</text><text class="receipt-value">{{ order.paidAt }}</text></view>
        <button class="receipt-link" @click="goPaymentResult()">查看付款记录 <NxIcon name="arrow" :size="16" /></button>
      </view>

      <button v-if="contactQr" class="contact-card" @click="previewContact"><view class="contact-symbol"><NxIcon name="message" :size="24" /></view><view class="contact-copy"><text class="contact-title">和工作室保持联系</text><text class="contact-hint">查看客服二维码，沟通课程安排</text></view><NxIcon name="arrow" :size="19" /></button>
      <view class="page-actions"><button class="primary-button" @click="goOrders">返回我的订单 <NxIcon name="arrow" :size="18" color="#FFFFFF" /></button><button class="text-button" @click="openTeacher">查看老师主页</button></view>
      <view class="page-ending"><view /><text>带着好奇，期待相遇</text><view /></view>
    </block>
    <NxImagePreview :visible="previewVisible" :src="previewSrc" :alt="previewAlt" @close="closePreview" />
  </view>
</template>

<style scoped>
.my-course-page{box-sizing:border-box;min-height:100vh;padding:34rpx 32rpx calc(44rpx + env(safe-area-inset-bottom));background:var(--nx-page-bg,#F7F4ED);color:var(--nx-text,#38372F)}
button{box-sizing:border-box;margin:0;border:0;line-height:1.5}button::after{border:0}.page-heading{padding:6rpx 4rpx 26rpx}.eyebrow{display:block;font-size:18rpx;letter-spacing:3rpx;color:var(--nx-brand-700,#A55C3B)}.page-label{display:block;margin-top:11rpx;font-family:'Songti SC','STSong',serif;font-size:42rpx;line-height:1.45}.course-card{border:1rpx solid var(--nx-border,#E4DED1);border-radius:24rpx;overflow:hidden;background:var(--nx-surface,#FFFFFF)}.cover-action{display:block;position:relative;width:100%;height:270rpx;overflow:hidden;padding:0;border-radius:0;background:#E6E0D4}.course-cover{display:block;width:100%;height:auto}.cover-caption{position:absolute;right:18rpx;bottom:17rpx;display:flex;align-items:center;gap:8rpx;padding:7rpx 13rpx;border-radius:8rpx;background:rgba(28,29,23,.44);color:#FFFFFF;font-size:18rpx}.course-intro{padding:27rpx 28rpx 30rpx}.enrolled-badge{display:inline-flex;align-items:center;gap:8rpx;padding:7rpx 14rpx;border-radius:7rpx;background:#EEF0E8;color:#687553;font-size:21rpx}.course-title{display:block;margin-top:18rpx;font-family:'Songti SC','STSong',serif;font-size:39rpx;font-weight:600;line-height:1.5;overflow-wrap:anywhere}.course-subtitle{display:block;margin-top:13rpx;font-size:24rpx;color:var(--nx-text-muted,#858073);line-height:1.8}.course-format{display:block;margin-top:18rpx;font-size:21rpx;letter-spacing:1rpx;color:var(--nx-brand-700,#A55C3B)}.section-heading{display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:12rpx;margin:36rpx 2rpx 20rpx}.section-title{display:block;font-family:'Songti SC','STSong',serif;font-size:32rpx;line-height:1.5}.section-kicker{font-size:15rpx;letter-spacing:1.5rpx;color:#958979}.schedule-card{padding:9rpx 27rpx;border:1rpx solid var(--nx-border,#E4DED1);border-radius:20rpx;background:var(--nx-surface,#FFFFFF)}.schedule-row{display:flex;align-items:flex-start;gap:25rpx;padding:23rpx 0;border-bottom:1rpx solid var(--nx-border,#E4DED1);font-size:24rpx;line-height:1.75}.schedule-row:last-child{border-bottom:0}.schedule-label{flex:0 0 104rpx;color:var(--nx-text-muted,#858073)}.schedule-value{flex:1;min-width:0;text-align:right;white-space:pre-wrap;overflow-wrap:anywhere}.catalog-note{display:block;margin-top:18rpx;padding:19rpx 22rpx;border-radius:12rpx;background:#EEE9DF;font-size:22rpx;line-height:1.8;color:#857866}.content-section{padding:37rpx 3rpx 30rpx}.body-copy{display:block;color:var(--nx-text-muted,#858073);font-size:24rpx;line-height:1.9;white-space:pre-wrap;overflow-wrap:anywhere}.section-copy{margin-top:18rpx}.outline-list{margin-top:21rpx;border-top:1rpx solid var(--nx-border,#E4DED1)}.outline-row{display:flex;gap:23rpx;align-items:flex-start;padding:25rpx 0;border-bottom:1rpx solid var(--nx-border,#E4DED1)}.outline-number{flex:0 0 42rpx;font-family:Georgia,serif;color:#AB927A;font-size:29rpx;line-height:1.6}.outline-text{flex:1;min-width:0;font-size:25rpx;line-height:1.85;white-space:pre-wrap;overflow-wrap:anywhere}.empty-copy{display:block;margin-top:20rpx;font-size:24rpx;color:var(--nx-text-muted,#858073);line-height:1.8}.highlights{display:flex;flex-direction:column;gap:15rpx;margin-top:24rpx}.highlight{display:flex;align-items:flex-start;gap:12rpx;font-size:23rpx;line-height:1.7;color:var(--nx-text-muted,#858073)}.notice-card{padding:27rpx;border-radius:20rpx;background:#EFEADF}.notice-heading{display:flex;align-items:center;gap:12rpx;margin-bottom:16rpx}.notice-title{font-size:27rpx}.receipt-heading{margin-top:35rpx}.receipt-card{padding:9rpx 27rpx 0;border:1rpx solid var(--nx-border,#E4DED1);border-radius:20rpx;background:var(--nx-surface,#FFFFFF)}.receipt-amount{display:flex;justify-content:space-between;align-items:center;gap:22rpx;padding:24rpx 0;border-bottom:1rpx solid var(--nx-border,#E4DED1)}.receipt-label{flex-shrink:0;font-size:23rpx;color:var(--nx-text-muted,#858073)}.currency{margin-right:6rpx;font-size:24rpx;color:var(--nx-brand-700,#A55C3B)}.amount{font-family:Georgia,serif;font-size:43rpx;color:var(--nx-brand-700,#A55C3B)}.receipt-row{display:flex;justify-content:space-between;align-items:flex-start;gap:24rpx;padding-top:21rpx;font-size:22rpx;line-height:1.7}.receipt-value{min-width:0;text-align:right;word-break:break-all}.confirmed{color:#687553}.receipt-link{display:flex;align-items:center;justify-content:flex-end;gap:8rpx;min-height:90rpx;width:100%;margin-top:14rpx;padding:19rpx 0;background:transparent;color:var(--nx-brand-700,#A55C3B);font-size:22rpx}.contact-card{display:flex;align-items:center;gap:18rpx;width:100%;padding:24rpx;margin-top:28rpx;border-radius:18rpx;border:1rpx solid var(--nx-border,#E4DED1);background:transparent;text-align:left}.contact-symbol{display:flex;align-items:center;justify-content:center;flex:0 0 64rpx;height:64rpx;background:#EEE6D9;border-radius:50%}.contact-copy{flex:1;min-width:0}.contact-title{display:block;font-size:25rpx;color:var(--nx-text,#38372F)}.contact-hint{display:block;margin-top:6rpx;color:var(--nx-text-muted,#858073);font-size:20rpx;line-height:1.7}.page-actions{margin-top:30rpx}.primary-button,.secondary-button{display:flex;align-items:center;justify-content:center;gap:14rpx;width:100%;min-height:94rpx;padding:22rpx;border-radius:14rpx;font-size:26rpx}.primary-button{background:var(--nx-brand-700,#A55C3B);color:#FFFFFF}.secondary-button{background:transparent;border:1rpx solid var(--nx-border,#E4DED1);color:var(--nx-brand-700,#A55C3B)}.text-button{display:flex;align-items:center;justify-content:center;width:100%;min-height:86rpx;padding:18rpx;background:transparent;color:var(--nx-text-muted,#858073);font-size:23rpx}.page-ending{display:flex;align-items:center;gap:17rpx;padding:15rpx 3rpx 0;font-size:20rpx;color:#999183;letter-spacing:1rpx}.page-ending view{height:1rpx;flex:1;background:var(--nx-border,#E4DED1)}.state-card{display:flex;flex-direction:column;align-items:center;gap:23rpx;padding:55rpx 28rpx;border:1rpx solid var(--nx-border,#E4DED1);border-radius:22rpx;background:var(--nx-surface,#FFFFFF);text-align:center}.state-title{font-family:'Songti SC','STSong',serif;font-size:34rpx;line-height:1.5}.state-card .primary-button,.state-card .secondary-button{margin-top:8rpx}
@media(min-width:600px){.my-course-page{max-width:800rpx;margin:auto}}
</style>
