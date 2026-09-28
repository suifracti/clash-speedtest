import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MetricHistoryPlot from '../MetricHistoryPlot.vue'

describe('shared download/service history plot', () => {
  it('shows a scoped time series and lets the user browse and pin actual records', async () => {
    const points = [
      { id: 'one', timestamp: '2026-09-27T01:00:00Z', value: 8.4, label: '8.4 Mbps', detail: '读取 1.0 MiB', success: true },
      { id: 'two', timestamp: '2026-09-27T02:00:00Z', value: 12, label: '12 Mbps', detail: '读取 2.0 MiB', success: true },
    ]
    const wrapper = mount(MetricHistoryPlot, { props: {
      points, unit: 'Mbps', windowSince: '2026-09-27T00:00:00Z', windowUntil: '2026-09-27T03:00:00Z',
      hoveredIndex: null, pinnedIndex: null, width: 760,
    } })
    const svg = wrapper.get('svg')
    Object.defineProperty(svg.element, 'getBoundingClientRect', { configurable: true, value: () => ({ left: 0, top: 0, width: 760, height: 132 }) })
    await svg.trigger('mousemove', { clientX: 300, clientY: 60 })
    expect(wrapper.emitted('hover')?.at(-1)).toEqual([0])
    await wrapper.setProps({ hoveredIndex: 0 })
    expect(wrapper.get('.metric-hover-label').text()).toBe('8.4 Mbps')
    await svg.trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.emitted('hover')?.at(-1)).toEqual([1])
    await wrapper.setProps({ hoveredIndex: 1 })
    await svg.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('pin')?.at(-1)).toEqual([1])
    expect(wrapper.findAll('circle')).toHaveLength(2)
  })
})
