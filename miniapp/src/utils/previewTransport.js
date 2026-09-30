import siteConfig from '../data/studioSiteConfig.json'
import videos from '../data/studioVideos.json'

const USER_KEY = 'nx_ui_preview_user'
const PROGRESS_KEY = 'nx_ui_preview_progress'
function read(key, fallback) {
  try { return uni.getStorageSync(key) || fallback } catch { return fallback }
}
const exampleBooking = { id: 9001, kind: 'course', contactName: '林小满', phone: '13800000000', intent: '九型人格与生命关系 · 10月17日—18日（演示）', preferredTime: '2026-10-17', message: '想更好地理解自己和家人。', status: 'confirmed', createTime: '2026-09-28 09:30' }
const clone = (data) => JSON.parse(JSON.stringify(data))

// Every preview request terminates here, including unknown routes and writes.
// It must never fall through to the real backend.
export async function previewRequest({ url, method = 'GET', data = {}, query = {} }) {
  if (!(import.meta.env?.DEV === true && import.meta.env?.VITE_UI_PREVIEW === 'true')) throw new Error('预览数据仅在本地演示中启用')
  if (url === '/public/site-config') return clone(siteConfig)
  if (url === '/wx/login') return { accessToken: 'local-ui-preview-session' }
  if (url === '/wx/userinfo') {
    const user = read(USER_KEY, { nickname: '林小满', avatar: '', mainType: null })
    if (method === 'PUT') uni.setStorageSync(USER_KEY, { ...user, ...data })
    return clone(method === 'PUT' ? { ...user, ...data } : user)
  }
  if (url === '/miniapp/bookings') {
    if (method === 'POST') {
      throw new Error('正式报名请在微信小程序内提交，信息将保存至后台管理')
    }
    // The H5 preview exposes a read-only fixture so it never depends on, or
    // mutates, device storage. Formal records come from the WeChat API.
    return { items: [clone(exampleBooking)] }
  }
  if (url === '/miniapp/test-records') {
    const items = read('nx_ui_preview_tests', [])
    if (method === 'POST') {
      const record = { ...data, id: Date.now(), createTime: new Date().toISOString().slice(0, 16).replace('T', ' ') }
      uni.setStorageSync('nx_ui_preview_tests', [record, ...items])
      return record
    }
    return { items }
  }
  if (url === '/public/game-results') return { saved: true }
  if (['/public/classroom/recent', '/public/classroom/standalone'].includes(url)) {
    const offset = Math.max(0, Number(query.offset) || 0)
    return { items: clone(videos.slice(offset, offset + (Number(query.limit) || 50))), total: videos.length }
  }
  if (url === '/public/classroom/series') return { items: [] }
  if (url === '/miniapp/classroom/continue-learning') {
    const progress = read(PROGRESS_KEY, {})
    return { items: Object.entries(progress).map(([id, positionSeconds]) => ({ ...videos.find((item) => String(item.id) === id), positionSeconds })).filter((item) => item.id) }
  }
  const content = url.match(/\/classroom\/content\/(\d+)(?:\/(ticket|play|progress))?$/)
  if (content) {
    const item = videos.find((row) => String(row.id) === content[1])
    if (!item) throw new Error('这段视频暂未收录')
    if (content[2] === 'ticket') return { ticket: 'local-preview' }
    if (content[2] === 'play') {
      const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:5178'
      return { url: `${origin}/__studio-media/${item.previewVideo}` }
    }
    if (content[2] === 'progress') {
      uni.setStorageSync(PROGRESS_KEY, { ...read(PROGRESS_KEY, {}), [item.id]: data.positionSeconds })
      return { saved: true }
    }
    return clone(item)
  }
  throw new Error('此操作尚未纳入演示，可继续浏览老师内容与体验报名')
}
