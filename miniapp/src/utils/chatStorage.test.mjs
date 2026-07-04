import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const profileSource = readFileSync(resolve(__dirname, '../pages/profile/profile.vue'), 'utf8')
const chatSource = readFileSync(resolve(__dirname, '../pages/chat/chat.vue'), 'utf8')
const storageSource = readFileSync(resolve(__dirname, './chatStorage.js'), 'utf8')

assert.match(storageSource, /CHAT_STORAGE_KEY\s*=\s*['"]nx_chat_messages['"]/, 'chat cache key should stay explicit')
assert.match(profileSource, /clearChatMessages\(\)/, 'logout/reset should clear cached chat messages')
assert.match(chatSource, /clearChatMessages\(\)/, 'chat clear action should reuse shared cache cleanup')

console.log('chat storage tests passed')
