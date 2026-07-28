<script setup>
import { computed, ref, watch } from 'vue'
import { onShow, onHide, onUnload } from '@dcloudio/uni-app'
import { ensureLogin } from '../../utils/auth'
import { createBookingApi } from '../../api'
import { userErrorMessage } from '../../utils/userMessage'
import { clearBookingDraft, loadBookingDraft, saveBookingDraft } from '../../utils/bookingDraft'
import { consumeBookingIntent } from '../../utils/bookingIntent'
import { getStoredSiteConfig } from '../../utils/siteConfig'
import { normalizePersonalExpertHome } from '../../utils/personalExpertHome'

const kinds = [
  { value: 'consult', label: '1v1 咨询' },
  { value: 'course', label: '课程报名' },
  { value: 'enterprise', label: '企业服务' },
]
const ENTERPRISE_KIND_INDEX = kinds.findIndex((item) => item.value === 'enterprise')
const kindIndex = ref(ENTERPRISE_KIND_INDEX)
const emptyForm = () => ({ contactName: '', phone: '', intent: '', preferredTime: '', message: '' })
const form = ref(emptyForm())
const fieldErrors = ref({ contactName: '', phone: '' })
const submitting = ref(false)
const submitted = ref(false)
const DRAFT_SAVE_DELAY = 250
let draftSaveTimer = null

const siteConfig = getStoredSiteConfig() || {}
const enterpriseView = ref(normalizePersonalExpertHome(siteConfig).enterprise)
const defaultScenarios = Object.freeze([
  { title: '团队协作需要共同语言', description: '让不同风格的人看见彼此动机，减少误解。' },
  { title: '管理者需要提升带队觉察', description: '帮助管理者理解沟通、防御与激励方式。' },
  { title: '关键团队进入调整期', description: '在变化中建立共识，把讨论落到行动。' },
])
const scenarioDescriptions = Object.freeze([
  '围绕真实工作场景，让团队把理解带回协作现场。',
  '适合管理者、项目负责人和高频协作团队。',
  '把九型语言转化为会议、沟通与复盘工具。',
  '可按企业阶段定制主题与练习方式。',
])
const scenarioItems = computed(() => {
  const modules = Array.isArray(enterpriseView.value.modules)
    ? enterpriseView.value.modules.map((item) => String(item || '').trim()).filter(Boolean)
    : []
  if (!modules.length) return defaultScenarios.map((item) => ({ ...item }))
  const fallbackDescription = scenarioDescriptions[scenarioDescriptions.length - 1]
  return modules.slice(0, 4).map((title, index) => ({
    title,
    description: scenarioDescriptions[index] || fallbackDescription,
  }))
})

const serviceModes = Object.freeze([
  { title: '企业内训', description: '围绕企业当下议题设计半天或全天共学。' },
  { title: '团队工作坊', description: '用互动练习帮助团队建立沟通和协作共识。' },
  { title: '管理者培训', description: '支持管理者识别不同类型成员的动机与压力反应。' },
])
const selectedServiceModeIndex = ref(-1)
const processSteps = computed(() => [
  { title: '需求沟通', description: '先了解团队背景、参与对象和希望解决的问题。' },
  { title: '方案共创', description: '结合九型主题、课件内容和企业节奏设计服务方式。' },
  { title: '落地交付', description: '完成课程或工作坊后，沉淀可复盘的团队语言。' },
])

const draft = loadBookingDraft()
const restoredDraftNotice = ref(!!draft)
if (draft) {
  const restoredKindIndex = kindIndexFor(draft.kind)
  if (restoredKindIndex >= 0) kindIndex.value = restoredKindIndex
  form.value = { ...emptyForm(), ...draft }
  delete form.value.kind
}

function kindIndexFor(kind) {
  return kinds.findIndex((item) => item.value === kind)
}

function currentKind() {
  return kinds[kindIndex.value]?.value || 'enterprise'
}

function currentDraft() {
  return { kind: currentKind(), ...form.value }
}

function cancelPendingDraftSave() {
  if (draftSaveTimer === null) return
  clearTimeout(draftSaveTimer)
  draftSaveTimer = null
}

function scheduleDraftSave() {
  if (submitted.value) return
  if (draftSaveTimer !== null) clearTimeout(draftSaveTimer)
  draftSaveTimer = setTimeout(() => {
    draftSaveTimer = null
    if (!submitted.value) saveBookingDraft(currentDraft())
  }, DRAFT_SAVE_DELAY)
}

