import {mount,flushPromises} from '@vue/test-utils'
import {ref} from 'vue'
import {beforeEach,expect,it,vi} from 'vitest'
import AirportReport from '../components/AirportReport.vue'
import RecommendationView from '../components/RecommendationView.vue'
import {workspaceKey} from '../workspace'
import {readQualityBatch} from '../qualityBatch'
import {api,saveFile} from '../api'
import {key,scope,type Attempt,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
vi.mock('../qualityBatch',()=>({readQualityBatch:vi.fn(async()=>({}))}))
vi.mock('../api',()=>({api:{settings:vi.fn(async()=>({})),exportClash:vi.fn(async()=>({yaml_content:'fixture',node_count:2}))},saveFile:vi.fn(async()=>true)}))
vi.mock('../usageDaily',async original=>({...await original<typeof import('../usageDaily')>(),readUsage:vi.fn(async()=>({accounts:[],intervals:[]}))}))
const nodes=['a','b'].map((profile_id,index)=>({profile_id,node_key:profile_id,node_identity_key:profile_id,config_revision_key:'r',display_name:'Node',country_code:index?'SG':'JP'}) as NodeOption)
const w={hours:ref(24),project:ref('combined'),inspectNode:ref<NodeOption|null>(null),page:ref('recommend'),airports:ref([{id:'a',name:'A'},{id:'b',name:'B'}]),nodes:ref(nodes),catalog:ref<unknown[]>([]),nodeAirportId:(n:NodeOption)=>n.profile_id,nodeAirportName:()=>'',nodeSubscriptionName:()=>'',notify:vi.fn(),fail:vi.fn()}
const global={provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}
beforeEach(()=>{
 global.provide[workspaceKey as symbol]={...w}
 w.nodes.value=nodes
 w.catalog.value=[]
 vi.mocked(readQualityBatch).mockClear()
})
it('reads only the selected airport and reads again when it changes',async()=>{
 vi.mocked(readQualityBatch).mockClear();const wrapper=mount(AirportReport,{global});await flushPromises()
 expect(readQualityBatch).toHaveBeenLastCalledWith([nodes[0]],expect.any(String),expect.any(String))
 expect(wrapper.text()).toContain('这个机场近 30 天没有检测记录');expect(wrapper.text()).not.toContain('0 / 0');expect(wrapper.find('.report-metrics').exists()).toBe(false)
 await wrapper.get('select').setValue('b');await flushPromises()
 expect(readQualityBatch).toHaveBeenCalledTimes(2);expect(readQualityBatch).toHaveBeenLastCalledWith([nodes[1]],expect.any(String),expect.any(String));wrapper.unmount()
})
it('reads seven days for recommendations and labels that window',async()=>{
 vi.mocked(readQualityBatch).mockClear();const wrapper=mount(RecommendationView,{global});await flushPromises()
 const [selected,since,until]=vi.mocked(readQualityBatch).mock.calls[0]!
 expect(selected).toEqual(nodes);expect(Date.parse(until)-Date.parse(since)).toBe(7*86400000);expect(wrapper.text()).toContain('近 7 天');wrapper.unmount()
})
it('filters by region and reuses the same snapshot when returning before the refresh interval',async()=>{
 const started=Date.now(),clock=vi.spyOn(Date,'now').mockReturnValue(started)
 let wrapper=mount(RecommendationView,{global});await flushPromises()
 await wrapper.get('select[aria-label="推荐地区"]').setValue('JP');await flushPromises()
 expect(wrapper.findAll('.region-result h3').map(row=>row.text())).toEqual(['日本'])
 expect(readQualityBatch).toHaveBeenCalledTimes(1)
 wrapper.unmount()
 clock.mockReturnValue(started+9*60_000)
 wrapper=mount(RecommendationView,{global});await flushPromises()
 expect(readQualityBatch).toHaveBeenCalledTimes(1)
 await wrapper.get('.recommendation-page .page-header button').trigger('click');await flushPromises()
 expect(readQualityBatch).toHaveBeenCalledTimes(2)
 wrapper.unmount()
 clock.mockReturnValue(started+20*60_000)
 wrapper=mount(RecommendationView,{global});await flushPromises()
 expect(readQualityBatch).toHaveBeenCalledTimes(3)
 wrapper.unmount();clock.mockRestore()
})
it('keeps regions visible with missing evidence and opens the historical reference for the selected service',async()=>{
 let wrapper=mount(RecommendationView,{global});await flushPromises()
 expect(wrapper.findAll('.region-result h3').map(row=>row.text())).toEqual(expect.arrayContaining(['日本','新加坡']))
 expect(wrapper.findAll('.region-note')).toHaveLength(2)
 expect(wrapper.text()).not.toContain('导出 Clash')
 await wrapper.get('.recommendation-export button').trigger('click');expect(w.page.value).toBe('home');wrapper.unmount()
 w.catalog.value=[{service_id:'ai',category:'AI',name:'AI',result_kind:'account_availability'}]
 vi.mocked(readQualityBatch).mockResolvedValueOnce({[key(nodes[0])]:{tests:[],monitor:[],downloads:[],services:[{...nodes[0],service_id:'ai',finished_at:'2026-10-19T00:00:00Z',node_key:'old',config_revision_key:'old-revision',execution_state:'completed',persistence_state:'saved',rule:{name:'AI'},result:{outcome:'matched'}}],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true} as unknown as QualityEvidence})
 wrapper=mount(RecommendationView,{global});await flushPromises()
 await wrapper.get('.recommendation-page .page-header button').trigger('click');await flushPromises()
 await wrapper.get('.recommendation-tabs button:nth-child(3)').trigger('click')
 expect(wrapper.text()).toContain('待复测')
 expect(wrapper.text()).not.toContain('导出 Clash')
 await wrapper.get('article button').trigger('click')
 expect(w.inspectNode.value?.config_revision_key).toBe('old-revision')
 expect(w.hours.value).toBe(168);expect(w.project.value).toBe('service');wrapper.unmount()
})

it('opens the actual recommended service scope when an unrelated latency record has the same timestamp',async()=>{
 const node={...nodes[0],node_key:'current-key',node_identity_key:'current-identity',config_revision_key:'current-revision'}
 const at='2026-11-18T00:00:00Z',old={...node,node_key:'old-key',node_identity_key:'old-identity',config_revision_key:'old-revision'}
 w.nodes.value=[node];w.catalog.value=[{service_id:'ai',category:'AI',name:'AI',result_kind:'account_availability'}]
 vi.mocked(readQualityBatch).mockResolvedValueOnce({[key(node)]:{tests:[{...node,attempt_id:'unrelated-latency',samples:[{timestamp:at}]}],monitor:[],downloads:[],services:[{...old,attempt_id:'old-service',service_id:'ai',finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{name:'AI'},result:{outcome:'matched'}}],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true} as unknown as QualityEvidence})
 const wrapper=mount(RecommendationView,{global});await flushPromises()
 await wrapper.get('.recommendation-tabs button:nth-child(3)').trigger('click')
 const ai=wrapper.get('.purpose-results')
 await ai.get('article button').trigger('click')
 expect(w.inspectNode.value).toMatchObject({profile_id:node.profile_id,node_key:'old-key',node_identity_key:'old-identity',config_revision_key:'old-revision',historyServiceId:'ai'})
 expect(ai.get('article').text()).toContain('依据来自旧配置')
 wrapper.unmount();w.nodes.value=nodes
})

it('shows an alternate with different conditions and exports picks without the regional reference',async()=>{
 const at='2026-11-18T00:00:00Z'
 const first={...nodes[0],display_name:'First',type:'ss'}
 const alternate={...first,node_key:'jp-backup',node_identity_key:'jp-backup',display_name:'Backup'}
 const reference={...nodes[1],display_name:'Unsteady',type:'ss'}
 w.nodes.value=[first,alternate,reference]
 const data:Record<string,QualityEvidence>={}
 for(const [i,node] of w.nodes.value.entries()){
  const partial=i===2,target=i===1?'https://speed.cloudflare.com/__down?bytes=1':'https://proof.ovh.net/files/1Gb.dat'
  const download:Attempt={...node,attempt_id:node.node_key,request_id:node.node_key,requested_at:at,execution_state:'completed',persistence_state:'saved',node_type:'ss',
   rule:{target_url:target,mode:'adaptive',method:'GET',rule_version:5,maximum_duration_ns:15e9},
   result:{outcome:partial?'time_limit':'completed',end_reason:partial?'time_limit':'speed_stable',bytes_read:50*2**20,
    stable_speed_mbps:partial?undefined:160,finished_at:at,phase_timings_ns:{response_body:5e9}}}
  data[key(node)]={tests:[],monitor:[],downloads:[download],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at}
 }
 vi.mocked(readQualityBatch).mockResolvedValueOnce(data)
 const wrapper=mount(RecommendationView,{global});await flushPromises()
 await wrapper.get('.recommendation-tabs button:nth-child(2)').trigger('click')
 expect(wrapper.findAll('.candidate-slot').map(el=>el.text()).sort()).toEqual(['待复测','次选','首选'].sort())
 expect(wrapper.text()).toContain('检测条件不同')
 await wrapper.get('.recommendation-export button').trigger('click');await flushPromises()
 expect(api.exportClash).toHaveBeenCalledWith(expect.arrayContaining([scope(first),scope(alternate)]),'下载地区推荐')
 expect(vi.mocked(api.exportClash).mock.calls.at(-1)?.[0]).toHaveLength(2)
 expect(saveFile).toHaveBeenCalledWith('fixture','speedtest-download.yaml','application/x-yaml')
 wrapper.unmount();w.nodes.value=nodes
})

it('starts an airport report with the airport already selected in the account ledger',async()=>{
 vi.mocked(readQualityBatch).mockClear()
 const wrapper=mount(AirportReport,{props:{initialAirportId:'b'},global});await flushPromises()
 expect(readQualityBatch).toHaveBeenLastCalledWith([nodes[1]],expect.any(String),expect.any(String))
 wrapper.unmount()
})

it('opens a preconfigured region plan from RecommendationView without starting a run',async()=>{
 const plan=ref<any>(null)
 const openPlan=vi.fn((list:NodeOption[])=>{plan.value={nodes:list,projects:['latency'],target:'cloudflare',samples:3,includeReputation:true,reputationServiceIds:['ippure_ip_quality'],serviceIds:[]}})
 const wWithPlan={...w,plan,openPlan}
 const wrapper=mount(RecommendationView,{global:{provide:{[workspaceKey as symbol]:wWithPlan},stubs:{TrendChart:true}}})
 await flushPromises()
 const buttons=wrapper.findAll('.region-plan-btn')
 expect(buttons.length).toBeGreaterThanOrEqual(2)
 await buttons[0]!.trigger('click')
 expect(openPlan).toHaveBeenCalledTimes(1)
 expect(plan.value.nodes).toHaveLength(1)
 expect(plan.value.projects).toEqual(['latency'])
 expect(plan.value.target).toBe('all')
 expect(plan.value.samples).toBe(6)
 expect(plan.value.includeReputation).toBe(false)
 wrapper.unmount()
})

it('filters recommendations by airport and risk policy and updates export group name',async()=>{
 const at='2026-11-21T02:00:00Z'
 const target='https://speed.cloudflare.com/__down?bytes=1'
 const makeLatency=(n:NodeOption,ms:number)=>({...n,attempt_id:'lat-'+n.node_key,display_name:n.display_name,node_type:'ss',source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,target,requested_at:at,finished_at:at,status:'completed',latency_ms:ms,total_samples:6,success_samples:6,failure_samples:0,persistence_state:'saved',samples:Array.from({length:6},(_,seq)=>({seq:seq+1,target,timestamp:at,latency_ms:ms,success:true}))})
 const data:Record<string,QualityEvidence>={
  [key(nodes[0])]:{tests:[makeLatency(nodes[0],120) as any],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at},
  [key(nodes[1])]:{tests:[makeLatency(nodes[1],90) as any],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at},
 }
 vi.mocked(readQualityBatch).mockResolvedValueOnce(data)
 const wrapper=mount(RecommendationView,{global});await flushPromises()
 expect(wrapper.findAll('.region-card')).toHaveLength(2)

 // Filter to airport A only
 const airportBtn=wrapper.findAll('.rec-filter-pill').find(b=>b.text().includes('A'))!
 await airportBtn.trigger('click');await flushPromises()
 expect(wrapper.findAll('.region-card')).toHaveLength(1)
 expect(wrapper.find('.region-card h3').text()).toBe('日本')

 // Export Clash uses airport-scoped group name
 await wrapper.get('.recommendation-export button').trigger('click');await flushPromises()
 expect(api.exportClash).toHaveBeenLastCalledWith([scope(nodes[0])],'A · 日常地区推荐')

 // Switching to verified_clean excludes untested node honestly
 const strictBtn=wrapper.findAll('.rec-filter-pill').find(b=>b.text()==='仅选已证纯净')!
 await strictBtn.trigger('click');await flushPromises()
 expect(wrapper.findAll('.region-card')).toHaveLength(0)
 expect(wrapper.text()).toContain('已过滤 1 条')
 wrapper.unmount()
})
