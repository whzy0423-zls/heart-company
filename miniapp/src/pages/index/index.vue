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
const materialCourses = computed(() => courses.value.slice(1))
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

function goRelation() {
  uni.navigateTo({ url: '/pages/relation/relation' })
}
</script>

<template>
  <view class="home nx-page page-stack ios-page ios-safe-bottom">
    <view class="home__paper">
      <view class="home-masthead">
        <text class="home-masthead__brand">九型芯之力</text>
        <text class="home-masthead__edition">学习专刊 · 09</text>
      </view>

      <view v-if="loading" class="sync-note" role="status">
        <text class="sync-note__line" aria-hidden="true" />
        <text>正在更新老师与课程资料…</text>
      </view>

      <view v-if="loadError" class="home-error" role="status">
        <text>{{ loadError }}</text>
        <view
          class="home-retry"
          role="button"
          tabindex="0"
          hover-class="control--pressed"
          @click="activateAction(loadContent, $event)"
          @keydown.enter.stop.prevent="activateAction(loadContent, $event)"
          @keydown.space.stop.prevent="activateAction(loadContent, $event)"
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
          <view class="teacher-hero__portrait-mark" aria-hidden="true">
            <text>主讲</text>
            <text>TEACHER</text>
          </view>
        </view>

        <view v-if="teacher" class="teacher-hero__copy">
          <text class="editorial-kicker">跟着真正懂九型的人学习</text>
          <text id="teacher-heading" class="teacher-hero__name">{{ teacher.name }}</text>
          <text class="teacher-hero__identity">{{ teacher.title }}</text>
          <view class="teacher-hero__rule" aria-hidden="true" />
          <text class="teacher-hero__bio">{{ teacher.bio }}</text>
          <view v-if="teacher.tags.length" class="teacher-hero__tags" aria-label="老师擅长领域">
            <text v-for="tag in teacher.tags" :key="tag" class="teacher-hero__tag">{{ tag }}</text>
          </view>
          <view
            class="home-primary"
            role="button"
            tabindex="0"
            hover-class="control--pressed"
            @click="activateAction(goLearn, $event)"
            @keydown.enter.stop.prevent="activateAction(goLearn, $event)"
            @keydown.space.stop.prevent="activateAction(goLearn, $event)"
          >开始学习</view>
        </view>

        <view v-else class="editorial-empty teacher-hero__empty">
          <text class="editorial-empty__title">老师资料整理中</text>
          <text>课程团队正在完善主讲老师介绍，稍后再来看看。</text>
        </view>
      </section>

      <section class="course-section" aria-labelledby="course-heading">
        <view class="section-heading">
          <view>
            <text class="editorial-kicker">CURRICULUM / 课程编排</text>
            <text id="course-heading" class="section-heading__title">从一门课，走进完整学习路径</text>
          </view>
          <text class="section-heading__note">理论 · 觉察 · 关系 · 成长</text>
        </view>

        <view v-if="courses.length" class="course-layout">
          <view
            v-if="featuredCourse"
            class="featured-course"
            role="button"
            tabindex="0"
            :aria-label="`查看课程：${featuredCourse.title}`"
            hover-class="control--pressed"
            @click="activateAction(goLearn, $event)"
            @keydown.enter.stop.prevent="activateAction(goLearn, $event)"
            @keydown.space.stop.prevent="activateAction(goLearn, $event)"
          >
            <view class="featured-course__visual">
              <image
                class="featured-course__cover"
                :src="courseImages[0]"
                mode="aspectFill"
                aria-hidden="true"
                @error="onCourseImageError(0)"
              />
              <text class="featured-course__spine" aria-hidden="true">九型芯之力</text>
            </view>
            <view class="featured-course__copy">
              <text class="featured-course__label">本期主课 / FEATURED</text>
              <text class="featured-course__title">{{ featuredCourse.title }}</text>
              <text class="featured-course__desc">{{ featuredCourse.description }}</text>
              <view class="course-meta">
                <text>{{ featuredCourse.badge }}</text>
                <text>{{ featuredCourse.duration }}</text>
              </view>
              <view v-if="featuredCourse.materialTypes.length" class="material-types">
                <text v-for="type in featuredCourse.materialTypes" :key="type">{{ type }}</text>
              </view>
              <view v-if="featuredCourse.bullets.length" class="course-bullets">
                <text v-for="bullet in featuredCourse.bullets" :key="bullet">— {{ bullet }}</text>
              </view>
              <text class="featured-course__link">查看课程资料 →</text>
            </view>
          </view>

          <view class="material-shelf">
            <view class="material-shelf__heading">
              <text>课件与材料</text>
              <text>MATERIAL SHELF</text>
            </view>

            <view
              v-for="(course, index) in materialCourses"
              :key="course.title + index"
              class="material-card"
              role="button"
              tabindex="0"
              :aria-label="`查看资料：${course.title}`"
              hover-class="control--pressed"
              @click="activateAction(goLearn, $event)"
              @keydown.enter.stop.prevent="activateAction(goLearn, $event)"
              @keydown.space.stop.prevent="activateAction(goLearn, $event)"
            >
              <image
                class="material-card__cover"
                :class="`material-card__cover--${index % 2 ? 'landscape' : 'book'}`"
                :src="courseImages[index + 1]"
                mode="aspectFill"
                aria-hidden="true"
                lazy-load
                @error="onCourseImageError(index + 1)"
              />
              <view class="material-card__copy">
                <view class="material-card__topline">
                  <text>{{ course.badge }}</text>
                  <text>{{ course.duration }}</text>
                </view>
                <text class="material-card__title">{{ course.title }}</text>
                <text class="material-card__desc">{{ course.description }}</text>
                <view v-if="course.materialTypes.length" class="material-types material-types--small">
                  <text v-for="type in course.materialTypes" :key="type">{{ type }}</text>
                </view>
                <view v-if="course.bullets.length" class="course-bullets course-bullets--small">
                  <text v-for="bullet in course.bullets" :key="bullet">— {{ bullet }}</text>
                </view>
              </view>
            </view>

            <view v-if="materialCourses.length === 0" class="editorial-empty editorial-empty--compact">
              <text class="editorial-empty__title">其余课件资料整理中</text>
            </view>
          </view>
        </view>

        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">课程资料整理中</text>
          <text>新一期课程与配套材料正在编排，稍后再来看看。</text>
        </view>
      </section>

      <nav class="secondary-nav" aria-label="更多九型工具">
        <view
          class="secondary-entry"
          role="button"
          tabindex="0"
          hover-class="control--pressed"
          @click="activateAction(startTest, $event)"
          @keydown.enter.stop.prevent="activateAction(startTest, $event)"
          @keydown.space.stop.prevent="activateAction(startTest, $event)"
        >
          <view>
            <text class="secondary-entry__kicker">SELF TEST · {{ total }} 题</text>
            <text class="secondary-entry__title">九型自测</text>
          </view>
          <text aria-hidden="true">→</text>
        </view>
        <view
          class="secondary-entry"
          role="button"
          tabindex="0"
          hover-class="control--pressed"
          @click="activateAction(goRelation, $event)"
          @keydown.enter.stop.prevent="activateAction(goRelation, $event)"
          @keydown.space.stop.prevent="activateAction(goRelation, $event)"
        >
          <view>
            <text class="secondary-entry__kicker">RELATIONSHIP</text>
            <text class="secondary-entry__title">关系合盘</text>
          </view>
          <text aria-hidden="true">→</text>
        </view>
      </nav>
    </view>
  </view>
