<script setup>
import { ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { showTimelineShareHint, isTimelinePreview } from '../utils/share'
defineProps({ disabled: { type: Boolean, default: false } })
const timelinePreview = ref(isTimelinePreview())
onShow(() => { timelinePreview.value = isTimelinePreview() })
</script>

<template>
  <!-- #ifdef MP-WEIXIN -->
  <view v-if="timelinePreview" class="nx-timeline-note">点击「前往小程序」，可继续浏览、报名和分享。</view>
  <view v-else class="nx-share-actions">
    <button class="nx-share-action" open-type="share" :disabled="disabled" hover-class="nx-share-action--pressed">分享给好友</button>
    <button class="nx-share-action" :disabled="disabled" hover-class="nx-share-action--pressed" @click="showTimelineShareHint()">分享到朋友圈</button>
  </view>
  <!-- #endif -->
</template>

<style scoped>
.nx-share-actions{display:flex;flex-wrap:wrap;gap:16rpx;padding:22rpx 0;box-sizing:border-box}.nx-share-action{flex:1;min-width:180rpx;min-height:76rpx;margin:0;padding:16rpx 18rpx;border:1rpx solid #DCCEBF;border-radius:12rpx;background:#F7F3EB;color:#915538;font-size:23rpx;line-height:1.8;font-weight:500;box-sizing:border-box}.nx-share-action::after{border:0}.nx-share-action[disabled]{color:#948C80;background:#EFEBE4;border-color:#E5DDD2}.nx-share-action--pressed{opacity:.72}
.nx-timeline-note{margin:22rpx 0;padding:20rpx 24rpx;border-radius:12rpx;background:#EFE9DE;color:#756B5D;font-size:23rpx;line-height:1.8}
</style>
