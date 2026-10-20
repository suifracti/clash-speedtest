import {afterEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import UsageDays from '../components/UsageDays.vue'

const mocks=vi.hoisted(()=>({accountDailyUsage:vi.fn(async()=>({account:undefined as import('../usageDaily').UsageSnapshot|undefined,report:undefined as import('../usageDaily').UsageReport|undefined,days:[] as import('../usageDaily').DayUsage[]}))}))
vi.mock('../usageDaily',async importOriginal=>{
  const actual=await importOriginal<typeof import('../usageDaily')>()
  return {...actual,accountDailyUsage:mocks.accountDailyUsage}
})

let wrapper:VueWrapper|undefined
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.clearAllMocks()})

describe('UsageDays refresh',()=>{
  it('reloads account history after a subscription refresh',async()=>{
    wrapper=mount(UsageDays,{props:{airportId:'a',subscriptionId:'s',days:14,refreshVersion:0}})
    await flushPromises()
    expect(mocks.accountDailyUsage).toHaveBeenCalledTimes(1)
    await wrapper.setProps({refreshVersion:1})
    await flushPromises()
    expect(mocks.accountDailyUsage).toHaveBeenCalledTimes(2)
  })

  it('renders three labeled bars with unknown days kept distinct from zero usage',async()=>{
    mocks.accountDailyUsage.mockResolvedValueOnce({account:{account_key:'a',airport_name:'Airport',subscription_name:'Account',captured_at:'2026-10-20T00:00:00Z',status:'ok',counter_kind:'subscription',upload:null,download:null,source:'subscription'},report:undefined,days:[
      {date:'2026-10-20',label:'今天',bytes:1024,estimated:false,recorded:true},
      {date:'2026-10-19',label:'昨天',bytes:0,estimated:true,recorded:true},
      {date:'2026-10-18',label:'前天',bytes:null,estimated:false,recorded:false},
    ]})
    wrapper=mount(UsageDays,{props:{airportId:'a',subscriptionId:'s',days:3,refreshVersion:0,summary:true}})
    await flushPromises()

    expect(wrapper!.findAll('.cst-usage-day')).toHaveLength(3)
    expect(wrapper!.findAll('.cst-usage-day-label').map(label=>label.text())).toEqual(['今天','昨天','前天'])
    expect(wrapper!.find('.cst-usage-day.estimated').exists()).toBe(true)
    expect(wrapper!.find('.cst-usage-day.unknown').exists()).toBe(true)
    expect(wrapper!.get('[role="img"]').attributes('aria-label')).toContain('昨天 约 0 B')
  })
})
