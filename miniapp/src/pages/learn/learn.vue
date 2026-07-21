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
    <view class="learn-paper">
      <header class="learn-masthead">
        <text class="learn-masthead__brand">九型芯之力</text>
        <text class="learn-masthead__edition">人物与课程 · 学习专刊</text>
      </header>

      <view v-if="loading" class="learn-sync" role="status">
        <text class="learn-sync__rule" aria-hidden="true" />
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
          <view class="learn-teacher__stamp" aria-hidden="true">
            <text>老师专访</text>
            <text>PROFILE</text>
          </view>
        </view>

        <view v-if="teacher" class="learn-teacher__copy">
          <text class="editorial-kicker">主讲老师 / PERSONAL VOICE</text>
          <text id="learn-teacher-heading" class="learn-teacher__name">{{ teacher.name }}</text>
          <text class="learn-teacher__title">{{ teacher.title }}</text>
          <view class="learn-teacher__rule" aria-hidden="true" />
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
          >先完成九型自测，再开始系统学习 →</view>
        </view>

        <view v-else class="editorial-empty learn-teacher__empty">
          <text id="learn-teacher-heading" class="editorial-empty__title">老师资料整理中</text>
          <text>课程团队正在整理主讲老师的介绍与研究方向。</text>
        </view>
      </section>

      <section class="publication-section" aria-labelledby="publication-heading">
        <view class="section-heading">
          <view>
            <text class="editorial-kicker">COURSEWARE PUBLICATION / 课件出版</text>
            <text id="publication-heading" class="section-heading__title">把课程留成可反复翻阅的资料书架</text>
          </view>
          <text class="section-heading__note">课件 · 视频 · 音频 · 练习</text>
        </view>

        <view v-if="coursewareItems.length" class="publication-list">
          <article
            v-for="(course, index) in coursewareItems"
            :key="`${course.title}::course::${index}`"
            class="publication-card"
          >
            <view class="publication-card__visual">
              <image
                class="publication-card__cover"
                :class="`publication-card__cover--${['book', 'magazine', 'folio'][index % 3]}`"
                :src="courseImages[index]"
                mode="aspectFill"
                aria-hidden="true"
                lazy-load
                @error="onCourseImageError(index)"
              />
              <text class="publication-card__number" aria-hidden="true">0{{ index + 1 }}</text>
            </view>
            <view class="publication-card__copy">
              <view class="publication-card__meta">
                <text>{{ course.badge }}</text>
                <text v-if="course.duration">{{ course.duration }}</text>
              </view>
              <text class="publication-card__title">{{ course.title }}</text>
              <text class="publication-card__desc">{{ course.description }}</text>
              <view v-if="course.materialTypes.length" class="material-types" aria-label="资料形式">
                <text v-for="(type, typeIndex) in course.materialTypes" :key="`${course.title}::material::${index}::${typeIndex}::${type}`">{{ type }}</text>
              </view>
              <view v-if="course.bullets.length" class="publication-card__bullets">
                <text v-for="(bullet, bulletIndex) in course.bullets" :key="`${course.title}::bullet::${index}::${bulletIndex}::${bullet}`">— {{ bullet }}</text>
              </view>
              <text class="publication-card__status">馆藏资料 · 持续更新</text>
            </view>
          </article>
        </view>

        <view v-else class="editorial-empty">
          <text class="editorial-empty__title">课程资料整理中</text>
          <text>新一期课件、视频与音频资料正在编校入库。</text>
        </view>
      </section>

      <section class="quote-section" aria-labelledby="quote-heading">
        <view class="quote-section__heading">
          <text class="editorial-kicker">TEACHER'S NOTE / 课堂札记</text>
          <text id="quote-heading">从一句话，回到真实的自己</text>
        </view>
        <view v-if="quotes.length" class="quote-stack">
          <view v-for="(quote, quoteIndex) in quotes" :key="`quote::${quoteIndex}::${quote}`" class="quote-card">
            <view class="pull-quote">
              <text class="quote-card__text">“{{ quote }}”</text>
              <text class="quote-card__mark">”</text>
            </view>
          </view>
        </view>
        <view v-else class="quote-empty">老师的课堂札记正在整理中。</view>
      </section>

      <section class="type-index" aria-labelledby="type-index-heading">
        <view class="type-index__heading">
          <text class="editorial-kicker">REFERENCE / 九型索引</text>
          <text id="type-index-heading">九种类型速查</text>
        </view>
        <view class="type-index__list">
          <view v-for="type in types" :key="`type::${type.id}`" class="type-index__item">
            <text class="type-index__number">{{ type.id }}</text>
            <view class="type-index__copy">
              <text class="type-index__name">{{ type.name }}</text>
              <text class="type-index__keywords">{{ type.keywords }}</text>
            </view>
          </view>
        </view>
      </section>
    </view>
  </view>
</template>