</template>

<style scoped>
.home {
  --paper: #F6F0E4;
  --paper-deep: #E9DDC8;
  --ink: #24241F;
  --muted: #645F55;
  --green: #173F35;
  --green-light: #D9E2D8;
  --cinnabar: #A93E2D;
  --sand: #D0B889;
  min-width: 0;
  overflow-x: hidden;
  background: var(--paper);
  color: var(--ink);
}

.home__paper {
  width: 100%;
  max-width: 980rpx;
  margin: 0 auto;
  padding: 20rpx 24rpx 52rpx;
  box-sizing: border-box;
  background:
    repeating-linear-gradient(0deg, transparent 0, transparent 15rpx, rgba(36, 36, 31, .018) 16rpx),
    var(--paper);
}

.home-masthead {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 20rpx;
  padding: 20rpx 0 16rpx;
  border-bottom: 2rpx solid var(--ink);
}

.home-masthead__brand {
  font-family: "Songti SC", "STSong", serif;
  font-size: 35rpx;
  font-weight: 900;
  letter-spacing: 4rpx;
}

.home-masthead__edition,
.editorial-kicker,
.section-heading__note,
.secondary-entry__kicker {
  color: var(--muted);
  font-size: 20rpx;
  font-weight: 800;
  line-height: 1.4;
  letter-spacing: 2rpx;
}

