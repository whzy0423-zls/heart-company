<script setup>
import { computed, onMounted, ref } from 'vue'
import { QUESTIONS } from '../../data/enneagramGame'
import { listClassroomRecentApi } from '../../api'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import { resolveContentAsset } from '../../utils/contentAsset'
import { DEFAULT_TEACHERS, normalizeTeachers } from '../../utils/teacherCourseware'
import { mapPublishedClassroomItems } from '../../utils/classroomCourseware'
import { clearLearningNavIntent, setLearningNavIntent } from '../../utils/learningNavIntent'
import { normalizeMiniappLearn } from '../../utils/miniappPages'
import { userErrorMessage } from '../../utils/userMessage'
import { previewImage } from '../../utils/imagePreview'

const TEACHER_FALLBACK = ''
const COURSE_FALLBACKS = [
  '/static/editorial/course-intro.webp',
  '/static/editorial/course-growth.webp',
  '/static/editorial/course-relation.webp',
]
const TEACHER_SECTION_PATHS = [
  ['teacher'],
  ['teachers'],
  ['home', 'teacher'],
  ['home', 'teachers'],
  ['home', 'teacherTeaser'],
]

const total = QUESTIONS.length
const teachers = ref(normalizeTeachers())
const courses = ref([])
const classroomEnabled = ref(true)
const loading = ref(true)
const loadError = ref('')
const teacherExpanded = ref(false)
const teacherImage = ref(TEACHER_FALLBACK)
const courseImages = ref([])
const teacherImageFallbackUsed = ref(false)
const courseImageFallbackUsed = ref({})
let loadTicket = 0
let keyboardActivationAt = 0
let keyboardActivationTarget = null

const teacher = computed(() => teachers.value[0] || null)
const featuredCourse = computed(() => courses.value[0] || null)
const latestMaterial = computed(() => {
  const course = courses.value.find((item) => Array.isArray(item.materialTypes) && item.materialTypes.length > 0)
  if (!course) return null
  return { course, type: course.materialTypes[0] }
})
const teacherImageLabel = computed(() => teacher.value ? `${teacher.value.name}老师肖像` : '授课老师肖像')

function courseFallback(index) {
  return COURSE_FALLBACKS[index % COURSE_FALLBACKS.length]
}

function hasSectionAtPath(config, path) {
  let current = config
  for (let index = 0; index < path.length; index += 1) {
    if (!current || typeof current !== 'object') return false
    const key = path[index]
    if (!Object.prototype.hasOwnProperty.call(current, key)) return false
    if (index === path.length - 1) return true
    current = current[key]
  }
  return false
}

function hasTeacherSection(config) {
  return TEACHER_SECTION_PATHS.some((path) => hasSectionAtPath(config, path))
}

