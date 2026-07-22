<script setup>
import { computed, onMounted, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { TYPES_INFO } from '../../data/enneagramGame'
import { resolveContentAsset } from '../../utils/contentAsset'
import { readLearningNavIntent } from '../../utils/learningNavIntent'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import {
  applyLearningContent,
  createActionActivationGuard,
  createInitialLearningContent,
  createLatestRequestGuard,
  createOneShotFallbackRegistry,
  flattenLearningMaterials,
  handleActionKeydown,
  resolveLearningCategory,
  retainLearningContentOnError,
} from '../../utils/learningPageState'
import { DEFAULT_TEACHERS } from '../../utils/teacherCourseware'
import { userErrorMessage } from '../../utils/userMessage'

const TEACHER_FALLBACK = DEFAULT_TEACHERS[0].avatar
const COURSE_FALLBACKS = [
  '/static/editorial/course-intro.webp',
  '/static/editorial/course-growth.webp',
  '/static/editorial/course-relation.webp',
]
const CATEGORY_ORDER = ['course', 'material', 'quote']
const initialContent = createInitialLearningContent()
const teachers = ref(initialContent.teachers)
const coursewareItems = ref(initialContent.coursewareItems)
const quotes = ref(initialContent.quotes)
const types = ref(Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] })))
const activeCategory = ref('course')
const teacherExpanded = ref(false)
const loading = ref(true)
const loadError = ref('')
const teacherImage = ref(TEACHER_FALLBACK)
const courseImages = ref([])
const teacherImageFallbackUsed = createOneShotFallbackRegistry()
const courseImageFallbackUsed = createOneShotFallbackRegistry()
const requestGuard = createLatestRequestGuard()
const actionActivationGuard = createActionActivationGuard()

const teacher = computed(() => teachers.value[0] || null)
const teacherImageLabel = computed(() => teacher.value ? `${teacher.value.name}老师肖像` : '主讲老师肖像')
const materialItems = computed(() => flattenLearningMaterials(coursewareItems.value))

function courseFallback(index) {
  return COURSE_FALLBACKS[index % COURSE_FALLBACKS.length]
}

