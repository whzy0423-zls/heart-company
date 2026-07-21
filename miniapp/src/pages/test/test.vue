<script setup>
import { ref, computed } from 'vue'
import { onUnload } from '@dcloudio/uni-app'
import { QUESTIONS } from '../../data/enneagramGame'
import { calcType } from '../../utils/enneagram'
import { setLastResult } from '../../utils/session'
import { questionVisualCenter } from '../../utils/questionVisuals'
import { reportGameResultApi } from '../../api'

const stage = ref('gender') // gender | quiz
const gender = ref(null)
const step = ref(0)
const answers = ref([])
const answerLocked = ref(false)
let advanceTimer = null

const q = computed(() => QUESTIONS[step.value])
const progress = computed(() => ((step.value + (answers.value[step.value] ? 1 : 0)) / QUESTIONS.length) * 100)
const visualCenter = computed(() => questionVisualCenter(step.value))
const questionVisualSrc = computed(() => `/static/editorial/center-${visualCenter.value}.webp`)

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
    <!-- 选性别 -->
    <view v-if="stage === 'gender'" class="gender nx-editorial-hero">
      <text class="gender__eyebrow">开始之前 · 约 3 分钟</text>
      <text class="gender__title">先选择你的性别</text>
      <text class="gender__tip">用于微调同分情况下的决胜权重，让画像更贴近你。</text>
      <view class="gender__row">
        <button
          class="gender__card gender__card--m"
          aria-label="选择男生"
          hover-class="gender__card--hover"
          @click="start('male')"
        >
          <text class="gender__mark">M</text>
          <text class="gender__b">男生</text>
          <text class="gender__d">更偏行动、边界与掌控感</text>
        </button>
        <button
          class="gender__card gender__card--f"
          aria-label="选择女生"
          hover-class="gender__card--hover"
          @click="start('female')"
        >
          <text class="gender__mark">F</text>
          <text class="gender__b">女生</text>
          <text class="gender__d">更偏关系、细腻与安全感</text>
        </button>
      </view>
    </view>

    <!-- 答题 -->
    <view v-else class="quiz nx-panel">
      <view class="quiz__progress-copy">
        <text class="quiz__progress-label">测试进度</text>
        <text class="quiz__progress-count">进度 {{ step + 1 }} / {{ QUESTIONS.length }}</text>
      </view>
      <view
        class="quiz__bar"
        role="progressbar"
        aria-valuemin="0"
        :aria-valuemax="QUESTIONS.length"
        :aria-valuenow="step + (answers[step] ? 1 : 0)"
        :aria-valuetext="'已完成 ' + (step + (answers[step] ? 1 : 0)) + ' / ' + QUESTIONS.length + ' 题'"
      >
        <view class="quiz__bar-fill" :style="{ width: progress + '%' }" />
      </view>

      <view class="quiz__visual" aria-hidden="true">
        <image
          class="quiz__illustration"
          :src="questionVisualSrc"
          mode="aspectFill"
          :alt="'第 ' + (step + 1) + ' 题抽象插画'"
        />
      </view>

      <view class="quiz__question-block">
        <text class="quiz__number">第 {{ step + 1 }} 题</text>
        <text class="quiz__q">{{ q.q }}</text>
      </view>

      <view class="quiz__options">
        <button
          v-for="(opt, k) in q.options"
          :key="k"
          class="quiz__opt"
          :class="{
            'quiz__opt--selected': answers[step] === opt,
            'quiz__opt--locked': answerLocked,
          }"
          :disabled="answerLocked"
          :aria-pressed="answers[step] === opt"
          :aria-label="'选择答案 ' + letter(k) + '：' + opt.t"
          hover-class="quiz__opt--hover"
          @click="choose(opt)"
        >
          <text class="quiz__idx">{{ letter(k) }}</text>
          <text class="quiz__t">{{ opt.t }}</text>
        </button>
      </view>

      <view class="quiz__footer">
        <button
          class="nx-button--text quiz__back"
          v-if="step > 0"
          aria-label="返回上一题"
          hover-class="quiz__back--hover"
          @click="back"
        >
          ← 上一题
        </button>
      </view>
    </view>
  </view>
</template>

<style scoped>
.gender {
  min-height: 680rpx;
}
.gender__eyebrow {
  color: var(--nx-coral);
  font-size: 24rpx;
  font-weight: 800;
  letter-spacing: 3rpx;
}
.gender__title {
  color: var(--nx-ink);
  font-size: 52rpx;
  font-weight: 900;
  line-height: 1.2;
}
.gender__tip {
  max-width: 620rpx;
  color: var(--nx-muted);
  font-size: 27rpx;
  line-height: 1.65;
}
.gender__row {
  display: flex;
  gap: var(--nx-space-3);
  margin-top: var(--nx-space-2);
}
.gender__card {
  flex: 1;
  min-height: 300rpx;
  margin: 0;
  border: 2rpx solid var(--nx-line);
  border-radius: var(--nx-radius-md);
  background: var(--nx-surface);
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 14rpx;
  padding: 28rpx;
  box-shadow: var(--nx-shadow-sm);
  box-sizing: border-box;
  line-height: 1.2;
  text-align: left;
  touch-action: manipulation;
}
.gender__card::after { border: none; }
.gender__card--hover { opacity: .86; transform: scale(.985); }
.gender__card--m {
  border-top: 8rpx solid var(--nx-blue);
}
.gender__card--f {
  border-top: 8rpx solid var(--nx-coral);
}
.gender__mark {
  width: 76rpx;
  height: 76rpx;
  border-radius: 24rpx;
  color: #FFFFFF;
  font-weight: 900;
  font-size: 34rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}
