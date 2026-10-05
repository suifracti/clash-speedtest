import {desktop} from './api'

export type Theme = 'light' | 'dark'

export function savedTheme(): Theme {
  try { return window.localStorage.getItem('theme') === 'dark' ? 'dark' : 'light' }
  catch { return 'light' }
}

export async function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#11151e' : '#f5f7fb')
  try { window.localStorage.setItem('theme', theme) } catch { /* The current window still uses the chosen theme. */ }
  if (desktop()) {
    const update = window.go?.desktop?.App?.SetWindowTheme
    if (update) await update(theme)
  }
}
