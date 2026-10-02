<script setup>
import { computed, ref, watch, nextTick } from 'vue'
import { onShow, onHide, onUnload } from '@dcloudio/uni-app'
import NxIcon from '../../components/NxIcon.vue'
import NxImagePreview from '../../components/NxImagePreview.vue'
import { ensureLogin, getToken } from '../../utils/auth'
import { createBookingApi, createCourseBookingOrderApi, devPayCourseBookingOrderApi, getCourseBookingOrderStatusApi } from '../../api'
import { userErrorMessage } from '../../utils/userMessage'
import { clearBookingDraft, loadBookingDraft, saveBookingDraft } from '../../utils/bookingDraft'
import { consumeBookingIntent } from '../../utils/bookingIntent'
import { refreshSiteConfig, getStoredSiteConfig } from '../../utils/siteConfig'
import { normalizePersonalExpertHome } from '../../utils/personalExpertHome'
import { normalizeMiniappCourses, normalizeTeachers } from '../../utils/teacherCourseware'
import { payWechatOrder } from '../../utils/payment'
import { createCoursePaymentController, coursePaymentResultUrl } from '../../utils/coursePayment'
import { normalizeMiniappLearn } from '../../utils/miniappPages'
import { STUDIO_COURSES, STUDIO_TEACHER } from '../../data/teacherStudio'
import { isCourseRegistrationEnabled } from '../../utils/courseRegistration'
import { UI_PREVIEW } from '../../utils/uiPreview'
import { previewImage } from '../../utils/imagePreview'
import { isWechatDevtools } from '../../utils/imagePreview'
import NxStudioHeader from '../../components/NxStudioHeader.vue'
import { useStudioNavigation } from '../../utils/studioNavigation'

// Keep the page state executable in the lightweight flow harness, where the
// component imports are intentionally stripped before evaluating this script.
const studioNavigation = typeof useStudioNavigation === 'function'
  ? useStudioNavigation()
  : { pageStyle: computed(() => ({})), refreshNavigation: () => {} }
const { pageStyle, refreshNavigation } = studioNavigation

// H5 studio preview is a visual fixture only. Real submissions require the
// WeChat runtime so wx.login can establish the backend miniapp session.
const H5_RUNTIME = UI_PREVIEW || typeof window !== 'undefined'

const kinds = [
  { value: 'course', label: '课程报名' },
  { value: 'consult', label: '1v1 咨询' },
  { value: 'enterprise', label: '企业共学' },
]
const kindIndex = ref(0)
const currentKind = computed(() => kinds[kindIndex.value]?.value || 'course')
const emptyForm = () => ({ contactName: '', phone: '', intent: '', preferredTime: '', message: '' })
const form = ref(emptyForm())
const selectedCourseId = ref('')
const fieldErrors = ref({ contactName: '', phone: '', consent: '' })
const consent = ref(false)
const submitting = ref(false)
const paymentMessage = ref('')
const submitted = ref(false)
const submittedPaid = ref(false)
let pendingBooking = null
let paymentController = null
let pageActive = true
const submittedKind = ref('course')
const siteConfig = ref(getStoredSiteConfig() || {})
const enterpriseView = computed(() => normalizePersonalExpertHome(siteConfig.value).enterprise)
const classroomEnabled = computed(() => normalizeMiniappLearn(siteConfig.value).classroom.enabled)
const courseRegistrationEnabled = computed(() => isCourseRegistrationEnabled(siteConfig.value))
const courses = computed(() => courseRegistrationEnabled.value ? (UI_PREVIEW ? STUDIO_COURSES : normalizeMiniappCourses(siteConfig.value)) : [])
const selectedCourse = computed(() => courses.value.find((course) => String(course.id) === selectedCourseId.value))
const teacher = computed(() => UI_PREVIEW ? STUDIO_TEACHER : normalizeTeachers(siteConfig.value)[0])
const teacherAvatar = computed(() => {
  const avatar = teacher.value?.avatar || ''
  const isLaohan = ['韩常青', '韩常青（老韩）', '韩常青(老韩)', '老韩', '韩老师'].includes(teacher.value?.name)
  if (isLaohan && (!avatar || /\/avatars\/|teacher-poster/i.test(avatar))) return '/static/teacher/portrait.jpg'
  return /\/avatars\//i.test(avatar) ? '' : avatar
})
const teacherAvatarFailed = ref(false)
const teacherAvatarPreviewVisible = ref(false)
const serviceModes = computed(() => enterpriseView.value.serviceModes || [])
const processSteps = computed(() => enterpriseView.value.processSteps || [])
const formTitle = computed(() => ({ course: '为下一次成长，留一个位置', consult: '从你正在经历的事，聊起', enterprise: '一起找到团队的共学方向' }[currentKind.value]))
const formHint = computed(() => ({ course: '留下联系方式，我们将与你确认课程安排。', consult: '简单说说你的困惑，老师会与你沟通咨询安排。', enterprise: '告诉我们团队背景，共同讨论适合的形式。' }[currentKind.value]))
const intentPlaceholder = computed(() => ({ course: courseRegistrationEnabled.value ? '选择上方课程，或填写感兴趣的主题' : '填写感兴趣的课程或学习方向', consult: '如：自我探索 / 亲密关系 / 职场沟通', enterprise: '如：团队工作坊 / 管理者培训' }[currentKind.value]))
const messagePlaceholder = computed(() => currentKind.value === 'enterprise' ? '团队规模、背景，或希望改善的协作议题（选填）' : '想提前告诉老师的事，或你对课程的期待（选填）')
const successTitle = computed(() => submittedPaid.value ? '课程已支付，期待与你相遇' : ({ course: '你的学习意向，已收到', consult: '你的咨询预约，已收到', enterprise: '你的企业需求，已收到' }[submittedKind.value]))
const DRAFT_SAVE_DELAY = 250
let draftSaveTimer = null
let configLoadId = 0
const draft = loadBookingDraft()
const restoredDraftNotice = ref(!!draft)
if (draft) {
  const index = kinds.findIndex((item) => item.value === draft.kind)
  if (index >= 0) kindIndex.value = index
  form.value = { ...emptyForm(), ...draft }
  selectedCourseId.value = draft.courseId || ''
  delete form.value.kind
  delete form.value.courseId
}