function flushDraftSave() {
  if (submitted.value) return
  if (draftSaveTimer !== null) clearTimeout(draftSaveTimer)
  draftSaveTimer = null
  saveBookingDraft(currentDraft())
}

watch(
  [kindIndex, form],
  scheduleDraftSave,
  { deep: true },
)

onShow(applyBookingIntent)
onHide(flushDraftSave)
onUnload(flushDraftSave)

function applyBookingIntent() {
  const intent = consumeBookingIntent()
  if (!intent) return null

  if (submitted.value) submitted.value = false

  const nextKindIndex = kindIndexFor(intent.kind)
  if (nextKindIndex >= 0) kindIndex.value = nextKindIndex

  if (intent.kind === 'enterprise') {
    const modeIndex = serviceModes.findIndex((item) => item.title === intent.intentText)
    if (modeIndex >= 0) selectedServiceModeIndex.value = modeIndex
  }

  if (intent.intentText && !form.value.intent.trim()) {
    form.value = { ...form.value, intent: intent.intentText }
  }

  return intent
}

function onKindChange(e) {
  const nextIndex = Number(e.detail.value)
  if (Number.isInteger(nextIndex) && kinds[nextIndex]) kindIndex.value = nextIndex
  if (currentKind() !== 'enterprise') selectedServiceModeIndex.value = -1
}

function selectServiceMode(index) {
  if (!serviceModes[index]) return
  selectedServiceModeIndex.value = index
  kindIndex.value = ENTERPRISE_KIND_INDEX
  if (!form.value.intent.trim()) {
    form.value = { ...form.value, intent: serviceModes[index].title }
  }
}

function clearFieldError(field) {
  if (!fieldErrors.value[field]) return
  fieldErrors.value = { ...fieldErrors.value, [field]: '' }
}

function resetForm() {
  cancelPendingDraftSave()
  kindIndex.value = ENTERPRISE_KIND_INDEX
  selectedServiceModeIndex.value = -1
  form.value = emptyForm()
  fieldErrors.value = { contactName: '', phone: '' }
  restoredDraftNotice.value = false
}

function clearRestoredDraft() {
  if (submitting.value) return
  clearBookingDraft()
  resetForm()
}

function validateForm() {
  const nextErrors = { contactName: '', phone: '' }
  if (!form.value.contactName.trim()) nextErrors.contactName = '请填写称呼'
  if (!/^1\d{10}$/.test(form.value.phone.trim())) nextErrors.phone = '请填写正确手机号'
  fieldErrors.value = nextErrors
  return !nextErrors.contactName && !nextErrors.phone
}

async function submit() {
  if (!validateForm()) {
    return uni.showToast({ title: fieldErrors.value.contactName || fieldErrors.value.phone, icon: 'none' })
  }
  if (submitting.value) return
  submitting.value = true
  try {
    await ensureLogin()
    await createBookingApi(currentDraft())
    uni.showToast({ title: '预约已提交', icon: 'success' })
    cancelPendingDraftSave()
    clearBookingDraft()
    submitted.value = true
    resetForm()
  } catch (e) {
    uni.showToast({ title: userErrorMessage(e, '提交失败，请重试'), icon: 'none' })
  } finally {
    submitting.value = false
  }
}

function viewBookingRecords() {
  uni.navigateTo({ url: '/pages/booking-records/booking-records' })
}

function continueClassroom() {
  uni.switchTab({ url: '/pages/learn/learn' })
}

function submitAnother() {
  submitted.value = false
  resetForm()
}
</script>