function homeCourseCover(course, index) {
  const cover = typeof course?.cover === 'string' ? course.cover.trim() : ''
  const isLegacyWheel = /^\/static\/wheel\.png(?:[?#].*)?$/i.test(cover)
  return !cover || isLegacyWheel ? courseFallback(index) : cover
}

function syncContentImages() {
  teacherImageFallbackUsed.value = false
  courseImageFallbackUsed.value = {}
  const portrait = teacher.value?.avatar === '/static/avatars/9.png' ? '' : teacher.value?.avatar
  teacherImage.value = resolveContentAsset(portrait, TEACHER_FALLBACK)
  courseImages.value = courses.value.map((course, index) => (
    resolveContentAsset(homeCourseCover(course, index), courseFallback(index))
  ))
}

function applyContent(config, options = {}) {
  classroomEnabled.value = normalizeMiniappLearn(config).classroom.enabled
  const preserveMissing = !!options.preserveMissing
  if (!preserveMissing || hasTeacherSection(config)) {
    teachers.value = normalizeTeachers(config)
  }
  syncContentImages()
}

function onTeacherImageError() {
  if (teacherImageFallbackUsed.value) return
  teacherImageFallbackUsed.value = true
  teacherImage.value = TEACHER_FALLBACK
}

function previewTeacherAvatar() {
  previewImage(teacherImage.value)
}

function onCourseImageError(index) {
  if (courseImageFallbackUsed.value[index]) return
  courseImageFallbackUsed.value = {
    ...courseImageFallbackUsed.value,
    [index]: true,
  }
  courseImages.value[index] = courseFallback(index)
}

function activateAction(action, event) {
  const eventType = event?.type || ''
  const now = Date.now()
  if (eventType === 'keydown') {
    if (event?.repeat) return
    keyboardActivationAt = now
    keyboardActivationTarget = event?.currentTarget || null
    action()
    return
  }
  if (
    eventType === 'click'
    && keyboardActivationTarget === (event?.currentTarget || null)
    && now - keyboardActivationAt < 500
  ) {
    keyboardActivationTarget = null
    return
  }
  keyboardActivationTarget = null
  action()
}

function onActionKeydown(event, action) {
  if (!['Enter', ' ', 'Spacebar'].includes(event?.key)) return
  event.preventDefault?.()
  event.stopPropagation?.()
  activateAction(action, event)
}

async function loadContent() {
  const ticket = ++loadTicket
  loading.value = true
  loadError.value = ''
  try {
    const [config, classroom] = await Promise.all([
      refreshSiteConfig(),
      listClassroomRecentApi({ limit: 6, offset: 0 }),
    ])
    if (ticket !== loadTicket) return
    applyContent(config, { preserveMissing: true })
    courses.value = mapPublishedClassroomItems(classroom?.items)
    syncContentImages()
  } catch (error) {
    if (ticket !== loadTicket) return
    loadError.value = userErrorMessage(error, '内容更新失败，当前仍可继续浏览')
  } finally {
    if (ticket === loadTicket) loading.value = false
  }
}

onMounted(() => {
  const cached = getStoredSiteConfig()
  if (cached) applyContent(cached)
  else syncContentImages()
  loadContent()
})

function toggleTeacher() {
  teacherExpanded.value = !teacherExpanded.value
}

function goCourse() {
  setLearningNavIntent('course')
  uni.switchTab({
    url: '/pages/learn/learn',
    fail() {
      clearLearningNavIntent()
    },
  })
}

function goMaterial() {
  setLearningNavIntent('material')
  uni.switchTab({
    url: '/pages/learn/learn',
    fail() {
      clearLearningNavIntent()
    },
  })
}

function startTest() {
  uni.navigateTo({ url: '/pages/test/test' })
}

function goRelation() {
  uni.navigateTo({ url: '/pages/relation/relation' })
}

function goBooking() {
  uni.switchTab({ url: '/pages/booking/booking' })
}

function goEnneagram() {
  uni.navigateTo({ url: '/pages/enneagram/enneagram' })
}
</script>

<template>
  <view class="home nx-page page-stack ios-page ios-safe-bottom">
    <main class="home__content">
      <view v-if="loading" class="sync-note" role="status">{{ classroomEnabled ? '正在更新老师与课程资料…' : '正在更新老师资料…' }}</view>

      <view v-if="loadError" class="home-error" role="status">
        <text>{{ loadError }}</text>
        <view
          class="home-retry"
          role="button"
          tabindex="0"
          hover-class="control--pressed"
          @tap="loadContent"
          @keydown="onActionKeydown($event, loadContent)"
        >重试更新</view>
      </view>

      <section class="expert-hero" aria-labelledby="home-hero-title">
        <text class="expert-hero__eyebrow">YOUR INNER MAP</text>
        <text id="home-hero-title" class="expert-hero__title">读懂自己，<br />也读懂重要的人</text>
        <text class="expert-hero__lead">把复杂的性格，变成清晰可用的生活地图</text>
        <view class="expert-hero__feature">
          <text class="expert-hero__tag">今日推荐 · 3分钟</text>
          <text class="expert-hero__feature-title">九型性格深度测试</text>
          <text class="expert-hero__feature-copy">不是给你贴标签，而是帮你发现更多选择。</text>
          <button class="expert-hero__primary" hover-class="expert-hero__primary--pressed" @tap="startTest">开始探索</button>
        </view>
      </section>

      <section class="teacher-welcome" aria-labelledby="teacher-heading">
        <template v-if="teacher">
          <button
            v-if="teacherImage"
            class="teacher-card__avatar-action"
            type="button"
            :aria-label="`预览${teacherImageLabel}`"
            hover-class="teacher-card__avatar-action--pressed"
            @click="previewTeacherAvatar"
          >
            <image
              class="teacher-hero__image"
              :src="teacherImage"
              mode="aspectFill"
              role="img"
              :aria-label="teacherImageLabel"
              @error="onTeacherImageError"
            />
          </button>
          <view v-else class="teacher-hero__image teacher-hero__image--placeholder" aria-hidden="true">韩</view>
          <view class="teacher-copy">
            <text class="teacher-eyebrow">你好，我是</text>
            <text id="teacher-heading" class="teacher-name">{{ teacher.name }}</text>
            <text class="teacher-identity">{{ teacher.title }}</text>
          </view>
          <text id="teacher-bio" class="teacher-bio" :class="{ 'teacher-bio--expanded': teacherExpanded }">{{ teacher.bio }}</text>
          <view
            class="teacher-toggle"
            role="button"
            tabindex="0"
            :aria-expanded="teacherExpanded"
            aria-controls="teacher-bio"
            hover-class="control--pressed"
            @tap="toggleTeacher"
            @keydown="onActionKeydown($event, toggleTeacher)"
          >{{ teacherExpanded ? '收起介绍' : '了解老师' }} <text aria-hidden="true">{{ teacherExpanded ? '↑' : '↓' }}</text></view>
        </template>
        <view v-else class="home-empty teacher-empty">
          <text id="teacher-heading" class="home-empty__title">老师资料整理中</text>
          <text>课程团队正在完善主讲老师介绍，稍后再来看看。</text>
        </view>
      </section>

      <section class="service-section" aria-labelledby="service-heading">
        <view class="section-heading">
          <text id="service-heading" class="section-title">探索工具</text>
          <text class="section-note">看见 · 理解 · 成长</text>
        </view>
        <nav class="service-grid" aria-label="常用服务">
          <view v-if="classroomEnabled" class="service-entry service-entry--course" role="button" tabindex="0" hover-class="control--pressed" @tap="goCourse" @keydown="onActionKeydown($event, goCourse)">
            <text class="service-index" aria-hidden="true">01</text>
            <text class="service-title">成长课堂</text>
            <text class="service-desc">按自己的节奏学习</text>
          </view>
          <view v-if="classroomEnabled" class="service-entry service-entry--material" role="button" tabindex="0" hover-class="control--pressed" @tap="goMaterial" @keydown="onActionKeydown($event, goMaterial)">
            <text class="service-index" aria-hidden="true">02</text>
            <text class="service-title">课件资料</text>
            <text class="service-desc">随时复习课程重点</text>
          </view>
          <view class="service-entry service-entry--test" role="button" tabindex="0" hover-class="control--pressed" @tap="startTest" @keydown="onActionKeydown($event, startTest)">
            <text class="service-index" aria-hidden="true">03</text>
            <text class="service-title">性格测试</text>
            <text class="service-desc">{{ total }} 道题认识自己</text>
          </view>
          <view class="service-entry service-entry--relation" role="button" tabindex="0" hover-class="control--pressed" @tap="goRelation" @keydown="onActionKeydown($event, goRelation)">
            <text class="service-index" aria-hidden="true">04</text>
            <text class="service-title">关系合盘</text>
            <text class="service-desc">看懂彼此相处模式</text>
          </view>
          <view class="service-entry service-entry--booking" role="button" tabindex="0" hover-class="control--pressed" @tap="goBooking" @keydown="onActionKeydown($event, goBooking)">
            <text class="service-index" aria-hidden="true">05</text>
            <text class="service-title">预约咨询</text>
            <text class="service-desc">获得专业陪伴</text>
          </view>
          <view class="service-entry service-entry--enneagram" role="button" tabindex="0" hover-class="control--pressed" @tap="goEnneagram" @keydown="onActionKeydown($event, goEnneagram)">
            <text class="service-index" aria-hidden="true">06</text>
            <text class="service-title">认识九型</text>
            <text class="service-desc">先理解地图，再理解自己</text>
          </view>
        </nav>
      </section>

      <section v-if="classroomEnabled" class="content-section" aria-labelledby="course-heading">
        <view class="section-heading section-heading--row">
          <text id="course-heading" class="section-title">推荐课程</text>
          <view
            class="section-link section-link--course"
            role="button"
            tabindex="0"
            hover-class="control--pressed"
            @tap="goCourse"
            @keydown="onActionKeydown($event, goCourse)"
          >更多课程</view>
        </view>
        <view
          v-if="featuredCourse"
          class="featured-course"
          role="button"
          tabindex="0"
          :aria-label="`查看课程：${featuredCourse.title}`"
          hover-class="control--pressed"
          @tap="goCourse"
          @keydown="onActionKeydown($event, goCourse)"
        >
          <image class="featured-course__cover" :src="courseImages[0]" mode="aspectFill" aria-hidden="true" @error="onCourseImageError(0)" />
          <view class="course-copy">
            <text class="course-title">{{ featuredCourse.title }}</text>
            <text class="course-desc">{{ featuredCourse.description }}</text>
            <text class="course-meta">{{ featuredCourse.badge }} · {{ featuredCourse.duration }}</text>
          </view>
          <text class="row-arrow" aria-hidden="true">›</text>
        </view>
        <view v-else class="home-empty">
          <text class="home-empty__title">课程正在准备中</text>
          <text>新一期课程正在准备，稍后再来看看。</text>
        </view>
      </section>

      <section v-if="classroomEnabled" class="content-section" aria-labelledby="material-heading">
        <view class="section-heading section-heading--row">
          <text id="material-heading" class="section-title">最新课件</text>
          <view
            class="section-link section-link--material"
            role="button"
            tabindex="0"
            hover-class="control--pressed"
            @tap="goMaterial"
            @keydown="onActionKeydown($event, goMaterial)"
          >全部资料</view>
        </view>
        <view
          v-if="latestMaterial"
          class="latest-material"
          role="button"
          tabindex="0"
          :aria-label="`查看课件：${latestMaterial.course.title}`"
          hover-class="control--pressed"
          @tap="goMaterial"
          @keydown="onActionKeydown($event, goMaterial)"
        >
          <view class="material-icon" aria-hidden="true">文</view>
          <view class="material-copy">
            <text class="material-title">{{ latestMaterial.course.title }}</text>
            <text class="material-meta">{{ latestMaterial.type }} · {{ latestMaterial.course.duration }}</text>
          </view>
          <text class="row-arrow" aria-hidden="true">›</text>
        </view>
        <view v-else class="home-empty">
          <text class="home-empty__title">学习资料整理中</text>
          <text>老师正在整理新的学习资料。</text>
        </view>
      </section>

      <view class="booking-prompt" role="button" tabindex="0" hover-class="control--pressed" @tap="goBooking" @keydown="onActionKeydown($event, goBooking)">
        <view class="booking-copy">
          <text class="booking-title">预约咨询</text>
          <text class="booking-desc">有具体困惑？和老师一对一聊聊</text>
        </view>
        <text class="booking-action">去预约 <text aria-hidden="true">›</text></text>
      </view>

      <view class="daily-guidance" role="note">
        <view class="daily-guidance__mark">光</view>
        <view class="daily-guidance__copy">
          <text class="daily-guidance__title">今日成长引导</text>
          <text class="daily-guidance__text">你此刻最需要被理解的，是什么？</text>
        </view>
      </view>
    </main>
  </view>
</template>

<style scoped>
.home {
  min-width: 0;
  overflow-x: hidden;
  background: var(--nx-mist-bg);
  color: var(--nx-text);
}

.home__content {
  width: 100%;
  max-width: 900rpx;
  margin: 0 auto;
  padding: 24rpx 24rpx 48rpx;
  box-sizing: border-box;
}

.sync-note,
.home-error {
  min-height: 72rpx;
  display: flex;
  align-items: center;
  gap: 16rpx;
  color: #66706B;
  font-size: 24rpx;
  line-height: 1.5;
}

.home-error {
  justify-content: space-between;
  margin-bottom: 16rpx;
  color: #8C3C30;
}

.home-retry,
.teacher-toggle {
  min-height: 88rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #335B4A;
  font-size: 24rpx;
  font-weight: 700;
}

.home-retry {
  flex: 0 0 auto;
  padding: 0 20rpx;
}

.teacher-welcome {
  display: grid;
  grid-template-columns: 128rpx minmax(0, 1fr);
  column-gap: 20rpx;
  padding: 24rpx;
  border: 2rpx solid #E6EAE6;
  border-radius: 20rpx;
  background: #FFFFFF;
}

.teacher-hero__image {
  width: 112rpx;
  height: 112rpx;
  border-radius: 16rpx;
  background: #EEF1EE;
}

.teacher-card__avatar-action {
  grid-row: 1 / span 2;
  width: 128rpx;
  height: 128rpx;
  padding: 0;
  border: 0;
  border-radius: 22rpx;
  background: var(--nx-surface-soft);
  overflow: hidden;
  box-shadow: 0 10rpx 24rpx rgba(32, 42, 55, .12);
}

.teacher-card__avatar-action::after { border: 0; }
.teacher-card__avatar-action--pressed { opacity: .78; transform: scale(.97); }
.teacher-card__avatar-action .teacher-hero__image { width: 128rpx; height: 128rpx; display: block; }

.teacher-hero__image--placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #335B4A;
  font-size: 48rpx;
  font-weight: 800;
}

.teacher-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 3rpx;
}

