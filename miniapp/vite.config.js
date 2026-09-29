import { defineConfig } from 'vite'
import uni from '@dcloudio/vite-plugin-uni'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { studioPreviewMedia } from './scripts/studio-preview-media.mjs'

import {
  createWechatCustomPropertiesPlugin,
  extractCustomProperties,
} from './scripts/postcss-wechat-custom-properties.mjs'

const sharedThemeFile = fileURLToPath(
  new URL('./src/styles/apple-mobile.css', import.meta.url),
)

function loadSharedThemeTokens() {
  return extractCustomProperties(readFileSync(sharedThemeFile, 'utf8'), {
    from: sharedThemeFile,
  })
}

export default defineConfig({
  plugins: [uni(), studioPreviewMedia()],
  css: {
    postcss: {
      plugins: process.env.UNI_PLATFORM === 'mp-weixin'
        ? [createWechatCustomPropertiesPlugin({
            globalTokens: loadSharedThemeTokens,
            dependencyFiles: [sharedThemeFile],
          })]
        : [],
    },
  },
})
