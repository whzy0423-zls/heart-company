import { Marked, Renderer } from 'marked'
import DOMPurify from 'isomorphic-dompurify'

const LINK_PROTOCOLS = new Set(['https:', 'mailto:', 'tel:'])
const MEDIA_PROTOCOLS = new Set(['https:'])
const SCHEME_RE = /^([a-z][a-z\d+.-]*):/i
const CONTROL_RE = /[\u0000-\u001F\u007F]/

function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (ch) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  })[ch])
}

function safeUrl(value, allowedProtocols) {
  if (!value) return ''
  const url = String(value).trim()
  if (!url || CONTROL_RE.test(url)) return ''
  if (url.startsWith('//')) return ''

  const normalized = url.replace(/\s+/g, '')
  const scheme = normalized.match(SCHEME_RE)?.[1]?.toLowerCase()
  if (!scheme) return url

  return allowedProtocols.has(`${scheme}:`) ? url : ''
}

const renderer = new Renderer()

renderer.html = () => ''

renderer.link = (href, title, text) => {
  const url = safeUrl(href, LINK_PROTOCOLS)
  if (!url) return text

  let out = `<a href="${escapeHtml(url)}"`
  if (title) out += ` title="${escapeHtml(title)}"`
  out += `>${text}</a>`
  return out
}

renderer.image = (href, title, text) => {
  const url = safeUrl(href, MEDIA_PROTOCOLS)
  if (!url) return escapeHtml(text)

  let out = `<img src="${escapeHtml(url)}" alt="${escapeHtml(text)}"`
  if (title) out += ` title="${escapeHtml(title)}"`
  out += '>'
  return out
}

const markdown = new Marked({
  breaks: true,
  gfm: true,
  renderer,
})

export function renderMarkdown(content = '') {
  if (!content) return ''
  return sanitizeRenderedHtml(markdown.parse(content))
}

export function sanitizeRenderedHtml(html = '') {
  if (!html) return ''
  return DOMPurify.sanitize(html, {
    ALLOWED_URI_REGEXP:
      /^(?!\/\/)(?:(?:https|mailto|tel):|[^a-z]|[a-z+.-]+(?:[^a-z+.-:]|$))/i,
    FORBID_ATTR: ['style'],
  })
}
