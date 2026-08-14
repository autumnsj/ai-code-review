import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发时把 /api /hooks /public 代理到 Go 后端，生产由 Go 直接 serve 构建产物。
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/hooks': 'http://localhost:8080',
      '/public': 'http://localhost:8080',
    },
  },
})
