<script setup>
import { ref } from 'vue'
import { onShow, onHide, onUnload } from '@dcloudio/uni-app'
import { getToken, clearToken } from '../../utils/auth'
import { normalizeWechatProfile, hasProfilePayload } from '../../utils/wechatProfile'
// #ifndef H5
import { getWechatProfilePayload } from '../../utils/wechatProfile'
// #endif
import { userErrorMessage } from '../../utils/userMessage'
import { getUserInfoApi, updateUserInfoApi } from '../../api'

const user = ref(null)
const nicknameDraft = ref('')
const avatarDraft = ref('')
const avatarFailed = ref(false)
const loadError = ref('')
const profileLoading = ref(false)
const profileSyncing = ref(false)
const profileSaving = ref(false)

let pageActive = false
let authRedirected = false
let sessionGeneration = 0

function resetProfileState() {
  user.value = null
  nicknameDraft.value = ''
  avatarDraft.value = ''
  avatarFailed.value = false
  loadError.value = ''
  profileLoading.value = false
  profileSyncing.value = false
  profileSaving.value = false
}

function syncDraftFromUser() {
  nicknameDraft.value = (user.value && user.value.nickname) || ''
  avatarDraft.value = (user.value && user.value.avatar) || ''
  avatarFailed.value = false
}

function isCurrentGeneration(generation) {
  return pageActive && generation === sessionGeneration
}

function isCurrentProfileSession(generation, token) {
  return pageActive && generation === sessionGeneration && token === getToken()
}

function isAuthFailure(error) {
  return Boolean(error && (error.authExpired || error.authRequired || error.statusCode === 401 || error.statusCode === 403))
}

function invalidateProfileSession() {
  pageActive = false
  sessionGeneration += 1
  resetProfileState()
}

function redirectToProfileLogin() {
  if (authRedirected) return
  authRedirected = true
  pageActive = false
  sessionGeneration += 1
  clearToken()
  resetProfileState()
  uni.showToast({ title: '登录已过期，请重新登录', icon: 'none' })
  uni.switchTab({ url: '/pages/profile/profile' })
}

async function loadProfile() {
  if (profileLoading.value) return
  const token = getToken()
  if (!token) {
    redirectToProfileLogin()
    return
  }

  const generation = sessionGeneration
  profileLoading.value = true
  loadError.value = ''
  try {
    const loadedUser = await getUserInfoApi()
    if (!isCurrentProfileSession(generation, token)) return
    user.value = loadedUser
    syncDraftFromUser()
  } catch (e) {
    if (!isCurrentGeneration(generation)) return
    if (isAuthFailure(e)) {
      redirectToProfileLogin()
      return
    }
    if (!isCurrentProfileSession(generation, token)) return
    loadError.value = userErrorMessage(e, '资料加载失败，请重试')
  } finally {
    if (isCurrentGeneration(generation)) profileLoading.value = false
  }
}

function onNicknameInput(e) {
  nicknameDraft.value = e.detail && e.detail.value ? e.detail.value : ''
}

// #ifndef H5
function onChooseAvatar(e) {
  avatarDraft.value = e.detail && e.detail.avatarUrl ? e.detail.avatarUrl : ''
  avatarFailed.value = false
}

async function syncWechatProfile() {
  if (profileSyncing.value) return
  const token = getToken()
  if (!token) {
    redirectToProfileLogin()
    return
  }

  const generation = sessionGeneration
  profileSyncing.value = true
  try {
    const payload = await getWechatProfilePayload()
    if (!isCurrentProfileSession(generation, token)) return
    if (!hasProfilePayload(payload)) {
      uni.showToast({ title: '请用下方头像昵称补充资料', icon: 'none' })
      return
    }
    const updatedUser = await updateUserInfoApi(payload)
    if (!isCurrentProfileSession(generation, token)) return
    user.value = updatedUser
    syncDraftFromUser()
    uni.showToast({ title: '资料已同步', icon: 'success' })
  } catch (e) {
    if (!isCurrentGeneration(generation)) return
    if (isAuthFailure(e)) {
      redirectToProfileLogin()
      return
    }
    if (!isCurrentProfileSession(generation, token)) return
    uni.showToast({ title: '可手动补充头像昵称', icon: 'none' })
  } finally {
    if (isCurrentGeneration(generation)) profileSyncing.value = false
  }
}
// #endif

