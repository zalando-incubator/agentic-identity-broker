import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import istanbul from 'vite-plugin-istanbul'
import path from 'path'

const browserCoverage = process.env.VITE_COVERAGE === '1'

export default defineConfig(({ command }) => ({
  plugins: [react(), ...(browserCoverage && command === 'build' ? [istanbul({
    include: 'src/**/*',
    exclude: ['**/*.stories.*'],
    extension: ['.ts', '.tsx'],
    forceBuildInstrument: true,
    cwd: __dirname,
  })] : [])],
  base: '/',
  build: {
    outDir: 'dist',
    sourcemap: browserCoverage ? 'hidden' : false,
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
      '@components': path.resolve(__dirname, './src/components'),
      '@hooks': path.resolve(__dirname, './src/hooks'),
      '@services': path.resolve(__dirname, './src/services'),
      '@types': path.resolve(__dirname, './src/types'),
      '@utils': path.resolve(__dirname, './src/utils'),
      '@assets': path.resolve(__dirname, './src/assets'),
      '@styles': path.resolve(__dirname, './src/styles'),
    },
  },
}))
