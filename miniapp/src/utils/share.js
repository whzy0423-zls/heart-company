import { resolveContentAsset } from './contentAsset.js'
import shareAssets from '../data/shareAssets.js'

const STUDIO_COVER = '/static/share/studio.jpg'
const MENUS = ['shareAppMessage', 'shareTimeline']
const PAGES = {
  home: ['index', '九型芯之力｜认识自己，理解彼此'],
  teacher: ['teacher', '认识老韩｜把对性格的理解带回生活'],
  daily: ['learn', '老韩的日常分享｜一起看见生活中的成长'],
  course: ['course-detail', '九型芯之力｜一起走进课堂'],
  classroom: ['classroom', '老师课堂｜从一段分享开始'],
  content: ['classroom-detail', '老韩的日常短讲'],
  explore: ['enneagram', '认识九型｜看见不同的自己'],
  type: ['enneagram-detail', '九型人格｜了解一种看世界的方式'],
  result: ['result', '九型芯之力｜你是哪一型？'],
  relation: ['relation', '关系合盘｜读懂彼此'],
}

function publicImage(value, fallback) {
  // Native share dialogs load outside the page's package-image context.
  // Publish bundled covers as immutable HTTPS assets; never guess a URL for
  // an unknown local image. The manifest is verified before each build.
  const fallbackUrl = resolveContentAsset(shareAssets[fallback] || shareAssets[STUDIO_COVER])
  const asset = resolveContentAsset(value)
  if (/^\/static\/teacher\/(?:hero-)?portrait\.jpg$/.test(asset)) return resolveContentAsset(shareAssets[STUDIO_COVER])
  // WeChat share covers support PNG/JPG, unlike in-page images (e.g. WebP).
  // Shares outlive signed URLs. Keep only stable supported assets; playback and
  // credential-bearing addresses are never embedded in a share card.
  if (!asset || /[?#]/.test(asset) || !/\.(?:jpe?g|png)$/i.test(asset)) return fallbackUrl
  if (asset.startsWith('/static/')) return resolveContentAsset(shareAssets[asset]) || fallbackUrl
  return asset.startsWith('https://') ? asset : fallbackUrl
}

export function buildShareCard({ kind = 'home', title, imageUrl, id, type, seriesId, myType, taType } = {}) {
  if (!PAGES[kind]) kind = 'home'
  let query = ''
  const typeId = /^[1-9]$/.test(String(type ?? '')) ? Number(type) : 0
  if (kind === 'course') {
    if (!/^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$/.test(String(id ?? ''))) return buildShareCard()
    query = `id=${encodeURIComponent(id)}`
  } else if (kind === 'content') {
    if (!/^[1-9]\d*$/.test(String(id ?? ''))) return buildShareCard()
    query = `id=${encodeURIComponent(id)}`
  } else if (kind === 'classroom' && /^[1-9]\d*$/.test(String(seriesId ?? ''))) {
    query = `tab=series&seriesId=${encodeURIComponent(seriesId)}`
  } else if (kind === 'result') {
    // Even invalid type shares explicitly select the public landing branch.
    query = `shareType=${typeId}`
  } else if (kind === 'type') {
    query = `type=${typeId || 1}`
  } else if (kind === 'relation') {
    const validType = value => ['string', 'number'].includes(typeof value) && /^[1-9]$/.test(String(value))
    if (validType(myType) && validType(taType)) query = `myType=${myType}&taType=${taType}`
  }
  const [page, defaultTitle] = PAGES[kind]
  const cleanedTitle = typeof title === 'string' ? title.replace(/[\u0000-\u001f\u007f]/g, ' ').trim().slice(0, 100) : ''
  const fallback = ['result','type'].includes(kind) ? `/static/share/result-${typeId || 'default'}.jpg` : STUDIO_COVER
  const relationTitle = kind === 'relation' && query ? `${myType}号 × ${taType}号关系合盘｜看见彼此的相处方式` : ''
  const metadata = { title: cleanedTitle || relationTitle || defaultTitle, imageUrl: publicImage(imageUrl, fallback) }
  return {
    appMessage: { ...metadata, path: `/pages/${page}/${page}${query ? `?${query}` : ''}` },
    timeline: { ...metadata, query },
  }
}

export function isTimelinePreview(runtime = typeof uni !== 'undefined' ? uni : null) {
  // Scene 1154 is WeChat's read-only Timeline single-page runtime. Prefer the
  // latest entry over an old launch when the full app has since been opened.
  for (const method of ['getEnterOptionsSync', 'getLaunchOptionsSync']) {
    try {
      const options = runtime?.[method]?.()
      if (options?.scene !== undefined) return Number(options.scene) === 1154
    } catch { /* Older SDKs may expose only launch options. */ }
  }
  return false
}

export function hidePrivateShareMenu(runtime = typeof uni !== 'undefined' ? uni : null) {
  // #ifdef MP-WEIXIN
  if (isTimelinePreview(runtime)) return
  if (typeof runtime?.hideShareMenu === 'function') runtime.hideShareMenu({ menus: [...MENUS] })
  // #endif
}

export function requireFullMiniapp(runtime = typeof uni !== 'undefined' ? uni : null) {
  if (!isTimelinePreview(runtime)) return true
  if (typeof runtime?.showModal === 'function') runtime.showModal({
    title: '进入小程序继续',
    content: '请点击「前往小程序」，继续浏览、报名或查看学习记录。',
    showCancel: false,
    confirmText: '我知道了',
    confirmColor: '#A55C3B',
  })
  return false
}

export function showPublicShareMenu(enabled = true, runtime = typeof uni !== 'undefined' ? uni : null) {
  // #ifdef MP-WEIXIN
  if (isTimelinePreview(runtime)) return
  if (!enabled) { hidePrivateShareMenu(runtime); return }
  if (typeof runtime?.showShareMenu === 'function') runtime.showShareMenu({ menus: [...MENUS] })
  // #endif
}

export function showTimelineShareHint(runtime = typeof uni !== 'undefined' ? uni : null) {
  // #ifdef MP-WEIXIN
  if (isTimelinePreview(runtime)) return
  if (typeof runtime?.showModal === 'function') runtime.showModal({
    title: '分享到朋友圈',
    content: '请点击微信右上角「···」，选择「分享到朋友圈」。分享卡片会使用当前页面的标题和封面。',
    showCancel: false,
    confirmText: '我知道了',
    confirmColor: '#A55C3B',
  })
  // #endif
}
