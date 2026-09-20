<script setup>
import { computed } from 'vue'
import { TYPES_INFO, CENTERS } from '../../data/enneagramGame'

const currentPages = getCurrentPages()
const route = currentPages[currentPages.length - 1]
const typeId = computed(() => {
  const value = Number(route?.options?.type || 1)
  return TYPES_INFO[value] ? value : 1
})
const info = computed(() => TYPES_INFO[typeId.value])
const center = computed(() => CENTERS[info.value.center])
const growth = computed(() => TYPES_INFO[info.value.growth])
const stress = computed(() => TYPES_INFO[info.value.stress])

function openTest() { uni.navigateTo({ url: '/pages/test/test' }) }
function openRelation() { uni.navigateTo({ url: `/pages/relation/relation?type=${typeId.value}` }) }
function openOverview() { uni.navigateTo({ url: '/pages/enneagram/enneagram' }) }
</script>

<template>
  <view class="detail-page nx-page ios-page ios-safe-bottom">
    <view class="detail-shell">
      <button class="back-button" hover-class="back-button--pressed" @click="openOverview">‹ <text>返回九型地图</text></button>
      <view class="detail-hero" :class="`detail-hero--${info.color}`">
        <image class="detail-hero__image" :src="`/static/enneagram/${typeId}.webp`" mode="aspectFill" />
        <view class="detail-hero__copy">
          <text class="detail-hero__number">0{{ typeId }} TYPE</text>
          <text class="detail-hero__title">{{ info.name }}</text>
          <text class="detail-hero__en">{{ info.en }}</text>
          <text class="detail-hero__keywords">{{ info.keywords }}</text>
        </view>
      </view>

      <view class="content-card">
        <text class="card-eyebrow">核心动力</text>
        <text class="card-title">你最想成为怎样的人？</text>
        <view class="fact-row"><text class="fact-label">核心欲望</text><text class="fact-value">{{ info.desire }}</text></view>
        <view class="fact-row"><text class="fact-label">基本恐惧</text><text class="fact-value">{{ info.fear }}</text></view>
      </view>

      <view class="content-card">
        <text class="card-eyebrow">所属中心</text>
        <text class="card-title">{{ center.name }}</text>
        <text class="card-copy">{{ center.desc }}。{{ center.issue }}。</text>
      </view>

      <view class="direction-grid">
        <view class="direction-card direction-card--growth"><text class="direction-label">成长方向</text><text class="direction-title">{{ growth.name }}</text><text class="direction-copy">向 {{ info.growth }} 号学习：从熟悉模式走向更多弹性。</text></view>
        <view class="direction-card direction-card--stress"><text class="direction-label">压力提醒</text><text class="direction-title">{{ stress.name }}</text><text class="direction-copy">留意自己是否开始借用 {{ info.stress }} 号的防御方式。</text></view>
      </view>

      <view class="content-card">
        <text class="card-eyebrow">真实生活里的你</text>
        <text class="card-title">你的优势，也可能成为盲点</text>
        <text class="card-copy">{{ info.keywords }}是你天然的资源。当你过度依赖它时，记得停下来问自己：我是在主动选择，还是被旧习惯推着走？</text>
      </view>

      <view class="action-row"><button class="action action--primary" hover-class="action--pressed" @click="openTest">重新测一测</button><button class="action action--secondary" hover-class="action--pressed" @click="openRelation">看看关系</button></view>
    </view>
  </view>
</template>

<style scoped>
.detail-page { min-height: 100vh; background: var(--nx-mist-bg); color: #183439; }
.detail-shell { width: 100%; max-width: 900rpx; margin: 0 auto; padding: 18rpx 24rpx 52rpx; box-sizing: border-box; }
.back-button { min-height: 72rpx; margin: 0 0 8rpx; padding: 0; border: 0; background: transparent; color: var(--nx-mist-brand); font-size: 24rpx; font-weight: 800; line-height: 72rpx; text-align: left; }.back-button::after { border: 0; }.back-button--pressed { opacity: .68; }
.detail-hero { position: relative; display: flex; min-height: 330rpx; overflow: hidden; border-radius: 28rpx; background: #DCEFED; box-shadow: 0 18rpx 42rpx rgba(31,102,99,.13); }.detail-hero--green { background: #E8F0E6; }.detail-hero--red { background: #F8E8E2; }
.detail-hero__image { width: 280rpx; height: 330rpx; flex: 0 0 280rpx; align-self: flex-end; }.detail-hero__copy { min-width: 0; display: flex; flex: 1; flex-direction: column; align-items: flex-start; justify-content: center; padding: 26rpx 22rpx; box-sizing: border-box; }.detail-hero__number { color: var(--nx-mist-brand); font-size: 21rpx; font-weight: 900; letter-spacing: 3rpx; }.detail-hero__title { margin-top: 16rpx; color: #183439; font-size: 46rpx; font-weight: 900; line-height: 1.2; }.detail-hero__en { margin-top: 8rpx; color: rgba(24,52,57,.62); font-size: 21rpx; line-height: 1.4; }.detail-hero__keywords { margin-top: 18rpx; padding: 8rpx 14rpx; border-radius: 999rpx; background: rgba(255,255,255,.72); color: #39716E; font-size: 21rpx; font-weight: 800; line-height: 1.4; }
.content-card { margin-top: 18rpx; padding: 24rpx; border: 2rpx solid var(--nx-mist-border); border-radius: 22rpx; background: var(--nx-mist-surface); }.card-eyebrow, .direction-label { display: block; color: #467D7B; font-size: 21rpx; font-weight: 900; letter-spacing: 2rpx; }.card-title { display: block; margin-top: 10rpx; color: #183439; font-size: 30rpx; font-weight: 900; line-height: 1.4; }.card-copy { display: block; margin-top: 10rpx; color: rgba(24,52,57,.7); font-size: 24rpx; line-height: 1.7; }.fact-row { display: flex; gap: 16rpx; margin-top: 18rpx; padding-top: 18rpx; border-top: 2rpx solid #EDF3F2; }.fact-label { flex: 0 0 118rpx; color: var(--nx-mist-brand); font-size: 23rpx; font-weight: 900; }.fact-value { min-width: 0; color: rgba(24,52,57,.72); font-size: 23rpx; line-height: 1.55; }
.direction-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 16rpx; }.direction-card { margin-top: 18rpx; padding: 22rpx; border-radius: 22rpx; }.direction-card--growth { background: #E1EFEC; }.direction-card--stress { background: #F7E9D8; }.direction-title { display: block; margin-top: 12rpx; color: #183439; font-size: 28rpx; font-weight: 900; }.direction-copy { display: block; margin-top: 8rpx; color: rgba(24,52,57,.68); font-size: 21rpx; line-height: 1.6; }.action-row { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 14rpx; margin-top: 22rpx; }.action { min-height: 88rpx; border-radius: 16rpx; font-size: 25rpx; font-weight: 900; line-height: 88rpx; }.action::after { border: 0; }.action--primary { background: var(--nx-mist-brand); color: #FFF; }.action--secondary { border: 2rpx solid var(--nx-mist-brand); background: transparent; color: var(--nx-mist-brand); }.action--pressed { opacity: .76; transform: translateY(2rpx); }
@media screen and (max-width: 600rpx) { .detail-hero__image { width: 230rpx; flex-basis: 230rpx; }.detail-hero__title { font-size: 40rpx; }.direction-grid { grid-template-columns: 1fr; } }
@media (prefers-reduced-motion: reduce) { .action--pressed { transform: none; } }
</style>
