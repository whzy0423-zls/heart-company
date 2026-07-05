<script setup>
import { ref } from 'vue'
import { QUESTIONS } from '../../data/enneagramGame'

const total = ref(QUESTIONS.length)

function startTest() {
  uni.navigateTo({ url: '/pages/test/test' })
}
function goLearn() {
  uni.switchTab({ url: '/pages/learn/learn' })
}
function goRelation() {
  uni.navigateTo({ url: '/pages/relation/relation' })
}
</script>

<template>
  <view class="wrap home page-stack ios-page ios-safe-bottom">
    <view class="home__header">
      <text class="eyebrow">九型芯之力</text>
      <text class="home__headline">跟着老师学懂九型人格</text>
      <text class="home__sub">从老师资料、入门课件到系统课程，先建立九型地图，再把类型理解落到关系与成长练习里。</text>
    </view>

    <view class="hero card ios-card">
      <view class="hero__copy">
        <text class="hero__kicker">老师导学 · 课件配套 · {{ total }} 题自测</text>
        <text class="hero__title gradient-title">从课程开始认识自己</text>
        <text class="hero__lead">先看老师整理的课件与课程路径，再结合九型自测理解动机、恐惧、欲望与三中心。</text>
        <view class="hero__actions">
          <button class="btn-primary ios-button hero__btn" @click="startTest">开始自测</button>
          <button class="btn-ghost ios-button hero__ghost" @click="goLearn">看老师课件</button>
        </view>
      </view>
      <view class="hero__visual">
        <view class="hero__halo"></view>
        <image class="hero__wheel" src="/static/wheel.png" mode="aspectFit" lazy-load />
      </view>
    </view>

    <view class="insight card ios-card">
      <view class="insight__item">
        <text class="insight__num">9</text>
        <text class="insight__label">人格类型</text>
      </view>
      <view class="insight__line"></view>
      <view class="insight__item">
        <text class="insight__num">3</text>
        <text class="insight__label">三中心分布</text>
      </view>
      <view class="insight__line"></view>
      <view class="insight__item">
        <text class="insight__num">课件</text>
        <text class="insight__label">老师导学</text>
      </view>
    </view>

    <view class="section-head ios-section">
      <text class="section-title">接下来想做什么？</text>
      <text class="section-lead">优先从老师资料和课程课件进入，再按需要完成自测与关系练习。</text>
    </view>

    <view class="grid">
      <view
        class="grid__item card ios-card grid__item--wide"
        role="button"
        aria-label="开始九型测试"
        aria-pressed="false"
        hover-class="grid__item--hover"
        @click="startTest"
      >
        <view class="grid__top"><text class="chip">01</text><text class="grid__pill">推荐</text></view>
        <text class="grid__t">九型测试</text>
        <text class="grid__d">测出主型、副型、三中心与成长方向</text>
      </view>
      <view
        class="grid__item card ios-card"
        role="button"
        aria-label="打开九型学习"
        aria-pressed="false"
        hover-class="grid__item--hover"
        @click="goLearn"
      >
        <text class="chip chip--orange">02</text>
        <text class="grid__t">老师课件</text>
        <text class="grid__d">老师资料、课件课程与阶段化练习</text>
      </view>
      <view
        class="grid__item card ios-card grid__item--wide grid__item--relation"
        role="button"
        aria-label="打开关系合盘"
        aria-pressed="false"
        hover-class="grid__item--hover"
        @click="goRelation"
      >
        <view class="grid__top"><text class="chip chip--red">03</text><text class="grid__pill grid__pill--soft">关系模式</text></view>
        <text class="grid__t">关系合盘</text>
        <text class="grid__d">看你和 TA 的沟通节奏、冲突触发点与相处底色</text>
      </view>
    </view>
  </view>
</template>

