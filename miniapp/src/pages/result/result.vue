<script setup>
import { ref, onMounted, computed, getCurrentInstance } from 'vue'
import { onShareAppMessage, onShareTimeline } from '@dcloudio/uni-app'
import { TYPES_INFO, CENTERS, RESULTS } from '../../data/enneagramGame'
import { isWing } from '../../utils/enneagram'
import { getLastResult } from '../../utils/session'
import { ensureLogin } from '../../utils/auth'
import { saveTestRecordApi, reportStatusApi, reportContentApi } from '../../api'
import { payForReport } from '../../utils/payment'

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
const reportPriceCents = ref(990)
const reportContent = ref('')
const reportLoading = ref(false)
const paying = ref(false)
const posterUrl = ref('')
const posterShow = ref(false)
const posterLoading = ref(false)
const instance = getCurrentInstance()

onMounted(() => {
  const last = getLastResult()
  if (!last.result) {
    uni.redirectTo({ url: '/pages/test/test' })
    return
  }
  result.value = last.result
  gender.value = last.gender
  const t = last.result.type
  r.value = RESULTS[t]
  info.value = TYPES_INFO[t]
  center.value = CENTERS[TYPES_INFO[t].center]
  persona.value = last.gender === 'male' ? RESULTS[t].male : RESULTS[t].female
  secondInfo.value = last.result.second ? TYPES_INFO[last.result.second] : null
  wing.value = isWing(t, last.result.second)
  growthInfo.value = TYPES_INFO[TYPES_INFO[t].growth]
  stressInfo.value = TYPES_INFO[TYPES_INFO[t].stress]
})

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
    if (rec && rec.id) {
      recordId.value = rec.id
      refreshReportStatus()
    }
    saved.value = true
    uni.showToast({ title: '已存入我的档案', icon: 'success' })
  } catch (e) {
    uni.showToast({ title: '存档失败，请重试', icon: 'none' })
  } finally {
    saving.value = false
  }
}

async function refreshReportStatus() {
  if (!recordId.value) return
  try {
    const st = await reportStatusApi(recordId.value)
    reportUnlocked.value = !!st.unlocked
    if (typeof st.priceCents === 'number') reportPriceCents.value = st.priceCents
    if (reportUnlocked.value) loadReportContent()
  } catch (e) {
    // 状态查询失败不影响主流程
  }
}

async function loadReportContent() {
  if (reportLoading.value || reportContent.value) return
  reportLoading.value = true
  try {
    const ans = await reportContentApi(recordId.value)
    reportContent.value = (ans && ans.answer) || ''
  } catch (e) {
    uni.showToast({ title: '报告生成中，请稍后重试', icon: 'none' })
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
    const msg = e && e.errMsg && e.errMsg.includes('cancel') ? '已取消支付' : '支付失败，请重试'
    uni.showToast({ title: msg, icon: 'none' })
  } finally {
    paying.value = false
  }
}

const reportPriceYuan = computed(() => (reportPriceCents.value / 100).toFixed(2))

function goBooking() {
  uni.switchTab({ url: '/pages/booking/booking' })
}
function restart() {
  uni.redirectTo({ url: '/pages/test/test' })
}

function goRelation() {
  uni.navigateTo({ url: `/pages/relation/relation?type=${result.value.type}` })
}

// 微信好友转发
onShareAppMessage(() => ({
  title: `我是${result.value?.type}号「${r.value?.title}」，来测测你的性格芯片`,
  path: '/pages/index/index',
  imageUrl: posterUrl.value || `/static/avatars/${result.value?.type}.png`,
}))
// 朋友圈分享
onShareTimeline(() => ({
  title: `九型芯之力 · 我是${result.value?.type}号「${r.value?.title}」`,
  query: '',
}))

