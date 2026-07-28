<script setup>
import { computed, onMounted, ref } from 'vue'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import { filterFailedCarouselItems, normalizeHomeCarousel } from '../../utils/homeCarousel'
import { MINIAPP_HOME_ENTRY_BEHAVIORS, normalizeMiniappHome } from '../../utils/homeMenu'

const wheelVisible = ref(true)
const carousel = ref(normalizeHomeCarousel())
const miniappHome = ref(normalizeMiniappHome())
const enabledHomeEntries = computed(() => miniappHome.value.entriesSection.items.filter((item) => item.enabled))
const carouselPaused = ref(false)
const failedCarouselImages = new Set()

function applyHomeConfig(config) {
  carousel.value = filterFailedCarouselItems(normalizeHomeCarousel(config), failedCarouselImages)
  miniappHome.value = normalizeMiniappHome(config)
  if (carousel.value.items.length <= 1) carouselPaused.value = false
}

function removeCarouselItem(image) {
  failedCarouselImages.add(image)
  carousel.value = filterFailedCarouselItems(carousel.value, failedCarouselImages)
}

function toggleCarouselPaused() {
  carouselPaused.value = !carouselPaused.value
}

onMounted(() => {
  const cached = getStoredSiteConfig()
  if (cached) applyHomeConfig(cached)

  refreshSiteConfig()
    .then(applyHomeConfig)
    .catch(() => {
      // 网络刷新失败时保留已经展示的首页内容。
    })
})

function activateHomeEntry(key) {
  const behavior = MINIAPP_HOME_ENTRY_BEHAVIORS[key]
  if (!behavior) return
  uni[behavior.method]({ url: behavior.url })
}
function startTest() {
  activateHomeEntry('test')
}
function goLearn() {
  activateHomeEntry('learn')
}
function goRelation() {
  activateHomeEntry('relation')
}
function goProfile() {
  activateHomeEntry('profile')
}
function goClassroom() {
  uni.navigateTo({ url: '/pages/classroom/classroom?tab=standalone' })
}
function hideWheel() {
  wheelVisible.value = false
}
</script>

