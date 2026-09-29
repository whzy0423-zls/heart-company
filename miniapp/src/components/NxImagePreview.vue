<script setup>
defineProps({
  visible: { type: Boolean, default: false },
  src: { type: String, default: '' },
  alt: { type: String, default: '图片预览' },
})

const emit = defineEmits(['close'])

function close() {
  emit('close')
}
</script>

<template>
  <view v-if="visible && src" class="nx-image-preview" role="dialog" aria-label="图片预览" @click="close">
    <view class="nx-image-preview__bar" @click.stop>
      <text class="nx-image-preview__title">图片预览</text>
      <button class="nx-image-preview__close" aria-label="关闭图片预览" @click="close">关闭</button>
    </view>
    <image class="nx-image-preview__image" :src="src" mode="aspectFit" :aria-label="alt" @click.stop />
    <text class="nx-image-preview__tip">点击任意位置关闭</text>
  </view>
</template>

<style scoped>
.nx-image-preview{position:fixed;z-index:999;top:0;right:0;bottom:0;left:0;display:flex;align-items:center;justify-content:center;background:rgba(17,20,23,.96);padding:calc(112rpx + env(safe-area-inset-top)) 28rpx calc(72rpx + env(safe-area-inset-bottom));box-sizing:border-box}
.nx-image-preview__bar{position:absolute;top:env(safe-area-inset-top);left:0;right:0;height:112rpx;padding:0 30rpx;display:flex;align-items:center;justify-content:space-between;box-sizing:border-box}
.nx-image-preview__title{color:rgba(255,255,255,.88);font-size:25rpx;letter-spacing:2rpx}
.nx-image-preview__close{margin:0;padding:0 18rpx;min-width:100rpx;height:62rpx;border:1rpx solid rgba(255,255,255,.42);border-radius:31rpx;background:rgba(255,255,255,.08);color:#fff;font-size:23rpx;line-height:62rpx}
.nx-image-preview__image{display:block;width:100%;height:100%;max-width:100%;max-height:100%}
.nx-image-preview__tip{position:absolute;bottom:calc(25rpx + env(safe-area-inset-bottom));left:0;right:0;text-align:center;color:rgba(255,255,255,.6);font-size:19rpx}
</style>
