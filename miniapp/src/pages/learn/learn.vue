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
import { mapPublishedClassroomItems } from '../../utils/classroomCourseware'
import { classroomContentRoute } from '../../utils/classroomDisplay'
import { normalizeMiniappLearn } from '../../utils/miniappPages'
import { userErrorMessage } from '../../utils/userMessage'
import { previewImage } from '../../utils/imagePreview'
import NxIcon from '../../components/NxIcon.vue'
import { UI_PREVIEW } from '../../utils/uiPreview'

const TEACHER_FALLBACK = '/static/teacher/portrait.jpg'
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
const activeTopic = ref('全部')
const searchQuery = ref('')
const publishedItems = ref([])
const topics = ['全部', '九型观察', '关系沟通', '成长日常']
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
const filteredCourses = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return courseEntries.value.filter(({ item }) => {
    const raw = publishedItems.value.find((entry) => String(entry.id) === String(item.id)) || {}
    const haystack = `${item.title} ${item.description} ${raw.category || ''}`.toLowerCase()
    const matchesTopic = activeTopic.value === '全部' || (raw.category
      ? raw.category === activeTopic.value
      : (activeTopic.value === '九型观察' && /九型|人格|动机|型号/.test(haystack))
        || (activeTopic.value === '关系沟通' && /关系|沟通|倾听|伴侣|亲子/.test(haystack))
        || (activeTopic.value === '成长日常' && /成长|日常|练习|生活|情绪/.test(haystack)))
    return matchesTopic && (!query || haystack.includes(query))
  })
})
const featuredCourse = computed(() => filteredCourses.value[0] || null)
const remainingCourses = computed(() => filteredCourses.value.slice(1))
const materialItems = computed(() => flattenLearningMaterials(coursewareItems.value))
const quoteEntries = computed(() => createLearningQuoteEntries(quotes.value))
const teacherTagEntries = computed(() => createLearningTagEntries(
  teacher.value?.tags,
  `${teacher.value?.name || ''}::${teacher.value?.title || ''}`,
))

function topicFor(course) {
  const raw = publishedItems.value.find((item) => String(item.id) === String(course.id))
  return raw?.category || (course.materialTypes?.includes('音频') ? '音频课件' : '日常分享')
}

function clearSearch() {
  searchQuery.value = ''
  activeTopic.value = '全部'
}

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
  const portraitSource = /teacher-poster/i.test(String(portrait || '')) ? '/static/teacher/portrait.jpg' : portrait
  teacherImage.value = resolveContentAsset(portraitSource, TEACHER_FALLBACK)
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

function previewTeacherAvatar() {
  previewImage(teacherImage.value)
}

