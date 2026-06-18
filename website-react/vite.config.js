import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    fs: {
      allow: [
        fileURLToPath(new URL('.', import.meta.url)),
        fileURLToPath(new URL('../shared', import.meta.url)),
      ],
    },
    // 开发期把 /api 代理到本地 Go server，便于运行时拉取站点配置。
    proxy: {
      '/api': {
        target: 'http://localhost:5320',
        changeOrigin: true,
      },
    },
  },
  build: {
    target: 'es2018',
    cssCodeSplit: true,
  },
})
