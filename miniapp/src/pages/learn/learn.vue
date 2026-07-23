<script setup>
import { ref, onMounted } from 'vue'
import { TYPES_INFO } from '../../data/enneagramGame'
import { getStoredSiteConfig, hasSiteConfigLearningSection, refreshSiteConfig } from '../../utils/siteConfig'
import { userErrorMessage } from '../../utils/userMessage'
import { normalizeCoursewareItems, normalizeTeachers } from '../../utils/teacherCourseware'

const teachers = ref([])
const coursewareItems = ref([])
const quotes = ref([])
const types = ref(Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] })))
const teacherImageErrors = ref({})
const courseImageErrors = ref({})
const typeImageErrors = ref({})
const loading = ref(true)
const loadError = ref('')
let loadTicket = 0

function applyContent(cfg) {
  teachers.value = normalizeTeachers(cfg)
  coursewareItems.value = normalizeCoursewareItems(cfg)
  quotes.value = cfg?.home?.quotes?.items || []
  teacherImageErrors.value = {}
  courseImageErrors.value = {}
}

function showStoredContent() {
  const cached = getStoredSiteConfig()
  if (!cached) return false
  applyContent(cached)
  loading.value = false
  loadError.value = ''
  return true
}

async function loadContent(options = {}) {
  const silent = !!options.silent
  const ticket = ++loadTicket
  if (!silent) {
    loading.value = true
    loadError.value = ''
  }
  try {
    const cfg = await refreshSiteConfig()
    if (ticket !== loadTicket) return
    if (silent && !hasSiteConfigLearningSection(cfg)) return
    applyContent(cfg)
    loadError.value = ''
  } catch (e) {
    if (ticket !== loadTicket) return
    if (!silent) {
      teachers.value = normalizeTeachers()
      coursewareItems.value = normalizeCoursewareItems()
      quotes.value = []
      loadError.value = userErrorMessage(e, '内容加载失败，请稍后重试')
    }
  } finally {
    if (ticket === loadTicket) loading.value = false
  }
}

function markTeacherImageError(name) {
  teacherImageErrors.value = { ...teacherImageErrors.value, [name]: true }
}

function markCourseImageError(key) {
  courseImageErrors.value = { ...courseImageErrors.value, [key]: true }
}

function markTypeImageError(id) {
  typeImageErrors.value = { ...typeImageErrors.value, [id]: true }
}

onMounted(() => {
  const hasCachedContent = showStoredContent()
  loadContent({ silent: hasCachedContent })
})

function goTest() {
  uni.switchTab({ url: '/pages/index/index' })
}
</script>

