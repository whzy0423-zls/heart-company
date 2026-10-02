<script setup>
import { ref, nextTick } from 'vue'
import { onLoad, onShow, onUnload, onShareAppMessage, onShareTimeline } from '@dcloudio/uni-app'
import { TYPES_INFO } from '../../data/enneagramGame'
import { isValidTypeId, normalizeTypeId } from '../../utils/session'
import { previewImage } from '../../utils/imagePreview'
import { buildRelationAnalysis } from './relationAnalysis'
import { buildShareCard, showPublicShareMenu, isTimelinePreview, requireFullMiniapp } from '../../utils/share'
import NxShareActions from '../../components/NxShareActions.vue'

const myType = ref(0)
const taType = ref(0)
const stage = ref('pick') // pick | result | redirecting
const myInfo = ref(null)
const taInfo = ref(null)
const analysis = ref(null)
const myAvatarFailed = ref(false)
const taAvatarFailed = ref(false)
const pickerAvatarFailed = ref({})
const sharedMode = ref(false)
const routeError = ref('')
const toolsExpanded = ref(false)
const sourcesExpanded = ref(false)
const timelinePreview = ref(isTimelinePreview())
const allTypes = Object.keys(TYPES_INFO).map((id) => ({ id: Number(id), ...TYPES_INFO[id] }))
let redirectTimer = null

function shareTypeId(value) {
  return ['string', 'number'].includes(typeof value) && /^[1-9]$/.test(String(value)) ? Number(value) : 0
}