function learnCourseCover(course, index) {
  const cover = typeof course?.cover === 'string' ? course.cover.trim() : ''
  const isLegacyWheel = /^\/static\/wheel\.png(?:[?#].*)?$/i.test(cover)
  return !cover || isLegacyWheel ? courseFallback(index) : cover
}

function syncContentImages() {
  teacherImageFallbackUsed.reset()
  courseImageFallbackUsed.reset()
  teacherImage.value = resolveContentAsset(teacher.value?.avatar, TEACHER_FALLBACK)
  courseImages.value = coursewareItems.value.map((course, index) => (
    resolveContentAsset(learnCourseCover(course, index), courseFallback(index))
  ))
}

function applyContent(config, options = {}) {
  const next = applyLearningContent({
    teachers: teachers.value,
    coursewareItems: coursewareItems.value,
    quotes: quotes.value,
  }, config, options)
  teachers.value = next.teachers
  coursewareItems.value = next.coursewareItems
  quotes.value = next.quotes
  syncContentImages()
}

function onTeacherImageError() {
  if (!teacherImageFallbackUsed.consume('portrait')) return
  teacherImage.value = TEACHER_FALLBACK
}

function onCourseImageError(index) {
  if (!courseImageFallbackUsed.consume(`course:${index}`)) return
  courseImages.value[index] = courseFallback(index)
}

function activateAction(action, event) {
  if (actionActivationGuard.shouldActivate(event)) action()
}

function onActionKeydown(event, action) {
  handleActionKeydown(event, () => activateAction(action, event))
}

function toggleTeacher() {
  teacherExpanded.value = !teacherExpanded.value
}

function selectCategory(category) {
  activeCategory.value = resolveLearningCategory(activeCategory.value, category)
}

function onTabKeydown(event, category) {
  const key = event?.key
  if (['Enter', ' ', 'Spacebar'].includes(key)) {
    event.preventDefault?.()
    event.stopPropagation?.()
    selectCategory(category)
    return
  }
  if (!['ArrowLeft', 'ArrowRight'].includes(key)) return
  event.preventDefault?.()
  event.stopPropagation?.()
  const currentIndex = CATEGORY_ORDER.indexOf(category)
  const step = key === 'ArrowRight' ? 1 : -1
  const nextIndex = (currentIndex + step + CATEGORY_ORDER.length) % CATEGORY_ORDER.length
  const nextCategory = CATEGORY_ORDER[nextIndex]
  selectCategory(nextCategory)
  event?.currentTarget?.parentElement?.children?.[nextIndex]?.focus?.()
}

function consumeNavigationIntent() {
  activeCategory.value = resolveLearningCategory(activeCategory.value, readLearningNavIntent())
}

async function loadContent(options = {}) {
  const silent = !!options.silent
  const ticket = requestGuard.issue()
  if (!silent) loading.value = true
  loadError.value = ''
  try {
    const config = await refreshSiteConfig()
    if (!requestGuard.isLatest(ticket)) return
    applyContent(config, { preserveMissing: true })
  } catch (error) {
    if (!requestGuard.isLatest(ticket)) return
    const retained = retainLearningContentOnError({
      teachers: teachers.value,
      coursewareItems: coursewareItems.value,
      quotes: quotes.value,
    }, userErrorMessage(error, '内容更新失败，当前资料仍可继续浏览'))
    loadError.value = retained.loadError
  } finally {
    if (requestGuard.isLatest(ticket)) loading.value = false
  }
}

onShow(consumeNavigationIntent)

onMounted(() => {
  const cached = getStoredSiteConfig()
  if (cached) {
    applyContent(cached)
    loading.value = false
  } else {
    syncContentImages()
  }
  loadContent({ silent: !!cached })
})
</script>

<template>
  <view class="wrap learn page-stack ios-page ios-safe-bottom">
    <view class="learn-content">
      <header class="learn-header">
        <text class="learn-header__title">学习中心</text>
        <text class="learn-header__intro">课程、课件与老师语录，按分类随时查阅。</text>
      </header>

      <view v-if="loading" class="learn-sync" role="status">
        <text>正在更新老师与课程资料，先读本地内容…</text>
      </view>

      <view v-if="loadError" class="learn-error" role="status">
        <text>{{ loadError }}</text>
        <view
          class="learn-retry"
          role="button"
          tabindex="0"
          hover-class="control--pressed"
          @click="activateAction(loadContent, $event)"
          @keydown="onActionKeydown($event, loadContent)"
        >重试更新</view>
      </view>

      <section class="learn-teacher" aria-labelledby="learn-teacher-heading">
        <view v-if="teacher" class="learn-teacher__content">
          <view class="learn-teacher__summary">
            <view class="learn-teacher__portrait">
              <image
                class="learn-teacher__image"
                :src="teacherImage"
                mode="aspectFill"
                role="img"
                :aria-label="teacherImageLabel"
                @error="onTeacherImageError"
              />
            </view>
            <view class="learn-teacher__identity">
              <text class="section-label">主讲老师</text>
              <text id="learn-teacher-heading" class="learn-teacher__name">{{ teacher.name }}</text>
              <text class="learn-teacher__title">{{ teacher.title }}</text>
            </view>
            <view
              class="learn-teacher__toggle"
              role="button"
              tabindex="0"
              :aria-expanded="teacherExpanded"
              aria-controls="learn-teacher-details"
              hover-class="control--pressed"
              @click="activateAction(toggleTeacher, $event)"
              @keydown="onActionKeydown($event, toggleTeacher)"
            >{{ teacherExpanded ? '收起' : '了解老师' }}</view>
          </view>
          <view v-if="teacherExpanded" id="learn-teacher-details" class="learn-teacher__details">
            <text class="learn-teacher__bio">{{ teacher.bio }}</text>
            <view v-if="teacher.tags.length" class="learn-teacher__tags" aria-label="老师擅长领域">
              <text v-for="(tag, tagIndex) in teacher.tags" :key="`${teacher.name}::tag::${tagIndex}::${tag}`" class="learn-teacher__tag">{{ tag }}</text>
            </view>
          </view>
        </view>

        <view v-else class="editorial-empty learn-teacher__empty">
          <text id="learn-teacher-heading" class="editorial-empty__title">老师资料整理中</text>
          <text>课程团队正在整理主讲老师的介绍与研究方向。</text>
        </view>
      </section>

      <view class="learn-tabs" role="tablist" aria-label="学习内容分类">
        <view class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'course' }" role="tab" data-category="course" :aria-selected="activeCategory === 'course'" :tabindex="activeCategory === 'course' ? 0 : -1" hover-class="learn-tab--pressed" @click="selectCategory('course')" @keydown="onTabKeydown($event, 'course')">课程</view>
        <view class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'material' }" role="tab" data-category="material" :aria-selected="activeCategory === 'material'" :tabindex="activeCategory === 'material' ? 0 : -1" hover-class="learn-tab--pressed" @click="selectCategory('material')" @keydown="onTabKeydown($event, 'material')">课件</view>
        <view class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'quote' }" role="tab" data-category="quote" :aria-selected="activeCategory === 'quote'" :tabindex="activeCategory === 'quote' ? 0 : -1" hover-class="learn-tab--pressed" @click="selectCategory('quote')" @keydown="onTabKeydown($event, 'quote')">语录</view>
      </view>

      <section v-if="activeCategory === 'course'" class="learning-panel" aria-labelledby="course-panel-heading">
        <text id="course-panel-heading" class="panel-title">全部课程</text>
        <view v-if="coursewareItems.length" class="course-list">
          <article v-for="(course, index) in coursewareItems" :key="`${course.title}::course::${index}`" class="course-row">
            <image class="course-row__cover" :src="courseImages[index]" mode="aspectFill" aria-hidden="true" lazy-load @error="onCourseImageError(index)" />
            <view class="course-row__copy">
              <text class="course-row__title">{{ course.title }}</text>
              <text v-if="course.materialTypes.length" class="course-row__materials">{{ course.materialTypes.join(' · ') }}</text>
              <text v-if="course.duration" class="course-row__duration">{{ course.duration }}</text>
              <text class="course-row__desc">{{ course.description }}</text>
            </view>
          </article>
        </view>
        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">课程内容整理中</text>
          <text>新课程上线后会在这里展示。</text>
        </view>
      </section>

      <section v-else-if="activeCategory === 'material'" class="learning-panel" aria-labelledby="material-panel-heading">
        <text id="material-panel-heading" class="panel-title">全部课件</text>
        <view v-if="materialItems.length" class="material-list">
          <article v-for="material in materialItems" :key="material.key" class="material-row">
            <view class="material-row__icon" aria-hidden="true">文</view>
            <view class="material-row__copy">
              <text class="material-row__title">{{ material.courseTitle }}</text>
              <text v-if="material.description" class="material-row__desc">{{ material.description }}</text>
              <view class="material-row__meta">
                <text class="material-row__type">{{ material.type }}</text>
                <text v-if="material.duration" class="material-row__duration">{{ material.duration }}</text>
              </view>
            </view>
          </article>
        </view>
        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">课件资料整理中</text>
          <text>讲义、音频与练习资料上线后会在这里展示。</text>
        </view>
      </section>

      <section v-else-if="activeCategory === 'quote'" class="learning-panel" aria-labelledby="quote-panel-heading">
        <text id="quote-panel-heading" class="panel-title">老师语录</text>
        <view v-if="quotes.length" class="quote-list">
          <article v-for="(quote, quoteIndex) in quotes" :key="`quote::${quoteIndex}::${quote}`" class="quote-card">
            <text class="quote-card__mark">”</text>
            <text class="quote-card__text">{{ quote }}</text>
          </article>
        </view>
        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">老师语录整理中</text>
          <text>课堂札记整理完成后会在这里展示。</text>
        </view>
      </section>

      <section class="type-index" aria-labelledby="type-index-heading">
        <text id="type-index-heading" class="type-index__heading">九型速查</text>
        <view class="type-index__list">
          <view v-for="type in types" :key="`type::${type.id}`" class="type-index__item">
            <text class="type-index__number">{{ type.id }}</text>
            <text class="type-index__name">{{ type.name }}</text>
          </view>
        </view>
      </section>
    </view>
  </view>
