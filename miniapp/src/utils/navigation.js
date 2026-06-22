export const CHAT_TAB_URL = '/pages/chat/chat'

export function openChatPage(uniApi = uni) {
  uniApi.switchTab({ url: CHAT_TAB_URL })
}
