<script setup>
import { ref, watch } from 'vue'
import { onHide, onUnload } from '@dcloudio/uni-app'
import { ensureLogin } from '../../utils/auth'
import { createBookingApi } from '../../api'
import { userErrorMessage } from '../../utils/userMessage'
import { clearBookingDraft, loadBookingDraft, saveBookingDraft } from '../../utils/bookingDraft'

const kinds = [
  { value: 'consult', label: '1v1 咨询' },
  { value: 'course', label: '课程报名' },
  { value: 'enterprise', label: '企业课程' },
]
const kindIndex = ref(0)
const emptyForm = () => ({ contactName: '', phone: '', intent: '', preferredTime: '', message: '' })
const form = ref(emptyForm())
const fieldErrors = ref({ contactName: '', phone: '' })
const submitting = ref(false)
const DRAFT_SAVE_DELAY = 250
let draftSaveTimer = null
const draft = loadBookingDraft()
if (draft) {
  const restoredKindIndex = kinds.findIndex((item) => item.value === draft.kind)
  if (restoredKindIndex >= 0) kindIndex.value = restoredKindIndex
  form.value = { ...emptyForm(), ...draft }
  delete form.value.kind
}

function currentDraft() {
  return { kind: kinds[kindIndex.value].value, ...form.value }
}

function cancelPendingDraftSave() {
  if (draftSaveTimer === null) return
  clearTimeout(draftSaveTimer)
  draftSaveTimer = null
}

function scheduleDraftSave() {
  if (draftSaveTimer !== null) clearTimeout(draftSaveTimer)
  draftSaveTimer = setTimeout(() => {
    draftSaveTimer = null
    saveBookingDraft(currentDraft())
  }, DRAFT_SAVE_DELAY)
}

function flushDraftSave() {
  if (draftSaveTimer !== null) clearTimeout(draftSaveTimer)
  draftSaveTimer = null
  saveBookingDraft(currentDraft())
}

watch(
  [kindIndex, form],
  scheduleDraftSave,
  { deep: true },
)

onHide(flushDraftSave)
onUnload(flushDraftSave)

function onKindChange(e) {
  kindIndex.value = Number(e.detail.value)
}

function clearFieldError(field) {
  if (!fieldErrors.value[field]) return
  fieldErrors.value = { ...fieldErrors.value, [field]: '' }
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
    await createBookingApi({ kind: kinds[kindIndex.value].value, ...form.value })
    uni.showToast({ title: '预约已提交', icon: 'success' })
    cancelPendingDraftSave()
    clearBookingDraft()
    kindIndex.value = 0
    form.value = emptyForm()
    fieldErrors.value = { contactName: '', phone: '' }
  } catch (e) {
    uni.showToast({ title: userErrorMessage(e, '提交失败，请重试'), icon: 'none' })
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <view class="wrap booking page-stack ios-page ios-safe-bottom">
    <view class="booking-hero nx-page-hero">
      <text class="booking-hero__eyebrow">预约咨询</text>
      <text class="booking-hero__title">让老师帮你找到合适的学习方式</text>
      <text class="booking-hero__lead">填写你的需求，老师会尽快联系你。未提交前，草稿会自动保存在当前设备。</text>
    </view>

    <view class="form-section nx-panel">
      <view class="form-section__head">
        <text class="form-section__step">01</text>
        <text class="form-section__title">预约类型</text>
      </view>
      <view class="field">
        <text class="label">预约类型</text>
        <picker aria-label="预约类型" :range="kinds" range-key="label" :value="kindIndex" @change="onKindChange">
          <view class="picker field-control">
            <text>{{ kinds[kindIndex].label }}</text>
            <view class="picker__arrow" aria-hidden="true" />
          </view>
        </picker>
      </view>
    </view>

    <view class="form-section nx-panel">
      <view class="form-section__head">
        <text class="form-section__step">02</text>
        <text class="form-section__title">联系信息</text>
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
    </view>

    <view class="form-section nx-panel">
      <view class="form-section__head">
        <text class="form-section__step">03</text>
        <text class="form-section__title">学习意向</text>
      </view>
      <view class="field">
        <text class="label">意向方向</text>
        <input class="input field-control" v-model="form.intent" aria-label="意向方向" placeholder="如：亲子关系 / 个人成长 / 团队" />
      </view>
      <view class="field">
        <text class="label">期望时间</text>
        <input class="input field-control" v-model="form.preferredTime" aria-label="期望时间" placeholder="如：周末 / 工作日晚上" />
      </view>
      <view class="field">
        <text class="label">留言</text>
        <textarea class="textarea field-control" v-model="form.message" aria-label="留言" placeholder="想了解的问题（选填）" />
      </view>
      <text class="draft-hint">填写内容会自动保存在当前设备</text>
    </view>

    <!-- #ifdef H5 -->
    <button class="booking-submit booking-submit--disabled ios-button" disabled>请在微信小程序内提交预约</button>
    <!-- #endif -->
    <!-- #ifndef H5 -->
    <button class="btn-primary booking-submit ios-button" :loading="submitting" :disabled="submitting" @click="submit">提交预约</button>
    <!-- #endif -->
  </view>
</template>

<style scoped>
.booking {
  overflow-x: hidden;
}
.booking-hero {
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  border-radius: 38rpx;
  background: linear-gradient(145deg, #c2410c, #2563eb);
  box-shadow: 0 30rpx 64rpx -38rpx rgba(37, 99, 235, .72);
}
.booking-hero__eyebrow {
  color: #fff;
  font-size: 24rpx;
  font-weight: 800;
}
.booking-hero__title {
  display: block;
  color: #fff;
  font-size: 48rpx;
  font-weight: 900;
  line-height: 1.22;
}
.booking-hero__lead {
  display: block;
  color: rgba(255, 255, 255, .94);
  font-size: 26rpx;
  line-height: 1.65;
}
.form-section {
  display: flex;
  flex-direction: column;
  gap: 20rpx;
  padding: 30rpx;
}
.form-section__head {
  display: flex;
  align-items: center;
  gap: 14rpx;
}
.form-section__step {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 52rpx;
  height: 52rpx;
  border-radius: 16rpx;
  background: rgba(37, 99, 235, .10);
  color: #2563eb;
  font-size: 24rpx;
  font-weight: 900;
}
.form-section__title {
  color: #0f172a;
  font-size: 31rpx;
  font-weight: 900;
  line-height: 1.3;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 10rpx;
}
.label {
  display: block;
  color: #334155;
  font-size: 25rpx;
  font-weight: 800;
}
.field-control {
  color: #0f172a;
  font-size: 28rpx;
}
.input {
  display: block;
  min-height: 88rpx;
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
  line-height: 1.55;
}
.picker {
  position: relative;
  color: #12151b;
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 88rpx;
  padding-right: 62rpx;
}
.picker__arrow {
  position: absolute;
  right: 28rpx;
  top: 30rpx;
  width: 18rpx;
  height: 18rpx;
  border-right: 3rpx solid #2563eb;
  border-bottom: 3rpx solid #2563eb;
  transform: rotate(45deg);
  box-sizing: border-box;
}
.draft-hint {
  display: block;
  color: #64748b;
  font-size: 24rpx;
  line-height: 1.55;
}
.booking-submit {
  width: 100%;
  min-height: 88rpx;
  font-size: 30rpx;
  font-weight: 800;
  box-sizing: border-box;
}
.booking-submit--disabled {
  border: 2rpx solid #cbd5e1;
  background: #f1f5f9;
  color: #475569;
  opacity: 1;
}
.booking-submit--disabled::after {
  border: none;
}
</style>