.teacher-eyebrow,
.teacher-identity {
  color: #68716C;
  font-size: 24rpx;
  line-height: 1.4;
}

.teacher-name {
  color: #20252B;
  font-size: 32rpx;
  font-weight: 800;
  line-height: 1.3;
}

.teacher-identity {
  color: #335B4A;
}

.teacher-bio {
  grid-column: 1 / -1;
  margin-top: 20rpx;
  overflow: hidden;
  color: #59615D;
  font-size: 25rpx;
  line-height: 1.65;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.teacher-bio--expanded {
  display: block;
  overflow: visible;
}

.teacher-toggle {
  grid-column: 1 / -1;
  justify-self: start;
  gap: 8rpx;
}

.teacher-empty {
  grid-column: 1 / -1;
}

.service-section,
.content-section {
  margin-top: 32rpx;
}

.section-heading {
  margin-bottom: 16rpx;
  display: flex;
  flex-direction: column;
  gap: 4rpx;
}

.section-heading--row {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  color: #20252B;
  font-size: 30rpx;
  font-weight: 800;
  line-height: 1.4;
}

.section-note,
.section-link {
  color: #4F5A54;
  font-size: 24rpx;
  line-height: 1.4;
}

.section-link {
  min-height: 88rpx;
  display: inline-flex;
  align-items: center;
  color: #335B4A;
}

.service-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14rpx;
}

