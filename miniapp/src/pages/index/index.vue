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
const teacherImage = ref(TEACHER_FALLBACK)
const courseImages = ref([])
const teacherImageFallbackUsed = ref(false)
const courseImageFallbackUsed = ref({})
let loadTicket = 0
let keyboardActivationAt = 0
let keyboardActivationTarget = null

const teacher = computed(() => teachers.value[0] || null)
const featuredCourse = computed(() => courses.value[0] || null)
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
    eventType === 'click' &&
    keyboardActivationTarget === (event?.currentTarget || null) &&
    now - keyboardActivationAt < 500
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

function startTest() {
  uni.navigateTo({ url: '/pages/test/test' })
}

function goLearn() {
  uni.switchTab({ url: '/pages/learn/learn' })
}

function goBooking() {
  uni.switchTab({ url: '/pages/booking/booking' })
}
</script>

<template>
  <view class="home nx-page page-stack ios-page ios-safe-bottom">
    <main class="home__content">
      <view v-if="loading" class="sync-note" role="status">
        正在更新老师与课程资料…
      </view>

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

      <section class="teacher-hero" aria-labelledby="teacher-heading">
        <view v-if="teacher" class="teacher-hero__portrait">
          <image
            class="teacher-hero__image"
            :src="teacherImage"
            mode="aspectFill"
            role="img"
            :aria-label="teacherImageLabel"
            @error="onTeacherImageError"
          />
        </view>

        <view v-if="teacher" class="teacher-hero__copy">
          <text id="teacher-heading" class="teacher-hero__name">{{ teacher.name }}</text>
          <text class="teacher-hero__identity">{{ teacher.title }}</text>
          <text class="teacher-hero__bio">{{ teacher.bio }}</text>
          <view
            class="home-primary"
            role="button"
            tabindex="0"
            hover-class="control--pressed"
            @click="activateAction(goLearn, $event)"
            @keydown="onActionKeydown($event, goLearn)"
          >开始学习</view>
        </view>

        <view v-else class="home-empty teacher-hero__empty">
          <text id="teacher-heading" class="home-empty__title">老师资料整理中</text>
          <text>课程团队正在完善主讲老师介绍，稍后再来看看。</text>
        </view>
      </section>

      <section class="course-section" aria-labelledby="course-heading">
        <text id="course-heading" class="course-section__title">精选课程</text>
        <view
          v-if="featuredCourse"
          class="featured-course"
          role="button"
          tabindex="0"
          :aria-label="`查看课程：${featuredCourse.title}`"
          hover-class="control--pressed"
          @click="activateAction(goLearn, $event)"
          @keydown="onActionKeydown($event, goLearn)"
        >
          <image
            class="featured-course__cover"
            :src="courseImages[0]"
            mode="aspectFill"
            aria-hidden="true"
            @error="onCourseImageError(0)"
          />
          <view class="featured-course__copy">
            <text class="featured-course__title">{{ featuredCourse.title }}</text>
            <text class="featured-course__desc">{{ featuredCourse.description }}</text>
            <text class="featured-course__meta">{{ featuredCourse.badge }} · {{ featuredCourse.duration }}</text>
          </view>
        </view>

        <view v-else class="home-empty">
          <text class="home-empty__title">课程资料整理中</text>
          <text>新一期课程正在准备，稍后再来看看。</text>
        </view>
      </section>

      <nav class="secondary-nav" aria-label="更多服务">
        <view
          class="secondary-entry"
          role="button"
          tabindex="0"
          hover-class="control--pressed"
          @click="activateAction(startTest, $event)"
          @keydown="onActionKeydown($event, startTest)"
        >
          <view>
            <text class="secondary-entry__title">九型自测</text>
            <text class="secondary-entry__desc">用 {{ total }} 道题了解自己的性格模式</text>
          </view>
          <text class="secondary-entry__arrow" aria-hidden="true">→</text>
        </view>
        <view
          class="secondary-entry"
          role="button"
          tabindex="0"
          hover-class="control--pressed"
          @click="activateAction(goBooking, $event)"
          @keydown="onActionKeydown($event, goBooking)"
        >
          <view>
            <text class="secondary-entry__title">预约咨询</text>
            <text class="secondary-entry__desc">选择合适的时间与老师交流</text>
          </view>
          <text class="secondary-entry__arrow" aria-hidden="true">→</text>
        </view>
      </nav>
    </main>
  </view>
</template>