<template>
  <view class="wrap learn page-stack ios-page ios-safe-bottom">
    <view class="learn-hero nx-page-hero">
      <text class="learn-hero__eyebrow">学习中心</text>
      <text class="learn-hero__title">跟着老师，把九型用进生活</text>
      <text class="learn-hero__lead">从理解自己开始，在关系与日常选择中练习更清醒的回应。</text>
    </view>

    <view class="learn-sections">
      <view class="card ios-card learn-section nx-panel section teacher-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">导师引领</text>
            <text class="sec-title">认识你的学习向导</text>
          </view>
        </view>
        <view v-if="loading" class="empty">老师资料加载中…</view>
        <view v-else-if="loadError" class="empty empty--error">
          <text>{{ loadError }}</text>
          <button class="retry" hover-class="retry--hover" @click="loadContent">重新加载</button>
        </view>
        <view v-for="teacher in teachers" :key="teacher.name" class="teacher-card">
          <image
            v-if="teacher.avatar && !teacherImageErrors[teacher.name]"
            class="teacher-media teacher-card__avatar"
            :src="teacher.avatar"
            mode="aspectFill"
            lazy-load
            @error="markTeacherImageError(teacher.name)"
          />
          <view v-else class="teacher-media__fallback" aria-hidden="true">
            {{ teacher.name ? teacher.name.slice(0, 1) : '师' }}
          </view>
          <view class="teacher-card__body">
            <text class="teacher-card__name">{{ teacher.name }}</text>
            <text class="teacher-card__title">{{ teacher.title }}</text>
            <text class="teacher-card__bio">{{ teacher.bio }}</text>
            <view v-if="teacher.tags.length" class="teacher-card__tags">
              <text v-for="tag in teacher.tags" :key="tag" class="nx-tag teacher-card__tag">{{ tag }}</text>
            </view>
          </view>
        </view>
      </view>

      <view class="card ios-card learn-section nx-panel section courseware-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">精选课程资料</text>
            <text class="sec-title">循序建立九型视角</text>
          </view>
        </view>
        <view v-if="loading" class="empty">课程内容加载中…</view>
        <block v-else>
          <view v-for="(c, i) in coursewareItems" :key="c.title + i" class="courseware-card">
            <image
              v-if="c.cover && !courseImageErrors[c.title + i]"
              class="course-media courseware-card__cover"
              :src="c.cover"
              mode="aspectFill"
              lazy-load
              @error="markCourseImageError(c.title + i)"
            />
            <view v-else class="course-media__fallback" aria-hidden="true">{{ c.badge || (i + 1) }}</view>
            <view class="courseware-card__body">
              <view class="courseware-card__meta">
                <text class="nx-tag courseware-card__badge">{{ c.badge || `第 ${i + 1} 课` }}</text>
                <text v-if="c.duration" class="courseware-card__duration">{{ c.duration }}</text>
              </view>
              <text class="courseware-card__title">{{ c.title }}</text>
              <text class="courseware-card__desc">{{ c.description }}</text>
            </view>
          </view>
        </block>
      </view>

      <view class="card ios-card learn-section nx-panel section quote-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">今日一念</text>
            <text class="sec-title">把觉察带回当下</text>
          </view>
        </view>
        <view v-if="loading" class="empty">语录内容加载中…</view>
        <view v-else-if="!loadError && quotes.length === 0" class="empty">语录内容即将上线</view>
        <view v-for="quote in quotes" :key="quote" class="quote-editorial">
          <text class="quote-editorial__mark" aria-hidden="true">“</text>
          <text class="quote-editorial__text">{{ quote }}</text>
        </view>
      </view>

      <view class="card ios-card learn-section nx-panel section type-section">
        <view class="nx-section-head">
          <view>
            <text class="section-kicker">九型图鉴</text>
            <text class="sec-title">九种性格，九条成长路径</text>
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
    </view>

    <button class="btn-primary ios-button learn-cta" hover-class="learn-cta--pressed" @click="goTest">先完成测试，建立你的学习地图</button>
  </view>
</template>

