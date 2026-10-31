var __assign = (this && this.__assign) || function () {
    __assign = Object.assign || function(t) {
        for (var s, i = 1, n = arguments.length; i < n; i++) {
            s = arguments[i];
            for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p))
                t[p] = s[p];
        }
        return t;
    };
    return __assign.apply(this, arguments);
};
var __spreadArray = (this && this.__spreadArray) || function (to, from, pack) {
    if (pack || arguments.length === 2) for (var i = 0, l = from.length, ar; i < l; i++) {
        if (ar || !(i in from)) {
            if (!ar) ar = Array.prototype.slice.call(from, 0, i);
            ar[i] = from[i];
        }
    }
    return to.concat(ar || Array.prototype.slice.call(from));
};
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import istanbul from 'vite-plugin-istanbul';
import path from 'path';
var browserCoverage = process.env.VITE_COVERAGE === '1';
export default defineConfig(function (_a) {
    var command = _a.command;
    return ({
        plugins: __spreadArray([react()], (browserCoverage && command === 'build' ? [istanbul({
                include: 'src/**/*',
                exclude: ['**/*.stories.*'],
                extension: ['.ts', '.tsx'],
                forceBuildInstrument: true,
                cwd: __dirname,
            })] : []), true),
        base: '/',
        build: {
            outDir: 'dist',
            sourcemap: browserCoverage ? 'hidden' : false,
            minify: 'terser',
        },
        server: __assign(__assign({ port: 3000, strictPort: false, open: false }, (process.env.VITE_USE_POLLING === 'true' ? {
            host: '0.0.0.0',
            watch: {
                usePolling: true,
                interval: 300, // Same as Air's 300ms poll_interval for consistency
            },
            hmr: {
                host: process.env.VITE_HMR_HOST || 'localhost',
                port: 3000,
                protocol: 'ws',
            },
        } : {})), { proxy: {
                // Proxy backend namespaces to the Go server; Vite serves all SPA view routes.
                '^/(api|oauth2|\\.well-known|health)(/|$)': {
                    target: process.env.VITE_API_URL || 'http://localhost:8000',
                    changeOrigin: true,
                    configure: function (proxy) {
                        proxy.on('proxyReq', function (proxyReq) {
                            proxyReq.setHeader('X-Remote-User', 'dev@example.com');
                        });
                    },
                },
            } }),
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
    });
});
