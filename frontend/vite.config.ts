import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    vuetify({ autoImport: true }),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      // Everything under /api hits the Go backend on :9090.
      '/api': {
        target: 'http://localhost:9090',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    target: 'es2022',
    sourcemap: false,
    rollupOptions: {
      output: {
        // Vite 8 / rolldown requires a function form for manualChunks.
        // Split vendor bundles by package to keep initial payload small.
        manualChunks(id: string) {
          if (!id.includes('node_modules')) return
          if (id.includes('vuetify')) return 'vuetify'
          if (id.includes('vue-i18n')) return 'vue-i18n'
          if (id.includes('vue-router')) return 'vue-router'
          if (id.includes('pinia')) return 'pinia'
          if (id.includes('@mdi/font')) return 'mdi'
        },
      },
    },
  },
})
