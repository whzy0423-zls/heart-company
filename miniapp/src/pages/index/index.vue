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
        tabindex="0"
        aria-label="打开我的成长档案"
        hover-class="home-nav__profile--pressed"
        @click="goProfile"
        @keydown.enter="goProfile"
        @keydown.space.prevent="goProfile"
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
          aria-hidden="true"
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
        tabindex="0"
        aria-label="开始九型人格测试"
        hover-class="energy-card--pressed"
        @click="startTest"
        @keydown.enter="startTest"
        @keydown.space.prevent="startTest"
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
        tabindex="0"
        aria-label="打开九型关系合盘"
        hover-class="energy-card--pressed"
        @click="goRelation"
        @keydown.enter="goRelation"
        @keydown.space.prevent="goRelation"
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
        tabindex="0"
        aria-label="打开老师课程与课件"
        hover-class="energy-card--pressed"
        @click="goLearn"
        @keydown.enter="goLearn"
        @keydown.space.prevent="goLearn"
      >
        <view class="energy-icon energy-icon--learn" aria-hidden="true">
          <view class="energy-icon__book energy-icon__book--left"></view>
          <view class="energy-icon__book energy-icon__book--right"></view>
          <view class="energy-icon__spine"></view>
        </view>
        <text class="energy-card__title">老师课程</text>
        <text class="energy-card__desc">跟着课件系统学习</text>
      </view>

      <view
        class="energy-card energy-card--profile"
        role="button"
        tabindex="0"
        aria-label="打开我的成长档案"
        hover-class="energy-card--pressed"
        @click="goProfile"
        @keydown.enter="goProfile"
        @keydown.space.prevent="goProfile"
      >
        <view class="energy-icon energy-icon--profile" aria-hidden="true">
          <view class="energy-icon__stem"></view>
          <view class="energy-icon__leaf energy-icon__leaf--left"></view>
          <view class="energy-icon__leaf energy-icon__leaf--right"></view>
        </view>
        <text class="energy-card__title">成长档案</text>
        <text class="energy-card__desc">记录你的探索轨迹</text>
      </view>
    </view>

    <view
      class="growth-card"
      role="button"
      tabindex="0"
      aria-label="打开老师课程与成长内容"
      hover-class="growth-card--pressed"
      @click="goLearn"
      @keydown.enter="goLearn"
      @keydown.space.prevent="goLearn"
    >
      <view class="growth-card__visual" aria-hidden="true">
        <view class="growth-card__sun"></view>
        <view class="growth-card__path"></view>
        <view class="growth-card__spark growth-card__spark--one"></view>
        <view class="growth-card__spark growth-card__spark--two"></view>
      </view>
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
  position: relative;
  gap: 28rpx;
  background:
    radial-gradient(circle at 8% 12%, rgba(75, 153, 255, .12), transparent 26%),
    radial-gradient(circle at 94% 28%, rgba(145, 92, 255, .11), transparent 24%);
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
  min-height: 540rpx;
  overflow: hidden;
  display: flex;
  align-items: center;
  gap: 24rpx;
  padding: 46rpx 38rpx;
  border-color: rgba(255, 255, 255, .18);
  border-radius: 38rpx;
  background: linear-gradient(138deg, #125fce 0%, #3c45cf 48%, #7229ad 100%);
  box-shadow: 0 34rpx 74rpx -34rpx rgba(45, 55, 177, .72);
}
.hero__orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(2rpx);
  pointer-events: none;
}
.hero__orb--blue {
  top: -78rpx;
  right: -54rpx;
  width: 230rpx;
  height: 230rpx;
  background: rgba(78, 225, 255, .24);
}
.hero__orb--orange {
  right: 210rpx;
  bottom: -54rpx;
  width: 156rpx;
  height: 156rpx;
  background: rgba(255, 100, 190, .2);
}
.hero__copy {
  position: relative;
  z-index: 2;
  width: 53%;
  max-width: 430rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16rpx;
}
.hero__kicker {
  color: rgba(255, 255, 255, .78);
  font-size: 23rpx;
  font-weight: 800;
}
.hero__title {
  color: #fff;
  font-size: 50rpx;
  font-weight: 900;
  line-height: 1.18;
  letter-spacing: -.8rpx;
}
.hero__lead {
  color: rgba(255, 255, 255, .82);
  font-size: 25rpx;
  line-height: 1.58;
}
.hero__cta {
  min-height: 88rpx;
  margin: 8rpx 0 0;
  padding: 0 36rpx;
  border: 0;
  border-radius: 999rpx;
  background: #fff;
  color: #344fc0;
  font-size: 27rpx;
  font-weight: 800;
  line-height: 88rpx;
  box-shadow: 0 20rpx 38rpx -18rpx rgba(15, 24, 96, .58);
  transition: opacity .2s ease, transform .2s ease;
}
.hero__cta--pressed {
  opacity: .84;
  transform: scale(.98);
}
.hero__visual {
  position: absolute;
  right: 24rpx;
  bottom: 28rpx;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: hero-float 5s ease-in-out infinite;
}
.hero__wheel {
  width: 300rpx;
  height: 300rpx;
  filter: drop-shadow(0 28rpx 34rpx rgba(15, 18, 80, .3));
}
.hero__wheel-fallback {
  box-sizing: border-box;
  width: 300rpx;
  height: 300rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 20rpx double rgba(255, 255, 255, .52);
  border-radius: 50%;
  color: rgba(255, 255, 255, .9);
  font-size: 116rpx;
  font-weight: 300;
  box-shadow: inset 0 0 0 4rpx rgba(255, 255, 255, .28), 0 26rpx 32rpx rgba(19, 17, 82, .22);
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
  min-height: 204rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8rpx;
  padding: 28rpx;
  border: 1rpx solid rgba(255, 255, 255, .2);
  border-radius: 30rpx;
  box-shadow: 0 20rpx 40rpx -24rpx rgba(25, 39, 92, .62);
  transition: opacity .2s ease, transform .2s ease;
}
.energy-card--pressed {
  opacity: .8;
  transform: scale(.97);
}
.energy-card--test {
  background: linear-gradient(140deg, #2468e8, #16bde2);
}
.energy-card--relation {
  background: linear-gradient(140deg, #7047df, #e94ea5);
}
.energy-card--learn {
  background: linear-gradient(140deg, #0c9f9b, #32bd70);
}
.energy-card--profile {
  background: linear-gradient(140deg, #f39a2e, #ed5f63);
}
.energy-card__title {
  margin-top: auto;
  color: #fff;
  font-size: 29rpx;
  font-weight: 900;
}
.energy-card__desc {
  color: rgba(255, 255, 255, .8);
  font-size: 22rpx;
  line-height: 1.45;
}
.energy-icon {
  position: relative;
  width: 72rpx;
  height: 72rpx;
  border-radius: 20rpx;
  background: rgba(255, 255, 255, .18);
  box-shadow: inset 0 1rpx 0 rgba(255, 255, 255, .22);
}
.energy-icon--test {
  background: rgba(255, 255, 255, .18);
}
.energy-icon__ring {
  position: absolute;
  inset: 17rpx;
  border: 4rpx solid #fff;
  border-radius: 50%;
}
.energy-icon__dot {
  position: absolute;
  top: 32rpx;
  left: 32rpx;
  width: 8rpx;
  height: 8rpx;
  border-radius: 50%;
  background: #fff;
}
.energy-icon--relation {
  background: rgba(255, 255, 255, .18);
}
.energy-icon__person {
  position: absolute;
  top: 13rpx;
  width: 17rpx;
  height: 17rpx;
  border: 4rpx solid #fff;
  border-radius: 50%;
}
.energy-icon__person::after {
  position: absolute;
  top: 18rpx;
  left: -6rpx;
  width: 21rpx;
  height: 14rpx;
  border: 4rpx solid #fff;
  border-bottom: 0;
  border-radius: 14rpx 14rpx 0 0;
  content: '';
}
.energy-icon__person--left {
  left: 13rpx;
}
.energy-icon__person--right {
  right: 13rpx;
}
.energy-icon__link {
  position: absolute;
  right: 28rpx;
  bottom: 15rpx;
  width: 16rpx;
  height: 4rpx;
  border-radius: 4rpx;
  background: #fff;
}
.energy-icon--learn {
  background: rgba(255, 255, 255, .18);
}
.energy-icon__book {
  position: absolute;
  top: 19rpx;
  width: 24rpx;
  height: 32rpx;
  border: 4rpx solid #fff;
}
.energy-icon__book--left {
  left: 12rpx;
  border-radius: 8rpx 2rpx 2rpx 8rpx;
  transform: skewY(7deg);
}
.energy-icon__book--right {
  right: 12rpx;
  border-radius: 2rpx 8rpx 8rpx 2rpx;
  transform: skewY(-7deg);
}
.energy-icon__spine {
  position: absolute;
  top: 21rpx;
  left: 34rpx;
  width: 4rpx;
  height: 31rpx;
  background: #fff;
}
.energy-icon--profile {
  background: rgba(255, 255, 255, .18);
}
.energy-icon__stem {
  position: absolute;
  left: 34rpx;
  bottom: 12rpx;
  width: 4rpx;
  height: 34rpx;
  border-radius: 4rpx;
  background: #fff;
}
.energy-icon__leaf {
  position: absolute;
  width: 20rpx;
  height: 13rpx;
  border: 4rpx solid #fff;
  border-radius: 16rpx 2rpx 16rpx 2rpx;
}
.energy-icon__leaf--left {
  top: 19rpx;
  left: 14rpx;
  transform: rotate(18deg);
}
.energy-icon__leaf--right {
  top: 9rpx;
  right: 13rpx;
  transform: rotate(-18deg) scaleX(-1);
}
.growth-card {
  min-height: 208rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 28rpx;
  padding: 30rpx;
  border: 1rpx solid rgba(72, 67, 128, .08);
  border-radius: 32rpx;
  background: linear-gradient(125deg, rgba(255, 255, 255, .92), rgba(246, 242, 255, .84));
  box-shadow: 0 24rpx 48rpx -30rpx rgba(56, 53, 113, .35);
  backdrop-filter: blur(18rpx);
  transition: opacity .2s ease, transform .2s ease;
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
.growth-card__visual {
  position: relative;
  flex: 0 0 112rpx;
  width: 112rpx;
  height: 112rpx;
  overflow: hidden;
  border-radius: 30rpx;
  background: linear-gradient(145deg, #4778ee, #8853d9 58%, #ed6f9f);
  box-shadow: 0 18rpx 32rpx -18rpx rgba(91, 70, 197, .62);
}
.growth-card__sun {
  position: absolute;
  top: 20rpx;
  right: 18rpx;
  width: 25rpx;
  height: 25rpx;
  border-radius: 50%;
  background: #ffd46b;
  box-shadow: 0 0 0 8rpx rgba(255, 212, 107, .16);
}
.growth-card__path {
  position: absolute;
  left: 19rpx;
  bottom: -18rpx;
  width: 76rpx;
  height: 88rpx;
  border: 8rpx solid rgba(255, 255, 255, .88);
  border-top-color: transparent;
  border-radius: 50%;
  transform: rotate(-18deg);
}
.growth-card__spark {
  position: absolute;
  width: 8rpx;
  height: 8rpx;
  border-radius: 50%;
  background: rgba(255, 255, 255, .9);
}
.growth-card__spark--one {
  top: 28rpx;
  left: 22rpx;
}
.growth-card__spark--two {
  top: 49rpx;
  left: 37rpx;
  width: 5rpx;
  height: 5rpx;
}
.growth-card__eyebrow {
  color: #665bc0;
  font-size: 22rpx;
  font-weight: 800;
}
.growth-card__title {
  color: #20263a;
  font-size: 31rpx;
  font-weight: 900;
}
.growth-card__desc {
  color: #6f778b;
  font-size: 23rpx;
  line-height: 1.5;
}
.growth-card__arrow {
  width: 25rpx;
  height: 25rpx;
  border-top: 4rpx solid #7771a4;
  border-right: 4rpx solid #7771a4;
  transform: rotate(45deg);
}
@keyframes hero-float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-10rpx); }
}
@media (min-width: 768px) {
  .hero {
    padding: 56rpx 48rpx;
  }
  .energy-card {
    padding: 34rpx;
  }
}
@media (max-width: 360px) {
  .hero {
    min-height: 680rpx;
    align-items: flex-start;
  }
  .hero__copy {
    width: 100%;
    max-width: 100%;
  }
  .hero__title {
    max-width: 430rpx;
    font-size: 44rpx;
  }
  .hero__lead {
    max-width: 410rpx;
  }
  .hero__visual {
    right: -40rpx;
    bottom: 8rpx;
  }
  .growth-card {
    gap: 20rpx;
  }
}
@media (prefers-reduced-motion: reduce) {
  .hero__visual {
    animation: none;
    transition: none;
  }
  .energy-card,
  .growth-card,
  .home-nav__profile,
  .hero__cta {
    transition: none;
  }
}
</style>
