<script setup>
import { computed, ref } from 'vue'
import { onLoad, onShow, onHide, onUnload, onShareAppMessage, onShareTimeline } from '@dcloudio/uni-app'
import NxIcon from '../../components/NxIcon.vue'
import NxImagePreview from '../../components/NxImagePreview.vue'
import NxShareActions from '../../components/NxShareActions.vue'
import { buildShareCard, showPublicShareMenu, isTimelinePreview, requireFullMiniapp } from '../../utils/share'
import { STUDIO_COURSES, STUDIO_TEACHER } from '../../data/teacherStudio'
import { UI_PREVIEW } from '../../utils/uiPreview'
import { getCachedSiteConfig, getStoredSiteConfig } from '../../utils/siteConfig'
import { normalizeMiniappCourses, normalizeTeachers } from '../../utils/teacherCourseware'
import { setBookingIntent } from '../../utils/bookingIntent'
import { previewImage } from '../../utils/imagePreview'
import { isWechatDevtools } from '../../utils/imagePreview'
import { ensureLogin, getToken } from '../../utils/auth'
import { createCourseOrderApi, devPayCourseBookingOrderApi, getCourseBookingOrderStatusApi, getCourseEnrollmentApi } from '../../api'
import { payWechatOrder } from '../../utils/payment'
import { createCoursePaymentController, coursePaymentResultUrl } from '../../utils/coursePayment'
import { userErrorMessage } from '../../utils/userMessage'

