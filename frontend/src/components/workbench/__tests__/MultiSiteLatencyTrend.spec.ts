import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import { nextTick } from 'vue'
import MultiSiteLatencyTrend from '../MultiSiteLatencyTrend.vue'
import { baselineLatencyTest, latencySiteResults, suiteHealth } from '../../../utils/latencyTargets'
import type { WorkbenchLatencyTest } from '../../../types'

it('keeps site medians separate, shows failure reasons, and isolates baseline health', async () => {
  const test = { attempt_id: 'multi-one', target: 'multi://latency-v1', finished_at: '2026-09-28T01:01:00Z', samples: [
    { seq: 1, timestamp: '2026-09-28T01:00:10Z', target: 'https://speed.cloudflare.com/__down?bytes=1', success: true, latency_ms: 100 },
    { seq: 2, timestamp: '2026-09-28T01:00:20Z', target: 'https://speed.cloudflare.com/__down?bytes=1', success: true, latency_ms: 200 },
    { seq: 3, timestamp: '2026-09-28T01:00:30Z', target: 'https://www.gstatic.com/generate_204', success: true, latency_ms: 400 },
    { seq: 4, timestamp: '2026-09-28T01:00:40Z', target: 'https://api.github.com/zen', success: false, latency_ms: 0, error: 'DNS failed' },
  ] } as WorkbenchLatencyTest
  const wrapper = mount(MultiSiteLatencyTrend, { props: { tests: [test], since: '2026-09-28T01:00:00Z', until: '2026-09-28T02:00:00Z' } })
  expect(wrapper.findAll('circle')).toHaveLength(2)
  expect(wrapper.findAll('circle title').map(title => title.text()).join(' ')).toContain('中位 150 ms')
  expect(wrapper.findAll('path title').map(title => title.text()).join(' ')).toContain('DNS failed')
  await wrapper.find('circle').trigger('focus')
  expect(wrapper.get('[role="status"]').text()).toContain('Google')
  await wrapper.get('svg').trigger('mouseleave')
  expect(wrapper.find('[role="status"]').exists()).toBe(true)
  expect(wrapper.get('[role="status"]').text()).toContain('DNS failed')
  wrapper.get('svg').element.dispatchEvent(new WheelEvent('wheel', { shiftKey: true, deltaY: 1 }))
  await nextTick()
  expect(wrapper.get('[role="status"]').text()).toContain('样本 1/4')
  expect(wrapper.get('[role="status"]').text()).toContain('100 ms')
  await wrapper.get('svg').trigger('click')
  await wrapper.find('circle').trigger('focus')
  await wrapper.get('svg').trigger('mouseleave')
  expect(wrapper.get('[role="status"]').text()).toContain('已固定读数')
  await wrapper.findAll('button').find(button => button.text() === '取消固定')!.trigger('click')
  expect(wrapper.find('[role="status"]').exists()).toBe(false)
  await wrapper.findAll('button').find(button => button.text() === 'GitHub')!.trigger('click')
  expect(wrapper.findAll('circle')).toHaveLength(0)
  expect(wrapper.findAll('path title')).toHaveLength(1)
  const baseline = baselineLatencyTest(test)
  expect(baseline.samples).toHaveLength(2)
  expect(baseline.latency_ms).toBe(150)
  const summary = latencySiteResults([test])
  expect(suiteHealth([test])).toMatchObject({ tested: 3, reachable: 2, complete: 2, count: 4, success: 3, rate: '75.0%' })
  expect(suiteHealth([test])!.failures).toEqual([{ label: 'GitHub', failed: 1, count: 1 }])
  expect(wrapper.get('[aria-label="分站历史统计"]').text()).toContain('GitHub 历史统计')
  expect(suiteHealth([{ ...test, target: 'https://speed.cloudflare.com/__down?bytes=1' }])).toBeNull()
  expect(summary.find(site => site.value === 'github')).toMatchObject({ count: 1, successes: 0, latency: null, error: 'DNS failed' })
  expect(summary.find(site => site.value === 'firefox')).toMatchObject({ count: 0, latency: null })
})
