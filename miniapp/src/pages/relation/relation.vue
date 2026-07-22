<script setup>
import { ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'
import { TYPES_INFO, CENTERS } from '../../data/enneagramGame'
import { isValidTypeId, normalizeTypeId } from '../../utils/session'

const myType = ref(0)
const taType = ref(0)
const stage = ref('pick') // pick | result | redirecting
const myInfo = ref(null)
const taInfo = ref(null)
const analysis = ref(null)
const myAvatarFailed = ref(false)
const taAvatarFailed = ref(false)
const allTypes = Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] }))

onLoad((q) => {
  if (q && Object.prototype.hasOwnProperty.call(q, 'type')) {
    const type = normalizeTypeId(q.type)
    if (!type) {
      rejectInvalidType()
      return
    }
    myType.value = type
  }
})

function pickMy(id) { myType.value = id }
function pickTa(id) { taType.value = id }

function analyze() {
  if (!myType.value || !taType.value) {
    uni.showToast({ title: '请选择两个型号', icon: 'none' })
    return
  }
  if (!isValidTypeId(myType.value) || !isValidTypeId(taType.value)) {
    uni.showToast({ title: '型号参数无效，请重新选择', icon: 'none' })
    return
  }
  const mine = normalizeTypeId(myType.value)
  const ta = normalizeTypeId(taType.value)
  const a = TYPES_INFO[mine]
  const b = TYPES_INFO[ta]
  myType.value = mine
  taType.value = ta
  myInfo.value = { id: mine, ...a }
  taInfo.value = { id: ta, ...b }
  analysis.value = buildAnalysis(mine, ta, a, b)
  myAvatarFailed.value = false
  taAvatarFailed.value = false
  stage.value = 'result'
}

function onMyAvatarError() {
  myAvatarFailed.value = true
}

function onTaAvatarError() {
  taAvatarFailed.value = true
}

function rejectInvalidType() {
  stage.value = 'redirecting'
  uni.showToast({ title: '型号参数无效，请重新测试', icon: 'none' })
  setTimeout(() => {
    uni.redirectTo({ url: '/pages/test/test' })
  }, 600)
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
  myAvatarFailed.value = false
  taAvatarFailed.value = false
}
</script>

