<script setup>
import NxIcon from './NxIcon.vue'
import { UI_PREVIEW } from '../utils/uiPreview'
import { useStudioNavigation } from '../utils/studioNavigation'

const { headerStyle, topbarStyle } = useStudioNavigation()
</script>

<template>
  <view class="studio-header" :style="headerStyle">
    <view class="studio-topbar" :style="topbarStyle">
      <view class="brand">
        <image class="brand-logo" src="/static/brand/logo.png" mode="aspectFit" aria-label="九型芯之力品牌 Logo" />
        <view class="brand-copy">
          <text class="brand-name">九型芯之力</text>
          <text class="brand-sub">看见自己 · 理解彼此</text>
        </view>
      </view>
      <!-- #ifndef MP-WEIXIN -->
      <text v-if="UI_PREVIEW" class="preview-label">体验版</text>
      <button v-else class="top-about" aria-label="了解老师"><NxIcon name="user" :size="21" color="#282A27" /></button>
      <!-- #endif -->
    </view>
  </view>
</template>

<style scoped>
.studio-header{position:fixed;top:0;left:0;right:0;z-index:30;box-sizing:border-box;padding-top:env(safe-area-inset-top,0px);background:#F7F5F0;}
.studio-topbar{box-sizing:border-box;width:100%;height:144rpx;max-width:900rpx;padding:24rpx 36rpx;display:flex;align-items:center;justify-content:space-between;margin:0 auto;}
.brand{min-width:0;display:flex;align-items:center;gap:16rpx}.brand-copy{min-width:0}.brand-name,.brand-sub{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.brand-logo{display:block;width:66rpx;height:66rpx;flex:none}.brand-name{display:block;font-size:31rpx;font-weight:650;letter-spacing:2rpx}.brand-sub{display:block;font-size:19rpx;color:#77786F;letter-spacing:3rpx;margin-top:3rpx}.preview-label{font-size:20rpx;color:#8B6A52;border:1rpx solid #E5D7C8;border-radius:30rpx;padding:6rpx 16rpx}.top-about{margin:0;width:88rpx;height:88rpx;display:flex;align-items:center;justify-content:center;background:transparent;padding:0}
button::after{border:0}
</style>
