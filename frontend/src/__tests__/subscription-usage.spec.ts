import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import SubscriptionUsageView from '../components/SubscriptionUsageView.vue'
import {api,request} from '../api'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{value:{getItem:()=>null},configurable:true})})
vi.mock('../api',async importOriginal=>({...await importOriginal<typeof import('../api')>(),request:vi.fn(),api:{settings:vi.fn(),saveSettings:vi.fn()}}))
vi.mock('../workspace',()=>({useWorkspace:()=>({notify:vi.fn(),refreshNodes:vi.fn()})}))
let wrapper:VueWrapper|undefined
const empty={from:'2026-10-01',until:'2026-10-31',period:'day',buckets:[{date:'2026-10-03',bytes:null,upload:null,download:null,intervals:0,estimated_intervals:0}],intervals:[],accounts:[],observed_bytes:null,unallocated_bytes:null,unknown_intervals:0,daily_enabled:true}
const settings={preferred_browser:'default',monitor_retention_policy:'keep_all',monitor_retention_custom_days:0,monitor_budget_max_concurrent:32,subscription_daily_update_enabled:true}
beforeEach(()=>{vi.useFakeTimers({toFake:['Date']});vi.setSystemTime(new Date('2026-10-03T04:00:00Z'));vi.mocked(api.settings).mockResolvedValue(settings);vi.mocked(request).mockResolvedValue(empty)})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.useRealTimers();vi.resetAllMocks()})

describe('subscription usage history',()=>{
  it('uses Shanghai day/week/month ranges, preserves unknowns, and rejects a late old view',async()=>{
    wrapper=mount(SubscriptionUsageView);await flushPromises()
    const params=()=>new URL('http://localhost'+vi.mocked(request).mock.calls.at(-1)![0]).searchParams
    expect([params().get('from'),params().get('until'),params().get('period')]).toEqual(['2026-10-01','2026-10-31','day'])
    expect(wrapper.get('.usage-summary').text()).toContain('待积累')
    expect(wrapper.get('.usage-buckets tbody').text()).toContain('暂无可归属数据')
    expect(wrapper.get('.usage-buckets tbody').text()).not.toContain('0 B')
    let lateResolve!:(value:typeof empty)=>void
    vi.mocked(request).mockReturnValueOnce(new Promise(resolve=>lateResolve=resolve))
    await wrapper.get('[aria-label="用量参照日期"]').setValue('2026-09-03')
    await wrapper.findAll('.segmented button')[1].trigger('click');await flushPromises()
    expect([params().get('from'),params().get('until'),params().get('period')]).toEqual(['2026-06-15','2026-09-06','week'])
    vi.mocked(request).mockResolvedValueOnce({...empty,from:'2025-10-01',until:'2026-09-30',period:'month',observed_bytes:1024,unknown_intervals:1,buckets:[{date:'2026-09',bytes:1024,upload:0,download:1024,intervals:1,estimated_intervals:1}],intervals:[{account_key:'a',name:'机场 · 订阅',until:'2026-09-03T04:00:00Z',bytes:null,upload:null,download:null,status:'reset',estimated:false}]})
    await wrapper.findAll('.segmented button')[2].trigger('click');await flushPromises()
    expect([params().get('from'),params().get('until'),params().get('period')]).toEqual(['2025-10-01','2026-09-30','month'])
    expect(wrapper.text()).toContain('1.0 KiB');expect(wrapper.text()).toContain('计数重置');expect(wrapper.text()).toContain('含 1 个估算区间')
    lateResolve({...empty,from:'OLD_VIEW'});await flushPromises()
    expect(wrapper.text()).not.toContain('OLD_VIEW');expect(wrapper.text()).toContain('1.0 KiB')
  })
  it('restores a failed schedule toggle and allows retry without changing other settings',async()=>{
    wrapper=mount(SubscriptionUsageView);await flushPromises()
    vi.mocked(api.saveSettings).mockRejectedValueOnce(new Error('fixture save failed'))
    await wrapper.get('input[type=checkbox]').setValue(false);await flushPromises()
    expect(wrapper.get('[role=alert]').text()).toContain('fixture save failed')
    expect((wrapper.get('input[type=checkbox]').element as HTMLInputElement).checked).toBe(true)
    expect(api.saveSettings).toHaveBeenLastCalledWith({...settings,subscription_daily_update_enabled:false})
    vi.mocked(api.saveSettings).mockResolvedValueOnce(undefined)
    await wrapper.get('input[type=checkbox]').setValue(false);await flushPromises()
    expect((wrapper.get('input[type=checkbox]').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
  })
})
