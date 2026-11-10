/**
 * Design System Typography Tokens
 *
 * Typography scale for the broker's design language: exact, sober, accountable.
 * Geist carries text and headings at weights 400 and 500. Geist Mono carries
 * identifiers, scopes and dates.
 */

export const typography = {
  fontFamily: {
    display: "'Geist', ui-sans-serif, system-ui, sans-serif",
    sans: "'Geist', ui-sans-serif, system-ui, sans-serif",
    mono: "'Geist Mono', ui-monospace, monospace",
  },
  fontSize: {
    xs: '0.75rem', // 12px
    sm: '0.875rem', // 14px
    base: '1rem', // 16px
    lg: '1.125rem', // 18px
    xl: '1.25rem', // 20px
    '2xl': '1.5rem', // 24px
    '3xl': '1.875rem', // 30px
    '4xl': '2.25rem', // 36px
    '5xl': '3rem', // 48px
  },
  fontWeight: {
    light: 300,
    normal: 400,
    medium: 500,
    semibold: 600,
    bold: 700,
    black: 900,
  },
  lineHeight: {
    tight: 1.2,
    snug: 1.375,
    normal: 1.5,
    relaxed: 1.625,
    loose: 2,
  },
  letterSpacing: {
    tighter: '-0.025em',
    tight: '-0.015em',
    normal: '0',
    wide: '0.025em',
    wider: '0.05em',
  },
} as const;

/**
 * Preset text styles for common use cases
 * Apply these as className strings in components
 */
export const textStyles = {
  h1: 'font-display text-4xl font-medium leading-tight tracking-tight',
  h2: 'font-display text-3xl font-medium leading-tight tracking-tight',
  h3: 'font-display text-2xl font-medium leading-snug tracking-tight',
  h4: 'font-display text-xl font-medium leading-snug',
  h5: 'font-display text-lg font-medium leading-snug',
  h6: 'font-display text-base font-medium leading-normal',
  body: 'font-sans text-base font-normal leading-normal',
  bodyLarge: 'font-sans text-lg font-normal leading-relaxed',
  bodySmall: 'font-sans text-sm font-normal leading-normal',
  caption: 'font-sans text-xs font-normal leading-normal',
  label: 'font-sans text-sm font-medium leading-normal',
  code: 'font-mono text-sm font-normal leading-normal',
} as const;

export type Typography = typeof typography;
export type TextStyles = typeof textStyles;
