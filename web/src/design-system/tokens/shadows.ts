/**
 * Design System Shadow Tokens
 *
 * Cards and panels sit on the page behind a hairline, not a shadow. One
 * floating shadow (`float`, exposed as `shadow-float` in colors.css) lifts
 * overlays: modals, dropdowns, popovers and toasts.
 */

export const shadows = {
  // The one floating shadow for overlays
  float: '0 1px 2px rgba(17, 24, 39, 0.04), 0 12px 32px rgba(17, 24, 39, 0.08)',

  // Standard elevation scale
  none: 'none',
  sm: '0 1px 2px 0 rgb(0 0 0 / 0.05)',
  base: '0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)',
  md: '0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)',
  lg: '0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)',
  xl: '0 20px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1)',
  '2xl': '0 25px 50px -12px rgb(0 0 0 / 0.25)',

  // Focus rings
  'focus-ring': '0 0 0 2px rgb(10 122 102)',
  'focus-ring-error': '0 0 0 2px rgb(180 35 24)',
} as const;

/**
 * Semantic shadow mapping for common use cases
 */
export const elevationLevels = {
  flat: shadows.none,
  raised: shadows.none,
  floating: shadows.float,
  overlay: shadows.float,
  modal: shadows.float,
} as const;

export type Shadows = typeof shadows;
export type ElevationLevels = typeof elevationLevels;
