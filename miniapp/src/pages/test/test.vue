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
      <view class="test-hero nx-page-hero">
        <text class="test-hero__eyebrow">自我探索 · 从这里开始</text>
        <text class="test-hero__title">遇见更真实的自己</text>
        <text class="test-hero__desc">先告诉我们你的性别，它只用于微调同分时的判断，让最终画像更贴近你。</text>
      </view>

      <view class="gender__intro">
        <text class="gender__title">选择你的身份</text>
        <text class="gender__tip">选择后将进入正式答题，全程大约需要 3 分钟。</text>
      </view>
      <view class="gender__row">
        <button
          class="gender__card gender__card--m nx-focusable"
          aria-label="选择男生"
          hover-class="gender__card--hover"
          @click="start('male')"
        >
          <text class="gender__mark">M</text>
          <text class="gender__b">男生</text>
          <text class="gender__d">关注行动方式与边界感</text>
          <text class="gender__go">开始探索 →</text>
        </button>
        <button
          class="gender__card gender__card--f nx-focusable"
          aria-label="选择女生"
          hover-class="gender__card--hover"
          @click="start('female')"
        >
          <text class="gender__mark">F</text>
          <text class="gender__b">女生</text>
          <text class="gender__d">关注关系方式与安全感</text>
          <text class="gender__go">开始探索 →</text>
        </button>
      </view>
    </view>

    <view v-else class="quiz-shell">
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
    radial-gradient(circle at 8% 8%, rgba(59, 130, 246, .15), transparent 34%),
    radial-gradient(circle at 94% 20%, rgba(109, 40, 217, .14), transparent 32%),
    #f5f7ff;
}
.gender {
  display: flex;
  flex-direction: column;
  gap: 34rpx;
}
.test-hero {
  position: relative;
  overflow: hidden;
  min-height: 330rpx;
  padding: 48rpx 40rpx;
  border-radius: 38rpx;
  background: linear-gradient(135deg, #1d4ed8 0%, #4338ca 48%, #6d28d9 100%);
  box-shadow: 0 28rpx 70rpx -34rpx rgba(67, 56, 202, .74);
  box-sizing: border-box;
}
.test-hero::after {
  content: '';
  position: absolute;
  right: -94rpx;
  bottom: -150rpx;
  width: 330rpx;
  height: 330rpx;
  border: 2rpx solid rgba(255, 255, 255, .18);
  border-radius: 50%;
  box-shadow:
    0 0 0 46rpx rgba(255, 255, 255, .055),
    0 0 0 92rpx rgba(255, 255, 255, .035);
}
.test-hero__eyebrow,
.test-hero__title,
.test-hero__desc {
  position: relative;
  z-index: 1;
  display: block;
}
.test-hero__eyebrow {
  color: rgba(255, 255, 255, .92);
  font-size: 23rpx;
  font-weight: 700;
  letter-spacing: 3rpx;
}
.test-hero__title {
  margin-top: 24rpx;
  color: #fff;
  font-size: 52rpx;
  font-weight: 900;
  line-height: 1.2;
  letter-spacing: -1rpx;
}
.test-hero__desc {
  max-width: 560rpx;
  margin-top: 20rpx;
  color: rgba(255, 255, 255, .92);
  font-size: 27rpx;
  line-height: 1.68;
}
.gender__intro {
  display: flex;
  flex-direction: column;
  gap: 10rpx;
  padding: 0 8rpx;
}
.gender__title {
  color: #0f172a;
  font-size: 38rpx;
  font-weight: 900;
  line-height: 1.3;
}
.gender__tip {
  color: #52627a;
  font-size: 25rpx;
  line-height: 1.6;
}
.gender__row {
  display: flex;
  gap: 20rpx;
}
.gender__card {
  flex: 1;
  min-height: 260rpx;
  margin: 0;
  border: 0;
  border-radius: 32rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: flex-start;
  gap: 12rpx;
  padding: 30rpx 28rpx 28rpx;
  box-sizing: border-box;
  line-height: 1.2;
  text-align: left;
  touch-action: manipulation;
  transition: transform .2s ease, opacity .2s ease, box-shadow .2s ease;
}
.gender__card::after { border: none; }
.gender__card--hover { opacity: .9; transform: scale(.98); }
.gender__card--m {
  background: linear-gradient(145deg, #155e75, #1d4ed8);
  box-shadow: 0 24rpx 52rpx -34rpx rgba(29, 78, 216, .82);
}
.gender__card--f {
  background: linear-gradient(145deg, #7e22ce, #be185d);
  box-shadow: 0 24rpx 52rpx -34rpx rgba(190, 24, 93, .74);
}
.gender__mark {
  width: 70rpx;
  height: 70rpx;
  border-radius: 22rpx;
  background: rgba(255, 255, 255, .16);
  color: #fff;
  font-weight: 900;
  font-size: 38rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2rpx solid rgba(255, 255, 255, .2);
}
.gender__b {
  margin-top: 4rpx;
  color: #fff;
  font-size: 32rpx;
  font-weight: 900;
}
.gender__d {
  color: rgba(255, 255, 255, .86);
  font-size: 24rpx;
  line-height: 1.5;
}
.gender__go {
  margin-top: auto;
  color: #fff;
  font-size: 24rpx;
  font-weight: 800;
}

.quiz-shell {
  position: relative;
  padding: 38rpx 32rpx 34rpx;
  border: 2rpx solid rgba(99, 102, 241, .11);
  border-radius: 36rpx;
  background: rgba(255, 255, 255, .94);
  box-shadow: 0 28rpx 80rpx -48rpx rgba(49, 46, 129, .52);
  box-sizing: border-box;
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
  color: #3730a3;
  font-size: 28rpx;
  font-weight: 900;
}
.quiz__total,
.quiz__percent {
  color: #64748b;
  font-size: 23rpx;
  font-weight: 700;
}
.quiz__bar {
  height: 14rpx;
  background: #e9eaf8;
  border-radius: 999rpx;
  overflow: hidden;
}
.quiz__bar-fill {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #2563eb, #6d28d9);
  transition: width .3s ease;
}
.quiz__question-block {
  display: flex;
  flex-direction: column;
  gap: 16rpx;
  margin-top: 46rpx;
}
.quiz__eyebrow {
  color: #64748b;
  font-size: 23rpx;
  font-weight: 700;
}
.quiz__back {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-height: 88rpx;
  margin: 28rpx 0 0;
  color: #4338ca;
  padding: 0 24rpx;
  border: 2rpx solid #d8dcf4;
  border-radius: 22rpx;
  background: #f8f9ff;
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.2;
  touch-action: manipulation;
  transition: transform .2s ease, opacity .2s ease;
}
.quiz__back::after { border: none; }
.quiz__back--hover { opacity: .82; transform: scale(.985); }
.quiz__q {
  color: #0f172a;
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
  display: flex;
  align-items: center;
  gap: 18rpx;
  width: 100%;
  min-height: 112rpx;
  margin: 0;
  background: #f8faff;
  border: 2rpx solid #dce1f0;
  border-radius: 24rpx;
  padding: 24rpx;
  box-shadow: 0 12rpx 30rpx -28rpx rgba(30, 41, 59, .55);
  box-sizing: border-box;
  text-align: left;
  line-height: 1.2;
  touch-action: manipulation;
  transition: transform .2s ease, opacity .2s ease, border-color .2s ease, box-shadow .2s ease;
}
.quiz__opt::after { border: none; }
.quiz__opt.disabled,
.quiz__opt[disabled] { pointer-events: none; opacity: .72; }
.quiz__opt--hover { opacity: .86; transform: scale(.992); }
.quiz__opt.on {
  border: 4rpx solid #4f46e5;
  background: linear-gradient(120deg, #eef2ff, #f5f3ff);
  box-shadow:
    inset 0 0 0 4rpx rgba(255, 255, 255, .9),
    inset 0 0 0 8rpx rgba(79, 70, 229, .16),
    0 18rpx 36rpx -30rpx rgba(79, 70, 229, .7);
}
.quiz__idx {
  width: 58rpx;
  height: 58rpx;
  flex-shrink: 0;
  border-radius: 18rpx;
  background: #e4e8f8;
  color: #3730a3;
  font-weight: 900;
  font-size: 25rpx;
  display: flex;
  align-items: center;
  justify-content: center;
}
.quiz__opt.on .quiz__idx {
  background: linear-gradient(135deg, #2563eb, #6d28d9);
  color: #fff;
}
.quiz__t {
  flex: 1;
  color: #334155;
  font-size: 28rpx;
  line-height: 1.55;
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
