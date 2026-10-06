import type { StorybookConfig } from '@storybook/react-vite';
import { mergeConfig } from 'vite';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const config: StorybookConfig = {
  stories: [
    '../src/design-system/**/*.mdx',
    '../src/design-system/**/*.stories.@(js|jsx|ts|tsx)',
    '../src/pages/**/*.stories.@(js|jsx|ts|tsx)',
    '../src/components/consent/**/*.stories.@(js|jsx|ts|tsx)',
    '../src/components/approvals/**/*.stories.@(js|jsx|ts|tsx)',
    '../src/components/icons/**/*.stories.@(js|jsx|ts|tsx)',
  ],
  addons: [
    '@storybook/addon-a11y',
    '@storybook/addon-docs',
    '@storybook/addon-themes',
    '@storybook/addon-vitest',
  ],
  framework: {
    name: '@storybook/react-vite',
    options: {},
  },
  docs: {
    defaultName: 'Documentation',
  },
  viteFinal: async (config) => {
    return mergeConfig(config, {
      resolve: {
        alias: {
          '@design-system': path.resolve(__dirname, '../src/design-system'),
          '@services': path.resolve(__dirname, '../src/services'),
          '@copy': path.resolve(__dirname, '../src/copy'),
          '@components': path.resolve(__dirname, '../src/components'),
          '@hooks': path.resolve(__dirname, '../src/hooks'),
          '@utils': path.resolve(__dirname, '../src/utils'),
          '@app-types': path.resolve(__dirname, '../src/types'),
          '@styles': path.resolve(__dirname, '../src/styles'),
        },
      },
    });
  },
  staticDirs: ['../public'],
};

export default config;
