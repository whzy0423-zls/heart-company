<script setup>
import { computed, onMounted, ref } from 'vue'
import { TYPES_INFO } from '../../data/enneagramGame'
import { resolveContentAsset } from '../../utils/contentAsset'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import {
  applyLearningContent,
  createActionActivationGuard,
  createInitialLearningContent,
  createLatestRequestGuard,
  createOneShotFallbackRegistry,
  handleActionKeydown,
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
const initialContent = createInitialLearningContent()
const teachers = ref(initialContent.teachers)
const coursewareItems = ref(initialContent.coursewareItems)
const quotes = ref(initialContent.quotes)
const types = ref(Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] })))
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

function goTest() {
  uni.navigateTo({ url: '/pages/test/test' })
}
</script>

<template>
  <view class="wrap learn page-stack ios-page ios-safe-bottom">
    <view class="learn-content">
      <header class="learn-header">
        <text class="learn-header__title">跟随老师，认识真实的自己</text>
        <text class="learn-header__intro">从老师介绍、课程资料和九型速查开始，按自己的节奏学习。</text>
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
        <view v-if="teacher" class="learn-teacher__portrait">
          <image
            class="learn-teacher__image"
            :src="teacherImage"
            mode="aspectFill"
            role="img"
            :aria-label="teacherImageLabel"
            @error="onTeacherImageError"
          />
        </view>

        <view v-if="teacher" class="learn-teacher__copy">
          <text class="section-label">主讲老师</text>
          <text id="learn-teacher-heading" class="learn-teacher__name">{{ teacher.name }}</text>
          <text class="learn-teacher__title">{{ teacher.title }}</text>
          <text class="learn-teacher__bio">{{ teacher.bio }}</text>
          <view v-if="teacher.tags.length" class="learn-teacher__tags" aria-label="老师擅长领域">
            <text v-for="(tag, tagIndex) in teacher.tags" :key="`${teacher.name}::tag::${tagIndex}::${tag}`" class="learn-teacher__tag">{{ tag }}</text>
          </view>
          <view
            class="learn-primary"
            role="button"
            tabindex="0"
            hover-class="control--pressed"
            @click="activateAction(goTest, $event)"
            @keydown="onActionKeydown($event, goTest)"
          >开始九型自测</view>
        </view>

        <view v-else class="editorial-empty learn-teacher__empty">
          <text id="learn-teacher-heading" class="editorial-empty__title">老师资料整理中</text>
          <text>课程团队正在整理主讲老师的介绍与研究方向。</text>
        </view>
      </section>

      <section class="course-section" aria-labelledby="course-heading">
        <view class="section-heading">
          <text class="section-label">课程资料</text>
          <text id="course-heading" class="section-heading__title">循序学习</text>
        </view>

        <view v-if="coursewareItems.length" class="course-list">
          <article
            v-for="(course, index) in coursewareItems"
            :key="`${course.title}::course::${index}`"
            class="course-row"
          >
            <image
              class="course-row__cover"
              :src="courseImages[index]"
              mode="aspectFill"
              aria-hidden="true"
              lazy-load
              @error="onCourseImageError(index)"
            />
            <view class="course-row__copy">
              <text class="course-row__title">{{ course.title }}</text>
              <view class="course-row__meta">
                <view v-if="course.materialTypes.length" class="material-types" aria-label="资料形式">
                  <text v-for="(type, typeIndex) in course.materialTypes" :key="`${course.title}::material::${index}::${typeIndex}::${type}`">{{ type }}</text>
                </view>
                <text v-if="course.duration" class="course-row__duration">{{ course.duration }}</text>
              </view>
              <text class="course-row__desc">{{ course.description }}</text>
            </view>
          </article>
        </view>

        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">课程资料整理中</text>
          <text>新一期课件、视频与音频资料正在编校入库。</text>
        </view>
      </section>

      <section class="quote-section" aria-labelledby="quote-heading">
        <text id="quote-heading" class="section-label">老师寄语</text>
        <view v-if="quotes.length" class="quote-card">
          <view class="pull-quote">
            <text class="pull-quote__text">“{{ quotes[0] }}”</text>
            <text class="quote-card__mark">”</text>
          </view>
        </view>
        <view v-else class="quote-empty">老师的课堂札记正在整理中。</view>
      </section>

      <section class="type-index" aria-labelledby="type-index-heading">
        <view class="type-index__heading">
          <text id="type-index-heading">九型速查</text>
        </view>
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
  padding: 28rpx 24rpx 56rpx;
  box-sizing: border-box;
}

