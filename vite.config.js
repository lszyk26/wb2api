import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    target: 'es2020',
    cssCodeSplit: false,
    assetsInlineLimit: 4096,
    // 拆包：动效实验室（56 个 demo，最大的一块）和管理台按需加载
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) {
            if (id.includes('/src/demos/')) return 'demos'
            if (id.includes('/src/pages/MotionLab')) return 'motion'
            if (id.includes('/src/pages/Admin')) return 'admin'
          }
          if (id.includes('node_modules/vue')) return 'vue'
          return undefined
        },
      },
    },
    chunkSizeWarningLimit: 700,
  },
  server: { host: '0.0.0.0', port: 5199 },
})
