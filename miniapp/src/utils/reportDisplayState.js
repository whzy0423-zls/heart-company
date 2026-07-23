export function reportDisplayState({ recordId, loading, error, unlocked, priceCents }) {
  if (unlocked) {
    return { key: 'unlocked', priceCents: null }
  }

  if (!recordId) {
    return { key: 'needs-save', priceCents: null }
  }

  if (loading) {
    return { key: 'status-loading', priceCents: null }
  }

  if (error) {
    return { key: 'status-error', priceCents: null }
  }

  if (Number.isFinite(priceCents) && priceCents > 0) {
    return { key: 'ready', priceCents }
  }

  return { key: 'status-error', priceCents: null }
}
