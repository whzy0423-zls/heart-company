// A build-time, development-only switch. Query parameters cannot enable fixtures.
export const UI_PREVIEW = import.meta.env?.DEV === true && import.meta.env?.VITE_UI_PREVIEW === 'true'

export function initializePreviewSession() {
  if (!UI_PREVIEW) return ''
  const token = 'local-ui-preview-session'
  uni.setStorageSync('nx_ui_preview_token', token)
  return token
}
