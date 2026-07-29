<script setup>
import { computed, onMounted, ref } from "vue";
import NxAsyncState from "../../components/NxAsyncState.vue";
import { listClassroomStandaloneApi } from "../../api";
import { getStoredSiteConfig, refreshSiteConfig } from "../../utils/siteConfig";
import { filterFailedCarouselItems, normalizeHomeCarousel } from "../../utils/homeCarousel";
import { MINIAPP_HOME_ENTRY_BEHAVIORS } from "../../utils/homeMenu";
import { normalizePersonalExpertHome } from "../../utils/personalExpertHome";
import { setBookingIntent } from "../../utils/bookingIntent";
import {
  classroomAccessLabel,
  classroomContentRoute,
  normalizeClassroomContent,
} from "../../utils/classroomDisplay";

const view = ref(normalizePersonalExpertHome());
const carousel = ref(normalizeHomeCarousel());
const carouselPaused = ref(false);
const failedCarouselImages = new Set();
const teacherDetailFailed = ref(false);

const siteStale = ref(false);
const siteRefreshing = ref(false);
let hasCachedSiteConfig = false;
let siteRefreshPromise = null;

const classroomItems = ref([]);
const classroomLoading = ref(true);
const classroomError = ref("");
const courseCoverErrors = ref({});
let classroomRequestPromise = null;
const secondaryEntriesEnabled = ref(true);

const secondaryEntries = computed(() => {
  if (!secondaryEntriesEnabled.value) return [];
  return view.value.secondaryEntries.filter(
    (entry) =>
      entry.enabled &&
      ["relation", "learn", "profile"].includes(entry.key) &&
      MINIAPP_HOME_ENTRY_BEHAVIORS[entry.key],
  );
});

const classroomState = computed(() => {
  if (classroomLoading.value) return "loading";
  if (classroomError.value) return "error";
  return classroomItems.value.length ? "ready" : "empty";
});

function isSecondaryEntriesEnabled(config) {
  try {
    return config?.home?.miniappHome?.entriesSection?.enabled !== false;
  } catch {
    return true;
  }
}

function applyHomeConfig(config) {
  secondaryEntriesEnabled.value = isSecondaryEntriesEnabled(config);
  view.value = normalizePersonalExpertHome(config);
  carousel.value = filterFailedCarouselItems(
    normalizeHomeCarousel(config),
    failedCarouselImages,
  );
  teacherDetailFailed.value = false;
  if (carousel.value.items.length <= 1) carouselPaused.value = false;
}

function refreshHomeConfig() {
  if (siteRefreshPromise) return siteRefreshPromise;

  siteRefreshing.value = true;
  siteRefreshPromise = refreshSiteConfig()
    .then((config) => {
      applyHomeConfig(config);
      siteStale.value = false;
      return config;
    })
    .catch(() => {
      if (hasCachedSiteConfig) siteStale.value = true;
      return null;
    })
    .finally(() => {
      siteRefreshing.value = false;
      siteRefreshPromise = null;
    });
  return siteRefreshPromise;
}

function initializeHome() {
  const cached = getStoredSiteConfig();
  hasCachedSiteConfig = !!cached;
  if (cached) applyHomeConfig(cached);
  return refreshHomeConfig();
}

function retrySiteConfig() {
  return refreshHomeConfig();
}

function toggleCarouselPaused() {
  carouselPaused.value = !carouselPaused.value;
}

function removeCarouselItem(image) {
  failedCarouselImages.add(image);
  carousel.value = filterFailedCarouselItems(carousel.value, failedCarouselImages);
  if (carousel.value.items.length <= 1) carouselPaused.value = false;
}

function markTeacherDetailError(source) {
  const failedImage =
    typeof source === "string"
      ? source
      : source?.currentTarget?.dataset?.image || source?.target?.dataset?.image || "";
  if (failedImage && failedImage !== view.value.expertHero.detailImage) return;
  teacherDetailFailed.value = true;
}

function markTeacherImageError(source) {
  markTeacherDetailError(source);
}

function previewTeacherDetail() {
  const detailImage = view.value.expertHero.detailImage;
  if (!detailImage) return;
  uni.previewImage({
    current: detailImage,
    urls: [detailImage],
  });
}

function courseCoverKey(item) {
  return `${item?.contentType || "video"}:${item?.id || ""}:${item?.coverUrl || ""}`;
}

function markCourseCoverError(key) {
  courseCoverErrors.value = { ...courseCoverErrors.value, [key]: true };
}