const timelinePreview = isTimelinePreview()
const courseId = ref('')
const config = ref(getStoredSiteConfig() || {})
const loading = ref(false)
const paying = ref(false)
const enrollment = ref(null)
const enrollmentLoading = ref(false)
const enrollmentError = ref('')
const enrollmentChecked = ref(timelinePreview || UI_PREVIEW || !getToken())
let paymentController = null
let active = true
let sharePageVisible = true
onHide(() => { sharePageVisible = false })
let enrollmentToken = timelinePreview ? '' : getToken()
let enrollmentTicket = 0
onUnload(() => {
  active = false
  sharePageVisible = false
  paymentController?.stop()
  enrollmentTicket += 1
  enrollment.value = null
  enrollmentLoading.value = false
  enrollmentError.value = ''
  enrollmentToken = ''
})
onShow(() => {
  if (!active) return
  sharePageVisible = true
  showPublicShareMenu(courseShareable.value)
  if (paying.value) {
    if (enrollmentToken !== getToken()) resetEnrollmentSession()
    paymentController?.resume()
    return
  }
  return refreshEnrollment()
})
const activeSection = ref('intro')
const sections = [{ id: 'intro', label: '课程介绍' }, { id: 'outline', label: '学习内容' }, { id: 'notice', label: '报名须知' }]
const courses = computed(() => UI_PREVIEW ? STUDIO_COURSES : normalizeMiniappCourses(config.value))
const course = computed(() => courses.value.find((item) => String(item.id) === courseId.value))
const courseShareable = computed(() => !loading.value && !!course.value && /^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$/.test(courseId.value))
function courseShareCard() {
  if (!courseShareable.value) return buildShareCard({ kind: 'home' })
  return buildShareCard({ kind: 'course', id: course.value.id, title: course.value.title, imageUrl: course.value.cover })
}
onShareAppMessage(() => courseShareCard().appMessage)
onShareTimeline(() => courseShareCard().timeline)
const teacher = computed(() => UI_PREVIEW ? STUDIO_TEACHER : normalizeTeachers(config.value)[0])
const teacherAvatar = computed(() => {
  const avatar = teacher.value?.avatar || ''
  const isLaohan = ['韩常青', '韩常青（老韩）', '韩常青(老韩)', '老韩', '韩老师'].includes(teacher.value?.name)
  if (isLaohan && (!avatar || /\/avatars\/|teacher-poster/i.test(avatar))) return '/static/teacher/portrait.jpg'
  return /\/avatars\//i.test(avatar) ? '' : avatar
})
const teacherAvatarFailed = ref(false)
const teacherAvatarPreviewVisible = ref(false)
const courseCoverFailed = ref(false)
const courseCoverPreviewVisible = ref(false)
const highlights = computed(() => Array.isArray(course.value?.highlights) ? course.value.highlights : [])
const outline = computed(() => Array.isArray(course.value?.outline) ? course.value.outline : [])
const isEnrolled = computed(() => enrollment.value?.owned === true && enrollment.value?.order?.status === 'paid')
const enrollmentPending = computed(() => !enrollmentChecked.value || enrollmentLoading.value)
const confirmationPending = computed(() => !isEnrolled.value && enrollment.value?.syncStatus === 'retrying')
const enrollmentActionDisabled = computed(() => paying.value || enrollmentPending.value)
const enrollmentActionText = computed(() => {
  if (paying.value) return '确认支付中'
  if (enrollmentPending.value) return '确认报名状态'
  if (enrollmentError.value) return '重试报名状态'
  if (isEnrolled.value) return '查看我的课程'
  if (confirmationPending.value) return enrollment.value?.bookingId ? '查看支付结果' : '刷新支付结果'
  return course.value?.paymentMode === 'paid' ? '立即支付' : '咨询老师'
})
const enrollmentCaption = computed(() => {
  if (enrollmentPending.value) return '正在确认你的课程报名'
  if (enrollmentError.value) return '请重试后查看报名状态'
  if (isEnrolled.value) return '查看课程安排与报名信息'
  if (confirmationPending.value) return '请勿重复支付，稍后刷新结果'
  return UI_PREVIEW ? '演示价格 · 无需在线支付' : course.value?.paymentMode === 'paid' ? '在线支付后确认报名' : '提交意向后确认安排'
})
onLoad(async (query) => {
  courseId.value = String(query?.id || '')
  if (UI_PREVIEW) { showPublicShareMenu(courseShareable.value); return }
  loading.value = true
  showPublicShareMenu(false)
  try { const updated = await getCachedSiteConfig(); if (active) config.value = updated || {} } catch { /* Keep cached content visible. */ }
  finally {
    if (active) {
      loading.value = false
      if (sharePageVisible) showPublicShareMenu(courseShareable.value)
    }
  }
})
function resetEnrollmentSession() {
  enrollmentTicket += 1
  enrollment.value = null
  enrollmentLoading.value = false
  enrollmentChecked.value = true
  enrollmentError.value = '登录状态已更新，请重试报名状态'
  enrollmentToken = getToken()
}
function validEnrollment(value, requestedCourse) {
  if (!value || value.courseId !== requestedCourse || typeof value.owned !== 'boolean' || !['confirmed', 'retrying'].includes(value.syncStatus)) {
    throw new Error('报名状态暂未确认，请重试')
  }
  const bookingId = String(value.bookingId || '')
  if (bookingId && !/^[1-9]\d*$/.test(bookingId)) throw new Error('报名信息暂未确认，请重试')
  if (value.owned && value.order?.status === 'paid') {
    if (!bookingId || String(value.order.bookingId) !== bookingId || value.order.courseId !== requestedCourse) {
      throw new Error('报名信息暂未确认，请重试')
    }
  } else if (value.order?.status === 'paid' || (value.owned && value.syncStatus !== 'retrying')) {
    throw new Error('报名状态暂未确认，请重试')
  }
  return { ...value, bookingId }
}
async function refreshEnrollment(whilePaying = false) {
  if (!active || (paying.value && !whilePaying)) return false
  if (timelinePreview) { enrollmentChecked.value = true; return true }
  const token = getToken()
  const requestedCourse = courseId.value
  const ticket = ++enrollmentTicket
  enrollmentToken = token
  enrollment.value = null
  enrollmentError.value = ''
  enrollmentLoading.value = false
  enrollmentChecked.value = UI_PREVIEW || !token
  if (UI_PREVIEW || !token) return true
  enrollmentLoading.value = true
  const current = () => active && ticket === enrollmentTicket && requestedCourse === courseId.value
  try {
    const result = await getCourseEnrollmentApi({ courseId: requestedCourse })
    if (!current()) return false
    if (getToken() !== token) { resetEnrollmentSession(); return false }
    enrollment.value = validEnrollment(result, requestedCourse)
    enrollmentChecked.value = true
    return true
  } catch (error) {
    if (!current()) return false
    if (getToken() !== token) { resetEnrollmentSession(); return false }
    enrollmentError.value = userErrorMessage(error, '报名状态暂未确认，请重试')
    enrollmentChecked.value = true
    return false
  } finally {
    if (current()) enrollmentLoading.value = false
  }
}
function openMyCourse() {
  if (!requireFullMiniapp()) return
  if (!active) return
  if (enrollmentToken !== getToken()) { resetEnrollmentSession(); return }
  if (!isEnrolled.value || !enrollment.value?.bookingId) return
  uni.navigateTo({ url: `/pages/my-course/my-course?bookingId=${encodeURIComponent(enrollment.value.bookingId)}` })
}
function showPendingPayment() {
  if (!requireFullMiniapp()) return
  if (!active || enrollmentToken !== getToken()) { if (active) resetEnrollmentSession(); return }
  const url = coursePaymentResultUrl(enrollment.value)
  if (url) uni.navigateTo({ url })
  else return refreshEnrollment()
}
function goBack() {
  if (!requireFullMiniapp()) return
  uni.navigateBack({ fail: () => uni.switchTab({ url: '/pages/booking/booking' }) })
}
function chooseSection(section) {
  activeSection.value = section
  uni.pageScrollTo({ selector: `#section-${section}`, duration: 250, offsetTop: -24 })
}
async function enroll() {
  if (!requireFullMiniapp()) return
  if (!active || paying.value) return
  if (enrollmentToken !== getToken()) { await refreshEnrollment(); return }
  if (enrollmentPending.value) return
  if (enrollmentError.value) { await refreshEnrollment(); return }
  if (isEnrolled.value) { openMyCourse(); return }
  if (confirmationPending.value) { return showPendingPayment() }
  if (!course.value) return
  if (course.value.priceCents > 0) {
    if (UI_PREVIEW || typeof window !== 'undefined') {
      uni.showToast({ title: '请在微信小程序内完成支付', icon: 'none' })
      return
    }
    paying.value = true
    let paymentToken = ''
    try {
      await ensureLogin()
      if (!active) return
      paymentToken = getToken()
      if (!paymentToken) return
      if (enrollmentToken !== paymentToken || !enrollment.value || enrollmentError.value) {
        if (!await refreshEnrollment(true) || !active || getToken() !== paymentToken) return
        if (isEnrolled.value) { openMyCourse(); return }
        if (confirmationPending.value) { return showPendingPayment() }
      }
      const selectedId = course.value.id
      const requireSession = () => {
        if (!active || !paymentToken || getToken() !== paymentToken) throw new Error('登录状态已更新，请重新支付')
      }
      paymentController = createCoursePaymentController({
        isCurrent: () => active && getToken() === paymentToken,
        create: () => { requireSession(); return createCourseOrderApi(selectedId) },
        pay: (order) => {
          requireSession()
          return payWechatOrder(order, { devPay: (pending) => devPayCourseBookingOrderApi(pending.outTradeNo) })
        },
        status: (order) => { requireSession(); return getCourseBookingOrderStatusApi(order.bookingId) },
      })
      const result = await paymentController.purchase()
      if (!active || getToken() !== paymentToken) return
      const url = coursePaymentResultUrl(result?.order)
      if (url) uni.navigateTo({ url })
    } catch (error) {
      if (active && (!paymentToken || getToken() === paymentToken)) {
        uni.showToast({ title: userErrorMessage(error, '订单状态待确认，请到我的订单查看'), icon: 'none' })
      }
    } finally { paying.value = false; paymentController = null }
    return
  }
  const detail = course.value.schedule ? `${course.value.title} · ${course.value.schedule}` : course.value.title
  const saved = setBookingIntent({ kind: 'course', courseId: course.value.id, intentText: detail })
  if (!saved) {
    uni.showToast({ title: '本机存储暂不可用，请在报名页填写课程名称', icon: 'none' })
  }
  uni.switchTab({ url: '/pages/booking/booking' })
}
function teacherDetail() { if (!requireFullMiniapp()) return; uni.navigateTo({ url: '/pages/teacher/teacher' }) }
function previewTeacherAvatar() {
  if (teacherAvatarFailed.value) return
  if (isWechatDevtools()) {
    teacherAvatarPreviewVisible.value = true
    return
  }
  previewImage(teacherAvatar.value)
}
function closeTeacherAvatarPreview() { teacherAvatarPreviewVisible.value = false }
function previewCourseCover() {
  const cover = course.value?.cover || ''
  if (!cover || courseCoverFailed.value) return
  if (isWechatDevtools()) {
    courseCoverPreviewVisible.value = true
    return
  }
  previewImage(cover)
}
function closeCourseCoverPreview() { courseCoverPreviewVisible.value = false }
function outlineTitle(item) { return typeof item === 'string' ? item : item.title || item.name || '主题练习' }
function outlineDescription(item) { return typeof item === 'object' && item ? item.description || item.content || '' : '' }
</script>

