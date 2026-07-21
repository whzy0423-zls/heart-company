<script setup>
import { ref } from 'vue'
import TypeBadge from '../../components/type-badge.vue'
import { QUESTIONS } from '../../data/enneagramGame'
import { DEFAULT_COURSEWARE_ITEMS } from '../../utils/teacherCourseware'

const total = ref(QUESTIONS.length)
const typeIds = [1, 2, 3, 4, 5, 6, 7, 8, 9]
const recommendedCourse = DEFAULT_COURSEWARE_ITEMS[0]

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
  <view class="home nx-page page-stack ios-page ios-safe-bottom">
    <view class="home__canvas">
      <view class="nx-editorial-hero home-editorial-hero">
        <view class="home-hero__composition">
          <view class="home-hero__art">
            <image
              class="home-hero__image"
              src="/static/editorial/home-hero.webp"
              mode="aspectFill"
            />
            <view class="home-hero__caption">
              <text>认识动机</text>
              <text>而不只是一张标签</text>
            </view>
          </view>

          <view class="home-hero__copy">
            <view class="home-hero__brand">
              <text class="home-hero__eyebrow">九型芯之力 · ENNEAGRAM</text>
              <text class="home-hero__issue">NO. 09</text>
            </view>
            <text class="home-hero__title">看见性格背后，真正驱动你的力量</text>
            <text class="home-hero__lead">用 {{ total }} 道题建立你的九型人格地图，从自我理解走向关系与成长。</text>
            <button class="nx-button--primary home-hero__cta" @click="startTest">开始测试</button>
          </view>

          <view class="home-hero__float-token" aria-hidden="true">
            <text>TYPE</text>
            <text>09</text>
          </view>
        </view>

        <view class="home-hero__types" aria-label="九种人格类型">
          <text class="home-hero__types-label">TYPE INDEX</text>
          <view class="home-hero__badges">
            <TypeBadge v-for="typeId in typeIds" :key="typeId" :type-id="typeId" size="sm" />
          </view>
        </view>
      </view>

      <view class="home-explore-band">
        <view class="home__section-heading">
          <text class="home__section-kicker">EXPLORE / 继续探索</text>
          <text class="home__section-title">把洞察带进真实生活</text>
        </view>

        <view class="home-bento">
          <button
            class="home-entry home-entry--relation"
            aria-label="进入关系合盘"
            hover-class="home-entry--pressed"
            @click="goRelation"
          >
            <view class="home-entry__number">01</view>
            <view class="home-entry__copy">
              <text class="home-entry__kicker">RELATIONSHIP</text>
              <text class="home-entry__title">关系合盘</text>
              <text class="home-entry__desc">看见彼此的沟通节奏、冲突触发点与相处底色。</text>
            </view>
            <text class="home-entry__arrow" aria-hidden="true">↗</text>
          </button>

          <button
            class="home-entry home-entry--learn"
            aria-label="进入九型学习"
            hover-class="home-entry--pressed"
            @click="goLearn"
          >
            <view class="home-entry__number">02</view>
            <view class="home-entry__copy">
              <text class="home-entry__kicker">LEARN WITH A GUIDE</text>
              <text class="home-entry__title">跟着老师学</text>
              <text class="home-entry__desc">从老师资料、课件与课程路径，建立清晰的九型地图。</text>
            </view>
            <text class="home-entry__arrow" aria-hidden="true">↗</text>
          </button>

          <view class="home-course-wrap">
            <view class="home-course__heading">
              <text class="home-course__label">本周推荐</text>
              <text class="home-course__edition">WEEKLY EDIT</text>
            </view>
            <button
              class="home-course nx-media-row"
              aria-label="打开推荐课程"
              hover-class="home-course--pressed"
              @click="goLearn"
            >
              <image
                class="home-course__cover"
                src="/static/editorial/course-intro.webp"
                mode="aspectFill"
                lazy-load
              />
              <view class="home-course__copy">
                <view class="home-course__meta">
                  <text class="home-course__badge">{{ recommendedCourse.badge }}</text>
                  <text class="home-course__duration">{{ recommendedCourse.duration }}</text>
                </view>
                <text class="home-course__title">{{ recommendedCourse.title }}</text>
                <text class="home-course__desc">{{ recommendedCourse.description }}</text>
                <text class="home-course__link">查看课程与课件 →</text>
              </view>
            </button>
          </view>
        </view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.home {
  background:
    linear-gradient(90deg, transparent 0, transparent 49%, rgba(23, 33, 43, .04) 50%, transparent 51%),
    var(--nx-bg);
  overflow-x: hidden;
}

.home__canvas {
  width: 100%;
  max-width: 900rpx;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 34rpx;
}

