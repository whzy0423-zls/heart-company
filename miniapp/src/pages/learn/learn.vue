<script setup>
import { ref, onMounted } from "vue";
import { TYPES_INFO } from "../../data/enneagramGame";
import {
  getStoredSiteConfig,
  hasSiteConfigLearningSection,
  refreshSiteConfig,
} from "../../utils/siteConfig";
import { userErrorMessage } from "../../utils/userMessage";
import { normalizeCoursewareItems, normalizeTeachers } from "../../utils/teacherCourseware";
import { listClassroomSeriesApi, listClassroomStandaloneApi } from "../../api";
import {
  classroomCoverRatioClass,
  normalizeClassroomContent,
  normalizeClassroomSeries,
} from "../../utils/classroomDisplay";

const teachers = ref([]);
const coursewareItems = ref([]);
const quotes = ref([]);
const types = ref(Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] })));
const teacherImageErrors = ref({});
const courseImageErrors = ref({});
const typeImageErrors = ref({});
const loading = ref(true);
const loadError = ref("");
const refreshError = ref("");
const refreshing = ref(false);
const classroomPreview = ref([]);
const classroomLoading = ref(true);
const classroomWarning = ref("");
const classroomPreviewCoverErrors = ref({});
let loadTicket = 0;
let classroomTicket = 0;

function applyContent(cfg) {
  teachers.value = normalizeTeachers(cfg);
  coursewareItems.value = normalizeCoursewareItems(cfg);
  quotes.value = cfg?.home?.quotes?.items || [];
  teacherImageErrors.value = {};
  courseImageErrors.value = {};
}

function showStoredContent() {
  const cached = getStoredSiteConfig();
  if (!cached || !hasSiteConfigLearningSection(cached)) return false;
  applyContent(cached);
  loading.value = false;
  loadError.value = "";
  return true;
}

async function loadContent(options = {}) {
  const silent = !!options.silent;
  const ticket = ++loadTicket;
  if (silent) {
    refreshing.value = true;
  } else {
    refreshing.value = false;
    loading.value = true;
    loadError.value = "";
    refreshError.value = "";
  }
  try {
    const cfg = await refreshSiteConfig();
    if (ticket !== loadTicket) return;
    if (silent && !hasSiteConfigLearningSection(cfg)) return;
    applyContent(cfg);
    loadError.value = "";
    refreshError.value = "";
  } catch (e) {
    if (ticket !== loadTicket) return;
    if (silent) {
      refreshError.value = userErrorMessage(e, "内容更新失败，请稍后重试");
    }
    if (!silent) {
      teachers.value = normalizeTeachers();
      coursewareItems.value = normalizeCoursewareItems();
      quotes.value = [];
      loadError.value = userErrorMessage(e, "内容加载失败，请稍后重试");
    }
  } finally {
    if (ticket === loadTicket) {
      if (silent) refreshing.value = false;
      loading.value = false;
    }
  }
}

function retryContentRefresh() {
  return loadContent({ silent: true });
}

function teacherMediaKey(teacher, index) {
  return `${teacher.name || ""}::${teacher.avatar || ""}::${index}`;
}

function courseMediaKey(course, index) {
  return `${course.title || ""}::${course.cover || ""}::${index}`;
}

function markTeacherImageError(key) {
  teacherImageErrors.value = { ...teacherImageErrors.value, [key]: true };
}

function markCourseImageError(key) {
  courseImageErrors.value = { ...courseImageErrors.value, [key]: true };
}

function markTypeImageError(id) {
  typeImageErrors.value = { ...typeImageErrors.value, [id]: true };
}

function classroomPreviewMediaKey(item) {
  return `${item?.contentType || "series"}:${item?.id || ""}`;
}

function markClassroomPreviewCoverError(key) {
  classroomPreviewCoverErrors.value = {
    ...classroomPreviewCoverErrors.value,
    [key]: true,
  };
}

