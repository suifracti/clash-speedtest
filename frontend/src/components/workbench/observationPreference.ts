import { freezeLatencyWindow, type LatencyWindowMode } from './latencyRequestGuard'

type Slot = 'selected' | 'h' | 'd'
export function readObservationPreference(slot: Slot): LatencyWindowMode {
  const fallback: LatencyWindowMode = slot === 'd' ? '7d' : '6h'
  try {
    const value = localStorage.getItem(`observation-window-${slot}`) as LatencyWindowMode
    freezeLatencyWindow(value)
    if (slot !== 'selected' && !value.endsWith(slot)) return fallback
    return value
  } catch { return fallback }
}

export function saveObservationPreference(slot: Slot, value: LatencyWindowMode) {
  freezeLatencyWindow(value)
  try { localStorage.setItem(`observation-window-${slot}`, value) } catch { /* Storage may be unavailable. */ }
}
