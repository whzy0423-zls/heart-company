<script setup>
import { computed, nextTick, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'
import { ensureLogin } from '../../utils/auth'
import { chatApi } from '../../api'
import { chatErrorMessage } from '../../utils/chatErrors'
import {
  buildRecentHistory,
  chatStatusText,
  limitMessages,
  normalizeSources,
  restoreMessages,
  serializeMessages,
} from '../../utils/chatState'

const input = ref('')
const sending = ref(false)
const bottomAnchor = 'chat-bottom'
const scrollIntoView = ref('')
const CHAT_STORAGE_KEY = 'nx_chat_messages'
let messageSeq = 0

function nextID(prefix) {
  messageSeq += 1
  return `${prefix}-${Date.now()}-${messageSeq}`
}

const messages = ref([
  {
    id: 'welcome',
    role: 'assistant',
    content: '你可以问我九型人格、关系沟通、成长练习或课程方向。我会先检索九型资料，再给你回答。',
    sources: [],
    localOnly: true,
  },
])

const suggestions = [
  '我的主型怎么成长？',
  '亲密关系里怎么沟通？',
  '有哪些课程适合我？',
]

const hasConversation = computed(() => messages.value.some((msg) => !msg.localOnly && msg.role === 'user'))
const latestAssistantSources = computed(() => {
  const latest = [...messages.value].reverse().find((msg) => !msg.error && msg.role === 'assistant' && !msg.localOnly)
  return latest ? latest.sources || [] : []
})
const statusText = computed(() =>
  chatStatusText({
    sending: sending.value,
    hasConversation: hasConversation.value,
    lastSources: latestAssistantSources.value,
  }),
)

function recentHistory() {
  return buildRecentHistory(messages.value)
}

function saveMessages() {
  try {
    uni.setStorageSync(CHAT_STORAGE_KEY, serializeMessages(messages.value))
  } catch {
    // 本地缓存失败不影响对话主流程。
  }
}

function pushMessage(message) {
  messages.value = limitMessages([...messages.value, message])
  saveMessages()
}

function restoreCachedMessages() {
  try {
    const cached = restoreMessages(uni.getStorageSync(CHAT_STORAGE_KEY))
    if (cached.length) {
      messages.value = limitMessages([messages.value[0], ...cached])
    }
  } catch {
    uni.removeStorageSync(CHAT_STORAGE_KEY)
  }
}

function onInput(e) {
  input.value = e.detail.value || ''
}

function useSuggestion(text) {
  input.value = text
}

async function clearChat() {
  if (sending.value) return
  messages.value = [messages.value[0]]
  try {
    uni.removeStorageSync(CHAT_STORAGE_KEY)
  } catch {}
  await scrollToBottom()
}

async function scrollToBottom() {
  scrollIntoView.value = ''
  await nextTick()
  // scroll-into-view 比累加 scrollTop 更稳定，也避免消息多时数值持续膨胀。
  scrollIntoView.value = bottomAnchor
}

async function requestAnswer(question, history, retried = false) {
  await ensureLogin()
  try {
    return await chatApi({ question, history })
  } catch (e) {
    if (!retried && (e.statusCode === 401 || e.statusCode === 403)) {
      await ensureLogin()
      return requestAnswer(question, history, true)
    }
    throw e
  }
}

async function send() {
  const question = input.value.trim()
  if (!question || sending.value) return

  const history = recentHistory()
  pushMessage({ id: nextID('user'), role: 'user', content: question, sources: [] })
  input.value = ''
  sending.value = true
  await scrollToBottom()

  try {
    const res = await requestAnswer(question, history)
    const sources = normalizeSources(res.sources)
    pushMessage({
      id: nextID('assistant'),
      role: 'assistant',
      content: res.answer || '我暂时没有找到合适回答，可以换个问法再试一次。',
      sources,
      noSources: sources.length === 0,
    })
  } catch (e) {
    pushMessage({
      id: nextID('error'),
      role: 'assistant',
      content: chatErrorMessage(e),
      sources: [],
      error: true,
    })
  } finally {
    sending.value = false
    await scrollToBottom()
  }
}

onLoad(() => {
  restoreCachedMessages()
  scrollToBottom()
})
</script>

<template>
  <view class="chat">
    <view class="chat__head">
      <view class="chat__topline">
        <text class="eyebrow">RAG 检索问答</text>
        <button class="chat__clear" :disabled="sending" @click="clearChat">清空</button>
      </view>
      <view class="chat__intro">
        <text class="chat__title">九型 AI 对话</text>
        <text class="chat__status">{{ statusText }}</text>
      </view>
      <text class="chat__lead">围绕你的测试档案、九型资料和课程内容回答。</text>
    </view>

    <scroll-view class="chat__body" scroll-y :scroll-into-view="scrollIntoView" scroll-with-animation>
      <view
        v-for="msg in messages"
        :key="msg.id"
        class="msg"
        :class="['msg--' + msg.role, { 'msg--error': msg.error }]"
      >
        <text class="msg__text">{{ msg.content }}</text>
        <view v-if="msg.sources && msg.sources.length" class="sources">
          <text class="sources__label">参考资料</text>
          <view v-for="src in msg.sources" :key="src.id" class="source">
            <text class="source__title">{{ src.title }}</text>
            <text class="source__snippet">{{ src.snippet }}</text>
          </view>
        </view>
        <text v-else-if="msg.noSources" class="msg__hint">本次未命中明确资料，可换个更具体的问题继续问。</text>
      </view>
      <view v-if="sending" class="msg msg--assistant">
        <text class="msg__text">正在检索资料并组织回答...</text>
      </view>
      <view :id="bottomAnchor" class="chat__anchor"></view>
    </scroll-view>

    <view class="chat__bottom">
      <scroll-view class="suggestions" scroll-x :show-scrollbar="false">
        <view class="suggestions__inner">
          <text v-for="item in suggestions" :key="item" class="suggestion" @click="useSuggestion(item)">{{ item }}</text>
        </view>
      </scroll-view>

      <view class="composer">
        <input
          class="composer__input"
          :value="input"
          confirm-type="send"
          placeholder="输入你想问的问题"
          :disabled="sending"
          @input="onInput"
          @confirm="send"
        />
        <button class="composer__send" :loading="sending" :disabled="sending || !input.trim()" @click="send">
          {{ sending ? '' : '发送' }}
        </button>
      </view>
    </view>
  </view>
</template>

<style scoped>
.chat {
  min-height: 100vh;
  height: 100vh;
  padding: 22rpx 24rpx calc(18rpx + env(safe-area-inset-bottom));
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 14rpx;
  background: linear-gradient(180deg, #fbfcff 0%, #f3f7fb 100%);
}
.chat__head {
  display: flex;
  flex-direction: column;
  gap: 7rpx;
  flex-shrink: 0;
}
.chat__topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16rpx;
}
.chat__clear {
  min-width: 92rpx;
  height: 52rpx;
  padding: 0 18rpx;
  border-radius: 999rpx;
  background: rgba(255,255,255,.82);
  border: 2rpx solid rgba(20,24,32,.08);
  color: #767d89;
  font-size: 22rpx;
  font-weight: 800;
  line-height: 52rpx;
}
.chat__clear::after {
  border: none;
}
.chat__clear[disabled] {
  opacity: .45;
}
.chat__intro {
  display: flex;
  align-items: center;
  gap: 14rpx;
  justify-content: space-between;
}
.chat__title {
  flex: 1;
  min-width: 0;
  color: #12151b;
  font-size: 38rpx;
  font-weight: 900;
  line-height: 1.2;
}
.chat__lead {
  color: #3c424d;
  font-size: 24rpx;
  line-height: 1.5;
}
.chat__status {
  flex-shrink: 0;
  min-height: 42rpx;
  max-width: 260rpx;
  padding: 5rpx 14rpx;
  border-radius: 999rpx;
  background: rgba(43,127,255,.08);
  color: #2b7fff;
  font-size: 20rpx;
  font-weight: 800;
  line-height: 30rpx;
  text-align: center;
}
.chat__body {
  flex: 1;
  min-height: 0;
  box-sizing: border-box;
  padding: 4rpx 2rpx;
}
.chat__anchor {
  height: 4rpx;
}
.chat__bottom {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 12rpx;
}
.msg {
  max-width: 86%;
  margin: 14rpx 0;
  padding: 19rpx 22rpx;
  border-radius: 22rpx;
  box-sizing: border-box;
}
.msg--assistant {
  background: #fff;
  border: 2rpx solid rgba(20,24,32,.07);
}
.msg--user {
  max-width: 82%;
  margin-left: auto;
  background: linear-gradient(135deg,#2b7fff,#5aa0ff);
  color: #fff;
}
.msg--error {
  border-color: rgba(226,58,71,.28);
}
.msg__text {
  font-size: 27rpx;
  line-height: 1.62;
  white-space: pre-wrap;
}
.msg__hint {
  display: block;
  margin-top: 12rpx;
  color: #767d89;
  font-size: 22rpx;
  line-height: 1.5;
}
.sources {
  margin-top: 16rpx;
  display: flex;
  flex-direction: column;
  gap: 10rpx;
}
.sources__label {
  color: #767d89;
  font-size: 21rpx;
  font-weight: 900;
}
.source {
  padding: 14rpx 16rpx;
  border-radius: 16rpx;
  background: rgba(43,127,255,.08);
}
.source__title {
  display: block;
  color: #2b7fff;
  font-size: 23rpx;
  font-weight: 900;
}
.source__snippet {
  display: block;
  color: #3c424d;
  font-size: 22rpx;
  line-height: 1.55;
  margin-top: 4rpx;
}
.suggestions {
  width: 100%;
  white-space: nowrap;
}
.suggestions__inner {
  display: inline-flex;
  gap: 12rpx;
  padding: 2rpx 2rpx 4rpx;
}
.suggestion {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 52rpx;
  padding: 12rpx 18rpx;
  border-radius: 999rpx;
  background: rgba(255,255,255,.86);
  border: 2rpx solid rgba(20,24,32,.07);
  color: #3c424d;
  font-size: 22rpx;
  font-weight: 800;
}
.composer {
  display: flex;
  gap: 14rpx;
  align-items: center;
}
.composer__input {
  flex: 1;
  min-width: 0;
  height: 84rpx;
  border-radius: 24rpx;
  background: rgba(255,255,255,.86);
  border: 2rpx solid rgba(20,24,32,.08);
  padding: 0 24rpx;
  box-sizing: border-box;
  color: #12151b;
  font-size: 27rpx;
}
.composer__send {
  width: 124rpx;
  height: 84rpx;
  padding: 0;
  border-radius: 22rpx;
  background: linear-gradient(135deg,#2b7fff,#5aa0ff);
  color: #fff;
  font-size: 26rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}
.composer__send::after {
  border: none;
}
.composer__send[disabled] {
  opacity: .56;
}
</style>
