<script setup>
import { computed } from 'vue'
import { TYPES_INFO } from '../../data/enneagramGame'

const types = computed(() => Object.entries(TYPES_INFO).map(([id, info]) => ({ id: Number(id), ...info })))

function openType(id) {
  uni.navigateTo({ url: `/pages/enneagram-detail/enneagram-detail?type=${id}` })
}
</script>

<template>
  <view class="enneagram-page nx-page ios-page ios-safe-bottom">
    <view class="enneagram-shell">
      <view class="intro-card">
        <text class="eyebrow">YOUR INNER MAP</text>
        <text class="intro-title">认识九型人格</text>
        <text class="intro-copy">九型人格不是给你贴标签，而是一张帮助你看见内在动力、理解关系模式的地图。</text>
        <view class="intro-note"><text class="intro-note__mark">9</text><text>九种视角，没有高低之分，只有不同的关注点。</text></view>
      </view>

      <view class="section-heading">
        <text class="section-title">九型人格地图</text>
        <text class="section-note">点击任意型号，查看完整介绍</text>
      </view>
      <view class="type-grid">
        <button v-for="type in types" :key="type.id" class="type-card" :class="`type-card--${type.color}`" hover-class="type-card--pressed" @click="openType(type.id)">
          <image class="type-card__image" :src="`/static/enneagram/${type.id}.webp`" mode="aspectFill" lazy-load />
          <view class="type-card__body">
            <text class="type-card__number">0{{ type.id }}</text>
            <text class="type-card__name">{{ type.name }}</text>
            <text class="type-card__keywords">{{ type.keywords }}</text>
          </view>
          <text class="type-card__arrow" aria-hidden="true">›</text>
        </button>
      </view>

      <button class="start-button" hover-class="start-button--pressed" @click="uni.navigateTo({ url: '/pages/test/test' })">还不确定？先做一次测试</button>
    </view>
  </view>
</template>

<style scoped>
.enneagram-page { min-height: 100vh; background: var(--nx-mist-bg); color: #183439; }
.enneagram-shell { width: 100%; max-width: 960rpx; margin: 0 auto; padding: 26rpx 24rpx 48rpx; box-sizing: border-box; }
.intro-card { position: relative; overflow: hidden; padding: 30rpx; border-radius: 28rpx; background: linear-gradient(145deg, var(--nx-mist-brand-soft), #EEF6F4); box-shadow: 0 18rpx 42rpx rgba(31,102,99,.12); }
.intro-card::after { content: '九'; position: absolute; right: 18rpx; top: -30rpx; color: rgba(242,189,90,.32); font-size: 180rpx; font-weight: 900; line-height: 1; }
.eyebrow { display: block; color: #467D7B; font-size: 21rpx; font-weight: 900; letter-spacing: 4rpx; }
.intro-title { display: block; margin-top: 14rpx; color: #183439; font-size: 48rpx; font-weight: 900; line-height: 1.2; }
.intro-copy { display: block; max-width: 630rpx; margin-top: 12rpx; color: rgba(24,52,57,.7); font-size: 25rpx; line-height: 1.65; }
.intro-note { position: relative; z-index: 1; display: flex; align-items: center; gap: 14rpx; margin-top: 24rpx; color: #39716E; font-size: 22rpx; font-weight: 700; }
.intro-note__mark { display: flex; align-items: center; justify-content: center; width: 44rpx; height: 44rpx; border-radius: 14rpx; background: var(--nx-mist-gold); color: #203735; font-size: 23rpx; font-weight: 900; }
.section-heading { display: flex; align-items: end; justify-content: space-between; margin: 34rpx 4rpx 16rpx; }
.section-title { color: #183439; font-size: 32rpx; font-weight: 900; }
.section-note { color: rgba(24,52,57,.58); font-size: 22rpx; }
.type-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16rpx; }
.type-card { position: relative; min-width: 0; min-height: 228rpx; display: flex; align-items: stretch; padding: 0; overflow: hidden; border: 2rpx solid var(--nx-mist-border); border-radius: 22rpx; background: var(--nx-mist-surface); color: #183439; text-align: left; box-shadow: 0 8rpx 24rpx rgba(35,75,75,.06); }
.type-card::after, .start-button::after { border: 0; }
.type-card__image { width: 150rpx; height: 228rpx; flex: 0 0 150rpx; background: var(--nx-mist-soft); }
.type-card__body { min-width: 0; display: flex; flex: 1; flex-direction: column; align-items: flex-start; padding: 20rpx 16rpx 18rpx; box-sizing: border-box; }
.type-card__number { color: var(--nx-mist-brand); font-size: 22rpx; font-weight: 900; letter-spacing: 2rpx; }
.type-card__name { margin-top: 12rpx; color: #183439; font-size: 29rpx; font-weight: 900; line-height: 1.3; }
.type-card__keywords { margin-top: 9rpx; color: rgba(24,52,57,.62); font-size: 21rpx; line-height: 1.5; }
.type-card__arrow { position: absolute; right: 14rpx; bottom: 12rpx; color: var(--nx-mist-brand); font-size: 36rpx; line-height: 1; }
.type-card--blue { border-top: 6rpx solid #7FB7B0; }.type-card--green { border-top: 6rpx solid #9DBB98; }.type-card--red { border-top: 6rpx solid #E69B82; }
.type-card--pressed, .start-button--pressed { opacity: .78; transform: translateY(2rpx); }
.start-button { width: 100%; min-height: 88rpx; margin-top: 28rpx; border-radius: 16rpx; background: var(--nx-mist-brand); color: #FFF; font-size: 26rpx; font-weight: 900; line-height: 88rpx; }
@media screen and (min-width: 720px) { .type-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (prefers-reduced-motion: reduce) { .type-card--pressed, .start-button--pressed { transform: none; } }
</style>
