<script setup>
import { ref, computed, nextTick } from 'vue'
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
const questionHeading = ref(null)
let advanceTimer = null

const q = computed(() => QUESTIONS[step.value])
const progress = computed(() => ((step.value + 1) / QUESTIONS.length) * 100)
const currentVisualCenter = computed(() => questionVisualCenter(step.value))
const questionVisualSrc = computed(() => `/static/editorial/center-${currentVisualCenter.value}.webp`)

function start(g) {
  clearAdvanceTimer()
  gender.value = g
  stage.value = 'quiz'
  step.value = 0
  answers.value = []
  answerLocked.value = false
  focusQuestionHeading()
}

function clearAdvanceTimer() {
  if (!advanceTimer) return
  clearTimeout(advanceTimer)
  advanceTimer = null
}

function focusQuestionHeading() {
  nextTick(() => {
    // #ifdef H5
    const heading = questionHeading.value?.$el || questionHeading.value
    heading?.focus?.()
    // #endif
  })
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
      focusQuestionHeading()
    }, 160)
  } else {
    clearAdvanceTimer()
    advanceTimer = setTimeout(() => {
      advanceTimer = null
      finish()
    }, 220)
  }
}

function back() {
  clearAdvanceTimer()
  answerLocked.value = false
  if (step.value > 0) {
    step.value -= 1
    focusQuestionHeading()
  }
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
    <view v-else class="quiz nx-panel" :class="'quiz--' + currentVisualCenter">
      <view class="quiz__progress-copy">
        <text class="quiz__progress-label">测试进度</text>
        <text class="quiz__progress-count">进度 {{ step + 1 }} / {{ QUESTIONS.length }}</text>
      </view>
      <view
        class="quiz__bar"
        role="progressbar"
        aria-valuemin="0"
        :aria-valuemax="QUESTIONS.length"
        :aria-valuenow="step + 1"
        :aria-valuetext="'当前第 ' + (step + 1) + ' / ' + QUESTIONS.length + ' 题'"
      >
        <view class="quiz__bar-fill" :style="{ width: progress + '%' }" />
      </view>

      <view class="quiz__body">
        <view :key="'visual-' + step" class="quiz__media-column">
          <view class="quiz__visual" aria-hidden="true">
            <image
              class="quiz__illustration"
              :src="questionVisualSrc"
              mode="aspectFill"
              :alt="'第 ' + (step + 1) + ' 题抽象插画'"
            />
          </view>
        </view>

        <view class="quiz__content-column">
          <view :key="'question-' + step" class="quiz__question-block">
            <text class="quiz__number">第 {{ step + 1 }} 题</text>
            <text
              ref="questionHeading"
              class="quiz__q"
              tabindex="-1"
              aria-live="polite"
              aria-atomic="true"
            >
              {{ q.q }}
            </text>
          </view>

          <view :key="'options-' + step" class="quiz__options">
            <button
              v-for="(opt, k) in q.options"
              :key="step + '-' + k"
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
              <view class="quiz__opt-accent" aria-hidden="true" />
              <text class="quiz__idx">{{ letter(k) }}</text>
              <text class="quiz__t">{{ opt.t }}</text>
              <text class="quiz__check" aria-hidden="true">✓</text>
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
  --quiz-accent: var(--nx-blue);
  --quiz-selected-bg: #E4E9FC;
  --quiz-atmosphere: rgba(49, 91, 234, .10);
  width: 100%;
  max-width: 720rpx;
  margin: 0 auto;
  padding: 36rpx;
  overflow: hidden;
  box-sizing: border-box;
  background: linear-gradient(145deg, var(--quiz-atmosphere) 0, rgba(255, 253, 248, .96) 34%, #FFFDF8 100%);
}
.quiz--head {
  --quiz-accent: #315BEA;
  --quiz-selected-bg: #E4E9FC;
  --quiz-atmosphere: rgba(49, 91, 234, .12);
}
.quiz--heart {
  --quiz-accent: #C9472D;
  --quiz-selected-bg: #F5DDD6;
  --quiz-atmosphere: rgba(201, 71, 45, .12);
}
.quiz--gut {
  --quiz-accent: #347B62;
  --quiz-selected-bg: #DDECE6;
  --quiz-atmosphere: rgba(52, 123, 98, .13);
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
  background: var(--quiz-accent);
  transition: width .24s ease;
}
.quiz__body {
  min-width: 0;
}
.quiz__media-column {
  animation: quiz-enter .22s ease-out backwards;
}
.quiz__visual {
  width: 100%;
  max-width: 260rpx;
  aspect-ratio: 3 / 2;
  margin: 24rpx auto 0;
  overflow: hidden;
  border-radius: var(--nx-radius-md);
  border: 2rpx solid rgba(23, 33, 43, .10);
  background: var(--quiz-selected-bg);
  box-shadow: 0 14rpx 32rpx rgba(23, 33, 43, .10);
  box-sizing: border-box;
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
  animation: quiz-enter .22s ease-out backwards;
}
.quiz__number {
  color: var(--quiz-accent);
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
.quiz__q:focus-visible {
  outline: 4rpx solid var(--nx-focus);
  outline-offset: 6rpx;
}
.quiz__options {
  display: flex;
  flex-direction: column;
  gap: 20rpx;
  margin-top: 36rpx;
  animation: quiz-enter .24s ease-out .04s backwards;
}
.quiz__opt {
  position: relative;
  display: flex;
  align-items: center;
  gap: 20rpx;
  width: 100%;
  min-height: 104rpx;
  margin: 0;
  border: 2rpx solid var(--nx-line);
  border-radius: var(--nx-radius-sm);
  background: var(--nx-surface);
  padding: 24rpx 24rpx 24rpx 30rpx;
  overflow: hidden;
  box-sizing: border-box;
  text-align: left;
  line-height: 1.2;
  touch-action: manipulation;
  transition: opacity .22s ease, transform .22s ease, border-color .22s ease, background-color .22s ease, box-shadow .22s ease;
}
.quiz__opt::after { border: none; }
.quiz__opt[disabled] {
  pointer-events: none;
  opacity: .5;
}
.quiz__opt--hover { opacity: .84; transform: translateY(2rpx) scale(.992); }
.quiz__opt--selected,
.quiz__opt--selected[disabled] {
  border-color: var(--quiz-accent);
  background: var(--quiz-selected-bg);
  box-shadow: inset 0 0 0 2rpx var(--quiz-accent), 0 8rpx 22rpx rgba(23, 33, 43, .12);
  opacity: 1;
}
.quiz__opt-accent {
  position: absolute;
  top: 18rpx;
  bottom: 18rpx;
  left: 0;
  width: 8rpx;
  border-radius: 0 8rpx 8rpx 0;
  background: var(--quiz-accent);
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
  border-color: var(--quiz-accent);
  background: var(--quiz-accent);
  color: #FFFFFF;
}
.quiz__t {
  flex: 1;
  color: var(--nx-ink);
  font-size: 29rpx;
  font-weight: 600;
  line-height: 1.5;
}
.quiz__check {
  width: 44rpx;
  height: 44rpx;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--quiz-accent);
  color: #FFFFFF;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24rpx;
  font-weight: 900;
  opacity: 0;
  transform: scale(.72);
  transition: opacity .22s ease, transform .22s ease;
}
.quiz__opt--selected .quiz__check {
  opacity: 1;
  transform: scale(1);
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

@keyframes quiz-enter {
  from {
    opacity: 0;
    transform: translateY(12rpx);
  }
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

@media screen and (min-width: 768px) {
  .test.wrap {
    max-width: 1400rpx;
    padding-left: clamp(32rpx, 4vw, 56rpx);
    padding-right: clamp(32rpx, 4vw, 56rpx);
  }
  .gender {
    width: 100%;
    max-width: 900rpx;
    margin: 0 auto;
    box-sizing: border-box;
  }
  .quiz {
    max-width: 1280rpx;
    padding: 48rpx;
  }
  .quiz__body {
    display: grid;
    grid-template-columns: minmax(0, .58fr) minmax(0, 1.42fr);
    align-items: start;
    gap: 40rpx;
  }
  .quiz__media-column {
    min-width: 0;
  }
  .quiz__visual {
    max-width: none;
    margin-top: 38rpx;
  }
  .quiz__content-column {
    min-width: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .quiz__media-column,
  .quiz__question-block,
  .quiz__options {
    animation: none;
  }
  .quiz__bar-fill,
  .quiz__opt,
  .quiz__check {
    transition: none;
  }
}
</style>