async function saveProfile() {
  if (profileSaving.value) return
  const token = getToken()
  if (!token) {
    redirectToProfileLogin()
    return
  }

  const payload = normalizeWechatProfile({
    nickname: nicknameDraft.value,
    avatar: avatarDraft.value,
  })
  if (!hasProfilePayload(payload)) {
    uni.showToast({ title: '请先填写昵称或选择头像', icon: 'none' })
    return
  }

  const generation = sessionGeneration
  profileSaving.value = true
  try {
    const updatedUser = await updateUserInfoApi(payload)
    if (!isCurrentProfileSession(generation, token)) return
    user.value = updatedUser
    syncDraftFromUser()
    uni.showToast({ title: '资料已保存', icon: 'success' })
  } catch (e) {
    if (!isCurrentGeneration(generation)) return
    if (isAuthFailure(e)) {
      redirectToProfileLogin()
      return
    }
    if (!isCurrentProfileSession(generation, token)) return
    uni.showToast({ title: userErrorMessage(e, '保存失败，请重试'), icon: 'none' })
  } finally {
    if (isCurrentGeneration(generation)) profileSaving.value = false
  }
}

function onAvatarError() {
  avatarFailed.value = true
}

onShow(() => {
  pageActive = true
  authRedirected = false
  sessionGeneration += 1
  resetProfileState()
  loadProfile()
})

onHide(invalidateProfileSession)
onUnload(invalidateProfileSession)
</script>