.learn-header {
  padding: 12rpx 0 28rpx;
  display: flex;
  flex-direction: column;
  gap: 10rpx;
}

.learn-header__title {
  font-family: "Songti SC", "STSong", serif;
  font-size: 40rpx;
  font-weight: 900;
  line-height: 1.3;
}

.learn-header__intro {
  color: rgba(32, 37, 43, .72);
  font-size: 24rpx;
  line-height: 1.65;
}

.learn-sync,
.learn-error {
  min-height: 64rpx;
  margin-bottom: 16rpx;
  padding: 10rpx 16rpx;
  display: flex;
  align-items: center;
  gap: 16rpx;
  box-sizing: border-box;
  border: 2rpx solid rgba(51, 91, 74, .2);
  border-radius: 16rpx;
  background: var(--learn-surface);
  color: rgba(32, 37, 43, .72);
  font-size: 22rpx;
  line-height: 1.5;
}

.learn-error {
  justify-content: space-between;
  color: var(--learn-cinnabar);
  border-color: rgba(164, 60, 44, .28);
}

.learn-retry {
  flex: 0 0 auto;
  min-height: 88rpx;
  padding: 0 22rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--learn-cinnabar);
  font-size: 23rpx;
  font-weight: 900;
  touch-action: manipulation;
}

.learn-teacher {
  min-width: 0;
  padding: 20rpx;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 22rpx;
  border: 2rpx solid rgba(51, 91, 74, .2);
  border-radius: 20rpx;
  background: var(--learn-surface);
}

.learn-teacher__portrait {
  position: relative;
  width: 100%;
  aspect-ratio: 4 / 5;
  overflow: hidden;
  border-radius: 16rpx;
  background: var(--learn-bg);
}

.learn-teacher__image {
  display: block;
  width: 100%;
  height: 100%;
}

.learn-teacher__copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 12rpx;
}

.section-label {
  color: var(--learn-green);
  font-size: 21rpx;
  font-weight: 900;
  line-height: 1.4;
}

.learn-teacher__name {
  font-family: "Songti SC", "STSong", serif;
  font-size: 44rpx;
  font-weight: 900;
  line-height: 1.2;
}

.learn-teacher__title {
  color: var(--learn-green);
  font-size: 24rpx;
  font-weight: 800;
  line-height: 1.5;
}

.learn-teacher__bio {
  color: rgba(32, 37, 43, .76);
  font-size: 24rpx;
  line-height: 1.65;
}

.learn-teacher__tags,
.material-types {
  display: flex;
  flex-wrap: wrap;
  gap: 10rpx;
}

.learn-teacher__tag,
.material-types text {
  padding: 6rpx 12rpx;
  border: 2rpx solid rgba(51, 91, 74, .3);
  border-radius: 16rpx;
  color: var(--learn-green);
  font-size: 19rpx;
  font-weight: 800;
  line-height: 1.3;
}

.learn-primary {
  width: 100%;
  min-height: 88rpx;
  margin-top: 14rpx;
  padding: 0 28rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  border-radius: 20rpx;
  background: var(--learn-green);
  color: var(--learn-surface);
  font-size: 26rpx;
  font-weight: 900;
  line-height: 1.4;
  text-align: center;
  touch-action: manipulation;
}

.learn-teacher__empty { grid-column: 1 / -1; }

.course-section { padding: 44rpx 0 12rpx; }

.section-heading {
  padding-bottom: 16rpx;
  display: flex;
  flex-direction: column;
  gap: 6rpx;
  border-bottom: 2rpx solid rgba(51, 91, 74, .28);
}

.section-heading__title {
  display: block;
  font-family: "Songti SC", "STSong", serif;
  font-size: 34rpx;
  font-weight: 900;
  line-height: 1.35;
}

