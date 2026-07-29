<script setup>
import { ref, onMounted, computed, getCurrentInstance } from 'vue'
import { onShareAppMessage, onShareTimeline } from '@dcloudio/uni-app'
import { TYPES_INFO, CENTERS, RESULTS } from '../../data/enneagramGame'
import { isWing } from '../../utils/enneagram'
import { resultPersonaText } from '../../utils/resultPersona'
import { getLastResult, normalizeLastResult } from '../../utils/session'
import { ensureLogin } from '../../utils/auth'
import { listClassroomStandaloneApi, saveTestRecordApi, reportStatusApi, reportContentApi } from '../../api'
import { payForReport } from '../../utils/payment'
import { userErrorMessage } from '../../utils/userMessage'
import { reportDisplayState } from '../../utils/reportDisplayState'
import { createResultPoster } from '../../utils/resultPoster'
import {
  classroomAccessLabel,
  classroomContentRoute,
  normalizeClassroomContent,
} from '../../utils/classroomDisplay'
import { setBookingIntent } from '../../utils/bookingIntent'

const result = ref(null)
const gender = ref(null)
const r = ref(null)
const info = ref(null)
const center = ref(null)
const persona = ref('')
const secondInfo = ref(null)
const wing = ref(false)
const growthInfo = ref(null)
const stressInfo = ref(null)
const saved = ref(false)
const saving = ref(false)
const recordId = ref('')
// 深度报告解锁
const reportUnlocked = ref(false)
const reportPriceCents = ref(null)
const reportStatusLoading = ref(false)
const reportStatusError = ref('')
const reportContent = ref('')
const reportLoading = ref(false)
const reportError = ref('')
const paying = ref(false)
const posterUrl = ref('')
const posterShow = ref(false)
const posterLoading = ref(false)
const posterError = ref('')
const avatarFailed = ref(false)
const instance = getCurrentInstance()
const classroomRecommendations = ref([])
const classroomRecommendationLoading = ref(false)
const classroomRecommendationError = ref('')
let classroomRecommendationPromise = null

const reportState = computed(() => reportDisplayState({
  recordId: recordId.value,
  loading: reportStatusLoading.value,
  error: reportStatusError.value,
  unlocked: reportUnlocked.value,
  priceCents: reportPriceCents.value,
}))

onMounted(() => {
  const last = getLastResult()
  const cachedResult = normalizeLastResult(last.result)
  if (!cachedResult) {
    uni.showToast({ title: '测试结果已失效，请重新测试', icon: 'none' })
    uni.redirectTo({ url: '/pages/test/test' })
    return
  }
  result.value = cachedResult
  gender.value = last.gender
  const t = cachedResult.type
  r.value = RESULTS[t]
  info.value = TYPES_INFO[t]
  center.value = CENTERS[TYPES_INFO[t].center]
  persona.value = resultPersonaText(RESULTS[t], last.gender)
  secondInfo.value = cachedResult.second ? TYPES_INFO[cachedResult.second] : null
  wing.value = isWing(t, cachedResult.second)
  growthInfo.value = TYPES_INFO[TYPES_INFO[t].growth]
  stressInfo.value = TYPES_INFO[TYPES_INFO[t].stress]
  loadClassroomRecommendations()
})

function loadClassroomRecommendations() {
  if (classroomRecommendationPromise) return classroomRecommendationPromise

  classroomRecommendationLoading.value = true
  classroomRecommendationError.value = ''
  classroomRecommendationPromise = listClassroomStandaloneApi({ limit: 2, offset: 0 })
    .then((response) => {
      classroomRecommendations.value = (Array.isArray(response?.items) ? response.items : [])
        .map(normalizeClassroomContent)
        .filter((item) => item.id)
        .slice(0, 2)
      return classroomRecommendations.value
    })
    .catch((error) => {
      classroomRecommendations.value = []
      classroomRecommendationError.value = error?.message || '课堂推荐暂未加载'
      return []
    })
    .finally(() => {
      classroomRecommendationLoading.value = false
      classroomRecommendationPromise = null
    })
  return classroomRecommendationPromise
}

