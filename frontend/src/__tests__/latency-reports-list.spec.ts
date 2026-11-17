import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {mount,flushPromises,type VueWrapper} from '@vue/test-utils'
import LatencyReports from '../components/LatencyReports.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import {readLatencyReports} from '../latencyReportData'
import type {LatencyBatch} from '../domain'
vi.hoisted(()=>{const store=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>store.get(k)||null,setItem:(k:string,v:string)=>store.set(k,v)}})})
vi.mock('../api',()=>({api:{autoPickReports:vi.fn()}}))
vi.mock('../latencyReportData',()=>({readLatencyReports:vi.fn()}))
const batch:LatencyBatch={batch_id:'manual',request_id:'manual',state:'completed',requested_at:'2026-11-18T08:00:00Z',target_id:'all',item_count:0,items:[]}
const picked={report_id:'auto',created_at:'2026-11-17T08:00:00Z',trigger:'manual',status:'selected',summary:'自动精选结果',selected_count:1,daily_count:1,streaming_count:0,ai_count:0,kept_previous:false}
let wrapper:VueWrapper|undefined
function attach(){const w=createWorkspace();wrapper=mount(LatencyReports,{global:{provide:{[workspaceKey as symbol]:w}}});return wrapper}
beforeEach(()=>{vi.resetAllMocks();vi.mocked(readLatencyReports).mockResolvedValue({reports:[{batch,monitor:false}],errors:[]});vi.mocked(api.autoPickReports).mockResolvedValue([picked])})
afterEach(()=>{wrapper?.unmount();wrapper=undefined})
it('keeps manual reports when the automatic report source fails and retries that source alone',async()=>{
 vi.mocked(api.autoPickReports).mockRejectedValueOnce(new Error('auto unavailable')).mockResolvedValue([picked])
 const view=attach();await flushPromises()
 expect(view.findAll('.report-row')).toHaveLength(1)
 expect(view.text()).toContain('auto unavailable');expect(view.text()).not.toContain('还没有')
 await view.findAll('button').find(b=>b.text().includes('重试自动精选'))!.trigger('click');await flushPromises()
 expect(view.findAll('.report-row')).toHaveLength(2);expect(readLatencyReports).toHaveBeenCalledTimes(1)
})
it('keeps automatic reports when latency history fails without claiming an empty history',async()=>{
 vi.mocked(readLatencyReports).mockRejectedValue(new Error('history unavailable'))
 const view=attach();await flushPromises()
 expect(view.findAll('.report-row')).toHaveLength(1)
 expect(view.text()).toContain('自动精选结果');expect(view.text()).not.toContain('还没有')
})
it('orders report types together by the recorded time',async()=>{
 const view=attach();await flushPromises()
 expect(view.findAll('.report-row')[0].text()).not.toContain('自动精选结果')
 expect(view.findAll('.report-row')[1].text()).toContain('自动精选结果')
})

it('shows partially read latency reports with their source warning',async()=>{
 vi.mocked(readLatencyReports).mockResolvedValue({reports:[{batch,monitor:false}],errors:['监测报告目录读取失败：offline']})
 const view=attach();await flushPromises()
 expect(view.findAll('.report-row')).toHaveLength(2)
 expect(view.text()).toContain('部分来源未读取');expect(view.text()).toContain('offline')
})
