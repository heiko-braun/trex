import { fileURLToPath, URL } from 'node:url'
import type { IncomingMessage } from 'node:http'

import { defineConfig, loadEnv, type ProxyOptions } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd())
  const apiTarget = env.VITE_API_BASE_URL || 'http://localhost:8080'

  // The SPA has a client-side route under /definitions/:tenant/:name that
  // collides with the backend's own /definitions path. vue-router handles
  // it in-browser and never hits this dev server, but a hard refresh (or
  // typing the URL) sends a real top-level GET for that path, which the
  // proxy below would otherwise forward straight to the backend instead of
  // letting Vite serve index.html — and the backend then 401s a bare
  // navigation request (no Bearer header). Detect real page navigations
  // (Sec-Fetch-Dest: document, with an Accept: text/html fallback) and
  // rewrite them to /index.html; real fetch/XHR calls still proxy as-is.
  // Ported from com.sixt.web.managed-agents/vite.config.ts.
  function bypassNavigation(req: IncomingMessage): string | undefined {
    const dest = req.headers['sec-fetch-dest']
    const accept = req.headers.accept
    const isNavigation = dest === 'document' || (typeof accept === 'string' && accept.includes('text/html'))
    return isNavigation ? '/index.html' : undefined
  }

  const proxyEntry: ProxyOptions = {
    target: apiTarget,
    changeOrigin: true,
    bypass: bypassNavigation,
  }

  return {
    plugins: [vue()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 8484,
      proxy: {
        '/api': proxyEntry,
        '/definitions': proxyEntry,
        '/agents': proxyEntry,
      },
    },
  }
})