.sync-note,
.home-error {
  min-height: 66rpx;
  display: flex;
  align-items: center;
  gap: 16rpx;
  padding: 10rpx 0;
  color: var(--muted);
  font-size: 22rpx;
  box-sizing: border-box;
}

.sync-note__line {
  width: 36rpx;
  height: 2rpx;
  background: var(--cinnabar);
}

.home-error {
  justify-content: space-between;
  color: #752A20;
  border-bottom: 2rpx solid rgba(117, 42, 32, .24);
}

.home-retry {
  flex: 0 0 auto;
  min-height: 88rpx;
  margin: 0;
  padding: 0 22rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  background: transparent;
  color: #752A20;
  font-size: 23rpx;
  font-weight: 900;
}

.home-retry::after,
.home-primary::after,
.featured-course::after,
.material-card::after,
.secondary-entry::after {
  border: 0;
}

.teacher-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  border-bottom: 2rpx solid var(--ink);
}

.teacher-hero__portrait {
  position: relative;
  width: 100%;
  aspect-ratio: 4 / 5;
  overflow: hidden;
  background: var(--paper-deep);
}

.teacher-hero__image {
  display: block;
  width: 100%;
  height: 100%;
}

.teacher-hero__portrait-mark {
  position: absolute;
  left: 22rpx;
  bottom: 22rpx;
  padding: 12rpx 16rpx;
  display: flex;
  flex-direction: column;
  background: var(--cinnabar);
  color: #FFFFFF;
  font-size: 18rpx;
  font-weight: 900;
  line-height: 1.35;
  letter-spacing: 1rpx;
}

.teacher-hero__copy {
  padding: 36rpx 4rpx 42rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16rpx;
}

.editorial-kicker { color: var(--cinnabar); }

.teacher-hero__name {
  color: var(--ink);
  font-family: "Songti SC", "STSong", serif;
  font-size: 62rpx;
  font-weight: 900;
  line-height: 1.12;
  letter-spacing: 2rpx;
}

.teacher-hero__identity {
  color: var(--green);
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.5;
}

.teacher-hero__rule {
  width: 82rpx;
  height: 4rpx;
  margin: 4rpx 0;
  background: var(--sand);
}

.teacher-hero__bio {
  max-width: 620rpx;
  color: #4E4A42;
  font-size: 27rpx;
  line-height: 1.75;
}

.teacher-hero__tags,
.material-types {
  display: flex;
  flex-wrap: wrap;
  gap: 10rpx;
}

.teacher-hero__tag,
.material-types text {
  padding: 7rpx 14rpx;
  border: 2rpx solid rgba(23, 63, 53, .36);
  color: var(--green);
  font-size: 21rpx;
  font-weight: 800;
  line-height: 1.3;
}

