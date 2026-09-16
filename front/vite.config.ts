import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  return {
    plugins: [react()],
    server: {
      // ローカル開発時は VITE_DEV_API_PROXY(例: https://xxxx.cloudfront.net)へ /api を転送する
      proxy: env.VITE_DEV_API_PROXY
        ? { '/api': { target: env.VITE_DEV_API_PROXY, changeOrigin: true } }
        : undefined,
    },
  }
})
