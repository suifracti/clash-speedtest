import {afterEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import UsageDays from '../components/UsageDays.vue'

const mocks=vi.hoisted(()=>({accountDailyUsage:vi.fn(async()=>({account:undefined,report:undefined,days:[]}))}))
vi.mock('../usageDaily',()=>({accountDailyUsage:mocks.accountDailyUsage}))

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
})