<template>
  <view class="course-detail-page">
    <view v-if="(loading || enrollmentPending) && !course && !isEnrolled" class="empty-state"><text class="empty-title">{{ enrollmentPending ? '正在确认课程报名…' : '正在整理课程信息…' }}</text></view>
    <view v-else-if="!course" class="empty-state">
      <NxIcon :name="isEnrolled ? 'check' : 'book'" :size="38" />
      <text class="empty-title">{{ isEnrolled ? enrollment.order.title || '已报名课程' : enrollmentError ? '报名状态待确认' : confirmationPending ? '支付结果确认中' : '这门课程暂未开放' }}</text>
      <text class="muted-copy">{{ isEnrolled ? '你已报名这门课程，可继续查看报名信息与课程安排。' : enrollmentError || (confirmationPending ? '正在同步微信支付结果，请勿重复支付。' : '可以先看看其他学习方向，或联系工作室了解安排。') }}</text>
      <button v-if="isEnrolled" class="primary-button" @click="openMyCourse">查看我的课程</button>
      <button v-else-if="enrollmentError || confirmationPending" class="primary-button" :disabled="enrollmentActionDisabled" :loading="enrollmentLoading" @click="enroll">{{ enrollmentActionText }}</button>
      <button v-else class="primary-button" @click="goBack">返回课程列表</button>
    </view>
    <block v-else>
      <view class="course-cover" role="button" aria-label="预览课程封面" @click="previewCourseCover"><image :src="course.cover" mode="aspectFill" class="cover-image" :aria-label="course.title" @error="courseCoverFailed = true" /><view class="cover-shade" /><view class="cover-caption"><text>认识自己，是一生的功课。</text><text class="caption-en">A JOURNEY TO YOURSELF</text></view></view>
      <view class="detail-content">
        <view class="course-heading"><view class="title-meta"><text>{{ course.tag || '主题课程' }}</text><text class="meta-divider" /><text>{{ course.format || '主题共学' }}</text></view><text class="course-title">{{ course.title }}</text><text class="course-subtitle">{{ course.subtitle || course.description }}</text><view class="course-info"><view><NxIcon name="calendar" :size="18" /><text>{{ course.schedule || '排期请咨询工作室' }}</text></view><view><NxIcon name="clock" :size="18" /><text>{{ course.duration }}</text></view></view><view v-if="course.location" class="location-line"><text>{{ course.location }}</text><text v-if="UI_PREVIEW" class="demo-badge">演示排期 · 以实际发布为准</text></view></view>
        <NxShareActions :disabled="!courseShareable" />
        <view class="section-tabs"><button v-for="section in sections" :key="section.id" :class="['section-tab', { active: activeSection === section.id }]" @click="chooseSection(section.id)">{{ section.label }}</button></view>
        <view id="section-intro" class="content-section"><text class="eyebrow">ABOUT THE COURSE</text><text class="section-title">从认识，到真正理解</text><text class="body-copy">{{ course.description }}</text><view v-if="highlights.length" class="highlight-list"><view v-for="item in highlights" :key="item" class="highlight-item"><view class="highlight-icon"><NxIcon name="check" :size="15" /></view><text>{{ item }}</text></view></view></view>
        <button v-if="teacher" class="teacher-card" @click="teacherDetail"><view v-if="teacherAvatar && !teacherAvatarFailed" class="teacher-avatar-action" :aria-label="`预览${teacher.name}老师头像`" @click.stop="previewTeacherAvatar"><image class="teacher-avatar" :src="teacherAvatar" mode="aspectFill" :aria-label="teacher.name" @error="teacherAvatarFailed = true" /></view><view v-else class="teacher-avatar teacher-avatar--fallback">{{ teacher.name.slice(0, 1) }}</view><view class="teacher-copy"><text class="teacher-overline">你的学习向导</text><text class="teacher-name">{{ teacher.name }}</text><text class="teacher-title">{{ teacher.title }}</text></view><NxIcon name="arrow" :size="21" /></button>
        <view id="section-outline" class="content-section"><text class="eyebrow">LEARNING JOURNEY</text><text class="section-title">一步一步，把觉察带回日常</text><view v-if="outline.length" class="outline-list"><view v-for="(item, index) in outline" :key="index" class="outline-item"><text class="outline-number">0{{ index + 1 }}</text><view><text class="outline-title">{{ outlineTitle(item) }}</text><text v-if="outlineDescription(item)" class="outline-description">{{ outlineDescription(item) }}</text></view></view></view><view v-else class="outline-empty"><text class="body-copy">{{ course.description }}</text><text class="muted-copy">详细学习内容与课程安排，可在报名沟通时向工作室了解。</text></view></view>
        <view class="quote-note"><text class="quote-mark">“</text><view class="quote-copy"><text>学习不是为自己贴上标签，</text><text>而是为改变留出空间。</text></view><text class="quote-footer">让理解发生，让成长继续。</text></view>
        <view id="section-notice" class="content-section notice-section"><text class="eyebrow">BEFORE WE MEET</text><text class="section-title">相遇之前，你可能想知道</text><view class="notice-item"><text class="notice-label">如何报名</text><text class="body-copy">{{ course.notice || (course.paymentMode === 'paid' ? '完成微信支付后，可在我的订单中查看报名信息与支付状态。' : '提交报名意向后，工作室会联系你确认课程、时间与费用。意向提交不等于支付或席位确认。') }}</text></view><view class="notice-item"><text class="notice-label">课程安排</text><text class="body-copy">{{ UI_PREVIEW ? '当前页面的排期、地点与价格为界面演示数据。正式开课信息，请以工作室实际发布与确认为准。' : '具体开课时间、地点、费用及调整规则，以工作室实际发布与确认为准。' }}</text></view><view class="notice-item"><text class="notice-label">学习建议</text><text class="body-copy">带着好奇和真实问题来，不需要提前确定自己的性格类型。九型人格帮助自我觉察，不替代专业心理诊疗。</text></view></view>
        <view class="page-ending"><view /><text>期待，与你在课堂相遇</text><view /></view>
      </view>
      <view v-if="!timelinePreview" class="enroll-bar">
        <view class="enroll-price">
          <text v-if="isEnrolled" class="enrolled-status">已报名</text>
          <text v-else-if="enrollmentPending || enrollmentError" class="consult-price">报名状态待确认</text>
          <text v-else-if="confirmationPending" class="consult-price">支付结果确认中</text>
          <view v-else-if="course.price !== undefined"><text class="currency">¥</text><text class="price">{{ course.price.toLocaleString() }}</text><text class="price-unit"> / 人</text></view>
          <text v-else class="consult-price">咨询老师</text>
          <text class="price-caption">{{ enrollmentCaption }}</text>
        </view>
        <button class="enroll-button" :disabled="enrollmentActionDisabled" :loading="paying || enrollmentLoading" @click="enroll">{{ enrollmentActionText }} <NxIcon name="arrow" :size="18" color="#FFFFFF" /></button>
      </view>
      <NxImagePreview
        v-if="teacherAvatar && !teacherAvatarFailed"
        :visible="teacherAvatarPreviewVisible"
        :src="teacherAvatar"
        :alt="`${teacher?.name || '老师'}头像`"
        @close="closeTeacherAvatarPreview"
      />
      <NxImagePreview
        v-if="course.cover && !courseCoverFailed"
        :visible="courseCoverPreviewVisible"
        :src="course.cover"
        :alt="`${course.title || '课程'}封面`"
        @close="closeCourseCoverPreview"
      />
    </block>
  </view>
