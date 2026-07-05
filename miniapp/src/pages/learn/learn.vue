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
const loading = ref(true)
const loadError = ref('')
let loadTicket = 0

function applyContent(cfg) {
  teachers.value = normalizeTeachers(cfg)
  coursewareItems.value = normalizeCoursewareItems(cfg)
  quotes.value = cfg?.home?.quotes?.items || []
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
    <view class="card ios-card section teacher-section">
      <text class="eyebrow">老师资料</text>
      <text class="sec-title">跟着老师系统学习</text>
      <view v-if="loading" class="empty">老师资料加载中…</view>
      <view v-else-if="loadError" class="empty empty--error">
        <text>{{ loadError }}</text>
        <button class="retry" hover-class="retry--hover" @click="loadContent">重新加载</button>
      </view>
      <view v-for="teacher in teachers" :key="teacher.name" class="teacher-card">
        <image class="teacher-card__avatar" :src="teacher.avatar" mode="aspectFill" lazy-load />
        <view class="teacher-card__body">
          <text class="teacher-card__name">{{ teacher.name }}</text>
          <text class="teacher-card__title">{{ teacher.title }}</text>
          <text class="teacher-card__bio">{{ teacher.bio }}</text>
          <view v-if="teacher.tags.length" class="teacher-card__tags">
            <text v-for="tag in teacher.tags" :key="tag" class="teacher-card__tag">{{ tag }}</text>
          </view>
        </view>
      </view>
    </view>

    <view class="card ios-card section courseware-section">
      <text class="eyebrow">课程资料</text>
      <text class="sec-title">课件 / 课程展示</text>
      <view v-if="loading" class="empty">课件内容加载中…</view>
      <block v-else>
        <view v-for="(c, i) in coursewareItems" :key="c.title + i" class="courseware-card">
          <image class="courseware-card__cover" :src="c.cover" mode="aspectFill" lazy-load />
          <view class="courseware-card__body">
            <view class="courseware-card__meta">
              <text class="chip courseware-card__badge">{{ c.badge || (i + 1) }}</text>
              <text v-if="c.duration" class="courseware-card__duration">{{ c.duration }}</text>
            </view>
            <text class="courseware-card__title">{{ c.title }}</text>
            <text class="courseware-card__desc">{{ c.description }}</text>
          </view>
        </view>
      </block>
    </view>

    <view class="card ios-card section">
      <text class="eyebrow">老韩语录</text>
      <text class="sec-title">语录互动区</text>
      <view v-if="loading" class="empty">语录内容加载中…</view>
      <view v-else-if="!loadError && quotes.length === 0" class="empty">语录内容即将上线</view>
      <view v-for="quote in quotes" :key="quote" class="quote-card">
        <text class="quote-card__text">“{{ quote }}”</text>
        <text class="quote-card__mark">”</text>
      </view>
    </view>

    <view class="card ios-card section">
      <text class="eyebrow">九型图鉴</text>
      <text class="sec-title">九种性格图鉴</text>
      <view class="types">
        <view v-for="t in types" :key="t.id" class="type" :class="'type--' + t.color">
          <image class="type__avatar" :src="`/static/avatars/${t.id}.png`" mode="aspectFill" lazy-load />
          <text class="type__num">{{ t.id }}</text>
          <text class="type__name">{{ t.name }}</text>
          <text class="type__kw">{{ t.keywords }}</text>
        </view>
      </view>
    </view>

    <button class="btn-primary ios-button" @click="goTest">先测类型，再跟课学习 →</button>
  </view>
</template>

<style scoped>
.sec-title { font-size: 34rpx; font-weight: 900; display: block; margin: 16rpx 0 20rpx; }
.section { display: flex; flex-direction: column; }
.empty { color: #767d89; font-size: 26rpx; padding: 20rpx 0; }
.empty--error { display: flex; align-items: center; justify-content: space-between; gap: 20rpx; }
.retry {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 88rpx;
  min-height: 88rpx;
  padding: 0 18rpx;
  color: #2b7fff;
  font-weight: 900;
  touch-action: manipulation;
  background: transparent;
  border: none;
  line-height: 1;
}
.retry::after { border: none; }
.retry--hover { opacity: .82; transform: scale(.985); }
.teacher-card { display: flex; gap: 22rpx; padding: 24rpx; border-radius: 28rpx; background: rgba(255,255,255,.72); border: 2rpx solid rgba(255,255,255,.88); box-shadow: 0 16rpx 38rpx -28rpx rgba(28,40,70,.42); }
.teacher-card__avatar { flex-shrink: 0; width: 132rpx; height: 132rpx; border-radius: 28rpx; border: 4rpx solid rgba(255,255,255,.94); background: #f8fafc; }
.teacher-card__body { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 8rpx; }
.teacher-card__name { color: #12151b; font-size: 32rpx; font-weight: 900; line-height: 1.25; }
.teacher-card__title { color: #2563eb; font-size: 24rpx; font-weight: 900; }
.teacher-card__bio { color: #3c424d; font-size: 25rpx; line-height: 1.62; }
.teacher-card__tags { display: flex; flex-wrap: wrap; gap: 10rpx; margin-top: 4rpx; }
.teacher-card__tag { min-height: 44rpx; padding: 0 16rpx; border-radius: 999rpx; background: rgba(37,99,235,.09); color: #2563eb; font-size: 21rpx; font-weight: 900; display: inline-flex; align-items: center; }
.courseware-card { display: flex; gap: 18rpx; padding: 22rpx 0; border-bottom: 2rpx solid rgba(20,24,32,.07); }
.courseware-card:last-child { border-bottom: none; }
.courseware-card__cover { flex-shrink: 0; width: 152rpx; height: 112rpx; border-radius: 22rpx; background: #e2e8f0; }
.courseware-card__body { min-width: 0; flex: 1; }
.courseware-card__meta { display: flex; align-items: center; gap: 12rpx; margin-bottom: 8rpx; }
.courseware-card__badge { flex-shrink: 0; }
.courseware-card__duration { color: #64748b; font-size: 22rpx; font-weight: 800; }
.courseware-card__title { color: #12151b; font-size: 30rpx; font-weight: 900; display: block; }
.courseware-card__desc { color: #3c424d; font-size: 25rpx; line-height: 1.65; display: block; margin-top: 6rpx; }
.quote-card {
  position: relative;
  min-height: 116rpx;
  margin-bottom: 18rpx;
  padding: 28rpx 82rpx 28rpx 28rpx;
  border-radius: 24rpx;
  background: rgba(255,255,255,.68);
  border: 2rpx solid rgba(255,255,255,.86);
  box-shadow: 0 16rpx 38rpx -28rpx rgba(28,40,70,.42);
  overflow: hidden;
  box-sizing: border-box;
}
.quote-card:last-child { margin-bottom: 0; }
.quote-card__text {
  position: relative;
  z-index: 1;
  color: #12151b;
  font-size: 28rpx;
  font-weight: 800;
  line-height: 1.7;
}
.quote-card__mark {
  position: absolute;
  top: 8rpx;
  right: 18rpx;
  z-index: 0;
  color: rgba(43,127,255,.13);
  font-family: Georgia, serif;
  font-size: 110rpx;
  line-height: 1;
  pointer-events: none;
}
.types { display: flex; flex-wrap: wrap; gap: 16rpx; }
.type {
  position: relative;
  width: calc((100% - 32rpx) / 3);
  min-height: 236rpx;
  box-sizing: border-box;
  background: rgba(255,255,255,.68);
  border: 2rpx solid rgba(255,255,255,.86);
  border-radius: 24rpx;
  padding: 18rpx 10rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6rpx;
  box-shadow: 0 10rpx 28rpx -24rpx rgba(28,40,70,.42);
}
.type__avatar {
  width: 82rpx;
  height: 82rpx;
  border-radius: 50%;
  border: 4rpx solid rgba(255,255,255,.92);
  box-shadow: 0 10rpx 24rpx -18rpx rgba(28,40,70,.46);
}
.type__num {
  position: absolute;
  top: 70rpx;
  right: 22rpx;
  width: 38rpx;
  height: 38rpx;
  border-radius: 13rpx;
  color: #fff;
  font-weight: 900;
  font-size: 21rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}
.type--green .type__num { background: #25b365; }
.type--blue .type__num { background: #2b7fff; }
.type--red .type__num { background: #e23a47; }
.type__name { color: #12151b; font-size: 26rpx; font-weight: 900; }
.type__kw { font-size: 18rpx; color: #767d89; text-align: center; line-height: 1.45; }
</style>