function previewCourseCover(courseKey) {
  const current = courseImages.value?.[courseKey]
  if (!current) return
  const urls = Object.values(courseImages.value || {}).filter(Boolean)
  previewImage(current, { urls })
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
    publishedItems.value = Array.isArray(classroom?.items) ? classroom.items : []
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
      <view class="learn-header">
        <view class="eyebrow-row"><text class="eyebrow">DAILY WITH HAN</text><text v-if="UI_PREVIEW" class="preview-badge">演示体验</text></view>
        <text class="learn-header__title">跟着老韩，慢慢成长</text>
        <text class="learn-header__intro">把课堂里的看见，带回每一天的生活。</text>
      </view>

      <view class="learn-tabs" role="tablist" aria-label="学习内容分类">
        <button v-if="classroomEnabled" id="learn-tab-course" class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'course' }" role="tab" aria-controls="learn-panel-course" :aria-selected="activeCategory === 'course'" :tabindex="activeCategory === 'course' ? 0 : -1" @click="selectCategory('course')" @keydown="onTabKeydown($event, 'course')">老师日常</button>
        <button v-if="classroomEnabled" id="learn-tab-material" class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'material' }" role="tab" aria-controls="learn-panel-material" :aria-selected="activeCategory === 'material'" :tabindex="activeCategory === 'material' ? 0 : -1" @click="selectCategory('material')" @keydown="onTabKeydown($event, 'material')">学习资料</button>
        <button id="learn-tab-quote" class="learn-tab" :class="{ 'learn-tab--active': activeCategory === 'quote' }" role="tab" aria-controls="learn-panel-quote" :aria-selected="activeCategory === 'quote'" :tabindex="activeCategory === 'quote' ? 0 : -1" @click="selectCategory('quote')" @keydown="onTabKeydown($event, 'quote')">课堂札记</button>
      </view>

      <view v-if="loading" class="learn-sync" role="status">正在更新最新内容…</view>
      <view v-if="loadError" class="learn-error" role="status"><text>{{ loadError }}</text><button class="learn-retry" @click="loadContent">重新加载</button></view>

      <view v-if="classroomEnabled && activeCategory === 'course'" id="learn-panel-course" role="tabpanel" aria-labelledby="learn-tab-course" class="daily-panel">
        <view class="search-field">
          <NxIcon name="search" :size="18" color="#77786F" />
          <input v-model="searchQuery" class="search-field__input" aria-label="搜索视频标题或内容" placeholder="寻找此刻想听的内容" placeholder-class="search-placeholder" confirm-type="search" />
          <button v-if="searchQuery" class="search-clear" aria-label="清空搜索内容" @click="searchQuery = ''">清空</button>
        </view>
        <view class="topic-filters" aria-label="视频主题">
          <button v-for="topic in topics" :key="topic" class="topic-filter" :class="{ 'topic-filter--active': activeTopic === topic }" :aria-pressed="activeTopic === topic" @click="activeTopic = topic">{{ topic }}</button>
        </view>

        <button v-if="featuredCourse" class="featured-video" @click="openPublishedCourse(featuredCourse.item)">
          <view class="featured-video__media">
            <image class="featured-video__cover" :src="courseImages[featuredCourse.key]" mode="aspectFill" :aria-label="featuredCourse.item.title" @click.stop="previewCourseCover(featuredCourse.key)" @error="onCourseImageError(featuredCourse.key)" />
            <view class="featured-video__shade" />
            <text class="featured-video__tag">{{ topicFor(featuredCourse.item) }}</text>
            <view class="featured-video__play"><NxIcon name="play" :size="23" color="#FFFFFF" /></view>
            <text v-if="featuredCourse.item.duration" class="featured-video__duration">{{ featuredCourse.item.duration }}</text>
          </view>
          <view class="featured-video__body">
            <view class="video-eyebrow"><text>这一刻，听老韩说</text><NxIcon name="arrow" :size="18" color="#A55C3B" /></view>
            <text class="featured-video__title">{{ featuredCourse.item.title }}</text>
            <text v-if="featuredCourse.item.description" class="featured-video__desc">{{ featuredCourse.item.description }}</text>
          </view>
        </button>

        <view v-if="remainingCourses.length" class="section-heading"><text class="section-title">更多日常</text><text class="section-note">{{ filteredCourses.length }} 则分享</text></view>
        <view v-if="remainingCourses.length" class="course-list">
          <button v-for="courseEntry in remainingCourses" :key="courseEntry.key" class="course-row" @click="openPublishedCourse(courseEntry.item)">
            <view class="course-row__media">
              <image class="course-row__cover" :src="courseImages[courseEntry.key]" mode="aspectFill" :aria-label="courseEntry.item.title" lazy-load @click.stop="previewCourseCover(courseEntry.key)" @error="onCourseImageError(courseEntry.key)" />
              <view class="course-row__play"><NxIcon name="play" :size="12" color="#FFFFFF" /></view>
              <text v-if="courseEntry.item.duration" class="course-row__duration">{{ courseEntry.item.duration }}</text>
            </view>
            <view class="course-row__copy">
              <text class="course-row__category">{{ topicFor(courseEntry.item) }}</text>
              <text class="course-row__title">{{ courseEntry.item.title }}</text>
              <text v-if="courseEntry.item.description" class="course-row__desc">{{ courseEntry.item.description }}</text>
              <text class="course-row__materials">{{ courseEntry.item.materialTypes.join(' · ') }}<text v-if="teacher"> · {{ teacher.name }}</text></text>
            </view>
          </button>
        </view>
        <view v-if="!loading && !filteredCourses.length" class="editorial-empty">
          <NxIcon name="video" :size="30" color="#A55C3B" />
          <text class="editorial-empty__title">{{ searchQuery || activeTopic !== '全部' ? '没有找到相关内容' : '课程正在准备中' }}</text>
          <text>{{ searchQuery || activeTopic !== '全部' ? '试试其他关键词，或看看全部分享。' : '新视频上线后，会在这里与你见面。' }}</text>
          <button v-if="searchQuery || activeTopic !== '全部'" class="empty-action" @click="clearSearch">查看全部分享</button>
        </view>
      </view>

      <section v-else-if="classroomEnabled && activeCategory === 'material'" id="learn-panel-material" role="tabpanel" aria-labelledby="learn-tab-material" class="learning-panel">
        <view class="section-heading"><text class="section-title">让学习，有迹可循</text><NxIcon name="book" :size="21" color="#A55C3B" /></view>
        <text class="panel-description">重听一段课堂，重新做一次练习。每次回看，都可能有新的发现。</text>
        <view v-if="materialItems.length" class="material-list">
          <button v-for="material in materialItems" :key="material.key" class="material-row" @click="openPublishedMaterial(material)">
            <view class="material-row__icon"><NxIcon :name="material.contentType === 'audio' ? 'message' : 'video'" :size="23" color="#A55C3B" /></view>
            <view class="material-row__copy"><text class="material-row__title">{{ material.courseTitle }}</text><text class="material-row__type">{{ material.type }}<text v-if="material.duration"> · {{ material.duration }}</text></text></view>
            <NxIcon name="chevron" :size="16" color="#77786F" />
          </button>
        </view>
        <view v-else class="editorial-empty"><text class="editorial-empty__title">课件资料整理中</text><text>讲义、音频与练习资料上线后会在这里展示。</text></view>
      </section>

      <section v-else-if="activeCategory === 'quote'" id="learn-panel-quote" role="tabpanel" aria-labelledby="learn-tab-quote" class="learning-panel">
        <view class="section-heading"><text class="section-title">留一句话，给今天的自己</text></view>
        <text class="panel-description">从课堂到生活，记下那些让我们停一停、想一想的时刻。</text>
        <view v-if="quoteEntries.length" class="quote-list">
          <article v-for="quoteEntry in quoteEntries" :key="quoteEntry.key" class="quote-card">
            <text class="quote-card__mark" aria-hidden="true">”</text>
            <text class="quote-card__text">{{ quoteEntry.text }}</text>
            <text v-if="teacher" class="quote-card__byline">— {{ teacher.name }} · 课堂札记</text>
          </article>
        </view>
        <view v-else class="editorial-empty"><text class="editorial-empty__title">老师语录整理中</text><text>课堂札记整理完成后会在这里展示。</text></view>
      </section>

      <section class="learn-teacher" aria-labelledby="learn-teacher-heading">
        <template v-if="teacher">
          <view class="learn-teacher__summary">
            <view class="learn-teacher__portrait">
              <button v-if="teacherImage" class="teacher-card__avatar-action" :aria-label="`预览${teacherImageLabel}`" @click="previewTeacherAvatar"><image class="learn-teacher__image" :src="teacherImage" mode="aspectFill" role="img" :aria-label="teacherImageLabel" @error="onTeacherImageError" /></button>
              <view v-else class="learn-teacher__image learn-teacher__image--placeholder">韩</view>
            </view>
            <view class="learn-teacher__identity"><text class="section-label">主讲老师</text><text id="learn-teacher-heading" class="learn-teacher__name">{{ teacher.name }}</text><text class="learn-teacher__title">{{ teacher.title }}</text></view>
            <button class="learn-teacher__toggle" :aria-expanded="teacherExpanded" aria-controls="learn-teacher-details" @click="toggleTeacher">{{ teacherExpanded ? '收起' : '了解老师' }}<NxIcon name="chevron" :size="14" color="#A55C3B" /></button>
          </view>
          <view v-if="teacherExpanded" id="learn-teacher-details" class="learn-teacher__details"><text class="learn-teacher__bio">{{ teacher.bio }}</text><view v-if="teacherTagEntries.length" class="learn-teacher__tags"><text v-for="tagEntry in teacherTagEntries" :key="tagEntry.key" class="learn-teacher__tag">{{ tagEntry.text }}</text></view></view>
        </template>
        <view v-else class="editorial-empty"><text id="learn-teacher-heading" class="editorial-empty__title">老师资料整理中</text><text>课程团队正在整理主讲老师的介绍。</text></view>
      </section>

      <section class="type-index" aria-labelledby="type-index-heading">
        <view class="section-heading"><text id="type-index-heading" class="section-title">认识九种生命力</text><text class="section-note">九型速查</text></view>
        <view class="type-index__list"><button v-for="type in types" :key="`type::${type.id}`" class="type-index__item" :aria-label="`查看${type.id}号${type.name}详情`" @click="openTypeDetail(type.id)"><text class="type-index__number">{{ type.id }}</text><text class="type-index__name">{{ type.name }}</text></button></view>
      </section>
      <text class="page-footnote">看见自己，也看见生活。</text>
    </view>
  </view>
