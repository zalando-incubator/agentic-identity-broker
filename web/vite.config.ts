import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'
import { themeInitPlugin } from './build/themeInitPlugin'
import { decisionModulesPlugin } from './build/decisionModulesPlugin'
import { assetCompressionPlugin } from './build/assetCompressionPlugin'

export default defineConfig({
  plugins: [
    react(),
    themeInitPlugin(),
    decisionModulesPlugin(),
    assetCompressionPlugin(),
    {
      name: 'react-router-production',
      apply: 'build',
      enforce: 'pre',
      // React Router's published exports still select development code: remix-run/react-router#14102.
      resolveId(source) {
        if (source === 'react-router') return path.resolve(__dirname, 'node_modules/react-router/dist/production/index.mjs')
        if (source === 'react-router/dom') return path.resolve(__dirname, 'node_modules/react-router/dist/production/dom-export.mjs')
      },
    },
  ],
  base: '/',
  build: {
    outDir: 'dist',
    manifest: true,
    sourcemap: false,
    minify: 'terser',
  },
  server: {
    port: 3000,
    strictPort: false,
    open: false,

    // Conditionally enable polling and HMR for Docker (only when VITE_USE_POLLING=true)
    ...(process.env.VITE_USE_POLLING === 'true' ? {
      host: '0.0.0.0',
      watch: {
        usePolling: true,
        interval: 300,  // Same as Air's 300ms poll_interval for consistency
      },
      hmr: {
        host: process.env.VITE_HMR_HOST || 'localhost',
        port: 3000,
        protocol: 'ws',
      },
    } : {}),

    proxy: {
      // Proxy backend namespaces to the Go server; Vite serves all SPA view routes.
      '^/(api|oauth2|\\.well-known|health)(/|$)': {
        target: process.env.VITE_API_URL || 'http://localhost:8000',
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq) => {
            proxyReq.setHeader('X-Remote-User', 'dev@example.com');
          });
        },
      },
    },
  },
  resolve: {
    alias: {
      '@design-system': path.resolve(__dirname, './src/design-system'),
      '@copy': path.resolve(__dirname, './src/copy'),
      '@components': path.resolve(__dirname, './src/components'),
      '@hooks': path.resolve(__dirname, './src/hooks'),
      '@services': path.resolve(__dirname, './src/services'),
      '@types': path.resolve(__dirname, './src/types'),
      '@utils': path.resolve(__dirname, './src/utils'),
      '@assets': path.resolve(__dirname, './src/assets'),
      '@styles': path.resolve(__dirname, './src/styles'),
    },
  },
})
