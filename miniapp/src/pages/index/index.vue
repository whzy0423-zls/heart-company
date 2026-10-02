<script setup>
import { computed, onMounted, ref } from 'vue'
import { onResize, onShow, onShareAppMessage, onShareTimeline } from '@dcloudio/uni-app'
import NxIcon from '../../components/NxIcon.vue'
import NxImagePreview from '../../components/NxImagePreview.vue'
import NxShareActions from '../../components/NxShareActions.vue'
import { buildShareCard, showPublicShareMenu, requireFullMiniapp } from '../../utils/share'
import { listClassroomRecentApi } from '../../api'
import studioVideos from '../../data/studioVideos.json'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import { normalizeTeachers, normalizeMiniappCourses } from '../../utils/teacherCourseware'
import { normalizeMiniappLearn } from '../../utils/miniappPages'
import { classroomContentRoute } from '../../utils/classroomDisplay'
import { setBookingIntent, clearBookingIntent } from '../../utils/bookingIntent'
import { STUDIO_TEACHER, STUDIO_COURSES } from '../../data/teacherStudio'
import { isCourseRegistrationEnabled } from '../../utils/courseRegistration'
import { UI_PREVIEW } from '../../utils/uiPreview'
import { previewImage } from '../../utils/imagePreview'
import { isWechatDevtools } from '../../utils/imagePreview'
import { resolveClassroomItems } from '../../utils/classroomCourseware'
import { resolveHomeNavigation } from '../../utils/homeNavigation'
import { resolveContentAsset } from '../../utils/contentAsset'

const LOCAL_TEACHER_PORTRAIT = '/static/teacher/hero-portrait.jpg'
const ORIGINAL_TEACHER_PORTRAIT = '/static/teacher/portrait.jpg'
const config = ref(getStoredSiteConfig() || {})
const videos = ref([])
const loading = ref(true)
const error = ref('')
const portraitFailed = ref(false)
const portraitPreviewVisible = ref(false)
const coverErrors = ref({})
const classroomEnabled = computed(() => normalizeMiniappLearn(config.value).classroom.enabled)
const BUNDLED_CLASSROOM_ITEMS = Array.isArray(studioVideos) ? studioVideos : []
let wechatNavigation = false
// #ifdef MP-WEIXIN
wechatNavigation = true
// #endif
const navigation = ref(resolveHomeNavigation(uni, { wechat: wechatNavigation }))
const statusBarHeight = computed(() => navigation.value.statusBarHeight)
const topbarStyle = computed(() => navigation.value.capsuleInset
  ? { paddingRight: `${navigation.value.capsuleInset}px` }
  : {})
