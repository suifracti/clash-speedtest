import {expect,it,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import AirportReport from '../components/AirportReport.vue'
import UiSelect from '../components/UiSelect.vue'
import {api} from '../api'
import {createWorkspace,workspaceKey} from '../workspace'
import {readQualityBatch} from '../qualityBatch'
vi.mock('../qualityBatch',()=>({readQualityBatch:vi.fn()}))
vi.mock('../usageDaily',async original=>({...await original<typeof import('../usageDaily')>(),readUsage:vi.fn(async()=>({accounts:[],intervals:[]}))}))
import {airportExitSummary,airportReport} from '../airportReport'
import {key,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
it('summarizes observed node rounds and leaves missing history unknown',()=>{
 const node={profile_id:'p',node_identity_key:'i',config_revision_key:'r',node_key:'n'} as NodeOption,at='2026-10-20T10:00:00Z'
 const evidence={tests:[{attempt_id:'good',samples:[{seq:1,target:'cloudflare',timestamp:at,success:true,latency_ms:10}]},{attempt_id:'bad',samples:[{seq:1,target:'cloudflare',timestamp:at,success:false,latency_ms:0,error:'timeout'}]}],monitor:[],reads:{latency:{complete:true},monitor:{complete:true}}} as unknown as QualityEvidence
 const report=airportReport([node],{[key(node)]:evidence});expect(report.rate).toBe(50);expect(report.total).toBe(2);expect(report.failures).toEqual([['超时',1]]);expect(report.trend[0].value).toBe(10)
 const missing=airportReport([node],{});expect(missing.rate).toBeNull();expect(missing.incomplete).toBe(true);expect(missing.trend).toEqual([])
})

it('filters the report and ranking together, sorts rows and opens the inspector',async()=>{
 const make=(id:string,country:string,name:string)=>({profile_id:'p',node_identity_key:id,config_revision_key:'r',node_key:id,country_code:country,display_name:name,type:'Hysteria2'} as NodeOption)
 const nodes=[make('fast','JP','日本专线01'),make('slow','JP','日本专线02'),make('other','SG','新加坡高速01')],at='2026-10-20T10:00:00Z'
 const evidence=Object.fromEntries(nodes.map((n,i)=>[key(n),{tests:[{...n,attempt_id:n.node_key,requested_at:at,finished_at:at,total_samples:3,success_samples:3,samples:[1000,200+i*200,200+i*200].map((ms,j)=>({seq:j+1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,success:true,latency_ms:ms}))}],monitor:[],reads:{latency:{complete:true},monitor:{complete:true}}}])) as unknown as Record<string,QualityEvidence>
 const slow=evidence[key(nodes[1]!)].tests[0]!;evidence[key(nodes[1]!)].tests=[0,1,2].map(i=>({...slow,attempt_id:'slow-'+i}))
 const settings=vi.spyOn(api,'settings').mockResolvedValue({} as never);vi.mocked(readQualityBatch).mockResolvedValue(evidence)
 const w=createWorkspace();w.airports.value=[{id:'a',name:'机场',subscriptions:[{id:'p',airport_id:'a'}]}] as never;w.nodes.value=nodes
 const wrapper=mount(AirportReport,{global:{provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}});await flushPromises()
 const selects=wrapper.get('.report-filters').findAllComponents(UiSelect)
 selects[0]!.vm.$emit('update:modelValue','JP');selects[1]!.vm.$emit('update:modelValue','专线');selects[2]!.vm.$emit('update:modelValue','Hysteria2');await flushPromises()
 expect(wrapper.get('.report-filters').text()).toContain('2 / 3 个节点')
 expect(wrapper.get('.report-metrics').text()).toContain('12/12 次');expect(wrapper.findAll('.rank-node').map(r=>r.text())).toEqual(['日本 · 02 · 专线','日本 · 01 · 专线'])
 expect(wrapper.find('th[aria-sort="ascending"]').exists()).toBe(false);expect(wrapper.get('thead').text()).toContain('轮数');expect(wrapper.findAll('.rank-table-wrap tbody tr')[0]!.findAll('td')[1]!.classes()).toContain('good');expect(wrapper.findAll('.rank-table-wrap tbody tr')[0]!.findAll('td')[2]!.text()).toBe('3');expect(wrapper.findAll('.rank-table-wrap tbody tr')[0]!.findAll('td')[4]!.text()).toBe('—')
 expect(wrapper.findAll('.rank-low')).toHaveLength(1);expect(wrapper.findAll('.rank-low')[0]!.text()).toBe('轮数少')
 expect(wrapper.findAll('.beat-cell.good')).toHaveLength(1);expect(readQualityBatch).toHaveBeenCalledTimes(1)
 await wrapper.findAll('th button').find(b=>b.text().startsWith('稳定延迟'))!.trigger('click')
 expect(wrapper.get('th[aria-sort="ascending"]').text()).toContain('稳定延迟');expect(wrapper.findAll('.rank-node')[0]!.text()).toContain('01')
 await wrapper.findAll('th button').find(b=>b.text().startsWith('稳定延迟'))!.trigger('click');expect(wrapper.findAll('.rank-node')[0]!.text()).toContain('02')
 await wrapper.findAll('.rank-node')[0]!.trigger('click');expect(w.viewRequest.value?.node_key).toBe('slow');expect(w.page.value).toBe('home')
 wrapper.unmount();w.dispose();settings.mockRestore()
})

it('keeps the selected airport report when an older airport read finishes later',async()=>{
 const make=(id:string)=>({profile_id:'p-'+id,node_identity_key:id,config_revision_key:'r',node_key:id,display_name:'节点 '+id,type:'Vless'} as NodeOption)
 const a=make('a'),b=make('b'),at='2026-10-20T10:00:00Z'
 const history=(node:NodeOption,success:boolean)=>({
  [key(node)]:{
   tests:[{...node,attempt_id:node.node_key,requested_at:at,total_samples:2,success_samples:success?2:0,
    samples:[1,2].map(seq=>({seq,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,success,latency_ms:success?200:0,error:success?'':'timeout'}))}],
   monitor:[],reads:{latency:{complete:true},monitor:{complete:true}},
  },
 }) as unknown as Record<string,QualityEvidence>
 let finishA!:(value:Record<string,QualityEvidence>)=>void
 vi.mocked(readQualityBatch).mockReset().mockImplementationOnce(()=>new Promise(resolve=>{finishA=resolve})).mockResolvedValueOnce(history(b,true))
 const settings=vi.spyOn(api,'settings').mockResolvedValue({airport_monthly_prices:{a:10,b:20}} as never)
 const w=createWorkspace();w.airports.value=[{id:'a',name:'机场A',subscriptions:[{id:'p-a',airport_id:'a'}]},{id:'b',name:'机场B',subscriptions:[{id:'p-b',airport_id:'b'}]}] as never;w.nodes.value=[a,b]
 const wrapper=mount(AirportReport,{global:{provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}})
 try{
  await flushPromises()
  wrapper.findAllComponents(UiSelect)[0]!.vm.$emit('update:modelValue','b');await flushPromises()
  expect(wrapper.get('.report-metrics').text()).toContain('2/2 次')
  finishA(history(a,false));await flushPromises()
  expect(wrapper.findAllComponents(UiSelect)[0]!.props('modelValue')).toBe('b')
  expect(wrapper.get('.report-metrics').text()).toContain('2/2 次')
  expect(wrapper.get('.rank-node').text()).toContain('b')
  expect((wrapper.get('[aria-label="机场月费"]').element as HTMLInputElement).value).toBe('20')
 }finally{wrapper.unmount();w.dispose();settings.mockRestore()}
})

it('saves the airport and price chosen before a delayed settings read even after switching airports',async()=>{
 const make=(id:string)=>({profile_id:'p-'+id,node_key:id,node_identity_key:id,config_revision_key:'r',display_name:id,type:'ss'}) as NodeOption
 const a=make('a'),b=make('b'),at='2026-11-18T00:00:00Z'
 const history=Object.fromEntries([a,b].map(n=>[key(n),{tests:[{...n,attempt_id:n.node_key,requested_at:at,total_samples:1,success_samples:1,samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,success:true,latency_ms:100}]}],monitor:[],reads:{latency:{complete:true},monitor:{complete:true}}}])) as unknown as Record<string,QualityEvidence>
 vi.mocked(readQualityBatch).mockReset().mockResolvedValue(history)
 const settings=vi.spyOn(api,'settings').mockResolvedValue({airport_monthly_prices:{a:10,b:20}} as never)
 const save=vi.spyOn(api,'saveSettings').mockResolvedValue(undefined)
 const w=createWorkspace();w.airports.value=[{id:'a',name:'A',subscriptions:[{id:'p-a',airport_id:'a'}]},{id:'b',name:'B',subscriptions:[{id:'p-b',airport_id:'b'}]}] as never;w.nodes.value=[a,b]
 const wrapper=mount(AirportReport,{global:{provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}})
 try{
  await flushPromises();await wrapper.get('[aria-label="机场月费"]').setValue(35)
  let finish!:(value:never)=>void;settings.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
  await wrapper.findAll('button').find(button=>button.text()==='保存月费')!.trigger('click')
  wrapper.findAllComponents(UiSelect)[0]!.vm.$emit('update:modelValue','b');await flushPromises()
  expect((wrapper.get('[aria-label="机场月费"]').element as HTMLInputElement).value).toBe('20')
  finish({airport_monthly_prices:{a:10,b:20}} as never);await flushPromises()
  expect(save).toHaveBeenCalledWith(expect.objectContaining({airport_monthly_prices:{a:35,b:20}}))
  expect((wrapper.get('[aria-label="机场月费"]').element as HTMLInputElement).value).toBe('20')
 }finally{wrapper.unmount();w.dispose();settings.mockRestore();save.mockRestore()}
})

