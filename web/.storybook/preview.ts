import type { Preview } from '@storybook/react';
import '../src/styles/index.css'; // Import TailwindCSS and global styles

const preview: Preview = {
  parameters: {
    actions: { argTypesRegex: '^on[A-Z].*' },
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/,
      },
    },
    backgrounds: {
      default: 'page',
      values: [
        { name: 'page', value: '#fbfcfd' },
        { name: 'surface', value: '#ffffff' },
        { name: 'raised', value: '#f0f2f6' },
        { name: 'ink', value: '#0b0d14' },
      ],
    },
    viewport: {
      viewports: {
        mobile: { name: 'Mobile', styles: { width: '375px', height: '667px' } },
        tablet: { name: 'Tablet', styles: { width: '768px', height: '1024px' } },
        desktop: { name: 'Desktop', styles: { width: '1280px', height: '800px' } },
      },
    },
    a11y: {
      config: {
        rules: [
          { id: 'color-contrast', enabled: true },
          { id: 'label', enabled: true },
          { id: 'button-name', enabled: true },
        ],
      },
    },
  },
  tags: ['autodocs'],
};

export default preview;
