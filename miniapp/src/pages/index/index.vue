<script setup>
import { ref } from 'vue'
import { QUESTIONS } from '../../data/enneagramGame'

const total = ref(QUESTIONS.length)
const wheelVisible = ref(true)

function startTest() {
  uni.navigateTo({ url: '/pages/test/test' })
}
function goLearn() {
  uni.switchTab({ url: '/pages/learn/learn' })
}
function goRelation() {
  uni.navigateTo({ url: '/pages/relation/relation' })
}
function goProfile() {
  uni.switchTab({ url: '/pages/profile/profile' })
}
function hideWheel() {
  wheelVisible.value = false
}
</script>

<template>
  <view class="wrap home page-stack ios-page ios-safe-bottom">
    <view class="home-nav">
      <view class="home-nav__copy">
        <text class="home-nav__brand">九型芯之力</text>
        <text class="home-nav__tagline">看见动机，找到成长方向</text>
      </view>
      <view
        class="home-nav__profile"
        role="button"
        aria-label="打开我的成长档案"
        hover-class="home-nav__profile--pressed"
        @click="goProfile"
      >
        <view class="profile-icon" aria-hidden="true">
          <view class="profile-icon__head"></view>
          <view class="profile-icon__body"></view>
        </view>
      </view>
    </view>

    <view class="hero card ios-card">
      <view class="hero__orb hero__orb--blue"></view>
      <view class="hero__orb hero__orb--orange"></view>
      <view class="hero__copy">
        <text class="hero__kicker">老师导学 · 课程配套 · {{ total }} 题自测</text>
        <text class="hero__title">读懂自己内在的能量地图</text>
        <text class="hero__lead">从核心动机出发，在老师课程中理解自己，也更从容地走进关系与成长。</text>
        <button
          class="hero__cta ios-button"
          hover-class="hero__cta--pressed"
          @click="startTest"
        >开始人格测试</button>
      </view>
      <view class="hero__visual">
        <image
          v-if="wheelVisible"
          class="hero__wheel"
          src="/static/wheel.png"
          mode="aspectFit"
          lazy-load
          @error="hideWheel"
        />
        <view v-else class="hero__wheel-fallback" aria-hidden="true">9</view>
      </view>
    </view>

    <view class="section-head ios-section">
      <text class="section-title">探索你的九型能量</text>
      <text class="section-lead">从测试、关系、课程到成长档案，选择此刻最需要的一步。</text>
    </view>

    <view class="energy-grid">
      <view
        class="energy-card energy-card--test"
        role="button"
        aria-label="开始九型人格测试"
        hover-class="energy-card--pressed"
        @click="startTest"
      >
        <view class="energy-icon energy-icon--test" aria-hidden="true">
          <view class="energy-icon__ring"></view>
          <view class="energy-icon__dot"></view>
        </view>
        <text class="energy-card__title">人格测试</text>
        <text class="energy-card__desc">找到你的核心动机</text>
      </view>

      <view
        class="energy-card energy-card--relation"
        role="button"
        aria-label="打开九型关系合盘"
        hover-class="energy-card--pressed"
        @click="goRelation"
      >
        <view class="energy-icon energy-icon--relation" aria-hidden="true">
          <view class="energy-icon__person energy-icon__person--left"></view>
          <view class="energy-icon__person energy-icon__person--right"></view>
          <view class="energy-icon__link"></view>
        </view>
        <text class="energy-card__title">关系合盘</text>
        <text class="energy-card__desc">看见彼此的互动模式</text>
      </view>

      <view
        class="energy-card energy-card--learn"
        role="button"
        aria-label="打开老师课程与课件"
        hover-class="energy-card--pressed"
        @click="goLearn"
      >
        <view class="energy-icon energy-icon--learn" aria-hidden="true">
          <view class="energy-icon__book energy-icon__book--left"></view>
          <view class="energy-icon__book energy-icon__book--right"></view>
          <view class="energy-icon__spine"></view>
        </view>
        <text class="energy-card__title">老师课程</text>
        <text class="energy-card__desc">沿着课件系统学习</text>
      </view>

      <view
        class="energy-card energy-card--profile"
        role="button"
        aria-label="打开我的成长档案"
        hover-class="energy-card--pressed"
        @click="goProfile"
      >
        <view class="energy-icon energy-icon--profile" aria-hidden="true">
          <view class="energy-icon__stem"></view>
          <view class="energy-icon__leaf energy-icon__leaf--left"></view>
          <view class="energy-icon__leaf energy-icon__leaf--right"></view>
        </view>
        <text class="energy-card__title">成长档案</text>
        <text class="energy-card__desc">记录每一次自我看见</text>
      </view>
    </view>

    <view
      class="growth-card"
      role="button"
      aria-label="打开老师课程与成长内容"
      hover-class="growth-card--pressed"
      @click="goLearn"
    >
      <view class="growth-card__copy">
        <text class="growth-card__eyebrow">老师陪伴 · 持续成长</text>
        <text class="growth-card__title">把测试发现带进课程练习</text>
        <text class="growth-card__desc">跟随老师的课程与课件，让理解沉淀为真实的成长行动。</text>
      </view>
      <view class="growth-card__arrow" aria-hidden="true"></view>
    </view>
  </view>
