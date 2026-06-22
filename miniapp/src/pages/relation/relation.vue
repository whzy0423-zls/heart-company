<script setup>
import { ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'
import { TYPES_INFO, CENTERS } from '../../data/enneagramGame'

const myType = ref(0)
const taType = ref(0)
const stage = ref('pick') // pick | result
const myInfo = ref(null)
const taInfo = ref(null)
const analysis = ref(null)
const allTypes = Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] }))

onLoad((q) => {
  if (q && q.type) {
    myType.value = Number(q.type)
  }
})

function pickMy(id) { myType.value = id }
function pickTa(id) { taType.value = id }

function analyze() {
  if (!myType.value || !taType.value) {
    uni.showToast({ title: '请选择两个型号', icon: 'none' })
    return
  }
  const a = TYPES_INFO[myType.value]
  const b = TYPES_INFO[taType.value]
  myInfo.value = { id: myType.value, ...a }
  taInfo.value = { id: taType.value, ...b }
  analysis.value = buildAnalysis(myType.value, taType.value, a, b)
  stage.value = 'result'
}

// 基于「中心异同 + 编号关系」生成关系解读
function buildAnalysis(mId, tId, a, b) {
  const sameCenter = a.center === b.center
  const diff = Math.abs(mId - tId)
  const adjacent = diff === 1 || diff === 8
  const same = mId === tId

  let bond, friction, tip, score
  if (same) {
    bond = '你们是同一型，彼此的节奏、在意的点高度一致，天然有「他懂我」的默契。'
    friction = '正因为太像，你们也容易把同一个盲点放大——两个人同时陷进相同的情绪或回避里。'
    tip = '刻意为关系引入一点「不同视角」，轮流当那个先冷静、先开口的人。'
    score = 85
  } else if (sameCenter) {
    bond = `你们同属「${CENTERS[a.center].name}」，面对世界的底层方式相近，沟通时更容易在同一个频道上。`
    friction = '相同中心也意味着相似的应激反应：压力来时你们可能用同一种方式逃，谁也接不住谁。'
    tip = '约定一个「暂停信号」，当两人同时上头时，先各自落地再回来谈。'
    score = 78
  } else if (adjacent) {
    bond = '你们型号相邻，像两块能咬合的拼图，差异不大却能互相补位，相处通常顺滑。'
    friction = '细微的价值排序差异，日久会变成「你怎么总是这样」的小摩擦。'
    tip = '把对方那点「和你不一样」当成资源而非毛病，常表达具体的欣赏。'
    score = 82
  } else {
    bond = `你们分属「${CENTERS[a.center].name}」与「${CENTERS[b.center].name}」，看世界的角度很不一样，正好能照见彼此的盲区。`
    friction = '差异大，最初容易互相看不顺眼：你重视的，TA 可能根本不在意。'
    tip = '先理解再回应——把「TA 为什么这么做」当成好奇而不是指责，差异会变成互补。'
    score = 70
  }

  return {
    score,
    bond,
    friction,
    tip,
    myDrive: `${mId}号 ${a.name}：${a.desire}`,
    taDrive: `${tId}号 ${b.name}：${b.desire}`,
  }
}

function reset() {
  stage.value = 'pick'
  analysis.value = null
}
</script>