// 生成分享海报（canvas 2d）
async function makePoster() {
  if (posterLoading.value) return
  posterLoading.value = true
  posterShow.value = true
  try {
    posterUrl.value = await drawPoster()
  } catch (e) {
    uni.showToast({ title: '海报生成失败', icon: 'none' })
    posterShow.value = false
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

// 用 canvas 2d 画竖版分享海报，返回临时文件路径
function drawPoster() {
  return new Promise((resolve, reject) => {
    const query = uni.createSelectorQuery().in(instance.proxy)
    query.select('#poster-canvas').fields({ node: true, size: true }).exec((res) => {
      if (!res || !res[0] || !res[0].node) return reject(new Error('canvas not found'))
      const canvas = res[0].node
      const ctx = canvas.getContext('2d')
      const dpr = uni.getSystemInfoSync().pixelRatio || 2
      const W = 320
      const H = 460
      canvas.width = W * dpr
      canvas.height = H * dpr
      ctx.scale(dpr, dpr)

      const t = result.value.type
      const accent = { green: '#38a83a', blue: '#1f73c4', red: '#e23a2f' }[info.value.color] || '#1f73c4'

      // 背景
      ctx.fillStyle = '#ffffff'
      ctx.fillRect(0, 0, W, H)
      ctx.fillStyle = accent
      ctx.fillRect(0, 0, W, 6)

      ctx.textAlign = 'center'
      ctx.fillStyle = '#9aa7b5'
      ctx.font = '12px sans-serif'
      ctx.fillText('九型芯之力 · 性格芯片测试', W / 2, 34)

      // 头像
      const avatar = canvas.createImage()
      avatar.onload = () => {
        const cx = W / 2
        const cy = 110
        const rad = 52
        ctx.save()
        ctx.beginPath()
        ctx.arc(cx, cy, rad, 0, Math.PI * 2)
        ctx.clip()
        ctx.drawImage(avatar, cx - rad, cy - rad, rad * 2, rad * 2)
        ctx.restore()
        ctx.beginPath()
        ctx.arc(cx, cy, rad, 0, Math.PI * 2)
        ctx.lineWidth = 3
        ctx.strokeStyle = accent
        ctx.stroke()

        // 文案
        ctx.fillStyle = '#1a2430'
        ctx.font = 'bold 24px sans-serif'
        ctx.fillText(`${t}号 · ${r.value.title}`, W / 2, 195)
        ctx.fillStyle = accent
        ctx.font = 'bold 13px sans-serif'
        ctx.fillText(`${info.value.en} · ${info.value.keywords}`, W / 2, 220)

        // summary 折行
        ctx.fillStyle = '#42505e'
        ctx.font = '14px sans-serif'
        wrapText(ctx, r.value.summary, W / 2, 252, W - 56, 22)

        // 底部引导
        ctx.fillStyle = '#f4f7f9'
        ctx.fillRect(0, H - 80, W, 80)
        ctx.fillStyle = '#1a2430'
        ctx.font = 'bold 14px sans-serif'
        ctx.fillText('长按识别 · 测测你是哪一块性格芯片', W / 2, H - 46)
        ctx.fillStyle = accent
        ctx.font = '12px sans-serif'
        ctx.fillText('微信搜索「九型芯之力」小程序', W / 2, H - 24)

        uni.canvasToTempFilePath({
          canvas,
          success: (r2) => resolve(r2.tempFilePath),
          fail: reject,
        }, instance.proxy)
      }
      avatar.onerror = reject
      avatar.src = `/static/avatars/${t}.png`
    })
  })
}

// canvas 文字折行
function wrapText(ctx, text, x, y, maxWidth, lineHeight) {
  const chars = text.split('')
  let line = ''
  let ty = y
  for (const ch of chars) {
    if (ctx.measureText(line + ch).width > maxWidth && line) {
      ctx.fillText(line, x, ty)
      line = ch
      ty += lineHeight
    } else {
      line += ch
    }
  }
  if (line) ctx.fillText(line, x, ty)
}
</script>

<template>
  <view class="wrap page-stack result-page" v-if="result">
    <view class="card head">
      <view class="avatar-wrap">
        <image class="avatar" :src="`/static/avatars/${result.type}.png`" mode="aspectFill" lazy-load />
        <view class="badge">{{ result.type }}</view>
      </view>
      <text class="title">{{ r.title }}</text>
      <text class="en">{{ info.en }} · {{ info.keywords }}</text>
      <text class="summary">{{ r.summary }}</text>
      <view class="persona">{{ persona }}</view>
    </view>

    <view class="drive">
      <view class="drive__item drive__item--fear">
        <text class="drive__label">基本恐惧</text>
        <text class="drive__txt">{{ info.fear }}</text>
      </view>
      <view class="drive__item drive__item--desire">
        <text class="drive__label">核心欲望</text>
        <text class="drive__txt">{{ info.desire }}</text>
      </view>
    </view>

    <view class="card">
      <text class="sec-title">你的三中心分布</text>
      <view v-for="c in result.centers" :key="c.key" class="bar">
        <text class="bar__name">{{ c.name }}</text>
        <view class="bar__track"><view class="bar__fill" :class="'bar__fill--' + c.key" :style="{ width: c.pct + '%' }" /></view>
        <text class="bar__pct">{{ c.pct }}%</text>
      </view>
    </view>

    <view v-if="secondInfo" class="card">
      <text class="sec-title">{{ wing ? '你的侧翼倾向' : '你的副型倾向' }}</text>
      <text class="sec-txt">主型 {{ result.type }} 号 {{ info.name }}，副型 {{ result.second }} 号 {{ secondInfo.name }} 特质也很突出，让你更立体。</text>
      <text class="sec-kw">{{ secondInfo.keywords }}</text>
    </view>

    <view class="arrows">
      <view class="arrow arrow--stress">
        <text class="arrow__tag">压力下 →</text>
        <text class="arrow__b">{{ info.stress }} 号 · {{ stressInfo.name }}</text>
      </view>
      <view class="arrow arrow--growth">
        <text class="arrow__tag">成长时 →</text>
        <text class="arrow__b">{{ info.growth }} 号 · {{ growthInfo.name }}</text>
      </view>
    </view>

    <view class="card grow">
      <text class="sec-title">成长建议</text>
      <text class="sec-txt">{{ r.growth }}</text>
    </view>

    <!-- 深度报告（付费解锁） -->
    <view class="card report">
      <view class="report__head">
        <text class="sec-title">AI 深度性格报告</text>
        <text v-if="reportUnlocked" class="report__badge report__badge--ok">已解锁</text>
        <text v-else class="report__badge">￥{{ reportPriceYuan }}</text>
      </view>
      <template v-if="reportUnlocked">
        <view v-if="reportLoading" class="report__loading">报告生成中，请稍候…</view>
        <text v-else-if="reportContent" class="report__content">{{ reportContent }}</text>
        <button v-else class="btn-ghost" @click="loadReportContent">查看报告</button>
      </template>
      <template v-else>
        <text class="report__intro">由 AI 结合九型知识库，为你生成专属的性格画像、成长盲点、人际与职业建议。</text>
        <button class="btn-primary" :loading="paying" :disabled="paying" @click="unlockReport">￥{{ reportPriceYuan }} 解锁深度报告</button>
      </template>
    </view>

    <view class="actions">
      <button class="btn-primary" :loading="saving" :disabled="saving || saved" @click="saveRecord">{{ saved ? '已存档' : '存入我的档案' }}</button>
      <view class="actions__row">
        <button class="btn-ghost" open-type="share">分享好友</button>
        <!-- #ifdef MP-WEIXIN -->
        <button class="btn-ghost" @click="makePoster">生成海报</button>
        <!-- #endif -->
        <!-- #ifdef H5 -->
        <button class="btn-ghost" disabled>小程序内生成海报</button>
        <!-- #endif -->
      </view>
      <button class="btn-soft" @click="goRelation">和 TA 合盘 · 看关系 →</button>
      <button class="btn-ghost" @click="goBooking">预约深入解读 →</button>
      <button class="btn-ghost" @click="restart">重新测试</button>
    </view>
    <text class="disclaimer">本测试基于九型人格体系简化设计，仅供趣味参考，不作专业诊断。</text>

    <!-- 离屏 canvas 用于绘制海报 -->
    <canvas id="poster-canvas" type="2d" class="poster-canvas"></canvas>

    <!-- 海报弹层 -->
    <view v-if="posterShow" class="poster-mask" @click="posterShow = false">
      <view class="poster-box" @click.stop>
        <view v-if="posterLoading" class="poster-loading">海报生成中…</view>
        <image v-else-if="posterUrl" class="poster-img" :src="posterUrl" mode="widthFix" show-menu-by-longpress />
        <view class="poster-ops">
          <button class="btn-primary" @click="savePoster">保存到相册</button>
          <button class="btn-ghost" @click="posterShow = false">关闭</button>
        </view>
        <text class="poster-tip">也可长按图片转发给好友</text>
      </view>
    </view>
  </view>
</template>

<style scoped>
.result-page {
  gap: 24rpx;
}
.head {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 15rpx;
  padding: 46rpx 32rpx 36rpx;
  background:
    linear-gradient(155deg, rgba(255,255,255,.92), rgba(255,255,255,.70)),
    radial-gradient(circle at 50% 0%, rgba(37,99,235,.20), transparent 42%);
}
.avatar-wrap {
  position: relative;
  width: 184rpx;
  height: 184rpx;
  margin-bottom: 8rpx;
}
.avatar {
  width: 184rpx;
  height: 184rpx;
  border-radius: 50%;
  border: 6rpx solid rgba(255,255,255,.95);
  box-shadow: 0 22rpx 44rpx -24rpx rgba(15,23,42,.38);
  box-sizing: border-box;
}
.badge {
  position: absolute;
  right: -2rpx;
  bottom: 6rpx;
  width: 60rpx;
  height: 60rpx;
  border-radius: 21rpx;
  background: linear-gradient(135deg,#60a5fa,#2563eb);
  color: #fff;
  font-size: 30rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 14rpx 34rpx -18rpx rgba(37,99,235,.72);
}
.title {
  color: #0f172a;
  font-size: 46rpx;
  font-weight: 900;
  line-height: 1.22;
}
.en {
  color: #2563eb;
  font-size: 24rpx;
  font-weight: 900;
}
.summary {
  color: #334155;
  font-size: 28rpx;
  line-height: 1.74;
}
.persona {
  margin-top: 8rpx;
  padding: 24rpx 26rpx;
  border-radius: 24rpx;
  background: linear-gradient(120deg, rgba(37,99,235,.10), rgba(249,115,22,.09));
  color: #0f172a;
  font-weight: 800;
  font-size: 26rpx;
  line-height: 1.66;
}
.drive,
.arrows {
  display: flex;
  gap: 16rpx;
}
.drive__item,
.arrow {
  flex: 1;
  border-radius: 26rpx;
  padding: 24rpx;
  display: flex;
  flex-direction: column;
  gap: 12rpx;
  background: rgba(255,255,255,.74);
  border: 2rpx solid rgba(255,255,255,.88);
  box-sizing: border-box;
  box-shadow: 0 16rpx 36rpx -30rpx rgba(15,23,42,.34);
}
.drive__item--fear,
.arrow--stress { border-color: rgba(225,29,72,.16); }
.drive__item--desire,
.arrow--growth { border-color: rgba(5,150,105,.18); }
.drive__label,
.arrow__tag {
  font-size: 22rpx;
  font-weight: 900;
  align-self: flex-start;
  min-height: 40rpx;
  display: inline-flex;
  align-items: center;
  padding: 0 16rpx;
  border-radius: 999rpx;
}
.drive__item--fear .drive__label,
.arrow--stress .arrow__tag { background: rgba(225,29,72,.10); color: #e11d48; }
.drive__item--desire .drive__label,
.arrow--growth .arrow__tag { background: rgba(5,150,105,.11); color: #059669; }
.drive__txt,
.arrow__b {
  font-size: 26rpx;
  color: #0f172a;
  font-weight: 800;
  line-height: 1.55;
}
.sec-title {
  margin-bottom: 18rpx;
}
.sec-txt {
  font-size: 27rpx;
}
.sec-kw {
  color: #2563eb;
  font-weight: 900;
  font-size: 25rpx;
  margin-top: 12rpx;
  display: block;
}
.bar {
  display: flex;
  align-items: center;
  gap: 16rpx;
  min-height: 58rpx;
  margin-bottom: 14rpx;
}
.bar:last-child { margin-bottom: 0; }
.bar__name {
  width: 130rpx;
  color: #334155;
  font-size: 25rpx;
  font-weight: 900;
}
.bar__track {
  flex: 1;
  height: 18rpx;
  background: rgba(15,23,42,.08);
  border-radius: 999rpx;
  overflow: hidden;
}
.bar__fill { height: 100%; border-radius: 999rpx; }
.bar__fill--gut { background: linear-gradient(90deg,#10b981,#6ee7b7); }
.bar__fill--heart { background: linear-gradient(90deg,#fb7185,#e11d48); }
.bar__fill--head { background: linear-gradient(90deg,#60a5fa,#2563eb); }
.bar__pct {
  width: 70rpx;
  text-align: right;
  font-size: 24rpx;
  font-weight: 900;
  color: #64748b;
}
.grow {
  background: linear-gradient(145deg, rgba(255,255,255,.86), rgba(239,246,255,.70));
}
.actions {
  display: flex;
  flex-direction: column;
  gap: 16rpx;
  margin-top: 8rpx;
  padding-bottom: env(safe-area-inset-bottom);
}
.actions__row {
  display: flex;
  gap: 16rpx;
}
.actions__row .btn-ghost { flex: 1; }
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
.report__badge--ok { background: rgba(5,150,105,.12); color: #059669; }
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
.report__loading {
  color: #64748b;
  font-size: 26rpx;
  padding: 26rpx 0;
  text-align: center;
}
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
.poster-img { width: 100%; border-radius: 18rpx; }
.poster-ops { display: flex; gap: 16rpx; width: 100%; }
.poster-ops .btn-primary,
.poster-ops .btn-ghost { flex: 1; }
.poster-tip { color: #94a3b8; font-size: 22rpx; }
</style>
