<script setup>
import { computed, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { TYPES_INFO } from '../../data/enneagramGame'
import { ensureLogin, getToken, clearToken } from '../../utils/auth'
import { hiddenCount, previewItems } from '../../utils/listPreview'
import { openChatPage } from '../../utils/navigation'
import { normalizeWechatProfile, hasProfilePayload, getWechatProfilePayload } from '../../utils/wechatProfile'
import { getUserInfoApi, updateUserInfoApi, listTestRecordsApi, listBookingsApi } from '../../api'

const logged = ref(false)
const user = ref(null)
const records = ref([])
const bookings = ref([])
const logging = ref(false)
const profileSaving = ref(false)
const nicknameDraft = ref('')
const avatarDraft = ref('')
const visibleRecords = computed(() => previewItems(records.value))
const visibleBookings = computed(() => previewItems(bookings.value))
const hiddenRecordCount = computed(() => hiddenCount(records.value))
const hiddenBookingCount = computed(() => hiddenCount(bookings.value))

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
  } catch {
    uni.showToast({ title: '登录失败', icon: 'none' })
  } finally {
    logging.value = false
  }
}

async function loadAll() {
  try {
    user.value = await getUserInfoApi()
    syncDraftFromUser()
  } catch (e) {
    resetLogin()
    uni.showToast({ title: '登录已过期，请重新登录', icon: 'none' })
    return
  }

  const [rec, bk] = await Promise.all([
    listTestRecordsApi().catch(() => ({ items: [] })),
    listBookingsApi().catch(() => ({ items: [] })),
  ])

  records.value = rec.items || []
  bookings.value = bk.items || []
}

function typeName(id) {
  return TYPES_INFO[id] ? `${id} 号 · ${TYPES_INFO[id].name}` : '—'
}

function syncDraftFromUser() {
  nicknameDraft.value = (user.value && user.value.nickname) || ''
  avatarDraft.value = (user.value && user.value.avatar) || ''
}