async function loadClassroomPreview() {
  const ticket = ++classroomTicket;
  classroomLoading.value = true;
  classroomWarning.value = "";
  try {
    const [seriesResult, standaloneResult] = await Promise.allSettled([
      listClassroomSeriesApi({ limit: 2, offset: 0 }),
      listClassroomStandaloneApi({ limit: 2, offset: 0 }),
    ]);
    if (ticket !== classroomTicket) return;
    const series =
      seriesResult.status === "fulfilled"
        ? (seriesResult.value?.items || []).map(normalizeClassroomSeries).filter((item) => item.id)
        : [];
    const standalone =
      standaloneResult.status === "fulfilled"
        ? (standaloneResult.value?.items || [])
            .map(normalizeClassroomContent)
            .filter((item) => item.id)
        : [];
    classroomPreview.value = [...series, ...standalone].slice(0, 3);
    classroomPreviewCoverErrors.value = {};
    const failures = [seriesResult, standaloneResult].filter(
      (result) => result.status === "rejected",
    );
    if (failures.length) {
      const fallback =
        failures.length === 2 ? "老师课堂加载失败，请稍后重试" : "部分课堂内容加载失败，请稍后重试";
      classroomWarning.value = userErrorMessage(failures[0].reason, fallback);
    }
  } catch (error) {
    if (ticket === classroomTicket)
      classroomWarning.value = userErrorMessage(error, "老师课堂加载失败，请稍后重试");
  } finally {
    if (ticket === classroomTicket) classroomLoading.value = false;
  }
}

function retryClassroomPreview() {
  return loadClassroomPreview();
}

function openClassroom(tab = "series") {
  uni.navigateTo({ url: `/pages/classroom/classroom?tab=${tab}` });
}

onMounted(() => {
  const hasCachedContent = showStoredContent();
  loadContent({ silent: hasCachedContent });
  loadClassroomPreview();
});

function goTest() {
  uni.switchTab({ url: "/pages/index/index" });
}
</script>

