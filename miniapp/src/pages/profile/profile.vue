<script setup>
import { computed, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { TYPES_INFO } from '../../data/enneagramGame'
import { ensureLogin, getToken, clearToken } from '../../utils/auth'
import { clearChatMessages } from '../../utils/chatStorage'
import { hiddenCount, previewItems } from '../../utils/listPreview'
import { openChatPage } from '../../utils/navigation'
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
const visibleRecords = computed(() => previewItems(records.value))
const visibleBookings = computed(() => previewItems(bookings.value))
const hiddenRecordCount = computed(() => hiddenCount(records.value))
const hiddenBookingCount = computed(() => hiddenCount(bookings.value))
const wechatLoginReady = computed(() => ({
  codeLogin: true,
  profile: true,
  phone: false,
  note: '微信 code 登录已接入；头像昵称已按微信新规范支持。',
}))
let loadTicket = 0

onShow(() => {
  logged.value = !!getToken()
  if (logged.value) loadAll()
})

async function login() {
  if (logging.value) return
  logging.value = true
  try {
    await ensureLogin()
    logged.value = true
    await loadAll()
    uni.showToast({ title: '登录成功', icon: 'success' })
  } catch (e) {
    uni.showToast({ title: userErrorMessage(e, '登录失败'), icon: 'none' })
  } finally {
    logging.value = false
  }
}

async function loadAll() {
  const ticket = ++loadTicket
  profileLoading.value = true
  recordsError.value = ''
  bookingsError.value = ''
  try {
    user.value = await getUserInfoApi()
    syncDraftFromUser()
  } catch (e) {
    if (ticket === loadTicket) {
      resetLogin()
      uni.showToast({ title: '登录已过期，请重新登录', icon: 'none' })
      profileLoading.value = false
    }
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
}

function resetLogin() {
  loadTicket += 1
  clearToken()
  clearChatMessages()
  logged.value = false
  user.value = null
  records.value = []
  bookings.value = []
  recordsError.value = ''
  bookingsError.value = ''
  nicknameDraft.value = ''
  avatarDraft.value = ''
  profileLoading.value = false
}

function logout() {
  resetLogin()
}

function goChat() {
  openChatPage()
}

function onChooseAvatar(e) {
  avatarDraft.value = e.detail && e.detail.avatarUrl ? e.detail.avatarUrl : ''
}

function onNicknameInput(e) {
  nicknameDraft.value = e.detail && e.detail.value ? e.detail.value : ''
}


async function syncWechatProfile() {
  if (profileSaving.value) return
  profileSaving.value = true
  try {
    const payload = await getWechatProfilePayload()
    if (hasProfilePayload(payload)) {
      user.value = await updateUserInfoApi(payload)
      syncDraftFromUser()
      uni.showToast({ title: '资料已同步', icon: 'success' })
    } else {
      uni.showToast({ title: '请用下方头像昵称补充资料', icon: 'none' })
    }
  } catch {
    uni.showToast({ title: '可手动补充头像昵称', icon: 'none' })
  } finally {
    profileSaving.value = false
  }
}

async function saveProfile() {
  if (profileSaving.value) return
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
    user.value = await updateUserInfoApi(payload)
    syncDraftFromUser()
    uni.showToast({ title: '资料已保存', icon: 'success' })
  } catch {
    uni.showToast({ title: '保存失败，请重试', icon: 'none' })
  } finally {
    profileSaving.value = false
  }
}
</script>

<template>
  <view class="wrap profile page-stack ios-page ios-safe-bottom">
    <!-- 未登录 -->
    <view v-if="!logged" class="card ios-card login">
      <view class="login__mark">九</view>
      <text class="eyebrow">个人档案</text>
      <text class="login__t">登录后可保存你的九型档案、测试历史和预约记录。</text>
      <!-- #ifdef H5 -->
      <button class="btn-primary ios-button" disabled>请在微信小程序内登录</button>
      <text class="login__hint">H5 可浏览公开内容；保存档案、AI 对话和预约记录请打开微信小程序。</text>
      <!-- #endif -->
      <!-- #ifndef H5 -->
      <button class="btn-primary ios-button" :loading="logging" :disabled="logging" @click="login">微信一键登录</button>
      <!-- #endif -->
    </view>

    <!-- 已登录 -->
    <template v-else>
      <view class="card ios-card user">
        <image v-if="user && user.avatar" class="user__avatar" :src="user.avatar" lazy-load />
        <view v-else class="user__avatar user__avatar--ph">{{ (user && user.mainType) || '九' }}</view>
        <view class="user__info">
          <text class="user__name">{{ (user && user.nickname) || '九型用户' }}</text>
          <text class="user__type" v-if="user && user.mainType">主型：{{ typeName(user.mainType) }}</text>
          <text class="user__type" v-else>已通过微信登录</text>
        </view>
        <button class="user__chat" @click="goChat">问 AI</button>
      </view>

      <view class="card ios-card profile-form">
        <view class="profile-form__head">
          <text class="sec-title">微信资料</text>
          <button class="mini-link" :loading="profileSaving" :disabled="profileSaving" @click="syncWechatProfile">一键同步</button>
        </view>
        <view class="wechat-slot">
          <text class="wechat-slot__title">微信登录能力</text>
          <text class="wechat-slot__desc">{{ wechatLoginReady.note }}</text>
        </view>
        <view class="profile-form__row">
          <button class="avatar-picker" open-type="chooseAvatar" @chooseavatar="onChooseAvatar">
            <image v-if="avatarDraft" class="avatar-picker__img" :src="avatarDraft" mode="aspectFill" lazy-load />
            <text v-else class="avatar-picker__ph">头像</text>
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

      <view class="card ios-card section-card">
        <text class="sec-title">我的测试历史</text>
        <view v-if="profileLoading" class="empty">正在同步测试历史…</view>
        <view v-else-if="recordsError" class="empty empty--error">
          <text>{{ recordsError }}</text>
          <button class="sync-retry" @click="loadAll">重试</button>
        </view>
        <view v-else-if="records.length === 0" class="empty">还没有记录，去测一测吧</view>
        <view v-for="rec in visibleRecords" :key="rec.id" class="row">
          <text class="row__main">{{ typeName(rec.resultType) }}</text>
          <text class="row__time">{{ rec.createTime }}</text>
        </view>
        <text v-if="hiddenRecordCount" class="more-tip">还有 {{ hiddenRecordCount }} 条记录已收起</text>
      </view>

      <view class="card ios-card section-card">
        <text class="sec-title">我的预约</text>
        <view v-if="profileLoading" class="empty">正在同步预约记录…</view>
        <view v-else-if="bookingsError" class="empty empty--error">
          <text>{{ bookingsError }}</text>
          <button class="sync-retry" @click="loadAll">重试</button>
        </view>
        <view v-else-if="bookings.length === 0" class="empty">暂无预约</view>
        <view v-for="b in visibleBookings" :key="b.id" class="row">
          <text class="row__main">{{ b.intent || b.kind }}</text>
          <text class="row__time">{{ b.status }} · {{ b.createTime }}</text>
        </view>
        <text v-if="hiddenBookingCount" class="more-tip">还有 {{ hiddenBookingCount }} 条预约已收起</text>
      </view>

      <button class="btn-ghost ios-button" @click="logout">退出登录</button>
    </template>
  </view>
</template>

<style scoped>
.profile {
  gap: 24rpx;
}
.login {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 20rpx;
  padding: 64rpx 34rpx;
  background:
    linear-gradient(155deg, rgba(255,255,255,.94), rgba(255,255,255,.70)),
    radial-gradient(circle at 50% 0%, rgba(37,99,235,.20), transparent 42%);
}
.login__mark {
  width: 116rpx;
  height: 116rpx;
  border-radius: 38rpx;
  background: linear-gradient(135deg,#60a5fa,#2563eb);
  color: #fff;
  font-size: 50rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 20rpx 44rpx -24rpx rgba(37,99,235,.72);
}
.login__hint {
  color: #64748b;
  font-size: 24rpx;
  line-height: 1.55;
}
.login .eyebrow { align-self: center; }
.login__t {
  color: #334155;
  font-size: 29rpx;
  line-height: 1.72;
}
.user {
  display: flex;
  align-items: center;
  gap: 22rpx;
  padding: 30rpx;
  background: linear-gradient(145deg, rgba(255,255,255,.90), rgba(239,246,255,.72));
}
.section-card {
  box-shadow: 0 16rpx 38rpx -30rpx rgba(15,23,42,.34);
}
.user__info { flex: 1; min-width: 0; }
.user__avatar {
  width: 116rpx;
  height: 116rpx;
  border-radius: 50%;
  border: 4rpx solid rgba(255,255,255,.95);
  box-sizing: border-box;
  box-shadow: 0 16rpx 34rpx -26rpx rgba(15,23,42,.42);
}
.user__avatar--ph {
  background: linear-gradient(135deg,#60a5fa,#2563eb);
  color: #fff;
  font-size: 44rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}
.user__name {
  color: #0f172a;
  font-size: 35rpx;
  font-weight: 900;
  display: block;
  line-height: 1.28;
}
.user__type {
  color: #475569;
  font-size: 25rpx;
  display: block;
  margin-top: 7rpx;
}
.user__chat {
  min-width: 112rpx;
  min-height: 88rpx;
  padding: 0 18rpx;
  border-radius: 999rpx;
  background: rgba(5,150,105,.12);
  color: #059669;
  font-size: 24rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.user__chat::after { border: none; }
.profile-form {
  display: flex;
  flex-direction: column;
  gap: 22rpx;
}
.profile-form__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
}
.mini-link {
  min-width: 146rpx;
  min-height: 88rpx;
  padding: 0 20rpx;
  border-radius: 999rpx;
  background: rgba(37,99,235,.10);
  color: #2563eb;
  font-size: 23rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}
.mini-link::after { border: none; }
.wechat-slot {
  margin: 18rpx 0 24rpx;
  padding: 22rpx;
  border-radius: 24rpx;
  background: rgba(37,99,235,.08);
  border: 2rpx solid rgba(37,99,235,.10);
}
.wechat-slot__title { display: block; color: #1e40af; font-size: 25rpx; font-weight: 900; margin-bottom: 8rpx; }
.wechat-slot__desc { display: block; color: #475569; font-size: 24rpx; line-height: 1.6; }
.wechat-slot__phone { margin-top: 16rpx; min-height: 88rpx; font-size: 25rpx; }
.profile-form__row {
  display: flex;
  align-items: center;
  gap: 20rpx;
}
.avatar-picker {
  width: 120rpx;
  height: 120rpx;
  padding: 0;
  border-radius: 38rpx;
  background: rgba(37,99,235,.10);
  border: 2rpx solid rgba(37,99,235,.16);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.avatar-picker::after { border: none; }
.avatar-picker__img { width: 120rpx; height: 120rpx; display: block; }
.avatar-picker__ph { color: #2563eb; font-size: 24rpx; font-weight: 900; }
.nickname-field {
  flex: 1;
  min-width: 0;
  min-height: 120rpx;
  border-radius: 26rpx;
  background: rgba(255,255,255,.82);
  border: 2rpx solid rgba(15,23,42,.08);
  padding: 18rpx 22rpx;
  box-sizing: border-box;
}
.nickname-field__label {
  color: #64748b;
  font-size: 22rpx;
  font-weight: 900;
  display: block;
}
.nickname-field__input {
  width: 100%;
  min-height: 52rpx;
  color: #0f172a;
  font-size: 30rpx;
  font-weight: 800;
  margin-top: 6rpx;
}
.profile-form__save { margin-top: 4rpx; }
.sec-title {
  margin-bottom: 16rpx;
}
.empty {
  color: #64748b;
  font-size: 25rpx;
  padding: 18rpx 0;
  text-align: center;
  border-radius: 22rpx;
  background: rgba(15,23,42,.035);
}
.empty--error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  padding: 18rpx 20rpx;
  text-align: left;
}
.sync-retry {
  flex-shrink: 0;
  min-width: 112rpx;
  min-height: 88rpx;
  padding: 0 20rpx;
  border-radius: 999rpx;
  background: rgba(37,99,235,.10);
  color: #2563eb;
  font-size: 24rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}
.sync-retry::after { border: none; }
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 18rpx;
  min-height: 82rpx;
  padding: 18rpx 0;
  border-bottom: 2rpx solid rgba(15,23,42,.07);
}
.row:last-child { border-bottom: none; }
.row__main {
  color: #0f172a;
  font-size: 28rpx;
  font-weight: 900;
  line-height: 1.35;
}
.row__time {
  color: #64748b;
  font-size: 22rpx;
  text-align: right;
  line-height: 1.35;
}
.more-tip {
  display: block;
  margin-top: 14rpx;
  color: #64748b;
  font-size: 23rpx;
}
</style>