function refreshNavigation() { navigation.value = resolveHomeNavigation(uni, { wechat: wechatNavigation }) }
onResize(refreshNavigation)
const teacher = computed(() => {
  const cfg = config.value
  const hasTeacher = [cfg.teacher, cfg.teachers, cfg.home?.teacher, cfg.home?.teachers, cfg.home?.teacherTeaser].some(value => value !== undefined)
  return hasTeacher ? normalizeTeachers(cfg)[0] || null : STUDIO_TEACHER
})
const isLaohan = computed(() => ['韩常青', '韩常青（老韩）', '韩常青(老韩)', '老韩'].includes(teacher.value?.name))
const portrait = computed(() => {
  const avatar = teacher.value?.avatar || ''
  if (isLaohan.value && (!avatar || /\/avatars\/|teacher-poster|\/static\/teacher\/portrait\.jpg/i.test(avatar))) return LOCAL_TEACHER_PORTRAIT
  return /\/avatars\//.test(avatar) ? '' : avatar
})
const portraitPreview = computed(() => {
  const avatar = teacher.value?.avatar || ''
  if (portraitFailed.value && isLaohan.value) return ORIGINAL_TEACHER_PORTRAIT
  if (isLaohan.value && (!avatar || /\/avatars\/|teacher-poster|\/static\/teacher\/portrait\.jpg/i.test(avatar))) return ORIGINAL_TEACHER_PORTRAIT
  return /\/avatars\//.test(avatar) ? '' : avatar
})
function homeShareCard() {
  return buildShareCard({ kind: 'home', title: teacher.value?.name ? `九型芯之力｜${teacher.value.name}的成长课堂` : '九型芯之力｜看见自己，理解彼此', imageUrl: portraitPreview.value || portrait.value })
}
onShow(async () => {
  showPublicShareMenu()
  try { config.value = await refreshSiteConfig() || {} } catch { /* Keep the last confirmed configuration. */ }
})
onShareAppMessage(() => homeShareCard().appMessage)
onShareTimeline(() => homeShareCard().timeline)
const teacherDisplayName = computed(() => (teacher.value?.name || '').replace(/[（(]老韩[）)]/, ''))
const courseRegistrationEnabled = computed(() => isCourseRegistrationEnabled(config.value))
const courses = computed(() => courseRegistrationEnabled.value ? (UI_PREVIEW ? STUDIO_COURSES : normalizeMiniappCourses(config.value)) : [])
const featuredCourse = computed(() => courses.value[0])
const dailyVideos = computed(() => videos.value.filter(item => item.itemType !== 'series').slice(0, 2))
let ticket = 0
async function load() {
  const current = ++ticket
  loading.value = true
  error.value = ''
  const results = await Promise.allSettled([refreshSiteConfig(), listClassroomRecentApi({ limit: 6 })])
  if (current !== ticket) return
  if (results[0].status === 'fulfilled') { config.value = getStoredSiteConfig() || results[0].value || {}; portraitFailed.value = false }
  if (results[1].status === 'fulfilled') {
    const remoteItems = Array.isArray(results[1].value?.items) ? results[1].value.items : []
    const resolved = resolveClassroomItems(remoteItems, BUNDLED_CLASSROOM_ITEMS, {
      allowFallback: isWechatDevtools(),
    })
    videos.value = resolved.items
    if (resolved.usedFallback) error.value = ''
  } else if (isWechatDevtools()) {
    const resolved = resolveClassroomItems([], BUNDLED_CLASSROOM_ITEMS, { allowFallback: true })
    videos.value = resolved.items
    error.value = resolved.usedFallback ? '' : '视频暂未更新，稍后可以再试一次'
  } else {
    error.value = '视频暂未更新，稍后可以再试一次'
  }
  loading.value = false
}
onMounted(() => {
  // Re-read after mount so the native value is applied before the first user
  // interaction (the initial render can run before the platform bridge exists).
  refreshNavigation()
  load()
})
function navigate(url) { if (!requireFullMiniapp()) return; uni.navigateTo({ url }) }
function daily() { if (!requireFullMiniapp()) return; uni.switchTab({ url: '/pages/learn/learn' }) }
function booking(kind = 'course') { if (!requireFullMiniapp()) return; setBookingIntent({ kind, intentText: '' }); uni.switchTab({ url: '/pages/booking/booking', fail: clearBookingIntent }) }
function openCourse() { if (featuredCourse.value) navigate(`/pages/course-detail/course-detail?id=${encodeURIComponent(featuredCourse.value.id)}`); else booking() }
function openVideo(item) { const url = classroomContentRoute(item); if (url) navigate(url) }
function duration(value) { return `${String(Math.floor((value || 0) / 60)).padStart(2, '0')}:${String((value || 0) % 60).padStart(2, '0')}` }
function previewPortrait() {
  if (isWechatDevtools()) {
    portraitPreviewVisible.value = true
    return
  }
  previewImage(portraitPreview.value || portrait.value)
}
function closePortraitPreview() { portraitPreviewVisible.value = false }
</script>