<template>
  <view class="wrap learn page-stack ios-page ios-safe-bottom">
    <view class="learn-hero nx-page-hero">
      <text class="learn-hero__eyebrow">学习中心</text>
      <text class="learn-hero__title">跟着老师，把九型用进生活</text>
      <text class="learn-hero__lead">从理解自己开始，在关系与日常选择中练习更清醒的回应。</text>
    </view>

    <view v-if="refreshError" class="content-refresh-notice" aria-live="polite">
      <text class="content-refresh-notice__text">{{ refreshError }}，当前仍展示上次内容。</text>
      <button class="refresh-retry" :disabled="refreshing" @click="retryContentRefresh">
        {{ refreshing ? "更新中…" : "重试" }}
      </button>
    </view>

    <view class="learn-sections">
      <view class="classroom-entry card ios-card learn-section nx-panel section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">老师课堂</text>
            <text class="sec-title">视频与音频课件</text>
          </view>
          <button class="classroom-entry__more" @click="openClassroom('series')">查看全部</button>
        </view>

        <view
          class="classroom-entry__hero"
          role="button"
          aria-role="button"
          tabindex="0"
          aria-label="进入老师课堂查看视频和音频课件"
          hover-class="classroom-entry__hero--pressed"
          @click="openClassroom('series')"
          @keydown.enter="openClassroom('series')"
          @keydown.space.prevent="openClassroom('series')"
        >
          <view class="classroom-entry__hero-copy">
            <text class="classroom-entry__hero-eyebrow">课堂精选</text>
            <text class="classroom-entry__hero-title">老师以往开课内容，已经整理成可反复学习的课件</text>
            <text class="classroom-entry__hero-lead">支持视频和音频，按系列学习，也可以从独立课件开始。</text>
            <button
              class="classroom-entry__hero-cta"
              hover-class="classroom-entry__hero-cta--pressed"
              @click.stop="openClassroom('series')"
            >
              进入老师课堂
            </button>
          </view>
          <view class="classroom-entry__hero-media" aria-hidden="true">
            <view class="classroom-entry__hero-screen">
              <view class="classroom-entry__hero-play"></view>
            </view>
            <view class="classroom-entry__hero-wave classroom-entry__hero-wave--one"></view>
            <view class="classroom-entry__hero-wave classroom-entry__hero-wave--two"></view>
          </view>
        </view>

        <view v-if="classroomWarning" class="classroom-entry__warning" aria-live="polite">
          <view>
            <text>{{ classroomWarning }}</text>
            <text class="classroom-entry__fallback">已加载的课堂和下方精选课程仍可继续浏览。</text>
          </view>
          <button class="retry" :disabled="classroomLoading" @click="retryClassroomPreview">
            重试课堂内容
          </button>
        </view>
        <view v-if="classroomLoading" class="empty" aria-live="polite">课堂内容加载中…</view>
        <view v-else-if="classroomPreview.length === 0" class="classroom-entry__empty">
          <text>{{
            classroomWarning
              ? "部分课堂内容暂未加载，可重试或继续浏览下方精选课程。"
              : "老师课堂正在准备中，下方精选课程仍可继续浏览。"
          }}</text>
          <button class="classroom-entry__browse" @click="openClassroom('standalone')">
            进入课堂
          </button>
        </view>
        <view v-else class="classroom-entry__grid">
          <view
            v-for="item in classroomPreview"
            :key="`${item.contentType || 'series'}:${item.id}`"
            class="classroom-entry__item"
            role="button"
            aria-role="button"
            tabindex="0"
            :aria-label="`查看${item.title || '老师课堂课件'}`"
            hover-class="classroom-entry__item--pressed"
            @click="openClassroom(item.contentType ? 'standalone' : 'series')"
            @keydown.enter="openClassroom(item.contentType ? 'standalone' : 'series')"
            @keydown.space.prevent="openClassroom(item.contentType ? 'standalone' : 'series')"
          >
            <image
              v-if="item.coverUrl && !classroomPreviewCoverErrors[classroomPreviewMediaKey(item)]"
              class="classroom-entry__cover"
              :class="classroomCoverRatioClass(item)"
              :src="item.coverUrl"
              mode="aspectFill"
              lazy-load
              @error="markClassroomPreviewCoverError(classroomPreviewMediaKey(item))"
            />
            <view
              v-else
              class="classroom-entry__cover classroom-entry__cover--fallback"
              :class="classroomCoverRatioClass(item)"
              aria-hidden="true"
              >课</view
            >
            <view class="classroom-entry__body">
              <text class="classroom-entry__kind">{{
                item.contentType ? (item.contentType === "audio" ? "音频" : "视频") : "系列"
              }}</text>
              <text class="classroom-entry__title">{{ item.title }}</text>
            </view>
          </view>
        </view>
      </view>

      <view class="card ios-card learn-section nx-panel section teacher-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">导师引领</text>
            <text class="sec-title">认识你的学习向导</text>
          </view>
        </view>
        <view v-if="loading" class="empty">老师资料加载中…</view>
        <view v-else-if="loadError" class="empty empty--error">
          <text>{{ loadError }}</text>
          <button class="retry" hover-class="retry--hover" @click="loadContent">重新加载</button>
        </view>
        <view
          v-for="(teacher, teacherIndex) in teachers"
          :key="teacherMediaKey(teacher, teacherIndex)"
          class="teacher-card"
        >
          <image
            v-if="teacher.avatar && !teacherImageErrors[teacherMediaKey(teacher, teacherIndex)]"
            class="teacher-media teacher-card__avatar"
            :src="teacher.avatar"
            mode="aspectFill"
            lazy-load
            @error="markTeacherImageError(teacherMediaKey(teacher, teacherIndex))"
          />
          <view v-else class="teacher-media__fallback" aria-hidden="true">
            {{ teacher.name ? teacher.name.slice(0, 1) : "师" }}
          </view>
          <view class="teacher-card__body">
            <text class="teacher-card__name">{{ teacher.name }}</text>
            <text class="teacher-card__title">{{ teacher.title }}</text>
            <text class="teacher-card__bio">{{ teacher.bio }}</text>
            <view v-if="teacher.tags.length" class="teacher-card__tags">
              <text v-for="tag in teacher.tags" :key="tag" class="nx-tag teacher-card__tag">{{
                tag
              }}</text>
            </view>
          </view>
        </view>
      </view>

      <view class="card ios-card learn-section nx-panel section courseware-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">精选课程资料</text>
            <text class="sec-title">循序建立九型视角</text>
          </view>
        </view>
        <view v-if="loading" class="empty">课程内容加载中…</view>
        <block v-else>
          <view
            v-for="(c, i) in coursewareItems"
            :key="courseMediaKey(c, i)"
            class="courseware-card"
          >
            <image
              v-if="c.cover && !courseImageErrors[courseMediaKey(c, i)]"
              class="course-media courseware-card__cover"
              :src="c.cover"
              mode="aspectFill"
              lazy-load
              @error="markCourseImageError(courseMediaKey(c, i))"
            />
            <view v-else class="course-media__fallback" aria-hidden="true">{{
              c.badge || i + 1
            }}</view>
            <view class="courseware-card__body">
              <view class="courseware-card__meta">
                <text class="nx-tag courseware-card__badge">{{ c.badge || `第 ${i + 1} 课` }}</text>
                <text v-if="c.duration" class="courseware-card__duration">{{ c.duration }}</text>
              </view>
              <text class="courseware-card__title">{{ c.title }}</text>
              <text class="courseware-card__desc">{{ c.description }}</text>
            </view>
          </view>
        </block>
      </view>

      <view class="card ios-card learn-section nx-panel section quote-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">今日一念</text>
            <text class="sec-title">把觉察带回当下</text>
          </view>
        </view>
        <view v-if="loading" class="empty">语录内容加载中…</view>
        <view v-else-if="!loadError && quotes.length === 0" class="empty">语录内容即将上线</view>
        <view v-for="quote in quotes" :key="quote" class="quote-editorial">
          <text class="quote-editorial__mark" aria-hidden="true">“</text>
          <text class="quote-editorial__text">{{ quote }}</text>
        </view>
      </view>

      <view class="card ios-card learn-section nx-panel section type-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">九型图鉴</text>
            <text class="sec-title">九种性格，九条成长路径</text>
          </view>
        </view>
        <view class="type-badge-grid">
          <view v-for="t in types" :key="t.id" class="type-badge" :class="'type-badge--' + t.color">
            <view class="type-badge__media">
              <image
                v-if="!typeImageErrors[t.id]"
                class="type-badge__avatar"
                :src="`/static/avatars/${t.id}.png`"
                mode="aspectFill"
                lazy-load
                @error="markTypeImageError(t.id)"
              />
              <view v-else class="type-badge__fallback" aria-hidden="true">{{ t.id }}</view>
              <text class="type-badge__num">{{ t.id }}</text>
            </view>
            <text class="type-badge__name">{{ t.name }}</text>
            <text class="type-badge__keywords">{{ t.keywords }}</text>
          </view>
        </view>
      </view>
    </view>

    <button
      class="btn-primary ios-button learn-cta"
      hover-class="learn-cta--pressed"
      @click="goTest"
    >
      先完成测试，建立你的学习地图
    </button>
  </view>