async function saveRecord() {
  if (saving.value || saved.value) return
  saving.value = true
  try {
    await ensureLogin()
    const rec = await saveTestRecordApi({
      gender: gender.value,
      resultType: result.value.type,
      secondType: result.value.second || 0,
      score: result.value.score,
      centers: result.value.centers,
    })
    if (!rec || !rec.id) throw new Error('存档失败，请重试')
    recordId.value = rec.id
    saved.value = true
    uni.showToast({ title: '已存入我的档案', icon: 'success' })
    await refreshReportStatus()
  } catch (e) {
    uni.showToast({ title: userErrorMessage(e, '存档失败，请重试'), icon: 'none' })
  } finally {
    saving.value = false
  }
}

async function refreshReportStatus() {
  if (!recordId.value) return
  reportStatusLoading.value = true
  reportStatusError.value = ''
  reportPriceCents.value = null
  try {
    const st = await reportStatusApi(recordId.value)
    reportUnlocked.value = !!st.unlocked
    if (reportUnlocked.value) {
      loadReportContent()
    } else if (Number.isFinite(st.priceCents) && st.priceCents > 0) {
      reportPriceCents.value = st.priceCents
    } else {
      reportStatusError.value = '暂时无法获取报告价格，请稍后重试'
    }
  } catch (e) {
    reportStatusError.value = userErrorMessage(e, '报告状态查询失败，请重试')
  } finally {
    reportStatusLoading.value = false
  }
}

async function loadReportContent() {
  if (reportLoading.value || reportContent.value) return
  reportLoading.value = true
  reportError.value = ''
  try {
    const ans = await reportContentApi(recordId.value)
    reportContent.value = (ans && ans.answer) || ''
  } catch (e) {
    reportError.value = userErrorMessage(e, '报告生成中，请稍后重试')
    uni.showToast({ title: reportError.value, icon: 'none' })
  } finally {
    reportLoading.value = false
  }
}

async function unlockReport() {
  if (paying.value) return
  paying.value = true
  try {
    await ensureLogin()
    if (!recordId.value) {
      await saveRecord()
    }
    if (!recordId.value) {
      uni.showToast({ title: '请先存档再解锁', icon: 'none' })
      return
    }
    await payForReport(recordId.value)
    uni.showToast({ title: '解锁成功', icon: 'success' })
    reportUnlocked.value = true
    loadReportContent()
  } catch (e) {
    uni.showToast({ title: userErrorMessage(e, '支付失败，请重试'), icon: 'none' })
  } finally {
    paying.value = false
  }
}

const reportPriceYuan = computed(() => {
  if (Number.isFinite(reportPriceCents.value) && reportPriceCents.value > 0) {
    return (reportPriceCents.value / 100).toFixed(2)
  }
  return ''
})

function goBooking() {
  setBookingIntent({ kind: 'enterprise', intentText: '企业九型工作坊' })
  uni.switchTab({ url: '/pages/booking/booking' })
}

function goClassroom() {
  uni.switchTab({ url: '/pages/learn/learn' })
}

function openClassroomRecommendation(item) {
  const url = classroomContentRoute(item)
  if (url) uni.navigateTo({ url })
}
function restart() {
  uni.redirectTo({ url: '/pages/test/test' })
}

function goRelation() {
  uni.navigateTo({ url: `/pages/relation/relation?type=${result.value.type}` })
}

function resultShareImage(type) {
  const normalizedType = Number(type)
  if (!Number.isInteger(normalizedType) || normalizedType < 1 || normalizedType > 9) {
    return '/static/share/result-default.jpg'
  }
  return `/static/share/result-${normalizedType}.jpg`
}

// 微信好友转发
onShareAppMessage(() => ({
  title: `我是 ${result.value?.type} 号「${r.value?.title}」｜你是哪一型？`,
  path: '/pages/index/index',
  imageUrl: resultShareImage(result.value?.type),
}))
// 朋友圈分享
onShareTimeline(() => ({
  title: `九型芯之力｜我是 ${result.value?.type} 号「${r.value?.title}」`,
  query: '',
  imageUrl: resultShareImage(result.value?.type),
}))

// 生成分享海报（canvas 2d）
async function makePoster() {
  if (posterLoading.value) return
  posterLoading.value = true
  posterShow.value = true
  posterError.value = ''
  posterUrl.value = ''
  try {
    posterUrl.value = await createResultPoster({
      instance,
      result: result.value,
      info: info.value,
      summary: r.value.summary,
      title: r.value.title,
    })
  } catch (e) {
    posterError.value = userErrorMessage(e, '海报生成失败，请重试')
  } finally {
    posterLoading.value = false
  }
}

