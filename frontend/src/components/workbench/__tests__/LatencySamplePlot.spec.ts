import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import LatencySamplePlot from '../LatencySamplePlot.vue'
import type { WorkbenchLatencySample } from '../../../types'

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
    await wrapper.setProps({ hoveredIndex: 1 })
    expect(wrapper.get('.sample-hover-label').text()).toContain('#2 · 超时')
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

  it('aligns all samples of a test run strictly in one vertical column and enables smooth vertical hover across all 6 samples', async () => {
    const multiSamples = [
      { seq: 1, timestamp: '2026-09-20T12:00:00.000Z', latency_ms: 100, success: true },
      { seq: 2, timestamp: '2026-09-20T12:00:00.050Z', latency_ms: 100, success: true },
      { seq: 3, timestamp: '2026-09-20T12:00:00.100Z', latency_ms: 100, success: true },
      { seq: 4, timestamp: '2026-09-20T12:00:00.150Z', latency_ms: 100, success: true },
      { seq: 5, timestamp: '2026-09-20T12:00:00.200Z', latency_ms: 100, success: true },
      { seq: 6, timestamp: '2026-09-20T12:00:00.250Z', latency_ms: 100, success: true },
    ]
    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: multiSamples,
        windowSince: '2026-09-20T10:00:00.000Z',
        windowUntil: '2026-09-20T14:00:00.000Z',
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

    // Strict vertical column: all circles share identical cx
    const circles = svg.findAll('circle').filter((c) => c.attributes('cx') !== undefined)
    const firstCx = circles[0].attributes('cx')
    expect(firstCx).toBeDefined()
    circles.forEach((circle) => {
      expect(circle.attributes('cx')).toBe(firstCx)
    })

    // No floating tooltip card
    expect(wrapper.find('.plot-readout').exists()).toBe(false)

    // Moving vertically across the vertical range selects each of the 6 samples
    // plotTop = 16, plotBottom = 86 (112 - 26). Height = 70.
    // Slices: [16, 27.67], [27.67, 39.33], [39.33, 51], [51, 62.67], [62.67, 74.33], [74.33, 86]
    const testYs = [20, 32, 44, 56, 68, 80]
    const selectedIndices: (number | null)[] = []
    for (const clientY of testYs) {
      await svg.trigger('mousemove', { clientX: 398, clientY })
      const lastHover = (wrapper.emitted('hover')?.at(-1)?.[0] as number | null | undefined) ?? null
      selectedIndices.push(lastHover)
    }

    // Every sample index 0..5 must be selected in order
    expect(selectedIndices).toEqual([0, 1, 2, 3, 4, 5])
  })

  it('allows easily selecting among clustered low latencies even when an outlier high latency is present', async () => {
    const skewedSamples = [
      { seq: 1, timestamp: '2026-09-20T12:00:00.000Z', latency_ms: 3615, success: true },
      { seq: 2, timestamp: '2026-09-20T12:00:00.050Z', latency_ms: 1500, success: true },
      { seq: 3, timestamp: '2026-09-20T12:00:00.100Z', latency_ms: 325, success: true },
      { seq: 4, timestamp: '2026-09-20T12:00:00.150Z', latency_ms: 321, success: true },
      { seq: 5, timestamp: '2026-09-20T12:00:00.200Z', latency_ms: 320, success: true },
      { seq: 6, timestamp: '2026-09-20T12:00:00.250Z', latency_ms: 319, success: true },
    ]
    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: skewedSamples,
        windowSince: '2026-09-20T10:00:00.000Z',
        windowUntil: '2026-09-20T14:00:00.000Z',
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

    // Moving near the top dot (Y = 24) selects sample 0 (3615ms)
    await svg.trigger('mousemove', { clientX: 398, clientY: 24 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(0)

    // Moving near the second dot (Y = 35) selects sample 1 (1500ms)
    await svg.trigger('mousemove', { clientX: 398, clientY: 35 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(1)

    // Moving across the subsequent slices (Y = 46, 57, 68, 79) selects samples 2, 3, 4, 5
    await svg.trigger('mousemove', { clientX: 398, clientY: 46 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(2)

    await svg.trigger('mousemove', { clientX: 398, clientY: 57 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(3)

    await svg.trigger('mousemove', { clientX: 398, clientY: 68 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(4)

    await svg.trigger('mousemove', { clientX: 398, clientY: 79 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(5)
  })

  it('supports mouse wheel and up/down arrows to step sequentially through multi-sample clusters with zero jitter', async () => {
    const multiSamples = [
      { seq: 1, timestamp: '2026-09-20T12:00:00.000Z', latency_ms: 3615, success: true },
      { seq: 2, timestamp: '2026-09-20T12:00:00.050Z', latency_ms: 1500, success: true },
      { seq: 3, timestamp: '2026-09-20T12:00:00.100Z', latency_ms: 325, success: true },
      { seq: 4, timestamp: '2026-09-20T12:00:00.150Z', latency_ms: 320, success: true },
    ]
    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: multiSamples,
        windowSince: '2026-09-20T10:00:00.000Z',
        windowUntil: '2026-09-20T14:00:00.000Z',
        hoveredIndex: 0,
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

    const scroll = new WheelEvent('wheel', { deltaY: 100, bubbles: true, cancelable: true })
    svg.element.dispatchEvent(scroll)
    expect(scroll.defaultPrevented).toBe(false)
    expect(wrapper.emitted('hover')).toBeUndefined()
    // Shift + wheel intentionally browses samples; ordinary wheel scrolls the page.
    svg.element.dispatchEvent(new WheelEvent('wheel', { shiftKey: true, clientX: 398, deltaY: 100, bubbles: true, cancelable: true }))
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(1)

    await wrapper.setProps({ hoveredIndex: 1 })
    svg.element.dispatchEvent(new WheelEvent('wheel', { shiftKey: true, clientX: 398, deltaY: 100, bubbles: true, cancelable: true }))
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(2)

    // Wheel up steps to previous sample
    await wrapper.setProps({ hoveredIndex: 2 })
    svg.element.dispatchEvent(new WheelEvent('wheel', { shiftKey: true, clientX: 398, deltaY: -100, bubbles: true, cancelable: true }))
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(1)

    // Keyboard ArrowDown steps to next sample
    await wrapper.setProps({ hoveredIndex: 1 })
    await svg.trigger('keydown', { key: 'ArrowDown' })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(2)

    // Keyboard ArrowUp steps to previous sample
    await wrapper.setProps({ hoveredIndex: 2 })
    await svg.trigger('keydown', { key: 'ArrowUp' })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(1)
  })

  it('renders a single trend line connecting only cluster medians with diamond points and raw samples as vertical stems', () => {
    const multiTestSamples = [
      // Test 1: 10:00 (samples 0, 1)
      { seq: 1, timestamp: '2026-09-20T10:00:00.000Z', latency_ms: 100, success: true },
      { seq: 2, timestamp: '2026-09-20T10:00:01.000Z', latency_ms: 200, success: true },
      // Test 2: 12:00 (samples 2, 3)
      { seq: 1, timestamp: '2026-09-20T12:00:00.000Z', latency_ms: 300, success: true },
      { seq: 2, timestamp: '2026-09-20T12:00:01.000Z', latency_ms: 400, success: true },
    ]
    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: multiTestSamples,
        windowSince: '2026-09-20T08:00:00.000Z',
        windowUntil: '2026-09-20T14:00:00.000Z',
        hoveredIndex: null,
        pinnedIndex: null,
        width: 760,
        height: 112,
      },
    })

    // Exactly one trend polyline connecting the 2 test medians
    const polyline = wrapper.findAll('polyline')
    expect(polyline.length).toBe(1)
    const pointsStr = polyline[0].attributes('points') || ''
    const points = pointsStr.trim().split(/\s+/)
    expect(points.length).toBe(2) // Only 2 median points on the trend line

    // Diamonds for cluster medians
    const medianDiamonds = wrapper.findAll('polygon[points*=","]')
    expect(medianDiamonds.length).toBeGreaterThanOrEqual(2)
  })

  it('renders axis-break indicator when an extreme outlier is present without compressing the normal scale', () => {
    const outlierSamples = [
      { seq: 1, timestamp: '2026-09-20T12:00:00.000Z', latency_ms: 3615, success: true }, // Outlier
      { seq: 2, timestamp: '2026-09-20T12:00:00.050Z', latency_ms: 320, success: true },
      { seq: 3, timestamp: '2026-09-20T12:00:00.100Z', latency_ms: 325, success: true },
      { seq: 4, timestamp: '2026-09-20T12:00:00.150Z', latency_ms: 319, success: true },
    ]
    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: outlierSamples,
        hoveredIndex: null,
        pinnedIndex: null,
        width: 760,
        height: 112,
      },
    })

    // Top outlier indicator
    expect(wrapper.text()).toContain('▲ 3615 ms')
  })

  it('locks cluster on X with hysteresis so moving vertically never jumps to adjacent tests', async () => {
    const twoClusters = [
      // Cluster 0 at t=0
      { seq: 1, timestamp: '2026-09-20T10:00:00.000Z', latency_ms: 100, success: true },
      { seq: 2, timestamp: '2026-09-20T10:00:01.000Z', latency_ms: 200, success: true },
      // Cluster 1 at t=4h
      { seq: 1, timestamp: '2026-09-20T14:00:00.000Z', latency_ms: 150, success: true },
      { seq: 2, timestamp: '2026-09-20T14:00:01.000Z', latency_ms: 250, success: true },
    ]
    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: twoClusters,
        windowSince: '2026-09-20T10:00:00.000Z',
        windowUntil: '2026-09-20T14:00:00.000Z',
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

    // Hover near cluster 0 (x ~ 52)
    await svg.trigger('mousemove', { clientX: 60, clientY: 30 })
    expect([0, 1]).toContain(wrapper.emitted('hover')?.at(-1)?.[0])

    // Moving vertically in cluster 0 across any Y retains cluster 0 samples (0 or 1), never jumping to cluster 1 (2 or 3)
    await svg.trigger('mousemove', { clientX: 70, clientY: 75 })
    expect([0, 1]).toContain(wrapper.emitted('hover')?.at(-1)?.[0])

    await svg.trigger('mousemove', { clientX: 80, clientY: 20 })
    expect([0, 1]).toContain(wrapper.emitted('hover')?.at(-1)?.[0])
  })

  it('guarantees the bottom sample is always #6 even when sample #5 latency is lower than sample #6', async () => {
    const nonMonotonicSamples = [
      { seq: 1, timestamp: '2026-09-20T12:00:00.000Z', latency_ms: 3615, success: true },
      { seq: 2, timestamp: '2026-09-20T12:00:00.050Z', latency_ms: 350, success: true },
      { seq: 3, timestamp: '2026-09-20T12:00:00.100Z', latency_ms: 340, success: true },
      { seq: 4, timestamp: '2026-09-20T12:00:00.150Z', latency_ms: 330, success: true },
      { seq: 5, timestamp: '2026-09-20T12:00:00.200Z', latency_ms: 315, success: true }, // lower ms than #6
      { seq: 6, timestamp: '2026-09-20T12:00:00.250Z', latency_ms: 325, success: true }, // higher ms than #5
    ]
    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: nonMonotonicSamples,
        windowSince: '2026-09-20T10:00:00.000Z',
        windowUntil: '2026-09-20T14:00:00.000Z',
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

    // Hovering at the very bottom (Y = 82) must select sample 5 (seq #6), NOT sample 4 (seq #5)
    await svg.trigger('mousemove', { clientX: 398, clientY: 82 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(5) // Index 5 is seq #6

    // Hovering above the bottom (Y = 70) must select sample 4 (seq #5)
    await svg.trigger('mousemove', { clientX: 398, clientY: 70 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(4) // Index 4 is seq #5

    // Hovering at the very top (Y = 20) must select sample 0 (seq #1)
    await svg.trigger('mousemove', { clientX: 398, clientY: 20 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(0) // Index 0 is seq #1
  })

  it('keeps separate test runs distinct under a 7-day window and supports horizontal scrubbing without vertical jitter', async () => {
    // 7-day window from 2026-09-18 to 2026-09-25
    // Test 1: 2026-09-25 13:23:00 (samples 0..5, 6 pings)
    // Test 2: 2026-09-25 14:08:00 (samples 6..11, 6 pings) — 45 mins later (~3px apart on 760px width)
    const sevenDaySamples = [
      // Test 1 (median = 306ms at index 2)
      { seq: 1, timestamp: '2026-09-25T13:23:00.000Z', latency_ms: 300, success: true },
      { seq: 2, timestamp: '2026-09-25T13:23:01.000Z', latency_ms: 304, success: true },
      { seq: 3, timestamp: '2026-09-25T13:23:02.000Z', latency_ms: 306, success: true },
      { seq: 4, timestamp: '2026-09-25T13:23:03.000Z', latency_ms: 310, success: true },
      { seq: 5, timestamp: '2026-09-25T13:23:04.000Z', latency_ms: 315, success: true },
      { seq: 6, timestamp: '2026-09-25T13:23:05.000Z', latency_ms: 320, success: true },
      // Test 2 (median = 320ms at index 8)
      { seq: 1, timestamp: '2026-09-25T14:08:00.000Z', latency_ms: 316, success: true },
      { seq: 2, timestamp: '2026-09-25T14:08:01.000Z', latency_ms: 318, success: true },
      { seq: 3, timestamp: '2026-09-25T14:08:02.000Z', latency_ms: 320, success: true },
      { seq: 4, timestamp: '2026-09-25T14:08:03.000Z', latency_ms: 322, success: true },
      { seq: 5, timestamp: '2026-09-25T14:08:04.000Z', latency_ms: 325, success: true },
      { seq: 6, timestamp: '2026-09-25T14:08:05.000Z', latency_ms: 1309, success: true },
    ]
    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: sevenDaySamples,
        windowSince: '2026-09-18T14:08:00.000Z',
        windowUntil: '2026-09-25T14:08:00.000Z',
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

    // Default showTimeBoundsLabels is false: bounds labels are hidden to avoid redundant dates in table rows
    expect(wrapper.text()).not.toContain('09/18')

    // When showTimeBoundsLabels is true (e.g. in history modal): bounds labels must reflect the window span
    await wrapper.setProps({ showTimeBoundsLabels: true })
    expect(wrapper.text()).toContain('09/18')
    expect(wrapper.text()).toContain('09/25')

    // Trend polyline connects the 2 distinct test clusters
    const polyline = wrapper.findAll('polyline')
    expect(polyline.length).toBe(1)
    const points = polyline[0].attributes('points')?.trim().split(/\s+/)
    expect(points?.length).toBe(2)

    // Moving far away horizontally on Day 2 (clientX: 200) across different Y positions
    // must hold the median sample steadily and NOT jitter between samples 0..5
    await svg.trigger('mousemove', { clientX: 200, clientY: 20 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(3) // median of Test 1 (310ms)

    await svg.trigger('mousemove', { clientX: 200, clientY: 80 })
    expect(wrapper.emitted('hover')?.at(-1)?.[0]).toBe(3) // still median of Test 1!

    // Moving close to Test 2 (clientX: 740, right edge) selects Test 2
    await svg.trigger('mousemove', { clientX: 744, clientY: 50 })
    const hoveredInTest2 = wrapper.emitted('hover')?.at(-1)?.[0] as number
    expect(hoveredInTest2).toBeGreaterThanOrEqual(6)
    expect(hoveredInTest2).toBeLessThanOrEqual(11)
  })

  it('switches between closely adjacent test clusters cleanly at midpoint without overshoot', async () => {
    // Two test clusters very close to each other (10 minutes apart in 4h window)
    const closeSamples: WorkbenchLatencySample[] = [
      // Cluster 0 (median sample 1: 310ms)
      { seq: 1, timestamp: '2026-09-25T14:00:00.000Z', latency_ms: 300, success: true },
      { seq: 2, timestamp: '2026-09-25T14:00:01.000Z', latency_ms: 310, success: true },
      { seq: 3, timestamp: '2026-09-25T14:00:02.000Z', latency_ms: 320, success: true },
      // Cluster 1 (median sample 4: 260ms)
      { seq: 1, timestamp: '2026-09-25T14:10:00.000Z', latency_ms: 250, success: true },
      { seq: 2, timestamp: '2026-09-25T14:10:01.000Z', latency_ms: 260, success: true },
      { seq: 3, timestamp: '2026-09-25T14:10:02.000Z', latency_ms: 270, success: true },
    ]

    const wrapper = mount(LatencySamplePlot, {
      props: {
        samples: closeSamples,
        windowSince: '2026-09-25T12:00:00.000Z',
        windowUntil: '2026-09-25T16:00:00.000Z',
        hoveredIndex: null,
        pinnedIndex: null,
        width: 800,
        height: 112,
      },
    })
    const svg = wrapper.find('svg')
    Object.defineProperty(svg.element, 'getBoundingClientRect', {
      configurable: true,
      value: () => ({ left: 0, top: 0, width: 800, height: 112, right: 800, bottom: 112 }),
    })

    // Midpoint between clusters is ~433.25:
    // 1. Move to X = 420 (closer to Cluster 0) -> selects Cluster 0
    await svg.trigger('mousemove', { clientX: 420, clientY: 50 })
    let hovered = wrapper.emitted('hover')?.at(-1)?.[0] as number
    expect(hovered).toBeLessThan(3)

    // 2. Move to X = 440 (just past midpoint towards Cluster 1) -> immediately switches to Cluster 1
    await svg.trigger('mousemove', { clientX: 440, clientY: 50 })
    hovered = wrapper.emitted('hover')?.at(-1)?.[0] as number
    expect(hovered).toBeGreaterThanOrEqual(3)

    // 3. Move back to X = 425 (back to left of midpoint) -> immediately switches back to Cluster 0
    await svg.trigger('mousemove', { clientX: 425, clientY: 50 })
    hovered = wrapper.emitted('hover')?.at(-1)?.[0] as number
    expect(hovered).toBeLessThan(3)

    // 4. Guideline line and diamond marker must always be synchronized on the active sample's cluster
    await wrapper.setProps({ hoveredIndex: 4 }) // Sample 4 in Cluster 1
    const lines = wrapper.findAll('line')
    const dashedLine = lines.find((l) => l.attributes('stroke-dasharray') === '3 3')
    expect(dashedLine).toBeDefined()
    const guidelineX = parseFloat(dashedLine?.attributes('x1') || '0')
    expect(guidelineX).toBeCloseTo(448.5, 0)
  })
})
