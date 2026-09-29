<script setup>
import { computed, onMounted, ref } from 'vue'
import NxIcon from '../../components/NxIcon.vue'
import { STUDIO_TEACHER } from '../../data/teacherStudio'
import { getStoredSiteConfig, refreshSiteConfig } from '../../utils/siteConfig'
import { normalizeTeachers } from '../../utils/teacherCourseware'
import { normalizeMiniappLearn } from '../../utils/miniappPages'
import { setBookingIntent, clearBookingIntent } from '../../utils/bookingIntent'
import { previewImage } from '../../utils/imagePreview'

const config = ref(getStoredSiteConfig() || {})
const imageFailed = ref(false)
const mentorPhoto = '/static/teacher/mentor.jpg'
const teacher = computed(() => {
  const cfg = config.value
  const explicit = [cfg.teacher, cfg.teachers, cfg.home?.teacher, cfg.home?.teachers, cfg.home?.teacherTeaser].some(value => value !== undefined)
  return explicit ? normalizeTeachers(cfg)[0] || null : STUDIO_TEACHER
})
const isLaohan = computed(() => ['韩常青', '韩常青（老韩）', '韩常青(老韩)', '老韩'].includes(teacher.value?.name))
const classroomEnabled = computed(() => normalizeMiniappLearn(config.value).classroom.enabled)
const portrait = computed(() => {
  const avatar = teacher.value?.avatar || ''
  if (isLaohan.value && (!avatar || /\/avatars\/|teacher-poster/i.test(avatar))) return STUDIO_TEACHER.avatar
  if (/teacher-poster/i.test(avatar)) return STUDIO_TEACHER.avatar
  return /\/avatars\//.test(avatar) ? '' : avatar
})
onMounted(async () => { try { const updated = await refreshSiteConfig(); config.value = getStoredSiteConfig() || updated || {}; imageFailed.value = false } catch { /* Retain verified local or cached introduction. */ } })
function book() { setBookingIntent({ kind: 'consult', intentText: '' }); uni.switchTab({ url: '/pages/booking/booking', fail: clearBookingIntent }) }
function daily() { uni.switchTab({ url: '/pages/learn/learn' }) }
function preview() { previewImage(portrait.value) }
function previewMentorPhoto() { previewImage(mentorPhoto) }
</script>
<template>
  <view class="teacher-page">
    <template v-if="teacher">
      <view class="teacher-intro">
        <view class="teacher-intro-copy"><text class="eyebrow">MEET YOUR MENTOR</text><text class="teacher-name">{{ teacher.name }}</text><text v-if="isLaohan" class="teacher-alias">你也可以，叫我老韩。</text><text class="teacher-role">{{ teacher.title }}</text><view class="signature-line" /></view>
        <button class="portrait-button" aria-label="点击查看老师大图" hover-class="portrait-button--pressed" @click="preview"><image v-if="portrait && !imageFailed" class="portrait-image" :src="portrait" mode="aspectFill" @error="imageFailed = true" /><text v-else class="portrait-placeholder">{{ teacher.name.slice(0,1) }}</text><text v-if="portrait && !imageFailed" class="portrait-hint">点击查看大图</text></button>
      </view>
      <view class="intro-statement"><text class="quote">“</text><text>{{ '看懂自己，\n是理解这个世界的开始。' }}</text></view>
      <view class="section about"><text class="eyebrow">ABOUT THE TEACHER</text><text class="section-title">从认识性格，到看见一个人</text><text class="body-copy">{{ teacher.bio }}</text><view class="tags"><text v-for="tag in teacher.tags" :key="tag">{{ tag }}</text></view></view>
      <view class="section approach"><text class="eyebrow">WAYS WE CAN GROW</text><text class="section-title">把理解，带回你的生活</text><view class="approach-row"><view class="approach-icon"><NxIcon name="spark" :size="25" /></view><view><text class="row-title">个人成长</text><text class="row-copy">理解自己的反应模式，练习更有觉察地选择。</text></view></view><view class="approach-row"><view class="approach-icon"><NxIcon name="heart" :size="25" /></view><view><text class="row-title">家庭与关系</text><text class="row-copy">在亲子、伴侣的相处里，学会倾听与表达。</text></view></view><view class="approach-row"><view class="approach-icon"><NxIcon name="grid" :size="25" /></view><view><text class="row-title">团队与协作</text><text class="row-copy">看见彼此动机，让沟通成为团队的共同语言。</text></view></view></view>
      <view v-if="isLaohan" class="section journey"><text class="eyebrow">A LIFELONG PRACTICE</text><text class="section-title">学习，是一条一直走的路</text><view class="timeline"><view v-for="item in STUDIO_TEACHER.timeline" :key="item.year" class="timeline-item"><text class="year">{{ item.year }}</text><view class="timeline-copy"><text class="row-title">{{ item.title }}</text><text class="row-copy">{{ item.text }}</text></view></view></view><image class="mentor-photo" :src="mentorPhoto" mode="widthFix" aria-label="韩常青与陈伟志博士合影" @click="previewMentorPhoto" /><text class="photo-caption">与恩师陈伟志博士合影 · 学习与传承</text></view>
      <button v-if="classroomEnabled" class="daily-link" @click="daily"><view><text class="row-title">先从一段日常分享认识我</text><text class="row-copy">关于性格、情绪，还有生活中的小事</text></view><NxIcon name="arrow" :size="22" /></button>
      <text class="source-note">老师介绍整理自九型芯之力已有官网资料</text>
      <view class="teacher-cta"><view><text class="cta-title">给成长，留一点时间</text><text class="cta-note">从一次真诚的交流开始</text></view><button @click="book">预约聊聊<NxIcon name="arrow" :size="18" color="#FFFFFF" /></button></view>
    </template>
    <view v-else class="empty">老师介绍正在整理中，欢迎先看看日常分享。<button @click="daily">浏览日常</button></view>
  </view>
