function text(value) {
  return typeof value === 'string' ? value.trim() : ''
}

function durationLabel(seconds) {
  const total = Math.max(0, Math.floor(Number(seconds) || 0))
  if (!total) return ''
  const minutes = Math.floor(total / 60)
  if (minutes < 60) return `约 ${minutes || 1} 分钟`
  const hours = Math.floor(minutes / 60)
  const remainder = minutes % 60
  return remainder ? `约 ${hours} 小时 ${remainder} 分钟` : `约 ${hours} 小时`
}

export function mapPublishedClassroomItems(items) {
  if (!Array.isArray(items)) return []
  return items
    .filter((item) => item && item.id)
    .map((item) => {
      const isAudio = item.contentType === 'audio'
      return {
        id: String(item.id),
        title: text(item.title) || '未命名课件',
        description: text(item.description),
        cover: text(item.coverUrl),
        badge: isAudio ? '音频课件' : '视频课件',
        duration: durationLabel(item.durationSeconds),
        materialTypes: [isAudio ? '音频' : '视频'],
        bullets: [],
        url: '',
        classroomId: String(item.id),
      }
    })
}

/**
 * Prefer published classroom data, while allowing the packaged preview
 * catalogue to keep the learning page useful when the devtools API host is
 * unavailable or has not been seeded yet.
 */
export function resolveClassroomItems(remoteItems, bundledItems, options = {}) {
  if (Array.isArray(remoteItems) && remoteItems.length > 0) {
    return { items: remoteItems, usedFallback: false }
  }
  if (!options.allowFallback) {
    return { items: Array.isArray(remoteItems) ? remoteItems : [], usedFallback: false }
  }
  const fallback = Array.isArray(bundledItems) ? bundledItems : []
  return { items: fallback, usedFallback: fallback.length > 0 }
}