<template>
  <view class="profile-edit-page page-stack ios-page ios-safe-bottom">
    <view class="profile-edit-hero nx-page-hero">
      <text class="profile-edit-hero__eyebrow">个人档案</text>
      <text class="profile-edit-hero__title">让资料更像现在的你</text>
      <text class="profile-edit-hero__lead">更新头像和昵称后，“我的”页面会在下次打开时同步展示。</text>
      <view class="profile-edit-hero__identity">
        <image
          v-if="avatarDraft && !avatarFailed"
          class="profile-edit-hero__avatar"
          :src="avatarDraft"
          mode="aspectFill"
          lazy-load
          @error="onAvatarError"
        />
        <view v-else class="profile-edit-hero__avatar profile-edit-hero__avatar--fallback">{{ nicknameDraft.slice(0, 1) || '九' }}</view>
        <view class="profile-edit-hero__copy">
          <text class="profile-edit-hero__name">{{ nicknameDraft || '九型用户' }}</text>
          <text class="profile-edit-hero__status">{{ profileLoading ? '正在同步资料…' : '头像与昵称可随时更新' }}</text>
        </view>
      </view>
    </view>

    <view v-if="profileLoading" class="profile-state nx-state ios-card" aria-live="polite">
      <text class="profile-state__title">正在加载个人资料</text>
      <text class="profile-state__desc">请稍候，正在同步你保存的信息。</text>
    </view>

    <view v-else-if="loadError" class="profile-state profile-state--error nx-state ios-card" aria-live="polite">
      <text class="profile-state__title">资料暂时没有加载成功</text>
      <text class="profile-state__desc">{{ loadError }}</text>
      <button class="profile-retry ios-button" @click="loadProfile">重新加载</button>
    </view>

    <view v-else class="profile-edit-panel nx-panel ios-card">
      <view class="profile-edit-panel__head">
        <view>
          <text class="profile-edit-panel__kicker">微信资料</text>
          <text class="profile-edit-panel__title">头像与昵称</text>
        </view>
        <!-- #ifndef H5 -->
        <button class="profile-sync" :loading="profileSyncing" :disabled="profileSyncing" @click="syncWechatProfile">一键同步</button>
        <!-- #endif -->
        <!-- #ifdef H5 -->
        <button class="profile-sync profile-sync--disabled" disabled>微信资料需在小程序同步</button>
        <!-- #endif -->
      </view>

      <view class="wechat-note">
        <!-- #ifndef H5 -->
        <text class="wechat-note__title">微信授权能力</text>
        <text class="wechat-note__desc">可以一键读取授权资料，也可以在下方分别选择头像、填写昵称。</text>
        <!-- #endif -->
        <!-- #ifdef H5 -->
        <text class="wechat-note__title">请在微信小程序内同步微信资料</text>
        <text class="wechat-note__desc">H5 已登录时仍可修改昵称并保存；头像选择和微信同步请打开小程序。</text>
        <!-- #endif -->
      </view>

      <view class="profile-fields">
        <!-- #ifndef H5 -->
        <button class="avatar-picker" open-type="chooseAvatar" @chooseavatar="onChooseAvatar">
          <image v-if="avatarDraft && !avatarFailed" class="avatar-picker__image" :src="avatarDraft" mode="aspectFill" lazy-load @error="onAvatarError" />
          <text v-else class="avatar-picker__fallback">选择头像</text>
        </button>
        <!-- #endif -->
        <!-- #ifdef H5 -->
        <button class="avatar-picker avatar-picker--disabled" disabled>
          <image v-if="avatarDraft && !avatarFailed" class="avatar-picker__image" :src="avatarDraft" mode="aspectFill" lazy-load @error="onAvatarError" />
          <text v-else class="avatar-picker__fallback">微信内选择</text>
        </button>
        <!-- #endif -->

        <view class="nickname-field">
          <text class="nickname-field__label">昵称</text>
          <!-- #ifndef H5 -->
          <input class="nickname-field__input" type="nickname" :value="nicknameDraft" placeholder="填写微信昵称" @input="onNicknameInput" @blur="onNicknameInput" />
          <!-- #endif -->
          <!-- #ifdef H5 -->
          <input class="nickname-field__input" type="text" :value="nicknameDraft" placeholder="填写昵称" @input="onNicknameInput" @blur="onNicknameInput" />
          <!-- #endif -->
        </view>
      </view>

      <button class="profile-save btn-primary ios-button" :loading="profileSaving" :disabled="profileSaving" @click="saveProfile">保存资料</button>
      <text class="profile-edit-panel__hint">保存成功后会留在当前页面，你可以确认资料后再返回。</text>
    </view>
  </view>
</template>