<style scoped>
.home {
  --home-bg: #F6F1E7;
  --home-surface: #FFFDF8;
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
  padding: 28rpx 24rpx 52rpx;
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
  color: #8C3C30;
  border-bottom: 2rpx solid rgba(140, 60, 48, .24);
}

.home-retry {
  flex: 0 0 auto;
  min-height: 88rpx;
  padding: 0 20rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #8C3C30;
  font-size: 23rpx;
  font-weight: 700;
}

.teacher-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 30rpx;
  padding: 24rpx 0 48rpx;
}

.teacher-hero__portrait {
  width: 100%;
  aspect-ratio: 4 / 5;
  overflow: hidden;
  border-radius: 24rpx;
  background: var(--home-surface);
}

.teacher-hero__image {
  display: block;
  width: 100%;
  height: 100%;
}

.teacher-hero__copy {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 14rpx;
}

.teacher-hero__name {
  color: var(--home-ink);
  font-size: 48rpx;
  font-weight: 800;
  line-height: 1.2;
}

.teacher-hero__identity {
  color: var(--home-green);
  font-size: 25rpx;
  font-weight: 700;
  line-height: 1.5;
}

.teacher-hero__bio {
  color: #59615D;
  font-size: 27rpx;
  line-height: 1.72;
}

.home-primary {
  width: 100%;
  min-height: 88rpx;
  margin-top: 12rpx;
  padding: 0 32rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  border-radius: 20rpx;
  background: var(--home-green);
  color: #FFFFFF;
  font-size: 28rpx;
  font-weight: 800;
  touch-action: manipulation;
}

.course-section {
  padding: 8rpx 0 36rpx;
}

.course-section__title {
  display: block;
  margin-bottom: 20rpx;
  color: var(--home-ink);
  font-size: 32rpx;
  font-weight: 800;
  line-height: 1.35;
}

.featured-course {
  width: 100%;
  min-width: 0;
  min-height: 88rpx;
  padding: 18rpx;
  display: grid;
  grid-template-columns: 176rpx minmax(0, 1fr);
  align-items: center;
  gap: 22rpx;
  box-sizing: border-box;
  border: 2rpx solid rgba(51, 91, 74, .24);
  border-radius: 20rpx;
  background: var(--home-surface);
  color: inherit;
  text-align: left;
  touch-action: manipulation;
}

.featured-course__cover {
  display: block;
  width: 176rpx;
  height: 176rpx;
  border-radius: 16rpx;
  background: var(--home-bg);
}

.featured-course__copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}

.featured-course__title {
  color: var(--home-ink);
  font-size: 29rpx;
  font-weight: 800;
  line-height: 1.38;
}

.featured-course__desc {
  color: #59615D;
  font-size: 23rpx;
  line-height: 1.55;
}

.featured-course__meta {
  color: var(--home-green);
  font-size: 21rpx;
  font-weight: 700;
  line-height: 1.4;
}

.secondary-nav {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  padding-top: 8rpx;
}

.secondary-entry {
  width: 100%;
  min-height: 88rpx;
  padding: 18rpx 22rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
  box-sizing: border-box;
  border: 2rpx solid rgba(51, 91, 74, .28);
  border-radius: 16rpx;
  background: transparent;
  color: var(--home-ink);
  text-align: left;
  touch-action: manipulation;
}

.secondary-entry > view {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4rpx;
}

.secondary-entry__title {
  font-size: 27rpx;
  font-weight: 800;
  line-height: 1.4;
}

.secondary-entry__desc {
  color: #66706B;
  font-size: 22rpx;
  line-height: 1.5;
}

.secondary-entry__arrow {
  flex: 0 0 auto;
  color: var(--home-green);
  font-size: 30rpx;
}

.home-empty {
  padding: 24rpx;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  border: 2rpx solid rgba(51, 91, 74, .2);
  border-radius: 16rpx;
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

.teacher-hero__empty {
  grid-column: 1 / -1;
}

.control--pressed {
  opacity: .74;
}

.home-primary:focus-visible,
.home-retry:focus-visible,
.featured-course:focus-visible,
.secondary-entry:focus-visible {
  outline: 4rpx solid #176B58;
  outline-offset: 4rpx;
}

@media screen and (min-width: 768px) {
  .home__content {
    max-width: 1240rpx;
    padding-left: 40rpx;
    padding-right: 40rpx;
  }

  .teacher-hero {
    grid-template-columns: minmax(0, 1fr) minmax(0, .9fr);
    align-items: center;
    gap: 48rpx;
  }

  .home-primary {
    width: 320rpx;
  }
}
</style>
