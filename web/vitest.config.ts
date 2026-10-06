import { defineConfig, defineProject } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { playwright } from '@vitest/browser-playwright';
import { storybookTest } from '@storybook/addon-vitest/vitest-plugin';
import path from 'node:path';
import { viewportDimensions, type MatrixViewport } from './.storybook/viewports';

function storybookProject(theme: 'light' | 'dark', size: MatrixViewport) {
  const viewport = viewportDimensions(size);
  return defineProject({
    extends: true,
    define: {
      __STORYBOOK_TEST_THEME__: JSON.stringify(theme),
      __STORYBOOK_TEST_VIEWPORT_NAME__: JSON.stringify(size),
    },
    optimizeDeps: { include: ['@tanstack/react-query', 'axios'] },
    plugins: [
      storybookTest({ configDir: path.resolve(__dirname, '.storybook') }),
      {
        name: 'storybook-theme-cache',
        enforce: 'post',
        // Storybook otherwise gives concurrent theme projects the same dependency optimizer.
        config: () => ({ cacheDir: path.resolve(__dirname, `node_modules/.cache/storybook-vitest/${theme}-${size}`) }),
      },
    ],
    test: {
      name: size === 'desktop' ? `storybook-${theme}` : `storybook-${theme}-${viewport.width}`,
      setupFiles: ['./.storybook/vitest.setup.ts'],
      fileParallelism: false,
      testTimeout: 20_000,
      browser: {
        enabled: true,
        headless: true,
        ...(process.env.PLAYWRIGHT_WS_ENDPOINT ? { api: { host: '127.0.0.1' } } : {}),
        provider: playwright({
          launchOptions: { channel: 'chromium' },
          contextOptions: { colorScheme: theme, locale: 'en-US', timezoneId: 'UTC', viewport },
          ...(process.env.PLAYWRIGHT_WS_ENDPOINT ? { connectOptions: { wsEndpoint: process.env.PLAYWRIGHT_WS_ENDPOINT, exposeNetwork: '<loopback>' } } : {}),
        }),
        instances: [{ browser: 'chromium' }],
        commands: {
          resetStoryCapture: async ({ page }) => {
            await page.emulateMedia({ reducedMotion: 'no-preference' });
            await page.mouse.move(-1, -1);
          },
          prepareStoryCapture: async ({ page }) => {
            await page.emulateMedia({ reducedMotion: 'reduce' });
          },
        },
        screenshotDirectory: '.storybook/__screenshots__',
        screenshotFailures: false,
        expect: {
          toMatchScreenshot: {
            comparatorName: 'pixelmatch',
            comparatorOptions: { threshold: 0.2, allowedMismatchedPixelRatio: 0 },
            resolveScreenshotPath: ({ arg, ext, root, project }) =>
              path.resolve(root, project.config.browser.screenshotDirectory ?? '.storybook/__screenshots__', `${arg.replace('__', '--')}${ext}`),
            resolveDiffPath: ({ arg, ext, root }) =>
              path.resolve(root, '.storybook/test-results', `${arg.replace('__', '--')}${ext}`),
          },
        },
      },
    },
  });
}

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@design-system': path.resolve(__dirname, './src/design-system'),
      '@copy': path.resolve(__dirname, './src/copy'),
      '@components': path.resolve(__dirname, './src/components'),
      '@services': path.resolve(__dirname, './src/services'),
      '@hooks': path.resolve(__dirname, './src/hooks'),
      '@app-types': path.resolve(__dirname, './src/types'),
      '@utils': path.resolve(__dirname, './src/utils'),
      '@assets': path.resolve(__dirname, './src/assets'),
      '@styles': path.resolve(__dirname, './src/styles'),
    },
  },
  test: {
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['**/*.stories.*'],
      reporter: ['text', 'html', 'clover', 'json', ['lcovonly', { projectRoot: '..' }]],
    },
    projects: [
      defineProject({
        extends: true,
        test: {
          name: 'unit',
          globals: true,
          environment: 'jsdom',
          environmentOptions: { jsdom: { resources: 'usable' } },
          setupFiles: ['./vitest.setup.ts'],
          css: true,
          pool: 'threads',
          maxWorkers: 4,
          testTimeout: 10_000,
          include: ['src/**/*.{test,spec}.{ts,tsx}', 'build/**/*.test.ts', 'eslint-rules/**/*.test.js'],
          exclude: ['build/decisionBundle.test.ts'],
        },
      }),
      storybookProject('light', 'desktop'),
      storybookProject('dark', 'desktop'),
      storybookProject('light', 'tablet'),
      storybookProject('dark', 'tablet'),
      storybookProject('light', 'mobile'),
      storybookProject('dark', 'mobile'),
      defineProject({
        extends: true,
        test: {
          name: 'bundle',
          environment: 'node',
          include: ['build/decisionBundle.test.ts'],
        },
      }),
    ],
  },
});
