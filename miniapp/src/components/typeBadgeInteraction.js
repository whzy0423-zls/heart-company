export function handleTypeBadgeClick(disabled, emit, event) {
  if (disabled) {
    event?.preventDefault?.()
    event?.stopPropagation?.()
    return false
  }

  emit('click', event)
  return true
}
