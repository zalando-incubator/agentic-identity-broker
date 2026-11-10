/**
 * Design System Color Tokens (TypeScript)
 * Semantic color tokens that map to CSS custom properties
 *
 * Source of Truth: colors.css
 * Use these constants for programmatic access to color values
 */

export const semanticColors = {
  // ACCENT - Jade. Actions, links and the granted state.
  accent: {
    default: 'var(--color-accent)',
    hover: 'var(--color-accent-hover)',
    ink: 'var(--color-accent-ink)',
    soft: 'var(--color-accent-soft)',
  },

  // INK RAMP - Page, surfaces and hairlines
  surface: {
    page: 'var(--color-page)',
    default: 'var(--color-surface)',
    raised: 'var(--color-raised)',
    line: 'var(--color-line)',
    lineStrong: 'var(--color-line-strong)',
  },

  // PRIMARY - Trust. Ink for text, the accent for actions.
  trust: {
    deep: 'var(--color-trust-deep)',
    default: 'var(--color-trust)',
    hover: 'var(--color-trust-hover)',
    light: 'var(--color-trust-light)',
  },

  // ACTION - CTA (Call-to-Action)
  cta: {
    default: 'var(--color-cta)',
    hover: 'var(--color-cta-hover)',
    light: 'var(--color-cta-light)',
  },

  // SUCCESS
  success: {
    primary: 'var(--color-success-primary)',
    hover: 'var(--color-success-hover)',
    light: 'var(--color-success-light)',
    dark: 'var(--color-success-dark)',
  },

  // ERROR
  error: {
    primary: 'var(--color-error-primary)',
    hover: 'var(--color-error-hover)',
    light: 'var(--color-error-light)',
    dark: 'var(--color-error-dark)',
  },

  // WARNING (unified with CTA)
  warning: {
    primary: 'var(--color-warning-primary)',
    hover: 'var(--color-warning-hover)',
    light: 'var(--color-warning-light)',
    dark: 'var(--color-warning-dark)',
  },

  // INFO
  info: {
    primary: 'var(--color-info-primary)',
    hover: 'var(--color-info-hover)',
    light: 'var(--color-info-light)',
    dark: 'var(--color-info-dark)',
  },

  // COOL NEUTRALS (replaces gray)
  neutral: {
    50: 'var(--color-neutral-50)',
    100: 'var(--color-neutral-100)',
    200: 'var(--color-neutral-200)',
    300: 'var(--color-neutral-300)',
    400: 'var(--color-neutral-400)',
    500: 'var(--color-neutral-500)',
    600: 'var(--color-neutral-600)',
    700: 'var(--color-neutral-700)',
    800: 'var(--color-neutral-800)',
    900: 'var(--color-neutral-900)',
  },

  // SEMANTIC ALIASES - Text
  text: {
    primary: 'var(--color-text-primary)',
    secondary: 'var(--color-text-secondary)',
    tertiary: 'var(--color-text-tertiary)',
    disabled: 'var(--color-text-disabled)',
    inverse: 'var(--color-text-inverse)',
  },

  // SEMANTIC ALIASES - Background
  bg: {
    primary: 'var(--color-bg-primary)',
    secondary: 'var(--color-bg-secondary)',
    tertiary: 'var(--color-bg-tertiary)',
    elevated: 'var(--color-bg-elevated)',
    overlay: 'var(--color-bg-overlay)',
  },

  // SEMANTIC ALIASES - Border
  border: {
    primary: 'var(--color-border-primary)',
    secondary: 'var(--color-border-secondary)',
    focus: 'var(--color-border-focus)',
  },
} as const;

export type SemanticColors = typeof semanticColors;

// Flat export for Tailwind className usage
export const colorTokens = {
  // Trust
  'trust-deep': semanticColors.trust.deep,
  trust: semanticColors.trust.default,
  'trust-hover': semanticColors.trust.hover,
  'trust-light': semanticColors.trust.light,

  // CTA
  cta: semanticColors.cta.default,
  'cta-hover': semanticColors.cta.hover,
  'cta-light': semanticColors.cta.light,

  // Success
  'success-primary': semanticColors.success.primary,
  'success-hover': semanticColors.success.hover,
  'success-light': semanticColors.success.light,
  'success-dark': semanticColors.success.dark,

  // Error
  'error-primary': semanticColors.error.primary,
  'error-hover': semanticColors.error.hover,
  'error-light': semanticColors.error.light,
  'error-dark': semanticColors.error.dark,

  // Warning
  'warning-primary': semanticColors.warning.primary,
  'warning-hover': semanticColors.warning.hover,
  'warning-light': semanticColors.warning.light,
  'warning-dark': semanticColors.warning.dark,

  // Info
  'info-primary': semanticColors.info.primary,
  'info-hover': semanticColors.info.hover,
  'info-light': semanticColors.info.light,
  'info-dark': semanticColors.info.dark,
} as const;