</template>

<style scoped>
.home {
  gap: 28rpx;
}
.home-nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24rpx;
  padding: 6rpx 2rpx 0;
}
.home-nav__copy {
  display: flex;
  flex-direction: column;
  gap: 6rpx;
}
.home-nav__brand {
  color: #172033;
  font-size: 36rpx;
  font-weight: 900;
  letter-spacing: 1rpx;
}
.home-nav__tagline {
  color: #778197;
  font-size: 23rpx;
  font-weight: 600;
}
.home-nav__profile {
  min-width: 88rpx;
  min-height: 88rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1rpx solid rgba(53, 73, 115, .09);
  border-radius: 50%;
  background: rgba(255, 255, 255, .88);
  box-shadow: 0 14rpx 32rpx rgba(30, 47, 81, .09);
  transition: opacity .18s ease, transform .18s ease;
}
.home-nav__profile--pressed {
  opacity: .72;
  transform: scale(.94);
}
.profile-icon {
  position: relative;
  width: 42rpx;
  height: 44rpx;
}
.profile-icon__head {
  position: absolute;
  top: 0;
  left: 50%;
  width: 18rpx;
  height: 18rpx;
  border: 4rpx solid #52627f;
  border-radius: 50%;
  transform: translateX(-50%);
}
.profile-icon__body {
  position: absolute;
  bottom: 0;
  left: 50%;
  width: 38rpx;
  height: 21rpx;
  border: 4rpx solid #52627f;
  border-bottom: 0;
  border-radius: 24rpx 24rpx 0 0;
  transform: translateX(-50%);
}
.hero {
  position: relative;
  min-height: 610rpx;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 24rpx;
  padding: 42rpx 36rpx 24rpx;
  background: linear-gradient(145deg, rgba(255,255,255,.98), rgba(244,248,255,.9));
}
.hero__orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(2rpx);
  pointer-events: none;
}
.hero__orb--blue {
  top: -90rpx;
  right: -72rpx;
  width: 260rpx;
  height: 260rpx;
  background: rgba(74, 133, 255, .14);
}
.hero__orb--orange {
  right: 150rpx;
  bottom: 70rpx;
  width: 116rpx;
  height: 116rpx;
  background: rgba(255, 153, 92, .16);
}
.hero__copy {
  position: relative;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16rpx;
}
.hero__kicker {
  color: #4f6fb3;
  font-size: 23rpx;
  font-weight: 800;
}
.hero__title {
  max-width: 610rpx;
  color: #172033;
  font-size: 52rpx;
  font-weight: 900;
  line-height: 1.18;
  letter-spacing: -.8rpx;
}
.hero__lead {
  max-width: 610rpx;
  color: #5c687e;
  font-size: 27rpx;
  line-height: 1.64;
}
.hero__cta {
  min-height: 88rpx;
  margin: 8rpx 0 0;
  padding: 0 36rpx;
  border: 0;
  border-radius: 999rpx;
  background: linear-gradient(135deg, #496fd0, #6e55c9);
  color: #fff;
  font-size: 27rpx;
  font-weight: 800;
  line-height: 88rpx;
  box-shadow: 0 20rpx 34rpx -18rpx rgba(73, 87, 190, .72);
  transition: opacity .18s ease, transform .18s ease;
}
.hero__cta--pressed {
  opacity: .84;
  transform: scale(.98);
}
.hero__visual {
  position: relative;
  z-index: 1;
  align-self: center;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: hero-float 5s ease-in-out infinite;
}
.hero__wheel {
  width: 300rpx;
  height: 300rpx;
  filter: drop-shadow(0 26rpx 32rpx rgba(31, 51, 91, .18));
}
.hero__wheel-fallback {
  width: 300rpx;
  height: 300rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 20rpx double rgba(79, 111, 179, .35);
  border-radius: 50%;
  color: #4f6fb3;
  font-size: 116rpx;
  font-weight: 300;
  box-shadow: inset 0 0 0 4rpx rgba(255, 255, 255, .9), 0 26rpx 32rpx rgba(31, 51, 91, .12);
}
.section-head {
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  padding: 4rpx 2rpx 0;
}
.section-title {
  color: #172033;
  font-size: 34rpx;
  font-weight: 900;
}
.section-lead {
  color: #7b8496;
  font-size: 24rpx;
  line-height: 1.5;
}
.energy-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18rpx;
}
.energy-card {
  min-height: 218rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8rpx;
  padding: 26rpx;
  border: 1rpx solid rgba(50, 65, 96, .07);
  border-radius: 30rpx;
  box-shadow: 0 16rpx 34rpx rgba(34, 49, 78, .07);
  transition: opacity .18s ease, transform .18s ease;
}
.energy-card--pressed {
  opacity: .8;
  transform: scale(.97);
}
.energy-card--test {
  background: linear-gradient(145deg, #f0f4ff, #fff);
}
.energy-card--relation {
  background: linear-gradient(145deg, #fff2f0, #fff);
}
.energy-card--learn {
  background: linear-gradient(145deg, #fff8e9, #fff);
}
.energy-card--profile {
  background: linear-gradient(145deg, #eff9f5, #fff);
}
.energy-card__title {
  margin-top: auto;
  color: #202a3d;
  font-size: 29rpx;
  font-weight: 900;
}
.energy-card__desc {
  color: #737e91;
  font-size: 22rpx;
  line-height: 1.45;
}
.energy-icon {
  position: relative;
  width: 62rpx;
  height: 62rpx;
}
.energy-icon--test {
  border-radius: 18rpx;
  background: rgba(83, 112, 202, .11);
}
.energy-icon__ring {
  position: absolute;
  inset: 13rpx;
  border: 4rpx solid #5a73be;
  border-radius: 50%;
}
.energy-icon__dot {
  position: absolute;
  top: 27rpx;
  left: 27rpx;
  width: 8rpx;
  height: 8rpx;
  border-radius: 50%;
  background: #5a73be;
}
.energy-icon--relation {
  border-radius: 18rpx;
  background: rgba(222, 103, 95, .1);
}
.energy-icon__person {
  position: absolute;
  top: 13rpx;
  width: 17rpx;
  height: 17rpx;
  border: 4rpx solid #d76f68;
  border-radius: 50%;
}
.energy-icon__person::after {
  position: absolute;
  top: 18rpx;
  left: -6rpx;
  width: 21rpx;
  height: 14rpx;
  border: 4rpx solid #d76f68;
  border-bottom: 0;
  border-radius: 14rpx 14rpx 0 0;
  content: '';
}
.energy-icon__person--left {
  left: 9rpx;
}
.energy-icon__person--right {
  right: 9rpx;
}
.energy-icon__link {
  position: absolute;
  right: 23rpx;
  bottom: 11rpx;
  width: 16rpx;
  height: 4rpx;
  border-radius: 4rpx;
  background: #d76f68;
}
.energy-icon--learn {
  border-radius: 18rpx;
  background: rgba(222, 164, 64, .12);
}
.energy-icon__book {
  position: absolute;
  top: 15rpx;
  width: 24rpx;
  height: 32rpx;
  border: 4rpx solid #c89438;
}
.energy-icon__book--left {
  left: 8rpx;
  border-radius: 8rpx 2rpx 2rpx 8rpx;
  transform: skewY(7deg);
}
.energy-icon__book--right {
  right: 8rpx;
  border-radius: 2rpx 8rpx 8rpx 2rpx;
  transform: skewY(-7deg);
}
.energy-icon__spine {
  position: absolute;
  top: 17rpx;
  left: 29rpx;
  width: 4rpx;
  height: 31rpx;
  background: #c89438;
}
.energy-icon--profile {
  border-radius: 18rpx;
  background: rgba(73, 156, 119, .11);
}
.energy-icon__stem {
  position: absolute;
  left: 29rpx;
  bottom: 10rpx;
  width: 4rpx;
  height: 34rpx;
  border-radius: 4rpx;
  background: #4a9b77;
}
.energy-icon__leaf {
  position: absolute;
  width: 20rpx;
  height: 13rpx;
  border: 4rpx solid #4a9b77;
  border-radius: 16rpx 2rpx 16rpx 2rpx;
}
.energy-icon__leaf--left {
  top: 19rpx;
  left: 10rpx;
  transform: rotate(18deg);
}
.energy-icon__leaf--right {
  top: 9rpx;
  right: 9rpx;
  transform: rotate(-18deg) scaleX(-1);
}
.growth-card {
  min-height: 208rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 28rpx;
  padding: 32rpx;
  border: 1rpx solid rgba(72, 67, 128, .08);
  border-radius: 32rpx;
  background: linear-gradient(125deg, #2f3655, #535080);
  box-shadow: 0 24rpx 46rpx -22rpx rgba(39, 43, 83, .65);
  transition: opacity .18s ease, transform .18s ease;
}
.growth-card--pressed {
  opacity: .82;
  transform: scale(.985);
}
.growth-card__copy {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 9rpx;
}
.growth-card__eyebrow {
  color: #cbd3ff;
  font-size: 22rpx;
  font-weight: 800;
}
.growth-card__title {
  color: #fff;
  font-size: 31rpx;
  font-weight: 900;
}
.growth-card__desc {
  color: rgba(255, 255, 255, .72);
  font-size: 23rpx;
  line-height: 1.5;
}
.growth-card__arrow {
  width: 25rpx;
  height: 25rpx;
  border-top: 4rpx solid rgba(255, 255, 255, .82);
  border-right: 4rpx solid rgba(255, 255, 255, .82);
  transform: rotate(45deg);
}
@keyframes hero-float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-10rpx); }
}
@media (prefers-reduced-motion: reduce) {
  .hero__visual {
    animation: none;
    transition: none;
  }
}
</style>