onLoad((q = {}) => {
  const hasPair = ['myType', 'taType'].some(key => Object.prototype.hasOwnProperty.call(q, key))
  if (hasPair) {
    const mine = shareTypeId(q.myType)
    const ta = shareTypeId(q.taType)
    if (!mine || !ta) {
      routeError.value = '这条合盘链接的型号不完整，请重新选择两种型号。'
      return
    }
    sharedMode.value = true
    myType.value = mine
    taType.value = ta
    analyze()
    return
  }
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
  analysis.value = buildRelationAnalysis(mine, ta)
  toolsExpanded.value = false
  sourcesExpanded.value = false
  routeError.value = ''
  myAvatarFailed.value = false
  taAvatarFailed.value = false
  stage.value = 'result'
  nextTick(() => {
    if (stage.value === 'result' && !isTimelinePreview()) uni.pageScrollTo({ scrollTop: 0, duration: 0 })
  })
}

function onMyAvatarError() {
  myAvatarFailed.value = true
}

function onTaAvatarError() {
  taAvatarFailed.value = true
}

function typeAvatarSource(typeId) {
  const id = normalizeTypeId(typeId)
  return id ? `/static/enneagram/${id}.png` : ''
}

function previewMyAvatar() {
  const current = !myAvatarFailed.value ? typeAvatarSource(myInfo.value?.id) : ''
  if (!current) return
  const urls = [
    current,
    !taAvatarFailed.value ? typeAvatarSource(taInfo.value?.id) : '',
  ].filter(Boolean)
  previewImage(current, { urls })
}

function previewTaAvatar() {
  const current = !taAvatarFailed.value ? typeAvatarSource(taInfo.value?.id) : ''
  if (!current) return
  const urls = [
    !myAvatarFailed.value ? typeAvatarSource(myInfo.value?.id) : '',
    current,
  ].filter(Boolean)
  previewImage(current, { urls })
}

function rejectInvalidType() {
  if (isTimelinePreview()) {
    routeError.value = '这条合盘链接的型号无效，请进入小程序重新选择。'
    return
  }
  if (redirectTimer) clearTimeout(redirectTimer)
  stage.value = 'redirecting'
  uni.showToast({ title: '型号参数无效，请重新测试', icon: 'none' })
  redirectTimer = setTimeout(() => {
    redirectTimer = null
    uni.redirectTo({ url: '/pages/test/test' })
  }, 600)
}

onUnload(() => {
  if (redirectTimer) clearTimeout(redirectTimer)
  redirectTimer = null
})

function relationShareCard() {
  return buildShareCard({
    kind: 'relation',
    myType: stage.value === 'result' ? myType.value : undefined,
    taType: stage.value === 'result' ? taType.value : undefined,
  })
}
onShareAppMessage(() => relationShareCard().appMessage)
onShareTimeline(() => relationShareCard().timeline)

function copySourceLink(id) {
  if (!requireFullMiniapp()) return
  const source = analysis.value?.sources.find(item => item.id === id)
  if (!source) return
  uni.setClipboardData({
    data: source.url,
    success: () => uni.showToast({ title: '原文链接已复制', icon: 'none' }),
    fail: () => uni.showToast({ title: '复制失败，请重试', icon: 'none' }),
  })
}

function reset() {
  if (!requireFullMiniapp()) return
  stage.value = 'pick'
  analysis.value = null
  sharedMode.value = false
  routeError.value = ''
  toolsExpanded.value = false
  sourcesExpanded.value = false
  myAvatarFailed.value = false
  taAvatarFailed.value = false
  uni.pageScrollTo({ scrollTop: 0, duration: 0 })
}
onShow(() => {
  timelinePreview.value = isTimelinePreview()
  showPublicShareMenu(stage.value !== 'redirecting')
})
</script>

<template>
  <view class="wrap relation page-stack ios-page ios-safe-bottom">
    <!-- 选型 -->
    <template v-if="stage === 'pick'">
      <view v-if="routeError" class="route-note" role="status">{{ routeError }}</view>
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
            <view class="type-chip__figure" aria-hidden="true">
              <image v-if="!pickerAvatarFailed[t.id]" class="type-chip__portrait" :src="`/static/enneagram-cutouts/${t.id}.png`" mode="aspectFit" @error="pickerAvatarFailed[t.id] = true" />
              <text v-else class="type-chip__fallback">{{ t.id }}</text>
            </view>
            <view class="type-chip__caption"><text class="type-chip__number">{{ t.id }}号</text><text class="type-chip__name">{{ t.name }}</text></view>
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
            role="button"
            aria-role="button"
            tabindex="0"
            hover-class="type-chip--pressed"
            @click="pickTa(t.id)"
            @keydown.enter="pickTa(t.id)"
            @keydown.space.prevent="pickTa(t.id)"
          >
            <view class="type-chip__figure" aria-hidden="true">
              <image v-if="!pickerAvatarFailed[t.id]" class="type-chip__portrait" :src="`/static/enneagram-cutouts/${t.id}.png`" mode="aspectFit" @error="pickerAvatarFailed[t.id] = true" />
              <text v-else class="type-chip__fallback">{{ t.id }}</text>
            </view>
            <view class="type-chip__caption"><text class="type-chip__number">{{ t.id }}号</text><text class="type-chip__name">{{ t.name }}</text></view>
            <text v-if="taType === t.id" class="type-chip__selected">已选</text>
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
      <view class="result-heading">
        <text class="relation-hero__eyebrow">BETTER TOGETHER</text>
        <text class="result-heading__title">读懂彼此，让相处更近一步</text>
        <text class="result-heading__desc">{{ sharedMode ? '一份分享给你的型号组合解读，一起看看哪些描述贴近真实的相处。' : '从彼此的需求出发，把理解变成日常里做得到的小事。' }}</text>
      </view>
      <view class="pair nx-page-hero">
        <view class="pair__side">
          <button
            v-if="!myAvatarFailed"
            class="pair__avatar-action"
            :aria-label="sharedMode ? '预览一方的能量头像' : '预览我的能量头像'"
            hover-class="pair__avatar-action--pressed"
            @click="previewMyAvatar()"
          >
            <image class="pair__avatar" :src="typeAvatarSource(myInfo.id)" mode="aspectFill" lazy-load @error="onMyAvatarError" />
          </button>
          <view v-else class="pair__avatar-fallback">{{ myInfo.id }}</view>
          <text class="pair__role">{{ sharedMode ? '一方的型号' : '我的能量' }}</text>
          <text class="pair__name">{{ myInfo.id }}号 · {{ myInfo.name }}</text>
        </view>
        <view class="pair-connection">
          <text class="pair-connection__eyebrow">{{ myType }} × {{ taType }}</text>
          <text class="pair-connection__symbol">&amp;</text>
          <text class="pair-connection__label">{{ analysis.label }}</text>
          <view class="pair-connection__line" />
        </view>
        <view class="pair__side">
          <button
            v-if="!taAvatarFailed"
            class="pair__avatar-action"
            :aria-label="sharedMode ? '预览另一方的能量头像' : '预览 TA 的能量头像'"
            hover-class="pair__avatar-action--pressed"
            @click="previewTaAvatar()"
          >
            <image class="pair__avatar" :src="typeAvatarSource(taInfo.id)" mode="aspectFill" lazy-load @error="onTaAvatarError" />
          </button>
          <view v-else class="pair__avatar-fallback">{{ taInfo.id }}</view>
          <text class="pair__role">{{ sharedMode ? '另一方的型号' : 'TA 的能量' }}</text>
          <text class="pair__name">{{ taInfo.id }}号 · {{ taInfo.name }}</text>
        </view>
      </view>

      <view class="insight nx-panel insight--bond">
        <view class="insight__icon insight__icon--bond" aria-hidden="true">
          <view class="insight__mark insight__mark--bond" />
        </view>
        <view class="insight__content">
          <text class="insight__eyebrow">PAIR NOTES · 组合解读</text>
          <text class="insight__title">{{ analysis.pairInsight.theme }}</text>
          <text class="insight__text">{{ analysis.pairInsight.bridge }}</text>
          <text class="source-caption">参考 Enneagram Institute 对本组型号的关系解读</text>
        </view>
      </view>

      <view class="detail-section nx-panel">
        <text class="drive__eyebrow">01 · UNDERSTAND EACH OTHER</text>
        <text class="drive__title">在关系里，各自在意什么</text>
        <view v-for="(item, index) in analysis.needs" :key="index" class="need-card">
          <text class="need-card__name">{{ sharedMode ? (index === 0 ? '一方' : '另一方') : item.role }} · {{ item.typeId }}号 {{ item.name }}</text>
          <text class="detail-copy">{{ item.need }}</text>
          <view class="need-card__support"><text class="detail-label">这样支持更贴心</text><text class="detail-copy">{{ item.support }}</text></view>
          <text class="stress-note">压力时，留意：{{ item.stress }}</text>
        </view>
      </view>

      <view class="detail-section nx-panel">
        <text class="drive__eyebrow">02 · YOUR STRENGTHS</text>
        <text class="drive__title">让这段关系发光的地方</text>
        <view v-for="(item, index) in analysis.strengths" :key="index" class="detail-row">
          <text class="detail-row__number">0{{ index + 1 }}</text>
          <view class="detail-row__body"><text class="detail-label">{{ item.title }}</text><text class="detail-copy">{{ item.text }}</text></view>
        </view>
      </view>

      <view class="insight nx-panel insight--friction">
        <view class="insight__icon insight__icon--friction" aria-hidden="true">
          <view class="insight__mark insight__mark--friction" />
        </view>
        <view class="insight__content">
          <text class="insight__eyebrow">FRICTION POINT</text>
          <text class="insight__title">潜在摩擦</text>
          <text class="insight__text">{{ analysis.pairInsight.friction }}</text>
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

      <view class="detail-section scene-panel nx-panel">
        <text class="drive__eyebrow">IN REAL LIFE · 情境练习</text>
        <text class="drive__title">{{ analysis.pairInsight.scene.title }}</text>
        <text class="source-caption">结合这组特点设计的练习情境</text>
        <view class="scene-step"><text class="detail-label">这样的时刻，熟悉吗</text><text class="detail-copy">{{ analysis.pairInsight.scene.situation }}</text></view>
        <view class="scene-step"><text class="detail-label">下一次，可以一起这样试</text><text class="detail-copy">{{ analysis.pairInsight.scene.tryThis }}</text></view>
        <view class="scene-question"><text class="detail-label">留给你们的一个问题</text><text class="dialogue-card__text">{{ analysis.pairInsight.scene.question }}</text></view>
      </view>

      <view class="detail-section nx-panel">
        <text class="drive__eyebrow">03 · WORDS THAT CONNECT</text>
        <text class="drive__title">换一种说法，让对方听见</text>
        <text class="section-intro">遇到分歧时，可以借用下面的话，再换成你们自己的表达。</text>
        <view v-for="(item, index) in analysis.dialogue" :key="index" class="dialogue-card">
          <text class="detail-label">{{ index === 0 ? (sharedMode ? '一方对另一方说' : item.role) : (sharedMode ? '另一方对一方说' : item.role) }}</text>
          <text class="dialogue-card__text">“{{ item.text }}”</text>
        </view>
      </view>

      <view class="drive nx-panel">
        <view class="drive__head">
          <text class="drive__eyebrow">INNER DRIVE</text>
          <text class="drive__title">各自的核心驱动</text>
        </view>
        <view class="drive-pair">
          <view class="drive-card drive-card--mine">
            <text class="drive-card__label">{{ sharedMode ? '一方的驱动力' : '我的驱动力' }}</text>
            <text class="drive-card__text">{{ analysis.myDrive }}</text>
          </view>
          <view class="drive-card drive-card--ta">
            <text class="drive-card__label">{{ sharedMode ? '另一方的驱动力' : 'TA 的驱动力' }}</text>
            <text class="drive-card__text">{{ analysis.taDrive }}</text>
          </view>
        </view>
      </view>

      <view class="detail-section nx-panel">
        <text class="drive__eyebrow">04 · LITTLE STEPS, TOGETHER</text>
        <text class="drive__title">从今天开始，一起试三件事</text>
        <view v-for="(item, index) in analysis.practices" :key="index" class="detail-row">
          <text class="practice-number">{{ index + 1 }}</text>
          <view class="detail-row__body"><text class="detail-label">{{ item.title }}</text><text class="detail-copy">{{ item.text }}</text></view>
        </view>
      </view>

      <view class="detail-section nx-panel">
        <text class="drive__eyebrow">MAKE ROOM FOR EACH OTHER</text>
        <text class="drive__title">有分歧时，也能好好说话</text>
        <text class="section-intro">从 Gottman Institute 的沟通方法中整理四个练习，任选一个，从下一次对话开始。</text>
        <button class="detail-toggle" :aria-expanded="toolsExpanded" @click="toolsExpanded = !toolsExpanded">{{ toolsExpanded ? '收起沟通工具' : '展开 4 个沟通工具' }}<text aria-hidden="true">{{ toolsExpanded ? '−' : '+' }}</text></button>
        <view v-if="toolsExpanded">
          <view v-for="item in analysis.communicationTools" :key="item.id" class="tool-card">
            <text class="detail-label">{{ item.title }}</text>
            <text class="detail-copy">{{ item.text }}</text>
            <text class="tool-example">{{ item.example }}</text>
          </view>
          <text class="source-caption">方法主要来自伴侣沟通资料；其他关系可按双方角色调整表达。以上话术为原创练习。</text>
        </view>
      </view>

      <view class="detail-section source-panel nx-panel">
        <text class="drive__eyebrow">READ & REFLECT</text>
        <text class="drive__title">这份解读，参考了什么</text>
        <text class="section-intro">九型部分是理解关系的理论视角；情境与练习由我们结合资料整理。可核对出处，再和真实经历对照。</text>
        <button class="detail-toggle" :aria-expanded="sourcesExpanded" @click="sourcesExpanded = !sourcesExpanded">{{ sourcesExpanded ? '收起资料来源' : '查看 4 条资料来源' }}<text aria-hidden="true">{{ sourcesExpanded ? '−' : '+' }}</text></button>
        <view v-if="sourcesExpanded">
          <view v-for="source in analysis.sources" :key="source.id" class="source-item">
            <text class="source-kind">{{ source.kind }}</text>
            <text class="source-publisher">{{ source.publisher }}</text>
            <text class="source-title">{{ source.title }}</text>
            <text class="source-note">{{ source.note }}</text>
            <!-- #ifdef H5 -->
            <a class="source-link" :href="source.url" target="_blank" rel="noopener noreferrer">阅读原文 ↗</a>
            <!-- #endif -->
            <!-- #ifdef MP-WEIXIN -->
            <button v-if="!timelinePreview" class="source-copy" @click="copySourceLink(source.id)">复制原文链接</button>
            <!-- #endif -->
          </view>
          <text class="source-caption">资料核对：2026年10月2日</text>
          <!-- #ifdef MP-WEIXIN -->
          <text class="source-caption">{{ timelinePreview ? '进入小程序后可复制原文链接。' : '复制原文链接后，可在浏览器中阅读。' }}</text>
          <!-- #endif -->
        </view>
      </view>

      <view class="share-panel nx-panel">
        <text class="drive__title">把这份理解，分享给在意的人</text>
        <text class="section-intro">{{ myType }}号 × {{ taType }}号 · 对方打开后，可以看到这组完整的合盘解读。</text>
        <NxShareActions />
        <!-- #ifdef H5 -->
        <text class="share-web-note">在微信小程序中打开，可分享给好友或朋友圈。</text>
        <!-- #endif -->
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
      <text class="disclaimer">型号只是理解彼此的起点。请结合真实经历交流，关系的可能性由你们共同创造。</text>
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
  position: relative;
  box-sizing: border-box;
  min-height: 300rpx;
  padding: 38rpx 34rpx 36rpx;
  overflow: hidden;
  border-radius: 38rpx;
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
  position: relative;
  width: calc((100% - 32rpx) / 3);
  height: 204rpx;
  min-height: 128rpx;
  margin: 0;
  padding: 14rpx 8rpx 16rpx;
  border-radius: 22rpx;
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
  border-color: var(--nx-brand-700);
  background: #F5EEE3;
  color: var(--nx-brand-900);
  box-shadow: inset 0 0 0 1rpx var(--nx-brand-700);
}
.type-chip--pressed { opacity: .82; transform: scale(.98); }
.type-chip__figure { position: relative; display: flex; align-items: center; justify-content: center; width: 140rpx; max-width: 100%; height: 128rpx; flex-shrink: 0; }
.type-chip__figure::before { content: ''; position: absolute; bottom: 0; left: 18%; width: 64%; height: 12rpx; border-radius: 50%; background: rgba(143, 118, 82, .1); }
.type-chip__portrait { position: relative; display: block; width: 132rpx; max-width: 100%; height: 128rpx; }
.type-chip__fallback { position: relative; font-size: 64rpx; font-weight: 700; color: var(--nx-brand-700); }
.type-chip__caption { display: flex; align-items: baseline; justify-content: center; gap: 8rpx; width: 100%; margin-top: 12rpx; white-space: nowrap; line-height: 1.3; }
.type-chip__number { color: var(--nx-text-muted); font-size: 22rpx; font-weight: 500; }
.type-chip__name { font-size: 25rpx; font-weight: 700; }
.type-chip__selected { position: absolute; top: 8rpx; right: 8rpx; padding: 4rpx 8rpx; border-radius: 8rpx; background: var(--nx-brand-700); color: var(--nx-surface); font-size: 18rpx; font-weight: 600; line-height: 1.3; }
.analyze--pressed, .reset--pressed { opacity: .84; transform: scale(.985); }

