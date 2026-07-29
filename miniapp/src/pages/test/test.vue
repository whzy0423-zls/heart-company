<script setup>
import { ref, computed } from 'vue'
import { onUnload } from '@dcloudio/uni-app'
import { QUESTIONS } from '../../data/enneagramGame'
import { calcType } from '../../utils/enneagram'
import { setLastResult } from '../../utils/session'
import { reportGameResultApi } from '../../api'

const stage = ref('gender') // gender | quiz
const gender = ref(null)
const step = ref(0)
const answers = ref([])
const answerLocked = ref(false)
let advanceTimer = null

const total = QUESTIONS.length
const q = computed(() => QUESTIONS[step.value])
const progress = computed(() => ((step.value + (answers.value[step.value] ? 1 : 0)) / QUESTIONS.length) * 100)

function start(g) {
  clearAdvanceTimer()
  gender.value = g
  stage.value = 'quiz'
  step.value = 0
  answers.value = []
  answerLocked.value = false
}

function clearAdvanceTimer() {
  if (!advanceTimer) return
  clearTimeout(advanceTimer)
  advanceTimer = null
}

function choose(opt) {
  if (answerLocked.value) return
  answerLocked.value = true
  answers.value[step.value] = opt
  if (step.value < QUESTIONS.length - 1) {
    clearAdvanceTimer()
    advanceTimer = setTimeout(() => {
      step.value += 1
      answerLocked.value = false
      advanceTimer = null
    }, 160)
  } else {
    finish()
  }
}

function back() {
  clearAdvanceTimer()
  answerLocked.value = false
  if (step.value > 0) step.value -= 1
}

function letter(k) {
  return String.fromCharCode(65 + k)
}

