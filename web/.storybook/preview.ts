import { createElement, useLayoutEffect, type ReactNode } from 'react';
import type { Preview } from '@storybook/react';
import { withThemeByDataAttribute } from '@storybook/addon-themes';
import { ThemeProvider, useTheme } from '../src/design-system/theme/ThemeProvider';
import { writeThemePreference } from '../src/design-system/theme/themePreference';
import '../src/styles/index.css';

function StoryTheme({ theme, children }: { theme: 'light' | 'dark'; children: ReactNode }) {
  const { setTheme } = useTheme();
  useLayoutEffect(() => { setTheme(theme); }, [setTheme, theme]);
  return children;
}

const preview: Preview = {
  initialGlobals: {
    theme: 'light',
    viewport: { value: 'desktop', isRotated: false },
  },
  beforeEach: ({ globals }) => {
    writeThemePreference(globals.theme === 'dark' ? 'dark' : 'light');
  },
  decorators: [
    withThemeByDataAttribute({
      themes: { light: 'light', dark: 'dark' },
      defaultTheme: 'light',
      attributeName: 'data-theme',
      parentSelector: 'html',
    }),
    (Story, context) => {
      const theme = context.globals.theme === 'dark' ? 'dark' : 'light';
      return createElement(ThemeProvider, {
        key: theme,
        children: createElement(StoryTheme, { theme, children: createElement(Story) }),
      });
    },
  ],
  parameters: {
    controls: { matchers: { color: /(background|color)$/i, date: /Date$/ } },
    viewport: {
      options: {
        narrow320: { name: 'Narrow 320px', styles: { width: '320px', height: '800px' } },
        mobile: { name: 'Mobile', styles: { width: '375px', height: '812px' } },
        tablet: { name: 'Tablet', styles: { width: '768px', height: '1024px' } },
        desktop: { name: 'Desktop', styles: { width: '1280px', height: '900px' } },
      },
    },
    a11y: {
      test: 'error',
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