<template>
  <view class="studio-home" :style="{ paddingTop: `calc(${statusBarHeight}px + 144rpx)` }">
    <view class="studio-header" :style="{ paddingTop: `${statusBarHeight}px` }">
      <view class="studio-topbar" :style="topbarStyle">
        <view class="brand"><image class="brand-logo" src="/static/brand/logo.png" mode="aspectFit" aria-label="九型芯之力品牌 Logo" /><view class="brand-copy"><text class="brand-name">九型芯之力</text><text class="brand-sub">看见自己 · 理解彼此</text></view></view>
        <!-- #ifndef MP-WEIXIN -->
        <text v-if="UI_PREVIEW" class="preview-label">体验版</text>
        <button v-else class="top-about" aria-label="了解老师" @click="navigate('/pages/teacher/teacher')"><NxIcon name="user" :size="21" color="#282A27" /></button>
        <!-- #endif -->
      </view>
    </view>

    <view class="home-body">
      <view class="opening"><view class="opening-line" /><text>一段向内探索的旅程</text><text class="opening-en">A LITTLE CLOSER TO YOURSELF</text></view>
      <view v-if="teacher" class="mentor-hero">
        <image v-if="portrait && !portraitFailed" class="mentor-portrait" :src="portrait" mode="aspectFill" :aria-label="`${teacher.name}老师肖像`" @error="portraitFailed = true" />
        <image v-else-if="isLaohan" class="mentor-portrait" :src="LOCAL_TEACHER_PORTRAIT" mode="aspectFill" :aria-label="`${teacher.name}老师肖像`" />
        <view class="mentor-shade" />
        <button v-if="portrait && (isLaohan || !portraitFailed)" class="mentor-portrait-action" aria-label="预览老师头像" @click="previewPortrait" />
        <view class="mentor-content">
          <view class="mentor-label"><view class="mentor-dot" /><text>{{ teacher.title || '九型芯之力首席导师' }}</text></view>
          <text class="mentor-title">{{ '懂自己，\n也懂你。' }}</text>
          <text class="mentor-name">{{ teacherDisplayName }}<text v-if="isLaohan" class="mentor-nickname"> / 老韩</text></text>
          <text class="mentor-desc">{{ '把对性格的理解，\n带回真实的生活。' }}</text>
          <button class="mentor-button" @click="navigate('/pages/teacher/teacher')">认识老师<NxIcon name="arrow" :size="17" color="#FFFFFF" /></button>
        </view>
        <text class="mentor-bottom">ENNEAGRAM · LIFE & GROWTH</text>
      </view>
      <view v-else class="teacher-empty">老师介绍正在整理中</view>

      <view class="welcome-note"><text class="quote-sign">“</text><text>成长，从看见自己开始。<text class="welcome-muted">在这里，和老韩一起，慢慢读懂生活。</text></text></view>

      <view class="quick-links">
        <button v-if="classroomEnabled" class="quick-link" @click="daily"><view class="quick-icon"><NxIcon name="video" :size="23" /></view><text>老师日常</text><text class="quick-note">听一段分享</text></button>
        <button class="quick-link" @click="booking()"><view class="quick-icon"><NxIcon name="book" :size="23" /></view><text>{{ courseRegistrationEnabled ? '课程报名' : '报名咨询' }}</text><text class="quick-note">{{ courseRegistrationEnabled ? '走近一堂课' : '填写报名意向' }}</text></button>
        <button class="quick-link" @click="booking('consult')"><view class="quick-icon"><NxIcon name="message" :size="23" /></view><text>预约咨询</text><text class="quick-note">认真聊一聊</text></button>
        <button class="quick-link" @click="navigate('/pages/test/test')"><view class="quick-icon"><NxIcon name="spark" :size="23" /></view><text>认识自己</text><text class="quick-note">九型小探索</text></button>
        <button class="quick-link" @click="navigate('/pages/relation/relation')"><view class="quick-icon"><NxIcon name="relation" :size="23" /></view><text>关系合盘</text><text class="quick-note">读懂彼此</text></button>
      </view>

      <view v-if="classroomEnabled" class="home-section">
        <view class="section-head"><view><text class="section-kicker">MOMENTS WITH LAOHAN</text><text class="section-title">日常里的小小启发</text></view><button class="section-more" @click="daily">更多<NxIcon name="arrow" :size="17" color="#72746B" /></button></view>
        <view v-if="loading && !dailyVideos.length" class="video-loading" aria-live="polite">正在整理老师的分享…</view>
        <view v-else-if="error" class="quiet-state"><text>{{ error }}</text><button @click="load">重新加载</button></view>
        <view v-else-if="!dailyVideos.length" class="quiet-state"><text>新的分享正在路上，先来认识老师吧。</text></view>
        <view v-else class="video-grid">
          <button v-for="item in dailyVideos" :key="item.id" class="video-card" @click="openVideo(item)">
            <view class="video-cover"><image v-if="item.coverUrl && !coverErrors[item.id]" :src="resolveContentAsset(item.coverUrl)" mode="aspectFill" @error="coverErrors[item.id] = true" /><view v-else class="video-cover-fallback">老韩 · 日常</view><view class="video-scrim" /><view class="video-play"><NxIcon name="play" :size="15" color="#FFFFFF" /></view><text class="video-duration">{{ duration(item.durationSeconds) }}</text></view>
            <text class="video-title">{{ item.title }}</text><text class="video-author">老韩的分享 <text class="author-dot">·</text> {{ item.category || '日常短讲' }}</text>
          </button>
        </view>
      </view>

      <view v-if="courseRegistrationEnabled" class="home-section course-section">
        <view class="section-head"><view><text class="section-kicker">LEARN & GROW TOGETHER</text><text class="section-title">下一次，课堂见</text></view><button class="section-more" @click="booking()">全部<NxIcon name="arrow" :size="17" color="#72746B" /></button></view>
        <button v-if="featuredCourse" class="course-feature" @click="openCourse">
          <view class="course-image"><image :src="featuredCourse.cover || '/static/editorial/course-intro.webp'" mode="aspectFill" /><text class="course-image-label">{{ featuredCourse.tag || '成长课堂' }}</text></view>
          <view class="course-feature-body"><view class="course-tag-row"><text>{{ UI_PREVIEW ? featuredCourse.format : '韩常青老师主讲' }}</text><text v-if="UI_PREVIEW" class="sample-label">演示排期</text></view><text class="course-title">{{ featuredCourse.title }}</text><text class="course-desc">{{ featuredCourse.subtitle || featuredCourse.description }}</text><view class="course-bottom"><text class="course-schedule">{{ UI_PREVIEW ? `${featuredCourse.schedule} · ${featuredCourse.location}` : '了解课程内容与参与方式' }}</text><view class="course-arrow"><NxIcon name="arrow" :size="20" color="#FFFFFF" /></view></view></view>
        </button>
        <button v-else class="course-invitation" @click="booking()"><text>找到适合自己的成长方向</text><text>和老师聊聊课程安排 <text>→</text></text></button>
      </view>

      <button class="consult-invitation" @click="booking('consult')"><view class="consult-symbol"><NxIcon name="message" :size="27" /></view><view><text class="consult-title">有些心事，值得好好聊聊</text><text class="consult-copy">一对一沟通，从你的真实困惑开始</text></view><NxIcon name="arrow" :size="20" /></button>
      <NxShareActions />
      <view class="home-footer"><text>向内看见，向外生长</text><text class="home-footer-en">GROW AT YOUR OWN PACE</text></view>
    </view>
  </view>
  <NxImagePreview
    v-if="portraitPreview || portrait"
    :visible="portraitPreviewVisible"
    :src="portraitPreview || portrait"
    :alt="`${teacher?.name || '老师'}头像`"
    @close="closePortraitPreview"
  />