<style scoped>
.profile-edit-page { gap: 24rpx; overflow-x: hidden; }
.profile-edit-hero { box-sizing: border-box; width: 100%; padding: 38rpx 34rpx; border-radius: 38rpx; background: linear-gradient(145deg, #172554, #4338ca 56%, #7c3aed); color: #ffffff; box-shadow: 0 26rpx 54rpx -34rpx rgba(49, 46, 129, .78); overflow: hidden; }
.profile-edit-hero__eyebrow { display: block; color: #ddd6fe; font-size: 24rpx; font-weight: 800; line-height: 1.4; }
.profile-edit-hero__title { display: block; margin-top: 10rpx; color: #ffffff; font-size: 42rpx; font-weight: 900; line-height: 1.25; }
.profile-edit-hero__lead { display: block; margin-top: 14rpx; color: #ede9fe; font-size: 25rpx; line-height: 1.65; }
.profile-edit-hero__identity { display: flex; align-items: center; gap: 22rpx; margin-top: 30rpx; padding: 22rpx; border-radius: 28rpx; background: rgba(255, 255, 255, .13); border: 2rpx solid rgba(255, 255, 255, .18); }
.profile-edit-hero__avatar { box-sizing: border-box; width: 104rpx; height: 104rpx; flex: 0 0 104rpx; border-radius: 34rpx; border: 3rpx solid rgba(255, 255, 255, .68); }
.profile-edit-hero__avatar--fallback { display: flex; align-items: center; justify-content: center; background: rgba(255, 255, 255, .16); color: #ffffff; font-size: 42rpx; font-weight: 900; }
.profile-edit-hero__copy { flex: 1; min-width: 0; }
.profile-edit-hero__name { display: block; color: #ffffff; font-size: 34rpx; font-weight: 900; line-height: 1.3; }
.profile-edit-hero__status { display: block; margin-top: 7rpx; color: #ddd6fe; font-size: 24rpx; line-height: 1.45; }
.profile-state { box-sizing: border-box; width: 100%; padding: 34rpx 28rpx; text-align: center; }
.profile-state--error { border: 2rpx solid #fecaca; background: #fff7f7; }
.profile-state__title { display: block; color: #0f172a; font-size: 30rpx; font-weight: 900; line-height: 1.4; }
.profile-state__desc { display: block; margin-top: 10rpx; color: #475569; font-size: 24rpx; line-height: 1.6; }
.profile-retry { width: 100%; min-height: 88rpx; margin-top: 22rpx; border-radius: 24rpx; background: #eef2ff; color: #3730a3; font-size: 26rpx; font-weight: 900; }
.profile-retry::after { border: none; }
.profile-edit-panel { box-sizing: border-box; width: 100%; padding: 30rpx; }
.profile-edit-panel__head { display: flex; align-items: center; justify-content: space-between; gap: 18rpx; }
.profile-edit-panel__kicker { display: block; color: #4f46e5; font-size: 24rpx; font-weight: 800; line-height: 1.35; }
.profile-edit-panel__title { display: block; margin-top: 6rpx; color: #0f172a; font-size: 32rpx; font-weight: 900; line-height: 1.3; }
.profile-sync { min-width: 146rpx; min-height: 88rpx; padding: 0 20rpx; border-radius: 22rpx; background: #eef2ff; color: #3730a3; font-size: 24rpx; font-weight: 900; display: flex; align-items: center; justify-content: center; }
.profile-sync::after { border: none; }
.profile-sync--disabled { max-width: 236rpx; color: #64748b; background: #f1f5f9; }
.wechat-note { margin-top: 22rpx; padding: 22rpx; border-radius: 22rpx; background: #f5f3ff; border: 2rpx solid #ede9fe; }
.wechat-note__title { display: block; color: #3730a3; font-size: 25rpx; font-weight: 900; line-height: 1.4; }
.wechat-note__desc { display: block; margin-top: 8rpx; color: #475569; font-size: 24rpx; line-height: 1.6; }
.profile-fields { display: flex; align-items: center; gap: 20rpx; margin-top: 24rpx; }
.avatar-picker { width: 120rpx; height: 120rpx; flex: 0 0 120rpx; padding: 0; border-radius: 38rpx; background: #eef2ff; border: 2rpx solid #c7d2fe; overflow: hidden; display: flex; align-items: center; justify-content: center; }
.avatar-picker::after { border: none; }
.avatar-picker--disabled { border-color: #e2e8f0; background: #f8fafc; }
.avatar-picker__image { width: 120rpx; height: 120rpx; display: block; }
.avatar-picker__fallback { padding: 0 12rpx; color: #3730a3; font-size: 24rpx; font-weight: 900; line-height: 1.3; }
.nickname-field { flex: 1; min-width: 0; min-height: 120rpx; box-sizing: border-box; padding: 18rpx 22rpx; border-radius: 26rpx; border: 2rpx solid #e2e8f0; background: #ffffff; }
.nickname-field__label { display: block; color: #475569; font-size: 24rpx; font-weight: 900; }
.nickname-field__input { width: 100%; min-height: 52rpx; margin-top: 6rpx; color: #0f172a; font-size: 30rpx; font-weight: 800; }
.profile-save { width: 100%; min-height: 88rpx; margin-top: 26rpx; border-radius: 24rpx; }
.profile-edit-panel__hint { display: block; margin-top: 14rpx; color: #475569; font-size: 24rpx; line-height: 1.55; text-align: center; }
@media (max-width: 360px) {
  .profile-edit-hero, .profile-edit-panel { padding-left: 26rpx; padding-right: 26rpx; }
  .profile-edit-panel__head { align-items: flex-start; }
  .profile-sync { min-width: 132rpx; }
}
</style>
