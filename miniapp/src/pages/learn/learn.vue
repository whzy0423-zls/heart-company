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
import { normalizeMiniappLearn } from "../../utils/miniappPages";
import { listClassroomSeriesApi, listClassroomStandaloneApi } from "../../api";
import {
  classroomCoverRatioClass,
  normalizeClassroomContent,
  normalizeClassroomSeries,
} from "../../utils/classroomDisplay";

const teachers = ref([]);
const coursewareItems = ref([]);
const quotes = ref([]);
const learnCopy = ref(normalizeMiniappLearn());
const types = ref(Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] })));
const teacherImageErrors = ref({});
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
  learnCopy.value = normalizeMiniappLearn(cfg);
  teachers.value = normalizeTeachers(cfg);
  coursewareItems.value = normalizeCoursewareItems(cfg);
  quotes.value = cfg?.home?.quotes?.items || [];
  teacherImageErrors.value = {};
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
      learnCopy.value = normalizeMiniappLearn();
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

function markTeacherImageError(key) {
  teacherImageErrors.value = { ...teacherImageErrors.value, [key]: true };
}

function previewTeacherAvatar(source) {
  const avatar = String(source || "").trim();
  if (!avatar) return;
  uni.previewImage({
    current: avatar,
    urls: [avatar],
  });
}

function markTypeImageError(id) {
  typeImageErrors.value = { ...typeImageErrors.value, [id]: true };
}

function classroomPreviewMediaKey(item) {
  return `${item?.contentType || "series"}:${item?.id || ""}`;
}