<style scoped>
.learn {
  --paper: #F6F0E4;
  --paper-deep: #E8DCC5;
  --ink: #24241F;
  --muted: #665F54;
  --green: #173F35;
  --green-soft: #D9E2D8;
  --cinnabar: #A43C2C;
  --sand: #CFB785;
  min-width: 0;
  overflow-x: hidden;
  background: var(--paper);
  color: var(--ink);
}

.learn-paper {
  width: 100%;
  max-width: 1100rpx;
  margin: 0 auto;
  padding: 20rpx 24rpx 56rpx;
  box-sizing: border-box;
  background:
    repeating-linear-gradient(0deg, transparent 0, transparent 15rpx, rgba(36, 36, 31, .018) 16rpx),
    var(--paper);
}

.learn-masthead {
  padding: 20rpx 0 16rpx;
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16rpx;
  border-bottom: 2rpx solid var(--ink);
}

.learn-masthead__brand {
  font-family: "Songti SC", "STSong", serif;
  font-size: 35rpx;
  font-weight: 900;
  letter-spacing: 3rpx;
}

.learn-masthead__edition,
.editorial-kicker,
.section-heading__note {
  color: var(--muted);
  font-size: 19rpx;
  font-weight: 800;
  line-height: 1.45;
  letter-spacing: 1rpx;
}

.learn-sync,
.learn-error {
  min-height: 70rpx;
  padding: 8rpx 0;
  display: flex;
  align-items: center;
  gap: 16rpx;
  box-sizing: border-box;
  color: var(--muted);
  font-size: 22rpx;
  line-height: 1.5;
}

.learn-sync__rule {
  flex: 0 0 36rpx;
  width: 36rpx;
  height: 2rpx;
  background: var(--cinnabar);
}

.learn-error {
  justify-content: space-between;
  color: #752A20;
  border-bottom: 2rpx solid rgba(117, 42, 32, .24);
}

.learn-retry {
  flex: 0 0 auto;
  min-height: 88rpx;
  padding: 0 22rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #752A20;
  font-size: 23rpx;
  font-weight: 900;
  transition: opacity .2s ease, transform .2s ease;
  touch-action: manipulation;
}

.learn-teacher {
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  border-bottom: 2rpx solid var(--ink);
}

.learn-teacher__portrait {
  position: relative;
  width: 100%;
  aspect-ratio: 4 / 5;
  overflow: hidden;
  background: var(--paper-deep);
}

.learn-teacher__image {
  display: block;
  width: 100%;
  height: 100%;
}

.learn-teacher__stamp {
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

.learn-teacher__copy {
  min-width: 0;
  padding: 36rpx 4rpx 44rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16rpx;
}

.editorial-kicker { color: var(--cinnabar); }

.learn-teacher__name {
  font-family: "Songti SC", "STSong", serif;
  font-size: 62rpx;
  font-weight: 900;
  line-height: 1.12;
  letter-spacing: 2rpx;
}

.learn-teacher__title {
  color: var(--green);
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.5;
}

.learn-teacher__rule {
  width: 82rpx;
  height: 4rpx;
  margin: 4rpx 0;
  background: var(--sand);
}

.learn-teacher__bio {
  max-width: 640rpx;
  color: #4E4A42;
  font-size: 27rpx;
  line-height: 1.75;
}

.learn-teacher__tags,
.material-types {
  display: flex;
  flex-wrap: wrap;
  gap: 10rpx;
}

.learn-teacher__tag,
.material-types text {
  padding: 7rpx 14rpx;
  border: 2rpx solid rgba(23, 63, 53, .38);
  color: var(--green);
  font-size: 20rpx;
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
  background: var(--green);
  color: #FFFFFF;
  font-size: 26rpx;
  font-weight: 900;
  line-height: 1.4;
  text-align: center;
  box-shadow: 8rpx 8rpx 0 var(--sand);
  transition: opacity .2s ease, transform .2s ease;
  touch-action: manipulation;
}

.learn-teacher__empty { grid-column: 1 / -1; }

.publication-section { padding: 52rpx 0 16rpx; }

.section-heading {
  padding-bottom: 24rpx;
  display: flex;
  flex-direction: column;
  gap: 16rpx;
  border-bottom: 4rpx solid var(--green);
}

.section-heading__title {
  display: block;
  max-width: 720rpx;
  margin-top: 10rpx;
  font-family: "Songti SC", "STSong", serif;
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.32;
}

.publication-list {
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
}

.publication-card {
  min-width: 0;
  padding: 30rpx 0 34rpx;
  display: grid;
  grid-template-columns: 190rpx minmax(0, 1fr);
  gap: 24rpx;
  border-bottom: 2rpx solid rgba(36, 36, 31, .28);
}

.publication-card__visual {
  position: relative;
  align-self: start;
  min-width: 0;
}

.publication-card__cover {
  display: block;
  width: 100%;
  background: var(--green-soft);
  box-shadow: 8rpx 10rpx 0 rgba(207, 183, 133, .72);
}

.publication-card__cover--book { aspect-ratio: 3 / 4; }
.publication-card__cover--magazine { aspect-ratio: 4 / 5; }
.publication-card__cover--folio { aspect-ratio: 4 / 3; }

.publication-card__number {
  position: absolute;
  right: -8rpx;
  bottom: -12rpx;
  padding: 5rpx 9rpx;
  background: var(--cinnabar);
  color: #FFFFFF;
  font-size: 17rpx;
  font-weight: 900;
  letter-spacing: 1rpx;
}

.publication-card__copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10rpx;
}

