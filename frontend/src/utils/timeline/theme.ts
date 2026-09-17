/**
 * Canvas theme resolution for the timeline.
 *
 * Colours are read from the existing CSS custom properties so the timeline follows the app's
 * light/dark theming instead of hard-coding a second palette.
 */

export interface TimelineTheme {
  background: string
  laneBand: string
  laneBandAlt: string
  laneBandHover: string
  baseline: string
  gridline: string
  gridlineMajor: string
  axisText: string
  laneText: string
  laneTextMuted: string
  success: string
  danger: string
  warning: string
  muted: string
  selection: string
  boundary: string
  border: string
}

export const DEFAULT_TIMELINE_THEME: TimelineTheme = {
  background: '#ffffff',
  laneBand: 'rgba(148, 163, 184, 0.05)',
  laneBandAlt: 'rgba(148, 163, 184, 0.10)',
  laneBandHover: 'rgba(37, 99, 235, 0.06)',
  baseline: 'rgba(148, 163, 184, 0.45)',
  gridline: 'rgba(148, 163, 184, 0.22)',
  gridlineMajor: 'rgba(148, 163, 184, 0.40)',
  axisText: '#64748b',
  laneText: '#0f172a',
  laneTextMuted: '#64748b',
  success: '#059669',
  danger: '#dc2626',
  warning: '#d97706',
  muted: '#64748b',
  selection: '#2563eb',
  boundary: '#7c3aed',
  border: '#e2e8f0',
}

function readVar(style: CSSStyleDeclaration, name: string, fallback: string): string {
  const value = style.getPropertyValue(name).trim()
  return value.length > 0 ? value : fallback
}

/** Resolves the theme from an element's computed style, falling back to the light defaults. */
export function resolveTimelineTheme(el: Element | null): TimelineTheme {
  if (!el || typeof window === 'undefined' || typeof window.getComputedStyle !== 'function') {
    return DEFAULT_TIMELINE_THEME
  }

  const style = window.getComputedStyle(el)
  const isDark = document.body?.classList.contains('dark-theme') || document.body?.classList.contains('dark')

  return {
    background: readVar(style, '--card-bg', DEFAULT_TIMELINE_THEME.background),
    laneBand: isDark ? 'rgba(148, 163, 184, 0.05)' : 'rgba(148, 163, 184, 0.07)',
    laneBandAlt: isDark ? 'rgba(148, 163, 184, 0.10)' : 'rgba(148, 163, 184, 0.13)',
    laneBandHover: isDark ? 'rgba(59, 130, 246, 0.10)' : 'rgba(37, 99, 235, 0.06)',
    baseline: isDark ? 'rgba(148, 163, 184, 0.45)' : 'rgba(100, 116, 139, 0.40)',
    gridline: isDark ? 'rgba(148, 163, 184, 0.20)' : 'rgba(100, 116, 139, 0.18)',
    gridlineMajor: isDark ? 'rgba(148, 163, 184, 0.38)' : 'rgba(100, 116, 139, 0.34)',
    axisText: readVar(style, '--text-muted', DEFAULT_TIMELINE_THEME.axisText),
    laneText: readVar(style, '--text-main', DEFAULT_TIMELINE_THEME.laneText),
    laneTextMuted: readVar(style, '--text-muted', DEFAULT_TIMELINE_THEME.laneTextMuted),
    success: readVar(style, '--success', DEFAULT_TIMELINE_THEME.success),
    danger: readVar(style, '--danger', DEFAULT_TIMELINE_THEME.danger),
    warning: readVar(style, '--warning', DEFAULT_TIMELINE_THEME.warning),
    muted: readVar(style, '--text-muted', DEFAULT_TIMELINE_THEME.muted),
    selection: readVar(style, '--primary', DEFAULT_TIMELINE_THEME.selection),
    boundary: isDark ? '#a78bfa' : '#7c3aed',
    border: readVar(style, '--border', DEFAULT_TIMELINE_THEME.border),
  }
}

export function toneColor(tone: 'success' | 'danger' | 'warning' | 'muted', theme: TimelineTheme): string {
  switch (tone) {
    case 'success':
      return theme.success
    case 'danger':
      return theme.danger
    case 'warning':
      return theme.warning
    default:
      return theme.muted
  }
}