.pair {
  position: relative;
  box-sizing: border-box;
  min-height: 330rpx;
  padding: 34rpx 30rpx;
  overflow: hidden;
  border-radius: 38rpx;
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
.pair__avatar-action {
  display: block;
  flex: 0 0 112rpx;
  width: 112rpx;
  height: 112rpx;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 0;
  border-radius: 50%;
  background: transparent;
  line-height: 1;
}
.pair__avatar-action::after { border: 0; }
.pair__avatar-action--pressed { opacity: .82; }
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
.pair-connection__symbol { color: var(--nx-accent-gold); font-family: Georgia, serif; font-size: 72rpx; line-height: 1; margin-top: 9rpx; }
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

.result-heading { padding: 12rpx 6rpx 4rpx; }
.result-heading .relation-hero__eyebrow { color: var(--nx-brand-700); font-size: 21rpx; letter-spacing: 2rpx; }
.result-heading__title { display: block; margin-top: 12rpx; font-size: 36rpx; font-weight: 800; line-height: 1.5; color: var(--nx-text); }
.result-heading__desc, .section-intro { display: block; margin-top: 12rpx; font-size: 25rpx; line-height: 1.75; color: var(--nx-text-muted); }
.detail-section { background: var(--nx-surface); border-color: var(--nx-border); }
.detail-section .drive__eyebrow { font-size: 21rpx; letter-spacing: 1rpx; }
.need-card { margin-top: 24rpx; padding: 24rpx; border: 1rpx solid var(--nx-border); border-radius: 20rpx; background: var(--nx-surface-soft); }
.need-card__name { display: block; color: var(--nx-brand-700); font-size: 27rpx; font-weight: 700; line-height: 1.5; }
.need-card__support { margin-top: 20rpx; padding-top: 20rpx; border-top: 1rpx solid var(--nx-border); }
.detail-copy { display: block; margin-top: 10rpx; font-size: 26rpx; color: var(--nx-text); line-height: 1.85; overflow-wrap: break-word; }
.detail-label { display: block; color: var(--nx-brand-700); font-size: 26rpx; font-weight: 700; line-height: 1.6; }
.stress-note { display: block; margin-top: 20rpx; color: var(--nx-text-muted); font-size: 24rpx; line-height: 1.75; }
.detail-row { display: flex; align-items: flex-start; gap: 20rpx; margin-top: 26rpx; padding-top: 24rpx; border-top: 1rpx solid var(--nx-border); }
.detail-row__number { flex: 0 0 40rpx; font-family: Georgia, serif; font-size: 32rpx; color: var(--nx-brand-700); line-height: 1.5; }
.detail-row__body { flex: 1; min-width: 0; }
.dialogue-card { margin-top: 22rpx; padding: 24rpx; border-radius: 20rpx; background: var(--nx-surface-soft); border-left: 4rpx solid var(--nx-accent-gold); }
.dialogue-card__text { display: block; margin-top: 12rpx; color: var(--nx-text); font-size: 28rpx; line-height: 1.9; }
.practice-number { display: flex; flex: 0 0 48rpx; height: 48rpx; align-items: center; justify-content: center; border-radius: 50%; background: var(--nx-surface-soft); color: var(--nx-brand-700); font-size: 25rpx; font-weight: 700; }
.share-panel { background: #F5EEE3; border-color: var(--nx-border); }
.share-web-note, .route-note { display: block; margin-top: 18rpx; padding: 20rpx 24rpx; color: var(--nx-brand-700); background: var(--nx-surface-soft); border-radius: 16rpx; font-size: 24rpx; line-height: 1.7; }
.source-caption { display: block; margin-top: 18rpx; color: var(--nx-text-muted); font-size: 22rpx; line-height: 1.7; }
.scene-panel { border-top: 5rpx solid var(--nx-accent-gold); }
.scene-step { margin-top: 26rpx; }
.scene-question { margin-top: 26rpx; padding: 24rpx; border-radius: 18rpx; background: #F5EEE3; }
.detail-toggle { display: flex; align-items: center; justify-content: space-between; width: 100%; min-height: 80rpx; margin: 22rpx 0 0; padding: 16rpx 20rpx; border: 1rpx solid var(--nx-border); border-radius: 14rpx; color: var(--nx-brand-700); background: var(--nx-surface-soft); font-size: 25rpx; text-align: left; line-height: 1.7; }
.detail-toggle::after, .source-copy::after { border: 0; }
.tool-card, .source-item { margin-top: 24rpx; padding-top: 24rpx; border-top: 1rpx solid var(--nx-border); }
.tool-example { display: block; margin-top: 16rpx; padding: 20rpx; border-radius: 14rpx; background: var(--nx-surface-soft); color: var(--nx-text); font-size: 25rpx; line-height: 1.8; }
.source-kind { display: inline-block; padding: 4rpx 12rpx; border-radius: 8rpx; background: #F5EEE3; color: var(--nx-brand-700); font-size: 21rpx; line-height: 1.6; }
.source-publisher { display: block; margin-top: 12rpx; color: var(--nx-text); font-size: 25rpx; font-weight: 700; line-height: 1.6; overflow-wrap: anywhere; }
.source-title, .source-note { display: block; margin-top: 10rpx; color: var(--nx-text-muted); font-size: 23rpx; line-height: 1.75; overflow-wrap: anywhere; }
.source-link { display: inline-block; margin-top: 14rpx; padding: 12rpx 0; color: var(--nx-brand-700); font-size: 24rpx; text-decoration: underline; }
.source-copy { display: inline-block; min-height: 72rpx; margin: 16rpx 0 0; padding: 12rpx 20rpx; border: 1rpx solid var(--nx-border); border-radius: 12rpx; color: var(--nx-brand-700); background: var(--nx-surface-soft); font-size: 23rpx; line-height: 1.8; }

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