<style scoped>
.home {
  gap: 26rpx;
}
.home__header {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  padding: 8rpx 4rpx 0;
}
.home__headline {
  max-width: 650rpx;
  color: #0f172a;
  font-size: 48rpx;
  font-weight: 900;
  line-height: 1.16;
  letter-spacing: -.6rpx;
}
.home__sub {
  color: #475569;
  font-size: 27rpx;
  line-height: 1.68;
}
.hero {
  min-height: 650rpx;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 28rpx;
  padding: 42rpx 34rpx 30rpx;
  background:
    linear-gradient(145deg, rgba(255,255,255,.92), rgba(255,255,255,.66)),
    radial-gradient(circle at 80% 20%, rgba(90,160,255,.26), transparent 42%);
}
.hero__copy {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  gap: 17rpx;
}
.hero__kicker {
  color: #2563eb;
  font-size: 24rpx;
  font-weight: 900;
}
.hero__title {
  max-width: 590rpx;
  font-size: 60rpx;
}
.hero__lead {
  max-width: 600rpx;
  color: #334155;
  font-size: 29rpx;
  line-height: 1.68;
}
.hero__actions {
  display: flex;
  gap: 18rpx;
  margin-top: 12rpx;
}
.hero__btn,
.hero__ghost {
  flex: 1;
}
.hero__visual {
  position: relative;
  align-self: center;
  width: 520rpx;
  height: 300rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}
.hero__halo {
  position: absolute;
  width: 450rpx;
  height: 220rpx;
  border-radius: 999rpx;
  background: linear-gradient(120deg, rgba(37,99,235,.18), rgba(249,115,22,.14));
  filter: blur(18rpx);
}
.hero__wheel {
  position: relative;
  z-index: 1;
  width: 100%;
  height: 100%;
  filter: drop-shadow(0 28rpx 38rpx rgba(15,23,42,.18));
}
.insight {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 26rpx 24rpx;
}
.insight__item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4rpx;
}
.insight__num {
  color: #0f172a;
  font-size: 36rpx;
  font-weight: 900;
  line-height: 1;
}
.insight__label {
  color: #64748b;
  font-size: 22rpx;
  font-weight: 800;
}
.insight__line {
  width: 2rpx;
  height: 54rpx;
  background: rgba(15,23,42,.08);
}
.section-head {
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  padding: 2rpx 4rpx;
}
.grid {
  display: flex;
  flex-wrap: wrap;
  gap: 18rpx;
}
.grid__item {
  width: calc((100% - 18rpx) / 2);
  min-height: 236rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12rpx;
  padding: 28rpx;
  transition: opacity .18s ease, transform .18s ease;
}
.grid__item--hover {
  opacity: .86;
  transform: scale(.985);
}
.grid__item--wide {
  width: 100%;
  min-height: 210rpx;
}
.grid__item--relation {
  background: linear-gradient(145deg, rgba(255,255,255,.90), rgba(255,247,237,.70));
}
.grid__top {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12rpx;
}
.grid__pill {
  min-height: 44rpx;
  padding: 0 18rpx;
  border-radius: 999rpx;
  background: rgba(249,115,22,.12);
  color: #ea580c;
  font-size: 22rpx;
  font-weight: 900;
  display: inline-flex;
  align-items: center;
}
.grid__pill--soft {
  background: rgba(226,58,71,.10);
  color: #e23a47;
}
.chip--red {
  background: linear-gradient(135deg, #fb7185, #e11d48);
  box-shadow: 0 14rpx 34rpx -18rpx rgba(225,29,72,.72);
}
.chip--green {
  background: linear-gradient(135deg, #34d399, #059669);
  box-shadow: 0 14rpx 34rpx -18rpx rgba(5,150,105,.72);
}
.chip--orange {
  background: linear-gradient(135deg, #fb923c, #f97316);
  box-shadow: 0 14rpx 34rpx -18rpx rgba(249,115,22,.72);
}
.grid__t {
  color: #0f172a;
  font-weight: 900;
  font-size: 32rpx;
  line-height: 1.25;
}
.grid__d {
  color: #64748b;
  font-size: 24rpx;
  line-height: 1.5;
}
</style>
