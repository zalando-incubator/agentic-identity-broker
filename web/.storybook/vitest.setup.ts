import { beforeAll, expect } from 'vitest';
import { page } from 'vitest/browser';
import { setProjectAnnotations } from '@storybook/react';
import * as a11yAnnotations from '@storybook/addon-a11y/preview';
import preview from './preview';

declare const __STORYBOOK_TEST_THEME__: 'light' | 'dark';

const theme = __STORYBOOK_TEST_THEME__;
const annotations = setProjectAnnotations([
  a11yAnnotations,
  preview,
  {
    initialGlobals: { theme },
    afterEach: async (context) => {
      await document.fonts.ready;
      // The body includes Radix portals outside the canvas, so open overlays are reviewed too.
      // Preserve the story separator because Vitest otherwise collapses repeated hyphens.
      await expect.element(page.elementLocator(document.body)).toMatchScreenshot(`${context.id.replace('--', '__')}-${theme}`, {
        screenshotOptions: { animations: 'disabled', caret: 'hide' },
      });
    },
  },
]);

beforeAll(annotations.beforeAll);
