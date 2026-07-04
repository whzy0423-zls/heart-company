export const CHAT_STORAGE_KEY = 'nx_chat_messages'

export function clearChatMessages() {
  try {
    uni.removeStorageSync(CHAT_STORAGE_KEY)
  } catch {}
}
