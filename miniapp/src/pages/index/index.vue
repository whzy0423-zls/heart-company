<script setup>
import { ref } from 'vue'
import { QUESTIONS } from '../../data/enneagramGame'
import { openChatPage } from '../../utils/navigation'

const total = ref(QUESTIONS.length)

function startTest() {
  uni.navigateTo({ url: '/pages/test/test' })
}
function goLearn() {
  uni.switchTab({ url: '/pages/learn/learn' })
}
function goChat() {
  openChatPage()
}
function goRelation() {
  uni.navigateTo({ url: '/pages/relation/relation' })
}
</script>

<template>
  <view class="wrap home page-stack">
    <view class="hero card">
      <view class="hero__copy">
        <text class="eyebrow">趣味体验</text>
        <text class="hero__title gradient-title">你的人设出厂设置</text>
        <text class="hero__lead">用 {{ total }} 道情境题，测出你天生自带的「性格芯片」。</text>
        <view class="hero__actions">
          <button class="btn-primary hero__btn" @click="startTest">开始测试</button>
          <button class="btn-ghost hero__ghost" @click="goLearn">先学一学</button>
        </view>
        <view class="hero__meta">
          <text>约 2 分钟</text>
          <text>共 {{ total }} 题</text>
          <text>凭直觉选择</text>
        </view>
      </view>
      <view class="hero__visual">
        <image class="hero__wheel" src="/static/wheel.png" mode="aspectFit" />
      </view>
    </view>

    <view class="grid">
      <view class="grid__item card" @click="startTest">
        <text class="chip">01</text>
        <text class="grid__t">九型测试</text>
        <text class="grid__d">测出你的主型与副型</text>
      </view>
      <view class="grid__item card" @click="goLearn">
        <text class="chip chip--red">02</text>
        <text class="grid__t">九型学习</text>
        <text class="grid__d">三阶段 · 课程 · 语录</text>
      </view>
      <view class="grid__item card" @click="goChat">
        <text class="chip chip--green">03</text>
        <text class="grid__t">AI 对话</text>
        <text class="grid__d">基于资料检索回答</text>
      </view>
      <view class="grid__item card" @click="goRelation">
        <text class="chip chip--red">04</text>
        <text class="grid__t">关系合盘</text>
        <text class="grid__d">看你和 TA 的相处底色</text>
      </view>
    </view>
  </view>
</template>

<style scoped>
.hero {
  min-height: 660rpx;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 34rpx;
  padding: 44rpx 34rpx 36rpx;
}
.hero__copy {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  gap: 18rpx;
}
.hero__title {
  max-width: 520rpx;
  font-size: 60rpx;
}
.hero__lead {
  max-width: 560rpx;
  color: #3c424d;
  font-size: 29rpx;
  line-height: 1.68;
}
.hero__actions {
  display: flex;
  gap: 18rpx;
  margin-top: 10rpx;
}
.hero__btn,
.hero__ghost {
  flex: 1;
}
.hero__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12rpx;
  margin-top: 2rpx;
}
.hero__meta text {
  padding: 8rpx 16rpx;
  border-radius: 999rpx;
  background: rgba(255,255,255,.66);
  color: #767d89;
  font-size: 22rpx;
  font-weight: 700;
}
.hero__visual {
  position: relative;
  align-self: center;
  width: 510rpx;
  height: 330rpx;
}
.hero__wheel {
  position: relative;
  z-index: 1;
  width: 100%;
  height: 100%;
  filter: drop-shadow(0 28rpx 36rpx rgba(28,40,70,.18));
}
.grid {
  display: flex;
  flex-wrap: wrap;
  gap: 20rpx;
}
.grid__item {
  width: calc((100% - 20rpx) / 2);
  min-height: 230rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10rpx;
  padding: 28rpx;
}
.chip--red {
  background: linear-gradient(135deg, #ff5a6a, #e23a47);
  box-shadow: 0 14rpx 34rpx -18rpx rgba(226,58,71,.72);
}
.chip--green {
  background: linear-gradient(135deg, #25b365, #16a06a);
  box-shadow: 0 14rpx 34rpx -18rpx rgba(37,179,101,.72);
}
.grid__t {
  color: #12151b;
  font-weight: 900;
  font-size: 31rpx;
}
.grid__d {
  color: #767d89;
  font-size: 23rpx;
  line-height: 1.45;
}
</style>