</template>
<style scoped>
.teacher-page{max-width:900rpx;margin:0 auto;min-height:100vh;padding:24rpx 36rpx calc(180rpx + env(safe-area-inset-bottom));background:#F7F5F0;color:#282A27}.teacher-intro{display:flex;align-items:flex-start;box-sizing:border-box;min-height:386rpx;border-bottom:1rpx solid #E6E1D8;padding-bottom:35rpx;gap:24rpx}.teacher-intro-copy{flex:1;min-width:0}.eyebrow{display:block;font-size:16rpx;letter-spacing:2rpx;color:#9A8268;margin-bottom:17rpx}.teacher-name{display:block;font-family:"Songti SC",STSong,serif;font-size:60rpx;line-height:1.25;letter-spacing:4rpx}.teacher-alias{display:block;font-size:24rpx;margin-top:17rpx;color:#72746B}.teacher-role{display:block;font-size:21rpx;color:#A55C3B;margin-top:30rpx}.signature-line{width:53rpx;height:3rpx;background:#A55C3B;margin-top:18rpx}.portrait-button{position:relative;display:flex;align-items:stretch;justify-content:center;box-sizing:border-box;width:246rpx;height:350rpx;min-height:350rpx;margin:0;padding:0;overflow:hidden;border-radius:120rpx 120rpx 14rpx 14rpx;background:#1B2025;flex-shrink:0;line-height:0;font-size:0}.portrait-button--pressed{opacity:.82}.portrait-image{display:block;flex:0 0 246rpx;width:246rpx;min-width:246rpx;height:350rpx;min-height:350rpx}.portrait-placeholder{display:flex;align-items:center;justify-content:center;width:100%;height:100%;font-size:90rpx;line-height:1;color:#D5AD85}.portrait-hint{position:absolute;right:12rpx;bottom:12rpx;padding:7rpx 10rpx;border-radius:8rpx;background:rgba(0,0,0,.55);color:#fff;font-size:17rpx;line-height:1.2}.intro-statement{white-space:pre-line;padding:36rpx 0 28rpx;display:flex;gap:20rpx;line-height:1.65;font-family:"Songti SC",STSong,serif;font-size:39rpx}.quote{font-family:Georgia,serif;font-size:80rpx;line-height:1;color:#BD8B6F}.section{margin-top:40rpx}.section-title{display:block;font-family:"Songti SC",STSong,serif;font-size:35rpx;line-height:1.45;font-weight:600}.body-copy{display:block;font-size:27rpx;line-height:1.95;color:#64665D;margin-top:25rpx}.tags{display:flex;gap:13rpx;flex-wrap:wrap;margin-top:24rpx}.tags text{padding:9rpx 20rpx;background:#EEE8DD;color:#806449;border-radius:8rpx;font-size:22rpx}.approach{background:#fff;border-radius:24rpx;padding:30rpx 28rpx}.approach-row{display:flex;align-items:center;gap:22rpx;padding:28rpx 0;border-bottom:1rpx solid #EDE9E2}.approach-row:last-child{border:0;padding-bottom:0}.approach-icon{width:76rpx;height:76rpx;display:flex;align-items:center;justify-content:center;flex-shrink:0;border-radius:22rpx;background:#F3EEE6}.row-title{display:block;font-size:28rpx;font-weight:550}.row-copy{display:block;font-size:24rpx;color:#72746B;line-height:1.8;margin-top:7rpx}.timeline{margin-top:28rpx}.timeline-item{display:flex;gap:25rpx;padding:0 0 35rpx}.year{width:85rpx;flex-shrink:0;font-size:30rpx;font-family:Georgia,serif;color:#A55C3B;line-height:1.6}.timeline-copy{border-left:1rpx solid #D9C8B6;padding-left:25rpx;flex:1}.mentor-photo{display:block;width:100%;border-radius:19rpx;margin-top:5rpx}.photo-caption{display:block;text-align:center;font-size:20rpx;margin-top:15rpx;color:#7D7F75}.daily-link{display:flex;align-items:center;justify-content:space-between;gap:20rpx;background:#EEE8DD;margin:35rpx 0 0;padding:29rpx 24rpx;text-align:left;border-radius:20rpx;line-height:1.5}.source-note{display:block;text-align:center;color:#85877D;font-size:19rpx;margin:29rpx 0}.teacher-cta{position:fixed;bottom:0;left:0;right:0;background:#fff;border-top:1rpx solid #E6E1D8;padding:22rpx 36rpx calc(22rpx + env(safe-area-inset-bottom));display:flex;justify-content:space-between;align-items:center;gap:20rpx;z-index:20}.cta-title{display:block;font-size:26rpx}.cta-note{display:block;font-size:21rpx;color:#72746B;margin-top:3rpx}.teacher-cta button{background:#A55C3B;color:#fff;font-size:27rpx;min-height:88rpx;margin:0;display:flex;align-items:center;gap:18rpx;padding:0 28rpx;border-radius:12rpx}.empty{padding:80rpx 10rpx;text-align:center;font-size:28rpx}.empty button{margin-top:30rpx;background:#A55C3B;color:white}button::after{border:0}button:active{opacity:.78}button:focus-visible{outline:3px solid #A55C3B;outline-offset:3px}@media(min-width:700px){.teacher-page{max-width:470px}.teacher-cta{max-width:470px;margin:0 auto}}
</style>