.home-editorial-hero {
  padding: 0;
  gap: 0;
  overflow: hidden;
  border-color: var(--nx-ink);
  background: var(--nx-surface);
  box-shadow: none;
}

.home-hero__composition {
  position: relative;
  padding: 0 24rpx 34rpx;
  background: var(--nx-coral-soft);
}

.home-hero__copy {
  position: relative;
  z-index: 2;
  width: calc(100% - 30rpx);
  margin-top: -76rpx;
  margin-left: 15rpx;
  padding: 34rpx 30rpx 32rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 20rpx;
  box-sizing: border-box;
  background: var(--nx-ink);
  color: #FFFFFF;
  animation: home-rise .22s ease-out both;
}

.home-hero__brand {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
}

.home-hero__eyebrow,
.home-hero__issue,
.home__section-kicker,
.home-entry__kicker,
.home-course__edition,
.home-hero__types-label {
  font-size: 20rpx;
  font-weight: 900;
  line-height: 1.3;
  letter-spacing: 2rpx;
}

.home-hero__eyebrow { color: #FFFFFF; }
.home-hero__issue { color: var(--nx-coral-soft); }

.home-hero__title {
  max-width: 620rpx;
  color: #FFFFFF;
  font-size: 54rpx;
  font-weight: 900;
  line-height: 1.12;
  letter-spacing: -1rpx;
}

.home-hero__lead {
  max-width: 590rpx;
  color: rgba(255, 255, 255, .76);
  font-size: 27rpx;
  line-height: 1.68;
}

.home-hero__cta {
  width: 280rpx;
  margin-top: 4rpx;
  border: 0;
  box-shadow: none;
  transition: transform .22s ease, background-color .22s ease, box-shadow .22s ease;
}

.home-hero__art {
  position: relative;
  width: calc(100% + 48rpx);
  margin-left: -24rpx;
  aspect-ratio: 4 / 3;
  overflow: hidden;
  background: var(--nx-coral-soft);
}

.home-hero__image {
  display: block;
  width: 100%;
  height: 100%;
}

.home-hero__caption {
  position: absolute;
  right: 24rpx;
  bottom: 24rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  padding: 14rpx 18rpx;
  background: var(--nx-coral);
  color: #FFFFFF;
  font-size: 20rpx;
  font-weight: 800;
  line-height: 1.4;
}

.home-hero__float-token {
  position: absolute;
  z-index: 3;
  top: 22rpx;
  left: 12rpx;
  width: 92rpx;
  height: 92rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border: 4rpx solid var(--nx-ink);
  background: #F4C64E;
  color: var(--nx-ink);
  font-size: 20rpx;
  font-weight: 900;
  line-height: 1.05;
  letter-spacing: 1rpx;
  transform: rotate(-7deg);
  pointer-events: none;
}

.home-hero__types {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  padding: 24rpx 28rpx 28rpx;
  background: var(--nx-surface);
  color: var(--nx-ink);
  border-top: 2rpx solid var(--nx-ink);
}

.home-hero__types-label { color: var(--nx-muted); }

.home-hero__badges {
  display: flex;
  flex-wrap: wrap;
  gap: 8rpx;
}

.home-explore-band {
  margin: 0 -24rpx;
  padding: 34rpx 24rpx 38rpx;
  background:
    repeating-linear-gradient(0deg, transparent 0, transparent 11rpx, rgba(23, 33, 43, .035) 12rpx),
    linear-gradient(120deg, #E8EEF3 0%, #E8EEF3 54%, #F3E9D2 54%, #F3E9D2 100%);
  border-top: 2rpx solid var(--nx-ink);
  border-bottom: 2rpx solid var(--nx-ink);
}

.home__section-heading {
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  padding: 0 4rpx 24rpx;
}

.home__section-kicker { color: var(--nx-blue); }

.home__section-title {
  color: var(--nx-ink);
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.2;
}

.home-bento {
  display: grid;
  grid-template-columns: 1fr;
  grid-template-areas:
    "relation"
    "learn"
    "course";
  gap: 18rpx;
}

.home-entry {
  position: relative;
  width: 100%;
  min-height: 330rpx;
  margin: 0;
  padding: 30rpx;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 26rpx;
  overflow: hidden;
  border: 2rpx solid var(--nx-ink);
  border-radius: 0;
  box-sizing: border-box;
  color: inherit;
  font: inherit;
  line-height: inherit;
  text-align: left;
  transition: opacity .22s ease, transform .22s ease;
  touch-action: manipulation;
  animation: home-rise .22s ease-out both;
}

.home-entry--relation {
  grid-area: relation;
  min-height: 360rpx;
  background: var(--nx-coral);
  color: #FFFFFF;
}

.home-entry--learn {
  grid-area: learn;
  min-height: 250rpx;
  background: #D8E5EE;
  color: var(--nx-ink);
  animation-delay: .04s;
}

.home-entry__number {
  align-self: flex-start;
  min-width: 56rpx;
  padding-bottom: 8rpx;
  border-bottom: 4rpx solid currentColor;
  font-size: 22rpx;
  font-weight: 900;
}

.home-entry__copy {
  display: flex;
  flex-direction: column;
  gap: 12rpx;
}

.home-entry__kicker { opacity: .72; }

.home-entry__title {
  font-size: 36rpx;
  font-weight: 900;
  line-height: 1.2;
}

.home-entry__desc {
  font-size: 24rpx;
  line-height: 1.62;
}

.home-entry__arrow {
  position: absolute;
  top: 22rpx;
  right: 26rpx;
  font-size: 34rpx;
  font-weight: 900;
}

.home-entry--pressed,
.home-course--pressed {
  opacity: .82;
  transform: scale(.985);
}

.home-course-wrap {
  grid-area: course;
  padding: 28rpx;
  border: 2rpx solid var(--nx-ink);
  background: #F2E8D0;
  animation: home-rise .22s .08s ease-out both;
}

.home-course__heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16rpx;
  padding-bottom: 20rpx;
  border-bottom: 2rpx solid var(--nx-line);
}

.home-course__label {
  color: var(--nx-ink);
  font-size: 30rpx;
  font-weight: 900;
}

.home-course__edition { color: #59636C; }

.home-course {
  width: 100%;
  min-height: 230rpx;
  margin: 0;
  padding: 24rpx 0 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-sizing: border-box;
  align-items: stretch;
  color: inherit;
  font: inherit;
  line-height: inherit;
  text-align: left;
  transition: opacity .22s ease, transform .22s ease;
  touch-action: manipulation;
}

.home-entry::after,
.home-course::after {
  border: 0;
}

.home-entry:focus-visible,
.home-course:focus-visible {
  outline: 4rpx solid var(--nx-focus);
  outline-offset: 4rpx;
}

.home-course__cover {
  flex: 0 0 190rpx;
  width: 190rpx;
  height: 250rpx;
  background: var(--nx-coral-soft);
}

.home-course__copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10rpx;
}

.home-course__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12rpx;
}