function loadClassroomPreview() {
  if (classroomRequestPromise) return classroomRequestPromise;

  classroomLoading.value = true;
  classroomError.value = "";
  classroomRequestPromise = listClassroomStandaloneApi({ limit: 2, offset: 0 })
    .then((response) => {
      classroomItems.value = (Array.isArray(response?.items) ? response.items : [])
        .map(normalizeClassroomContent)
        .filter((item) => item.id)
        .slice(0, 2);
      courseCoverErrors.value = {};
      return classroomItems.value;
    })
    .catch((error) => {
      classroomError.value = error?.message || "课堂内容加载失败，请稍后重试";
      return classroomItems.value;
    })
    .finally(() => {
      classroomLoading.value = false;
      classroomRequestPromise = null;
    });
  return classroomRequestPromise;
}

function retryClassroomPreview() {
  return loadClassroomPreview();
}

function formatDuration(seconds) {
  const total = Math.max(0, Math.floor(Number(seconds) || 0));
  if (!total) return "";
  const minutes = Math.floor(total / 60);
  const remaining = total % 60;
  return `${String(minutes).padStart(2, "0")}:${String(remaining).padStart(2, "0")}`;
}

function bookEnterprise() {
  setBookingIntent({ kind: "enterprise", intentText: "" });
  uni.switchTab({ url: "/pages/booking/booking" });
}

function bookEnterpriseService(service) {
  setBookingIntent({ kind: "enterprise", intentText: service.title });
  uni.switchTab({ url: "/pages/booking/booking" });
}

function startTest() {
  uni.navigateTo({ url: "/pages/test/test" });
}

function activateSecondaryEntry(entry) {
  const behavior = MINIAPP_HOME_ENTRY_BEHAVIORS[entry.key];
  if (!behavior || !["relation", "learn", "profile"].includes(entry.key)) return;
  uni[behavior.method]({ url: behavior.url });
}

function openClassroomItem(item) {
  const url = classroomContentRoute(item);
  if (url) uni.navigateTo({ url });
}

function goClassroom() {
  uni.navigateTo({ url: "/pages/classroom/classroom?tab=standalone" });
}

onMounted(() => {
  initializeHome();
  loadClassroomPreview();
});
</script>