function finish() {
  clearAdvanceTimer()
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

onUnload(() => {
  clearAdvanceTimer()
})
</script>

<template>
  <view class="wrap test page-stack ios-page ios-safe-bottom">
    <view v-if="stage === 'gender'" class="gender">
      <view class="test-hero nx-card">
        <text class="test-hero__eyebrow">九型测试小游戏</text>
        <text class="test-hero__title">用 18 道生活情境题看见你的核心动机</text>
        <text class="test-hero__desc">约 3 分钟完成。性别只用于同分时微调判断，答题时选择更接近真实反应的一项。</text>
        <view class="test-hero__meta" aria-label="测试说明">
          <text>18 道生活情境题</text>
          <text>约 3 分钟</text>
          <text>趣味自测</text>
        </view>
      </view>

      <view class="gender__intro">
        <text class="gender__title">先选择答题身份</text>
        <text class="gender__tip">选择后进入正式答题，系统会保留原有计分与结果逻辑。</text>
      </view>
      <view class="gender__row">
        <button
          class="gender__card nx-focusable"
          aria-label="选择男生并开始九型测试小游戏"
          hover-class="gender__card--hover"
          @click="start('male')"
        >
          <text class="gender__mark">M</text>
          <text class="gender__b">男生</text>
          <text class="gender__d">进入 18 道生活情境题，选择更像你的真实反应。</text>
          <text class="gender__go">开始测试</text>
        </button>
        <button
          class="gender__card nx-focusable"
          aria-label="选择女生并开始九型测试小游戏"
          hover-class="gender__card--hover"
          @click="start('female')"
        >
          <text class="gender__mark">F</text>
          <text class="gender__b">女生</text>
          <text class="gender__d">进入 18 道生活情境题，选择更像你的真实反应。</text>
          <text class="gender__go">开始测试</text>
        </button>
      </view>
    </view>

    <view v-else class="quiz-shell nx-card">
      <view class="quiz__topline">
        <view
          class="quiz__progress-meta"
          :aria-label="`第 ${step + 1} 题，共 ${total} 题`"
        >
          <text class="quiz__step">第 {{ step + 1 }} 题</text>
          <text class="quiz__total">/ 共 {{ total }} 题</text>
        </view>
        <text class="quiz__percent">{{ Math.round(progress) }}%</text>
      </view>

      <view class="quiz__bar" aria-hidden="true">
        <view class="quiz__bar-fill" :style="{ width: progress + '%' }" />
      </view>

      <view class="quiz__question-block">
        <text class="quiz__eyebrow">选择最符合你真实反应的一项</text>
        <text class="quiz__q">{{ q.q }}</text>
      </view>

      <view class="quiz__options">
        <button
          v-for="(opt, k) in q.options"
          :key="k"
          class="quiz__opt nx-focusable"
          :class="{ on: answers[step] === opt, disabled: answerLocked }"
          :disabled="answerLocked"
          :aria-label="'选择答案 ' + letter(k) + '：' + opt.t"
          hover-class="quiz__opt--hover"
          @click="choose(opt)"
        >
          <text class="quiz__idx">{{ letter(k) }}</text>
          <text class="quiz__t">{{ opt.t }}</text>
        </button>
      </view>

      <button
        v-if="step > 0"
        class="quiz__back nx-focusable"
        aria-label="返回上一题"
        hover-class="quiz__back--hover"
        @click="back"
      >
        ← 上一题
      </button>
    </view>
  </view>
</template>

<style scoped>
.test {
  position: relative;
  overflow: hidden;
  background:
    linear-gradient(180deg, var(--nx-surface-soft), var(--nx-page-bg));
  color: var(--nx-text);
}

button {
  box-sizing: border-box;
  margin: 0;
}

button::after {
  border: 0;
}

.gender {
  display: flex;
  flex-direction: column;
  gap: 34rpx;
}

.test-hero {
  position: relative;
  overflow: hidden;
  min-height: 360rpx;
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  padding: 48rpx 40rpx;
  border: 2rpx solid var(--test-gold-border);
  border-radius: 38rpx;
  background: linear-gradient(135deg, var(--nx-brand-900), var(--nx-brand-700));
  box-shadow: 0 28rpx 70rpx -38rpx var(--test-brand-shadow);
}

.test-hero::after {
  position: absolute;
  right: -94rpx;
  bottom: -150rpx;
  width: 330rpx;
  height: 330rpx;
  border: 2rpx solid var(--test-gold-border);
  border-radius: 50%;
  content: '';
}

.test-hero__eyebrow,
.test-hero__title,
.test-hero__desc,
.test-hero__meta {
  position: relative;
  z-index: 1;
  display: block;
}

.test-hero__eyebrow {
  color: var(--nx-accent-gold);
  font-size: 23rpx;
  font-weight: 900;
  letter-spacing: 3rpx;
}

.test-hero__title {
  color: var(--nx-surface);
  font-size: 50rpx;
  font-weight: 900;
  line-height: 1.2;
  letter-spacing: -1rpx;
}

.test-hero__desc {
  max-width: 580rpx;
  color: var(--test-on-brand-muted);
  font-size: 26rpx;
  line-height: 1.68;
}

.test-hero__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12rpx;
}

.test-hero__meta text {
  padding: 10rpx 16rpx;
  border: 2rpx solid var(--test-gold-border);
  border-radius: 999rpx;
  color: var(--nx-surface);
  font-size: 22rpx;
  font-weight: 800;
}

.gender__intro {
  display: flex;
  flex-direction: column;
  gap: 10rpx;
  padding: 0 8rpx;
}

.gender__title {
  color: var(--nx-brand-900);
  font-size: 38rpx;
  font-weight: 900;
  line-height: 1.3;
}

.gender__tip {
  color: var(--nx-text-muted);
  font-size: 25rpx;
  line-height: 1.6;
}

.gender__row {
  display: flex;
  gap: 20rpx;
}

.gender__card {
  flex: 1;
  min-height: 286rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 12rpx;
  padding: 30rpx 28rpx 28rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 32rpx;
  background: var(--nx-surface);
  line-height: 1.2;
  text-align: left;
  touch-action: manipulation;
  transition: transform 180ms ease-out, opacity 180ms ease-out, border-color 180ms ease-out;
}

.gender__card--hover {
  opacity: .86;
  transform: scale(.985);
}