.gender__card--m .gender__mark {
  background: var(--nx-blue);
}
.gender__card--f .gender__mark {
  background: var(--nx-coral);
}
.gender__b {
  color: var(--nx-ink);
  font-size: 32rpx;
  font-weight: 900;
}
.gender__d {
  color: var(--nx-muted);
  font-size: 22rpx;
  line-height: 1.45;
}

.quiz {
  width: 100%;
  max-width: 720rpx;
  margin: 0 auto;
  padding: 36rpx;
  box-sizing: border-box;
}
.quiz__progress-copy {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--nx-space-2);
  margin-bottom: 16rpx;
}
.quiz__progress-label {
  color: var(--nx-ink);
  font-size: 24rpx;
  font-weight: 800;
}
.quiz__progress-count {
  color: var(--nx-muted);
  font-size: 24rpx;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.quiz__bar {
  height: 16rpx;
  background: var(--nx-line);
  border-radius: var(--nx-radius-pill);
  overflow: hidden;
}
.quiz__bar-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--nx-coral);
  transition: width .24s ease;
}
.quiz__visual {
  width: 100%;
  aspect-ratio: 3 / 2;
  margin-top: 32rpx;
  overflow: hidden;
  border-radius: var(--nx-radius-md);
  background: var(--nx-bg);
}
.quiz__illustration {
  display: block;
  width: 100%;
  height: 100%;
}
.quiz__question-block {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  margin-top: 36rpx;
}
.quiz__number {
  color: var(--nx-coral);
  font-size: 24rpx;
  font-weight: 800;
  letter-spacing: 2rpx;
}
.quiz__q {
  color: var(--nx-ink);
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.38;
}
.quiz__options {
  display: flex;
  flex-direction: column;
  gap: 20rpx;
  margin-top: 36rpx;
}
.quiz__opt {
  display: flex;
  align-items: center;
  gap: 20rpx;
  width: 100%;
  min-height: 104rpx;
  margin: 0;
  border: 2rpx solid var(--nx-line);
  border-radius: var(--nx-radius-sm);
  background: var(--nx-surface);
  padding: 24rpx;
  box-sizing: border-box;
  text-align: left;
  line-height: 1.2;
  touch-action: manipulation;
  transition: border-color .16s ease, background-color .16s ease, box-shadow .16s ease, transform .16s ease;
}
.quiz__opt::after { border: none; }
.quiz__opt[disabled] {
  pointer-events: none;
  opacity: .5;
}
.quiz__opt--hover { transform: translateY(2rpx) scale(.992); }
.quiz__opt--selected,
.quiz__opt--selected[disabled] {
  border-color: var(--nx-blue);
  background: var(--nx-primary-soft);
  box-shadow: 0 0 0 4rpx rgba(49, 91, 234, .12);
  opacity: 1;
}
.quiz__idx {
  width: 56rpx;
  height: 56rpx;
  flex-shrink: 0;
  border: 2rpx solid var(--nx-line);
  border-radius: var(--nx-radius-sm);
  background: var(--nx-bg);
  color: var(--nx-ink);
  font-weight: 900;
  font-size: 25rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}
.quiz__opt--selected .quiz__idx {
  border-color: var(--nx-blue);
  background: var(--nx-blue);
  color: #FFFFFF;
}
.quiz__t {
  flex: 1;
  color: var(--nx-ink);
  font-size: 29rpx;
  font-weight: 600;
  line-height: 1.5;
}
.quiz__footer {
  display: flex;
  justify-content: flex-start;
  min-height: 88rpx;
  margin-top: 20rpx;
}
.quiz__back {
  min-width: 144rpx;
  min-height: 88rpx;
  margin: 0;
  padding: 0 12rpx;
  color: var(--nx-muted);
  font-size: 25rpx;
  font-weight: 700;
}
.quiz__back::after { border: none; }
.quiz__back--hover { color: var(--nx-blue); opacity: .78; }
.quiz__opt:focus-visible,
.quiz__back:focus-visible {
  outline: 4rpx solid var(--nx-focus);
  outline-offset: 4rpx;
}

@media (max-width: 420px) {
  .gender__row {
    flex-direction: column;
  }
  .gender__card {
    min-height: 220rpx;
  }
  .quiz {
    padding: 28rpx 24rpx;
  }
  .quiz__q {
    font-size: 38rpx;
  }
}

@media (min-width: 760px) {
  .quiz {
    padding: 48rpx;
  }
}
</style>