<template>
  <view class="wrap home page-stack ios-page ios-safe-bottom">
    <view class="expert-hero nx-card">
      <button
        class="expert-hero__portrait"
        aria-label="预览完整导师介绍海报"
        hover-class="expert-hero__portrait--pressed"
        @click="previewTeacherDetail"
      >
        <image
          v-if="view.expertHero.detailImage && !teacherDetailFailed"
          class="expert-hero__image"
          :key="view.expertHero.detailImage"
          :src="view.expertHero.detailImage"
          :data-image="view.expertHero.detailImage"
          mode="aspectFit"
          aria-label="完整导师介绍海报"
          @error="markTeacherImageError"
        />
        <view v-else class="expert-hero__monogram" aria-label="导师介绍海报占位">
          {{ view.expertHero.monogram || '九' }}
        </view>
      </button>
      <button
        class="expert-hero__secondary"
        hover-class="expert-hero__secondary--pressed"
        @click="goClassroom"
      >进入老师课堂</button>
    </view>

    <view v-if="view.proofStats.length" class="proof-stats" aria-label="导师资历数据">
      <view v-for="stat in view.proofStats" :key="`${stat.value}:${stat.label}`" class="proof-stat">
        <text class="proof-stat__value">{{ stat.value }}{{ stat.suffix }}</text>
        <text class="proof-stat__label">{{ stat.label }}</text>
      </view>
    </view>

    <view v-if="carousel.items.length" class="carousel" aria-label="品牌内容轮播">
      <swiper
        class="carousel__swiper"
        :autoplay="carousel.items.length > 1 && carousel.autoplay && !carouselPaused"
        :interval="carousel.interval"
        :duration="450"
        :circular="carousel.items.length > 1"
        :indicator-dots="carousel.items.length > 1"
      >
        <swiper-item v-for="(item, index) in carousel.items" :key="item.image">
          <image
            class="carousel__image"
            :src="item.image"
            mode="aspectFill"
            lazy-load
            :aria-label="'品牌轮播图 第' + (index + 1) + '张'"
            @error="removeCarouselItem(item.image)"
          />
        </swiper-item>
      </swiper>
      <button
        v-if="carousel.items.length > 1 && carousel.autoplay"
        class="carousel__toggle"
        :aria-label="carouselPaused ? '继续轮播图自动播放' : '暂停轮播图自动播放'"
        hover-class="carousel__toggle--pressed"
        @click="toggleCarouselPaused"
      >{{ carouselPaused ? '继续轮播' : '暂停轮播' }}</button>
    </view>

    <view class="enterprise-services">
      <view class="section-heading">
        <text class="section-heading__eyebrow">{{ view.enterprise.eyebrow }}</text>
        <text class="section-heading__title">{{ view.enterprise.title }}</text>
        <text class="section-heading__lead">{{ view.enterprise.lead }}</text>
      </view>
      <view v-if="view.enterprise.modules.length" class="enterprise-services__modules">
        <text v-for="module in view.enterprise.modules" :key="module" class="enterprise-services__module">{{ module }}</text>
      </view>
      <view class="enterprise-services__list">
        <button
          v-for="service in view.enterprise.services"
          :key="service.title"
          class="enterprise-service"
          :aria-label="`预约${service.title}`"
          hover-class="enterprise-service--pressed"
          @click="bookEnterpriseService(service)"
        >
          <view class="enterprise-service__number" aria-hidden="true" />
          <view class="enterprise-service__copy">
            <text class="enterprise-service__title">{{ service.title }}</text>
            <text class="enterprise-service__description">{{ service.description }}</text>
          </view>
          <view class="enterprise-service__arrow" aria-hidden="true" />
        </button>
      </view>
    </view>

    <view v-if="view.game.enabled" class="test-game nx-card">
      <view class="test-game__copy">
        <text class="test-game__eyebrow">{{ view.game.eyebrow }}</text>
        <text class="test-game__title">{{ view.game.title }}</text>
        <text class="test-game__lead">{{ view.game.lead }}</text>
        <text class="test-game__meta">18道生活情境题 · 约3分钟</text>
      </view>
      <button class="test-game__cta" hover-class="test-game__cta--pressed" @click="startTest">
        {{ view.game.buttonText }}
      </button>
    </view>

    <view class="classroom-preview">
      <view class="section-heading section-heading--row">
        <view>
          <text class="section-heading__eyebrow">老师课堂</text>
          <text class="section-heading__title">最近更新</text>
        </view>
        <button class="classroom-preview__more" @click="goClassroom">查看全部</button>
      </view>

      <NxAsyncState v-if="classroomState === 'loading'" state="loading" />
      <NxAsyncState
        v-else-if="classroomState === 'error'"
        state="error"
        title="课堂内容暂未加载"
        :description="classroomError"
        action-text="重新加载"
        :busy="classroomLoading"
        @action="retryClassroomPreview"
      />
      <NxAsyncState
        v-else-if="classroomState === 'empty'"
        state="empty"
        title="课堂内容正在准备"
        description="老师正在整理更多视频与音频内容，稍后再来看看。"
        action-text="刷新课堂"
        :busy="classroomLoading"
        @action="retryClassroomPreview"
      />
      <view v-else class="classroom-preview__list">
        <button
          v-for="item in classroomItems"
          :key="item.id"
          class="classroom-card"
          :aria-label="`查看${item.title || '老师课堂课件'}`"
          hover-class="classroom-card--pressed"
          @click="openClassroomItem(item)"
        >
          <view class="classroom-card__media">
            <image
              v-if="item.coverUrl && !courseCoverErrors[courseCoverKey(item)]"
              class="classroom-card__cover"
              :src="item.coverUrl"
              mode="aspectFill"
              lazy-load
              :aria-label="`${item.title || '课堂课件'}封面`"
              @error="markCourseCoverError(courseCoverKey(item))"
            />
            <view v-else class="classroom-card__cover-fallback" aria-hidden="true">
              <view class="classroom-card__fallback-line classroom-card__fallback-line--long" />
              <view class="classroom-card__fallback-line" />
              <view class="classroom-card__fallback-play" />
            </view>
          </view>
          <view class="classroom-card__body">
            <view class="classroom-card__meta">
              <text>{{ item.contentType === 'audio' ? '音频' : '视频' }}</text>
              <text>{{ classroomAccessLabel(item.effectiveAccess) }}</text>
              <text v-if="formatDuration(item.durationSeconds)">{{ formatDuration(item.durationSeconds) }}</text>
            </view>
            <text class="classroom-card__title">{{ item.title || '未命名课件' }}</text>
            <text v-if="item.description" class="classroom-card__description">{{ item.description }}</text>
          </view>
        </button>
      </view>
    </view>

    <view v-if="secondaryEntries.length" class="secondary-entries">
      <view class="section-heading">
        <text class="section-heading__eyebrow">继续探索</text>
        <text class="section-heading__title">把理解带进生活</text>
      </view>
      <view class="secondary-entries__grid">
        <button
          v-for="entry in secondaryEntries"
          :key="entry.key"
          class="secondary-entry"
          :aria-label="MINIAPP_HOME_ENTRY_BEHAVIORS[entry.key].ariaLabel"
          hover-class="secondary-entry--pressed"
          @click="activateSecondaryEntry(entry)"
        >
          <view :class="['secondary-entry__mark', `secondary-entry__mark--${entry.icon}`]" aria-hidden="true">
            <view class="secondary-entry__mark-line" />
          </view>
          <text class="secondary-entry__title">{{ entry.title }}</text>
          <text class="secondary-entry__description">{{ entry.description }}</text>
        </button>
      </view>
    </view>

    <view class="enterprise-final-cta">
      <text class="enterprise-final-cta__eyebrow">为团队定制一次真正有共识的学习</text>
      <text class="enterprise-final-cta__title">从一次沟通开始，找到适合团队的共学方式</text>
      <button
        class="enterprise-final-cta__button"
        hover-class="enterprise-final-cta__button--pressed"
        @click="bookEnterprise"
      >{{ view.enterprise.buttonText }}</button>
    </view>

    <NxAsyncState
      v-if="siteStale"
      state="stale"
      title="当前展示上次内容"
      description="最新站点内容暂未同步，首页其他功能仍可继续使用。"
      action-text="重新同步"
      :busy="siteRefreshing"
      @action="retrySiteConfig"
    />
  </view>
