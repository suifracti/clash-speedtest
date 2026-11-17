import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import MonitorTimeBand from '../components/MonitorTimeBand.vue'
import {api} from '../api'
import type {MonitorJob,MonitorSample,NodeOption} from '../domain'
vi.mock('../api',()=>({api:{monitorSamples:vi.fn()}}))
const now=Date.parse('2026-11-18T12:00:00Z')
const node:NodeOption={profile_id:'account-a',profile_name:'Account A',node_key:'key',node_identity_key:'identity',config_revision_key:'revision',display_name:'Node',type:'ss',country_code:'HK',country_flag:''}
const job:MonitorJob={id:'job',name:'Job',profile_id:node.profile_id,profile_name:node.profile_name,nodes:[node],node_keys:[node.node_key],probe_set:'latency_four_v1',sampling_tier:'regular',interval_seconds:300,timeout_seconds:5,state:'running',resume_on_launch:false,updated_at:'2026-11-18T12:00:00Z'}
const sample=(minutes:number,extra:Partial<MonitorSample>={}):MonitorSample=>({...node,sample_id:'sample-'+minutes,display_name_snapshot:'Node',timestamp:new Date(now-minutes*60000).toISOString(),target:'Cloudflare',probe_type:'http_latency',success:true,latency:1000000,ttfb:1000000,sampling_tier:'regular',...extra})
let wrapper:VueWrapper|undefined
beforeEach(()=>{vi.resetAllMocks();vi.spyOn(Date,'now').mockReturnValue(now)})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.restoreAllMocks()})
const cells=()=>wrapper!.findAll('.cst-band-strip span')
it('marks the unread part from the raw page boundary even when the page contains no selected nodes',async()=>{
 vi.mocked(api.monitorSamples).mockResolvedValue({items:[sample(10,{node_key:'other',node_identity_key:'other'})],has_more:true,next_cursor:'older'})
 wrapper=mount(MonitorTimeBand,{props:{job}});await flushPromises()
 expect(cells().filter(c=>c.classes().includes('unread'))).toHaveLength(48)
 expect(wrapper.text()).not.toContain('最近 24 小时没有采样')
 expect(wrapper.text()).toContain('未完整读取')
})
it('does not paint a partially read time slot as a complete successful observation',async()=>{
 vi.mocked(api.monitorSamples).mockResolvedValue({items:[sample(10)],has_more:true,next_cursor:'older'})
 wrapper=mount(MonitorTimeBand,{props:{job}});await flushPromises()
 expect(cells()[47].classes()).toContain('unread')
 expect(cells()[47].attributes('title')).toContain('已读 1/1')
})
it('matches the frozen account identity revision and key without borrowing another revision',async()=>{
 vi.mocked(api.monitorSamples).mockResolvedValue({items:[sample(10),sample(20,{config_revision_key:'older'}),sample(40,{profile_id:'account-b'}),sample(50,{node_key:'different'})],has_more:false,next_cursor:''})
 wrapper=mount(MonitorTimeBand,{props:{job}});await flushPromises()
 expect(wrapper.get('[role="img"]').attributes('aria-label')).toContain('1 个已读样本')
 expect(wrapper.text()).toContain('冻结配置')
})
it('shows unknown on an initial read failure instead of empty sampling slots',async()=>{
 vi.mocked(api.monitorSamples).mockRejectedValue(new Error('read failed'))
 wrapper=mount(MonitorTimeBand,{props:{job}});await flushPromises()
 expect(cells().filter(c=>c.classes().includes('unread'))).toHaveLength(48)
 expect(wrapper.text()).toContain('read failed')
 expect(cells().some(c=>c.attributes('title')?.includes('没有采样'))).toBe(false)
})
it('retains the last successful time window on refresh failure but clears it when the frozen scope changes',async()=>{
 vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[sample(10)],has_more:false,next_cursor:''}).mockRejectedValue(new Error('offline'))
 wrapper=mount(MonitorTimeBand,{props:{job}});await flushPromises()
 const before=cells().map(c=>c.attributes('title'))
 vi.mocked(Date.now).mockReturnValue(now+3600000)
 await wrapper.setProps({job:{...job,updated_at:'later'}});await flushPromises()
 expect(cells().map(c=>c.attributes('title'))).toEqual(before)
 expect(wrapper.text()).toContain('上次成功读取')
 await wrapper.setProps({job:{...job,nodes:[{...node,config_revision_key:'new'}]}});await flushPromises()
 expect(wrapper.get('[role="img"]').attributes('aria-label')).toContain('0 个已读样本')
 expect(cells().filter(c=>c.classes().includes('unread'))).toHaveLength(48)
})

it('keeps the boundary slot incomplete when another page can contain the same timestamp',async()=>{
 vi.mocked(api.monitorSamples).mockResolvedValue({items:[sample(30)],has_more:true,next_cursor:'same-timestamp-earlier-id'})
 wrapper=mount(MonitorTimeBand,{props:{job}});await flushPromises()
 expect(cells()[47].classes()).toContain('unread')
 expect(cells()[47].attributes('title')).toContain('已读 1/1')
})