function currentDraft() {
  return {
    kind: currentKind.value,
    ...(courseRegistrationEnabled.value && currentKind.value === 'course' && selectedCourseId.value ? { courseId: selectedCourseId.value } : {}),
    ...form.value,
  }
}
function cancelPendingDraftSave() {
  if (draftSaveTimer !== null) clearTimeout(draftSaveTimer)
  draftSaveTimer = null
}
function persistDraft() {
  if (submitted.value) return
  // Visiting the course tab alone should not create a blank draft.
  if (Object.values(form.value).some((value) => String(value).trim())) saveBookingDraft(currentDraft())
  else clearBookingDraft()
}
function scheduleDraftSave() {
  cancelPendingDraftSave()
  if (submitted.value) return
  draftSaveTimer = setTimeout(() => { draftSaveTimer = null; persistDraft() }, DRAFT_SAVE_DELAY)
}
function flushDraftSave() { cancelPendingDraftSave(); persistDraft() }
watch([kindIndex, form], scheduleDraftSave, { deep: true })
onShow(() => {
  // #ifdef MP-WEIXIN
  if (typeof uni !== 'undefined' && typeof uni.hideShareMenu === 'function') {
    uni.hideShareMenu({ menus: ['shareAppMessage', 'shareTimeline'] })
  }
  // #endif
  paymentController?.resume()
  refreshNavigation()
  return applyBookingIntent()
})
onHide(flushDraftSave)
onUnload(() => { pageActive = false; paymentController?.stop(); flushDraftSave() })