.course-list {
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
}

.course-row {
  min-width: 0;
  padding: 22rpx 0;
  display: grid;
  grid-template-columns: 132rpx minmax(0, 1fr);
  gap: 20rpx;
  border-bottom: 2rpx solid rgba(32, 37, 43, .14);
}

.course-row__cover {
  display: block;
  width: 100%;
  aspect-ratio: 4 / 3;
  border-radius: 16rpx;
  background: var(--learn-surface);
}

.course-row__copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8rpx;
}

.course-row__meta {
  width: 100%;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8rpx 14rpx;
}

.course-row__title {
  font-family: "Songti SC", "STSong", serif;
  font-size: 28rpx;
  font-weight: 900;
  line-height: 1.35;
}

.course-row__duration {
  color: var(--learn-green);
  font-size: 19rpx;
  font-weight: 800;
}

.course-row__desc {
  color: rgba(32, 37, 43, .7);
  font-size: 22rpx;
  line-height: 1.55;
}

.editorial-empty {
  min-height: 180rpx;
  padding: 30rpx 4rpx;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 10rpx;
  border-bottom: 2rpx solid rgba(32, 37, 43, .18);
  color: rgba(32, 37, 43, .7);
  font-size: 24rpx;
  line-height: 1.6;
}

.editorial-empty__title {
  color: var(--learn-ink);
  font-family: "Songti SC", "STSong", serif;
  font-size: 32rpx;
  font-weight: 900;
}

.quote-section {
  padding: 36rpx 0;
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  border-bottom: 2rpx solid rgba(32, 37, 43, .16);
}

.pull-quote {
  position: relative;
  min-height: 120rpx;
  padding: 24rpx 58rpx 24rpx 24rpx;
  display: flex;
  align-items: center;
  box-sizing: border-box;
  border-left: 6rpx solid var(--learn-green);
  border-radius: 0 16rpx 16rpx 0;
  background: var(--learn-surface);
}

.pull-quote__text {
  color: var(--learn-ink);
  font-family: "Songti SC", "STSong", serif;
  font-size: 28rpx;
  font-weight: 800;
  line-height: 1.65;
}

.quote-card__mark {
  position: absolute;
  top: 8rpx;
  right: 16rpx;
  color: rgba(51, 91, 74, .18);
  font-family: Georgia, serif;
  font-size: 64rpx;
  line-height: 1;
  pointer-events: none;
}

.quote-empty {
  padding: 26rpx 0;
  color: rgba(32, 37, 43, .7);
  font-size: 23rpx;
}

.type-index { padding: 32rpx 0 8rpx; }

.type-index__heading {
  padding-bottom: 14rpx;
  border-bottom: 2rpx solid rgba(51, 91, 74, .22);
  font-family: "Songti SC", "STSong", serif;
  font-size: 26rpx;
  font-weight: 900;
}

.type-index__list {
  padding-top: 14rpx;
  display: flex;
  flex-wrap: wrap;
  gap: 10rpx;
}

.type-index__item {
  padding: 7rpx 12rpx;
  display: flex;
  align-items: center;
  gap: 6rpx;
  border: 2rpx solid rgba(51, 91, 74, .22);
  border-radius: 16rpx;
  background: var(--learn-surface);
  color: var(--learn-green);
  font-size: 19rpx;
  line-height: 1.35;
}

.type-index__number {
  font-weight: 900;
}

.type-index__name {
  font-weight: 800;
}

.control--pressed { opacity: .82; transform: translateY(2rpx); }

.learn-retry:focus-visible,
.learn-primary:focus-visible {
  outline: 4rpx solid var(--learn-cinnabar);
  outline-offset: 4rpx;
}

@media screen and (min-width: 768px) {
  .learn-content { padding-left: 48rpx; padding-right: 48rpx; }

  .learn-teacher {
    grid-template-columns: minmax(240rpx, .72fr) minmax(0, 1.28fr);
    align-items: center;
    padding: 28rpx;
  }

  .learn-teacher__copy { padding-left: 20rpx; }
  .learn-primary { width: 100%; max-width: 360rpx; min-width: 0; box-sizing: border-box; }
}
</style>
