import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import LatencySamplePlot from '../LatencySamplePlot.vue'

const samples = [
  { seq: 1, timestamp: '2026-09-20T12:00:00.000Z', latency_ms: 24, success: true },
  { seq: 2, timestamp: '2026-09-20T12:00:01.000Z', latency_ms: 0, success: false, error: '连接超时' },
  { seq: 3, timestamp: '2026-09-20T12:00:02.000Z', latency_ms: 31, success: true },
]

function mountPlot() {
  const wrapper = mount(LatencySamplePlot, {
    props: {
      samples,
      hoveredIndex: null,
      pinnedIndex: null,
      width: 760,
      height: 112,
    },
  })
  const svg = wrapper.find('svg')
  Object.defineProperty(svg.element, 'getBoundingClientRect', {
    configurable: true,
    value: () => ({ left: 0, top: 0, width: 760, height: 112, right: 760, bottom: 112 }),
  })
  return { wrapper, svg }
}

describe('LatencySamplePlot interaction contract', () => {
  it('captures the nearest sample on the shared time axis without requiring a point hit', async () => {
    const { wrapper, svg } = mountPlot()

    // x=52 is the first sample's time position; the event targets the plot, not its small circle.
    await svg.trigger('mousemove', { clientX: 52 })
    expect(wrapper.emitted('hover')?.at(-1)).toEqual([0])

    // The second sample is a failure and is still a selectable time position.
    await svg.trigger('mousemove', { clientX: 430 })
    expect(wrapper.emitted('hover')?.at(-1)).toEqual([1])
  })

  it('keeps hover browsing independent from pinning and supports keyboard cancellation', async () => {
    const { wrapper, svg } = mountPlot()

    await svg.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.emitted('hover')?.at(-1)).toEqual([1])

    await wrapper.setProps({ hoveredIndex: 1 })
    await svg.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('pin')?.at(-1)).toEqual([1])

    await svg.trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('unpin')).toHaveLength(1)
  })
})