async function applyBookingIntent() {
  const intent = consumeBookingIntent()
  if (intent) {
    submitted.value = false
    submittedPaid.value = false
    const index = kinds.findIndex((item) => item.value === intent.kind)
    if (index >= 0) kindIndex.value = index
    selectedCourseId.value = intent.kind === 'course' ? (intent.courseId || '') : ''
    // A new explicit course selection replaces the old direction, while contact details remain.
    if (intent.intentText) form.value = { ...form.value, intent: intent.intentText }
  }
  const loadId = ++configLoadId
  try {
    const config = await refreshSiteConfig()
    if (pageActive && loadId === configLoadId) applySiteConfig(config)
  } catch {
    // Cached content and the saved draft remain usable during a network interruption.
  }
  if (intent) {
    await nextTick()
    scrollToForm()
  }
}
function applySiteConfig(config) {
  siteConfig.value = config || {}
  if (!courseRegistrationEnabled.value) {
    selectedCourseId.value = ''
    pendingBooking = null
  }
}
function selectKind(index) {
  if (submitting.value) return
  kindIndex.value = index
  if (kinds[index]?.value !== 'course') selectedCourseId.value = ''
}
function viewCourse(course) { if (!courseRegistrationEnabled.value) { scrollToForm(); return }; uni.navigateTo({ url: `/pages/course-detail/course-detail?id=${encodeURIComponent(course.id)}` }) }
function previewTeacherAvatar() {
  if (teacherAvatarFailed.value) return
  if (isWechatDevtools()) {
    teacherAvatarPreviewVisible.value = true
    return
  }
  previewImage(teacherAvatar.value)
}
function closeTeacherAvatarPreview() { teacherAvatarPreviewVisible.value = false }
function selectServiceMode(mode) {
  form.value = { ...form.value, intent: mode.title }
  scrollToForm()
}
function scrollToForm() { uni.pageScrollTo({ selector: '#booking-form', duration: 260 }) }
function clearFieldError(field) {
  if (fieldErrors.value[field]) fieldErrors.value = { ...fieldErrors.value, [field]: '' }
}
function resetForm() {
  cancelPendingDraftSave()
  kindIndex.value = 0
  form.value = emptyForm()
  selectedCourseId.value = ''
  paymentMessage.value = ''
  fieldErrors.value = { contactName: '', phone: '', consent: '' }
  restoredDraftNotice.value = false
  consent.value = false
}
function clearRestoredDraft() {
  if (submitting.value) return
  clearBookingDraft()
  resetForm()
}
function changeConsent(event) {
  consent.value = event.detail.value.includes('agree')
  clearFieldError('consent')
}
function showPrivacy() {
  uni.showModal({
    title: '预约信息使用说明',
    content: H5_RUNTIME
      ? '当前为界面预览。正式提交请打开微信小程序，报名信息会保存至后台管理并用于本次预约沟通。请勿在留言中填写身份证、银行卡等敏感信息。'
      : '你填写的称呼、手机号、意向和留言将提交给工作室，仅用于本次预约沟通与课程安排。提交前的草稿保存在当前设备，可随时清空。请勿在留言中填写身份证、银行卡等敏感信息。',
    showCancel: false,
    confirmText: '我知道了',
    confirmColor: '#A55C3B',
  })
}
function validateForm() {
  const errors = { contactName: '', phone: '', consent: '' }
  if (!form.value.contactName.trim()) errors.contactName = '请填写你的称呼'
  if (!/^1\d{10}$/.test(form.value.phone.trim())) errors.phone = '请填写 11 位有效手机号'
  if (!consent.value) errors.consent = '请阅读并同意预约信息使用说明'
  fieldErrors.value = errors
  return !Object.values(errors).some(Boolean)
}
async function submit() {
  if (submitting.value) return
  if (H5_RUNTIME) {
    uni.showToast({ title: '请在微信小程序内提交报名意向', icon: 'none' })
    return
  }
  if (!validateForm()) {
    uni.showToast({ title: Object.values(fieldErrors.value).find(Boolean), icon: 'none' })
    return
  }
  submitting.value = true
  paymentMessage.value = ''
  let requestToken = ''
  let resultUrl = ''
  try {
    await ensureLogin()
    if (!pageActive) return
    requestToken = getToken()
    const currentSession = () => pageActive && getToken() === requestToken
    const requireSession = () => { if (!currentSession()) throw new Error('登录状态已更新，请重试') }
    if (selectedCourseId.value) {
      const latest = await refreshSiteConfig()
      requireSession()
      applySiteConfig(latest)
    }
    const payload = currentDraft()
    const fingerprint = JSON.stringify(payload)
    const booking = pendingBooking?.token === requestToken && pendingBooking.fingerprint === fingerprint
      ? pendingBooking.record : await createBookingApi(payload)
    if (!currentSession()) return
    if (!UI_PREVIEW && payload.kind === 'course' && booking?.paymentMode === 'paid') {
      pendingBooking = { token: requestToken, fingerprint, record: booking }
      resultUrl = coursePaymentResultUrl({ bookingId: booking.id })
      const status = await getCourseBookingOrderStatusApi(booking.id)
      requireSession()
      if (status?.syncStatus === 'retrying' || ['closed', 'refunded', 'cancelled', 'failed'].includes(status?.status)) {
        if (resultUrl) uni.navigateTo({ url: resultUrl })
        return
      }
      if (status?.status !== 'paid') {
        paymentController = createCoursePaymentController({
          isCurrent: currentSession,
          create: () => { requireSession(); return createCourseBookingOrderApi(booking.id) },
          pay: (order) => { requireSession(); return payWechatOrder(order, { devPay: (pending) => devPayCourseBookingOrderApi(pending.outTradeNo) }) },
          status: (order) => { requireSession(); return getCourseBookingOrderStatusApi(order.bookingId) },
          onChange: (snapshot) => { if (currentSession()) paymentMessage.value = snapshot.message || '' },
        })
        const result = await paymentController.purchase()
        requireSession()
        resultUrl = coursePaymentResultUrl(result?.order) || resultUrl
        if (result?.state !== 'success') {
          if (resultUrl) uni.navigateTo({ url: resultUrl })
          return
        }
      }
      submittedPaid.value = true
    } else {
      submittedPaid.value = booking?.paymentStatus === 'paid'
    }
    pendingBooking = null
    submittedKind.value = payload.kind
    cancelPendingDraftSave()
    clearBookingDraft()
    submitted.value = true
    resetForm()
    if (resultUrl) uni.navigateTo({ url: resultUrl })
    else uni.pageScrollTo({ scrollTop: 0, duration: 250 })
  } catch (error) {
    if (pageActive && (!requestToken || getToken() === requestToken)) {
      if (resultUrl) uni.navigateTo({ url: resultUrl })
      else uni.showToast({ title: userErrorMessage(error, '提交失败，填写内容已保留，请重试'), icon: 'none' })
    }
  } finally { submitting.value = false; paymentController = null }
}
function viewBookingRecords() { uni.navigateTo({ url: '/pages/booking-records/booking-records' }) }
function viewSubmittedRecord() { uni.navigateTo({ url: submittedPaid.value ? '/pages/orders/orders' : '/pages/booking-records/booking-records' }) }
function continueClassroom() { uni.switchTab({ url: '/pages/learn/learn' }) }
function submitAnother() { resetForm(); submitted.value = false; submittedPaid.value = false }
</script>