</template>

<style scoped>
.learn {
  --learn-bg: #F6F1E7;
  --learn-surface: #FFFDF8;
  --learn-ink: #20252B;
  --learn-green: #335B4A;
  --learn-cinnabar: #A43C2C;
  min-width: 0;
  overflow-x: hidden;
  background: var(--learn-bg);
  color: var(--learn-ink);
}

.learn-content {
  width: 100%;
  max-width: 980rpx;
  margin: 0 auto;
  padding: 24rpx 24rpx 56rpx;
  box-sizing: border-box;
}

.learn-header { padding: 8rpx 4rpx 20rpx; display: flex; flex-direction: column; gap: 6rpx; }
.learn-header__title { font-size: 36rpx; font-weight: 800; line-height: 1.35; }
.learn-header__intro { color: rgba(32, 37, 43, .66); font-size: 24rpx; line-height: 1.55; }

.learn-sync,
.learn-error {
  min-height: 64rpx;
  margin-bottom: 16rpx;
  padding: 10rpx 16rpx;
  display: flex;
  align-items: center;
  gap: 16rpx;
  box-sizing: border-box;
  border: 2rpx solid rgba(51, 91, 74, .18);
  border-radius: 16rpx;
  background: var(--learn-surface);
  color: rgba(32, 37, 43, .72);
  font-size: 22rpx;
  line-height: 1.5;
}

