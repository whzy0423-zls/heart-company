const DEFAULT_HISTORY_LIMIT = 6
const DEFAULT_HISTORY_TEXT_LIMIT = 220
const DEFAULT_MESSAGE_LIMIT = 28
const DEFAULT_SOURCE_LIMIT = 2
const DEFAULT_SOURCE_TEXT_LIMIT = 84

function clean(value) {
  return String(value || '').trim()
}

function trimText(value, limit) {
  const text = clean(value).replace(/\s+/g, ' ')
  if (!limit || text.length <= limit) return text
  return `${text.slice(0, limit)}...`
}

export function buildRecentHistory(messages, limit = DEFAULT_HISTORY_LIMIT) {
  return (messages || [])
    .filter((msg) => !msg.localOnly && (msg.role === 'user' || msg.role === 'assistant'))
    .slice(-limit)
    .map((msg) => ({
      role: msg.role,
      content: trimText(msg.content, DEFAULT_HISTORY_TEXT_LIMIT),
    }))
    .filter((msg) => msg.content)
}

export function limitMessages(messages, limit = DEFAULT_MESSAGE_LIMIT) {
  const list = Array.isArray(messages) ? messages : []
  if (list.length <= limit) return list
  return list.slice(list.length - limit)
}

export function normalizeSources(sources, limit = DEFAULT_SOURCE_LIMIT) {
  return (sources || [])
    .map((source) => ({
      id: clean(source.id || source.title),
      title: clean(source.title),
      snippet: trimText(source.snippet, DEFAULT_SOURCE_TEXT_LIMIT),
    }))
    .filter((source) => source.id && source.title)
    .slice(0, limit)
}

export function serializeMessages(messages) {
  return limitMessages(messages)
    .filter((msg) => !msg.localOnly && (msg.role === 'user' || msg.role === 'assistant'))
    .map((msg) => ({
      id: clean(msg.id),
      role: msg.role,
      content: trimText(msg.content, 1200),
      sources: normalizeSources(msg.sources),
    }))
    .filter((msg) => msg.id && msg.content)
}

export function restoreMessages(messages) {
  return limitMessages(messages)
    .filter((msg) => msg && (msg.role === 'user' || msg.role === 'assistant'))
    .map((msg) => ({
      id: clean(msg.id),
      role: msg.role,
      content: trimText(msg.content, 1200),
      sources: normalizeSources(msg.sources),
    }))
    .filter((msg) => msg.id && msg.content)
}

export function chatStatusText({ sending = false, hasConversation = false, lastSources = [] } = {}) {
  if (sending) return '正在检索知识库'
  const count = Array.isArray(lastSources) ? lastSources.length : 0
  if (count > 0) return `已命中 ${count} 条资料`
  if (hasConversation) return '可继续追问'
  return 'RAG 知识检索已开启'
}
