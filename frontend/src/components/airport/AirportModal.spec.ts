import { reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import AirportModal from './AirportModal.vue'
import * as api from '../../api/bridge'

const state = vi.hoisted(() => ({ store: null as any }))
vi.mock('../../stores/workbench', () => ({ useWorkbenchStore: () => state.store }))
vi.mock('../../api/bridge', () => ({ refreshSubscription: vi.fn(), getSubscriptionURL: vi.fn(), saveAirportMaintenance: vi.fn() }))
vi.mock('../../api/monitor', () => ({ fetchMonitorNodeOptions: vi.fn() }))
import { fetchMonitorNodeOptions } from '../../api/monitor'
const sub = (id: string) => ({ id, airport_id: 'ap', name: id, url_display: 'https://example.invalid/•••', url_configured: true, node_count: 3, has_cache: true, status: 'normal' })
it('persists read reminders and repeats alerts at half of each acknowledged balance', async () => {
 const storage = new Map<string, string>()
 vi.stubGlobal('localStorage', { getItem: (key: string) => storage.get(key) || null, setItem: (key: string, value: string) => storage.set(key, value) })
 const gib = 1024 ** 3
 state.store.airports[0].subscriptions = [{ ...sub('low'), usage: { total: 600 * gib, upload: 0, download: 588 * gib } }]
 let wrapper = mount(AirportModal, { props: { embedded: true } })
 await wrapper.get('.attention-read').trigger('click')
 expect(wrapper.text()).toContain('当前提醒均已读')
 wrapper.unmount()
 wrapper = mount(AirportModal, { props: { embedded: true } })
 expect(wrapper.text()).toContain('当前提醒均已读')
 state.store.airports[0].subscriptions[0].usage.download += 1024
 await flushPromises()
 expect(wrapper.text()).toContain('当前提醒均已读')
 state.store.airports[0].subscriptions[0].usage.download = 594 * gib
 await flushPromises()
 expect(wrapper.get('.attention-heading h2').text()).toBe('1 条订阅需要你看一眼')
 expect(wrapper.get('.attention-read').text()).toBe('标为已读')
 await wrapper.get('.attention-read').trigger('click')
 state.store.airports[0].subscriptions[0].usage.download = 596 * gib
 await flushPromises()
 expect(wrapper.text()).toContain('当前提醒均已读')
 state.store.airports[0].subscriptions[0].usage.download = 597 * gib
 await flushPromises()
 expect(wrapper.get('.attention-heading h2').text()).toBe('1 条订阅需要你看一眼')
 wrapper.unmount()
 vi.unstubAllGlobals()
})
beforeEach(() => {
 vi.resetAllMocks()
 vi.mocked(fetchMonitorNodeOptions).mockResolvedValue([])
 state.store = reactive({ isAirportModalOpen: true, loadAirports: vi.fn(), airports: [{ id: 'ap', name: 'Fixture', website_url: 'https://example.invalid', node_count: 6, subscriptions: [sub('one'), sub('two')] }] })
})
it('shows real progress, partial failure and a retry without exposing or fetching full URLs', async () => {
 let finish!: (value: any) => void
 vi.mocked(api.refreshSubscription).mockImplementationOnce(() => new Promise(resolve => { finish = resolve })).mockRejectedValueOnce(new Error('secret must not appear'))
 const wrapper = mount(AirportModal)
 const refresh = wrapper.get('button.refresh-all')
 await refresh.trigger('click')
 expect(wrapper.text()).toContain('正在刷新 0 / 2')
 expect(api.refreshSubscription).toHaveBeenCalledTimes(1)
 await refresh.trigger('click')
 expect(api.refreshSubscription).toHaveBeenCalledTimes(1)
 finish({ ...sub('one'), usage: { upload: 1024, download: 3072, total: 8192, updated_at: '2026-09-27T00:00:00Z' } })
 await flushPromises()
 expect(wrapper.text()).toContain('1 成功，1 失败')
 expect(wrapper.text()).toContain('剩余 4.0 KiB')
 expect(wrapper.text()).not.toContain('secret must not appear')
 expect(api.getSubscriptionURL).not.toHaveBeenCalled()
 vi.mocked(api.refreshSubscription).mockResolvedValueOnce(sub('two'))
 await wrapper.findAll('button').find(b => b.text() === '重试失败项')!.trigger('click')
 await flushPromises()
 expect(wrapper.text()).toContain('已更新 1 个订阅')
 expect(api.refreshSubscription).toHaveBeenLastCalledWith('ap', 'two')
 expect(wrapper.text()).toContain('未提供有效用量')
})
it('manages a single airport in the workspace and opens its cached subscription without fetching secrets', async () => {
 state.store.airports.push({ id: 'second', name: 'Second', node_count: 3, subscriptions: [{ ...sub('other'), airport_id: 'second' }] })
 const wrapper = mount(AirportModal, { props: { embedded: true, visible: true } })
 expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
 expect(wrapper.findAll('.airport-card')).toHaveLength(1)
 await wrapper.findAll('.airport-browser-item')[1].trigger('click')
 expect(wrapper.get('.airport-card').text()).toContain('Second')
 await wrapper.findAll('button').find(b => b.text() === '查看节点 →')!.trigger('click')
 expect(wrapper.emitted('browse-profile')).toEqual([['other']])
 expect(api.getSubscriptionURL).not.toHaveBeenCalled()
 expect(api.refreshSubscription).not.toHaveBeenCalled()
})

it('surfaces links discovered in subscription notices and uses an absolute low-traffic threshold', async () => {
 const gib = 1024 ** 3
 state.store.airports = [{
  id: 'ap', name: '宝可梦', node_count: 20,
  subscriptions: [
   { ...sub('large'), usage: { upload: 0, download: 924 * gib, total: 1024 * gib, updated_at: '2026-09-27T00:00:00Z' } },
   { ...sub('low'), usage: { upload: 0, download: 588 * gib, total: 600 * gib, updated_at: '2026-09-27T00:00:00Z' } },
  ],
 }]
 vi.mocked(fetchMonitorNodeOptions).mockResolvedValue([{
  profileId: 'large', profileName: '宝可梦', nodeKey: 'notice', nodeIdentityKey: 'notice-id', configRevisionKey: 'notice-rev',
  displayName: '放丢失官网2:https://love2.p6m6.com', type: 'Vless', countryCode: 'OTHER', countryFlag: '',
 }])
 const wrapper = mount(AirportModal, { props: { embedded: true, visible: true } })
 await flushPromises()

 expect(wrapper.text()).toContain('订阅发现')
 expect(wrapper.text()).toContain('放丢失官网2')
 expect(wrapper.text()).toContain('加入日常管理')
 expect(wrapper.findAll('.usage-warning')).toHaveLength(1)
 expect(wrapper.get('.usage-warning').text()).toBe('仅剩 12.00 GiB')
 expect(wrapper.text()).not.toContain('流量不足 10%')
 expect(wrapper.get('.manager-overview button strong').text()).toContain('1')
 await wrapper.findAll('button').find(button => button.text() === '加入日常管理')!.trigger('click')
 await flushPromises()
 expect(api.saveAirportMaintenance).toHaveBeenCalledWith('ap', expect.objectContaining({
  links: [{ label: '放丢失官网2', url: 'https://love2.p6m6.com/', monthly_day: 0 }],
 }))
})