<style scoped>
.learn { background: #f4f8f6; }
.learn-hero {
  padding: 38rpx 34rpx 40rpx;
  border-radius: 38rpx;
  background: linear-gradient(135deg, #0f766e 0%, #15803d 100%);
  color: #ffffff;
  box-shadow: 0 24rpx 54rpx -34rpx rgba(15, 118, 110, .7);
}
.learn-hero__eyebrow { display: block; color: #ffffff; font-size: 24rpx; font-weight: 800; }
.learn-hero__title { display: block; margin-top: 14rpx; color: #ffffff; font-size: 44rpx; font-weight: 900; line-height: 1.28; }
.learn-hero__lead { display: block; margin-top: 16rpx; color: #ffffff; font-size: 26rpx; line-height: 1.65; }
.learn-sections { display: flex; flex-direction: column; gap: 22rpx; }
.learn-section { display: flex; flex-direction: column; padding: 30rpx; }
.section-kicker { display: block; color: #0f766e; font-size: 24rpx; font-weight: 800; }
.sec-title { display: block; margin-top: 8rpx; color: #17211d; font-size: 34rpx; font-weight: 900; line-height: 1.35; }
.empty { color: #475569; font-size: 26rpx; line-height: 1.6; padding: 24rpx 0; }
.empty--error { display: flex; align-items: center; justify-content: space-between; gap: 20rpx; }
.retry {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 88rpx;
  min-height: 88rpx;
  padding: 0 20rpx;
  color: #0f6b4f;
  font-size: 24rpx;
  font-weight: 900;
  touch-action: manipulation;
  background: #ecfdf5;
  border: 2rpx solid #a7f3d0;
  border-radius: 18rpx;
  line-height: 1;
}
.retry::after { border: none; }
.retry--hover { opacity: .82; transform: scale(.985); }
.teacher-card { display: flex; align-items: flex-start; gap: 22rpx; margin-top: 24rpx; }
.teacher-media,
.teacher-media__fallback {
  flex-shrink: 0;
  width: 112rpx;
  height: 112rpx;
  border-radius: 28rpx;
  box-sizing: border-box;
}
.teacher-media { width: 112rpx; height: 112rpx; background: #dbeee8; }
.teacher-media__fallback { display: flex; align-items: center; justify-content: center; width: 112rpx; height: 112rpx; color: #ffffff; background: #0f766e; font-size: 40rpx; font-weight: 900; }
.teacher-card__body { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 8rpx; }
.teacher-card__name { color: #17211d; font-size: 32rpx; font-weight: 900; line-height: 1.3; }
.teacher-card__title { color: #0f6b4f; font-size: 24rpx; font-weight: 800; }
.teacher-card__bio { color: #334155; font-size: 25rpx; line-height: 1.65; }
.teacher-card__tags { display: flex; flex-wrap: wrap; gap: 10rpx; margin-top: 4rpx; }
.teacher-card__tag { color: #0f6b4f; background: #ecfdf5; font-size: 24rpx; }
.courseware-card { display: flex; align-items: flex-start; gap: 18rpx; padding: 24rpx 0; border-bottom: 2rpx solid #e2e8f0; }
.courseware-card:last-child { padding-bottom: 0; border-bottom: none; }
.course-media,
.course-media__fallback {
  flex-shrink: 0;
  width: 220rpx;
  height: 150rpx;
  border-radius: 20rpx;
  box-sizing: border-box;
}
.course-media { width: 220rpx; height: 150rpx; background: #dbeee8; }
.course-media__fallback { display: flex; align-items: center; justify-content: center; width: 220rpx; height: 150rpx; padding: 16rpx; color: #ffffff; background: #15803d; font-size: 28rpx; font-weight: 900; text-align: center; }
.courseware-card__body { min-width: 0; flex: 1; }
.courseware-card__meta { display: flex; align-items: center; flex-wrap: wrap; gap: 10rpx; margin-bottom: 8rpx; }
.courseware-card__badge { color: #0f6b4f; background: #ecfdf5; font-size: 24rpx; }
.courseware-card__duration { color: #475569; font-size: 24rpx; font-weight: 700; }
.courseware-card__title { display: block; color: #17211d; font-size: 30rpx; font-weight: 900; line-height: 1.35; }
.courseware-card__desc { display: block; margin-top: 8rpx; color: #334155; font-size: 24rpx; line-height: 1.6; }
.quote-editorial { position: relative; margin-top: 20rpx; padding: 30rpx; border-radius: 22rpx; background: #ecfdf5; overflow: hidden; }
.quote-editorial__mark { display: block; color: #0f766e; font-family: Georgia, serif; font-size: 54rpx; font-weight: 900; line-height: 1; }
.quote-editorial__text { position: relative; display: block; margin-top: 8rpx; color: #1f3a31; font-size: 28rpx; font-weight: 700; line-height: 1.7; }
.type-badge-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16rpx; margin-top: 24rpx; }
.type-badge { min-width: 0; min-height: 190rpx; padding: 20rpx 14rpx; border: 2rpx solid #dbe7e2; border-radius: 20rpx; background: #f8fbfa; box-sizing: border-box; display: flex; flex-direction: column; align-items: center; justify-content: center; }
.type-badge__media { position: relative; }
.type-badge__avatar,
.type-badge__fallback { width: 78rpx; height: 78rpx; border-radius: 50%; box-sizing: border-box; }
.type-badge__avatar { width: 78rpx; height: 78rpx; background: #dbeee8; }
.type-badge__fallback { display: flex; align-items: center; justify-content: center; width: 78rpx; height: 78rpx; color: #ffffff; background: #0f766e; font-size: 28rpx; font-weight: 900; }
.type-badge__num { position: absolute; right: -12rpx; bottom: -4rpx; display: flex; align-items: center; justify-content: center; width: 36rpx; height: 36rpx; border-radius: 12rpx; color: #ffffff; background: #0f766e; font-size: 24rpx; font-weight: 900; }
.type-badge--blue .type-badge__num { background: #2563eb; }
.type-badge--red .type-badge__num { background: #dc2626; }
.type-badge__name { margin-top: 14rpx; color: #17211d; font-size: 27rpx; font-weight: 900; text-align: center; }
.type-badge__keywords { margin-top: 6rpx; color: #475569; font-size: 24rpx; line-height: 1.45; text-align: center; overflow-wrap: anywhere; }
.learn-cta { min-height: 88rpx; margin-top: 4rpx; font-size: 28rpx; touch-action: manipulation; }
.learn-cta--pressed { opacity: .86; transform: scale(.99); }

@media (min-width: 768px) {
  .type-badge-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}

</style>