function savePoster() {
  if (!posterUrl.value) return
  uni.saveImageToPhotosAlbum({
    filePath: posterUrl.value,
    success: () => uni.showToast({ title: '已保存到相册', icon: 'success' }),
    fail: () => uni.showToast({ title: '保存失败，请允许相册权限', icon: 'none' }),
  })
}

</script>

<template>
  <view class="wrap page-stack ios-page ios-safe-bottom result-page" v-if="result">
    <view class="result-hero nx-page-hero" :class="`result-hero--${info.color}`">
      <view class="result-hero__avatar-wrap">
        <image v-if="!avatarFailed" class="result-hero__avatar" :src="`/static/avatars/${result.type}.png`" mode="aspectFill" lazy-load @error="avatarFailed = true" />
        <view v-else class="result-hero__avatar-fallback">{{ result.type }}</view>
        <view class="result-hero__number">{{ result.type }}</view>
      </view>
      <text class="result-hero__eyebrow">你的核心人格</text>
      <text class="result-hero__title">{{ r.title }}</text>
      <text class="result-hero__meta">{{ info.en }} · {{ info.keywords }}</text>
      <text class="result-hero__summary">{{ r.summary }}</text>
      <view class="result-hero__persona">{{ persona }}</view>
    </view>

    <view class="drive-grid">
      <view class="drive-card drive-card--fear">
        <text class="drive-card__label">基本恐惧</text>
        <text class="drive-card__text">{{ info.fear }}</text>
      </view>
      <view class="drive-card drive-card--desire">
        <text class="drive-card__label">核心欲望</text>
        <text class="drive-card__text">{{ info.desire }}</text>
      </view>
    </view>

    <view class="center-panel nx-panel ios-card">
      <view class="nx-section-head"><text class="section-title">你的三中心分布</text><text class="section-caption">能量偏好</text></view>
      <view v-for="c in result.centers" :key="c.key" class="center-row">
        <view class="center-row__meta"><text class="center-row__name">{{ c.name }}</text><text class="center-row__pct">{{ c.pct }}%</text></view>
        <view class="center-row__track"><view class="center-row__fill" :class="`center-row__fill--${c.key}`" :style="{ width: c.pct + '%' }" /></view>
      </view>
    </view>

    <view v-if="secondInfo" class="secondary-panel nx-panel ios-card">
      <text class="section-kicker">{{ wing ? '侧翼能量' : '副型能量' }}</text>
      <text class="section-title">{{ wing ? '你的侧翼倾向' : '你的副型倾向' }}</text>
      <text class="secondary-panel__text">主型 {{ result.type }} 号 {{ info.name }}，副型 {{ result.second }} 号 {{ secondInfo.name }} 特质也很突出，让你更立体。</text>
      <text class="secondary-panel__keywords">{{ secondInfo.keywords }}</text>
    </view>

    <view class="direction-grid">
      <view class="direction-card direction-card--stress">
        <text class="direction-card__label">压力方向</text>
        <text class="direction-card__type">{{ info.stress }} 号 · {{ stressInfo.name }}</text>
        <text class="direction-card__hint">看见紧绷时的自动反应</text>
      </view>
      <view class="direction-card direction-card--growth">
        <text class="direction-card__label">成长方向</text>
        <text class="direction-card__type">{{ info.growth }} 号 · {{ growthInfo.name }}</text>
        <text class="direction-card__hint">练习更完整的表达方式</text>
      </view>
    </view>

    <view class="growth-insight nx-panel ios-card">
      <text class="section-kicker">今日成长提示</text>
      <text class="section-title">从觉察开始改变</text>
      <text class="growth-insight__text">{{ r.growth }}</text>
    </view>

    <view class="report-panel">
      <view class="report__head">
        <view><text class="report__eyebrow">PERSONAL INSIGHT</text><text class="report__title">AI 深度性格报告</text></view>
        <text v-if="reportState.key === 'unlocked'" class="report__badge">已解锁</text>
      </view>

      <template v-if="reportState.key === 'needs-save'">
        <text class="report__intro">先将本次结果存入成长档案，再查询专属报告价格。</text>
        <!-- #ifdef H5 -->
        <button class="report__cta report__cta--disabled" disabled>请在微信小程序内登录后保存</button>
        <!-- #endif -->
        <!-- #ifdef MP-WEIXIN -->
        <button class="report__cta" :loading="saving" :disabled="saving" @click="saveRecord">{{ saving ? '正在存档' : '存入档案并查看价格' }}</button>
        <!-- #endif -->
      </template>
      <template v-else-if="reportState.key === 'status-loading'">
        <view class="report__status" aria-live="polite">查询报告状态中，请稍候</view>
      </template>
      <template v-else-if="reportState.key === 'status-error'">
        <text class="report__error">{{ reportStatusError || '暂时无法获取报告状态，请稍后重试' }}</text>
        <!-- #ifdef MP-WEIXIN -->
        <button class="report__secondary report__retry" :disabled="reportStatusLoading" @click="refreshReportStatus">重新查询</button>
        <!-- #endif -->
      </template>
      <template v-else-if="reportState.key === 'ready'">
        <view class="report__price"><text class="report__price-symbol">￥</text>{{ reportPriceYuan }}</view>
        <text class="report__intro">结合你的核心动力、压力模式与成长方向，生成更完整的个性化解读。</text>
        <!-- #ifdef H5 -->
        <button class="report__cta report__cta--disabled" disabled>请在微信小程序内完成存档与支付</button>
        <!-- #endif -->
        <!-- #ifdef MP-WEIXIN -->
        <button class="report__cta" :loading="paying" :disabled="paying" @click="unlockReport">￥{{ reportPriceYuan }} 解锁深度报告</button>
        <!-- #endif -->
      </template>
      <template v-else>
        <view v-if="reportLoading" class="report__status" aria-live="polite">报告生成中，请稍候</view>
        <view v-else-if="reportError" class="report__content-error">
          <text>{{ reportError }}</text>
          <button class="report__secondary report__content-retry" :disabled="reportLoading" @click="loadReportContent">重试生成</button>
        </view>
        <text v-else-if="reportContent" class="report__content">{{ reportContent }}</text>
        <button v-else class="report__secondary" @click="loadReportContent">查看报告</button>
      </template>
    </view>

    <view class="result-recommendations nx-panel ios-card">
      <view class="result-recommendations__head">
        <view>
          <text class="result-recommendations__eyebrow">老师课堂</text>
          <text class="result-recommendations__title">测完后继续学一学</text>
        </view>
        <button class="result-recommendations__more" @click="goClassroom">继续浏览老师课堂</button>
      </view>
      <view v-if="classroomRecommendationLoading" class="result-recommendations__state" aria-live="polite">
        正在整理适合继续学习的课件…
      </view>
      <view v-else-if="classroomRecommendations.length" class="result-recommendations__list">
        <button
          v-for="item in classroomRecommendations"
          :key="item.id"
          class="result-recommendation-card"
          :aria-label="`查看${item.title || '老师课堂课件'}`"
          @click="openClassroomRecommendation(item)"
        >
          <view class="result-recommendation-card__meta">
            <text>{{ item.contentType === 'audio' ? '音频' : '视频' }}</text>
            <text>{{ classroomAccessLabel(item.effectiveAccess) }}</text>
          </view>
          <text class="result-recommendation-card__title">{{ item.title || '未命名课件' }}</text>
          <text v-if="item.description" class="result-recommendation-card__description">{{ item.description }}</text>
        </button>
      </view>
      <text v-else class="result-recommendations__state">
        {{ classroomRecommendationError ? '推荐课件暂未同步，可以先进入老师课堂查看全部内容。' : '老师正在整理更多视频与音频内容，稍后再来看看。' }}
      </text>
    </view>

    <view class="result-actions">
      <!-- #ifdef MP-WEIXIN -->
      <view class="result-actions__share-row">
        <button class="result-actions__secondary" open-type="share">分享好友</button>
        <button class="result-actions__secondary" :loading="posterLoading" :disabled="posterLoading" @click="makePoster">生成海报</button>
      </view>
      <!-- #endif -->
      <!-- #ifdef H5 -->
      <button class="result-actions__secondary" disabled>小程序内生成海报</button>
      <!-- #endif -->
      <button class="result-actions__relation" @click="goRelation">和 TA 合盘 · 看关系</button>
      <button class="result-actions__booking" @click="goBooking">预约企业九型工作坊</button>
      <button class="restart-button" @click="restart">重新测试</button>
    </view>
    <text class="disclaimer">本测试基于九型人格体系简化设计，仅供趣味参考，不作专业诊断。</text>

    <!-- 离屏 canvas 用于绘制海报 -->
    <canvas id="poster-canvas" type="2d" class="poster-canvas"></canvas>

    <!-- 海报弹层 -->
    <view v-if="posterShow" class="poster-mask" @click="posterShow = false">
      <view class="poster-box" role="dialog" aria-modal="true" @click.stop>
        <view v-if="posterLoading" class="poster-loading" aria-live="polite">海报生成中…</view>
        <image v-else-if="posterUrl" class="poster-img" :src="posterUrl" mode="widthFix" show-menu-by-longpress />
        <view v-else-if="posterError" class="poster-error" aria-live="polite">
          <text>{{ posterError }}</text>
          <button class="btn-primary ios-button" :disabled="posterLoading" @click="makePoster">重新生成</button>
        </view>
        <view class="poster-ops">
          <button v-if="posterUrl && !posterLoading" class="btn-primary ios-button" @click="savePoster">保存到相册</button>
          <button class="btn-ghost ios-button" aria-label="关闭海报" @click="posterShow = false">关闭</button>
        </view>
        <text v-if="posterUrl" class="poster-tip">也可长按图片转发给好友</text>
      </view>
    </view>
  </view>