</template>

<style scoped>
.learn {
  background:
    radial-gradient(circle at 0 0, rgba(79, 70, 229, 0.10), transparent 30%),
    radial-gradient(circle at 100% 14%, rgba(245, 158, 11, 0.10), transparent 28%),
    #f8fafc;
}
.learn-hero {
  padding: 38rpx 34rpx 40rpx;
  border-radius: 38rpx;
  background:
    radial-gradient(circle at 86% 10%, rgba(255, 255, 255, 0.18), transparent 24%),
    linear-gradient(135deg, #4338ca 0%, #7c3aed 100%);
  color: #ffffff;
  box-shadow: 0 24rpx 54rpx -34rpx rgba(67, 56, 202, 0.66);
}
.learn-hero__eyebrow {
  display: block;
  color: #ffffff;
  font-size: 24rpx;
  font-weight: 800;
}
.learn-hero__title {
  display: block;
  margin-top: 14rpx;
  color: #ffffff;
  font-size: 44rpx;
  font-weight: 900;
  line-height: 1.28;
}
.learn-hero__lead {
  display: block;
  margin-top: 16rpx;
  color: #ffffff;
  font-size: 26rpx;
  line-height: 1.65;
}
.content-refresh-notice {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  padding: 20rpx 24rpx;
  color: #475569;
  background: #eef2ff;
  border: 2rpx solid #c7d2fe;
  border-radius: 24rpx;
}
.content-refresh-notice__text {
  flex: 1;
  font-size: 24rpx;
  line-height: 1.55;
}
.refresh-retry {
  flex-shrink: 0;
  min-height: 88rpx;
  padding: 0 22rpx;
  color: #4338ca;
  font-size: 24rpx;
  font-weight: 900;
  background: #ffffff;
  border: 2rpx solid #a5b4fc;
  border-radius: 16rpx;
  line-height: 88rpx;
}
.refresh-retry::after {
  border: none;
}
.refresh-retry[disabled] {
  opacity: 0.6;
}
.classroom-entry__more,
.classroom-entry__browse {
  min-height: 88rpx;
  padding: 0 24rpx;
  color: #4338ca;
  font-size: 24rpx;
  font-weight: 800;
  line-height: 88rpx;
  background: #eef2ff;
  border-radius: 18rpx;
}
.classroom-entry__more::after,
.classroom-entry__browse::after,
.classroom-entry__hero-cta::after {
  border: 0;
}
.classroom-entry__hero {
  position: relative;
  display: flex;
  align-items: stretch;
  gap: 22rpx;
  margin: 24rpx 0 22rpx;
  padding: 26rpx;
  overflow: hidden;
  color: #ffffff;
  background:
    radial-gradient(circle at 14% 8%, rgba(255, 255, 255, 0.18), transparent 28%),
    linear-gradient(135deg, #0f172a 0%, #4338ca 58%, #f59e0b 132%);
  border-radius: 26rpx;
  box-shadow: 0 22rpx 42rpx -28rpx rgba(15, 23, 42, 0.56);
}
.classroom-entry__hero--pressed {
  opacity: 0.88;
}
.classroom-entry__hero-copy {
  position: relative;
  z-index: 1;
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 9rpx;
}
.classroom-entry__hero-eyebrow,
.classroom-entry__hero-title,
.classroom-entry__hero-lead {
  display: block;
}
.classroom-entry__hero-eyebrow {
  color: rgba(255, 255, 255, 0.82);
  font-size: 22rpx;
  font-weight: 900;
}
.classroom-entry__hero-title {
  color: #ffffff;
  font-size: 30rpx;
  font-weight: 900;
  line-height: 1.36;
}
.classroom-entry__hero-lead {
  color: rgba(255, 255, 255, 0.78);
  font-size: 23rpx;
  line-height: 1.55;
}
.classroom-entry__hero-cta {
  align-self: flex-start;
  min-height: 88rpx;
  margin-top: 8rpx;
  padding: 0 24rpx;
  color: #0f172a;
  font-size: 24rpx;
  font-weight: 900;
  line-height: 88rpx;
  background: rgba(255, 255, 255, 0.96);
  border-radius: 999rpx;
}
.classroom-entry__hero-cta--pressed {
  opacity: 0.82;
}
.classroom-entry__hero-media {
  position: relative;
  flex: 0 0 150rpx;
  width: 150rpx;
  min-height: 158rpx;
}
.classroom-entry__hero-screen {
  position: absolute;
  top: 18rpx;
  right: 0;
  width: 140rpx;
  height: 104rpx;
  border: 2rpx solid rgba(255, 255, 255, 0.2);
  border-radius: 24rpx;
  background: linear-gradient(145deg, rgba(255, 255, 255, 0.18), rgba(255, 255, 255, 0.06));
  box-shadow: 0 20rpx 34rpx -24rpx rgba(15, 23, 42, 0.5);
}
.classroom-entry__hero-play {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 0;
  height: 0;
  border-top: 16rpx solid transparent;
  border-bottom: 16rpx solid transparent;
  border-left: 26rpx solid #ffffff;
  transform: translate(-35%, -50%);
}
.classroom-entry__hero-wave {
  position: absolute;
  height: 10rpx;
  border-radius: 999rpx;
  background: rgba(255, 255, 255, 0.56);
}
.classroom-entry__hero-wave--one {
  right: 18rpx;
  bottom: 28rpx;
  width: 94rpx;
}
.classroom-entry__hero-wave--two {
  right: 46rpx;
  bottom: 50rpx;
  width: 58rpx;
  opacity: 0.55;
}
.classroom-entry__warning {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  margin-bottom: 18rpx;
  padding: 18rpx 22rpx;
  color: #c2410c;
  font-size: 24rpx;
  line-height: 1.55;
  background: #fff7ed;
  border: 2rpx solid #fed7aa;
  border-radius: 20rpx;
}
.classroom-entry__fallback {
  display: block;
  margin-top: 10rpx;
  color: #64748b;
}
.classroom-entry__empty {
  color: #64748b;
  font-size: 25rpx;
  line-height: 1.6;
  text-align: center;
}
.classroom-entry__browse {
  display: block;
  margin: 20rpx auto 0;
}
.classroom-entry__grid {
  display: grid;
  gap: 18rpx;
}
.classroom-entry__item {
  display: flex;
  align-items: flex-start;
  overflow: hidden;
  background: #f8fafc;
  border: 2rpx solid #e0e7ff;
  border-radius: 22rpx;
  box-shadow: 0 14rpx 28rpx -24rpx rgba(67, 56, 202, 0.42);
  transition: opacity 0.18s ease, transform 0.18s ease;
}
.classroom-entry__item--pressed {
  opacity: 0.82;
  transform: scale(0.988);
}
.classroom-entry__cover {
  flex-shrink: 0;
  width: 156rpx;
  background: #e0e7ff;
}
.classroom-entry__cover.classroom-cover--16x9 {
  height: 88rpx;
}
.classroom-entry__cover.classroom-cover--9x16 {
  height: 277rpx;
}
.classroom-entry__cover.classroom-cover--1x1 {
  height: 156rpx;
}
.classroom-entry__cover--fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #4338ca;
  font-size: 40rpx;
  font-weight: 900;
}
.classroom-entry__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  justify-content: center;
  min-width: 0;
  padding: 16rpx 20rpx;
}
.classroom-entry__kind {
  color: #4338ca;
  font-size: 21rpx;
  font-weight: 800;
}
.classroom-entry__title {
  margin-top: 8rpx;
  color: #111827;
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.4;
}
.learn-sections {
  display: flex;
  flex-direction: column;
  gap: 22rpx;
}
.learn-section {
  display: flex;
  flex-direction: column;
  padding: 30rpx;
}
.section-kicker {
  display: block;
  color: #4338ca;
  font-size: 24rpx;
  font-weight: 800;
}
.sec-title {
  display: block;
  margin-top: 8rpx;
  color: #0f172a;
  font-size: 34rpx;
  font-weight: 900;
  line-height: 1.35;
}
.empty {
  color: #475569;
  font-size: 26rpx;
  line-height: 1.6;
  padding: 24rpx 0;
}
.empty--error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
}
.retry {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 88rpx;
  min-height: 88rpx;
  padding: 0 20rpx;
  color: #4338ca;
  font-size: 24rpx;
  font-weight: 900;
  touch-action: manipulation;
  background: #eef2ff;
  border: 2rpx solid #c4b5fd;
  border-radius: 18rpx;
  line-height: 1;
}
.retry::after {
  border: none;
}
.retry--hover {
  opacity: 0.82;
  transform: scale(0.985);
}
.teacher-card {
  display: flex;
  align-items: flex-start;
  gap: 22rpx;
  margin-top: 24rpx;
}
.teacher-media,
.teacher-media__fallback {
  flex-shrink: 0;
  width: 112rpx;
  height: 112rpx;
  border-radius: 28rpx;
  box-sizing: border-box;
}
.teacher-media {
  width: 112rpx;
  height: 112rpx;
  background: #e0e7ff;
}
.teacher-media__fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 112rpx;
  height: 112rpx;
  color: #ffffff;
  background: #4338ca;
  font-size: 40rpx;
  font-weight: 900;
}
.teacher-card__body {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}
.teacher-card__name {
  color: #0f172a;
  font-size: 32rpx;
  font-weight: 900;
  line-height: 1.3;
}
.teacher-card__title {
  color: #4338ca;
  font-size: 24rpx;
  font-weight: 800;
}
.teacher-card__bio {
  color: #334155;
  font-size: 25rpx;
  line-height: 1.65;
}
.teacher-card__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 10rpx;
  margin-top: 4rpx;
}
.teacher-card__tag {
  color: #4338ca;
  background: #eef2ff;
  font-size: 24rpx;
}
.courseware-card {
  display: flex;
  align-items: flex-start;
  gap: 18rpx;
  padding: 24rpx 0;
  border-bottom: 2rpx solid #e2e8f0;
}
.courseware-card:last-child {
  padding-bottom: 0;
  border-bottom: none;
}
.course-media,
.course-media__fallback {
  flex-shrink: 0;
  width: 220rpx;
  height: 150rpx;
  border-radius: 20rpx;
  box-sizing: border-box;
}
.course-media {
  width: 220rpx;
  height: 150rpx;
  background: #e0e7ff;
}
.course-media__fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 220rpx;
  height: 150rpx;
  padding: 16rpx;
  color: #ffffff;
  background: #7c3aed;
  font-size: 28rpx;
  font-weight: 900;
  text-align: center;
}
.courseware-card__body {
  min-width: 0;
  flex: 1;
}
.courseware-card__meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10rpx;
  margin-bottom: 8rpx;
}
.courseware-card__badge {
  color: #4338ca;
  background: #eef2ff;
  font-size: 24rpx;
}
.courseware-card__duration {
  color: #475569;
  font-size: 24rpx;
  font-weight: 700;
}
.courseware-card__title {
  display: block;
  color: #0f172a;
  font-size: 30rpx;
  font-weight: 900;
  line-height: 1.35;
}
.courseware-card__desc {
  display: block;
  margin-top: 8rpx;
  color: #334155;
  font-size: 24rpx;
  line-height: 1.6;
}
.quote-editorial {
  position: relative;
  margin-top: 20rpx;
  padding: 30rpx;
  border-radius: 22rpx;
  background: #eef2ff;
  overflow: hidden;
}
.quote-editorial__mark {
  display: block;
  color: #4338ca;
  font-family: Georgia, serif;
  font-size: 54rpx;
  font-weight: 900;
  line-height: 1;
}
.quote-editorial__text {
  position: relative;
  display: block;
  margin-top: 8rpx;
  color: #334155;
  font-size: 28rpx;
  font-weight: 700;
  line-height: 1.7;
}
.type-badge-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16rpx;
  margin-top: 24rpx;
}
.type-badge {
  min-width: 0;
  min-height: 190rpx;
  padding: 20rpx 14rpx;
  border: 2rpx solid #e2e8f0;
  border-radius: 20rpx;
  background: #ffffff;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}
.type-badge__media {
  position: relative;
}
.type-badge__avatar,
.type-badge__fallback {
  width: 78rpx;
  height: 78rpx;
  border-radius: 50%;
  box-sizing: border-box;
}
.type-badge__avatar {
  width: 78rpx;
  height: 78rpx;
  background: #e0e7ff;
}
.type-badge__fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 78rpx;
  height: 78rpx;
  color: #ffffff;
  background: #4338ca;
  font-size: 28rpx;
  font-weight: 900;
}
.type-badge__num {
  position: absolute;
  right: -12rpx;
  bottom: -4rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36rpx;
  height: 36rpx;
  border-radius: 12rpx;
  color: #ffffff;
  background: #4338ca;
  font-size: 24rpx;
  font-weight: 900;
}
.type-badge--blue .type-badge__num {
  background: #2563eb;
}
.type-badge--red .type-badge__num {
  background: #dc2626;
}
.type-badge__name {
  margin-top: 14rpx;
  color: #0f172a;
  font-size: 27rpx;
  font-weight: 900;
  text-align: center;
}
.type-badge__keywords {
  margin-top: 6rpx;
  color: #475569;
  font-size: 24rpx;
  line-height: 1.45;
  text-align: center;
  overflow-wrap: anywhere;
}
.learn-cta {
  min-height: 88rpx;
  margin-top: 4rpx;
  font-size: 28rpx;
  touch-action: manipulation;
}
.learn-cta--pressed {
  opacity: 0.86;
  transform: scale(0.99);
}

@media (min-width: 768px) {
  .type-badge-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
</style>
