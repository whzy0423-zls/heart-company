<script setup>
import { ref } from 'vue'
import { ensureLogin } from '../../utils/auth'
import { createBookingApi } from '../../api'

const kinds = [
  { value: 'consult', label: '1v1 咨询' },
  { value: 'course', label: '课程报名' },
  { value: 'enterprise', label: '企业课程' },
]
const kindIndex = ref(0)
const form = ref({ contactName: '', phone: '', intent: '', preferredTime: '', message: '' })
const submitting = ref(false)

function onKindChange(e) {
  kindIndex.value = Number(e.detail.value)
}

async function submit() {
  if (!form.value.contactName.trim()) return uni.showToast({ title: '请填写称呼', icon: 'none' })
  if (!/^1\d{10}$/.test(form.value.phone.trim())) return uni.showToast({ title: '请填写正确手机号', icon: 'none' })
  if (submitting.value) return
  submitting.value = true
  try {
    await ensureLogin()
    await createBookingApi({ kind: kinds[kindIndex.value].value, ...form.value })
    uni.showToast({ title: '预约已提交', icon: 'success' })
    form.value = { contactName: '', phone: '', intent: '', preferredTime: '', message: '' }
  } catch (e) {
    uni.showToast({ title: '提交失败，请重试', icon: 'none' })
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <view class="wrap booking page-stack">
    <view class="card">
      <text class="eyebrow">预约咨询</text>
      <text class="title gradient-title">预约咨询 / 报名</text>
      <text class="sub">填写后老师会尽快与你联系，帮你匹配更适合的学习方式。</text>

      <view class="field">
        <text class="label">预约类型</text>
        <picker :range="kinds" range-key="label" :value="kindIndex" @change="onKindChange">
          <view class="picker field-control">
            <text>{{ kinds[kindIndex].label }}</text>
            <text class="picker__arrow">⌄</text>
          </view>
        </picker>
      </view>
      <view class="field">
        <text class="label">称呼</text>
        <input class="input field-control" v-model="form.contactName" placeholder="怎么称呼你" />
      </view>
      <view class="field">
        <text class="label">手机号</text>
        <input class="input field-control" v-model="form.phone" type="number" maxlength="11" placeholder="方便老师联系" />
      </view>
      <view class="field">
        <text class="label">意向方向</text>
        <input class="input field-control" v-model="form.intent" placeholder="如：亲子关系 / 个人成长 / 团队" />
      </view>
      <view class="field">
        <text class="label">期望时间</text>
        <input class="input field-control" v-model="form.preferredTime" placeholder="如：周末 / 工作日晚上" />
      </view>
      <view class="field">
        <text class="label">留言</text>
        <textarea class="textarea field-control" v-model="form.message" placeholder="想了解的问题（选填）" />
      </view>

      <button class="btn-primary" :loading="submitting" @click="submit">提交预约</button>
    </view>
  </view>
</template>

<style scoped>
.title { font-size: 46rpx; display: block; margin-top: 18rpx; }
.sub { color: #3c424d; font-size: 26rpx; line-height: 1.7; display: block; margin: 10rpx 0 30rpx; }
.field { margin-bottom: 24rpx; }
.label { font-size: 25rpx; font-weight: 800; color: #3c424d; display: block; margin-bottom: 12rpx; }
.input { display: block; }
.textarea {
  display: block;
  height: 176rpx;
  line-height: 1.55;
}
.picker {
  color: #12151b;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.picker__arrow {
  color: #2b7fff;
  font-size: 34rpx;
  font-weight: 900;
}
</style>
