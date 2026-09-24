import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./relation.vue', import.meta.url), 'utf8')
const templateStart = source.indexOf('<template>')
const templateEnd = source.lastIndexOf('</template>')
const template = templateStart >= 0 && templateEnd > templateStart
  ? source.slice(templateStart + '<template>'.length, templateEnd)
  : ''
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1] || ''

assert.match(script, /import\s+\{\s*previewImage\s*\}\s+from\s+['"]\.\.\/\.\.\/utils\/imagePreview(?:\.js)?['"]/, 'relation should use the shared image preview helper')
assert.match(script, /function\s+previewMyAvatar\s*\(\s*\)/, 'relation should expose a preview action for my avatar')
assert.match(script, /function\s+previewTaAvatar\s*\(\s*\)/, 'relation should expose a preview action for TA avatar')
assert.match(script, /previewMyAvatar[\s\S]*previewImage\(/, 'my avatar preview should call the shared helper')
assert.match(script, /previewTaAvatar[\s\S]*previewImage\(/, 'TA avatar preview should call the shared helper')

assert.match(
  template,
  /<button\b(?=[^>]*class="pair__avatar-action")(?=[^>]*@click="previewMyAvatar\(\)")[^>]*>[\s\S]*?<image\b(?=[^>]*class="pair__avatar")/,
  'my avatar should be inside an accessible preview button',
)
assert.match(
  template,
  /<button\b(?=[^>]*class="pair__avatar-action")(?=[^>]*@click="previewTaAvatar\(\)")[^>]*>[\s\S]*?<image\b(?=[^>]*class="pair__avatar")/,
  'TA avatar should be inside an accessible preview button',
)
assert.match(template, /class="pair__avatar-fallback"/, 'avatar failures should retain their fallback')

console.log('relation avatar preview tests passed')