</template>

<style scoped>
.studio-home{min-height:100vh;background:#F7F5F0;color:#282A27;padding-top:calc(var(--status-bar-height, env(safe-area-inset-top, 0px)) + 144rpx);padding-bottom:32rpx;}
.studio-header{position:fixed;top:0;left:0;right:0;z-index:30;box-sizing:border-box;padding-top:var(--status-bar-height, env(safe-area-inset-top, 0px));background:#F7F5F0;}
.studio-topbar{box-sizing:border-box;width:100%;height:144rpx;max-width:900rpx;padding:24rpx 36rpx;display:flex;align-items:center;justify-content:space-between;margin:0 auto;}
.brand{min-width:0}.brand-copy{min-width:0}.brand-name,.brand-sub{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.brand{display:flex;align-items:center;gap:16rpx}.brand-logo{display:block;width:66rpx;height:66rpx;flex:none}.brand-name{display:block;font-size:31rpx;font-weight:650;letter-spacing:2rpx}.brand-sub{display:block;font-size:19rpx;color:#77786F;letter-spacing:3rpx;margin-top:3rpx}.preview-label{font-size:20rpx;color:#8B6A52;border:1rpx solid #E5D7C8;border-radius:30rpx;padding:6rpx 16rpx}.top-about{margin:0;width:88rpx;height:88rpx;display:flex;align-items:center;justify-content:center;background:transparent;padding:0}
.home-body{max-width:900rpx;margin:0 auto;padding:0 36rpx}.opening{display:flex;gap:12rpx;align-items:center;color:#77786F;font-size:20rpx;margin:6rpx 0 22rpx}.opening-line{height:1rpx;width:25rpx;background:#A55C3B}.opening-en{margin-left:auto;font-size:13rpx;letter-spacing:1rpx}
.mentor-hero{position:relative;height:610rpx;border-radius:26rpx;overflow:hidden;background:#151C24;color:#fff}.mentor-portrait{position:absolute;width:100%;height:100%;right:0;top:0}.mentor-shade{position:absolute;inset:0;z-index:0;background:linear-gradient(90deg,#151C24 0%,rgba(21,28,36,.95) 12%,rgba(21,28,36,.66) 41%,rgba(21,28,36,0) 70%),linear-gradient(0deg,rgba(10,15,20,.45),transparent 35%)}.mentor-portrait-action{position:absolute;inset:0 0 0 48%;z-index:1;width:auto;height:auto;margin:0;padding:0;border:0;border-radius:0;background:transparent}.mentor-content{position:relative;z-index:2;padding:40rpx 32rpx}.mentor-label{display:flex;gap:9rpx;align-items:center;font-size:19rpx;letter-spacing:1rpx;color:#E8D6BC}.mentor-dot{width:7rpx;height:7rpx;border-radius:50%;background:#D5AD85}.mentor-title{white-space:pre-line;display:block;font-family:"Songti SC","Noto Serif SC",STSong,serif;font-size:74rpx;line-height:1.34;letter-spacing:5rpx;margin:27rpx 0 18rpx}.mentor-name{font-size:29rpx;letter-spacing:3rpx}.mentor-nickname{font-size:21rpx;letter-spacing:2rpx;color:#D5D2CC}.mentor-desc{white-space:pre-line;display:block;font-size:23rpx;line-height:1.75;margin-top:16rpx;color:#E1DED6}.mentor-button{display:flex;align-items:center;justify-content:center;gap:20rpx;min-height:88rpx;width:215rpx;border-radius:10rpx;border:1rpx solid rgba(255,255,255,.45);background:rgba(255,255,255,.07);color:#fff;font-size:23rpx;margin:27rpx 0 0;padding:0 17rpx}.mentor-bottom{position:absolute;z-index:2;bottom:24rpx;right:26rpx;font-size:12rpx;letter-spacing:2rpx;color:#D0C3AD}
.welcome-note{display:flex;gap:17rpx;padding:28rpx 7rpx 27rpx;font-size:24rpx;line-height:1.8;align-items:flex-start}.quote-sign{font-family:Georgia,serif;font-size:61rpx;line-height:1;color:#B58165}.welcome-muted{display:block;font-size:21rpx;color:#72746B}.quick-links{display:flex;border-top:1rpx solid #E6E1D8;border-bottom:1rpx solid #E6E1D8;padding:28rpx 0 30rpx;gap:2rpx}.quick-link{flex:1;min-width:0;margin:0;padding:0;background:transparent;display:flex;align-items:center;flex-direction:column;font-size:24rpx;line-height:1.5;min-height:145rpx}.quick-icon{width:77rpx;height:77rpx;border-radius:24rpx;background:#EEE8DD;display:flex;align-items:center;justify-content:center;margin-bottom:13rpx}.quick-note{font-size:19rpx;color:#77786F;margin-top:4rpx}
.home-section{margin-top:44rpx}.section-head{display:flex;align-items:center;justify-content:space-between;margin-bottom:22rpx}.section-kicker{display:block;letter-spacing:2rpx;font-size:15rpx;color:#999081;margin-bottom:8rpx}.section-title{font-size:36rpx;font-weight:600;letter-spacing:1rpx;font-family:"Songti SC",STSong,serif}.section-more{display:flex;gap:8rpx;align-items:center;min-height:88rpx;font-size:22rpx;background:transparent;color:#72746B;margin:0;padding:0 0 0 16rpx}.video-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:22rpx}.video-card{text-align:left;margin:0;padding:0;background:transparent;border-radius:0;line-height:1.5}.video-cover{height:231rpx;position:relative;border-radius:17rpx;overflow:hidden;background:#E9E3D8}.video-cover image{width:100%;height:100%}.video-cover-fallback{display:flex;align-items:center;justify-content:center;height:100%;font-family:serif}.video-scrim{position:absolute;inset:50% 0 0;background:linear-gradient(transparent,rgba(0,0,0,.5))}.video-play{position:absolute;bottom:15rpx;left:17rpx;width:44rpx;height:44rpx;border-radius:50%;background:rgba(255,255,255,.2);display:flex;align-items:center;justify-content:center}.video-duration{position:absolute;bottom:17rpx;right:17rpx;color:#fff;font-size:19rpx;font-variant-numeric:tabular-nums}.video-title{display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;font-size:27rpx;margin-top:16rpx;line-height:1.55;min-height:84rpx;font-weight:550}.video-author{display:block;color:#77786F;font-size:19rpx;margin-top:8rpx}.author-dot{padding:0 5rpx;color:#BCA78F}.quiet-state,.video-loading{padding:35rpx 22rpx;background:#F0ECE5;border-radius:18rpx;color:#72746B;font-size:24rpx}.quiet-state button{font-size:24rpx;background:transparent;color:#A55C3B;min-height:88rpx}
.course-feature{display:block;text-align:left;padding:0;margin:0;background:#fff;border-radius:21rpx;overflow:hidden;line-height:1.5}.course-image{height:233rpx;position:relative}.course-image image{width:100%;height:100%}.course-image-label{position:absolute;left:24rpx;top:22rpx;padding:8rpx 17rpx;background:#F9F5ED;color:#665B49;border-radius:7rpx;font-size:20rpx}.course-feature-body{padding:23rpx 26rpx 25rpx}.course-tag-row{display:flex;justify-content:space-between;font-size:19rpx;color:#A55C3B;letter-spacing:1rpx}.sample-label{color:#858276;font-size:18rpx}.course-title{display:block;font-size:33rpx;font-weight:600;margin-top:9rpx}.course-desc{display:block;color:#72746B;font-size:23rpx;margin-top:7rpx}.course-bottom{display:flex;align-items:center;justify-content:space-between;margin-top:22rpx;padding-top:19rpx;border-top:1rpx solid #EEEAE2;gap:12rpx}.course-schedule{font-size:20rpx;color:#72746B}.course-arrow{width:54rpx;height:54rpx;border-radius:50%;background:#A55C3B;display:flex;align-items:center;justify-content:center;flex-shrink:0}.course-invitation{background:#EDE6DB;padding:28rpx;font-size:27rpx;text-align:left;line-height:2}.course-invitation text{display:block}
.consult-invitation{display:flex;align-items:center;gap:19rpx;padding:29rpx 23rpx;background:#EEE9DE;margin:32rpx 0 0;text-align:left;line-height:1.6;border-radius:20rpx}.consult-invitation>view:nth-child(2){flex:1}.consult-symbol{width:66rpx;height:70rpx;display:flex;align-items:center;justify-content:center}.consult-title{display:block;font-size:26rpx;font-weight:500}.consult-copy{display:block;font-size:20rpx;color:#72746B;margin-top:6rpx}.home-footer{display:flex;flex-direction:column;align-items:center;padding:43rpx 0 25rpx;color:#8B897D;font-family:"Songti SC",STSong,serif;font-size:24rpx;letter-spacing:4rpx}.home-footer-en{font-family:Arial,sans-serif;font-size:13rpx;letter-spacing:3rpx;color:#A9A498;margin-top:11rpx}.teacher-empty{padding:80rpx 30rpx;text-align:center;background:#EDE6DB;border-radius:24rpx;color:#72746B}
button::after{border:0}button:active{opacity:.76}button:focus-visible{outline:3px solid #A55C3B;outline-offset:3px}
@media(min-width:700px){.studio-topbar,.home-body{max-width:470px}.opening-en{font-size:9px}}
</style>
