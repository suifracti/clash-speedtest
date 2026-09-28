import { mount, flushPromises } from '@vue/test-utils'
import { it, expect, vi } from 'vitest'
import WorkbenchBatchHistoryPanel from './WorkbenchBatchHistoryPanel.vue'
import { fetchWorkbenchLatencyBatches, fetchWorkbenchLatencyBatch } from '../../api/bridge'
vi.mock('../../api/bridge', () => ({ fetchWorkbenchLatencyBatches: vi.fn(), fetchWorkbenchLatencyBatch: vi.fn(), retryWorkbenchLatencyBatchItem: vi.fn() }))

it('loads actual node results when the list contains summaries only, and refreshes the selected detail', async () => {
  const summary = { batch_id: 'batch-1', state: 'completed', item_count: 1, timeout_seconds: 5, requested_at: '2026-09-27T01:00:00Z' }
  const detail = { ...summary, items: [{ item_id: 'item-1', display_name: 'Saved node', node_type: 'ss', profile_id: 'p', execution_state: 'completed', persistence_state: 'saved', result: { success_samples: 3, failure_samples: 0, latency_ms: 125, jitter_ms: 5, samples: [] } }] }
  vi.mocked(fetchWorkbenchLatencyBatches).mockResolvedValue([summary] as never)
  vi.mocked(fetchWorkbenchLatencyBatch).mockResolvedValue(detail as never)
  const wrapper = mount(WorkbenchBatchHistoryPanel)
  await flushPromises()
  expect(wrapper.text()).toContain('Saved node')
  expect(wrapper.text()).toContain('125 ms')
  detail.items[0].result.latency_ms = 150
  await wrapper.findAll('button').find(button => button.text() === '重新读取历史')!.trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('150 ms')
  wrapper.unmount()
})