.publication-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8rpx 16rpx;
  color: var(--cinnabar);
  font-size: 19rpx;
  font-weight: 900;
}

.publication-card__title {
  font-family: "Songti SC", "STSong", serif;
  font-size: 33rpx;
  font-weight: 900;
  line-height: 1.32;
}

.publication-card__desc {
  color: var(--muted);
  font-size: 24rpx;
  line-height: 1.65;
}

.publication-card__bullets {
  display: flex;
  flex-direction: column;
  gap: 6rpx;
  color: #4E4A42;
  font-size: 21rpx;
  line-height: 1.52;
}

.publication-card__status {
  margin-top: auto;
  padding-top: 6rpx;
  color: var(--green);
  font-size: 19rpx;
  font-weight: 800;
  letter-spacing: 1rpx;
}

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

.editorial-empty__title {
  color: var(--ink);
  font-family: "Songti SC", "STSong", serif;
  font-size: 32rpx;
  font-weight: 900;
}

.quote-section {
  padding: 46rpx 0;
  border-top: 2rpx solid var(--ink);
  border-bottom: 2rpx solid var(--ink);
}

.quote-section__heading {
  margin-bottom: 24rpx;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
  font-family: "Songti SC", "STSong", serif;
  font-size: 34rpx;
  font-weight: 900;
  line-height: 1.35;
}

.quote-stack { display: grid; gap: 20rpx; }

.quote-card {
  position: relative;
  overflow: hidden;
  background: var(--green);
  color: #FFFFFF;
}

.pull-quote {
  position: relative;
  min-height: 160rpx;
  padding: 34rpx 86rpx 34rpx 30rpx;
  display: flex;
  align-items: center;
  box-sizing: border-box;
}

.quote-card__text {
  position: relative;
  z-index: 1;
  font-family: "Songti SC", "STSong", serif;
  font-size: 31rpx;
  font-weight: 800;
  line-height: 1.7;
}

.quote-card__mark {
  position: absolute;
  top: 2rpx;
  right: 16rpx;
  color: rgba(255, 255, 255, .15);
  font-family: Georgia, serif;
  font-size: 126rpx;
  line-height: 1;
  pointer-events: none;
}

.quote-empty {
  padding: 26rpx 0;
  color: var(--muted);
  font-size: 23rpx;
}

.type-index { padding: 42rpx 0 8rpx; }

.type-index__heading {
  padding-bottom: 18rpx;
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 18rpx;
  border-bottom: 2rpx solid var(--green);
  font-family: "Songti SC", "STSong", serif;
  font-size: 30rpx;
  font-weight: 900;
}

.type-index__list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.type-index__item {
  min-width: 0;
  min-height: 88rpx;
  padding: 10rpx 8rpx;
  display: flex;
  align-items: center;
  gap: 8rpx;
  box-sizing: border-box;
  border-bottom: 2rpx solid rgba(36, 36, 31, .18);
}

.type-index__number {
  flex: 0 0 38rpx;
  width: 38rpx;
  height: 38rpx;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--sand);
  color: var(--ink);
  font-size: 19rpx;
  font-weight: 900;
}

.type-index__copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2rpx;
}

.type-index__name {
  font-size: 21rpx;
  font-weight: 900;
  line-height: 1.3;
}

.type-index__keywords {
  overflow: hidden;
  color: var(--muted);
  font-size: 16rpx;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.control--pressed { opacity: .82; transform: translateY(2rpx); }

.learn-retry:focus-visible,
.learn-primary:focus-visible {
  outline: 4rpx solid #C84D3A;
  outline-offset: 4rpx;
}

@media screen and (min-width: 768px) {
  .learn-paper { padding-left: 48rpx; padding-right: 48rpx; }

  .learn-teacher {
    grid-template-columns: minmax(0, .9fr) minmax(0, 1.1fr);
    align-items: stretch;
  }

  .learn-teacher__copy { padding: 54rpx 0 54rpx 48rpx; justify-content: center; }
  .learn-primary { width: 100%; max-width: 360rpx; min-width: 0; box-sizing: border-box; }

  .section-heading {
    flex-direction: row;
    align-items: flex-end;
    justify-content: space-between;
  }

  .publication-card { grid-template-columns: 180rpx minmax(0, 1fr); }
  .quote-stack { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .type-index__list { grid-template-columns: repeat(9, minmax(0, 1fr)); }
  .type-index__item { flex-direction: column; justify-content: center; text-align: center; }
  .type-index__keywords { display: none; }
}

@media (prefers-reduced-motion: reduce) {
  .learn-retry,
  .learn-primary {
    transition: none;
  }
}
</style>