<template>
  <view class="wrap booking page-stack ios-page ios-safe-bottom">
    <view v-if="submitted" class="booking-success nx-card" aria-live="polite">
      <text class="booking-success__eyebrow">预约已提交</text>
      <text class="booking-success__title">老师会尽快与你确认企业需求</text>
      <text class="booking-success__lead">你可以查看预约记录，也可以继续浏览老师课堂了解视频与音频课件。</text>
      <view class="booking-success__actions">
        <button class="booking-success__primary" @click="viewBookingRecords">查看预约记录</button>
        <button class="booking-success__secondary" @click="continueClassroom">继续浏览老师课堂</button>
        <button class="booking-success__text" @click="submitAnother">再提交一个需求</button>
      </view>
    </view>

    <block v-else>
      <view class="enterprise-hero nx-card">
        <text class="enterprise-hero__eyebrow">{{ enterpriseView.eyebrow }}</text>
        <text class="enterprise-hero__title">{{ enterpriseView.title }}</text>
        <text class="enterprise-hero__lead">{{ enterpriseView.lead }}</text>
      </view>

      <view class="enterprise-scenarios">
        <view class="section-heading">
          <text class="section-heading__eyebrow">适用场景</text>
          <text class="section-heading__title">把九型语言带进真实团队议题</text>
        </view>
        <view class="enterprise-scenarios__grid">
          <view v-for="item in scenarioItems" :key="item.title" class="enterprise-scenario nx-card">
            <text class="enterprise-scenario__title">{{ item.title }}</text>
            <text class="enterprise-scenario__description">{{ item.description }}</text>
          </view>
        </view>
      </view>

      <view class="enterprise-modes">
        <view class="section-heading">
          <text class="section-heading__eyebrow">服务方式</text>
          <text class="section-heading__title">选择你想先沟通的方向</text>
        </view>
        <view class="enterprise-modes__list">
          <button
            v-for="(mode, index) in serviceModes"
            :key="mode.title"
            :class="['enterprise-mode', { 'enterprise-mode--active': selectedServiceModeIndex === index }]"
            hover-class="enterprise-mode--pressed"
            @click="selectServiceMode(index)"
          >
            <view class="enterprise-mode__copy">
              <text class="enterprise-mode__title">{{ mode.title }}</text>
              <text class="enterprise-mode__description">{{ mode.description }}</text>
            </view>
            <text class="enterprise-mode__action">预约沟通</text>
          </button>
        </view>
        <view v-if="enterpriseView.services.length" class="enterprise-modes__courseware">
          <text v-for="service in enterpriseView.services" :key="service.title" class="enterprise-modes__pill">{{ service.title }}</text>
        </view>
      </view>

      <view class="enterprise-process">
        <view class="section-heading">
          <text class="section-heading__eyebrow">合作流程</text>
          <text class="section-heading__title">从一次沟通到一次可落地的共学</text>
        </view>
        <view class="enterprise-process__list">
          <view v-for="(step, index) in processSteps" :key="step.title" class="enterprise-process__item nx-card">
            <text class="enterprise-process__index">{{ index + 1 }}</text>
            <view class="enterprise-process__copy">
              <text class="enterprise-process__title">{{ step.title }}</text>
              <text class="enterprise-process__description">{{ step.description }}</text>
            </view>
          </view>
        </view>
      </view>

      <view v-if="restoredDraftNotice" class="draft-restored nx-card" aria-live="polite">
        <view class="draft-restored__copy">
          <text class="draft-restored__title">已恢复上次填写的草稿</text>
          <text class="draft-restored__hint">你可以继续填写，或清空后重新开始。</text>
        </view>
        <button class="draft-restored__clear" :disabled="submitting" @click="clearRestoredDraft">清空草稿</button>
      </view>

      <view class="booking-form nx-card">
        <view class="section-heading">
          <text class="section-heading__eyebrow">预约表单</text>
          <text class="section-heading__title">留下企业需求与联系方式</text>
          <text class="section-heading__lead">未提交前，草稿会自动保存在当前设备。</text>
        </view>

        <view class="form-section">
          <view class="field">
            <text class="label">预约类型</text>
            <picker aria-label="预约类型" :range="kinds" range-key="label" :value="kindIndex" @change="onKindChange">
              <view class="picker field-control">
                <text>{{ kinds[kindIndex].label }}</text>
                <view class="picker__arrow" aria-hidden="true" />
              </view>
            </picker>
          </view>
          <view class="field">
            <text class="label">称呼</text>
            <input
              class="input field-control"
              v-model="form.contactName"
              aria-label="称呼"
              aria-describedby="contact-name-error"
              placeholder="怎么称呼你"
              :aria-invalid="!!fieldErrors.contactName"
              @input="clearFieldError('contactName')"
            />
            <text v-if="fieldErrors.contactName" id="contact-name-error" class="field-error" role="alert">{{ fieldErrors.contactName }}</text>
          </view>
          <view class="field">
            <text class="label">手机号</text>
            <input
              class="input field-control"
              v-model="form.phone"
              aria-label="手机号"
              aria-describedby="phone-error"
              type="number"
              maxlength="11"
              placeholder="方便老师联系"
              :aria-invalid="!!fieldErrors.phone"
              @input="clearFieldError('phone')"
            />
            <text v-if="fieldErrors.phone" id="phone-error" class="field-error" role="alert">{{ fieldErrors.phone }}</text>
          </view>
          <view class="field">
            <text class="label">意向方向</text>
            <input class="input field-control" v-model="form.intent" aria-label="意向方向" placeholder="如：企业内训 / 团队工作坊 / 管理者培训" />
          </view>
          <view class="field">
            <text class="label">期望时间</text>
            <input class="input field-control" v-model="form.preferredTime" aria-label="期望时间" placeholder="如：工作日上午 / 下月内" />
          </view>
          <view class="field">
            <text class="label">留言</text>
            <textarea class="textarea field-control" v-model="form.message" aria-label="留言" placeholder="团队背景、人数范围或想先解决的问题（选填）" />
          </view>
          <text class="draft-hint">填写内容会自动保存在当前设备</text>
        </view>

        <!-- #ifdef H5 -->
        <button class="booking-submit booking-submit--disabled ios-button" disabled>请在微信小程序内提交预约</button>
        <!-- #endif -->
        <!-- #ifndef H5 -->
        <button class="booking-submit ios-button" :loading="submitting" :disabled="submitting" @click="submit">{{ enterpriseView.buttonText }}</button>
        <!-- #endif -->
      </view>
    </block>
  </view>