<template>
  <view class="wrap home page-stack ios-page ios-safe-bottom">
    <swiper
      v-if="carousel.items.length"
      class="home-carousel"
      :autoplay="carousel.items.length > 1 && carousel.autoplay && !carouselPaused"
      :interval="carousel.interval"
      :duration="450"
      :circular="carousel.items.length > 1"
      :indicator-dots="carousel.items.length > 1"
    >
      <swiper-item v-for="(item, index) in carousel.items" :key="item.image">
        <image
          class="home-carousel__image"
          :src="item.image"
          mode="aspectFill"
          lazy-load
          :aria-label="'首页轮播图 第' + (index + 1) + '张'"
          @error="removeCarouselItem(item.image)"
        />
      </swiper-item>
    </swiper>
    <button
      v-if="carousel.items.length > 1 && carousel.autoplay"
      class="home-carousel__toggle"
      hover-class="home-carousel__toggle--pressed"
      :aria-label="carouselPaused ? '继续轮播图自动播放' : '暂停轮播图自动播放'"
      @click="toggleCarouselPaused"
    >{{ carouselPaused ? '继续轮播' : '暂停轮播' }}</button>

    <view v-if="miniappHome.brand.enabled" class="home-nav">
      <view class="home-nav__copy">
        <text class="home-nav__brand">{{ miniappHome.brand.name }}</text>
        <text class="home-nav__tagline">{{ miniappHome.brand.tagline }}</text>
      </view>
      <view
        class="home-nav__profile"
        role="button"
        aria-role="button"
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

    <view v-if="miniappHome.hero.enabled" class="hero card ios-card">
      <view class="hero__orb hero__orb--blue"></view>
      <view class="hero__orb hero__orb--orange"></view>
      <view class="hero__copy">
        <text class="hero__kicker">{{ miniappHome.hero.kicker }}</text>
        <text class="hero__title">{{ miniappHome.hero.title }}</text>
        <text class="hero__lead">{{ miniappHome.hero.description }}</text>
        <button
          class="hero__cta ios-button"
          hover-class="hero__cta--pressed"
          @click="startTest"
        >{{ miniappHome.hero.buttonText }}</button>
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

    <view
      class="classroom-spotlight ios-card"
      role="button"
      aria-role="button"
      tabindex="0"
      aria-label="进入老师课堂，查看视频和音频课件"
      hover-class="classroom-spotlight--pressed"
      @click="goClassroom"
      @keydown.enter="goClassroom"
      @keydown.space.prevent="goClassroom"
    >
      <view class="classroom-spotlight__copy">
        <text class="classroom-spotlight__eyebrow">老师课堂 · 视频 / 音频课件</text>
        <text class="classroom-spotlight__title">老师以往开课内容，沉淀成可反复学习的课件</text>
        <text class="classroom-spotlight__desc">按系列循序学，也可以从一节独立课件开始。</text>
        <button class="classroom-spotlight__cta ios-button" hover-class="classroom-spotlight__cta--pressed" @click.stop="goClassroom">进入老师课堂</button>
      </view>
      <view class="classroom-spotlight__media" aria-hidden="true">
        <view class="classroom-spotlight__screen">
          <view class="classroom-spotlight__ring"></view>
          <view class="classroom-spotlight__pulse classroom-spotlight__pulse--one"></view>
          <view class="classroom-spotlight__pulse classroom-spotlight__pulse--two"></view>
          <view class="classroom-spotlight__play"></view>
        </view>
      </view>
    </view>

    <view v-if="miniappHome.entriesSection.enabled" class="section-head ios-section">
      <text class="section-title">{{ miniappHome.entriesSection.title }}</text>
      <text class="section-lead">{{ miniappHome.entriesSection.description }}</text>
    </view>

    <view v-if="miniappHome.entriesSection.enabled" class="energy-grid">
      <view
        v-for="entry in enabledHomeEntries"
        :key="entry.key"
        :class="['energy-card', `energy-card--${entry.theme}`]"
        role="button"
        aria-role="button"
        tabindex="0"
        :aria-label="MINIAPP_HOME_ENTRY_BEHAVIORS[entry.key].ariaLabel"
        hover-class="energy-card--pressed"
        @click="activateHomeEntry(entry.key)"
        @keydown.enter="activateHomeEntry(entry.key)"
        @keydown.space.prevent="activateHomeEntry(entry.key)"
      >
        <view :class="['energy-icon', `energy-icon--${entry.icon}`]" aria-hidden="true">
          <view class="energy-icon__shape"></view>
        </view>
        <text class="energy-card__title">{{ entry.title }}</text>
        <text class="energy-card__desc">{{ entry.description }}</text>
      </view>
    </view>

    <view
      v-if="miniappHome.growth.enabled"
      class="growth-card"
      role="button"
      aria-role="button"
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
        <text class="growth-card__eyebrow">{{ miniappHome.growth.eyebrow }}</text>
        <text class="growth-card__title">{{ miniappHome.growth.title }}</text>
        <text class="growth-card__desc">{{ miniappHome.growth.description }}</text>
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
.home-carousel {
  width: 100%;
  height: 300rpx;
  overflow: hidden;
  border-radius: 32rpx;
  box-shadow: 0 24rpx 48rpx rgba(47, 69, 122, .16);
}
.home-carousel__image {
  display: block;
  width: 100%;
  height: 100%;
}
.home-carousel__toggle {
  align-self: flex-end;
  min-height: 88rpx;
  margin-top: -16rpx;
  padding: 0 24rpx;
  border: 1rpx solid rgba(53, 73, 115, .12);
  border-radius: 999rpx;
  background: rgba(255, 255, 255, .92);
  color: #475569;
  font-size: 22rpx;
  font-weight: 700;
  line-height: 88rpx;
  box-shadow: 0 10rpx 24rpx rgba(30, 47, 81, .08);
}
.home-carousel__toggle::after {
  border: 0;
}
.home-carousel__toggle--pressed {
  opacity: .72;
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
  color: #64748b;
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
  color: rgba(255, 255, 255, .94);
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
.classroom-spotlight {
  position: relative;
  display: flex;
  align-items: stretch;
  gap: 26rpx;
  padding: 30rpx;
  overflow: hidden;
  color: #fff;
  border: 1rpx solid rgba(15, 118, 110, .18);
  border-radius: 34rpx;
  background:
    radial-gradient(circle at 8% 10%, rgba(255, 255, 255, .18), transparent 28%),
    linear-gradient(135deg, #0f172a 0%, #0f766e 56%, #f97316 126%);
  box-shadow: 0 28rpx 54rpx -30rpx rgba(15, 23, 42, .5);
}
.classroom-spotlight--pressed {
  opacity: .88;
  transform: scale(.988);
}
.classroom-spotlight__copy {
  position: relative;
  z-index: 1;
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 10rpx;
}
.classroom-spotlight__eyebrow,
.classroom-spotlight__title,
.classroom-spotlight__desc {
  display: block;
}
.classroom-spotlight__eyebrow {
  color: rgba(255, 255, 255, .86);
  font-size: 22rpx;
  font-weight: 800;
}
.classroom-spotlight__title {
  color: #fff;
  font-size: 34rpx;
  font-weight: 900;
  line-height: 1.35;
}
.classroom-spotlight__desc {
  color: rgba(255, 255, 255, .82);
  font-size: 24rpx;
  line-height: 1.55;
}
.classroom-spotlight__cta {
  align-self: flex-start;
  min-height: 88rpx;
  margin-top: 8rpx;
  padding: 0 26rpx;
  color: #0f172a;
  font-size: 24rpx;
  font-weight: 900;
  line-height: 88rpx;
  background: rgba(255, 255, 255, .96);
  border-radius: 999rpx;
  box-shadow: 0 18rpx 32rpx -18rpx rgba(15, 23, 42, .48);
}
.classroom-spotlight__cta::after {
  border: 0;
}
.classroom-spotlight__cta--pressed {
  opacity: .84;
}
.classroom-spotlight__media {
  position: relative;
  flex: 0 0 206rpx;
  width: 206rpx;
  min-height: 180rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}
.classroom-spotlight__screen {
  position: relative;
  width: 188rpx;
  height: 188rpx;
  border-radius: 34rpx;
  background:
    radial-gradient(circle at 72% 28%, rgba(255, 255, 255, .46), transparent 18%),
    linear-gradient(145deg, rgba(255, 255, 255, .12), rgba(255, 255, 255, .04));
  box-shadow:
    inset 0 1rpx 0 rgba(255, 255, 255, .18),
    0 24rpx 40rpx -24rpx rgba(15, 23, 42, .6);
}
.classroom-spotlight__ring {
  position: absolute;
  inset: 22rpx;
  border: 2rpx solid rgba(255, 255, 255, .18);
  border-radius: 28rpx;
}
.classroom-spotlight__pulse {
  position: absolute;
  border-radius: 50%;
  background: rgba(255, 255, 255, .16);
}
.classroom-spotlight__pulse--one {
  top: 26rpx;
  left: 24rpx;
  width: 38rpx;
  height: 38rpx;
}
.classroom-spotlight__pulse--two {
  right: 26rpx;
  bottom: 26rpx;
  width: 54rpx;
  height: 54rpx;
}
.classroom-spotlight__play {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 0;
  height: 0;
  border-top: 20rpx solid transparent;
  border-bottom: 20rpx solid transparent;
  border-left: 32rpx solid #fff;
  transform: translate(-35%, -50%);
  filter: drop-shadow(0 8rpx 16rpx rgba(15, 23, 42, .3));
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
  color: #64748b;
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
.energy-card--blue {
  background:
    linear-gradient(180deg, rgba(8, 15, 38, 0) 18%, rgba(8, 15, 38, .72) 100%),
    linear-gradient(140deg, #2059d4, #087b9b);
}
.energy-card--purple {
  background:
    linear-gradient(180deg, rgba(8, 15, 38, 0) 18%, rgba(8, 15, 38, .72) 100%),
    linear-gradient(140deg, #6338c7, #aa2d72);
}
.energy-card--orange {
  background:
    linear-gradient(180deg, rgba(8, 15, 38, 0) 18%, rgba(8, 15, 38, .72) 100%),
    linear-gradient(140deg, #c46813, #c93d46);
}
.energy-card--pink {
  background:
    linear-gradient(180deg, rgba(8, 15, 38, 0) 18%, rgba(8, 15, 38, .72) 100%),
    linear-gradient(140deg, #b72d75, #7b3bc7);
}
.energy-card--cyan {
  background:
    linear-gradient(180deg, rgba(8, 15, 38, 0) 18%, rgba(8, 15, 38, .72) 100%),
    linear-gradient(140deg, #087b78, #187d45);
}
.energy-card__title {
  margin-top: auto;
  color: #fff;
  font-size: 29rpx;
  font-weight: 900;
}
.energy-card__desc {
  color: rgba(255, 255, 255, .94);
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
.energy-icon__shape,
.energy-icon__shape::before,
.energy-icon__shape::after {
  position: absolute;
  box-sizing: border-box;
  content: '';
}
.energy-icon--compass {
  color: #fff;
}
.energy-icon--compass .energy-icon__shape {
  inset: 15rpx;
  border: 4rpx solid currentColor;
  border-radius: 50%;
}
.energy-icon--compass .energy-icon__shape::before {
  top: 7rpx;
  left: 13rpx;
  width: 10rpx;
  height: 18rpx;
  border: 5rpx solid transparent;
  border-bottom-color: currentColor;
  transform: rotate(28deg);
}
.energy-icon--compass .energy-icon__shape::after {
  top: 14rpx;
  left: 14rpx;
  width: 7rpx;
  height: 7rpx;
  border-radius: 50%;
  background: currentColor;
}
.energy-icon--relation {
  color: #fff;
}
.energy-icon--relation .energy-icon__shape::before,
.energy-icon--relation .energy-icon__shape::after {
  top: 14rpx;
  width: 21rpx;
  height: 34rpx;
  border: 4rpx solid currentColor;
  border-radius: 50% 50% 12rpx 12rpx;
}
.energy-icon--relation .energy-icon__shape::before {
  left: 12rpx;
}
.energy-icon--relation .energy-icon__shape::after {
  left: 39rpx;
}
.energy-icon--book {
  color: #fff;
}
.energy-icon--book .energy-icon__shape {
  top: 18rpx;
  left: 14rpx;
  width: 44rpx;
  height: 34rpx;
  border: 4rpx solid currentColor;
  border-radius: 7rpx;
}
.energy-icon--book .energy-icon__shape::after {
  top: -4rpx;
  left: 18rpx;
  width: 4rpx;
  height: 34rpx;
  background: currentColor;
}
.energy-icon--growth {
  color: #fff;
}
.energy-icon--growth .energy-icon__shape {
  left: 34rpx;
  bottom: 13rpx;
  width: 4rpx;
  height: 38rpx;
  border-radius: 4rpx;
  background: currentColor;
}
.energy-icon--growth .energy-icon__shape::before,
.energy-icon--growth .energy-icon__shape::after {
  width: 22rpx;
  height: 14rpx;
  border: 4rpx solid currentColor;
  border-radius: 16rpx 2rpx 16rpx 2rpx;
}
.energy-icon--growth .energy-icon__shape::before {
  top: 8rpx;
  right: 0;
}
.energy-icon--growth .energy-icon__shape::after {
  top: -5rpx;
  left: 0;
  transform: scaleX(-1);
}
.energy-icon--spark {
  color: #fff;
}
.energy-icon--spark .energy-icon__shape {
  top: 17rpx;
  left: 33rpx;
  width: 6rpx;
  height: 38rpx;
  border-radius: 6rpx;
  background: currentColor;
}
.energy-icon--spark .energy-icon__shape::after {
  top: 16rpx;
  left: -16rpx;
  width: 38rpx;
  height: 6rpx;
  border-radius: 6rpx;
  background: currentColor;
}
.energy-icon--heart {
  color: #fff;
}
.energy-icon--heart .energy-icon__shape {
  top: 22rpx;
  left: 24rpx;
  width: 28rpx;
  height: 28rpx;
  background: currentColor;
  transform: rotate(45deg);
}
.energy-icon--heart .energy-icon__shape::before,
.energy-icon--heart .energy-icon__shape::after {
  width: 28rpx;
  height: 28rpx;
  border-radius: 50%;
  background: currentColor;
}
.energy-icon--heart .energy-icon__shape::before {
  top: -14rpx;
}
.energy-icon--heart .energy-icon__shape::after {
  left: -14rpx;
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
  min-width: 0;
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
  color: #64748b;
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
.home-nav__profile:focus,
.energy-card:focus,
.growth-card:focus,
.hero__cta:focus {
  outline: 4rpx solid #2563eb;
  outline-offset: 6rpx;
  box-shadow: 0 0 0 8rpx rgba(37, 99, 235, .2);
}
@keyframes hero-float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-10rpx); }
}
@media (min-width: 768px) {
  .hero {
    padding: 56rpx 48rpx;
  }
  .classroom-spotlight {
    padding: 34rpx;
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
    width: 68%;
    max-width: 68%;
  }
  .hero__title {
    max-width: 430rpx;
    font-size: 44rpx;
  }
  .hero__lead {
    max-width: 410rpx;
  }
  .hero__visual {
    right: -68rpx;
    bottom: 8rpx;
  }
  .classroom-spotlight {
    flex-direction: column;
  }
  .classroom-spotlight__media {
    align-self: center;
    flex-basis: auto;
    width: 188rpx;
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
  .classroom-spotlight,
  .home-nav__profile,
  .hero__cta {
    transition: none;
  }
}
</style>