<template>
  <view class="booking-page" :style="pageStyle">
    <NxStudioHeader />
    <view class="page-masthead"><text class="masthead-name">共学与成长</text><button class="records-link" @click="viewBookingRecords">我的报名 <NxIcon name="arrow" :size="16" /></button></view>
    <view v-if="submitted" class="booking-success">
      <view class="success-symbol"><NxIcon name="check" :size="32" /></view>
      <text class="eyebrow">{{ submittedPaid ? '已支付 · 报名成功' : '已提交 · 等待确认' }}</text>
      <text class="success-title">{{ successTitle }}</text>
      <text class="success-copy">工作室将通过你留下的联系方式与你沟通。具体时间、费用与安排，以双方确认为准。</text>
      <button class="primary-button" @click="viewSubmittedRecord">{{ submittedPaid ? '查看我的订单' : '查看预约记录' }} <NxIcon name="arrow" :size="18" color="#FFFFFF" /></button>
      <button v-if="classroomEnabled" class="secondary-button" @click="continueClassroom">继续浏览老师课堂</button>
      <button class="text-button" @click="submitAnother">再提交一个需求</button>
    </view>
    <block v-else>
      <view v-if="courseRegistrationEnabled" class="booking-hero">
        <text class="eyebrow">LEARN TOGETHER</text>
        <view class="hero-title"><text>把理解，</text><text>带进生活里</text></view>
        <text class="hero-description">从一次相遇开始，走向更懂自己的日常。</text>
        <view class="hero-rule"><view /><text>觉察 · 练习 · 成长</text></view>
      </view>
      <view class="kind-tabs" role="tablist" aria-label="报名类型">
        <button v-for="(kind, index) in kinds" :key="kind.value" :class="['kind-tab', { 'is-active': kindIndex === index }]" role="tab" :aria-selected="kindIndex === index" @click="selectKind(index)">{{ kind.label }}</button>
      </view>

      <view v-if="courseRegistrationEnabled && currentKind === 'course'" class="course-section">
        <view class="section-heading"><text class="section-title">一起，在课堂里相遇</text><text class="section-meta">{{ courses.length }} 个学习方向</text></view>
        <button v-for="(course, index) in courses" :key="course.id" class="course-card" hover-class="pressed" @click="viewCourse(course)">
          <view class="course-image-wrap"><image class="course-image" :src="course.cover" mode="aspectFill" :aria-label="course.title" /><text class="course-tag">{{ course.tag || '主题课程' }}</text><text class="course-index">0{{ index + 1 }}</text></view>
          <view class="course-body">
            <view class="course-subtitle"><text>{{ course.format || '主题共学' }}</text><text class="dot">·</text><text>{{ course.duration }}</text></view>
            <text class="course-title">{{ course.title }}</text>
            <text class="course-description">{{ course.subtitle || course.description }}</text>
            <view class="course-schedule"><NxIcon name="calendar" :size="15" /><text>{{ course.schedule || '具体排期请咨询工作室' }}</text><text v-if="UI_PREVIEW" class="demo-label">演示排期</text></view>
            <view class="course-bottom"><view><text v-if="course.price !== undefined" class="course-price"><text class="currency">¥</text>{{ course.price.toLocaleString() }}</text><text v-else class="price-consult">咨询老师</text><text v-if="course.price !== undefined" class="price-unit"> / 人</text></view><view class="course-link"><text>了解课程</text><NxIcon name="arrow" :size="18" /></view></view>
          </view>
        </button>
        <view v-if="!courses.length" class="empty-card"><NxIcon name="book" :size="30" /><text class="section-title">新的共学，正在准备</text><text class="muted-copy">留下感兴趣的学习方向，我们会与你联系。</text></view>
      </view>

      <view v-else-if="courseRegistrationEnabled && currentKind === 'consult'" class="consult-section">
        <view class="consult-card">
          <view class="consult-top"><button v-if="teacherAvatar && !teacherAvatarFailed" class="teacher-avatar-action" aria-label="预览老师头像" @click.stop="previewTeacherAvatar"><image class="teacher-avatar" :src="teacherAvatar" mode="widthFix" :aria-label="`${teacher?.name || '老师'}头像，点击查看原图`" @error="teacherAvatarFailed = true" /></button><view v-else class="teacher-avatar teacher-avatar--fallback">{{ (teacher?.name || '老师').slice(0, 1) }}</view><view class="consult-teacher"><text class="teacher-name">{{ teacher?.name || '老师' }} · 一对一</text><text class="muted-copy">一段被认真倾听的时间</text></view><NxIcon name="message" :size="25" /></view>
          <text class="consult-title">当你想更靠近真实的自己</text>
          <text class="consult-copy">从一段关系、一次情绪，或一个反复出现的困惑开始。和老师一起，看见行为背后的需要，找到属于你的下一步。</text>
          <view class="topic-tags"><text>自我探索</text><text>关系沟通</text><text>成长困惑</text></view>
          <view class="consult-note"><NxIcon name="clock" :size="18" /><text>时间与形式将在沟通后共同确认</text></view>
        </view>
        <text class="section-footnote">九型人格用于自我觉察与成长，不替代专业心理诊疗。</text>
      </view>

      <view v-else-if="courseRegistrationEnabled" class="enterprise-section">
        <view class="enterprise-intro"><text class="eyebrow">GROW AS A TEAM</text><text class="section-title">{{ enterpriseView.title }}</text><text class="muted-copy">{{ enterpriseView.lead }}</text></view>
        <button v-for="(mode, index) in serviceModes" :key="mode.title" :class="['service-card', { selected: form.intent === mode.title }]" @click="selectServiceMode(mode)"><text class="service-number">0{{ index + 1 }}</text><view class="service-copy"><text class="service-title">{{ mode.title }}</text><text class="muted-copy">{{ mode.description }}</text></view><NxIcon :name="form.intent === mode.title ? 'check' : 'arrow'" :size="20" /></button>
        <view class="process-list"><view v-for="(step, index) in processSteps" :key="step.title" class="process-step"><text class="process-number">{{ index + 1 }}</text><text>{{ step.title }}</text></view></view>
      </view>

      <view v-if="courseRegistrationEnabled" class="form-divider"><view /><NxIcon name="spark" :size="21" /><view /></view>
      <view id="booking-form" class="booking-form">
        <text class="eyebrow">LET'S BEGIN</text><text class="form-title">{{ formTitle }}</text><text class="form-hint">{{ formHint }}</text>
        <view v-if="restoredDraftNotice" class="draft-restored"><text>已恢复上次填写的内容</text><button :disabled="submitting" @click="clearRestoredDraft">清空草稿</button></view>
        <view class="field"><text class="field-label">你的称呼 <text class="required">*</text></text><input v-model="form.contactName" class="field-control" maxlength="40" aria-label="你的称呼" :aria-invalid="!!fieldErrors.contactName" placeholder="方便我们怎么称呼你" @input="clearFieldError('contactName')" /><text v-if="fieldErrors.contactName" class="field-error" role="alert">{{ fieldErrors.contactName }}</text></view>
        <view class="field"><text class="field-label">手机号码 <text class="required">*</text></text><input v-model="form.phone" class="field-control" type="number" maxlength="11" aria-label="手机号码" :aria-invalid="!!fieldErrors.phone" placeholder="用于确认课程与预约安排" @input="clearFieldError('phone')" /><text v-if="fieldErrors.phone" class="field-error" role="alert">{{ fieldErrors.phone }}</text></view>
        <view class="field"><text class="field-label">{{ currentKind === 'course' ? '意向课程' : '想聊的方向' }}</text><input v-model="form.intent" class="field-control" maxlength="120" aria-label="意向方向" :placeholder="intentPlaceholder" /></view>
        <view class="field"><text class="field-label">方便联系的时间 <text class="optional">选填</text></text><input v-model="form.preferredTime" class="field-control" maxlength="100" aria-label="方便联系的时间" placeholder="例如：工作日 18:00 后" /></view>
        <view class="field"><text class="field-label">想对老师说 <text class="optional">选填</text></text><textarea v-model="form.message" class="field-control message-input" maxlength="1000" aria-label="想对老师说" :placeholder="messagePlaceholder" /><text class="message-count">{{ form.message.length }} / 1000</text></view>
        <view class="consent-row"><checkbox-group @change="changeConsent"><label class="consent-label"><checkbox value="agree" :checked="consent" color="#A55C3B" /><text>我已阅读并同意</text></label></checkbox-group><button class="privacy-link" @click="showPrivacy">预约信息使用说明</button></view>
        <text v-if="fieldErrors.consent" class="field-error" role="alert">{{ fieldErrors.consent }}</text>
        <!-- #ifdef H5 -->
        <button class="primary-button" disabled>请在微信小程序内提交报名意向</button>
        <!-- #endif -->
        <!-- #ifndef H5 -->
        <button class="primary-button" :loading="submitting" :disabled="submitting" @click="submit">{{ submitting ? (paymentMessage || '正在提交') : currentKind === 'course' ? selectedCourse?.paymentMode === 'paid' ? '提交报名并支付' : '提交报名意向' : '提交预约意向' }} <NxIcon v-if="!submitting" name="arrow" :size="19" color="#FFFFFF" /></button>
        <!-- #endif -->
        <text class="form-footer">{{ H5_RUNTIME ? '当前为界面预览 · 请在微信小程序内正式提交，信息将同步到后台管理' : selectedCourse?.paymentMode === 'paid' ? '提交后将进入微信支付 · 支付结果以服务端确认状态为准' : '提交意向无需付款 · 具体安排以工作室确认为准' }}</text>
        <text class="draft-hint">未提交的内容会自动保存为本机草稿</text>
      </view>
      <view class="page-ending"><text>每一步靠近，都是成长。</text><text class="ending-en">GROW, AT YOUR OWN PACE.</text></view>
    </block>
    <NxImagePreview
      v-if="teacherAvatar && !teacherAvatarFailed"
      :visible="teacherAvatarPreviewVisible"
      :src="teacherAvatar"
      :alt="`${teacher?.name || '老师'}头像`"
      @close="closeTeacherAvatarPreview"
    />
  </view>
