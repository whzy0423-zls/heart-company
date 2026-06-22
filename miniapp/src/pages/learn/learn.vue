<script setup>
import { ref, onMounted } from 'vue'
import { TYPES_INFO } from '../../data/enneagramGame'
import { getSiteConfigApi } from '../../api'

const courses = ref([])
const quotes = ref([])
const types = ref(Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] })))
const loading = ref(true)

onMounted(async () => {
  try {
    const cfg = await getSiteConfigApi()
    courses.value = cfg?.home?.courses?.items || []
    quotes.value = cfg?.home?.quotes?.items || []
  } catch {
    courses.value = []
    quotes.value = []
  } finally {
    loading.value = false
  }
})

function goTest() {
  uni.switchTab({ url: '/pages/index/index' })
}
</script>

<template>
  <view class="wrap learn page-stack">
    <view class="card section">
      <text class="eyebrow">课程体系</text>
      <text class="sec-title">线上课程</text>
      <view v-if="!loading && courses.length === 0" class="empty">课程内容即将上线</view>
      <view v-for="(c, i) in courses" :key="i" class="course">
        <text class="chip course__badge">{{ c.badge || (i + 1) }}</text>
        <view class="course__body">
          <text class="course__title">{{ c.title }}</text>
          <text class="course__desc">{{ c.description }}</text>
        </view>
      </view>
    </view>

    <view class="card section">
      <text class="eyebrow">老韩语录</text>
      <text class="sec-title">语录互动区</text>
      <view v-if="!loading && quotes.length === 0" class="empty">语录内容即将上线</view>
      <view v-for="quote in quotes" :key="quote" class="quote-card">
        <text class="quote-card__text">“{{ quote }}”</text>
        <text class="quote-card__mark">”</text>
      </view>
    </view>

    <view class="card section">
      <text class="eyebrow">九型图鉴</text>
      <text class="sec-title">九种性格图鉴</text>
      <view class="types">
        <view v-for="t in types" :key="t.id" class="type" :class="'type--' + t.color">
          <image class="type__avatar" :src="`/static/avatars/${t.id}.png`" mode="aspectFill" />
          <text class="type__num">{{ t.id }}</text>
          <text class="type__name">{{ t.name }}</text>
          <text class="type__kw">{{ t.keywords }}</text>
        </view>
      </view>
    </view>

    <button class="btn-primary" @click="goTest">去测测我是哪一型 →</button>
  </view>
</template>

<style scoped>
.sec-title { font-size: 34rpx; font-weight: 900; display: block; margin: 16rpx 0 20rpx; }
.section { display: flex; flex-direction: column; }
.empty { color: #767d89; font-size: 26rpx; padding: 20rpx 0; }
.course { display: flex; gap: 18rpx; padding: 22rpx 0; border-bottom: 2rpx solid rgba(20,24,32,.07); }
.course:last-child { border-bottom: none; }
.course__badge { flex-shrink: 0; }
.course__title { color: #12151b; font-size: 30rpx; font-weight: 900; display: block; }
.course__desc { color: #3c424d; font-size: 25rpx; line-height: 1.65; display: block; margin-top: 6rpx; }
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
