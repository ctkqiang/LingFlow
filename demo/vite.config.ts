import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:4030',
        changeOrigin: true
      },
      '/chat': {
        target: 'ws://localhost:4030',
        ws: true,
        changeOrigin: true
      }
    }
  }
})