</template>

<style scoped>
.home {
  gap: 32rpx;
  background:
    radial-gradient(circle at 94% 3%, var(--nx-home-gold-halo), transparent 24%),
    linear-gradient(180deg, var(--nx-surface-soft), var(--nx-page-bg));
  color: var(--nx-text);
}

button {
  box-sizing: border-box;
  margin: 0;
}

button::after {
  border: 0;
}

.expert-hero__portrait--pressed,
.expert-hero__secondary--pressed,
.enterprise-service--pressed,
.test-game__cta--pressed,
.classroom-card--pressed,
.secondary-entry--pressed,
.enterprise-final-cta__button--pressed,
.carousel__toggle--pressed {
  opacity: 0.76;
  transform: scale(0.98);
}

.expert-hero {
  position: relative;
  width: 640rpx;
  max-width: 100%;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  overflow: visible;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

.section-heading__eyebrow,
.test-game__eyebrow,
.enterprise-final-cta__eyebrow {
  color: var(--nx-accent-gold);
  font-size: 22rpx;
  font-weight: 800;
  letter-spacing: 2rpx;
}

.expert-hero__secondary {
  min-height: 88rpx;
  position: relative;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 26rpx;
  border: 2rpx solid var(--nx-home-gold-portrait-border);
  border-radius: 999rpx;
  background: var(--nx-brand-900);
  color: var(--nx-surface);
  font-size: 24rpx;
  font-weight: 800;
  line-height: 1.2;
  transition: opacity 180ms ease-out, transform 180ms ease-out;
}

.expert-hero__portrait {
  position: relative;
  width: 100%;
  min-height: 88rpx;
  height: 1140rpx;
  overflow: hidden;
  margin: 0;
  padding: 0;
  border: 2rpx solid var(--nx-home-gold-portrait-border);
  border-radius: 40rpx;
  background: var(--nx-brand-900);
  box-shadow: 0 30rpx 64rpx -34rpx var(--nx-home-elevated-shadow);
  line-height: normal;
  transition: opacity 180ms ease-out, transform 180ms ease-out;
}

.expert-hero__image,
.expert-hero__monogram {
  width: 100%;
  height: 100%;
}

.expert-hero__monogram {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--nx-accent-gold);
  font-size: 112rpx;
  font-weight: 300;
  background:
    linear-gradient(155deg, transparent 30%, var(--nx-home-gold-monogram)),
    var(--nx-brand-700);
}

.proof-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 2rpx;
  overflow: hidden;
  border: 2rpx solid var(--nx-border);
  border-radius: 28rpx;
  background: var(--nx-border);
}

.proof-stat {
  min-height: 142rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6rpx;
  padding: 20rpx 12rpx;
  background: var(--nx-surface);
  text-align: center;
}