<template>
  <view class="wrap relation page-stack ios-page ios-safe-bottom">
    <!-- 选型 -->
    <template v-if="stage === 'pick'">
      <view class="relation-hero nx-page-hero">
        <text class="relation-hero__eyebrow">RELATION ENERGY</text>
        <text class="relation-hero__title">看见你们之间的连接</text>
        <text class="relation-hero__desc">选择你和 TA 的型号，从相处底色、潜在摩擦与核心驱动力读懂这段关系。</text>
        <view class="relation-hero__orbit relation-hero__orbit--one" />
        <view class="relation-hero__orbit relation-hero__orbit--two" />
      </view>

      <view class="type-picker nx-panel">
        <view class="type-picker__head">
          <view>
            <text class="type-picker__step">STEP 01</text>
            <text class="pick__label">我的型号</text>
          </view>
          <text class="type-picker__hint">选择代表你的类型</text>
        </view>
        <view class="grid">
          <button
            v-for="t in allTypes" :key="'m' + t.id"
            class="type-chip nx-focusable" :class="{ on: myType === t.id }"
            :aria-label="`选择我的型号 ${t.id} ${t.name}`"
            :aria-pressed="myType === t.id"
            hover-class="type-chip--pressed"
            @click="pickMy(t.id)"
          >
            <text class="type-chip__number">{{ t.id }}</text>
            <text class="type-chip__name">{{ t.name }}</text>
            <text v-if="myType === t.id" class="type-chip__selected">已选</text>
          </button>
        </view>
      </view>

      <view class="type-picker nx-panel">
        <view class="type-picker__head">
          <view>
            <text class="type-picker__step">STEP 02</text>
            <text class="pick__label">TA 的型号</text>
          </view>
          <text class="type-picker__hint">选择代表 TA 的类型</text>
        </view>
        <view class="grid">
          <button
            v-for="t in allTypes" :key="'t' + t.id"
            class="type-chip nx-focusable" :class="{ on: taType === t.id }"
            :aria-label="`选择 TA 的型号 ${t.id} ${t.name}`"
            :aria-pressed="taType === t.id"
            hover-class="type-chip--pressed"
            @click="pickTa(t.id)"
          >
            <text class="type-chip__number">{{ t.id }}</text>
            <text class="type-chip__name">{{ t.name }}</text>
            <text v-if="taType === t.id" class="type-chip__selected">已选</text>
          </button>
        </view>
      </view>

      <button class="btn-primary ios-button nx-focusable" hover-class="analyze--pressed" @click="analyze">生成合盘解读</button>
    </template>

    <!-- 结果 -->
    <template v-else-if="stage === 'result'">
      <view class="pair nx-page-hero">
        <view class="pair__side">
          <image v-if="!myAvatarFailed" class="pair__avatar" :src="`/static/avatars/${myInfo.id}.png`" mode="aspectFill" lazy-load @error="onMyAvatarError" />
          <view v-else class="pair__avatar-fallback">{{ myInfo.id }}</view>
          <text class="pair__role">我的能量</text>
          <text class="pair__name">{{ myInfo.id }}号 · {{ myInfo.name }}</text>
        </view>
        <view class="pair-connection">
          <text class="pair-connection__eyebrow">CONNECTION</text>
          <text class="pair-connection__score">{{ analysis.score }}</text>
          <text class="pair-connection__label">契合指数</text>
          <view class="pair-connection__line" />
        </view>
        <view class="pair__side">
          <image v-if="!taAvatarFailed" class="pair__avatar" :src="`/static/avatars/${taInfo.id}.png`" mode="aspectFill" lazy-load @error="onTaAvatarError" />
          <view v-else class="pair__avatar-fallback">{{ taInfo.id }}</view>
          <text class="pair__role">TA 的能量</text>
          <text class="pair__name">{{ taInfo.id }}号 · {{ taInfo.name }}</text>
        </view>
      </view>

      <view class="insight nx-panel insight--bond">
        <view class="insight__icon insight__icon--bond">✦</view>
        <view class="insight__content">
          <text class="insight__eyebrow">RELATION BOND</text>
          <text class="insight__title">相处底色</text>
          <text class="insight__text">{{ analysis.bond }}</text>
        </view>
      </view>

      <view class="insight nx-panel insight--friction">
        <view class="insight__icon insight__icon--friction">⚡</view>
        <view class="insight__content">
          <text class="insight__eyebrow">FRICTION POINT</text>
          <text class="insight__title">潜在摩擦</text>
          <text class="insight__text">{{ analysis.friction }}</text>
        </view>
      </view>

      <view class="insight nx-panel insight--tip">
        <view class="insight__icon insight__icon--tip">↗</view>
        <view class="insight__content">
          <text class="insight__eyebrow">GROW TOGETHER</text>
          <text class="insight__title">相处建议</text>
          <text class="insight__text">{{ analysis.tip }}</text>
        </view>
      </view>

      <view class="drive nx-panel">
        <view class="drive__head">
          <text class="drive__eyebrow">INNER DRIVE</text>
          <text class="drive__title">各自的核心驱动</text>
        </view>
        <view class="drive-pair">
          <view class="drive-card drive-card--mine">
            <text class="drive-card__label">我的驱动力</text>
            <text class="drive-card__text">{{ analysis.myDrive }}</text>
          </view>
          <view class="drive-card drive-card--ta">
            <text class="drive-card__label">TA 的驱动力</text>
            <text class="drive-card__text">{{ analysis.taDrive }}</text>
          </view>
        </view>
      </view>

      <button class="btn-ghost ios-button nx-focusable" hover-class="reset--pressed" @click="reset">换一对再看</button>
      <text class="disclaimer">合盘基于九型中心与型号关系生成，供关系沟通参考，非专业咨询结论。</text>
    </template>

    <view v-else class="card ios-card redirecting">
      <text class="sec-title">型号参数无效</text>
      <text class="sec-txt">正在返回测试页，请重新完成测试。</text>
    </view>
  </view>
</template>