.home-primary {
  width: 100%;
  min-height: 88rpx;
  margin: 14rpx 0 0;
  padding: 0 36rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 4rpx;
  background: var(--green);
  color: #FFFFFF;
  font-size: 28rpx;
  font-weight: 900;
  letter-spacing: 2rpx;
  box-shadow: 8rpx 8rpx 0 var(--sand);
  transition: opacity .2s ease, transform .2s ease;
  touch-action: manipulation;
}

.teacher-hero__empty { grid-column: 1 / -1; }

.course-section { padding: 50rpx 0 10rpx; }

.section-heading {
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  padding-bottom: 24rpx;
  border-bottom: 4rpx solid var(--green);
}

.section-heading__title {
  display: block;
  max-width: 660rpx;
  margin-top: 10rpx;
  font-family: "Songti SC", "STSong", serif;
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.32;
}

.course-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
}

.featured-course {
  width: 100%;
  min-width: 0;
  min-height: 88rpx;
  margin: 0;
  padding: 32rpx 0 38rpx;
  display: grid;
  grid-template-columns: 210rpx minmax(0, 1fr);
  gap: 26rpx;
  border: 0;
  border-bottom: 2rpx solid rgba(36, 36, 31, .3);
  border-radius: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  line-height: inherit;
  text-align: left;
  transition: opacity .2s ease, transform .2s ease;
  touch-action: manipulation;
}

.featured-course__visual {
  position: relative;
  align-self: start;
  width: 210rpx;
  aspect-ratio: 3 / 4;
  padding-left: 12rpx;
  box-sizing: border-box;
  background: var(--green);
  box-shadow: 10rpx 12rpx 0 rgba(208, 184, 137, .7);
}

.featured-course__cover {
  display: block;
  width: 100%;
  height: 100%;
  background: var(--green-light);
}

.featured-course__spine {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 13rpx;
  overflow: hidden;
  color: transparent;
  background: var(--green);
}

.featured-course__copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 12rpx;
}

.featured-course__label {
  color: var(--cinnabar);
  font-size: 19rpx;
  font-weight: 900;
  letter-spacing: 1rpx;
}

.featured-course__title {
  color: var(--ink);
  font-family: "Songti SC", "STSong", serif;
  font-size: 35rpx;
  font-weight: 900;
  line-height: 1.3;
}

.featured-course__desc,
.material-card__desc {
  color: var(--muted);
  font-size: 24rpx;
  line-height: 1.62;
}

.course-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8rpx 18rpx;
  color: var(--green);
  font-size: 21rpx;
  font-weight: 800;
}

.course-bullets {
  display: flex;
  flex-direction: column;
  gap: 6rpx;
  color: #4E4A42;
  font-size: 22rpx;
  line-height: 1.5;
}

.featured-course__link {
  margin-top: auto;
  padding-top: 8rpx;
  color: var(--cinnabar);
  font-size: 22rpx;
  font-weight: 900;
}

.material-shelf { min-width: 0; }

.material-shelf__heading {
  min-height: 74rpx;
  padding: 18rpx 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16rpx;
  border-bottom: 2rpx solid var(--ink);
  box-sizing: border-box;
  color: var(--ink);
  font-size: 25rpx;
  font-weight: 900;
}

.material-shelf__heading text:last-child {
  color: var(--muted);
  font-size: 18rpx;
  letter-spacing: 1rpx;
}

.material-card {
  width: 100%;
  min-width: 0;
  min-height: 88rpx;
  margin: 0;
  padding: 24rpx 0;
  display: flex;
  align-items: flex-start;
  gap: 20rpx;
  border: 0;
  border-bottom: 2rpx solid rgba(36, 36, 31, .22);
  border-radius: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  line-height: inherit;
  text-align: left;
  transition: opacity .2s ease, transform .2s ease;
  touch-action: manipulation;
}

.material-card__cover {
  flex: 0 0 126rpx;
  width: 126rpx;
  aspect-ratio: 3 / 4;
  background: var(--paper-deep);
  box-shadow: 5rpx 6rpx 0 rgba(23, 63, 53, .17);
}

