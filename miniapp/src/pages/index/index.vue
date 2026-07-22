<script setup>
import { computed, onMounted, ref } from 'vue'
import { QUESTIONS } from '../../data/enneagramGame'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import { resolveContentAsset } from '../../utils/contentAsset'
import {
  DEFAULT_COURSEWARE_ITEMS,
  DEFAULT_TEACHERS,
  normalizeCoursewareItems,
  normalizeTeachers,
} from '../../utils/teacherCourseware'
import { clearLearningNavIntent, setLearningNavIntent } from '../../utils/learningNavIntent'
import { userErrorMessage } from '../../utils/userMessage'

const TEACHER_FALLBACK = DEFAULT_TEACHERS[0].avatar
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
const COURSE_SECTION_PATHS = [
  ['courseware'],
  ['materials'],
  ['lessons'],
  ['courses'],
  ['home', 'courseware'],
  ['home', 'materials'],
  ['home', 'lessons'],
  ['home', 'courses'],
]

const total = QUESTIONS.length
const teachers = ref(normalizeTeachers())
const courses = ref(normalizeCoursewareItems())
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

function hasCourseSection(config) {
  return COURSE_SECTION_PATHS.some((path) => hasSectionAtPath(config, path))
}

function homeCourseCover(course, index) {
  const cover = typeof course?.cover === 'string' ? course.cover.trim() : ''
  const isLegacyWheel = /^\/static\/wheel\.png(?:[?#].*)?$/i.test(cover)
  return !cover || isLegacyWheel ? courseFallback(index) : cover
}

function syncContentImages() {
  teacherImageFallbackUsed.value = false
  courseImageFallbackUsed.value = {}
  teacherImage.value = resolveContentAsset(teacher.value?.avatar, TEACHER_FALLBACK)
  courseImages.value = courses.value.map((course, index) => (
    resolveContentAsset(homeCourseCover(course, index), courseFallback(index))
  ))
}

function applyContent(config, options = {}) {
  const preserveMissing = !!options.preserveMissing
  if (!preserveMissing || hasTeacherSection(config)) {
    teachers.value = normalizeTeachers(config)
  }
  if (!preserveMissing || hasCourseSection(config)) {
    courses.value = normalizeCoursewareItems(config)
  }
  syncContentImages()
}

function onTeacherImageError() {
  if (teacherImageFallbackUsed.value) return
  teacherImageFallbackUsed.value = true
  teacherImage.value = TEACHER_FALLBACK
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
    const config = await refreshSiteConfig()
    if (ticket !== loadTicket) return
    applyContent(config, { preserveMissing: true })
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
</script>

<template>
  <view class="home nx-page page-stack ios-page ios-safe-bottom">
    <main class="home__content">
      <view v-if="loading" class="sync-note" role="status">正在更新老师与课程资料…</view>

      <view v-if="loadError" class="home-error" role="status">
        <text>{{ loadError }}</text>
        <view
          class="home-retry"
          role="button"
          tabindex="0"
          hover-class="control--pressed"
          @click="activateAction(loadContent, $event)"
          @keydown="onActionKeydown($event, loadContent)"
        >重试更新</view>
      </view>

      <section class="teacher-welcome" aria-labelledby="teacher-heading">
        <template v-if="teacher">
          <image
            class="teacher-hero__image"
            :src="teacherImage"
            mode="aspectFill"
            role="img"
            :aria-label="teacherImageLabel"
            @error="onTeacherImageError"
          />
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
            @click="activateAction(toggleTeacher, $event)"
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
          <text id="service-heading" class="section-title">常用服务</text>
          <text class="section-note">选择你现在想了解的内容</text>
        </view>
        <nav class="service-grid" aria-label="常用服务">
          <view class="service-entry" role="button" tabindex="0" hover-class="control--pressed" @click="activateAction(goCourse, $event)" @keydown="onActionKeydown($event, goCourse)">
            <text class="service-index" aria-hidden="true">01</text>
            <text class="service-title">课程学习</text>
            <text class="service-desc">系统认识九型人格</text>
          </view>
          <view class="service-entry" role="button" tabindex="0" hover-class="control--pressed" @click="activateAction(goMaterial, $event)" @keydown="onActionKeydown($event, goMaterial)">
            <text class="service-index" aria-hidden="true">02</text>
            <text class="service-title">课件资料</text>
            <text class="service-desc">随时复习课程重点</text>
          </view>
          <view class="service-entry" role="button" tabindex="0" hover-class="control--pressed" @click="activateAction(startTest, $event)" @keydown="onActionKeydown($event, startTest)">
            <text class="service-index" aria-hidden="true">03</text>
            <text class="service-title">九型测试</text>
            <text class="service-desc">{{ total }} 道题认识自己</text>
          </view>
          <view class="service-entry" role="button" tabindex="0" hover-class="control--pressed" @click="activateAction(goRelation, $event)" @keydown="onActionKeydown($event, goRelation)">
            <text class="service-index" aria-hidden="true">04</text>
            <text class="service-title">关系合盘</text>
            <text class="service-desc">看懂彼此相处模式</text>
          </view>
        </nav>
      </section>

      <section class="content-section" aria-labelledby="course-heading">
        <view class="section-heading section-heading--row">
          <text id="course-heading" class="section-title">推荐课程</text>
          <text class="section-link">更多课程</text>
        </view>
        <view
          v-if="featuredCourse"
          class="featured-course"
          role="button"
          tabindex="0"
          :aria-label="`查看课程：${featuredCourse.title}`"
          hover-class="control--pressed"
          @click="activateAction(goCourse, $event)"
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
          <text class="home-empty__title">课程资料整理中</text>
          <text>新一期课程正在准备，稍后再来看看。</text>
        </view>
      </section>

      <section class="content-section" aria-labelledby="material-heading">
        <view class="section-heading section-heading--row">
          <text id="material-heading" class="section-title">最新课件</text>
          <text class="section-link">全部资料</text>
        </view>
        <view
          v-if="latestMaterial"
          class="latest-material"
          role="button"
          tabindex="0"
          :aria-label="`查看课件：${latestMaterial.course.title}`"
          hover-class="control--pressed"
          @click="activateAction(goMaterial, $event)"
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

      <view class="booking-prompt" role="button" tabindex="0" hover-class="control--pressed" @click="activateAction(goBooking, $event)" @keydown="onActionKeydown($event, goBooking)">
        <view class="booking-copy">
          <text class="booking-title">预约咨询</text>
          <text class="booking-desc">有具体困惑？和老师一对一聊聊</text>
        </view>
        <text class="booking-action">去预约 <text aria-hidden="true">›</text></text>
      </view>
    </main>
  </view>
</template>

<style scoped>
.home {
  --home-bg: #F5F6F4;
  --home-surface: #FFFFFF;
  --home-ink: #20252B;
  --home-green: #335B4A;
  min-width: 0;
  overflow-x: hidden;
  background: var(--home-bg);
  color: var(--home-ink);
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
  font-size: 23rpx;
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
  color: var(--home-green);
  font-size: 24rpx;
  font-weight: 700;
}

.home-retry {
  flex: 0 0 auto;
  padding: 0 20rpx;
}

.teacher-welcome {
  display: grid;
  grid-template-columns: 112rpx minmax(0, 1fr);
  column-gap: 20rpx;
  padding: 24rpx;
  border: 2rpx solid #E6EAE6;
  border-radius: 20rpx;
  background: var(--home-surface);
}

.teacher-hero__image {
  grid-row: 1 / span 2;
  width: 112rpx;
  height: 112rpx;
  border-radius: 16rpx;
  background: #EEF1EE;
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
  font-size: 22rpx;
  line-height: 1.4;
}

.teacher-name {
  color: var(--home-ink);
  font-size: 32rpx;
  font-weight: 800;
  line-height: 1.3;
}

.teacher-identity {
  color: var(--home-green);
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
  color: var(--home-ink);
  font-size: 30rpx;
  font-weight: 800;
  line-height: 1.4;
}

.section-note,
.section-link {
  color: #7A827E;
  font-size: 22rpx;
  line-height: 1.4;
}

.section-link {
  color: var(--home-green);
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
  background: var(--home-surface);
  touch-action: manipulation;
}

.service-index {
  margin-bottom: 12rpx;
  color: var(--home-green);
  font-size: 21rpx;
  font-weight: 800;
  line-height: 1;
}

.service-title {
  color: var(--home-ink);
  font-size: 27rpx;
  font-weight: 800;
  line-height: 1.4;
}

.service-desc {
  margin-top: 4rpx;
  color: #737B77;
  font-size: 21rpx;
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
  background: var(--home-surface);
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
  color: var(--home-ink);
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.4;
}

.course-desc {
  overflow: hidden;
  color: #69716D;
  font-size: 21rpx;
  line-height: 1.45;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.course-meta,
.material-meta,
.booking-desc {
  color: #747C78;
  font-size: 21rpx;
  line-height: 1.45;
}

.course-meta {
  color: var(--home-green);
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
  color: var(--home-green);
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
  color: var(--home-green);
  font-size: 23rpx;
  font-weight: 700;
}

.home-empty {
  padding: 24rpx;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  border: 2rpx solid #E6EAE6;
  border-radius: 18rpx;
  background: var(--home-surface);
  color: #66706B;
  font-size: 24rpx;
  line-height: 1.55;
}

.home-empty__title {
  color: var(--home-ink);
  font-size: 27rpx;
  font-weight: 800;
}

.control--pressed {
  opacity: .72;
}

.home-retry:focus-visible,
.teacher-toggle:focus-visible,
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
</style>