.service-entry {
  min-width: 0;
  min-height: 168rpx;
  padding: 20rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  box-sizing: border-box;
  border: 2rpx solid #E6EAE6;
  border-radius: 18rpx;
  background: #FFFFFF;
  touch-action: manipulation;
}

.service-index {
  margin-bottom: 12rpx;
  color: #335B4A;
  font-size: 21rpx;
  font-weight: 800;
  line-height: 1;
}

.service-title {
  color: #20252B;
  font-size: 27rpx;
  font-weight: 800;
  line-height: 1.4;
}

.service-desc {
  margin-top: 4rpx;
  color: #4F5A54;
  font-size: 24rpx;
  line-height: 1.45;
}

.featured-course,
.latest-material,
.booking-prompt {
  width: 100%;
  min-width: 0;
  min-height: 88rpx;
  padding: 18rpx;
  display: flex;
  align-items: center;
  gap: 18rpx;
  box-sizing: border-box;
  border: 2rpx solid #E6EAE6;
  border-radius: 18rpx;
  background: #FFFFFF;
  color: inherit;
  touch-action: manipulation;
}

.featured-course__cover {
  flex: 0 0 auto;
  width: 144rpx;
  height: 108rpx;
  border-radius: 14rpx;
  background: #EEF1EE;
}

