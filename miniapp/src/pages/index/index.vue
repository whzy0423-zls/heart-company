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
        <view class="home-hero__copy">
          <view class="home-hero__brand">
            <text class="home-hero__eyebrow">九型芯之力 · ENNEAGRAM</text>
            <text class="home-hero__issue">NO. 09</text>
          </view>
          <text class="home-hero__title">看见性格背后，真正驱动你的力量</text>
          <text class="home-hero__lead">用 {{ total }} 道题建立你的九型人格地图，从自我理解走向关系与成长。</text>
          <button class="nx-button--primary home-hero__cta" @click="startTest">开始测试</button>
        </view>

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

        <view class="home-hero__types" aria-label="九种人格类型">
          <text class="home-hero__types-label">TYPE INDEX</text>
          <view class="home-hero__badges">
            <TypeBadge v-for="typeId in typeIds" :key="typeId" :type-id="typeId" size="sm" />
          </view>
        </view>
      </view>

      <view class="home__section-heading">
        <text class="home__section-kicker">EXPLORE / 继续探索</text>
        <text class="home__section-title">把洞察带进真实生活</text>
      </view>

      <view class="home-entries">
        <view
          class="home-entry home-entry--relation"
          role="button"
          aria-label="进入关系合盘"
          aria-pressed="false"
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
        </view>

        <view
          class="home-entry home-entry--learn"
          role="button"
          aria-label="进入九型学习"
          aria-pressed="false"
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
        </view>
      </view>

      <view class="home-course-wrap">
        <view class="home-course__heading">
          <text class="home-course__label">本周推荐</text>
          <text class="home-course__edition">WEEKLY EDIT</text>
        </view>
        <view
          class="home-course nx-media-row"
          role="button"
          aria-label="打开推荐课程"
          aria-pressed="false"
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
        </view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.home {
  background:
    linear-gradient(90deg, transparent 0, transparent 49%, rgba(23, 33, 43, .035) 50%, transparent 51%),
    var(--nx-bg);
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
  background: var(--nx-ink);
  color: #FFFFFF;
}

.home-hero__copy {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 22rpx;
  padding: 40rpx 34rpx 34rpx;
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
  font-size: 58rpx;
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
}

.home-hero__art {
  position: relative;
  width: 100%;
  height: 410rpx;
  overflow: hidden;
  background: var(--nx-coral-soft);
}

.home-hero__image {
  display: block;
  width: 100%;
  height: 410rpx;
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

.home-hero__types {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  padding: 24rpx 28rpx 28rpx;
  background: var(--nx-surface);
  color: var(--nx-ink);
}

.home-hero__types-label { color: var(--nx-muted); }

.home-hero__badges {
  display: flex;
  flex-wrap: wrap;
  gap: 8rpx;
}

.home__section-heading {
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  padding: 12rpx 4rpx 0;
}

.home__section-kicker { color: var(--nx-blue); }

.home__section-title {
  color: var(--nx-ink);
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.2;
}

.home-entries {
  display: grid;
  grid-template-columns: 1.18fr .82fr;
  gap: 18rpx;
}

.home-entry {
  position: relative;
  min-height: 330rpx;
  padding: 30rpx;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 26rpx;
  overflow: hidden;
  border: 2rpx solid var(--nx-ink);
  box-sizing: border-box;
  transition: opacity .16s ease, transform .16s ease;
  touch-action: manipulation;
}

.home-entry--relation {
  background: var(--nx-coral);
  color: #FFFFFF;
}

.home-entry--learn {
  margin-top: 52rpx;
  background: var(--nx-surface);
  color: var(--nx-ink);
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
  padding: 28rpx;
  border: 2rpx solid var(--nx-line);
  background: var(--nx-surface);
  box-shadow: var(--nx-shadow-sm);
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

.home-course__edition { color: var(--nx-jade); }

.home-course {
  min-height: 230rpx;
  padding-top: 24rpx;
  align-items: stretch;
  transition: opacity .16s ease, transform .16s ease;
  touch-action: manipulation;
}

.home-course__cover {
  flex: 0 0 210rpx;
  width: 210rpx;
  height: 230rpx;
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
  color: var(--nx-muted);
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
  color: var(--nx-muted);
  font-size: 23rpx;
  line-height: 1.54;
}

.home-course__link {
  margin-top: auto;
  color: var(--nx-blue);
  font-size: 23rpx;
  font-weight: 900;
}

@media screen and (min-width: 700px) {
  .home-hero__copy { padding: 54rpx 48rpx 42rpx; }
  .home-hero__title { font-size: 64rpx; }
  .home-hero__art,
  .home-hero__image { height: 450rpx; }
  .home-entry { min-height: 300rpx; }
  .home-entry--learn { margin-top: 34rpx; }
}

@media screen and (max-width: 360px) {
  .home-entries { grid-template-columns: 1fr; }
  .home-entry { min-height: 250rpx; }
  .home-entry--learn { margin-top: 0; }
  .home-course { flex-direction: column; }
  .home-course__cover { width: 100%; height: 300rpx; flex-basis: 300rpx; }
}

@media (prefers-reduced-motion: reduce) {
  .home-entry,
  .home-course { transition: none; }
}
</style>
