/** @type {import('tailwindcss').Config} */

// Neutral/text/foreground colors are CSS variables (RGB channels) so the theme
// can flip between light and dark while opacity utilities keep working. The
// accent (brand) palette is fixed and reads well on both themes.
const ch = (v) => `rgb(var(${v}) / <alpha-value>)`

export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ink: {
          950: ch('--ink-950'),
          900: ch('--ink-900'),
          850: ch('--ink-850'),
          800: ch('--ink-800'),
          700: ch('--ink-700'),
          600: ch('--ink-600'),
        },
        // Foreground/text tokens (formerly hard white + slate scale).
        white: ch('--fg'),
        fg: ch('--fg'),
        onaccent: ch('--onaccent'),
        accentSoft: ch('--accent-soft'),
        slate: {
          100: ch('--slate-100'),
          200: ch('--slate-200'),
          300: ch('--slate-300'),
          400: ch('--slate-400'),
          500: ch('--slate-500'),
          600: ch('--slate-600'),
        },
        accent: {
          DEFAULT: '#7c6cf0',
          50: '#f1f0fe',
          100: '#e4e1fd',
          200: '#cdc8fb',
          300: '#aea5f7',
          400: '#9384f2',
          500: '#7c6cf0',
          600: '#6a52e3',
          700: '#5b41c8',
          800: '#4b37a2',
          900: '#3f3281',
        },
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'sans-serif',
        ],
      },
      borderRadius: {
        xl: '0.875rem',
        '2xl': '1.125rem',
      },
      boxShadow: {
        soft: '0 1px 2px rgba(0,0,0,0.04), 0 6px 20px -8px var(--shadow)',
        glow: '0 0 0 1px rgba(124, 108, 240, 0.4), 0 8px 32px -8px rgba(124, 108, 240, 0.35)',
      },
      keyframes: {
        shimmer: {
          '100%': { transform: 'translateX(100%)' },
        },
        'fade-in': {
          from: { opacity: '0', transform: 'translateY(4px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
      },
      animation: {
        shimmer: 'shimmer 1.6s infinite',
        'fade-in': 'fade-in 0.2s ease-out',
      },
    },
  },
  plugins: [],
}
