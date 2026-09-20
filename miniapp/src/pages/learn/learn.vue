<script setup>
import { computed, onMounted, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { listClassroomRecentApi } from '../../api'
import { TYPES_INFO } from '../../data/enneagramGame'
import { resolveContentAsset } from '../../utils/contentAsset'
import { readLearningNavIntent } from '../../utils/learningNavIntent'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import {
  applyLearningContent,
  createActionActivationGuard,
  createInitialLearningContent,
  createLatestRequestGuard,
  createLearningCourseEntries,
  createLearningQuoteEntries,
  createLearningTagEntries,
  createOneShotFallbackRegistry,
  flattenLearningMaterials,
  handleActionKeydown,
  learningTabTransition,
  resolveLearningCategory,
  retainLearningContentOnError,
} from '../../utils/learningPageState'
import { DEFAULT_TEACHERS } from '../../utils/teacherCourseware'
import { mapPublishedClassroomItems } from '../../utils/classroomCourseware'
import { classroomContentRoute } from '../../utils/classroomDisplay'
import { normalizeMiniappLearn } from '../../utils/miniappPages'
import { userErrorMessage } from '../../utils/userMessage'

const TEACHER_FALLBACK = ''
const COURSE_FALLBACKS = [
  '/static/editorial/course-intro.webp',
  '/static/editorial/course-growth.webp',
  '/static/editorial/course-relation.webp',
]
const initialContent = createInitialLearningContent()
const teachers = ref(initialContent.teachers)
const coursewareItems = ref([])
const classroomEnabled = ref(true)
const quotes = ref(initialContent.quotes)
const types = ref(Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] })))
const activeCategory = ref('course')
const teacherExpanded = ref(false)
const loading = ref(true)
const loadError = ref('')
const teacherImage = ref(TEACHER_FALLBACK)
const courseImages = ref({})
const teacherImageFallbackUsed = createOneShotFallbackRegistry()
const courseImageFallbackUsed = createOneShotFallbackRegistry()
const requestGuard = createLatestRequestGuard()
const actionActivationGuard = createActionActivationGuard()

const teacher = computed(() => teachers.value[0] || null)
const teacherImageLabel = computed(() => teacher.value ? `${teacher.value.name}老师肖像` : '主讲老师肖像')
const courseEntries = computed(() => createLearningCourseEntries(coursewareItems.value))
const materialItems = computed(() => flattenLearningMaterials(coursewareItems.value))
const quoteEntries = computed(() => createLearningQuoteEntries(quotes.value))
const teacherTagEntries = computed(() => createLearningTagEntries(
  teacher.value?.tags,
  `${teacher.value?.name || ''}::${teacher.value?.title || ''}`,
))

function courseFallback(courseKey) {
  const slot = [...String(courseKey)].reduce((total, character) => total + character.charCodeAt(0), 0)
  return COURSE_FALLBACKS[slot % COURSE_FALLBACKS.length]
}

