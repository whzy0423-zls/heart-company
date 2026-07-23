<script setup>
import { computed, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { TYPES_INFO } from '../../data/enneagramGame'
import { ensureLogin, getToken, clearToken } from '../../utils/auth'
import { hiddenCount, previewItems } from '../../utils/listPreview'
import { normalizeWechatProfile, hasProfilePayload, getWechatProfilePayload } from '../../utils/wechatProfile'
import { userErrorMessage } from '../../utils/userMessage'
import { getUserInfoApi, updateUserInfoApi, listTestRecordsApi, listBookingsApi } from '../../api'

const logged = ref(false)
const user = ref(null)
const records = ref([])
const bookings = ref([])
const recordsError = ref('')
const bookingsError = ref('')
const logging = ref(false)
const profileLoading = ref(false)
const profileSaving = ref(false)
const nicknameDraft = ref('')
const avatarDraft = ref('')
const userAvatarFailed = ref(false)
const draftAvatarFailed = ref(false)
const visibleRecords = computed(() => previewItems(records.value))
const visibleBookings = computed(() => previewItems(bookings.value))
const hiddenRecordCount = computed(() => hiddenCount(records.value))
const hiddenBookingCount = computed(() => hiddenCount(bookings.value))
const recordCount = computed(() => records.value.length)
const bookingCount = computed(() => bookings.value.length)
const recordCountLabel = computed(() => profileLoading.value || recordsError.value ? '—' : String(recordCount.value))
const bookingCountLabel = computed(() => profileLoading.value || bookingsError.value ? '—' : String(bookingCount.value))
const wechatLoginReady = computed(() => ({
  codeLogin: true,
  profile: true,
  phone: false,
  note: '微信 code 登录已接入；头像昵称已按微信新规范支持。',
}))
let loadTicket = 0
let sessionGeneration = 0

onShow(() => {
  logged.value = !!getToken()
  if (logged.value) loadAll()
})

async function login() {
  if (logging.value) return
  let generation = sessionGeneration
  logging.value = true
  try {
    await ensureLogin()
    sessionGeneration += 1
    generation = sessionGeneration
    logged.value = true
    await loadAll()
    if (!logged.value || generation !== sessionGeneration) return
    uni.showToast({ title: '登录成功', icon: 'success' })
  } catch (e) {
    if (generation !== sessionGeneration) return
    uni.showToast({ title: userErrorMessage(e, '登录失败'), icon: 'none' })
  } finally {
    if (generation === sessionGeneration) logging.value = false
  }
}

async function loadAll() {
  const ticket = ++loadTicket
  profileLoading.value = true
  recordsError.value = ''
  bookingsError.value = ''
  try {
    const loadedUser = await getUserInfoApi()
    if (ticket !== loadTicket) return
    user.value = loadedUser
    syncDraftFromUser()
  } catch (e) {
    if (ticket !== loadTicket) return
    resetLogin()
    uni.showToast({ title: '登录已过期，请重新登录', icon: 'none' })
    profileLoading.value = false
    return
  }

  const [rec, bk] = await Promise.allSettled([
    listTestRecordsApi(),
    listBookingsApi(),
  ])

  if (ticket !== loadTicket) return
  if (rec.status === 'fulfilled') {
    records.value = rec.value.items || []
  } else {
    recordsError.value = userErrorMessage(rec.reason, '同步失败，重试')
  }
  if (bk.status === 'fulfilled') {
    bookings.value = bk.value.items || []
  } else {
    bookingsError.value = userErrorMessage(bk.reason, '同步失败，重试')
  }
  profileLoading.value = false
}

function typeName(id) {
  return TYPES_INFO[id] ? `${id} 号 · ${TYPES_INFO[id].name}` : '—'
}

function syncDraftFromUser() {
  nicknameDraft.value = (user.value && user.value.nickname) || ''
  avatarDraft.value = (user.value && user.value.avatar) || ''
  userAvatarFailed.value = false
  draftAvatarFailed.value = false
}

function resetLogin() {
  sessionGeneration += 1
  loadTicket += 1
  clearToken()
  logged.value = false
  user.value = null
  records.value = []
  bookings.value = []
  recordsError.value = ''
  bookingsError.value = ''
  nicknameDraft.value = ''
  avatarDraft.value = ''
  userAvatarFailed.value = false
  draftAvatarFailed.value = false
  profileLoading.value = false
  profileSaving.value = false
  logging.value = false
}

function logout() {
  resetLogin()
}

function onChooseAvatar(e) {
  avatarDraft.value = e.detail && e.detail.avatarUrl ? e.detail.avatarUrl : ''
  draftAvatarFailed.value = false
}

function onUserAvatarError() {
  userAvatarFailed.value = true
}

function onDraftAvatarError() {
  draftAvatarFailed.value = true
}

function onNicknameInput(e) {
  nicknameDraft.value = e.detail && e.detail.value ? e.detail.value : ''
}


async function syncWechatProfile() {
  if (profileSaving.value) return
  const generation = sessionGeneration
  profileSaving.value = true
  try {
    const payload = await getWechatProfilePayload()
    if (!logged.value || generation !== sessionGeneration) return
    if (hasProfilePayload(payload)) {
      const updatedUser = await updateUserInfoApi(payload)
      if (!logged.value || generation !== sessionGeneration) return
      user.value = updatedUser
      syncDraftFromUser()
      uni.showToast({ title: '资料已同步', icon: 'success' })
    } else {
      uni.showToast({ title: '请用下方头像昵称补充资料', icon: 'none' })
    }
  } catch {
    if (!logged.value || generation !== sessionGeneration) return
    uni.showToast({ title: '可手动补充头像昵称', icon: 'none' })
  } finally {
    if (generation === sessionGeneration) profileSaving.value = false
  }
}

async function saveProfile() {
  if (profileSaving.value) return
  const generation = sessionGeneration
  const payload = normalizeWechatProfile({
    nickname: nicknameDraft.value,
    avatar: avatarDraft.value,
  })
  if (!hasProfilePayload(payload)) {
    uni.showToast({ title: '请先填写昵称或选择头像', icon: 'none' })
    return
  }

  profileSaving.value = true
  try {
    const updatedUser = await updateUserInfoApi(payload)
    if (!logged.value || generation !== sessionGeneration) return
    user.value = updatedUser
    syncDraftFromUser()
    uni.showToast({ title: '资料已保存', icon: 'success' })
  } catch {
    if (!logged.value || generation !== sessionGeneration) return
    uni.showToast({ title: '保存失败，请重试', icon: 'none' })
  } finally {
    if (generation === sessionGeneration) profileSaving.value = false
  }
}
</script>

<template>
  <view class="wrap profile page-stack ios-page ios-safe-bottom">
    <view v-if="!logged" class="profile-hero nx-page-hero login">
      <view class="profile-hero__mark">九</view>
      <text class="profile-hero__eyebrow">个人档案</text>
      <text class="profile-hero__title">记录每一次自我看见</text>
      <text class="profile-hero__lead">登录后沉淀你的九型档案、测试历史和预约记录。</text>
      <!-- #ifdef H5 -->
      <button class="profile-login ios-button" disabled>请在微信小程序内登录</button>
      <text class="login__hint">H5 可浏览公开内容；保存档案和预约记录请打开微信小程序。</text>
      <!-- #endif -->
      <!-- #ifndef H5 -->
      <button class="profile-login ios-button" :loading="logging" :disabled="logging" @click="login">微信一键登录</button>
      <!-- #endif -->
    </view>

    <template v-else>
      <view class="profile-hero nx-page-hero user">
        <view class="profile-hero__identity">
          <image v-if="user && user.avatar && !userAvatarFailed" class="user__avatar" :src="user.avatar" mode="aspectFill" lazy-load @error="onUserAvatarError" />
        <view v-else class="user__avatar user__avatar--ph">{{ (user && user.mainType) || '九' }}</view>
          <view class="user__info">
            <text class="profile-hero__eyebrow">个人档案</text>
            <text class="user__name">{{ (user && user.nickname) || '九型用户' }}</text>
            <text class="user__type" v-if="user && user.mainType">{{ typeName(user.mainType) }}</text>
            <text class="user__type" v-else>已通过微信登录</text>
          </view>
        </view>
        <text class="profile-hero__title">记录每一次自我看见</text>
        <text class="profile-hero__lead">你的成长轨迹，正在每一次探索中变得更清晰。</text>
        <view class="profile-stats">
          <view class="profile-stat">
            <text class="profile-stat__value">{{ user && user.mainType ? `${user.mainType}号` : '—' }}</text>
            <text class="profile-stat__label">主型</text>
          </view>
          <view class="profile-stat">
            <text class="profile-stat__value">{{ recordCountLabel }}</text>
            <text class="profile-stat__label">测试</text>
          </view>
          <view class="profile-stat">
            <text class="profile-stat__value">{{ bookingCountLabel }}</text>
            <text class="profile-stat__label">预约</text>
          </view>
        </view>
      </view>

      <view class="profile-edit nx-panel ios-card profile-form">
        <view class="profile-form__head">
          <view>
            <text class="section-kicker">个人信息</text>
            <text class="sec-title">微信资料</text>
          </view>
          <button class="mini-link" :loading="profileSaving" :disabled="profileSaving" @click="syncWechatProfile">一键同步</button>
        </view>
        <view class="wechat-slot">
          <text class="wechat-slot__title">微信登录能力</text>
          <text class="wechat-slot__desc">{{ wechatLoginReady.note }}</text>
        </view>
        <view class="profile-form__row">
          <button class="avatar-picker" open-type="chooseAvatar" @chooseavatar="onChooseAvatar">
            <image v-if="avatarDraft && !draftAvatarFailed" class="avatar-picker__img" :src="avatarDraft" mode="aspectFill" lazy-load @error="onDraftAvatarError" />
            <text v-else class="avatar-picker__ph">{{ (user && user.mainType) || '头像' }}</text>
          </button>
          <view class="nickname-field">
            <text class="nickname-field__label">昵称</text>
            <input
              class="nickname-field__input"
              type="nickname"
              :value="nicknameDraft"
              placeholder="填写微信昵称"
              @input="onNicknameInput"
              @blur="onNicknameInput"
            />
          </view>
        </view>
        <button class="btn-primary profile-form__save" :loading="profileSaving" :disabled="profileSaving" @click="saveProfile">保存资料</button>
      </view>

      <view class="history-section nx-panel ios-card">
        <view class="section-head">
          <view>
            <text class="section-kicker">自我探索</text>
            <text class="sec-title">我的测试历史</text>
          </view>
          <text class="section-count">{{ recordCountLabel }}</text>
        </view>
        <view v-if="profileLoading" class="empty">正在同步测试历史…</view>
        <view v-else-if="recordsError" class="empty empty--error">
          <text>{{ recordsError }}</text>
          <button class="sync-retry" @click="loadAll">重试</button>
        </view>
        <view v-else-if="records.length === 0" class="empty">还没有记录，去测一测吧</view>
        <view v-else class="history-timeline">
          <view v-for="rec in visibleRecords" :key="rec.id" class="history-item">
            <view class="history-item__rail"><view class="history-item__dot" /></view>
            <view class="history-item__body">
              <text class="history-item__main">{{ typeName(rec.resultType) }}</text>
              <text class="history-item__meta">{{ rec.createTime }}</text>
            </view>
          </view>
          <text v-if="hiddenRecordCount" class="more-tip">还有 {{ hiddenRecordCount }} 条记录已收起</text>
        </view>
      </view>

      <view class="history-section nx-panel ios-card">
        <view class="section-head">
          <view>
            <text class="section-kicker">持续陪伴</text>
            <text class="sec-title">我的预约</text>
          </view>
          <text class="section-count">{{ bookingCountLabel }}</text>
        </view>
        <view v-if="profileLoading" class="empty">正在同步预约记录…</view>
        <view v-else-if="bookingsError" class="empty empty--error">
          <text>{{ bookingsError }}</text>
          <button class="sync-retry" @click="loadAll">重试</button>
        </view>
        <view v-else-if="bookings.length === 0" class="empty">暂无预约</view>
        <view v-else class="history-timeline">
          <view v-for="b in visibleBookings" :key="b.id" class="history-item">
            <view class="history-item__rail"><view class="history-item__dot" /></view>
            <view class="history-item__body">
              <text class="history-item__main">{{ b.intent || b.kind }}</text>
              <text class="history-item__meta">{{ b.status }} · {{ b.createTime }}</text>
            </view>
          </view>
          <text v-if="hiddenBookingCount" class="more-tip">还有 {{ hiddenBookingCount }} 条预约已收起</text>
        </view>
      </view>

      <button class="logout ios-button" @click="logout">退出登录</button>
    </template>
  </view>
</template>

<style scoped>
.profile { gap: 24rpx; overflow-x: hidden; }
.profile-hero { box-sizing: border-box; width: 100%; padding: 36rpx; border-radius: 38rpx; background: linear-gradient(145deg, #172554, #4338ca 56%, #7c3aed); color: #ffffff; box-shadow: 0 26rpx 54rpx -34rpx rgba(49, 46, 129, .78); overflow: hidden; }
.login { display: flex; flex-direction: column; align-items: center; text-align: center; gap: 18rpx; padding-top: 54rpx; padding-bottom: 50rpx; }
.profile-hero__mark { width: 116rpx; height: 116rpx; border-radius: 34rpx; background: rgba(255, 255, 255, .16); border: 2rpx solid rgba(255, 255, 255, .32); color: #ffffff; font-size: 50rpx; font-weight: 900; display: flex; align-items: center; justify-content: center; }
.profile-hero__eyebrow { color: #ddd6fe; font-size: 24rpx; font-weight: 800; line-height: 1.45; }
.profile-hero__title { display: block; margin-top: 10rpx; color: #ffffff; font-size: 42rpx; font-weight: 900; line-height: 1.25; }
.profile-hero__lead { display: block; margin-top: 14rpx; color: #ede9fe; font-size: 25rpx; line-height: 1.65; }
.profile-login { width: 100%; min-height: 88rpx; margin-top: 16rpx; border-radius: 24rpx; background: #ffffff; color: #312e81; font-size: 28rpx; font-weight: 900; }
.profile-login::after { border: none; }
.login__hint { color: #ddd6fe; font-size: 24rpx; line-height: 1.55; }
.profile-hero__identity { display: flex; align-items: center; gap: 22rpx; }
.profile-stats { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12rpx; margin-top: 30rpx; }
.profile-stat { min-width: 0; padding: 20rpx 10rpx; border-radius: 22rpx; background: rgba(255, 255, 255, .14); border: 2rpx solid rgba(255, 255, 255, .16); text-align: center; }
.profile-stat__value { display: block; color: #ffffff; font-size: 34rpx; font-weight: 900; line-height: 1.2; font-variant-numeric: tabular-nums; }
.profile-stat__label { display: block; margin-top: 8rpx; color: #ddd6fe; font-size: 24rpx; line-height: 1.35; }
.user__info { flex: 1; min-width: 0; }
.user__avatar { width: 116rpx; height: 116rpx; flex: 0 0 116rpx; border-radius: 50%; border: 4rpx solid rgba(255, 255, 255, .72); box-sizing: border-box; box-shadow: 0 16rpx 34rpx -26rpx rgba(15, 23, 42, .62); }
.user__avatar--ph { background: rgba(255, 255, 255, .16); color: #ffffff; font-size: 44rpx; font-weight: 900; display: flex; align-items: center; justify-content: center; }
.user__name { color: #ffffff; font-size: 35rpx; font-weight: 900; display: block; line-height: 1.28; }
.user__type { color: #ddd6fe; font-size: 25rpx; display: block; margin-top: 7rpx; }
.profile-edit, .history-section { box-sizing: border-box; width: 100%; padding: 30rpx; }
.profile-form { display: flex; flex-direction: column; gap: 22rpx; }
.profile-form__head { display: flex; align-items: center; justify-content: space-between; gap: 18rpx; }
.section-kicker { display: block; margin-bottom: 6rpx; color: #4f46e5; font-size: 24rpx; font-weight: 800; line-height: 1.35; }
.sec-title { display: block; color: #0f172a; font-size: 32rpx; font-weight: 900; line-height: 1.3; }
.mini-link { min-width: 146rpx; min-height: 88rpx; padding: 0 20rpx; border-radius: 22rpx; background: #eef2ff; color: #3730a3; font-size: 24rpx; font-weight: 900; display: flex; align-items: center; justify-content: center; }
.mini-link::after { border: none; }
.wechat-slot { margin: 0 0 2rpx; padding: 22rpx; border-radius: 22rpx; background: #f5f3ff; border: 2rpx solid #ede9fe; }
.wechat-slot__title { display: block; color: #3730a3; font-size: 25rpx; font-weight: 900; margin-bottom: 8rpx; }
.wechat-slot__desc { display: block; color: #475569; font-size: 24rpx; line-height: 1.6; }
.profile-form__row { display: flex; align-items: center; gap: 20rpx; }
.avatar-picker { width: 120rpx; height: 120rpx; padding: 0; border-radius: 38rpx; background: #eef2ff; border: 2rpx solid #c7d2fe; overflow: hidden; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.avatar-picker::after { border: none; }
.avatar-picker__img { width: 120rpx; height: 120rpx; display: block; }
.avatar-picker__ph { color: #3730a3; font-size: 24rpx; font-weight: 900; }
.nickname-field { flex: 1; min-width: 0; min-height: 120rpx; border-radius: 26rpx; background: #ffffff; border: 2rpx solid #e2e8f0; padding: 18rpx 22rpx; box-sizing: border-box; }
.nickname-field__label { color: #475569; font-size: 24rpx; font-weight: 900; display: block; }
.nickname-field__input { width: 100%; min-height: 52rpx; color: #0f172a; font-size: 30rpx; font-weight: 800; margin-top: 6rpx; }
.profile-form__save { margin-top: 4rpx; }
.section-head { display: flex; align-items: center; justify-content: space-between; gap: 20rpx; margin-bottom: 22rpx; }
.section-count { min-width: 64rpx; color: #4338ca; font-size: 30rpx; font-weight: 900; text-align: right; font-variant-numeric: tabular-nums; }
.empty { color: #475569; font-size: 25rpx; padding: 28rpx 20rpx; text-align: center; border-radius: 22rpx; background: #f8fafc; }
.empty--error { display: flex; align-items: center; justify-content: space-between; gap: 18rpx; padding: 18rpx 20rpx; text-align: left; }
.sync-retry { flex-shrink: 0; min-width: 112rpx; min-height: 88rpx; padding: 0 20rpx; border-radius: 999rpx; background: #eef2ff; color: #3730a3; font-size: 24rpx; font-weight: 900; display: flex; align-items: center; justify-content: center; }
.sync-retry::after { border: none; }
.history-timeline { width: 100%; }
.history-item { display: flex; align-items: stretch; gap: 20rpx; min-height: 88rpx; border-bottom: 2rpx solid #e2e8f0; }
.history-item:last-of-type { border-bottom: none; }
.history-item__rail { width: 20rpx; flex: 0 0 20rpx; display: flex; justify-content: center; padding-top: 30rpx; }
.history-item__dot { width: 16rpx; height: 16rpx; border-radius: 50%; background: #6366f1; box-shadow: 0 0 0 7rpx #eef2ff; }
.history-item__body { min-width: 0; flex: 1; display: flex; flex-direction: column; justify-content: center; padding: 18rpx 0; }
.history-item__main { color: #0f172a; font-size: 28rpx; font-weight: 900; line-height: 1.35; }
.history-item__meta { display: block; margin-top: 7rpx; color: #475569; font-size: 24rpx; line-height: 1.45; }
.more-tip { display: block; margin-top: 16rpx; color: #475569; font-size: 24rpx; line-height: 1.5; }
.logout { width: 100%; min-height: 88rpx; border-radius: 24rpx; background: transparent; border: 2rpx solid #fecaca; color: #b91c1c; font-size: 26rpx; font-weight: 800; }
.logout::after { border: none; }
@media (max-width: 360px) {
  .profile-hero, .profile-edit, .history-section { padding-left: 26rpx; padding-right: 26rpx; }
}
</style>
