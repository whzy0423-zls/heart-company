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
            role="button"
            aria-role="button"
            tabindex="0"
            hover-class="type-chip--pressed"
            @click="pickMy(t.id)"
            @keydown.enter="pickMy(t.id)"
            @keydown.space.prevent="pickMy(t.id)"
          >
            <text class="type-chip__number">{{ t.id }}</text>
            <text class="type-chip__name">{{ t.name }}</text>
            <text v-if="myType === t.id" class="type-chip__selected">已选</text>
            <text v-else class="type-chip__selected type-chip__selected--placeholder" aria-hidden="true">已选</text>
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
            role="button"
            aria-role="button"
            tabindex="0"
            hover-class="type-chip--pressed"
            @click="pickTa(t.id)"
            @keydown.enter="pickTa(t.id)"
            @keydown.space.prevent="pickTa(t.id)"
          >
            <text class="type-chip__number">{{ t.id }}</text>
            <text class="type-chip__name">{{ t.name }}</text>
            <text v-if="taType === t.id" class="type-chip__selected">已选</text>
            <text v-else class="type-chip__selected type-chip__selected--placeholder" aria-hidden="true">已选</text>
          </button>
        </view>
      </view>

      <button
        class="btn-primary ios-button nx-focusable"
        role="button"
        aria-role="button"
        tabindex="0"
        hover-class="analyze--pressed"
        @click="analyze"
        @keydown.enter="analyze"
        @keydown.space.prevent="analyze"
      >生成合盘解读</button>
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
        <view class="insight__icon insight__icon--bond" aria-hidden="true">
          <view class="insight__mark insight__mark--bond" />
        </view>
        <view class="insight__content">
          <text class="insight__eyebrow">RELATION BOND</text>
          <text class="insight__title">相处底色</text>
          <text class="insight__text">{{ analysis.bond }}</text>
        </view>
      </view>

      <view class="insight nx-panel insight--friction">
        <view class="insight__icon insight__icon--friction" aria-hidden="true">
          <view class="insight__mark insight__mark--friction" />
        </view>
        <view class="insight__content">
          <text class="insight__eyebrow">FRICTION POINT</text>
          <text class="insight__title">潜在摩擦</text>
          <text class="insight__text">{{ analysis.friction }}</text>
        </view>
      </view>

      <view class="insight nx-panel insight--tip">
        <view class="insight__icon insight__icon--tip" aria-hidden="true">
          <view class="insight__mark insight__mark--tip" />
        </view>
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

      <button
        class="btn-ghost ios-button nx-focusable"
        role="button"
        aria-role="button"
        tabindex="0"
        hover-class="reset--pressed"
        @click="reset"
        @keydown.enter="reset"
        @keydown.space.prevent="reset"
      >换一对再看</button>
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
  background: var(--nx-page-bg);
}
.relation-hero {
  min-height: 300rpx;
  display: flex;
  flex-direction: column;
  justify-content: center;
  border: 2rpx solid rgba(223, 188, 127, .34);
  background:
    radial-gradient(circle at 92% 8%, rgba(223, 188, 127, .26), transparent 34%),
    linear-gradient(135deg, var(--nx-brand-900), var(--nx-brand-700));
  box-shadow: 0 28rpx 64rpx -36rpx rgba(32, 42, 55, .72);
}
.relation-hero__eyebrow {
  position: relative;
  z-index: 1;
  color: var(--nx-accent-gold);
  font-size: 24rpx;
  font-weight: 800;
  letter-spacing: 3rpx;
}
.relation-hero__title {
  position: relative;
  z-index: 1;
  color: var(--nx-surface);
  font-size: 42rpx;
  font-weight: 900;
  line-height: 1.2;
  margin-top: 14rpx;
}
.relation-hero__desc {
  position: relative;
  z-index: 1;
  max-width: 570rpx;
  color: rgba(255, 255, 255, .82);
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
.type-picker { background: var(--nx-surface); border-color: var(--nx-border); box-shadow: 0 18rpx 46rpx -36rpx rgba(32, 42, 55, .42); }
.type-picker__head { display: flex; align-items: flex-end; justify-content: space-between; gap: 20rpx; margin-bottom: 22rpx; }
.type-picker__step { display: block; color: var(--nx-brand-700); font-size: 24rpx; font-weight: 900; letter-spacing: 2rpx; margin-bottom: 7rpx; }
.pick__label { color: var(--nx-text); font-size: 31rpx; font-weight: 800; display: block; }
.type-picker__hint { color: var(--nx-text-muted); font-size: 24rpx; line-height: 1.4; text-align: right; }
.grid { display: flex; flex-wrap: wrap; gap: 16rpx; }
.type-chip {
  width: calc((100% - 32rpx) / 3);
  height: 128rpx;
  min-height: 128rpx;
  margin: 0;
  padding: 13rpx 12rpx;
  border-radius: 24rpx;
  background: var(--nx-surface-soft);
  color: var(--nx-text);
  border: 2rpx solid var(--nx-border);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  line-height: 1;
}
.type-chip::after { border: none; }
.type-chip.on {
  border: 4rpx solid var(--nx-accent-gold);
  background: var(--nx-surface);
  color: var(--nx-brand-900);
  box-shadow: inset 0 0 0 2rpx rgba(255, 255, 255, .9), 0 12rpx 28rpx -20rpx rgba(32, 42, 55, .42);
}
.type-chip--pressed { opacity: .82; transform: scale(.98); }
.type-chip__number { font-size: 30rpx; font-weight: 900; }
.type-chip__name { font-size: 24rpx; font-weight: 700; line-height: 1.25; margin-top: 7rpx; }
.type-chip__selected { color: var(--nx-brand-700); font-size: 24rpx; font-weight: 900; line-height: 1.2; margin-top: 7rpx; }
.type-chip__selected--placeholder { visibility: hidden; }
.analyze--pressed, .reset--pressed { opacity: .84; transform: scale(.985); }

.pair {
  min-height: 330rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  color: var(--nx-surface);
  border: 2rpx solid rgba(223, 188, 127, .34);
  background:
    radial-gradient(circle at 50% 105%, rgba(223, 188, 127, .22), transparent 35%),
    linear-gradient(140deg, var(--nx-brand-900), var(--nx-brand-700));
  box-shadow: 0 28rpx 64rpx -36rpx rgba(32, 42, 55, .72);
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
  color: var(--nx-brand-900);
  background: var(--nx-surface-soft);
  font-size: 42rpx;
  font-weight: 900;
}
.pair__role { color: rgba(255, 255, 255, .88); font-size: 24rpx; font-weight: 800; margin-top: 14rpx; }
.pair__name { max-width: 190rpx; color: var(--nx-surface); font-size: 24rpx; font-weight: 800; line-height: 1.35; margin-top: 6rpx; text-align: center; }
.pair-connection { position: relative; z-index: 1; width: 154rpx; display: flex; flex-direction: column; align-items: center; }
.pair-connection__eyebrow { color: rgba(255, 255, 255, .86); font-size: 24rpx; font-weight: 900; letter-spacing: 1rpx; }
.pair-connection__score { color: var(--nx-accent-gold); font-size: 72rpx; font-weight: 900; line-height: 1; margin-top: 9rpx; }
.pair-connection__label { color: var(--nx-surface); font-size: 24rpx; font-weight: 800; margin-top: 8rpx; }
.pair-connection__line { width: 112rpx; height: 4rpx; border-radius: 999rpx; margin-top: 18rpx; background: linear-gradient(90deg, transparent, var(--nx-accent-gold), transparent); }

.insight { display: flex; align-items: flex-start; gap: 22rpx; border: 2rpx solid var(--nx-border); background: var(--nx-surface); box-shadow: 0 16rpx 42rpx -34rpx rgba(32, 42, 55, .34); }
.insight--bond { border-left: 6rpx solid var(--nx-accent-gold); }
.insight--friction { border-left: 6rpx solid var(--nx-text-muted); }
.insight--tip { border-left: 6rpx solid var(--nx-brand-700); }
.insight__icon { flex: 0 0 72rpx; width: 72rpx; height: 72rpx; border-radius: 22rpx; display: flex; align-items: center; justify-content: center; }
.insight__icon--bond { color: var(--nx-brand-900); background: rgba(223, 188, 127, .26); }
.insight__icon--friction { color: var(--nx-text-muted); background: var(--nx-surface-soft); }
.insight__icon--tip { color: var(--nx-brand-700); background: var(--nx-surface-soft); }
.insight__mark { position: relative; display: block; box-sizing: border-box; color: inherit; }
.insight__mark--bond { width: 28rpx; height: 28rpx; border: 5rpx solid currentColor; border-radius: 50%; }
.insight__mark--bond::after { content: ''; position: absolute; width: 8rpx; height: 8rpx; border-radius: 50%; left: 5rpx; top: 5rpx; background: currentColor; }
.insight__mark--friction { width: 7rpx; height: 34rpx; border-radius: 999rpx; background: currentColor; transform: rotate(28deg); }
.insight__mark--friction::after { content: ''; position: absolute; width: 7rpx; height: 22rpx; border-radius: 999rpx; left: 9rpx; top: 5rpx; background: currentColor; transform: rotate(-56deg); }
.insight__mark--tip { width: 27rpx; height: 27rpx; border-top: 5rpx solid currentColor; border-right: 5rpx solid currentColor; }
.insight__mark--tip::after { content: ''; position: absolute; width: 5rpx; height: 35rpx; border-radius: 999rpx; right: 10rpx; top: -3rpx; background: currentColor; transform: rotate(45deg); transform-origin: top center; }
.insight__content { flex: 1; min-width: 0; }
.insight__eyebrow { color: var(--nx-text-muted); font-size: 24rpx; font-weight: 900; letter-spacing: 1rpx; }
.insight__title { display: block; color: var(--nx-text); font-size: 30rpx; font-weight: 900; margin-top: 5rpx; }
.insight__text { display: block; color: var(--nx-text); font-size: 26rpx; line-height: 1.72; margin-top: 13rpx; }

.drive { background: var(--nx-surface); border-color: var(--nx-border); box-shadow: 0 16rpx 42rpx -34rpx rgba(32, 42, 55, .34); }
.drive__head { margin-bottom: 20rpx; }
.drive__eyebrow { display: block; color: var(--nx-brand-700); font-size: 24rpx; font-weight: 900; letter-spacing: 2rpx; }
.drive__title { display: block; color: var(--nx-text); font-size: 30rpx; font-weight: 900; margin-top: 7rpx; }
.drive-pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16rpx; }
.drive-card { min-height: 170rpx; padding: 22rpx; border-radius: 24rpx; box-sizing: border-box; }
.drive-card--mine { background: var(--nx-surface-soft); border: 2rpx solid var(--nx-border); }
.drive-card--ta { background: rgba(223, 188, 127, .16); border: 2rpx solid rgba(223, 188, 127, .42); }
.drive-card__label { display: block; color: var(--nx-brand-700); font-size: 24rpx; font-weight: 900; }
.drive-card__text { display: block; color: var(--nx-text); font-size: 24rpx; line-height: 1.6; margin-top: 10rpx; }

.redirecting { text-align: center; }
.btn-primary, .btn-ghost { min-height: 88rpx; border-radius: 999rpx; font-size: 30rpx; }
.btn-primary { background: linear-gradient(110deg, var(--nx-brand-900), var(--nx-brand-700)); box-shadow: 0 18rpx 38rpx -24rpx rgba(32, 42, 55, .56); }
.btn-ghost { background: var(--nx-surface); color: var(--nx-brand-900); border: 2rpx solid var(--nx-border); }
.btn-ghost::after { border: none; }
.disclaimer { color: var(--nx-text-muted); font-size: 24rpx; text-align: center; margin-top: 8rpx; line-height: 1.6; }

@media (max-width: 360px) {
  .type-picker__hint { display: none; }
  .pair { padding-left: 22rpx; padding-right: 22rpx; }
  .pair-connection { width: 130rpx; }
}
</style>