function learnCourseCover(course, courseKey) {
  const cover = typeof course?.cover === 'string' ? course.cover.trim() : ''
  const isLegacyWheel = /^\/static\/wheel\.png(?:[?#].*)?$/i.test(cover)
  return !cover || isLegacyWheel ? courseFallback(courseKey) : cover
}

function syncContentImages() {
  teacherImageFallbackUsed.reset()
  courseImageFallbackUsed.reset()
  const portrait = teacher.value?.avatar === '/static/avatars/9.png' ? '' : teacher.value?.avatar
  teacherImage.value = resolveContentAsset(portrait, TEACHER_FALLBACK)
  courseImages.value = Object.fromEntries(courseEntries.value.map(({ key: courseKey, item: course }) => [
    courseKey,
    resolveContentAsset(learnCourseCover(course, courseKey), courseFallback(courseKey)),
  ]))
}

function applyContent(config, options = {}) {
  classroomEnabled.value = normalizeMiniappLearn(config).classroom.enabled
  if (!classroomEnabled.value && ['course', 'material'].includes(activeCategory.value)) {
    activeCategory.value = 'quote'
  }
  const next = applyLearningContent({
    teachers: teachers.value,
    coursewareItems: coursewareItems.value,
    quotes: quotes.value,
  }, config, options)
  teachers.value = next.teachers
  quotes.value = next.quotes
  syncContentImages()
}

function onTeacherImageError() {
  if (!teacherImageFallbackUsed.consume('portrait')) return
  teacherImage.value = TEACHER_FALLBACK
}

function onCourseImageError(courseKey) {
  if (!courseImageFallbackUsed.consume(courseKey)) return
  courseImages.value[courseKey] = courseFallback(courseKey)
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

function openTypeDetail(id) {
  const typeId = Number(id)
  if (!Number.isInteger(typeId) || !TYPES_INFO[typeId]) return
  uni.navigateTo({ url: `/pages/enneagram-detail/enneagram-detail?type=${typeId}` })
}

function openPublishedCourse(course) {
  const url = classroomContentRoute({
    id: course?.classroomId || course?.id,
    contentType: course?.materialTypes?.includes('音频') ? 'audio' : 'video',
  })
  if (url) uni.navigateTo({ url })
}

function openPublishedMaterial(material) {
  const url = classroomContentRoute({
    id: material?.contentId,
    contentType: material?.contentType,
  })
  if (url) uni.navigateTo({ url })
}

function selectCategory(category) {
  const next = resolveLearningCategory(activeCategory.value, category)
  activeCategory.value = !classroomEnabled.value && ['course', 'material'].includes(next)
    ? 'quote'
    : next
}

function onTabKeydown(event, category) {
  const transition = learningTabTransition(category, event?.key)
  if (!transition.handled) return
  event.preventDefault?.()
  event.stopPropagation?.()
  selectCategory(transition.category)
  event?.currentTarget?.parentElement?.children?.[transition.focusIndex]?.focus?.()
}

function consumeNavigationIntent() {
  selectCategory(readLearningNavIntent())
}

async function loadContent(options = {}) {
  const silent = !!options.silent
  const ticket = requestGuard.issue()
  if (!silent) loading.value = true
  loadError.value = ''
  try {
    const [config, classroom] = await Promise.all([
      refreshSiteConfig(),
      listClassroomRecentApi({ limit: 20, offset: 0 }),
    ])
    if (!requestGuard.isLatest(ticket)) return
    applyContent(config, { preserveMissing: true })
    coursewareItems.value = mapPublishedClassroomItems(classroom?.items)
    syncContentImages()
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
        <text class="learn-header__intro">{{ classroomEnabled ? '课程、课件与老师语录，按分类随时查阅。' : '老师资料与老师语录，按分类随时查阅。' }}</text>
      </header>

      <view v-if="loading" class="learn-sync" role="status">
        <text>{{ classroomEnabled ? '正在更新老师与课程资料，先读本地内容…' : '正在更新老师资料与语录…' }}</text>
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
                v-if="teacherImage"
                class="learn-teacher__image"
                :src="teacherImage"
                mode="aspectFill"
                role="img"
                :aria-label="teacherImageLabel"
                @error="onTeacherImageError"
              />
              <view v-else class="learn-teacher__image learn-teacher__image--placeholder" aria-hidden="true">韩</view>
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
            <view v-if="teacherTagEntries.length" class="learn-teacher__tags" aria-label="老师擅长领域">
              <text v-for="tagEntry in teacherTagEntries" :key="tagEntry.key" class="learn-teacher__tag">{{ tagEntry.text }}</text>
            </view>
          </view>
        </view>

        <view v-else class="editorial-empty learn-teacher__empty">
          <text id="learn-teacher-heading" class="editorial-empty__title">老师资料整理中</text>
          <text>课程团队正在整理主讲老师的介绍与研究方向。</text>
        </view>
      </section>

      <view class="learn-tabs" role="tablist" aria-label="学习内容分类">
        <view v-if="classroomEnabled" id="learn-tab-course" class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'course' }" role="tab" data-category="course" aria-controls="learn-panel-course" :aria-selected="activeCategory === 'course'" :tabindex="activeCategory === 'course' ? 0 : -1" hover-class="learn-tab--pressed" @click="selectCategory('course')" @keydown="onTabKeydown($event, 'course')">课程</view>
        <view v-if="classroomEnabled" id="learn-tab-material" class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'material' }" role="tab" data-category="material" aria-controls="learn-panel-material" :aria-selected="activeCategory === 'material'" :tabindex="activeCategory === 'material' ? 0 : -1" hover-class="learn-tab--pressed" @click="selectCategory('material')" @keydown="onTabKeydown($event, 'material')">课件</view>
        <view id="learn-tab-quote" class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'quote' }" role="tab" data-category="quote" aria-controls="learn-panel-quote" :aria-selected="activeCategory === 'quote'" :tabindex="activeCategory === 'quote' ? 0 : -1" hover-class="learn-tab--pressed" @click="selectCategory('quote')" @keydown="onTabKeydown($event, 'quote')">语录</view>
      </view>

      <section v-if="classroomEnabled && activeCategory === 'course'" id="learn-panel-course" role="tabpanel" aria-labelledby="learn-tab-course" class="learning-panel">
        <text id="course-panel-heading" class="panel-title">全部课程</text>
        <view v-if="courseEntries.length" class="course-list">
          <article
            v-for="courseEntry in courseEntries"
            :key="courseEntry.key"
            class="course-row"
            role="button"
            tabindex="0"
            hover-class="control--pressed"
            @tap="openPublishedCourse(courseEntry.item)"
            @keydown="onActionKeydown($event, () => openPublishedCourse(courseEntry.item))"
          >
            <image class="course-row__cover" :src="courseImages[courseEntry.key]" mode="aspectFill" aria-hidden="true" lazy-load @error="onCourseImageError(courseEntry.key)" />
            <view class="course-row__copy">
              <text class="course-row__title">{{ courseEntry.item.title }}</text>
              <text v-if="courseEntry.item.materialTypes.length" class="course-row__materials">{{ courseEntry.item.materialTypes.join(' · ') }}</text>
              <text v-if="courseEntry.item.duration" class="course-row__duration">{{ courseEntry.item.duration }}</text>
              <text class="course-row__desc">{{ courseEntry.item.description }}</text>
            </view>
          </article>
        </view>
        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">课程正在准备中</text>
          <text>新课程上线后会在这里展示。</text>
        </view>
      </section>

      <section v-else-if="classroomEnabled && activeCategory === 'material'" id="learn-panel-material" role="tabpanel" aria-labelledby="learn-tab-material" class="learning-panel">
        <text id="material-panel-heading" class="panel-title">全部课件</text>
        <text class="panel-description">视频、音频与配套资料会集中在这里，点击即可进入学习。</text>
        <view v-if="materialItems.length" class="material-list">
          <article
            v-for="material in materialItems"
            :key="material.key"
            class="material-row"
            role="button"
            tabindex="0"
            hover-class="control--pressed"
            @tap="openPublishedMaterial(material)"
            @keydown="onActionKeydown($event, () => openPublishedMaterial(material))"
          >
            <view class="material-row__icon" aria-hidden="true">{{ material.contentType === 'audio' ? '听' : '播' }}</view>
            <view class="material-row__copy">
              <text class="material-row__title">{{ material.courseTitle }}</text>
              <text v-if="material.description" class="material-row__desc">{{ material.description }}</text>
              <view class="material-row__meta">
                <text class="material-row__type">{{ material.type }}</text>
                <text v-if="material.duration" class="material-row__duration">{{ material.duration }}</text>
              </view>
            </view>
            <text class="material-row__arrow" aria-hidden="true">›</text>
          </article>
        </view>
        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">课件资料整理中</text>
          <text>讲义、音频与练习资料上线后会在这里展示。</text>
        </view>
      </section>

      <section v-else-if="activeCategory === 'quote'" id="learn-panel-quote" role="tabpanel" aria-labelledby="learn-tab-quote" class="learning-panel">
        <text id="quote-panel-heading" class="panel-title">老师语录</text>
        <view v-if="quoteEntries.length" class="quote-list">
          <article v-for="quoteEntry in quoteEntries" :key="quoteEntry.key" class="quote-card">
            <text class="quote-card__mark">”</text>
            <text class="quote-card__text">{{ quoteEntry.text }}</text>
          </article>
        </view>
        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">老师语录整理中</text>
          <text>课堂札记整理完成后会在这里展示。</text>
        </view>
      </section>

      <section class="type-index" aria-labelledby="type-index-heading">
        <view class="type-index__head">
          <text id="type-index-heading" class="type-index__heading">九型速查</text>
          <text class="type-index__hint">点击查看详情</text>
        </view>
        <view class="type-index__list">
          <view
            v-for="type in types"
            :key="`type::${type.id}`"
            class="type-index__item"
            role="button"
            tabindex="0"
            hover-class="type-index__item--pressed"
            :aria-label="`查看${type.id}号${type.name}详情`"
            @click="openTypeDetail(type.id)"
            @keydown="onActionKeydown($event, () => openTypeDetail(type.id))"
          >
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
  min-width: 0;
  overflow-x: hidden;
  background: #F6F1E7;
  color: #20252B;
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
  background: #FFFDF8;
  color: rgba(32, 37, 43, .72);
  font-size: 22rpx;
  line-height: 1.5;
}

.learn-error { justify-content: space-between; color: #A43C2C; border-color: rgba(164, 60, 44, .24); }
.learn-retry { flex: 0 0 auto; min-height: 88rpx; padding: 0 20rpx; display: inline-flex; align-items: center; color: #A43C2C; font-size: 24rpx; font-weight: 700; touch-action: manipulation; }

.learn-teacher { min-width: 0; padding: 20rpx; border: 2rpx solid rgba(51, 91, 74, .14); border-radius: 20rpx; background: #FFFDF8; }
.learn-teacher__content { min-width: 0; }
.learn-teacher__summary { min-width: 0; display: flex; align-items: center; gap: 18rpx; }
.learn-teacher__portrait { flex: 0 0 108rpx; width: 108rpx; aspect-ratio: 4 / 5; overflow: hidden; border-radius: 16rpx; background: #F6F1E7; }
.learn-teacher__image { display: block; width: 100%; height: 100%; }
.learn-teacher__image--placeholder { display: flex; align-items: center; justify-content: center; color: #335B4A; font-size: 42rpx; font-weight: 800; }
.learn-teacher__identity { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 5rpx; }
.section-label { color: #335B4A; font-size: 22rpx; font-weight: 700; line-height: 1.4; }
.learn-teacher__name { font-size: 30rpx; font-weight: 800; line-height: 1.35; }
.learn-teacher__title { color: rgba(32, 37, 43, .65); font-size: 23rpx; line-height: 1.45; }
.learn-teacher__toggle { flex: 0 0 auto; min-height: 88rpx; padding: 0 8rpx; display: inline-flex; align-items: center; color: #335B4A; font-size: 23rpx; font-weight: 700; touch-action: manipulation; }
.learn-teacher__details { margin-top: 18rpx; padding-top: 18rpx; display: flex; flex-direction: column; gap: 14rpx; border-top: 2rpx solid rgba(32, 37, 43, .1); }
.learn-teacher__bio { color: rgba(32, 37, 43, .76); font-size: 26rpx; line-height: 1.65; }
.learn-teacher__tags { display: flex; flex-wrap: wrap; gap: 10rpx; }
.learn-teacher__tag { padding: 6rpx 12rpx; border-radius: 16rpx; background: rgba(51, 91, 74, .08); color: #335B4A; font-size: 22rpx; line-height: 1.4; }

.learn-tabs { margin-top: 20rpx; padding: 6rpx; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 4rpx; border: 2rpx solid rgba(32, 37, 43, .1); border-radius: 18rpx; background: #FFFDF8; }
.learn-tab { min-width: 0; min-height: 88rpx; display: flex; align-items: center; justify-content: center; border-radius: 14rpx; color: rgba(32, 37, 43, .62); font-size: 26rpx; font-weight: 700; touch-action: manipulation; }
.learn-tab--active { background: #335B4A; color: #FFFFFF; }
.learn-tab--pressed { opacity: .82; }

.learning-panel { margin-top: 16rpx; padding: 0 20rpx; border: 2rpx solid rgba(32, 37, 43, .1); border-radius: 20rpx; background: #FFFDF8; }
.panel-title { min-height: 82rpx; display: flex; align-items: center; border-bottom: 2rpx solid rgba(32, 37, 43, .1); font-size: 28rpx; font-weight: 800; }
.course-list, .material-list, .quote-list { min-width: 0; display: flex; flex-direction: column; }

.course-row { min-width: 0; padding: 22rpx 0; display: grid; grid-template-columns: 132rpx minmax(0, 1fr); gap: 18rpx; border-bottom: 2rpx solid rgba(32, 37, 43, .1); }
.course-row:last-child, .material-row:last-child, .quote-card:last-child { border-bottom: 0; }
.course-row__cover { display: block; width: 132rpx; height: 104rpx; border-radius: 14rpx; background: #F6F1E7; }
.course-row__copy { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 7rpx; }
.course-row__title { font-size: 28rpx; font-weight: 800; line-height: 1.4; }
.course-row__materials { color: #335B4A; font-size: 22rpx; line-height: 1.4; }
.course-row__duration { color: rgba(32, 37, 43, .58); font-size: 23rpx; line-height: 1.4; }
.course-row__desc { color: rgba(32, 37, 43, .7); font-size: 26rpx; line-height: 1.55; }

.panel-description { display: block; margin-top: 8rpx; color: rgba(32, 37, 43, .62); font-size: 23rpx; line-height: 1.55; }
.material-row { min-width: 0; padding: 22rpx 0; display: flex; align-items: center; gap: 18rpx; border-bottom: 2rpx solid rgba(32, 37, 43, .1); }
.material-row__icon { flex: 0 0 72rpx; width: 72rpx; height: 72rpx; display: flex; align-items: center; justify-content: center; border-radius: 16rpx; background: rgba(51, 91, 74, .1); color: #335B4A; font-size: 25rpx; font-weight: 800; }
.material-row__copy { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 7rpx; }
.material-row__title { font-size: 28rpx; font-weight: 800; line-height: 1.4; }
.material-row__desc { color: rgba(32, 37, 43, .68); font-size: 25rpx; line-height: 1.5; }
.material-row__meta { display: flex; flex-wrap: wrap; gap: 12rpx; }
.material-row__type { color: #335B4A; font-size: 23rpx; font-weight: 700; }
.material-row__duration { color: rgba(32, 37, 43, .58); font-size: 23rpx; }
.material-row__arrow { flex: 0 0 auto; color: var(--nx-brand-700); font-size: 42rpx; font-weight: 500; line-height: 1; }

.quote-card { position: relative; min-width: 0; padding: 24rpx 40rpx 24rpx 6rpx; display: flex; border-bottom: 2rpx solid rgba(32, 37, 43, .1); }
.quote-card__mark { position: absolute; top: 16rpx; right: 2rpx; color: rgba(51, 91, 74, .28); font-family: Georgia, serif; font-size: 40rpx; line-height: 1; }
.quote-card__text { color: rgba(32, 37, 43, .82); font-size: 27rpx; line-height: 1.7; }

.editorial-empty { min-height: 180rpx; padding: 28rpx 0; display: flex; flex-direction: column; justify-content: center; gap: 8rpx; color: rgba(32, 37, 43, .66); font-size: 24rpx; line-height: 1.55; }
.editorial-empty__title { color: #20252B; font-size: 28rpx; font-weight: 800; }

.type-index { margin-top: 28rpx; padding: 0 4rpx; }
.type-index__heading { color: rgba(32, 37, 43, .58); font-size: 23rpx; font-weight: 700; }
.type-index__list { padding-top: 12rpx; display: flex; flex-wrap: wrap; gap: 8rpx; }
.type-index__item { padding: 6rpx 10rpx; display: flex; align-items: center; gap: 5rpx; border: 2rpx solid rgba(51, 91, 74, .15); border-radius: 14rpx; color: rgba(51, 91, 74, .76); font-size: 22rpx; line-height: 1.35; }
.type-index__number, .type-index__name { font-weight: 700; }
.control--pressed { opacity: .82; transform: translateY(2rpx); }

.learn-retry:focus-visible,
.learn-teacher__toggle:focus-visible,
.learn-tab:focus-visible { outline: 4rpx solid #A43C2C; outline-offset: 2rpx; }

@media screen and (min-width: 768px) {
  .learn-content { padding-left: 48rpx; padding-right: 48rpx; }
  .learn-teacher { padding-left: 28rpx; padding-right: 28rpx; }
  .learning-panel { padding-left: 28rpx; padding-right: 28rpx; }
}

/* Shared editorial treatment used by the refreshed home and profile surfaces. */
.learn {
  background: var(--nx-page-bg);
  color: var(--nx-text);
}

.learn-content {
  max-width: 960rpx;
  padding: 28rpx 24rpx 64rpx;
}

.learn-header {
  position: relative;
  overflow: hidden;
  min-height: 220rpx;
  justify-content: flex-end;
  padding: 30rpx 28rpx;
  border: 2rpx solid rgba(223, 188, 127, .34);
  border-radius: 28rpx;
  background: linear-gradient(145deg, var(--nx-brand-900), var(--nx-brand-700));
  box-shadow: 0 24rpx 52rpx -34rpx rgba(32, 42, 55, .72);
}

.learn-header::after {
  content: '学';
  position: absolute;
  right: 26rpx;
  top: -38rpx;
  color: rgba(223, 188, 127, .18);
  font-size: 190rpx;
  font-weight: 900;
  line-height: 1;
}

.learn-header__title,
.learn-header__intro { position: relative; z-index: 1; }
.learn-header__title { color: var(--nx-surface); font-size: 42rpx; font-weight: 900; line-height: 1.25; }
.learn-header__intro { max-width: 620rpx; margin-top: 10rpx; color: rgba(255, 255, 255, .78); font-size: 24rpx; line-height: 1.6; }

.learn-sync,
.learn-error {
  min-height: 72rpx;
  margin-top: 16rpx;
  margin-bottom: 0;
  padding: 12rpx 18rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 18rpx;
  background: var(--nx-surface);
  color: var(--nx-text-muted);
}

.learn-error { color: var(--nx-danger); border-color: rgba(180, 35, 24, .24); }
.learn-retry { color: var(--nx-brand-700); font-weight: 900; }

.learn-teacher {
  margin-top: 18rpx;
  padding: 24rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 24rpx;
  background: var(--nx-surface);
  box-shadow: 0 14rpx 34rpx -28rpx rgba(32, 42, 55, .42);
}

.learn-teacher__portrait { flex-basis: 124rpx; width: 124rpx; border-radius: 20rpx; background: var(--nx-surface-soft); }
.learn-teacher__summary { gap: 20rpx; }
.section-label { color: var(--nx-brand-700); font-weight: 900; letter-spacing: 1rpx; }
.learn-teacher__name { color: var(--nx-brand-900); font-size: 32rpx; }
.learn-teacher__title { color: var(--nx-text-muted); }
.learn-teacher__toggle { color: var(--nx-brand-700); font-weight: 900; }
.learn-teacher__details { border-top-color: var(--nx-border); }
.learn-teacher__bio { color: var(--nx-text); }
.learn-teacher__tag { border: 2rpx solid rgba(223, 188, 127, .38); background: #F5EDDF; color: var(--nx-brand-700); }

.learn-tabs {
  margin-top: 20rpx;
  padding: 6rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 20rpx;
  background: var(--nx-surface-soft);
}

.learn-tab { min-height: 92rpx; border-radius: 16rpx; color: var(--nx-text-muted); font-weight: 900; }
.learn-tab--active { color: var(--nx-surface); background: var(--nx-brand-900); box-shadow: inset 0 -4rpx 0 var(--nx-accent-gold); }

.learning-panel {
  margin-top: 16rpx;
  padding: 0 24rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 24rpx;
  background: var(--nx-surface);
  box-shadow: 0 14rpx 34rpx -30rpx rgba(32, 42, 55, .38);
}

.panel-title { min-height: 92rpx; color: var(--nx-brand-900); border-bottom-color: var(--nx-border); font-size: 30rpx; }
.course-row,
.material-row,
.quote-card { border-bottom-color: var(--nx-border); }
.course-row { padding: 24rpx 0; grid-template-columns: 156rpx minmax(0, 1fr); gap: 20rpx; }
.course-row__cover { width: 156rpx; height: 116rpx; border-radius: 18rpx; background: var(--nx-surface-soft); }
.course-row__title,
.material-row__title { color: var(--nx-brand-900); font-size: 29rpx; }
.course-row__materials,
.material-row__type { color: var(--nx-brand-700); font-weight: 900; }
.course-row__duration,
.material-row__duration { color: var(--nx-text-muted); }
.course-row__desc,
.material-row__desc { color: var(--nx-text-muted); }

.material-row__icon { border: 2rpx solid rgba(223, 188, 127, .42); background: #F5EDDF; color: var(--nx-brand-700); }
.quote-card__mark { color: rgba(223, 188, 127, .72); }
.quote-card__text { color: var(--nx-text); }

.editorial-empty {
  min-height: 210rpx;
  margin: 18rpx 0 22rpx;
  padding: 30rpx 20rpx;
  align-items: center;
  border: 2rpx dashed var(--nx-border);
  border-radius: 18rpx;
  background: var(--nx-surface-soft);
  color: var(--nx-text-muted);
  text-align: center;
}

.editorial-empty__title { color: var(--nx-brand-900); font-size: 30rpx; font-weight: 900; }
.type-index { margin-top: 22rpx; padding: 22rpx 24rpx 24rpx; border: 2rpx solid var(--nx-border); border-radius: 24rpx; background: var(--nx-surface); }
.type-index__head { display: flex; align-items: center; justify-content: space-between; gap: 16rpx; }
.type-index__heading { color: var(--nx-brand-700); font-size: 25rpx; font-weight: 900; letter-spacing: 1rpx; }
.type-index__hint { color: var(--nx-text-muted); font-size: 22rpx; }
.type-index__list { gap: 10rpx; }
.type-index__item { padding: 8rpx 12rpx; border-color: var(--nx-border); border-radius: 14rpx; background: var(--nx-surface-soft); color: var(--nx-brand-700); touch-action: manipulation; }
.type-index__item--pressed { opacity: .76; transform: translateY(2rpx); border-color: var(--nx-accent-gold); }
.type-index__item:focus-visible { outline: 4rpx solid var(--nx-accent-gold); outline-offset: 2rpx; }

.learn-retry:focus-visible,
.learn-teacher__toggle:focus-visible,
.learn-tab:focus-visible { outline-color: var(--nx-accent-gold); }
</style>