it('summarizes saved exit purity/CF challenge stats and filters rank table by search and problems',async()=>{
 const make=(id:string,name:string)=>({profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',display_name:name,country_code:'JP',type:'ss'} as NodeOption)
 const okNode=make('ok','日本01'),badNode=make('bad','日本02'),at='2026-11-20T08:00:00Z'
 const history:Record<string,QualityEvidence>={
  [key(okNode)]:{
   tests:[{...okNode,attempt_id:'t-ok',requested_at:at,total_samples:3,success_samples:3,samples:[1,2,3].map(seq=>({seq,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,success:true,latency_ms:120}))}],
   monitor:[],downloads:[],
   services:[
    {...okNode,attempt_id:'s-pure',service_id:'ippure_ip_quality',execution_state:'completed',persistence_state:'saved',rule:{service_id:'ippure_ip_quality'},result:{outcome:'profiled',finished_at:at,details:{risk_score:'12',risk_scale_max:'100',risk_direction:'lower_is_better'}}},
    {...okNode,attempt_id:'s-cf',service_id:'cloudflare_challenge',execution_state:'completed',persistence_state:'saved',rule:{service_id:'cloudflare_challenge'},result:{outcome:'profiled',finished_at:at,details:{cf_challenge_detail:'ChatGPT: 直通(200)'}}},
   ],
   reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at,
  } as unknown as QualityEvidence,
  [key(badNode)]:{
   tests:[{...badNode,attempt_id:'t-bad',requested_at:at,total_samples:3,success_samples:1,samples:[1,2,3].map(seq=>({seq,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,success:seq===1,latency_ms:seq===1?150:0,error:seq===1?'':'timeout'}))}],
   monitor:[],downloads:[],
   services:[
    {...badNode,attempt_id:'s-cf2',service_id:'cloudflare_challenge',execution_state:'completed',persistence_state:'saved',rule:{service_id:'cloudflare_challenge'},result:{outcome:'challenge',finished_at:at,details:{cf_challenge_detail:'ChatGPT: 弹盾(403)'}}},
   ],
   reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at,
  } as unknown as QualityEvidence,
 }
 expect(airportExitSummary([okNode,badNode],history)).toMatchObject({purityCount:1,purityAvg:88,purityClean:1,cfCount:2,cfGood:1,cfBad:1})
 vi.mocked(readQualityBatch).mockReset().mockResolvedValue(history)
 const settings=vi.spyOn(api,'settings').mockResolvedValue({} as never)
 const w=createWorkspace();w.airports.value=[{id:'a',name:'机场A',subscriptions:[{id:'p',airport_id:'a'}]}] as never;w.nodes.value=[okNode,badNode]
 const wrapper=mount(AirportReport,{global:{provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}})
 try{
  await flushPromises()
  expect(wrapper.get('.report-metrics').text()).toContain('均分 88')
  expect(wrapper.get('.report-metrics').text()).toContain('CF 盾：1 直通 · 0 部分 · 1 易弹')
  await wrapper.get('.rank-filter-pill').trigger('click')
  expect(wrapper.findAll('.rank-node').map(r=>r.text())).toEqual(['日本 · 02'])
  await wrapper.get('.rank-filter-pill').trigger('click')
  await wrapper.get('.rank-search').setValue('01')
  expect(wrapper.findAll('.rank-node').map(r=>r.text())).toEqual(['日本 · 01'])
 }finally{wrapper.unmount();w.dispose();settings.mockRestore()}
})

it('flags divergent and single-source fallback purity nodes separately from clean consensus in airport report',async()=>{
 const make=(id:string,name:string)=>({profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',display_name:name,country_code:'SG',type:'Hysteria2'} as NodeOption)
 const divNode=make('div','新加坡01'),fbNode=make('fb','新加坡02'),at='2026-11-20T12:00:00Z'
 const mkLatency=(n:NodeOption)=>({...n,attempt_id:'t-'+n.node_key,requested_at:at,total_samples:3,success_samples:3,samples:[1,2,3].map(seq=>({seq,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,success:true,latency_ms:100}))})
 const mkSvc=(n:NodeOption,id:string,outcome:string,details:Record<string,string>={})=>({...n,attempt_id:`s-${n.node_key}-${id}`,service_id:id,execution_state:'completed',persistence_state:'saved',rule:{service_id:id},result:{outcome,finished_at:at,details}})
 const history:Record<string,QualityEvidence>={
  [key(divNode)]:{
   tests:[mkLatency(divNode)],monitor:[],downloads:[],
   services:[mkSvc(divNode,'ipquery_ip_quality','profiled',{risk_score:'0'}),mkSvc(divNode,'iplogs_ip_quality','profiled',{risk_score:'38'})],
   reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at,
  } as unknown as QualityEvidence,
  [key(fbNode)]:{
   tests:[mkLatency(fbNode)],monitor:[],downloads:[],
   services:[mkSvc(fbNode,'ippure_ip_quality','timed_out'),mkSvc(fbNode,'ping0_ipv4','profiled',{risk_score:'15'})],
   reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at,
  } as unknown as QualityEvidence,
 }
 expect(airportExitSummary([divNode,fbNode],history)).toMatchObject({purityCount:2,purityClean:0,purityDivergent:1,purityFallback:1,purityTone:'warn'})
 vi.mocked(readQualityBatch).mockReset().mockResolvedValue(history)
 const settings=vi.spyOn(api,'settings').mockResolvedValue({} as never)
 const w=createWorkspace();w.airports.value=[{id:'a',name:'机场A',subscriptions:[{id:'p',airport_id:'a'}]}] as never;w.nodes.value=[divNode,fbNode]
 const wrapper=mount(AirportReport,{global:{provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}})
 try{
  await flushPromises()
  expect(wrapper.get('.report-metrics').text()).toContain('分歧大 1 · 单源回退 1')
  expect(wrapper.get('.rank-table-wrap').text()).toContain('IPQuery0')
  expect(wrapper.get('.rank-table-wrap').text()).toContain('IPLogs38')
  expect(wrapper.get('.rank-table-wrap').text()).toContain('单源 85')
  // Both divergent and fallback nodes appear when filtering "仅看有问题".
  await wrapper.get('.rank-filter-pill').trigger('click')
  expect(wrapper.findAll('.rank-node')).toHaveLength(2)
 }finally{wrapper.unmount();w.dispose();settings.mockRestore()}
})