.course-copy,
.material-copy,
.booking-copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 5rpx;
}

.course-title,
.material-title,
.booking-title {
  color: #20252B;
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.4;
}

.course-desc {
  overflow: hidden;
  color: #4F5A54;
  font-size: 24rpx;
  line-height: 1.45;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.course-meta,
.material-meta,
.booking-desc {
  color: #4F5A54;
  font-size: 24rpx;
  line-height: 1.45;
}

.course-meta {
  color: #335B4A;
}

.row-arrow {
  flex: 0 0 auto;
  color: #9BA19E;
  font-size: 38rpx;
}

.material-icon {
  flex: 0 0 auto;
  width: 72rpx;
  height: 72rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 16rpx;
  background: #EDF3EF;
  color: #335B4A;
  font-size: 24rpx;
  font-weight: 800;
}

.booking-prompt {
  margin-top: 32rpx;
  padding: 22rpx 24rpx;
  border-color: #DDE5DF;
}

.booking-action {
  flex: 0 0 auto;
  color: #335B4A;
  font-size: 24rpx;
  font-weight: 700;
}

.home-empty {
  padding: 24rpx;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  border: 2rpx solid #E6EAE6;
  border-radius: 18rpx;
  background: #FFFFFF;
  color: #66706B;
  font-size: 24rpx;
  line-height: 1.55;
}

.home-empty__title {
  color: #20252B;
  font-size: 27rpx;
  font-weight: 800;
}

.control--pressed {
  opacity: .72;
}

.home-retry:focus-visible,
.teacher-toggle:focus-visible,
.section-link:focus-visible,
.service-entry:focus-visible,
.featured-course:focus-visible,
.latest-material:focus-visible,
.booking-prompt:focus-visible {
  outline: 4rpx solid #176B58;
  outline-offset: 4rpx;
}

@media screen and (min-width: 768px) {
  .home__content {
    max-width: 1120rpx;
    padding-left: 40rpx;
    padding-right: 40rpx;
  }
}

.expert-hero {
  position: relative;
  overflow: hidden;
  margin-bottom: 28rpx;
  padding: 28rpx;
  border-radius: 28rpx;
  background: var(--nx-mist-brand-soft);
  box-shadow: 0 18rpx 42rpx rgba(31, 102, 99, .14);
}
.expert-hero::after {
  position: absolute;
  content: '';
  width: 220rpx;
  height: 220rpx;
  top: -54rpx;
  right: -42rpx;
  border-radius: 50%;
  background: var(--nx-mist-gold);
  opacity: .58;
  box-shadow: 0 0 0 42rpx rgba(242, 189, 90, .13);
}
.expert-hero__eyebrow,
.expert-hero__title,
.expert-hero__lead,
.expert-hero__feature { position: relative; z-index: 1; display: block; }
.expert-hero__eyebrow { color: #467D7B; font-size: 22rpx; font-weight: 900; letter-spacing: 4rpx; }
.expert-hero__title { margin-top: 14rpx; color: #183439; font-size: 48rpx; font-weight: 900; line-height: 1.25; }
.expert-hero__lead { margin-top: 10rpx; color: rgba(24, 52, 57, .7); font-size: 24rpx; line-height: 1.6; }
.expert-hero__feature { margin-top: 24rpx; padding: 22rpx; border-radius: 22rpx; background: rgba(255, 255, 255, .52); border: 2rpx solid rgba(255, 255, 255, .7); }
.expert-hero__tag { display: inline-flex; padding: 8rpx 16rpx; border-radius: 999rpx; background: var(--nx-mist-surface); color: #39716E; font-size: 20rpx; font-weight: 900; }
.expert-hero__feature-title { display: block; margin-top: 22rpx; color: #183439; font-size: 34rpx; font-weight: 900; line-height: 1.35; }
.expert-hero__feature-copy { display: block; margin: 8rpx 0 22rpx; color: rgba(24, 52, 57, .68); font-size: 22rpx; line-height: 1.55; }
.expert-hero__primary { width: auto; min-width: 220rpx; min-height: 88rpx; margin: 0; padding: 0 28rpx; border: 0; border-radius: 16rpx; background: var(--nx-mist-brand); color: #FFF; font-size: 25rpx; font-weight: 900; line-height: 88rpx; }
.expert-hero__primary::after { border: 0; }
.expert-hero__primary--pressed { opacity: .78; transform: translateY(2rpx); }
.teacher-welcome,
.service-entry,
.featured-course,
.latest-material,
.booking-prompt,
.home-empty { border-color: var(--nx-mist-border); background: var(--nx-mist-surface); }
.teacher-welcome { box-shadow: 0 10rpx 28rpx rgba(35, 75, 75, .06); }
.service-entry { min-height: 176rpx; border-radius: 20rpx; box-shadow: 0 8rpx 22rpx rgba(35, 75, 75, .05); }
.service-entry--booking { grid-column: span 2; min-height: 132rpx; flex-direction: row; align-items: center; gap: 20rpx; }
.service-entry--booking .service-index { margin: 0; }
.service-entry--booking .service-desc { margin-left: auto; }
.service-index { color: var(--nx-mist-brand); }
.service-title, .course-title, .material-title, .booking-title, .home-empty__title { color: #183439; }
.service-desc, .course-desc, .course-meta, .material-meta, .booking-desc { color: rgba(24, 52, 57, .64); }
.course-meta { color: var(--nx-mist-brand); }
.material-icon { background: var(--nx-mist-soft); color: var(--nx-mist-brand); }
.booking-prompt { border-color: var(--nx-mist-border); background: var(--nx-mist-soft); }
.booking-action, .section-link { color: var(--nx-mist-brand); }
.daily-guidance { display: flex; align-items: center; gap: 18rpx; margin-top: 24rpx; padding: 20rpx; border-radius: 20rpx; background: var(--nx-mist-soft); }
.daily-guidance__mark { display: flex; align-items: center; justify-content: center; width: 68rpx; height: 68rpx; flex: 0 0 68rpx; border-radius: 22rpx; background: var(--nx-mist-brand); color: #FFF; font-size: 26rpx; font-weight: 900; }
.daily-guidance__copy { min-width: 0; display: flex; flex-direction: column; gap: 5rpx; }
.daily-guidance__title { color: #183439; font-size: 25rpx; font-weight: 900; }
.daily-guidance__text { color: rgba(24, 52, 57, .68); font-size: 22rpx; line-height: 1.5; }

/* Distinct entry accents keep the tool grid scannable without introducing a new palette. */
.service-entry {
  position: relative;
  overflow: hidden;
  align-items: flex-start;
  border-color: var(--nx-border);
  box-shadow: 0 14rpx 30rpx -26rpx rgba(32, 42, 55, .58);
}
.service-entry::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 8rpx;
  background: var(--nx-accent-gold);
}
.service-entry--course { background: #F1F3F1; }
.service-entry--course::before { background: var(--nx-brand-700); }
.service-entry--material { background: #F8F1E5; }
.service-entry--test { background: #F4ECE7; }
.service-entry--test::before { background: #7A6253; }
.service-entry--relation { background: #EDF0F2; }
.service-entry--relation::before { background: #66798A; }
.service-entry--booking { background: var(--nx-surface-soft); }
.service-entry--booking::before { background: var(--nx-brand-900); }
.service-entry--enneagram { background: #F5EDDF; }
.service-entry--course,
.service-entry--test,
.service-entry--booking { grid-column: span 1; min-height: 176rpx; flex-direction: column; align-items: flex-start; gap: 0; }
.service-entry--course .service-index,
.service-entry--test .service-index,
.service-entry--booking .service-index { margin: 0 0 14rpx; }
.service-entry--course .service-desc,
.service-entry--test .service-desc,
.service-entry--booking .service-desc { margin-left: 0; text-align: left; }
.service-index {
  min-width: 54rpx;
  min-height: 38rpx;
  padding: 7rpx 10rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 14rpx;
  border-radius: 10rpx;
  background: var(--nx-accent-gold);
  color: var(--nx-surface);
  font-size: 20rpx;
  letter-spacing: 1rpx;
}
.service-title { color: var(--nx-brand-900); font-size: 29rpx; font-weight: 900; }
.service-desc { color: var(--nx-text-muted); }
.service-entry--booking .service-index { margin: 0 0 14rpx; }
.service-entry--booking { align-items: flex-start; }
.service-entry--booking .service-title { font-size: 28rpx; }
.service-entry:focus-visible { outline-color: var(--nx-accent-gold); }

/* Keep the existing card language, but make the home page breathe better on
 * smaller phones and keep the teacher portrait a clear interactive anchor. */
.home__content { padding-bottom: 12rpx; }
.teacher-welcome { grid-template-columns: 128rpx minmax(0, 1fr); gap: 20rpx; padding: 26rpx; border-radius: 24rpx; }
.service-section,
.content-section { margin-top: 28rpx; }
.service-grid { gap: 16rpx; }
.featured-course,
.latest-material { border-radius: 22rpx; }

@media screen and (max-width: 600rpx) {
  .service-entry--course,
  .service-entry--test,
  .service-entry--booking { min-height: 164rpx; }
  .service-entry--course .service-desc,
  .service-entry--test .service-desc,
  .service-entry--booking .service-desc { max-width: none; }
}

@media (prefers-reduced-motion: reduce) {
  .expert-hero__primary--pressed { transform: none; }
}
</style>
