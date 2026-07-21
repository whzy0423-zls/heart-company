export function handleTypeBadgeClick(interactive, disabled, emit, event) {
  if (!interactive) return false

  if (disabled) {
    event?.preventDefault?.()
    event?.stopPropagation?.()
    return false
  }

  emit('click', event)
  return true
}