.home-course__badge {
  padding: 4rpx 12rpx;
  background: var(--nx-jade);
  color: #FFFFFF;
  font-size: 20rpx;
  font-weight: 900;
}

.home-course__duration {
  color: #59636C;
  font-size: 21rpx;
  font-weight: 700;
}

.home-course__title {
  color: var(--nx-ink);
  font-size: 30rpx;
  font-weight: 900;
  line-height: 1.3;
}

.home-course__desc {
  color: #59636C;
  font-size: 23rpx;
  line-height: 1.54;
}

.home-course__link {
  margin-top: auto;
  color: var(--nx-blue);
  font-size: 23rpx;
  font-weight: 900;
}

@keyframes home-rise {
  from { opacity: 0; transform: translateY(10rpx); }
  to { opacity: 1; transform: translateY(0); }
}

@media screen and (min-width: 768px) {
  .home__canvas { max-width: 1180rpx; }
  .home-hero__composition {
    display: grid;
    grid-template-columns: 1.08fr .92fr;
    align-items: center;
    padding: 0;
    background: var(--nx-coral-soft);
  }
  .home-hero__art {
    width: calc(100% + 48rpx);
    margin-left: -24rpx;
    grid-column: 1;
    grid-row: 1;
  }
  .home-hero__copy {
    width: calc(100% + 38rpx);
    margin-top: 0;
    margin-left: -38rpx;
    grid-column: 2;
    grid-row: 1;
    padding: 46rpx 42rpx 40rpx;
  }
  .home-hero__title { font-size: 64rpx; }
  .home-hero__float-token { left: calc(54% - 70rpx); }
  .home-bento {
    grid-template-columns: 1.14fr .86fr;
    grid-template-areas:
      "relation learn"
      "relation course";
    align-items: stretch;
  }
  .home-entry--relation { min-height: 620rpx; }
  .home-entry--learn { min-height: 250rpx; }
  .home-course-wrap { min-height: 350rpx; }
  .home-course__cover { flex-basis: 170rpx; width: 170rpx; height: 230rpx; }
}

@media (prefers-reduced-motion: reduce) {
  .home-hero__copy,
  .home-hero__cta,
  .home-entry,
  .home-course,
  .home-course-wrap {
    animation: none;
    transition: none;
  }
}
</style>