<style scoped>
.relation {
  display: flex;
  flex-direction: column;
  gap: 24rpx;
  background:
    radial-gradient(circle at 0 4%, rgba(168, 85, 247, .13), transparent 34%),
    radial-gradient(circle at 100% 28%, rgba(236, 72, 153, .10), transparent 30%),
    #f6f3fb;
}
.relation-hero {
  min-height: 300rpx;
  display: flex;
  flex-direction: column;
  justify-content: center;
  background: linear-gradient(135deg, #6d28d9 0%, #db2777 100%);
  box-shadow: 0 28rpx 64rpx rgba(109, 40, 217, .24);
}
.relation-hero__eyebrow {
  position: relative;
  z-index: 1;
  color: rgba(255, 255, 255, .92);
  font-size: 21rpx;
  font-weight: 800;
  letter-spacing: 3rpx;
}
.relation-hero__title {
  position: relative;
  z-index: 1;
  color: #fff;
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.2;
  margin-top: 14rpx;
}
.relation-hero__desc {
  position: relative;
  z-index: 1;
  max-width: 570rpx;
  color: rgba(255, 255, 255, .92);
  font-size: 26rpx;
  line-height: 1.65;
  margin-top: 18rpx;
}
.relation-hero__orbit {
  position: absolute;
  border: 2rpx solid rgba(255, 255, 255, .22);
  border-radius: 50%;
}
.relation-hero__orbit--one { width: 260rpx; height: 260rpx; right: -88rpx; top: -86rpx; }
.relation-hero__orbit--two { width: 150rpx; height: 150rpx; right: 64rpx; bottom: -102rpx; }
.type-picker { box-shadow: 0 18rpx 46rpx rgba(76, 29, 149, .08); }
.type-picker__head { display: flex; align-items: flex-end; justify-content: space-between; gap: 20rpx; margin-bottom: 22rpx; }
.type-picker__step { display: block; color: #7e22ce; font-size: 20rpx; font-weight: 900; letter-spacing: 2rpx; margin-bottom: 7rpx; }
.pick__label { color: #1e293b; font-size: 31rpx; font-weight: 800; display: block; }
.type-picker__hint { color: #64748b; font-size: 22rpx; line-height: 1.4; text-align: right; }
.grid { display: flex; flex-wrap: wrap; gap: 16rpx; }
.type-chip {
  width: calc((100% - 32rpx) / 3);
  min-height: 88rpx;
  margin: 0;
  padding: 13rpx 12rpx;
  border-radius: 24rpx;
  background: #faf7ff;
  color: #334155;
  border: 2rpx solid transparent;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  line-height: 1;
}
.type-chip::after { border: none; }
.type-chip.on {
  border: 4rpx solid #9333ea;
  background: linear-gradient(145deg, #f5e9ff, #fdf2f8);
  color: #6b21a8;
  box-shadow: inset 0 0 0 2rpx rgba(255, 255, 255, .9), 0 10rpx 24rpx rgba(147, 51, 234, .14);
}
.type-chip--pressed { opacity: .82; transform: scale(.98); }
.type-chip__number { font-size: 30rpx; font-weight: 900; }
.type-chip__name { font-size: 21rpx; font-weight: 700; line-height: 1.25; margin-top: 7rpx; }
.type-chip__selected { color: #86198f; font-size: 18rpx; font-weight: 900; margin-top: 7rpx; }
.analyze--pressed, .reset--pressed { opacity: .84; transform: scale(.985); }

.pair {
  min-height: 330rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  color: #fff;
  background: linear-gradient(140deg, #4c1d95 0%, #7e22ce 46%, #be185d 100%);
  box-shadow: 0 28rpx 64rpx rgba(107, 33, 168, .24);
}
.pair::after {
  content: '';
  position: absolute;
  width: 250rpx;
  height: 250rpx;
  border-radius: 50%;
  right: -100rpx;
  bottom: -130rpx;
  background: rgba(255, 255, 255, .08);
}
.pair__side { position: relative; z-index: 1; flex: 1; min-width: 0; display: flex; flex-direction: column; align-items: center; }
.pair__avatar {
  width: 112rpx;
  height: 112rpx;
  border-radius: 50%;
  border: 6rpx solid rgba(255, 255, 255, .76);
  box-sizing: border-box;
  background: rgba(255, 255, 255, .18);
}
.pair__avatar-fallback {
  width: 112rpx;
  height: 112rpx;
  border-radius: 50%;
  border: 6rpx solid rgba(255, 255, 255, .76);
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #581c87;
  background: #fdf4ff;
  font-size: 42rpx;
  font-weight: 900;
}
.pair__role { color: rgba(255, 255, 255, .88); font-size: 19rpx; font-weight: 800; margin-top: 14rpx; }
.pair__name { max-width: 190rpx; color: #fff; font-size: 23rpx; font-weight: 800; line-height: 1.35; margin-top: 6rpx; text-align: center; }
.pair-connection { position: relative; z-index: 1; width: 154rpx; display: flex; flex-direction: column; align-items: center; }
.pair-connection__eyebrow { color: rgba(255, 255, 255, .86); font-size: 16rpx; font-weight: 900; letter-spacing: 1rpx; }
.pair-connection__score { color: #fff; font-size: 72rpx; font-weight: 900; line-height: 1; margin-top: 9rpx; }
.pair-connection__label { color: #fff; font-size: 20rpx; font-weight: 800; margin-top: 8rpx; }
.pair-connection__line { width: 112rpx; height: 4rpx; border-radius: 999rpx; margin-top: 18rpx; background: linear-gradient(90deg, transparent, #f9a8d4, transparent); }

.insight { display: flex; align-items: flex-start; gap: 22rpx; border-width: 2rpx; border-style: solid; box-shadow: 0 16rpx 42rpx rgba(51, 65, 85, .06); }
.insight--bond { border-color: #c084fc; background: linear-gradient(135deg, #fff, #faf5ff); }
.insight--friction { border-color: #fb7185; background: linear-gradient(135deg, #fff, #fff1f2); }
.insight--tip { border-color: #2dd4bf; background: linear-gradient(135deg, #fff, #f0fdfa); }
.insight__icon { flex: 0 0 72rpx; width: 72rpx; height: 72rpx; border-radius: 22rpx; display: flex; align-items: center; justify-content: center; font-size: 34rpx; font-weight: 900; }
.insight__icon--bond { color: #7e22ce; background: #f3e8ff; }
.insight__icon--friction { color: #be123c; background: #ffe4e6; }
.insight__icon--tip { color: #0f766e; background: #ccfbf1; }
.insight__content { flex: 1; min-width: 0; }
.insight__eyebrow { color: #64748b; font-size: 18rpx; font-weight: 900; letter-spacing: 1rpx; }
.insight__title { display: block; color: #1e293b; font-size: 30rpx; font-weight: 900; margin-top: 5rpx; }
.insight__text { display: block; color: #334155; font-size: 26rpx; line-height: 1.72; margin-top: 13rpx; }

.drive { box-shadow: 0 16rpx 42rpx rgba(51, 65, 85, .06); }
.drive__head { margin-bottom: 20rpx; }
.drive__eyebrow { display: block; color: #7e22ce; font-size: 19rpx; font-weight: 900; letter-spacing: 2rpx; }
.drive__title { display: block; color: #1e293b; font-size: 30rpx; font-weight: 900; margin-top: 7rpx; }
.drive-pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16rpx; }
.drive-card { min-height: 170rpx; padding: 22rpx; border-radius: 24rpx; box-sizing: border-box; }
.drive-card--mine { background: #f5f3ff; }
.drive-card--ta { background: #fdf2f8; }
.drive-card__label { display: block; color: #6b21a8; font-size: 20rpx; font-weight: 900; }
.drive-card__text { display: block; color: #334155; font-size: 23rpx; line-height: 1.6; margin-top: 10rpx; }

.redirecting { text-align: center; }
.btn-primary, .btn-ghost { border-radius: 999rpx; font-size: 30rpx; }
.btn-primary { background: linear-gradient(110deg, #7e22ce, #db2777); box-shadow: 0 18rpx 38rpx rgba(126, 34, 206, .22); }
.btn-ghost { background: #fff; color: #4c1d95; border: 2rpx solid #d8b4fe; }
.btn-ghost::after { border: none; }
.disclaimer { color: #64748b; font-size: 22rpx; text-align: center; margin-top: 8rpx; line-height: 1.6; }

@media (max-width: 360px) {
  .type-picker__hint { display: none; }
  .pair { padding-left: 22rpx; padding-right: 22rpx; }
  .pair-connection { width: 130rpx; }
}
</style>