</template>

<style scoped>
.result-page {
  gap: 24rpx;
}
.report__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  margin-bottom: 16rpx;
}
.report__badge {
  min-height: 44rpx;
  display: inline-flex;
  align-items: center;
  background: rgba(249,115,22,.12);
  color: #ea580c;
  font-size: 24rpx;
  font-weight: 900;
  padding: 0 18rpx;
  border-radius: 999rpx;
  flex-shrink: 0;
}
.report__intro {
  color: #475569;
  font-size: 26rpx;
  line-height: 1.72;
  display: block;
  margin-bottom: 22rpx;
}
.report__content {
  color: #1e293b;
  font-size: 28rpx;
  line-height: 1.85;
  white-space: pre-wrap;
  display: block;
}
.report__retry { margin-top: 18rpx; }
.report__error { color: #b45309; }
.disclaimer {
  color: #64748b;
  font-size: 22rpx;
  text-align: center;
  margin-top: 10rpx;
  line-height: 1.6;
}
.poster-canvas { position: fixed; left: -9999rpx; top: -9999rpx; width: 320px; height: 460px; }
.poster-mask {
  position: fixed;
  inset: 0;
  padding: calc(28rpx + env(safe-area-inset-top)) 28rpx calc(28rpx + env(safe-area-inset-bottom));
  background: rgba(15,23,42,.58);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 99;
  box-sizing: border-box;
}
.poster-box {
  width: 620rpx;
  max-width: 100%;
  background: rgba(255,255,255,.96);
  border-radius: 30rpx;
  padding: 28rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 18rpx;
  box-sizing: border-box;
}
.poster-loading { padding: 80rpx 0; color: #64748b; font-size: 28rpx; }
.poster-error { width: 100%; padding: 48rpx 0 24rpx; color: #b45309; font-size: 26rpx; line-height: 1.6; text-align: center; }
.poster-error .btn-primary { margin-top: 24rpx; }
.poster-img { width: 100%; border-radius: 18rpx; }
.poster-ops { display: flex; gap: 16rpx; width: 100%; }
.poster-ops .btn-primary,
.poster-ops .btn-ghost { flex: 1; }
.poster-tip { color: #94a3b8; font-size: 22rpx; }

.result-hero {
  min-height: 520rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14rpx;
  padding: 42rpx 34rpx 36rpx;
  border-radius: 38rpx;
  text-align: center;
  color: #fff;
  box-sizing: border-box;
}
.result-hero--blue { background: linear-gradient(145deg, #1d4ed8, #4338ca); }
.result-hero--green { background: linear-gradient(145deg, #047857, #0f766e); }
.result-hero--red { background: linear-gradient(145deg, #be123c, #c2410c); }
.result-hero__avatar-wrap { position: relative; width: 184rpx; height: 184rpx; }
.result-hero__avatar {
  width: 184rpx;
  height: 184rpx;
  border-radius: 50%;
  border: 6rpx solid rgba(255, 255, 255, .92);
  box-sizing: border-box;
}
.result-hero__avatar-fallback {
  width: 184rpx;
  height: 184rpx;
  border-radius: 50%;
  border: 6rpx solid rgba(255, 255, 255, .92);
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, .18);
  color: #fff;
  font-size: 72rpx;
  font-weight: 900;
  box-sizing: border-box;
}
.result-hero__number {
  position: absolute;
  right: -2rpx;
  bottom: 6rpx;
  width: 60rpx;
  height: 60rpx;
  border-radius: 21rpx;
  background: #fff;
  color: #1e293b;
  font-size: 30rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}
.result-hero__eyebrow { color: rgba(255, 255, 255, .92); font-size: 24rpx; font-weight: 800; }
.result-hero__title { color: #fff; font-size: 46rpx; font-weight: 900; line-height: 1.22; }
.result-hero__meta { color: rgba(255, 255, 255, .94); font-size: 24rpx; font-weight: 800; }
.result-hero__summary { color: #fff; font-size: 28rpx; line-height: 1.68; }
.result-hero__persona {
  margin-top: 8rpx;
  padding: 24rpx 26rpx;
  border-radius: 24rpx;
  background: rgba(15, 23, 42, .24);
  border: 2rpx solid rgba(255, 255, 255, .22);
  color: #fff;
  font-size: 26rpx;
  font-weight: 800;
  line-height: 1.66;
}
.drive-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16rpx;
}
.direction-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16rpx;
}
.drive-card,
.direction-card { min-width: 0; border-radius: 24rpx; padding: 24rpx; display: flex; flex-direction: column; gap: 12rpx; box-sizing: border-box; }
.drive-card--fear { background: linear-gradient(145deg, #fff7ed, #ffe4e6); border: 2rpx solid #fdba74; }
.drive-card--desire { background: linear-gradient(145deg, #eff6ff, #ede9fe); border: 2rpx solid #a5b4fc; }
.drive-card__label,
.direction-card__label { font-size: 24rpx; font-weight: 900; }
.drive-card--fear .drive-card__label { color: #9f1239; }
.drive-card--desire .drive-card__label { color: #4338ca; }
.drive-card__text,
.direction-card__type { color: #0f172a; font-size: 26rpx; font-weight: 800; line-height: 1.55; }
.section-title { display: block; color: #0f172a; font-size: 30rpx; font-weight: 900; }
.section-caption,
.section-kicker { color: #475569; font-size: 24rpx; font-weight: 800; }
.center-panel { background: #f8fafc; }
.center-row { margin-top: 22rpx; }
.center-row__meta { display: flex; justify-content: space-between; margin-bottom: 10rpx; }
.center-row__name,
.center-row__pct { color: #334155; font-size: 24rpx; font-weight: 800; }
.center-row__track { height: 16rpx; background: #e2e8f0; border-radius: 999rpx; overflow: hidden; }
.center-row__fill { height: 100%; border-radius: 999rpx; }
.center-row__fill--gut { background: #0f766e; }
.center-row__fill--heart { background: #be123c; }
.center-row__fill--head { background: #1d4ed8; }
.secondary-panel__text,
.growth-insight__text { display: block; margin-top: 16rpx; color: #334155; font-size: 26rpx; line-height: 1.72; }
.secondary-panel__keywords { display: block; margin-top: 12rpx; color: #4338ca; font-size: 24rpx; font-weight: 900; }
.direction-card--stress { background: #fff7ed; border: 2rpx solid #fdba74; }
.direction-card--growth { background: #f0fdfa; border: 2rpx solid #5eead4; }
.direction-card--stress .direction-card__label { color: #9a3412; }
.direction-card--growth .direction-card__label { color: #0f766e; }
.direction-card__hint { color: #475569; font-size: 24rpx; line-height: 1.5; }
.growth-insight { background: #ecfdf5; border-color: #a7f3d0; }
.report-panel {
  background: linear-gradient(135deg, #111827 0%, #312e81 100%);
  border-radius: 34rpx;
  padding: 34rpx;
  color: #fff;
  box-sizing: border-box;
}
.report__eyebrow { display: block; color: rgba(255, 255, 255, .92); font-size: 24rpx; font-weight: 800; margin-bottom: 8rpx; }
.report__title { display: block; color: #fff; font-size: 34rpx; font-weight: 900; }
.report__badge { background: #d1fae5; color: #065f46; }
.report__intro { color: rgba(255, 255, 255, .92); font-size: 26rpx; line-height: 1.72; }
.report__status { color: #fff; font-size: 26rpx; line-height: 1.6; padding: 30rpx 0; text-align: center; }
.report__error { display: block; color: #fecdd3; font-size: 26rpx; line-height: 1.65; margin-bottom: 18rpx; }
.report__price { color: #fff; font-size: 54rpx; font-weight: 900; margin-bottom: 12rpx; }
.report__price-symbol { font-size: 28rpx; }
.report__content { color: rgba(255, 255, 255, .94); font-size: 28rpx; line-height: 1.85; }
.report__content-error { color: #fecdd3; font-size: 26rpx; line-height: 1.65; }
.report__cta,
.report__secondary,
.result-actions button {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 24rpx;
  line-height: 1.2;
}
.report__cta {
  width: 100%;
  min-height: 88rpx;
  border-radius: 22rpx;
  background: #fff;
  color: #9f1239;
  font-size: 28rpx;
  font-weight: 900;
}
.report__secondary {
  width: 100%;
  min-height: 88rpx;
  margin-top: 18rpx;
  border-radius: 22rpx;
  background: transparent;
  color: #fff;
  border: 2rpx solid rgba(255, 255, 255, .82);
  font-size: 28rpx;
  font-weight: 900;
}
.report__cta--disabled { color: #475569; opacity: .78; }
.result-recommendations {
  display: flex;
  flex-direction: column;
  gap: 20rpx;
  padding: 30rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 30rpx;
  background: var(--nx-surface);
}
.result-recommendations__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
}
.result-recommendations__eyebrow {
  display: block;
  color: var(--nx-brand-700);
  font-size: 23rpx;
  font-weight: 900;
  margin-bottom: 8rpx;
}
.result-recommendations__title {
  display: block;
  color: var(--nx-brand-900);
  font-size: 31rpx;
  font-weight: 900;
  line-height: 1.35;
}
.result-recommendations__more {
  flex: none;
  min-height: 88rpx;
  margin: 0;
  padding: 0 22rpx;
  border: 0;
  background: transparent;
  color: var(--nx-brand-700);
  font-size: 23rpx;
  font-weight: 900;
  line-height: 88rpx;
}
.result-recommendations__more::after,
.result-recommendation-card::after { border: none; }
.result-recommendations__list {
  display: flex;
  flex-direction: column;
  gap: 14rpx;
}
.result-recommendations__state {
  color: var(--nx-text-muted);
  font-size: 24rpx;
  line-height: 1.6;
}
.result-recommendation-card {
  width: 100%;
  min-height: 112rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10rpx;
  margin: 0;
  padding: 24rpx;
  border: 2rpx solid var(--nx-border);
  border-radius: 24rpx;
  background: var(--nx-surface-soft);
  color: var(--nx-text);
  text-align: left;
}
.result-recommendation-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12rpx;
  color: var(--nx-brand-700);
  font-size: 21rpx;
  font-weight: 900;
}
.result-recommendation-card__title {
  color: var(--nx-brand-900);
  font-size: 27rpx;
  font-weight: 900;
  line-height: 1.4;
}
.result-recommendation-card__description {
  color: var(--nx-text-muted);
  font-size: 23rpx;
  line-height: 1.5;
}
.result-actions { display: flex; flex-direction: column; gap: 16rpx; margin-top: 8rpx; }
.result-actions button {
  min-height: 88rpx;
  border-radius: 22rpx;
  font-size: 27rpx;
  font-weight: 800;
  box-sizing: border-box;
}
.result-actions__share-row { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16rpx; }
.result-actions__secondary,
.result-actions__booking { background: #fff; color: #334155; border: 2rpx solid #cbd5e1; }
.result-actions__relation { background: var(--nx-brand-900); color: var(--nx-surface); }
.result-actions__booking { border-color: var(--nx-accent-gold); color: var(--nx-brand-900); background: var(--nx-accent-gold); }
.restart-button {
  min-height: 88rpx;
  background: transparent;
  color: #475569;
  border: 0;
}
.disclaimer { color: #475569; font-size: 24rpx; }
.poster-loading { color: #475569; }
.poster-tip { color: #475569; font-size: 24rpx; }
</style>
