import { defineConfig, defineProject } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { playwright } from '@vitest/browser-playwright';
import { storybookTest } from '@storybook/addon-vitest/vitest-plugin';
import path from 'node:path';

function storybookProject(theme: 'light' | 'dark') {
  return defineProject({
    extends: true,
    define: { __STORYBOOK_TEST_THEME__: JSON.stringify(theme) },
    optimizeDeps: { include: ['@tanstack/react-query', 'axios'] },
    plugins: [
      storybookTest({ configDir: path.resolve(__dirname, '.storybook') }),
      {
        name: 'storybook-theme-cache',
        enforce: 'post',
        // Storybook otherwise gives concurrent theme projects the same dependency optimizer.
        config: () => ({ cacheDir: path.resolve(__dirname, `node_modules/.cache/storybook-vitest/${theme}`) }),
      },
    ],
    test: {
      name: `storybook-${theme}`,
      setupFiles: ['./.storybook/vitest.setup.ts'],
      testTimeout: 20_000,
      browser: {
        enabled: true,
        headless: true,
        provider: playwright({ contextOptions: { colorScheme: theme, locale: 'en-US', timezoneId: 'UTC' } }),
        instances: [{ browser: 'chromium' }],
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
      '@types': path.resolve(__dirname, './src/types'),
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
          testTimeout: 10_000,
          include: ['src/**/*.{test,spec}.{ts,tsx}', 'build/**/*.test.ts', 'eslint-rules/**/*.test.js'],
          exclude: ['build/decisionBundle.test.ts'],
        },
      }),
      storybookProject('light'),
      storybookProject('dark'),
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
