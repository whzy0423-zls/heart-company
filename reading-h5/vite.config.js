import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5330,
    // 开发期把 /api 代理到本地 Go server。
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