.material-card__cover--landscape {
  flex-basis: 150rpx;
  width: 150rpx;
  aspect-ratio: 4 / 3;
}

.material-card__copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}

.material-card__topline {
  display: flex;
  flex-wrap: wrap;
  gap: 8rpx 14rpx;
  color: var(--cinnabar);
  font-size: 19rpx;
  font-weight: 800;
}

.material-card__title {
  color: var(--ink);
  font-size: 29rpx;
  font-weight: 900;
  line-height: 1.35;
}

.material-types--small text {
  padding: 4rpx 10rpx;
  font-size: 18rpx;
}

.course-bullets--small { font-size: 20rpx; }

.editorial-empty {
  min-height: 220rpx;
  padding: 36rpx 4rpx;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 10rpx;
  border-bottom: 2rpx solid var(--ink);
  color: var(--muted);
  font-size: 24rpx;
  line-height: 1.6;
}

.editorial-empty--compact {
  min-height: 120rpx;
  padding: 24rpx 0;
  border-bottom: 0;
}

.editorial-empty__title {
  color: var(--ink);
  font-family: "Songti SC", "STSong", serif;
  font-size: 32rpx;
  font-weight: 900;
}

.secondary-nav {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  margin-top: 38rpx;
  border-top: 2rpx solid var(--ink);
}

.secondary-entry {
  width: 100%;
  min-width: 0;
  min-height: 88rpx;
  margin: 0;
  padding: 24rpx 4rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16rpx;
  border: 0;
  border-bottom: 2rpx solid rgba(36, 36, 31, .3);
  border-radius: 0;
  background: transparent;
  color: var(--ink);
  font: inherit;
  line-height: inherit;
  text-align: left;
  transition: opacity .2s ease, transform .2s ease;
  touch-action: manipulation;
}

.secondary-entry view {
  display: flex;
  flex-direction: column;
  gap: 5rpx;
}

.secondary-entry__title {
  display: block;
  font-size: 29rpx;
  font-weight: 900;
}

.control--pressed { opacity: .78; transform: scale(.985); }

.home-primary:focus-visible,
.home-retry:focus-visible,
.featured-course:focus-visible,
.material-card:focus-visible,
.secondary-entry:focus-visible {
  outline: 4rpx solid #176B58;
  outline-offset: 5rpx;
}

@media screen and (min-width: 768px) {
  .home__paper {
    max-width: 1280rpx;
    padding-left: 42rpx;
    padding-right: 42rpx;
  }

  .teacher-hero {
    grid-template-columns: minmax(0, 1.08fr) minmax(0, .92fr);
    align-items: stretch;
  }

  .teacher-hero__portrait { min-height: 720rpx; }

  .teacher-hero__copy {
    justify-content: center;
    padding: 54rpx 0 54rpx 52rpx;
  }

  .home-primary { width: 300rpx; }

  .section-heading {
    flex-direction: row;
    align-items: flex-end;
    justify-content: space-between;
  }

  .course-layout {
    grid-template-columns: minmax(0, .94fr) minmax(0, 1.06fr);
    gap: 46rpx;
  }

  .featured-course {
    align-content: start;
    grid-template-columns: minmax(0, 1fr);
    padding-right: 42rpx;
    border-right: 2rpx solid rgba(36, 36, 31, .28);
    border-bottom: 0;
  }

  .featured-course__visual {
    width: min(100%, 390rpx);
    justify-self: center;
  }

  .material-shelf { padding-top: 14rpx; }

  .secondary-nav { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .secondary-entry:first-child { padding-right: 34rpx; border-right: 2rpx solid var(--ink); }
  .secondary-entry:last-child { padding-left: 34rpx; }
}

@media (prefers-reduced-motion: reduce) {
  .home-primary,
  .home-retry,
  .featured-course,
  .material-card,
  .secondary-entry {
    transition: none;
  }
}
</style>
