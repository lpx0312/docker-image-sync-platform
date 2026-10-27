import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// 单测/覆盖率配置：node 环境跑纯逻辑，jsdom 跑 DOM/composable/store 用例
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['tests/**/*.test.js'],
    coverage: {
      provider: 'v8',
      include: ['src/utils/**', 'src/composables/**', 'src/stores/**', 'src/api/**'],
      reporter: ['text', 'json-summary']
    }
  }
})