</template>

<style scoped>
/* Leave room around the portrait so the circular crop shows the head and upper body. */
.consult-top .teacher-avatar-action{background:#111419}
.teacher-avatar-action .teacher-avatar{width:72rpx;height:auto;margin:4rpx auto 0;border-radius:0}
.booking-page{box-sizing:border-box;min-height:100vh;padding:calc(28rpx + var(--status-bar-height, 0px)) 36rpx calc(56rpx + env(safe-area-inset-bottom));padding-top:calc(144rpx + env(safe-area-inset-top, 0px));background:var(--nx-page-bg);color:var(--nx-text)}
button{box-sizing:border-box;margin:0;border:0;line-height:1.5}button::after{border:0}button[disabled]{opacity:.48}.page-masthead{display:flex;align-items:center;justify-content:space-between;gap:16rpx}.masthead-name{font-size:24rpx;letter-spacing:3rpx}.records-link{min-height:88rpx;padding:0;display:flex;align-items:center;gap:10rpx;background:transparent;font-size:23rpx;color:var(--nx-text-muted)}.booking-hero{padding:35rpx 2rpx 40rpx}.eyebrow{display:block;color:var(--nx-brand-700);font-size:20rpx;letter-spacing:4rpx;line-height:1.6}.hero-title{display:block;margin-top:20rpx;font-family:'Songti SC','STSong',serif;font-size:64rpx;font-weight:500;line-height:1.4;letter-spacing:1rpx}.hero-title text{display:block}.hero-description{display:block;margin-top:23rpx;font-size:24rpx;line-height:1.8;color:var(--nx-text-muted)}.hero-rule{display:flex;align-items:center;gap:20rpx;margin-top:34rpx;color:var(--nx-text-muted);font-size:20rpx;letter-spacing:3rpx}.hero-rule view{width:58rpx;height:2rpx;background:var(--nx-brand-700)}.kind-tabs{display:flex;border-bottom:1rpx solid var(--nx-border);margin-bottom:36rpx;gap:28rpx}.kind-tab{position:relative;min-height:92rpx;flex:1;background:transparent;padding:20rpx 0;color:var(--nx-text-muted);font-size:27rpx;white-space:nowrap;border-radius:0}.kind-tab.is-active{color:var(--nx-brand-700);font-weight:600}.kind-tab.is-active::before{content:'';position:absolute;bottom:0;left:26%;width:48%;height:4rpx;border-radius:4rpx;background:var(--nx-brand-700)}.section-heading{display:flex;justify-content:space-between;align-items:center;gap:16rpx;margin:0 0 24rpx}.section-title{display:block;font-family:'Songti SC','STSong',serif;font-size:33rpx;line-height:1.5}.section-meta{color:var(--nx-text-muted);font-size:21rpx;flex-shrink:0}.course-card{display:block;padding:0;width:100%;margin-bottom:28rpx;border-radius:24rpx;overflow:hidden;background:var(--nx-surface);text-align:left;box-shadow:0 5rpx 20rpx rgba(56,45,32,.025)}.course-image-wrap{height:296rpx;position:relative;background:#E7E0D3}.course-image{width:100%;height:100%;display:block}.course-tag{position:absolute;top:24rpx;left:24rpx;font-size:20rpx;padding:9rpx 17rpx;border-radius:8rpx;background:rgba(255,255,255,.92);color:var(--nx-text)}.course-index{position:absolute;right:22rpx;bottom:7rpx;font-family:Georgia,serif;font-size:76rpx;color:rgba(255,255,255,.7)}.course-body{padding:29rpx 28rpx 24rpx}.course-subtitle{display:flex;align-items:center;gap:12rpx;font-size:20rpx;color:var(--nx-brand-700);letter-spacing:1rpx}.dot{color:#B5A99A}.course-title{display:block;margin-top:12rpx;font-size:36rpx;font-family:'Songti SC','STSong',serif;line-height:1.4;font-weight:600}.course-description{display:block;margin-top:12rpx;font-size:23rpx;line-height:1.65;color:var(--nx-text-muted)}.course-schedule{display:flex;align-items:center;flex-wrap:wrap;gap:9rpx;margin-top:27rpx;font-size:21rpx;color:var(--nx-text-muted)}.demo-label{font-size:17rpx;border:1rpx solid var(--nx-border);border-radius:4rpx;padding:2rpx 6rpx}.course-bottom{border-top:1rpx solid #EEEAE3;margin-top:24rpx;padding-top:21rpx;display:flex;align-items:center;justify-content:space-between}.course-price{font-family:Georgia,serif;font-size:39rpx;color:var(--nx-brand-700)}.currency{font-size:23rpx;margin-right:5rpx}.price-unit{color:var(--nx-text-muted);font-size:20rpx}.price-consult{color:var(--nx-brand-700);font-size:24rpx}.course-link{display:flex;gap:14rpx;align-items:center;font-size:22rpx}.pressed{opacity:.8}.empty-card{padding:50rpx 30rpx;display:flex;flex-direction:column;align-items:center;gap:20rpx;background:var(--nx-surface);border-radius:24rpx;text-align:center}.muted-copy{display:block;color:var(--nx-text-muted);font-size:23rpx;line-height:1.7}.consult-card{padding:32rpx;background:var(--nx-surface);border-radius:24rpx}.consult-top{display:flex;align-items:center;gap:20rpx}.teacher-avatar-action{display:block;width:90rpx;height:90rpx;flex:0 0 90rpx;padding:0;margin:0;border:0;border-radius:50%;background:transparent;overflow:hidden}.teacher-avatar-action::after{border:0}.teacher-avatar{display:block;width:90rpx;height:90rpx;border-radius:50%;background:var(--nx-border)}.teacher-avatar--fallback{display:flex;align-items:center;justify-content:center;color:var(--nx-brand-700);font-size:32rpx;background:var(--nx-page-bg)}.consult-teacher{flex:1;min-width:0}.teacher-name{display:block;margin-bottom:6rpx;font-size:26rpx}.consult-title{display:block;margin-top:38rpx;font-family:'Songti SC','STSong',serif;font-size:34rpx;line-height:1.5}.consult-copy{display:block;color:var(--nx-text-muted);font-size:25rpx;line-height:1.9;margin-top:18rpx}.topic-tags{display:flex;gap:12rpx;flex-wrap:wrap;margin-top:27rpx}.topic-tags text{background:var(--nx-page-bg);padding:10rpx 18rpx;border-radius:8rpx;color:#6A695F;font-size:21rpx}.consult-note{display:flex;align-items:center;gap:12rpx;margin-top:34rpx;padding-top:24rpx;border-top:1rpx solid var(--nx-border);color:var(--nx-text-muted);font-size:22rpx}.section-footnote{display:block;font-size:20rpx;line-height:1.7;color:var(--nx-text-muted);margin:24rpx 8rpx}.enterprise-intro{padding:16rpx 0 28rpx}.enterprise-intro .section-title{margin:16rpx 0}.service-card{padding:28rpx 24rpx;background:var(--nx-surface);border:1rpx solid transparent;border-radius:20rpx;display:flex;align-items:center;gap:20rpx;margin-bottom:16rpx;text-align:left}.service-card.selected{border-color:var(--nx-brand-700)}.service-number{font-family:Georgia,serif;font-size:31rpx;color:#AC9882;align-self:flex-start;padding-top:3rpx}.service-copy{flex:1}.service-title{display:block;font-size:27rpx;margin-bottom:9rpx}.process-list{display:flex;gap:12rpx;justify-content:space-between;padding:25rpx 0 0}.process-step{display:flex;gap:10rpx;align-items:center;font-size:20rpx;color:var(--nx-text-muted)}.process-number{width:30rpx;height:30rpx;border-radius:50%;border:1rpx solid #CBC1B3;display:flex;align-items:center;justify-content:center;font-size:18rpx}.form-divider{display:flex;align-items:center;gap:23rpx;margin:50rpx 55rpx;color:#A79884}.form-divider view{height:1rpx;background:var(--nx-border);flex:1}.booking-form{padding:32rpx 28rpx;background:var(--nx-surface);border-radius:24rpx}.form-title{display:block;margin:16rpx 0;font-family:'Songti SC','STSong',serif;font-size:36rpx;line-height:1.5}.form-hint{display:block;color:var(--nx-text-muted);font-size:23rpx;line-height:1.7;margin-bottom:34rpx}.draft-restored{display:flex;align-items:center;justify-content:space-between;gap:12rpx;background:#F3F0E9;border-radius:10rpx;padding:4rpx 18rpx;margin:0 0 24rpx;font-size:21rpx;color:var(--nx-text-muted)}.draft-restored button{min-height:88rpx;padding:10rpx 0;display:flex;align-items:center;background:transparent;color:var(--nx-brand-700);font-size:21rpx}.field{position:relative;margin-top:27rpx}.field-label{display:block;font-size:25rpx;margin-bottom:14rpx}.required{color:var(--nx-brand-700);font-size:20rpx}.optional{margin-left:10rpx;font-size:20rpx;color:#8B8B82}.field-control{box-sizing:border-box;width:100%;height:88rpx;line-height:1.5;padding:0 20rpx;background:#F8F7F3;border:1rpx solid #EEEAE3;border-radius:10rpx;font-size:24rpx;color:var(--nx-text)}.message-input{height:188rpx;min-height:188rpx;padding:20rpx 20rpx 37rpx}.message-count{position:absolute;right:16rpx;bottom:11rpx;font-size:18rpx;color:#929087}.field-error{display:block;color:#A34432;margin-top:10rpx;font-size:22rpx;line-height:1.5}.consent-row{display:flex;align-items:center;flex-wrap:wrap;gap:0;margin-top:23rpx;font-size:21rpx;color:var(--nx-text-muted)}.consent-label{display:flex;align-items:center;min-height:88rpx}.consent-label checkbox{transform:scale(.76);transform-origin:left center;margin-right:-2rpx}.privacy-link{display:flex;align-items:center;min-height:88rpx;background:transparent;padding:0 0 0 4rpx;font-size:21rpx;color:var(--nx-brand-700);text-decoration:underline}.primary-button{display:flex;align-items:center;justify-content:center;gap:22rpx;min-height:96rpx;width:100%;padding:20rpx;border-radius:14rpx;background:var(--nx-brand-700);color:#fff;font-size:27rpx;letter-spacing:1rpx;margin-top:20rpx}.form-footer{display:block;text-align:center;margin-top:21rpx;font-size:20rpx;color:var(--nx-text-muted);line-height:1.6}.draft-hint{display:block;text-align:center;margin-top:9rpx;font-size:18rpx;color:#919087}.page-ending{display:flex;flex-direction:column;align-items:center;gap:15rpx;margin-top:48rpx;color:#979186;font-family:'Songti SC','STSong',serif;font-size:25rpx;letter-spacing:2rpx}.ending-en{font-family:Arial,sans-serif;font-size:16rpx;letter-spacing:3rpx}.booking-success{display:flex;flex-direction:column;align-items:center;padding:60rpx 24rpx;text-align:center}.success-symbol{display:flex;align-items:center;justify-content:center;width:116rpx;height:116rpx;border-radius:50%;background:#EAEDE3;color:#6E785C;margin-bottom:40rpx}.success-title{font-family:'Songti SC','STSong',serif;font-size:46rpx;line-height:1.5;margin-top:18rpx}.success-copy{font-size:26rpx;line-height:1.9;color:var(--nx-text-muted);margin:24rpx 0 40rpx}.secondary-button{min-height:92rpx;width:100%;display:flex;align-items:center;justify-content:center;border:1rpx solid var(--nx-border);background:transparent;color:var(--nx-text);font-size:25rpx;border-radius:14rpx;margin-top:18rpx}.text-button{min-height:88rpx;background:transparent;color:var(--nx-text-muted);font-size:24rpx;margin-top:18rpx;display:flex;align-items:center}.booking-page :deep(.uni-input-placeholder),.booking-page :deep(.uni-textarea-placeholder){color:#96958D}
@media(min-width:600px){.booking-page{max-width:800rpx;margin:auto}.hero-title{font-size:58rpx}}
@media(prefers-reduced-motion:reduce){.booking-page{scroll-behavior:auto!important;transition:none!important}}
</style>