</template>

<style scoped>
.booking {
  --booking-gold-soft: rgba(223, 188, 127, 0.18);
  --booking-gold-border: rgba(223, 188, 127, 0.34);
  --booking-on-brand-muted: rgba(255, 255, 255, 0.82);
  --booking-brand-shadow: rgba(32, 42, 55, 0.28);
  gap: 32rpx;
  overflow-x: hidden;
  color: var(--nx-text);
}

button {
  box-sizing: border-box;
  margin: 0;
}

button::after {
  border: 0;
}

.enterprise-hero,
.booking-success {
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  padding: 42rpx 36rpx;
  border: 2rpx solid var(--booking-gold-border);
  border-radius: 38rpx;
  background:
    linear-gradient(135deg, var(--nx-brand-900), var(--nx-brand-700));
  box-shadow: 0 28rpx 62rpx -40rpx var(--booking-brand-shadow);
}

.enterprise-hero__eyebrow,
.booking-success__eyebrow,
.section-heading__eyebrow {
  color: var(--nx-accent-gold);
  font-size: 23rpx;
  font-weight: 900;
  letter-spacing: 2rpx;
}

.enterprise-hero__title,
.booking-success__title {
  display: block;
  color: var(--nx-surface);
  font-size: 46rpx;
  font-weight: 900;
  line-height: 1.22;
}

.enterprise-hero__lead,
.booking-success__lead {
  display: block;
  color: var(--booking-on-brand-muted);
  font-size: 26rpx;
  line-height: 1.68;
}

.enterprise-scenarios,
.enterprise-modes,
.enterprise-process,
.booking-form {
  display: flex;
  flex-direction: column;
  gap: 22rpx;
}

.section-heading {
  display: flex;
  flex-direction: column;
  gap: 9rpx;
}

.section-heading__eyebrow {
  color: var(--nx-brand-700);
}

.section-heading__title {
  color: var(--nx-brand-900);
  font-size: 36rpx;
  font-weight: 900;
  line-height: 1.32;
}

.section-heading__lead {
  color: var(--nx-text-muted);
  font-size: 24rpx;
  line-height: 1.58;
}

.enterprise-scenarios__grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14rpx;
}

.enterprise-scenario,
.enterprise-process__item {
  padding: 26rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 28rpx;
  background: var(--nx-surface);
}

.enterprise-scenario__title,
.enterprise-process__title,
.enterprise-mode__title,
.draft-restored__title {
  color: var(--nx-brand-900);
  font-size: 27rpx;
  font-weight: 900;
  line-height: 1.4;
}

.enterprise-scenario__description,
.enterprise-process__description,
.enterprise-mode__description,
.draft-restored__hint,
.draft-hint {
  display: block;
  margin-top: 8rpx;
  color: var(--nx-text-muted);
  font-size: 23rpx;
  line-height: 1.55;
}

.enterprise-modes__list,
.enterprise-process__list {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
}

.enterprise-mode {
  width: 100%;
  min-height: 104rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
  padding: 26rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 28rpx;
  background: var(--nx-surface);
  color: var(--nx-text);
  text-align: left;
  transition: opacity 180ms ease-out, transform 180ms ease-out, border-color 180ms ease-out;
}

.enterprise-mode--active {
  border-color: var(--nx-accent-gold);
  background: linear-gradient(110deg, var(--booking-gold-soft), var(--nx-surface));
}