.proof-stat__value {
  color: var(--nx-brand-900);
  font-size: 36rpx;
  font-weight: 800;
}

.proof-stat__label {
  color: var(--nx-text-muted);
  font-size: 21rpx;
  line-height: 1.4;
}

.carousel {
  display: flex;
  flex-direction: column;
  gap: 10rpx;
}

.carousel__swiper {
  width: 100%;
  height: 300rpx;
  overflow: hidden;
  border: 2rpx solid var(--nx-border);
  border-radius: 30rpx;
  background: var(--nx-surface);
}

.carousel__image {
  display: block;
  width: 100%;
  height: 100%;
}

.carousel__toggle,
.classroom-preview__more {
  align-self: flex-end;
  min-height: 88rpx;
  padding: 0 22rpx;
  border: 0;
  background: transparent;
  color: var(--nx-brand-700);
  font-size: 23rpx;
  font-weight: 700;
  line-height: 88rpx;
}

.enterprise-services,
.classroom-preview,
.secondary-entries {
  display: flex;
  flex-direction: column;
  gap: 22rpx;
}

.section-heading {
  display: flex;
  flex-direction: column;
  gap: 9rpx;
}

.section-heading--row {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}

.section-heading__eyebrow {
  display: block;
  color: var(--nx-brand-700);
}

.section-heading__title {
  display: block;
  color: var(--nx-brand-900);
  font-size: 38rpx;
  font-weight: 800;
  line-height: 1.3;
}

.section-heading__lead {
  color: var(--nx-text-muted);
  font-size: 25rpx;
  line-height: 1.65;
}

.enterprise-services__modules {
  display: flex;
  flex-wrap: wrap;
  gap: 12rpx;
}

.enterprise-services__module {
  padding: 12rpx 20rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 999rpx;
  background: var(--nx-surface-soft);
  color: var(--nx-brand-700);
  font-size: 22rpx;
}

.enterprise-services__list {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
}

.enterprise-service {
  width: 100%;
  min-height: 88rpx;
  display: flex;
  align-items: center;
  gap: 22rpx;
  padding: 26rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 28rpx;
  background: var(--nx-surface);
  color: var(--nx-text);
  text-align: left;
  transition: opacity 180ms ease-out, transform 180ms ease-out;
}

.enterprise-service__number {
  flex: 0 0 22rpx;
  width: 22rpx;
  height: 22rpx;
  border: 6rpx solid var(--nx-accent-gold);
  border-radius: 50%;
  box-shadow: 0 0 0 5rpx var(--nx-home-gold-halo);
}

.enterprise-service__copy {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}

.enterprise-service__title {
  color: var(--nx-brand-900);
  font-size: 29rpx;
  font-weight: 800;
  line-height: 1.35;
}

.enterprise-service__description {
  color: var(--nx-text-muted);
  font-size: 23rpx;
  line-height: 1.55;
}

.enterprise-service__arrow {
  width: 16rpx;
  height: 16rpx;
  border-top: 3rpx solid var(--nx-brand-700);
  border-right: 3rpx solid var(--nx-brand-700);
  transform: rotate(45deg);
}

.test-game {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24rpx;
  padding: 34rpx;
  border: 2rpx solid var(--nx-home-gold-test-border);
  border-radius: 32rpx;
  background:
    linear-gradient(110deg, var(--nx-home-gold-test-wash), transparent 56%),
    var(--nx-surface);
}

.test-game__copy {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}

.test-game__eyebrow {
  color: var(--nx-brand-700);
}

.test-game__title {
  color: var(--nx-brand-900);
  font-size: 36rpx;
  font-weight: 800;
}

.test-game__lead,
.test-game__meta {
  color: var(--nx-text-muted);
  font-size: 23rpx;
  line-height: 1.55;
}

.test-game__meta {
  color: var(--nx-brand-700);
  font-weight: 700;
}

.test-game__cta {
  flex: 0 0 auto;
  min-height: 88rpx;
  padding: 0 26rpx;
  border-radius: 999rpx;
  background: var(--nx-brand-900);
  color: var(--nx-surface);
  font-size: 23rpx;
  font-weight: 800;
  line-height: 88rpx;
  transition: opacity 180ms ease-out, transform 180ms ease-out;
}

.classroom-preview__list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16rpx;
}

.classroom-card {
  min-height: 88rpx;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 0;
  border: 2rpx solid var(--nx-border);
  border-radius: 28rpx;
  background: var(--nx-surface);
  color: var(--nx-text);
  text-align: left;
  transition: opacity 180ms ease-out, transform 180ms ease-out;
}