</template>

<style scoped>
.learn { min-width: 0; overflow-x: hidden; padding: 0; padding-bottom: calc(24rpx + env(safe-area-inset-bottom) + var(--window-bottom, 0px)); background: var(--nx-page-bg, #F7F5F0); color: var(--nx-text, #282A27); }
.learn-content { width: 100%; max-width: 980rpx; margin: 0 auto; padding: 36rpx 36rpx 52rpx; box-sizing: border-box; }
button { box-sizing: border-box; margin: 0; padding: 0; border-radius: 0; background: transparent; color: inherit; font-size: inherit; line-height: 1.5; text-align: left; }
button::after { border: 0; }
button:active { opacity: .75; }
button:focus-visible { outline: 3rpx solid #A55C3B; outline-offset: 5rpx; }
.learn-header { padding: 8rpx 0 26rpx; }
.eyebrow-row { display: flex; align-items: center; justify-content: space-between; gap: 16rpx; }
.eyebrow { color: #A55C3B; font-size: 21rpx; font-weight: 600; letter-spacing: 4rpx; }
.preview-badge { padding: 5rpx 12rpx; border: 1rpx solid #D9C6B9; border-radius: 8rpx; color: #8D634D; font-size: 20rpx; }
.learn-header__title { display: block; margin-top: 24rpx; font-family: 'Songti SC', 'STSong', serif; font-size: 48rpx; font-weight: 600; letter-spacing: 1rpx; line-height: 1.45; }
.learn-header__intro { display: block; margin-top: 14rpx; color: #77786F; font-size: 26rpx; line-height: 1.7; }
.learn-tabs { display: flex; gap: 40rpx; border-bottom: 1rpx solid #E6E1D8; margin-bottom: 30rpx; }
.learn-tab { position: relative; min-height: 88rpx; color: #77786F; font-size: 28rpx; white-space: nowrap; }
.learn-tab--active { color: #282A27; font-weight: 600; }
.learn-tab--active::before { content: ''; position: absolute; bottom: 0; left: 50%; width: 34rpx; height: 5rpx; border-radius: 5rpx; transform: translateX(-50%); background: #A55C3B; }
.learn-tab:focus-visible { outline: 3rpx solid #A55C3B; }
.learn-sync, .learn-error { display: flex; gap: 16rpx; align-items: center; justify-content: space-between; margin-bottom: 24rpx; padding: 18rpx; border-radius: 16rpx; background: #F0EBE3; color: #77786F; font-size: 24rpx; line-height: 1.6; }
.learn-error { color: #964A32; }
.learn-retry { min-height: 88rpx; flex: none; color: #A55C3B; padding: 0 12rpx; }
.search-field { display: flex; gap: 18rpx; align-items: center; height: 88rpx; padding: 0 24rpx; box-sizing: border-box; background: #EFEBE4; border-radius: 16rpx; }
.search-field__input { flex: 1; min-width: 0; font-size: 26rpx; color: #282A27; height: 88rpx; }
.search-placeholder { color: #77786F; }
.search-clear { display: flex; align-items: center; min-height: 88rpx; padding-left: 8rpx; color: #A55C3B; font-size: 24rpx; }
.topic-filters { display: flex; gap: 10rpx; margin: 18rpx 0 28rpx; }
.topic-filter { flex: 1; min-width: 0; min-height: 88rpx; display: flex; align-items: center; justify-content: center; font-size: 25rpx; color: #77786F; white-space: nowrap; }
.topic-filter--active { color: #A55C3B; font-weight: 600; }
.topic-filter--active::after { display: block; top: 12rpx; bottom: 12rpx; width: 100%; height: auto; box-sizing: border-box; border: 1rpx solid #DAC4B5; border-radius: 999rpx; transform: none; }
.featured-video { display: block; width: 100%; overflow: hidden; border-radius: 24rpx; background: #FFFFFF; border: 1rpx solid #E6E1D8; }
.featured-video__media { position: relative; height: 382rpx; background: #DED9CF; }
.featured-video__cover { display: block; width: 100%; height: 100%; }
.featured-video__shade { position: absolute; inset: 0; background: linear-gradient(180deg, rgba(0,0,0,.12), transparent 45%, rgba(0,0,0,.32)); }
.featured-video__tag { position: absolute; left: 26rpx; top: 26rpx; padding: 7rpx 14rpx; background: rgba(247,245,240,.94); border-radius: 7rpx; color: #52483C; font-size: 21rpx; }
.featured-video__play { position: absolute; left: 50%; top: 50%; width: 84rpx; height: 84rpx; transform: translate(-50%, -50%); display: flex; align-items: center; justify-content: center; border: 2rpx solid rgba(255,255,255,.8); border-radius: 50%; background: rgba(40,42,39,.16); }
.featured-video__duration { position: absolute; right: 24rpx; bottom: 22rpx; color: #FFFFFF; font-size: 22rpx; }
.featured-video__body { padding: 28rpx 30rpx 30rpx; }
.video-eyebrow { display: flex; align-items: center; justify-content: space-between; color: #A55C3B; font-size: 22rpx; letter-spacing: 1rpx; }
.featured-video__title { display: block; margin-top: 14rpx; font-family: 'Songti SC', 'STSong', serif; font-size: 36rpx; font-weight: 600; line-height: 1.5; }
.featured-video__desc { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; margin-top: 14rpx; color: #77786F; font-size: 25rpx; line-height: 1.8; }
.section-heading { display: flex; align-items: center; justify-content: space-between; gap: 16rpx; margin: 40rpx 0 24rpx; }
.section-title { font-family: 'Songti SC', 'STSong', serif; font-size: 34rpx; font-weight: 600; line-height: 1.45; }
.section-note { color: #77786F; font-size: 22rpx; flex: none; }
.course-list { display: flex; flex-direction: column; }
.course-row { display: flex; align-items: center; gap: 24rpx; width: 100%; padding: 26rpx 0; border-bottom: 1rpx solid #E6E1D8; }
.course-row:first-child { padding-top: 0; }
.course-row__media { position: relative; width: 230rpx; height: 194rpx; flex: none; border-radius: 16rpx; overflow: hidden; background: #E6E1D8; }
.course-row__cover { display: block; width: 100%; height: 100%; }
.course-row__play { position: absolute; left: 12rpx; bottom: 12rpx; width: 38rpx; height: 38rpx; display: flex; align-items: center; justify-content: center; background: rgba(40,42,39,.55); border-radius: 50%; }
.course-row__duration { position: absolute; right: 10rpx; bottom: 12rpx; padding: 2rpx 6rpx; color: #FFFFFF; background: rgba(40,42,39,.65); border-radius: 5rpx; font-size: 22rpx; }
.course-row__copy { flex: 1; min-width: 0; }
.course-row__category { color: #A55C3B; font-size: 21rpx; }
.course-row__title { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; margin-top: 8rpx; font-size: 29rpx; font-weight: 600; line-height: 1.55; }
.course-row__desc { display: none; font-size: 26rpx; }
.course-row__materials { display: block; margin-top: 16rpx; color: #77786F; font-size: 22rpx; }
.learning-panel { background: var(--nx-surface, #FFFFFF); border: 1rpx solid #E6E1D8; padding: 0 26rpx 24rpx; border-radius: 24rpx; }
.learning-panel .section-heading { margin-top: 28rpx; }
.panel-description { display: block; color: #77786F; font-size: 26rpx; line-height: 1.8; }
.material-row { display: flex; gap: 20rpx; align-items: center; min-height: 132rpx; width: 100%; padding: 24rpx 0; border-bottom: 1rpx solid #E6E1D8; }
.material-row:last-child { border-bottom: 0; }
.material-row__icon { width: 82rpx; height: 88rpx; flex: none; display: flex; align-items: center; justify-content: center; background: #F3EDE4; border-radius: 14rpx; }
.material-row__copy { flex: 1; min-width: 0; }
.material-row__title { display: block; font-size: 28rpx; line-height: 1.6; }
.material-row__type { display: block; margin-top: 10rpx; color: #77786F; font-size: 23rpx; }
.quote-card { padding: 12rpx 12rpx 34rpx; border-bottom: 1rpx solid #E6E1D8; }
.quote-card:last-child { border-bottom: 0; }
.quote-card__mark { display: block; height: 72rpx; font-family: Georgia, serif; font-size: 104rpx; color: #C8AD94; line-height: 1.3; }
.quote-card__text { display: block; font-family: 'Songti SC', 'STSong', serif; font-size: 32rpx; line-height: 1.9; }
.quote-card__byline { display: block; margin-top: 24rpx; color: #77786F; font-size: 23rpx; }
.editorial-empty { display: flex; align-items: center; flex-direction: column; gap: 18rpx; padding: 44rpx 22rpx; color: #77786F; font-size: 25rpx; line-height: 1.8; text-align: center; }
.editorial-empty__title { display: block; color: #282A27; font-size: 30rpx; }
.empty-action { display: flex; align-items: center; justify-content: center; min-height: 88rpx; color: #A55C3B; }
.learn-teacher { margin-top: 40rpx; padding: 24rpx; border: 1rpx solid #E6E1D8; border-radius: 24rpx; background: var(--nx-surface, #FFFFFF); }
.learn-teacher__summary { display: flex; gap: 20rpx; align-items: center; }
.learn-teacher__portrait { flex: none; width: 88rpx; aspect-ratio: 4 / 5; border-radius: 14rpx; overflow: hidden; }
.teacher-card__avatar-action { display: block; width: 88rpx; height: 110rpx; padding: 0; margin: 0; border: 0; border-radius: 14rpx; background: transparent; overflow: hidden; }
.teacher-card__avatar-action::after { border: 0; }
.learn-teacher__image { display: block; width: 88rpx; height: 110rpx; border-radius: 14rpx; }
.learn-teacher__image--placeholder { display: flex; align-items: center; justify-content: center; color: #A55C3B; background: #F0E9DD; font-family: 'Songti SC', serif; font-size: 40rpx; }
.learn-teacher__identity { flex: 1; min-width: 0; }
.section-label { color: #77786F; font-size: 21rpx; }
.learn-teacher__name { display: block; margin-top: 5rpx; font-size: 28rpx; font-weight: 600; }
.learn-teacher__title { display: block; margin-top: 5rpx; color: #77786F; font-size: 21rpx; }
.learn-teacher__toggle { display: flex; align-items: center; min-height: 88rpx; flex: none; gap: 4rpx; color: #A55C3B; font-size: 22rpx; }
.learn-teacher__details { margin-top: 24rpx; padding-top: 24rpx; border-top: 1rpx solid #E6E1D8; }
.learn-teacher__bio { display: block; color: #77786F; font-size: 26rpx; line-height: 1.9; }
.learn-teacher__tags { display: flex; flex-wrap: wrap; gap: 12rpx; margin-top: 22rpx; }
.learn-teacher__tag { padding: 7rpx 13rpx; background: #F3EDE4; border-radius: 6rpx; color: #8D634D; font-size: 22rpx; }
.type-index__list { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12rpx; }
.type-index__item { display: flex; align-items: center; gap: 12rpx; min-height: 100rpx; padding: 10rpx 16rpx; border: 1rpx solid #E6E1D8; border-radius: 14rpx; font-size: 23rpx; }
.type-index__number { color: #A55C3B; font-family: Georgia, serif; font-size: 36rpx; }
.type-index__name { color: #77786F; font-size: 22rpx; }
.page-footnote { display: block; margin-top: 48rpx; color: #99978D; font-size: 22rpx; text-align: center; letter-spacing: 2rpx; }
@media screen and (min-width: 768px) { .learn-content { padding-top: 44rpx; } .course-row__desc { display: block; margin-top: 10rpx; color: #77786F; line-height: 1.7; } }
</style>