.enterprise-mode--pressed,
.booking-submit--pressed,
.booking-success__primary--pressed,
.booking-success__secondary--pressed,
.booking-success__text--pressed {
  opacity: .78;
  transform: scale(.99);
}

.enterprise-mode__copy {
  flex: 1;
  min-width: 0;
}

.enterprise-mode__action {
  flex: none;
  color: var(--nx-brand-700);
  font-size: 22rpx;
  font-weight: 900;
}

.enterprise-modes__courseware {
  display: flex;
  flex-wrap: wrap;
  gap: 12rpx;
}

.enterprise-modes__pill {
  padding: 10rpx 18rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 999rpx;
  background: var(--nx-surface-soft);
  color: var(--nx-brand-700);
  font-size: 21rpx;
  font-weight: 700;
}

.enterprise-process__item {
  display: flex;
  align-items: flex-start;
  gap: 20rpx;
}

.enterprise-process__index {
  min-width: 54rpx;
  height: 54rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 18rpx;
  background: var(--nx-brand-900);
  color: var(--nx-accent-gold);
  font-size: 24rpx;
  font-weight: 900;
}

.enterprise-process__copy {
  flex: 1;
  min-width: 0;
}

.draft-restored {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
  padding: 24rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 28rpx;
  background: var(--nx-surface-soft);
}

.draft-restored__copy {
  flex: 1;
  min-width: 0;
}

.draft-restored__clear {
  flex: none;
  min-width: 152rpx;
  min-height: 88rpx;
  padding: 0 22rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 999rpx;
  background: var(--nx-surface);
  color: var(--nx-brand-700);
  font-size: 24rpx;
  font-weight: 900;
  line-height: 84rpx;
}

.booking-form {
  padding: 32rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 34rpx;
  background: var(--nx-surface);
}

.form-section {
  display: flex;
  flex-direction: column;
  gap: 20rpx;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 10rpx;
}

.label {
  display: block;
  color: var(--nx-brand-900);
  font-size: 25rpx;
  font-weight: 800;
}

.field-control {
  box-sizing: border-box;
  width: 100%;
  border: 2rpx solid var(--nx-border);
  border-radius: 24rpx;
  background: var(--nx-surface-soft);
  color: var(--nx-text);
  font-size: 28rpx;
}

.input {
  display: block;
  min-height: 88rpx;
  padding: 0 24rpx;
}

.field-error {
  display: block;
  color: var(--nx-danger);
  font-size: 24rpx;
  font-weight: 700;
  line-height: 1.5;
}

.textarea {
  display: block;
  height: 176rpx;
  min-height: 176rpx;
  padding: 20rpx 24rpx;
  line-height: 1.55;
}

.picker {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 88rpx;
  padding: 0 62rpx 0 24rpx;
}

.picker__arrow {
  position: absolute;
  right: 28rpx;
  top: 30rpx;
  width: 18rpx;
  height: 18rpx;
  border-right: 3rpx solid var(--nx-brand-700);
  border-bottom: 3rpx solid var(--nx-brand-700);
  box-sizing: border-box;
  transform: rotate(45deg);
}

.booking-submit,
.booking-success__primary,
.booking-success__secondary,
.booking-success__text {
  min-height: 88rpx;
  border-radius: 999rpx;
  font-size: 25rpx;
  font-weight: 900;
  line-height: 88rpx;
  transition: opacity 180ms ease-out, transform 180ms ease-out;
}

.booking-submit {
  width: 100%;
  margin-top: 10rpx;
  background: var(--nx-brand-900);
  color: var(--nx-surface);
}

.booking-submit--disabled {
  border: 2rpx solid var(--nx-border);
  background: var(--nx-surface-soft);
  color: var(--nx-text-muted);
  opacity: 1;
}

.booking-success__actions {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  margin-top: 10rpx;
}

.booking-success__primary {
  background: var(--nx-accent-gold);
  color: var(--nx-brand-900);
}

.booking-success__secondary {
  border: 2rpx solid var(--booking-gold-border);
  background: transparent;
  color: var(--nx-surface);
}

.booking-success__text {
  background: transparent;
  color: var(--booking-on-brand-muted);
}

@media (prefers-reduced-motion: reduce) {
  .booking button {
    transition: none;
  }
}

@media (max-width: 380px) {
  .enterprise-scenarios__grid {
    grid-template-columns: 1fr;
  }

  .draft-restored,
  .enterprise-mode {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