.learn-error { justify-content: space-between; color: var(--learn-cinnabar); border-color: rgba(164, 60, 44, .24); }
.learn-retry { flex: 0 0 auto; min-height: 88rpx; padding: 0 20rpx; display: inline-flex; align-items: center; color: var(--learn-cinnabar); font-size: 24rpx; font-weight: 700; touch-action: manipulation; }

.learn-teacher { min-width: 0; padding: 20rpx; border: 2rpx solid rgba(51, 91, 74, .14); border-radius: 20rpx; background: var(--learn-surface); }
.learn-teacher__content { min-width: 0; }
.learn-teacher__summary { min-width: 0; display: flex; align-items: center; gap: 18rpx; }
.learn-teacher__portrait { flex: 0 0 108rpx; width: 108rpx; aspect-ratio: 4 / 5; overflow: hidden; border-radius: 16rpx; background: var(--learn-bg); }
.learn-teacher__image { display: block; width: 100%; height: 100%; }
.learn-teacher__identity { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 5rpx; }
.section-label { color: var(--learn-green); font-size: 22rpx; font-weight: 700; line-height: 1.4; }
.learn-teacher__name { font-size: 30rpx; font-weight: 800; line-height: 1.35; }
.learn-teacher__title { color: rgba(32, 37, 43, .65); font-size: 23rpx; line-height: 1.45; }
.learn-teacher__toggle { flex: 0 0 auto; min-height: 88rpx; padding: 0 8rpx; display: inline-flex; align-items: center; color: var(--learn-green); font-size: 23rpx; font-weight: 700; touch-action: manipulation; }
.learn-teacher__details { margin-top: 18rpx; padding-top: 18rpx; display: flex; flex-direction: column; gap: 14rpx; border-top: 2rpx solid rgba(32, 37, 43, .1); }
.learn-teacher__bio { color: rgba(32, 37, 43, .76); font-size: 26rpx; line-height: 1.65; }
.learn-teacher__tags { display: flex; flex-wrap: wrap; gap: 10rpx; }
.learn-teacher__tag { padding: 6rpx 12rpx; border-radius: 16rpx; background: rgba(51, 91, 74, .08); color: var(--learn-green); font-size: 22rpx; line-height: 1.4; }