.classroom-card__media {
  width: 100%;
  height: 190rpx;
  overflow: hidden;
  background: var(--nx-brand-700);
}

.classroom-card__cover,
.classroom-card__cover-fallback {
  width: 100%;
  height: 100%;
}

.classroom-card__cover-fallback {
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  gap: 12rpx;
  box-sizing: border-box;
  padding: 28rpx;
  background:
    linear-gradient(145deg, var(--nx-home-gold-border), transparent 50%),
    var(--nx-brand-700);
}

.classroom-card__fallback-line {
  width: 48%;
  height: 8rpx;
  border-radius: 999rpx;
  background: var(--nx-home-on-brand-soft);
}

.classroom-card__fallback-line--long {
  width: 68%;
}

.classroom-card__fallback-play {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 0;
  height: 0;
  border-top: 18rpx solid transparent;
  border-bottom: 18rpx solid transparent;
  border-left: 28rpx solid var(--nx-surface);
  transform: translate(-38%, -50%);
}

.classroom-card__body {
  display: flex;
  flex-direction: column;
  gap: 10rpx;
  padding: 22rpx;
}

.classroom-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10rpx;
  color: var(--nx-brand-700);
  font-size: 20rpx;
  font-weight: 700;
}

.classroom-card__title {
  color: var(--nx-brand-900);
  font-size: 27rpx;
  font-weight: 800;
  line-height: 1.4;
}

.classroom-card__description {
  color: var(--nx-text-muted);
  font-size: 22rpx;
  line-height: 1.5;
}

.secondary-entries__grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14rpx;
}

.secondary-entry {
  min-height: 88rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8rpx;
  padding: 24rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 26rpx;
  background: var(--nx-surface);
  color: var(--nx-text);
  text-align: left;
  transition: opacity 180ms ease-out, transform 180ms ease-out;
}

.secondary-entry__mark {
  position: relative;
  width: 56rpx;
  height: 56rpx;
  margin-bottom: 12rpx;
  border: 2rpx solid var(--nx-home-mark-border);
  border-radius: 18rpx;
  background: var(--nx-surface-soft);
}

.secondary-entry__mark-line,
.secondary-entry__mark-line::before,
.secondary-entry__mark-line::after {
  position: absolute;
  box-sizing: border-box;
  content: "";
}

.secondary-entry__mark-line {
  inset: 14rpx;
  border: 4rpx solid var(--nx-brand-700);
  border-radius: 50%;
}

.secondary-entry__mark--book .secondary-entry__mark-line {
  border-radius: 5rpx;
}

.secondary-entry__mark--growth .secondary-entry__mark-line {
  left: 25rpx;
  width: 5rpx;
  border: 0;
  border-radius: 999rpx;
  background: var(--nx-brand-700);
}

.secondary-entry__title {
  color: var(--nx-brand-900);
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.35;
}

.secondary-entry__description {
  color: var(--nx-text-muted);
  font-size: 21rpx;
  line-height: 1.5;
}

.enterprise-final-cta {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16rpx;
  padding: 42rpx 36rpx;
  border-radius: 34rpx;
  background:
    linear-gradient(130deg, transparent 42%, var(--nx-home-gold-final-wash)),
    var(--nx-brand-900);
}

.enterprise-final-cta__title {
  max-width: 600rpx;
  color: var(--nx-surface);
  font-size: 38rpx;
  font-weight: 800;
  line-height: 1.38;
}

.enterprise-final-cta__button {
  min-height: 88rpx;
  margin-top: 6rpx;
  padding: 0 32rpx;
  border-radius: 999rpx;
  background: var(--nx-accent-gold);
  color: var(--nx-brand-900);
  font-size: 25rpx;
  font-weight: 800;
  line-height: 88rpx;
  transition: opacity 180ms ease-out, transform 180ms ease-out;
}

@media (prefers-reduced-motion: reduce) {
  .home button {
    transition: none;
  }
}

@media (max-width: 380px) {
  .expert-hero {
    width: 100%;
  }

  .expert-hero__portrait {
    height: auto;
    min-height: 0;
    aspect-ratio: 1024 / 1824;
  }

  .expert-hero__secondary {
    min-height: 88rpx;
  }

  .test-game {
    align-items: flex-start;
    flex-direction: column;
  }

  .secondary-entries__grid {
    grid-template-columns: 1fr;
  }
}
</style>
