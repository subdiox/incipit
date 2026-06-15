/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ink: {
          950: '#0b0b0f',
          900: '#101017',
          850: '#15151f',
          800: '#1a1a26',
          700: '#23232f',
          600: '#2e2e3c',
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
        soft: '0 4px 24px -8px rgba(0, 0, 0, 0.6)',
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