<template>
  <view class="wrap relation">
    <!-- 选型 -->
    <template v-if="stage === 'pick'">
      <view class="card intro">
        <text class="intro__t">九型关系合盘</text>
        <text class="intro__d">选择你和 TA 的型号，看你们的相处底色、潜在摩擦与相处建议。</text>
      </view>

      <view class="card">
        <text class="pick__label">我的型号</text>
        <view class="grid">
          <view
            v-for="t in allTypes" :key="'m' + t.id"
            class="chip" :class="{ on: myType === t.id }"
            @click="pickMy(t.id)"
          >{{ t.id }} {{ t.name }}</view>
        </view>
      </view>

      <view class="card">
        <text class="pick__label">TA 的型号</text>
        <view class="grid">
          <view
            v-for="t in allTypes" :key="'t' + t.id"
            class="chip" :class="{ on: taType === t.id }"
            @click="pickTa(t.id)"
          >{{ t.id }} {{ t.name }}</view>
        </view>
      </view>

      <button class="btn-primary" @click="analyze">生成合盘解读</button>
    </template>

    <!-- 结果 -->
    <template v-else>
      <view class="card pair">
        <view class="pair__side">
          <image class="pair__avatar" :src="`/static/avatars/${myInfo.id}.png`" mode="aspectFill" />
          <text class="pair__name">我 · {{ myInfo.id }}号</text>
        </view>
        <view class="pair__score">
          <text class="pair__num">{{ analysis.score }}</text>
          <text class="pair__lbl">契合指数</text>
        </view>
        <view class="pair__side">
          <image class="pair__avatar" :src="`/static/avatars/${taInfo.id}.png`" mode="aspectFill" />
          <text class="pair__name">TA · {{ taInfo.id }}号</text>
        </view>
      </view>

      <view class="card">
        <text class="sec-title">相处底色</text>
        <text class="sec-txt">{{ analysis.bond }}</text>
      </view>
      <view class="card">
        <text class="sec-title">潜在摩擦</text>
        <text class="sec-txt">{{ analysis.friction }}</text>
      </view>
      <view class="card grow">
        <text class="sec-title">相处建议</text>
        <text class="sec-txt">{{ analysis.tip }}</text>
      </view>
      <view class="card">
        <text class="sec-title">各自的核心驱动</text>
        <text class="sec-txt">· {{ analysis.myDrive }}</text>
        <text class="sec-txt">· {{ analysis.taDrive }}</text>
      </view>

      <button class="btn-ghost" @click="reset">换一对再看</button>
      <text class="disclaimer">合盘基于九型中心与型号关系生成，供关系沟通参考，非专业咨询结论。</text>
    </template>
  </view>
</template>

<style scoped>
.relation { display: flex; flex-direction: column; gap: 20rpx; padding-bottom: 60rpx; }
.intro__t { font-size: 36rpx; font-weight: 800; display: block; }
.intro__d { color: #5d6b7e; font-size: 26rpx; line-height: 1.6; display: block; margin-top: 10rpx; }
.pick__label { font-size: 28rpx; font-weight: 700; display: block; margin-bottom: 16rpx; }
.grid { display: flex; flex-wrap: wrap; gap: 14rpx; }
.chip { padding: 14rpx 22rpx; border-radius: 999rpx; background: #f4f7f9; font-size: 24rpx; color: #42505e; border: 2rpx solid transparent; }
.chip.on { background: #2b7fff14; color: #1f73c4; border-color: #2b7fff66; font-weight: 700; }

.pair { display: flex; align-items: center; justify-content: space-between; }
.pair__side { display: flex; flex-direction: column; align-items: center; gap: 10rpx; }
.pair__avatar { width: 110rpx; height: 110rpx; border-radius: 50%; }
.pair__name { font-size: 25rpx; font-weight: 700; }
.pair__score { display: flex; flex-direction: column; align-items: center; }
.pair__num { font-size: 64rpx; font-weight: 800; color: #1f73c4; line-height: 1; }
.pair__lbl { font-size: 22rpx; color: #9aa7b5; margin-top: 8rpx; }

.sec-title { font-size: 30rpx; font-weight: 700; display: block; margin-bottom: 14rpx; }
.sec-txt { color: #42505e; font-size: 27rpx; line-height: 1.7; display: block; margin-bottom: 6rpx; }
.grow { background: linear-gradient(120deg, #38a83a0f, #1f73c40a); }
.btn-primary, .btn-ghost { border-radius: 999rpx; font-size: 30rpx; }
.btn-ghost { background: #fff; color: #1a2430; border: 2rpx solid #e3e8ee; }
.btn-ghost::after { border: none; }
.disclaimer { color: #9aa7b5; font-size: 22rpx; text-align: center; margin-top: 12rpx; line-height: 1.6; }
</style>
