import type { Config } from 'tailwindcss'

export default {
  content: [
    './src/**/*.{js,jsx,ts,tsx}',
    './index.html',
  ],
  theme: {
    extend: {
      colors: {
        // SEMANTIC TOKENS - Primary brand colors
        'trust-deep': 'var(--color-trust-deep)',
        'trust': 'var(--color-trust)',
        'trust-hover': 'var(--color-trust-hover)',
        'trust-light': 'var(--color-trust-light)',

        // SEMANTIC TOKENS - Action colors
        'cta': 'var(--color-cta)',
        'cta-hover': 'var(--color-cta-hover)',
        'cta-light': 'var(--color-cta-light)',

        // SEMANTIC TOKENS - Success
        'success-primary': 'var(--color-success-primary)',
        'success-hover': 'var(--color-success-hover)',
        'success-light': 'var(--color-success-light)',
        'success-dark': 'var(--color-success-dark)',

        // SEMANTIC TOKENS - Error
        'error-primary': 'var(--color-error-primary)',
        'error-hover': 'var(--color-error-hover)',
        'error-light': 'var(--color-error-light)',
        'error-dark': 'var(--color-error-dark)',

        // SEMANTIC TOKENS - Warning (unified with CTA)
        'warning-primary': 'var(--color-warning-primary)',
        'warning-hover': 'var(--color-warning-hover)',
        'warning-light': 'var(--color-warning-light)',
        'warning-dark': 'var(--color-warning-dark)',

        // SEMANTIC TOKENS - Info
        'info-primary': 'var(--color-info-primary)',
        'info-hover': 'var(--color-info-hover)',
        'info-light': 'var(--color-info-light)',
        'info-dark': 'var(--color-info-dark)',

        // WARM NEUTRALS - Complete scale (replaces gray)
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

        // SEMANTIC ALIASES - Contextual colors
        'text-primary': 'var(--color-text-primary)',
        'text-secondary': 'var(--color-text-secondary)',
        'text-tertiary': 'var(--color-text-tertiary)',
        'text-disabled': 'var(--color-text-disabled)',
        'text-inverse': 'var(--color-text-inverse)',

        'bg-primary': 'var(--color-bg-primary)',
        'bg-secondary': 'var(--color-bg-secondary)',
        'bg-tertiary': 'var(--color-bg-tertiary)',
        'bg-elevated': 'var(--color-bg-elevated)',
        'bg-overlay': 'var(--color-bg-overlay)',

        'border-primary': 'var(--color-border-primary)',
        'border-secondary': 'var(--color-border-secondary)',
        'border-focus': 'var(--color-border-focus)',


      },
      fontFamily: {
        display: ['Geist', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        sans: ['Geist', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['Geist Mono', 'ui-monospace', 'monospace'],
      },
      boxShadow: {
        // One floating shadow for overlays; cards use hairlines
        float: '0 1px 2px rgba(17, 24, 39, 0.04), 0 12px 32px rgba(17, 24, 39, 0.08)',
      },
      animation: {
        'fade-in': 'fadeIn 0.5s ease-out',
        'slide-up': 'slideUp 0.6s cubic-bezier(0.16, 1, 0.3, 1)',
        'scale-in': 'scaleIn 0.4s cubic-bezier(0.34, 1.56, 0.64, 1)',
        'progress-stripes': 'progressStripes 1s linear infinite',
        'progress-indeterminate': 'progressIndeterminate 1.5s ease-in-out infinite',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { transform: 'translateY(12px)', opacity: '0' },
          '100%': { transform: 'translateY(0)', opacity: '1' },
        },
        scaleIn: {
          '0%': { transform: 'scale(0.95)', opacity: '0' },
          '100%': { transform: 'scale(1)', opacity: '1' },
        },
        progressStripes: {
          '0%': { backgroundPosition: '1rem 0' },
          '100%': { backgroundPosition: '0 0' },
        },
        progressIndeterminate: {
          '0%': { left: '-30%' },
          '50%': { left: '100%' },
          '100%': { left: '100%' },
        },
      },
      typography: {
        DEFAULT: {
          css: {
            fontFamily: 'Geist, ui-sans-serif, system-ui, sans-serif',
          },
        },
      },
    },
  },
  plugins: [],
} satisfies Config
