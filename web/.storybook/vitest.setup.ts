import { beforeAll, beforeEach, expect, vi } from 'vitest';
import { commands, page } from 'vitest/browser';
import { setProjectAnnotations } from '@storybook/react';
import * as a11yAnnotations from '@storybook/addon-a11y/preview';
import preview from './preview';
import { storyNow } from '../src/storybook/fixtures';
import { SIDEBAR_STORAGE_KEY } from '../src/design-system/theme/themePreference';
import { storyViewports, viewportDimensions, type MatrixViewport } from './viewports';

declare const __STORYBOOK_TEST_THEME__: 'light' | 'dark';
declare const __STORYBOOK_TEST_VIEWPORT_NAME__: MatrixViewport;

declare module 'vitest/internal/browser' {
  interface BrowserCommands {
    resetStoryCapture(): Promise<void>;
    prepareStoryCapture(): Promise<void>;
  }
}

function removeStorybookAnimationPause() {
  // Portable stories pause animations before afterEach and leak the pause rule when a hook fails.
  for (const element of document.head.querySelectorAll('style')) {
    const rules = element.sheet?.cssRules;
    if (rules?.length !== 1) continue;
    const rule = rules[0];
    if (rule instanceof CSSStyleRule && rule.selectorText.split(',').some(selector => selector.trim() === '*')
      && rule.style.getPropertyValue('animation-play-state') === 'paused'
      && rule.style.getPropertyPriority('animation-play-state') === 'important'
      && rule.style.getPropertyValue('transition') === 'none'
      && rule.style.getPropertyPriority('transition') === 'important') {
      element.remove();
    }
  }
}

async function waitForRenderFrames() {
  for (let frame = 0; frame < 2; frame++) {
    const { promise, resolve } = Promise.withResolvers<void>();
    requestAnimationFrame(() => resolve());
    await promise;
  }
}

const theme = __STORYBOOK_TEST_THEME__;
const viewportName = __STORYBOOK_TEST_VIEWPORT_NAME__;
const viewport = viewportDimensions(viewportName);
const annotations = setProjectAnnotations([
  a11yAnnotations,
  preview,
  {
    initialGlobals: { theme, viewport: { value: viewportName, isRotated: false } },
    afterEach: async (context) => {
      const actual = { width: window.innerWidth, height: window.innerHeight };
      const selected = context.globals.viewport?.value;
      const expected = selected && selected in storyViewports
        ? viewportDimensions(selected as keyof typeof storyViewports) : viewport;
      expect(actual).toEqual(expected);
      // Screen stories must exercise this project's actual media-query breakpoint, not a story override.
      if (context.id.startsWith('screens-')) {
        expect(actual).toEqual(viewport);
        expect(context.globals.theme).toBe(theme);
      }
      removeStorybookAnimationPause();
      await document.fonts.ready;
      await waitForRenderFrames();
      // Native SVG opacity must commit its final CSS before reduced motion stops it and resets SVG attributes.
      for (const animation of document.getAnimations()) {
        if (animation.playState !== 'idle' && animation.playState !== 'finished'
          && Number.isFinite(animation.effect?.getComputedTiming().endTime)) animation.finish();
      }
      await waitForRenderFrames();
      // Plays retain normal motion; capture resets remaining JS-driven movement to its final state.
      await commands.prepareStoryCapture();
      await waitForRenderFrames();
      if (theme !== 'light' || viewportName !== 'desktop' || context.globals.theme !== 'light'
        || actual.width !== viewport.width || actual.height !== viewport.height) return;
      // Capture the body so Radix portals outside the story canvas are reviewed too.
      const effectiveTheme = context.globals.theme === 'dark' ? 'dark' : 'light';
      const screenshotTheme = effectiveTheme === theme ? theme : `${effectiveTheme}-in-${theme}`;
      const actualSize = `${actual.width}x${actual.height}`;
      const projectSize = `${viewport.width}x${viewport.height}`;
      const screenshotSize = actualSize === projectSize ? actualSize : `${actualSize}-in-${projectSize}`;
      await expect.element(page.elementLocator(document.body)).toMatchScreenshot(`${context.id.replace('--', '__')}-${screenshotTheme}-${screenshotSize}`, {
        screenshotOptions: { animations: 'disabled', caret: 'hide' },
      });
    },
  },
]);

beforeEach(async () => {
  removeStorybookAnimationPause();
  window.localStorage.removeItem(SIDEBAR_STORAGE_KEY);
  await commands.resetStoryCapture();
});

beforeAll(() => {
  vi.spyOn(Date, 'now').mockReturnValue(storyNow);
  return annotations.beforeAll();
});
