/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        canvas: 'var(--canvas)',
        card: {
          DEFAULT: 'var(--card-bg)',
          subtle: 'var(--card-subtle)',
          hover: 'var(--card-hover)',
        },
        border: {
          DEFAULT: 'var(--border)',
          subtle: 'var(--border-subtle)',
          focus: 'var(--border-focus)',
        },
        content: {
          main: 'var(--text-main)',
          secondary: 'var(--text-secondary)',
          muted: 'var(--text-muted)',
        },
        brand: {
          DEFAULT: 'var(--primary)',
          hover: 'var(--primary-hover)',
          subtle: 'var(--primary-subtle)',
        },
        status: {
          success: 'var(--success)',
          'success-bg': 'var(--success-bg)',
          warning: 'var(--warning)',
          'warning-bg': 'var(--warning-bg)',
          danger: 'var(--danger)',
          'danger-bg': 'var(--danger-bg)',
        }
      }
    },
  },
  plugins: [],
}
