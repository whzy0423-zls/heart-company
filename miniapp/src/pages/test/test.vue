<script setup>
import { ref, computed } from 'vue'
import { QUESTIONS } from '../../data/enneagramGame'
import { calcType } from '../../utils/enneagram'
import { setLastResult } from '../../utils/session'
import { reportGameResultApi } from '../../api'

const stage = ref('gender') // gender | quiz
const gender = ref(null)
const step = ref(0)
const answers = ref([])

const q = computed(() => QUESTIONS[step.value])
const progress = computed(() => ((step.value + (answers.value[step.value] ? 1 : 0)) / QUESTIONS.length) * 100)

function start(g) {
  gender.value = g
  stage.value = 'quiz'
  step.value = 0
  answers.value = []
}

function choose(opt) {
  answers.value[step.value] = opt
  if (step.value < QUESTIONS.length - 1) {
    setTimeout(() => { step.value += 1 }, 160)
  } else {
    finish()
  }
}

function back() {
  if (step.value > 0) step.value -= 1
}

function letter(k) {
  return String.fromCharCode(65 + k)
}

function finish() {
  const result = calcType(answers.value, gender.value)
  setLastResult(result, gender.value)
  // 匿名统计上报（不阻塞）
  reportGameResultApi({
    gender: gender.value,
    resultType: result.type,
    secondType: result.second || 0,
    score: result.score,
    centers: result.centers,
  }).catch(() => {})
  uni.redirectTo({ url: '/pages/result/result' })
}
</script>

<template>
  <view class="wrap page-stack">
    <!-- 选性别 -->
    <view v-if="stage === 'gender'" class="gender card">
      <text class="eyebrow">开始之前</text>
      <text class="gender__title gradient-title">先选择你的性别</text>
      <text class="gender__tip">用于微调同分情况下的决胜权重，让画像更贴近你。</text>
      <view class="gender__row">
        <view class="gender__card gender__card--m" @click="start('male')">
          <text class="gender__mark">M</text>
          <text class="gender__b">男生</text>
          <text class="gender__d">更偏行动、边界与掌控感</text>
        </view>
        <view class="gender__card gender__card--f" @click="start('female')">
          <text class="gender__mark">F</text>
          <text class="gender__b">女生</text>
          <text class="gender__d">更偏关系、细腻与安全感</text>
        </view>
      </view>
    </view>

    <!-- 答题 -->
    <view v-else class="quiz card">
      <view class="quiz__bar"><view class="quiz__bar-fill" :style="{ width: progress + '%' }" /></view>
      <view class="quiz__head">
        <text>第 {{ step + 1 }} / {{ QUESTIONS.length }} 题</text>
        <text v-if="step > 0" class="quiz__back" @click="back">上一题</text>
      </view>
      <text class="quiz__q">{{ q.q }}</text>
      <view class="quiz__options">
        <view
          v-for="(opt, k) in q.options"
          :key="k"
          class="quiz__opt"
          :class="{ on: answers[step] === opt }"
          @click="choose(opt)"
        >
          <text class="quiz__idx">{{ letter(k) }}</text>
          <text class="quiz__t">{{ opt.t }}</text>
        </view>
      </view>
    </view>
  </view>
</template>

<style scoped>
.gender {
  min-height: 680rpx;
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  padding: 48rpx 34rpx;
}
.gender__title {
  font-size: 52rpx;
}
.gender__tip {
  color: #3c424d;
  font-size: 27rpx;
  line-height: 1.7;
}
.gender__row {
  display: flex;
  gap: 20rpx;
  margin-top: 34rpx;
}
.gender__card {
  flex: 1;
  min-height: 300rpx;
  border-radius: 28rpx;
  background: rgba(255,255,255,.68);
  border: 2rpx solid rgba(255,255,255,.92);
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 14rpx;
  padding: 28rpx;
  box-shadow: 0 18rpx 42rpx -30rpx rgba(28,40,70,.46);
  box-sizing: border-box;
}
.gender__card--m {
  background: linear-gradient(145deg, rgba(90,160,255,.22), rgba(255,255,255,.72));
}
.gender__card--f {
  background: linear-gradient(145deg, rgba(255,90,106,.18), rgba(255,255,255,.72));
}
.gender__mark {
  width: 76rpx;
  height: 76rpx;
  border-radius: 24rpx;
  color: #fff;
  font-weight: 900;
  font-size: 34rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}
.gender__card--m .gender__mark {
  background: linear-gradient(135deg, #5aa0ff, #2b7fff);
}
.gender__card--f .gender__mark {
  background: linear-gradient(135deg, #ff5a6a, #e23a47);
}
.gender__b {
  color: #12151b;
  font-size: 32rpx;
  font-weight: 900;
}
.gender__d {
  color: #767d89;
  font-size: 22rpx;
  line-height: 1.45;
}

.quiz {
  padding: 36rpx 30rpx;
}
.quiz__bar {
  height: 12rpx;
  background: rgba(20,24,32,.08);
  border-radius: 999rpx;
  overflow: hidden;
}
.quiz__bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #25b365, #2b7fff 52%, #e23a47);
  transition: width .3s;
}
.quiz__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: #767d89;
  font-size: 24rpx;
  font-weight: 700;
  margin: 28rpx 0 22rpx;
}
.quiz__back {
  color: #2b7fff;
  padding: 8rpx 16rpx;
  border-radius: 999rpx;
  background: rgba(43,127,255,.1);
}
.quiz__q {
  color: #12151b;
  font-size: 39rpx;
  font-weight: 900;
  line-height: 1.45;
}
.quiz__options {
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  margin-top: 34rpx;
}
.quiz__opt {
  display: flex;
  align-items: center;
  gap: 18rpx;
  background: rgba(255,255,255,.72);
  border: 2rpx solid rgba(20,24,32,.08);
  border-radius: 24rpx;
  padding: 28rpx 24rpx;
  box-shadow: 0 10rpx 30rpx -28rpx rgba(28,40,70,.45);
}
.quiz__opt.on {
  border-color: rgba(43,127,255,.46);
  background: linear-gradient(120deg, rgba(43,127,255,.12), rgba(37,179,101,.08));
}
.quiz__idx {
  width: 52rpx;
  height: 52rpx;
  flex-shrink: 0;
  border-radius: 17rpx;
  background: linear-gradient(135deg, #5aa0ff, #2b7fff);
  color: #fff;
  font-weight: 900;
  font-size: 25rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}
.quiz__t {
  flex: 1;
  color: #3c424d;
  font-size: 29rpx;
  line-height: 1.55;
}
</style>