.learn-tabs { margin-top: 20rpx; padding: 6rpx; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 4rpx; border: 2rpx solid rgba(32, 37, 43, .1); border-radius: 18rpx; background: var(--learn-surface); }
.learn-tab { min-width: 0; min-height: 76rpx; display: flex; align-items: center; justify-content: center; border-radius: 14rpx; color: rgba(32, 37, 43, .62); font-size: 26rpx; font-weight: 700; touch-action: manipulation; }
.learn-tab--active { background: var(--learn-green); color: #FFFFFF; }
.learn-tab--pressed { opacity: .82; }

.learning-panel { margin-top: 16rpx; padding: 0 20rpx; border: 2rpx solid rgba(32, 37, 43, .1); border-radius: 20rpx; background: var(--learn-surface); }
.panel-title { min-height: 82rpx; display: flex; align-items: center; border-bottom: 2rpx solid rgba(32, 37, 43, .1); font-size: 28rpx; font-weight: 800; }
.course-list, .material-list, .quote-list { min-width: 0; display: flex; flex-direction: column; }

.course-row { min-width: 0; padding: 22rpx 0; display: grid; grid-template-columns: 132rpx minmax(0, 1fr); gap: 18rpx; border-bottom: 2rpx solid rgba(32, 37, 43, .1); }
.course-row:last-child, .material-row:last-child, .quote-card:last-child { border-bottom: 0; }
.course-row__cover { display: block; width: 132rpx; height: 104rpx; border-radius: 14rpx; background: var(--learn-bg); }
.course-row__copy { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 7rpx; }
.course-row__title { font-size: 28rpx; font-weight: 800; line-height: 1.4; }
.course-row__materials { color: var(--learn-green); font-size: 22rpx; line-height: 1.4; }
.course-row__duration { color: rgba(32, 37, 43, .58); font-size: 23rpx; line-height: 1.4; }
.course-row__desc { color: rgba(32, 37, 43, .7); font-size: 26rpx; line-height: 1.55; }

.material-row { min-width: 0; padding: 22rpx 0; display: flex; gap: 18rpx; border-bottom: 2rpx solid rgba(32, 37, 43, .1); }
.material-row__icon { flex: 0 0 72rpx; width: 72rpx; height: 72rpx; display: flex; align-items: center; justify-content: center; border-radius: 16rpx; background: rgba(51, 91, 74, .1); color: var(--learn-green); font-size: 25rpx; font-weight: 800; }
.material-row__copy { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 7rpx; }
.material-row__title { font-size: 28rpx; font-weight: 800; line-height: 1.4; }
.material-row__desc { color: rgba(32, 37, 43, .68); font-size: 25rpx; line-height: 1.5; }
.material-row__meta { display: flex; flex-wrap: wrap; gap: 12rpx; }
.material-row__type { color: var(--learn-green); font-size: 23rpx; font-weight: 700; }
.material-row__duration { color: rgba(32, 37, 43, .58); font-size: 23rpx; }

.quote-card { position: relative; min-width: 0; padding: 24rpx 40rpx 24rpx 6rpx; display: flex; border-bottom: 2rpx solid rgba(32, 37, 43, .1); }
.quote-card__mark { position: absolute; top: 16rpx; right: 2rpx; color: rgba(51, 91, 74, .28); font-family: Georgia, serif; font-size: 40rpx; line-height: 1; }
.quote-card__text { color: rgba(32, 37, 43, .82); font-size: 27rpx; line-height: 1.7; }

.editorial-empty { min-height: 180rpx; padding: 28rpx 0; display: flex; flex-direction: column; justify-content: center; gap: 8rpx; color: rgba(32, 37, 43, .66); font-size: 24rpx; line-height: 1.55; }
.editorial-empty__title { color: var(--learn-ink); font-size: 28rpx; font-weight: 800; }

.type-index { margin-top: 28rpx; padding: 0 4rpx; }
.type-index__heading { color: rgba(32, 37, 43, .58); font-size: 23rpx; font-weight: 700; }
.type-index__list { padding-top: 12rpx; display: flex; flex-wrap: wrap; gap: 8rpx; }
.type-index__item { padding: 6rpx 10rpx; display: flex; align-items: center; gap: 5rpx; border: 2rpx solid rgba(51, 91, 74, .15); border-radius: 14rpx; color: rgba(51, 91, 74, .76); font-size: 22rpx; line-height: 1.35; }
.type-index__number, .type-index__name { font-weight: 700; }
.control--pressed { opacity: .82; transform: translateY(2rpx); }

.learn-retry:focus-visible,
.learn-teacher__toggle:focus-visible,
.learn-tab:focus-visible { outline: 4rpx solid var(--learn-cinnabar); outline-offset: 2rpx; }

@media screen and (min-width: 768px) {
  .learn-content { padding-left: 48rpx; padding-right: 48rpx; }
  .learn-teacher { padding-left: 28rpx; padding-right: 28rpx; }
  .learning-panel { padding-left: 28rpx; padding-right: 28rpx; }
}
</style>
