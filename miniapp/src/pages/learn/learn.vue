<script setup>
import { ref, onMounted } from "vue";
import NxAsyncState from "../../components/NxAsyncState.vue";
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
    classroomPreview.value = [...standalone, ...series].slice(0, 3);
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

function openClassroom(tab = "standalone") {
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
      <text class="learn-hero__eyebrow">老师课堂</text>
      <text class="learn-hero__title">跟着老师，把九型真正用进工作与生活</text>
      <text class="learn-hero__lead">从视频与音频课件开始，理解自己、改善关系，也为团队协作建立更清晰的共同语言。</text>
      <view class="learn-hero__meta" aria-hidden="true">
        <text>视频课程</text>
        <text>音频精讲</text>
        <text>九型实践</text>
      </view>
    </view>

    <view v-if="refreshError" class="content-refresh-notice" aria-live="polite">
      <NxAsyncState
        state="stale"
        :title="refreshError"
        description="当前仍展示上次内容，可继续浏览。"
        action-text="重新更新"
        :busy="refreshing"
        @action="retryContentRefresh"
      />
    </view>

    <view class="learn-sections">
      <view class="classroom-entry card ios-card learn-section nx-panel section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">课堂精选</text>
            <text class="sec-title">视频与音频课件</text>
          </view>
          <button class="classroom-entry__more" @click="openClassroom('standalone')">查看全部</button>
        </view>

        <view class="classroom-entry__hero">
          <view class="classroom-entry__hero-copy">
            <text class="classroom-entry__hero-eyebrow">随时回看 · 反复练习</text>
            <text class="classroom-entry__hero-title">把老师以往开课内容，整理成可以持续学习的专业课件</text>
            <text class="classroom-entry__hero-lead">支持视频和音频；先看独立课件，也可以进入系列课程循序学习。</text>
            <button
              class="classroom-entry__hero-cta"
              hover-class="classroom-entry__hero-cta--pressed"
              @click="openClassroom('standalone')"
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

        <view
          v-if="classroomWarning && classroomPreview.length > 0"
          class="classroom-entry__warning"
          aria-live="polite"
        >
          <view>
            <text>{{ classroomWarning }}</text>
            <text class="classroom-entry__fallback">已加载的课堂内容仍可继续浏览。</text>
          </view>
          <button class="retry" :disabled="classroomLoading" @click="retryClassroomPreview">
            重试课堂内容
          </button>
        </view>
        <NxAsyncState v-if="classroomLoading" state="loading" />
        <NxAsyncState
          v-else-if="classroomWarning && classroomPreview.length === 0"
          state="error"
          title="课堂内容暂未加载"
          :description="classroomWarning"
          action-text="重新加载"
          :busy="classroomLoading"
          @action="retryClassroomPreview"
        />
        <NxAsyncState
          v-else-if="classroomPreview.length === 0"
          state="empty"
          title="老师课堂正在准备中"
          description="可以先浏览老师介绍和课程方向，新的视频与音频课件会在这里持续更新。"
          action-text="进入课堂看看"
          @action="openClassroom('standalone')"
        />
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
              <text class="classroom-entry__action">开始学习 ›</text>
            </view>
          </view>
        </view>
      </view>

      <view class="card ios-card learn-section nx-panel section teacher-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">老师简介</text>
            <text class="sec-title">认识你的学习向导</text>
          </view>
        </view>
        <NxAsyncState v-if="loading" state="loading" />
        <NxAsyncState
          v-else-if="loadError"
          state="error"
          title="老师资料暂未加载"
          :description="loadError"
          action-text="重新加载"
          @action="loadContent"
        />
        <block v-else>
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
        </block>
      </view>

      <view class="card ios-card learn-section nx-panel section courseware-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">课程方向</text>
            <text class="sec-title">循序建立九型视角</text>
          </view>
        </view>
        <NxAsyncState v-if="loading" state="loading" />
        <NxAsyncState
          v-else-if="coursewareItems.length === 0"
          state="empty"
          title="课程方向正在整理中"
          description="更多面向个人成长、关系沟通与企业团队的学习主题会持续补充。"
        />
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

      <view class="card ios-card learn-section nx-panel section type-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">九型内容</text>
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

      <view class="card ios-card learn-section nx-panel section quote-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">课堂一念</text>
            <text class="sec-title">把觉察带回当下</text>
          </view>
        </view>
        <NxAsyncState v-if="loading" state="loading" />
        <NxAsyncState
          v-else-if="!loadError && quotes.length === 0"
          state="empty"
          title="课堂语录即将上线"
        />
        <view v-for="quote in quotes" :key="quote" class="quote-editorial">
          <text class="quote-editorial__mark" aria-hidden="true">“</text>
          <text class="quote-editorial__text">{{ quote }}</text>
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
  min-height: 100vh;
  background:
    radial-gradient(circle at 0 0, rgba(223, 188, 127, 0.18), transparent 30%),
    linear-gradient(180deg, var(--nx-surface-soft), var(--nx-page-bg));
}
.learn-hero {
  position: relative;
  overflow: hidden;
  padding: 40rpx 34rpx 36rpx;
  color: var(--nx-surface);
  background:
    radial-gradient(circle at 88% 10%, rgba(223, 188, 127, 0.34), transparent 28%),
    linear-gradient(135deg, var(--nx-brand-900), var(--nx-brand-700));
  border-radius: 38rpx;
  box-shadow: 0 24rpx 52rpx -34rpx rgba(32, 42, 55, 0.62);
}
.learn-hero__eyebrow,
.learn-hero__title,
.learn-hero__lead { display: block; }
.learn-hero__eyebrow {
  color: var(--nx-accent-gold);
  font-size: 24rpx;
  font-weight: 900;
  letter-spacing: 4rpx;
}
.learn-hero__title {
  max-width: 590rpx;
  margin-top: 14rpx;
  color: var(--nx-surface);
  font-size: 44rpx;
  font-weight: 900;
  line-height: 1.28;
}
.learn-hero__lead {
  max-width: 590rpx;
  margin-top: 16rpx;
  color: rgba(255, 255, 255, 0.82);
  font-size: 25rpx;
  line-height: 1.68;
}
.learn-hero__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12rpx;
  margin-top: 24rpx;
}
.learn-hero__meta text {
  padding: 8rpx 16rpx;
  color: var(--nx-surface);
  font-size: 21rpx;
  font-weight: 800;
  background: rgba(255, 255, 255, 0.12);
  border: 2rpx solid rgba(255, 255, 255, 0.18);
  border-radius: 999rpx;
}
.content-refresh-notice {
  overflow: hidden;
  border: 2rpx solid var(--nx-border);
  border-radius: 24rpx;
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
  color: var(--nx-brand-700);
  font-size: 23rpx;
  font-weight: 900;
  letter-spacing: 2rpx;
}
.sec-title {
  display: block;
  margin-top: 8rpx;
  color: var(--nx-text);
  font-size: 34rpx;
  font-weight: 900;
  line-height: 1.35;
}
.classroom-entry__more,
.classroom-entry__hero-cta {
  min-height: 88rpx;
  padding: 0 24rpx;
  font-size: 24rpx;
  font-weight: 900;
  line-height: 88rpx;
  border-radius: 18rpx;
}
.classroom-entry__more {
  color: var(--nx-brand-900);
  background: var(--nx-surface-soft);
  border: 2rpx solid var(--nx-border);
}
.classroom-entry__more::after,
.classroom-entry__hero-cta::after,
.retry::after { border: 0; }
.classroom-entry__hero {
  position: relative;
  display: flex;
  align-items: stretch;
  gap: 22rpx;
  margin: 24rpx 0 22rpx;
  padding: 28rpx;
  overflow: hidden;
  color: var(--nx-surface);
  background:
    radial-gradient(circle at 14% 8%, rgba(223, 188, 127, 0.22), transparent 30%),
    linear-gradient(135deg, var(--nx-brand-900), var(--nx-brand-700));
  border-radius: 28rpx;
  box-shadow: 0 22rpx 42rpx -30rpx rgba(32, 42, 55, 0.58);
}
.classroom-entry__hero-copy {
  position: relative;
  z-index: 1;
  display: flex;
  flex: 1;
  flex-direction: column;
  justify-content: center;
  min-width: 0;
  gap: 9rpx;
}
.classroom-entry__hero-eyebrow,
.classroom-entry__hero-title,
.classroom-entry__hero-lead { display: block; }
.classroom-entry__hero-eyebrow {
  color: var(--nx-accent-gold);
  font-size: 22rpx;
  font-weight: 900;
}
.classroom-entry__hero-title {
  color: var(--nx-surface);
  font-size: 30rpx;
  font-weight: 900;
  line-height: 1.4;
}
.classroom-entry__hero-lead {
  color: rgba(255, 255, 255, 0.76);
  font-size: 23rpx;
  line-height: 1.58;
}
.classroom-entry__hero-cta {
  align-self: flex-start;
  margin-top: 10rpx;
  color: var(--nx-brand-900);
  background: var(--nx-accent-gold);
  border-radius: 999rpx;
}
.classroom-entry__hero-cta--pressed { opacity: 0.84; }
.classroom-entry__hero-media {
  position: relative;
  flex: 0 0 150rpx;
  width: 150rpx;
  min-height: 176rpx;
}
.classroom-entry__hero-screen {
  position: absolute;
  top: 18rpx;
  right: 0;
  width: 140rpx;
  height: 104rpx;
  background: rgba(255, 255, 255, 0.1);
  border: 2rpx solid rgba(255, 255, 255, 0.2);
  border-radius: 24rpx;
}
.classroom-entry__hero-play {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 0;
  height: 0;
  border-top: 16rpx solid transparent;
  border-bottom: 16rpx solid transparent;
  border-left: 26rpx solid var(--nx-accent-gold);
  transform: translate(-35%, -50%);
}
.classroom-entry__hero-wave {
  position: absolute;
  height: 10rpx;
  background: var(--nx-accent-gold);
  border-radius: 999rpx;
}
.classroom-entry__hero-wave--one { right: 18rpx; bottom: 30rpx; width: 94rpx; }
.classroom-entry__hero-wave--two { right: 46rpx; bottom: 52rpx; width: 58rpx; opacity: 0.55; }
.classroom-entry__warning {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  margin-bottom: 18rpx;
  padding: 18rpx 22rpx;
  color: var(--nx-brand-900);
  font-size: 24rpx;
  line-height: 1.55;
  background: var(--nx-surface-soft);
  border: 2rpx solid var(--nx-accent-gold);
  border-radius: 20rpx;
}
.classroom-entry__fallback {
  display: block;
  margin-top: 10rpx;
  color: var(--nx-text-muted);
}
.retry {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 88rpx;
  min-height: 88rpx;
  padding: 0 20rpx;
  color: var(--nx-brand-900);
  font-size: 24rpx;
  font-weight: 900;
  background: var(--nx-surface);
  border: 2rpx solid var(--nx-border);
  border-radius: 18rpx;
}
.classroom-entry__grid {
  display: grid;
  gap: 18rpx;
}
.classroom-entry__item {
  display: flex;
  align-items: stretch;
  min-height: 156rpx;
  overflow: hidden;
  background: var(--nx-surface-soft);
  border: 2rpx solid var(--nx-border);
  border-radius: 22rpx;
  box-shadow: 0 14rpx 28rpx -26rpx rgba(32, 42, 55, 0.46);
}
.classroom-entry__item--pressed { opacity: 0.84; transform: scale(0.988); }
.classroom-entry__cover {
  flex-shrink: 0;
  width: 156rpx;
  background: var(--nx-border);
}
.classroom-entry__cover.classroom-cover--16x9 { height: 88rpx; }
.classroom-entry__cover.classroom-cover--9x16 { height: 277rpx; }
.classroom-entry__cover.classroom-cover--1x1 { height: 156rpx; }
.classroom-entry__cover--fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--nx-brand-900);
  font-size: 40rpx;
  font-weight: 900;
  background: linear-gradient(135deg, var(--nx-surface-soft), var(--nx-accent-gold));
}
.classroom-entry__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  justify-content: center;
  min-width: 0;
  padding: 18rpx 20rpx;
}
.classroom-entry__kind,
.classroom-entry__action {
  color: var(--nx-brand-700);
  font-size: 21rpx;
  font-weight: 900;
}
.classroom-entry__title {
  margin-top: 8rpx;
  color: var(--nx-text);
  font-size: 26rpx;
  font-weight: 900;
  line-height: 1.4;
}
.classroom-entry__action { margin-top: 12rpx; color: var(--nx-brand-900); }
.teacher-card {
  display: flex;
  align-items: flex-start;
  gap: 22rpx;
  margin-top: 24rpx;
  padding: 22rpx;
  background: var(--nx-surface-soft);
  border: 2rpx solid var(--nx-border);
  border-radius: 24rpx;
}
.teacher-media,
.teacher-media__fallback {
  flex-shrink: 0;
  width: 116rpx;
  height: 116rpx;
  border-radius: 28rpx;
  box-sizing: border-box;
}
.teacher-media { background: var(--nx-border); }
.teacher-media__fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--nx-surface);
  background: linear-gradient(135deg, var(--nx-brand-900), var(--nx-brand-700));
  font-size: 40rpx;
  font-weight: 900;
}
.teacher-card__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
  gap: 8rpx;
}
.teacher-card__name { color: var(--nx-text); font-size: 32rpx; font-weight: 900; line-height: 1.3; }
.teacher-card__title { color: var(--nx-brand-700); font-size: 24rpx; font-weight: 900; }
.teacher-card__bio { color: var(--nx-text-muted); font-size: 25rpx; line-height: 1.65; }
.teacher-card__tags { display: flex; flex-wrap: wrap; gap: 10rpx; margin-top: 4rpx; }
.teacher-card__tag,
.courseware-card__badge { color: var(--nx-brand-900); background: var(--nx-surface); font-size: 23rpx; }
.courseware-card {
  display: flex;
  align-items: flex-start;
  gap: 18rpx;
  padding: 24rpx 0;
  border-bottom: 2rpx solid var(--nx-border);
}
.courseware-card:last-child { padding-bottom: 0; border-bottom: 0; }
.course-media,
.course-media__fallback {
  flex-shrink: 0;
  width: 220rpx;
  height: 150rpx;
  border-radius: 20rpx;
  box-sizing: border-box;
}
.course-media { background: var(--nx-border); }
.course-media__fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16rpx;
  color: var(--nx-brand-900);
  background: linear-gradient(135deg, var(--nx-accent-gold), var(--nx-surface-soft));
  font-size: 28rpx;
  font-weight: 900;
  text-align: center;
}
.courseware-card__body { flex: 1; min-width: 0; }
.courseware-card__meta { display: flex; align-items: center; flex-wrap: wrap; gap: 10rpx; margin-bottom: 8rpx; }
.courseware-card__duration { color: var(--nx-text-muted); font-size: 23rpx; font-weight: 700; }
.courseware-card__title { display: block; color: var(--nx-text); font-size: 30rpx; font-weight: 900; line-height: 1.35; }
.courseware-card__desc { display: block; margin-top: 8rpx; color: var(--nx-text-muted); font-size: 24rpx; line-height: 1.6; }
.type-badge-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16rpx;
  margin-top: 24rpx;
}
.type-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  min-width: 0;
  min-height: 190rpx;
  padding: 20rpx 14rpx;
  background: var(--nx-surface-soft);
  border: 2rpx solid var(--nx-border);
  border-radius: 20rpx;
  box-sizing: border-box;
}
.type-badge__media { position: relative; }
.type-badge__avatar,
.type-badge__fallback { width: 78rpx; height: 78rpx; border-radius: 50%; box-sizing: border-box; }
.type-badge__avatar { background: var(--nx-border); }
.type-badge__fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--nx-surface);
  background: var(--nx-brand-700);
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
  color: var(--nx-brand-900);
  background: var(--nx-accent-gold);
  border-radius: 12rpx;
  font-size: 24rpx;
  font-weight: 900;
}
.type-badge__name { margin-top: 14rpx; color: var(--nx-text); font-size: 27rpx; font-weight: 900; text-align: center; }
.type-badge__keywords { margin-top: 6rpx; color: var(--nx-text-muted); font-size: 24rpx; line-height: 1.45; text-align: center; overflow-wrap: anywhere; }
.quote-editorial {
  position: relative;
  margin-top: 20rpx;
  padding: 30rpx;
  overflow: hidden;
  background: var(--nx-surface-soft);
  border-left: 8rpx solid var(--nx-accent-gold);
  border-radius: 22rpx;
}
.quote-editorial__mark {
  display: block;
  color: var(--nx-accent-gold);
  font-family: Georgia, serif;
  font-size: 54rpx;
  font-weight: 900;
  line-height: 1;
}
.quote-editorial__text {
  position: relative;
  display: block;
  margin-top: 8rpx;
  color: var(--nx-text);
  font-size: 28rpx;
  font-weight: 700;
  line-height: 1.7;
}
.learn-cta { min-height: 88rpx; margin-top: 4rpx; font-size: 28rpx; touch-action: manipulation; }
.learn-cta--pressed { opacity: 0.86; transform: scale(0.99); }
@media (min-width: 768px) {
  .type-badge-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
</style>