function resetLogin() {
  clearToken()
  logged.value = false
  user.value = null
  records.value = []
  bookings.value = []
  nicknameDraft.value = ''
  avatarDraft.value = ''
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
  <view class="wrap profile page-stack">
    <!-- 未登录 -->
    <view v-if="!logged" class="card login">
      <view class="login__mark">九</view>
      <text class="eyebrow">个人档案</text>
      <text class="login__t">登录后可保存你的九型档案、测试历史和预约记录。</text>
      <button class="btn-primary" :loading="logging" :disabled="logging" @click="login">微信一键登录</button>
    </view>

    <!-- 已登录 -->
    <template v-else>
      <view class="card user">
        <image v-if="user && user.avatar" class="user__avatar" :src="user.avatar" />
        <view v-else class="user__avatar user__avatar--ph">{{ (user && user.mainType) || '九' }}</view>
        <view class="user__info">
          <text class="user__name">{{ (user && user.nickname) || '九型用户' }}</text>
          <text class="user__type" v-if="user && user.mainType">主型：{{ typeName(user.mainType) }}</text>
          <text class="user__type" v-else>已通过微信登录</text>
        </view>
        <button class="user__chat" @click="goChat">问 AI</button>
      </view>

      <view class="card profile-form">
        <view class="profile-form__head">
          <text class="sec-title">微信资料</text>
          <button class="mini-link" :loading="profileSaving" @click="syncWechatProfile">一键同步</button>
        </view>
        <view class="profile-form__row">
          <button class="avatar-picker" open-type="chooseAvatar" @chooseavatar="onChooseAvatar">
            <image v-if="avatarDraft" class="avatar-picker__img" :src="avatarDraft" mode="aspectFill" />
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

      <view class="card section-card">
        <text class="sec-title">我的测试历史</text>
        <view v-if="records.length === 0" class="empty">还没有记录，去测一测吧</view>
        <view v-for="rec in visibleRecords" :key="rec.id" class="row">
          <text class="row__main">{{ typeName(rec.resultType) }}</text>
          <text class="row__time">{{ rec.createTime }}</text>
        </view>
        <text v-if="hiddenRecordCount" class="more-tip">还有 {{ hiddenRecordCount }} 条记录已收起</text>
      </view>

      <view class="card section-card">
        <text class="sec-title">我的预约</text>
        <view v-if="bookings.length === 0" class="empty">暂无预约</view>
        <view v-for="b in visibleBookings" :key="b.id" class="row">
          <text class="row__main">{{ b.intent || b.kind }}</text>
          <text class="row__time">{{ b.status }} · {{ b.createTime }}</text>
        </view>
        <text v-if="hiddenBookingCount" class="more-tip">还有 {{ hiddenBookingCount }} 条预约已收起</text>
      </view>

      <button class="btn-ghost" @click="logout">退出登录</button>
    </template>
  </view>
</template>

<style scoped>
.login { display: flex; flex-direction: column; align-items: center; text-align: center; gap: 20rpx; padding: 58rpx 34rpx; }
.login__mark {
  width: 112rpx;
  height: 112rpx;
  border-radius: 36rpx;
  background: linear-gradient(135deg,#5aa0ff,#2b7fff);
  color: #fff;
  font-size: 48rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 18rpx 42rpx -24rpx rgba(43,127,255,.72);
}
.login .eyebrow { align-self: center; }
.login__t { color: #3c424d; font-size: 28rpx; line-height: 1.7; }
.user { display: flex; align-items: center; gap: 24rpx; }
.section-card {
  box-shadow: 0 12rpx 34rpx -28rpx rgba(28,40,70,.36);
}
.user__info { flex: 1; min-width: 0; }
.user__avatar {
  width: 112rpx;
  height: 112rpx;
  border-radius: 50%;
  border: 4rpx solid rgba(255,255,255,.92);
  box-sizing: border-box;
}
.user__avatar--ph {
  background: linear-gradient(135deg,#5aa0ff,#2b7fff);
  color: #fff;
  font-size: 44rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}
.user__name { color: #12151b; font-size: 34rpx; font-weight: 900; display: block; }
.user__type { color: #3c424d; font-size: 25rpx; display: block; margin-top: 6rpx; }
.user__chat {
  width: 118rpx;
  height: 64rpx;
  padding: 0;
  border-radius: 999rpx;
  background: rgba(37,179,101,.12);
  color: #16a06a;
  font-size: 24rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.user__chat::after { border: none; }
.profile-form { display: flex; flex-direction: column; gap: 22rpx; }
.profile-form__head { display: flex; align-items: center; justify-content: space-between; gap: 18rpx; }
.mini-link {
  min-width: 142rpx;
  height: 58rpx;
  padding: 0 20rpx;
  border-radius: 999rpx;
  background: rgba(43,127,255,.1);
  color: #2b7fff;
  font-size: 23rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}
.mini-link::after { border: none; }
.profile-form__row { display: flex; align-items: center; gap: 22rpx; }
.avatar-picker {
  width: 118rpx;
  height: 118rpx;
  padding: 0;
  border-radius: 36rpx;
  background: rgba(43,127,255,.1);
  border: 2rpx solid rgba(43,127,255,.16);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.avatar-picker::after { border: none; }
.avatar-picker__img { width: 118rpx; height: 118rpx; display: block; }
.avatar-picker__ph { color: #2b7fff; font-size: 24rpx; font-weight: 900; }
.nickname-field {
  flex: 1;
  min-width: 0;
  min-height: 118rpx;
  border-radius: 24rpx;
  background: rgba(255,255,255,.72);
  border: 2rpx solid rgba(20,24,32,.08);
  padding: 18rpx 22rpx;
  box-sizing: border-box;
}
.nickname-field__label { color: #767d89; font-size: 22rpx; font-weight: 800; display: block; }
.nickname-field__input {
  width: 100%;
  min-height: 48rpx;
  color: #12151b;
  font-size: 30rpx;
  font-weight: 800;
  margin-top: 6rpx;
}
.profile-form__save { margin-top: 4rpx; }
.sec-title { color: #12151b; font-size: 31rpx; font-weight: 900; display: block; margin-bottom: 16rpx; }
.empty { color: #767d89; font-size: 25rpx; padding: 12rpx 0; }
.row { display: flex; justify-content: space-between; align-items: center; gap: 18rpx; padding: 20rpx 0; border-bottom: 2rpx solid rgba(20,24,32,.07); }
.row:last-child { border-bottom: none; }
.row__main { color: #12151b; font-size: 28rpx; font-weight: 800; }
.row__time { color: #767d89; font-size: 22rpx; text-align: right; }
.more-tip { display: block; margin-top: 14rpx; color: #767d89; font-size: 23rpx; }
</style>