.gender__mark {
  width: 70rpx;
  height: 70rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2rpx solid var(--test-gold-border);
  border-radius: 22rpx;
  background: var(--test-gold-soft);
  color: var(--nx-brand-900);
  font-size: 34rpx;
  font-weight: 900;
}

.gender__b {
  margin-top: 4rpx;
  color: var(--nx-brand-900);
  font-size: 32rpx;
  font-weight: 900;
}

.gender__d {
  color: var(--nx-text-muted);
  font-size: 24rpx;
  line-height: 1.5;
}

.gender__go {
  margin-top: auto;
  color: var(--nx-brand-700);
  font-size: 24rpx;
  font-weight: 900;
}

.quiz-shell {
  position: relative;
  padding: 38rpx 32rpx 34rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 36rpx;
  background: var(--nx-surface);
  box-shadow: 0 28rpx 80rpx -54rpx var(--test-brand-shadow);
}

.quiz__topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
  margin-bottom: 18rpx;
}

.quiz__progress-meta {
  display: flex;
  align-items: baseline;
  gap: 8rpx;
}

.quiz__step {
  color: var(--nx-brand-700);
  font-size: 28rpx;
  font-weight: 900;
}

.quiz__total,
.quiz__percent {
  color: var(--nx-text-muted);
  font-size: 23rpx;
  font-weight: 700;
}

.quiz__bar {
  height: 14rpx;
  overflow: hidden;
  border-radius: 999rpx;
  background: var(--nx-border);
}

.quiz__bar-fill {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, var(--nx-accent-gold), var(--nx-brand-700));
  transition: width 300ms ease-out;
}

.quiz__question-block {
  display: flex;
  flex-direction: column;
  gap: 16rpx;
  margin-top: 46rpx;
}

.quiz__eyebrow {
  color: var(--nx-text-muted);
  font-size: 23rpx;
  font-weight: 700;
}

.quiz__q {
  color: var(--nx-brand-900);
  font-size: 40rpx;
  font-weight: 900;
  line-height: 1.48;
}

.quiz__options {
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  margin-top: 38rpx;
}

.quiz__opt {
  width: 100%;
  min-height: 112rpx;
  display: flex;
  align-items: center;
  gap: 18rpx;
  padding: 24rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 24rpx;
  background: var(--nx-surface-soft);
  line-height: 1.2;
  text-align: left;
  touch-action: manipulation;
  transition: transform 180ms ease-out, opacity 180ms ease-out, border-color 180ms ease-out;
}

.quiz__opt.disabled,
.quiz__opt[disabled] {
  pointer-events: none;
  opacity: .72;
}

.quiz__opt--hover {
  opacity: .86;
  transform: scale(.992);
}

.quiz__opt.on {
  border: 4rpx solid var(--nx-accent-gold);
  background: linear-gradient(120deg, var(--test-gold-soft), var(--nx-surface));
}

.quiz__idx {
  width: 58rpx;
  height: 58rpx;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 18rpx;
  background: var(--nx-surface);
  color: var(--nx-brand-700);
  font-size: 25rpx;
  font-weight: 900;
}

.quiz__opt.on .quiz__idx {
  background: var(--nx-brand-900);
  color: var(--nx-accent-gold);
}

.quiz__t {
  flex: 1;
  color: var(--nx-text);
  font-size: 28rpx;
  line-height: 1.55;
}

.quiz__back {
  width: 100%;
  min-height: 88rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 28rpx 0 0;
  padding: 0 24rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 999rpx;
  background: var(--nx-surface);
  color: var(--nx-brand-700);
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.2;
  touch-action: manipulation;
  transition: transform 180ms ease-out, opacity 180ms ease-out;
}

.quiz__back--hover {
  opacity: .82;
  transform: scale(.985);
}

@media (max-width: 360px) {
  .quiz__q {
    font-size: 36rpx;
  }
}

@media (prefers-reduced-motion: reduce) {
  .gender__card,
  .quiz__bar-fill,
  .quiz__opt,
  .quiz__back {
    transition: none;
  }
}
</style>