</template>

<style scoped>
.course-detail-page{min-height:100vh;background:var(--nx-page-bg);color:var(--nx-text);padding-bottom:calc(160rpx + env(safe-area-inset-bottom));box-sizing:border-box}button{box-sizing:border-box;border:0;margin:0;line-height:1.5}button::after{border:0}.course-cover{height:500rpx;position:relative;background:#DBD5C7}.cover-image{display:block;width:100%;height:100%}.cover-shade{position:absolute;inset:0;background:linear-gradient(180deg,transparent 40%,rgba(35,32,24,.42))}.cover-caption{position:absolute;bottom:38rpx;left:40rpx;display:flex;flex-direction:column;gap:14rpx;color:#fff;font-size:29rpx;font-family:'Songti SC','STSong',serif;letter-spacing:2rpx}.caption-en{font-family:Arial,sans-serif;font-size:17rpx;letter-spacing:3rpx;opacity:.85}.detail-content{padding:0 36rpx}.course-heading{padding:36rpx 0 30rpx}.title-meta{display:flex;gap:15rpx;align-items:center;font-size:21rpx;letter-spacing:1rpx;color:var(--nx-brand-700)}.meta-divider{width:1rpx;height:18rpx;background:#C6AE97}.course-title{display:block;font-family:'Songti SC','STSong',serif;font-size:49rpx;font-weight:600;line-height:1.45;margin-top:19rpx}.course-subtitle{display:block;font-size:25rpx;color:var(--nx-text-muted);line-height:1.8;margin-top:15rpx}.course-info{display:flex;flex-wrap:wrap;gap:22rpx 32rpx;margin-top:28rpx}.course-info>view{display:flex;align-items:center;gap:10rpx;font-size:23rpx;color:#66675E}.location-line{display:flex;align-items:center;flex-wrap:wrap;gap:15rpx;font-size:22rpx;color:var(--nx-text-muted);margin-top:20rpx}.demo-badge{font-size:18rpx;color:#897C68;border:1rpx solid #DAD1C2;padding:5rpx 9rpx;border-radius:5rpx}.section-tabs{display:flex;justify-content:space-between;border-bottom:1rpx solid var(--nx-border);gap:10rpx}.section-tab{position:relative;min-height:96rpx;display:flex;align-items:center;justify-content:center;flex:1;padding:20rpx 0;background:transparent;border-radius:0;font-size:25rpx;color:var(--nx-text-muted)}.section-tab.active{color:var(--nx-brand-700);font-weight:600}.section-tab.active::before{content:'';position:absolute;bottom:0;left:24%;width:52%;height:4rpx;background:var(--nx-brand-700);border-radius:4rpx}.content-section{padding:40rpx 0}.eyebrow{display:block;font-size:18rpx;letter-spacing:3rpx;color:var(--nx-brand-700)}.section-title{display:block;margin:15rpx 0 24rpx;font-family:'Songti SC','STSong',serif;font-size:36rpx;line-height:1.5}.body-copy{display:block;font-size:26rpx;color:#66685F;line-height:1.95}.highlight-list{margin-top:26rpx;display:flex;flex-direction:column;gap:20rpx}.highlight-item{display:flex;align-items:flex-start;gap:14rpx;font-size:25rpx;line-height:1.7}.highlight-icon{margin-top:3rpx;flex-shrink:0;display:flex;align-items:center;justify-content:center;width:35rpx;height:35rpx;border-radius:50%;color:var(--nx-brand-700);background:#EFE8DB}.teacher-card{display:flex;align-items:center;gap:23rpx;width:100%;background:var(--nx-surface);padding:27rpx;border-radius:24rpx;text-align:left;color:var(--nx-text)}.teacher-avatar-action{display:block;width:108rpx;height:126rpx;flex:0 0 108rpx;overflow:hidden;border-radius:12rpx}.teacher-avatar{display:block;width:108rpx;height:126rpx;flex-shrink:0;object-fit:cover;border-radius:12rpx;background:var(--nx-border)}.teacher-avatar--fallback{display:flex;align-items:center;justify-content:center;color:var(--nx-brand-700);font-size:40rpx;background:var(--nx-page-bg)}.teacher-copy{flex:1;min-width:0}.teacher-overline{display:block;color:var(--nx-text-muted);font-size:19rpx;letter-spacing:1rpx}.teacher-name{display:block;font-size:30rpx;font-family:'Songti SC','STSong',serif;margin-top:6rpx}.teacher-title{display:block;font-size:21rpx;color:var(--nx-text-muted);line-height:1.6;margin-top:7rpx}.outline-list{border-top:1rpx solid var(--nx-border)}.outline-item{display:flex;gap:24rpx;padding:28rpx 0;border-bottom:1rpx solid var(--nx-border)}.outline-number{font-family:Georgia,serif;color:#A38B73;font-size:32rpx;line-height:1.3;flex-shrink:0}.outline-title{display:block;font-size:27rpx;line-height:1.5}.outline-description{display:block;font-size:23rpx;color:var(--nx-text-muted);line-height:1.8;margin-top:10rpx}.outline-empty{background:var(--nx-surface);padding:26rpx;border-radius:20rpx}.muted-copy{display:block;font-size:24rpx;color:var(--nx-text-muted);line-height:1.8;margin-top:18rpx}.quote-note{position:relative;background:#EEEAE0;border-radius:24rpx;padding:42rpx 30rpx;text-align:center}.quote-mark{display:block;color:#BEAA90;font-family:Georgia,serif;font-size:73rpx;height:57rpx;line-height:1}.quote-copy{display:block;font-family:'Songti SC','STSong',serif;font-size:32rpx;line-height:1.8;color:#5D5E51}.quote-copy text{display:block}.quote-footer{display:block;margin-top:25rpx;font-size:20rpx;letter-spacing:2rpx;color:#878573}.notice-item{margin-top:26rpx}.notice-label{display:block;font-size:26rpx;font-weight:500;margin-bottom:10rpx}.notice-item .body-copy{font-size:24rpx}.page-ending{display:flex;align-items:center;gap:20rpx;padding:12rpx 0 30rpx;color:#999183;font-size:22rpx;letter-spacing:1rpx}.page-ending view{height:1rpx;background:var(--nx-border);flex:1}.enroll-bar{position:fixed;z-index:10;bottom:0;left:0;right:0;display:flex;align-items:center;justify-content:space-between;gap:20rpx;padding:22rpx 32rpx calc(22rpx + env(safe-area-inset-bottom));background:rgba(255,255,255,.98);border-top:1rpx solid var(--nx-border);box-sizing:border-box}.enroll-price{min-width:0}.currency{font-size:25rpx;color:var(--nx-brand-700);margin-right:5rpx}.price{font-family:Georgia,serif;font-size:40rpx;color:var(--nx-brand-700)}.price-unit{font-size:20rpx;color:var(--nx-text-muted)}.price-caption{display:block;font-size:18rpx;color:var(--nx-text-muted);margin-top:7rpx}.consult-price{font-size:25rpx}.enroll-button{display:flex;align-items:center;justify-content:center;gap:15rpx;min-height:92rpx;padding:20rpx 30rpx;border-radius:14rpx;background:var(--nx-brand-700);font-size:25rpx;color:#fff;flex-shrink:0}.empty-state{display:flex;flex-direction:column;align-items:center;padding:120rpx 40rpx;text-align:center;gap:20rpx}.empty-title{font-family:'Songti SC','STSong',serif;font-size:38rpx}.primary-button{min-height:96rpx;display:flex;align-items:center;justify-content:center;width:100%;border-radius:14rpx;margin-top:30rpx;background:var(--nx-brand-700);color:#fff;font-size:27rpx}
@media(min-width:600px){.course-detail-page{max-width:800rpx;margin:auto}.enroll-bar{max-width:800rpx;margin:auto}}
.enrolled-status{font-family:'Songti SC','STSong',serif;font-size:34rpx;color:var(--nx-brand-700)}.enroll-button[disabled]{background:#B87A5D;color:#FFFFFF}
@media(prefers-reduced-motion:reduce){.course-detail-page{scroll-behavior:auto!important;transition:none!important}}
</style>