function classroomPreviewPresentation(item = {}) {
  const isSeries = !item.contentType;
  const isAudio = item.contentType === "audio";
  return {
    kind: isSeries ? "系列" : isAudio ? "音频" : "视频",
    fallback: isSeries ? "系" : isAudio ? "音" : "视",
    action: isSeries ? "查看系列 ›" : "查看课件 ›",
    tab: isSeries ? "series" : "standalone",
    title:
      String(item.title || "").trim() || (isSeries ? "未命名系列" : "未命名课件"),
  };
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
      <text class="learn-hero__eyebrow">{{ learnCopy.hero.eyebrow }}</text>
      <text class="learn-hero__title">{{ learnCopy.hero.title }}</text>
      <text class="learn-hero__lead">{{ learnCopy.hero.lead }}</text>
      <view class="learn-hero__meta" aria-hidden="true">
        <text v-for="(item, index) in learnCopy.hero.meta" :key="`${item}-${index}`">{{ item }}</text>
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
        <view class="nx-section-head classroom-entry__head">
          <view>
            <text class="section-kicker">{{ learnCopy.classroom.eyebrow }}</text>
            <text class="sec-title">{{ learnCopy.classroom.title }}</text>
          </view>
          <button
            class="classroom-entry__more"
            :aria-label="learnCopy.classroom.ctaText"
            @click="openClassroom('standalone')"
          >
            {{ learnCopy.classroom.moreText }}
            <text aria-hidden="true">›</text>
          </button>
        </view>

        <view class="classroom-entry__intro">
          <text class="classroom-entry__intro-eyebrow">{{ learnCopy.classroom.heroEyebrow }}</text>
          <text class="classroom-entry__intro-title">{{ learnCopy.classroom.heroTitle }}</text>
          <text class="classroom-entry__intro-lead">{{ learnCopy.classroom.heroLead }}</text>
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
          :title="learnCopy.classroom.emptyTitle"
          :description="learnCopy.classroom.emptyDescription"
          :action-text="learnCopy.classroom.emptyActionText"
          @action="openClassroom('standalone')"
        />
        <view v-else class="classroom-entry__grid">
          <view
            v-for="item in classroomPreview"
            :key="`${item.contentType || 'series'}:${item.id}`"
            class="classroom-entry__item"
            role="button"
            tabindex="0"
            :aria-label="`查看${classroomPreviewPresentation(item).kind}课件：${classroomPreviewPresentation(item).title}`"
            hover-class="classroom-entry__item--pressed"
            @click="openClassroom(classroomPreviewPresentation(item).tab)"
            @keydown.enter="openClassroom(classroomPreviewPresentation(item).tab)"
            @keydown.space.prevent="openClassroom(classroomPreviewPresentation(item).tab)"
          >
            <view class="classroom-entry__media">
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
                >{{ classroomPreviewPresentation(item).fallback }}</view
              >
            </view>
            <view class="classroom-entry__body">
              <view class="classroom-entry__meta">
                <text class="classroom-entry__kind">{{ classroomPreviewPresentation(item).kind }}课件</text>
                <text v-if="item.teacherName" class="classroom-entry__teacher">{{ item.teacherName }}</text>
              </view>
              <text class="classroom-entry__title">{{ classroomPreviewPresentation(item).title }}</text>
              <text class="classroom-entry__action">
                {{ classroomPreviewPresentation(item).action }}
              </text>
            </view>
          </view>
        </view>
      </view>

      <view class="card ios-card learn-section nx-panel section teacher-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">{{ learnCopy.sections.teacher.eyebrow }}</text>
            <text class="sec-title">{{ learnCopy.sections.teacher.title }}</text>
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
            <view class="teacher-card__header">
              <button
                v-if="teacher.avatar && !teacherImageErrors[teacherMediaKey(teacher, teacherIndex)]"
                class="teacher-card__avatar-action"
                :aria-label="`预览${teacher.name || '老师'}头像`"
                hover-class="teacher-card__avatar-action--pressed"
                @click="previewTeacherAvatar(teacher.avatar)"
              >
                <image
                  class="teacher-media teacher-card__avatar"
                  :src="teacher.avatar"
                  mode="aspectFill"
                  lazy-load
                  @error="markTeacherImageError(teacherMediaKey(teacher, teacherIndex))"
                />
              </button>
              <view v-else class="teacher-media__fallback" aria-hidden="true">
                {{ teacher.name ? teacher.name.slice(0, 1) : "师" }}
              </view>
              <view class="teacher-card__identity">
                <text class="teacher-card__name">{{ teacher.name }}</text>
                <text class="teacher-card__title">{{ teacher.title }}</text>
              </view>
            </view>
            <view class="teacher-card__details">
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
            <text class="section-kicker">{{ learnCopy.sections.courses.eyebrow }}</text>
            <text class="sec-title">{{ learnCopy.sections.courses.title }}</text>
          </view>
        </view>
        <NxAsyncState v-if="loading" state="loading" />
        <NxAsyncState
          v-else-if="coursewareItems.length === 0"
          state="empty"
          :title="learnCopy.sections.courses.emptyTitle"
          :description="learnCopy.sections.courses.emptyDescription"
        />
        <view v-else class="courseware-list">
          <view
            v-for="(c, i) in coursewareItems"
            :key="`${c.title || ''}:${i}`"
            class="courseware-card"
          >
            <view class="courseware-card__mark" aria-hidden="true">{{ c.badge || i + 1 }}</view>
            <view class="courseware-card__body">
              <text v-if="c.duration" class="courseware-card__duration">{{ c.duration }}</text>
              <text class="courseware-card__title">{{ c.title }}</text>
              <text class="courseware-card__desc">{{ c.description }}</text>
            </view>
          </view>
        </view>
      </view>

      <view class="card ios-card learn-section nx-panel section type-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">{{ learnCopy.sections.types.eyebrow }}</text>
            <text class="sec-title">{{ learnCopy.sections.types.title }}</text>
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
            <text class="section-kicker">{{ learnCopy.sections.quotes.eyebrow }}</text>
            <text class="sec-title">{{ learnCopy.sections.quotes.title }}</text>
          </view>
        </view>
        <NxAsyncState v-if="loading" state="loading" />
        <NxAsyncState
          v-else-if="!loadError && quotes.length === 0"
          state="empty"
          :title="learnCopy.sections.quotes.emptyTitle"
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
      {{ learnCopy.bottomCtaText }}
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
.classroom-entry__head {
  align-items: flex-start;
  flex-direction: row;
  flex-wrap: nowrap;
  gap: 16rpx;
}
.classroom-entry__head > view {
  flex: 1;
  min-width: 0;
}
.classroom-entry__more {
  flex-shrink: 0;
  width: auto;
  min-height: 88rpx;
  margin: 0;
  padding: 0 10rpx 0 18rpx;
  color: var(--nx-brand-900);
  font-size: 24rpx;
  font-weight: 900;
  line-height: 88rpx;
  border-radius: 18rpx;
  background: transparent;
}
.classroom-entry__more text { margin-left: 4rpx; font-size: 30rpx; }
.classroom-entry__more::after,
.retry::after { border: 0; }
.classroom-entry__intro {
  margin: 10rpx 0 18rpx;
  padding: 18rpx 20rpx;
  background: var(--nx-surface-soft);
  border-left: 6rpx solid var(--nx-accent-gold);
  border-radius: 0 18rpx 18rpx 0;
}
.classroom-entry__intro-eyebrow,
.classroom-entry__intro-title,
.classroom-entry__intro-lead { display: block; }
.classroom-entry__intro-eyebrow { color: var(--nx-brand-700); font-size: 21rpx; font-weight: 900; }
.classroom-entry__intro-title { margin-top: 6rpx; color: var(--nx-text); font-size: 27rpx; font-weight: 900; line-height: 1.45; }
.classroom-entry__intro-lead { margin-top: 6rpx; color: var(--nx-text-muted); font-size: 22rpx; line-height: 1.55; }
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
  gap: 0;
}
.classroom-entry__item {
  display: flex;
  align-items: center;
  min-height: 144rpx;
  box-sizing: border-box;
  gap: 18rpx;
  padding: 16rpx 0;
  overflow: hidden;
  background: transparent;
  border-bottom: 2rpx solid var(--nx-border);
  border-radius: 0;
  box-shadow: none;
}
.classroom-entry__item:last-child { border-bottom: 0; }
.classroom-entry__item--pressed { opacity: 0.84; transform: scale(0.988); }
.classroom-entry__media {
  position: relative;
  flex-shrink: 0;
  width: 200rpx;
  height: 112rpx;
  overflow: hidden;
  background: var(--nx-border);
  border-radius: 16rpx;
}
.classroom-entry__cover {
  display: block;
  width: 200rpx;
  height: 112rpx;
  background: var(--nx-border);
}
.classroom-entry__cover--fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--nx-brand-900);
  font-size: 36rpx;
  font-weight: 900;
  background: linear-gradient(135deg, var(--nx-surface-soft), var(--nx-accent-gold));
}
.classroom-entry__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  justify-content: flex-start;
  align-self: stretch;
  min-width: 0;
  min-height: 112rpx;
}
.classroom-entry__meta { display: flex; align-items: center; gap: 10rpx; color: var(--nx-text-muted); font-size: 20rpx; }
.classroom-entry__kind { color: var(--nx-brand-700); font-weight: 900; }
.classroom-entry__teacher { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.classroom-entry__title {
  display: -webkit-box;
  overflow: hidden;
  max-height: 2.8em;
  color: var(--nx-text);
  margin-top: 4rpx;
  font-size: 27rpx;
  font-weight: 900;
  line-height: 1.4;
  text-overflow: ellipsis;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
.classroom-entry__action {
  flex-shrink: 0;
  margin-top: auto;
  padding-top: 6rpx;
  color: var(--nx-brand-900);
  font-size: 22rpx;
  font-weight: 900;
}
.teacher-card {
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  margin-top: 24rpx;
  padding: 22rpx;
  background: var(--nx-surface-soft);
  border: 2rpx solid var(--nx-border);
  border-radius: 24rpx;
}
.teacher-card__header {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 20rpx;
  width: 100%;
}
.teacher-card__avatar-action {
  display: block;
  flex-shrink: 0;
  width: 116rpx;
  height: 116rpx;
  margin: 0;
  padding: 0;
  overflow: hidden;
  background: transparent;
  border: 0;
  border-radius: 28rpx;
  line-height: 1;
}
.teacher-card__avatar-action::after { border: 0; }
.teacher-card__avatar-action--pressed { opacity: .82; }
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
.teacher-card__identity,
.teacher-card__details {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 8rpx;
}
.teacher-card__identity { flex: 1; }
.teacher-card__details { width: 100%; }
.teacher-card__name { color: var(--nx-text); font-size: 32rpx; font-weight: 900; line-height: 1.3; }
.teacher-card__title { color: var(--nx-brand-700); font-size: 24rpx; font-weight: 900; }
.teacher-card__bio { color: var(--nx-text-muted); font-size: 25rpx; line-height: 1.65; }
.teacher-card__tags { display: flex; flex-wrap: wrap; gap: 10rpx; margin-top: 4rpx; }
.teacher-card__tag { color: var(--nx-brand-900); background: var(--nx-surface); font-size: 23rpx; }
.courseware-list {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  width: 100%;
  margin-top: 22rpx;
}
.courseware-card {
  display: flex;
  align-items: flex-start;
  gap: 18rpx;
  min-height: 144rpx;
  padding: 20rpx;
  background: var(--nx-surface-soft);
  border: 2rpx solid var(--nx-border);
  border-radius: 22rpx;
  box-sizing: border-box;
}
.courseware-card__mark {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 72rpx;
  height: 72rpx;
  color: var(--nx-brand-900);
  background: linear-gradient(135deg, var(--nx-accent-gold), var(--nx-surface));
  border: 2rpx solid rgba(223, 188, 127, 0.52);
  border-radius: 20rpx;
  font-size: 28rpx;
  font-weight: 900;
  box-sizing: border-box;
}
.courseware-card__body { flex: 1; min-width: 0; }
.courseware-card__duration { display: block; margin-bottom: 4rpx; color: var(--nx-brand-700); font-size: 21rpx; font-weight: 800; }
.courseware-card__title { display: block; color: var(--nx-text); font-size: 30rpx; font-weight: 900; line-height: 1.35; }
.courseware-card__desc { display: block; margin-top: 6rpx; color: var(--nx-text-muted); font-size: 23rpx; line-height: 1.55; }
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
