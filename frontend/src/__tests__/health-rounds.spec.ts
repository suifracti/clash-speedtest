import {describe,expect,it,vi} from 'vitest'
import {mount} from '@vue/test-utils'
import {ref} from 'vue'
import HealthBars from '../components/HealthBars.vue'
import RoundResults from '../components/RoundResults.vue'
import MeasurementResults from '../components/MeasurementResults.vue'
import {points,monitorRoundPoints,serviceHealthPoints,serviceSummary} from '../presentation'
import {createWorkspace} from '../workspace'
import {key,type Attempt,type LatencyTest,type MonitorSample,type NodeOption} from '../domain'
vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(key:string)=>items.get(key)||null,setItem:(key:string,value:string)=>items.set(key,value),removeItem:(key:string)=>items.delete(key)}})})

const node:NodeOption={profile_id:'subscription',node_key:'node',node_identity_key:'identity',config_revision_key:'revision',profile_name:'机场',display_name:'新加坡',type:'ss',country_code:'SG',country_flag:''}
const base:LatencyTest={...node,attempt_id:'round-a',node_type:'ss',requested_at:'2026-10-01T00:00:00Z',finished_at:'2026-10-01T00:00:03Z',status:'completed',latency_ms:80,total_samples:3,success_samples:2,failure_samples:1,persistence_state:'saved',samples:[]}

describe('one health column per round',()=>{
  it('keeps the bar specific to its selected site while the hovered round shows every site and failure',async()=>{
    const history:LatencyTest[]=[{...base,samples:[
      {seq:3,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-01T00:00:03Z',success:false,latency_ms:0,error:'timeout'},
      {seq:2,target:'https://www.gstatic.com/generate_204',timestamp:'2026-10-01T00:00:02Z',success:true,latency_ms:900},
      {seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-01T00:00:01Z',success:true,latency_ms:80}
    ]},{...base,attempt_id:'round-older',finished_at:'2026-09-30T23:00:00Z',samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-09-30T23:00:00Z',success:true,latency_ms:70}]}]
    const workspace={latency:ref({[key(node)]:history}),downloads:ref<Record<string,Attempt[]>>({}),services:ref<Record<string,Attempt[]>>({}),displayTarget:ref('cloudflare'),serviceIds:ref<string[]>([])}
    const rounds=points(workspace,node,'latency')
    expect(rounds.map(r=>r.id)).toEqual(['round-older','round-a'])
    expect(rounds[1].samples?.map(s=>[s.label,s.value,s.tone])).toEqual([['第 1 次',80,'good'],['第 2 次',null,'bad']])
    const wrapper=mount(HealthBars,{props:{points:rounds,label:'延迟检测'}})
    const columns=wrapper.findAll('.health-column')
    expect(columns).toHaveLength(16)
    expect(columns.slice(0,14).every(c=>c.findAll('.health-segment.empty').length===1)).toBe(true)
    expect(columns[15].findAll('.health-segment').map(s=>s.classes().filter(c=>['good','bad'].includes(c)))).toEqual([['good'],['bad']])
    await wrapper.findAll('.health-cell')[15].trigger('mouseenter')
    expect(wrapper.emitted('inspect')?.[0]?.[0]).toMatchObject({id:'round-a',value:80})
    const details=mount(RoundResults,{props:{point:wrapper.emitted('inspect')![0][0] as typeof rounds[number]}})
    expect(details.find('.round-summary').text()).toBe('本节点本轮已读 2 / 6 个默认站点记录 · 实际执行 2 / 6 站点 · 3 次采样 · 2/3 成功')
    const groups=details.findAll('.round-target')
    expect(groups[0].text()).toContain('Cloudflare')
    expect(groups[0].findAll('.round-result strong').map(s=>s.text())).toEqual(['80 ms','未确认'])
    expect(groups[0].text()).toContain('timeout')
    expect(groups[1].text()).toContain('Google')
    expect(groups[1].find('.round-result strong').text()).toBe('900 ms')
    expect(details.text()).not.toContain('70 ms')
    const google=mount(HealthBars,{props:{points:points(workspace,node,'latency','google'),label:'Google'}})
    const unmeasured=google.findAll('.health-cell')[14]
    expect(unmeasured.attributes('aria-disabled')).toBeUndefined()
    expect(unmeasured.attributes('aria-label')).toContain('0/0')
    await unmeasured.trigger('mouseenter');await unmeasured.trigger('focus');await unmeasured.trigger('click')
    expect(google.emitted('open')?.[0]?.[0]).toMatchObject({id:'round-older',value:null})
    await google.findAll('.health-cell')[15].trigger('click')
    expect(google.emitted('open')?.[1]?.[0]).toMatchObject({id:'round-a',value:900})
    google.unmount()
    details.unmount()
    wrapper.unmount()
  })
  it('groups monitor samples by their saved round and orders layers by timestamp',()=>{
    const sample:MonitorSample={...node,sample_id:'a',run_id:'run-a',timestamp:'2026-10-01T00:00:01Z',display_name_snapshot:'新加坡',target:'https://cp.cloudflare.com/generate_204',probe_type:'rtt',success:true,latency:80000000,ttfb:0,sampling_tier:'regular'}
    const rounds=monitorRoundPoints([{...sample,sample_id:'b',timestamp:'2026-10-01T00:00:02Z',success:false,error_class:'timeout'}, {...sample,sample_id:'c',run_id:'run-b',timestamp:'2026-10-01T00:01:00Z',latency:120000000},sample])
    expect(rounds.map(r=>[r.id,r.value,r.tone])).toEqual([['run-a',80,'warn'],['run-b',120,'good']])
    expect(rounds[0].samples?.map(s=>[s.id,s.value,s.tone])).toEqual([['a',80,'good'],['b',null,'bad']])
    expect(rounds[1].samples).toHaveLength(1)
    expect(monitorRoundPoints([{...sample,run_id:undefined},{...sample,sample_id:'legacy-b',run_id:undefined}])).toHaveLength(2)
  })
  it('shows each service outcome as a layer of its actual round and keeps legacy attempts separate',()=>{
    const attempt=(id:string,service:string,name:string,outcome:string,request:string):Attempt=>({...node,attempt_id:id,request_id:request,service_id:service,display_name:node.display_name,requested_at:'2026-10-01T00:00:00Z',finished_at:'2026-10-01T00:00:01Z',execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://example.test',name,success_criterion:'服务实际返回符合规则'},result:{outcome,bytes_read:0,finished_at:'2026-10-01T00:00:01Z',error_message:outcome==='transport_error'?'connection refused':undefined}})
    const items=[attempt('n','netflix','Netflix','unlocked','round:run-a:item-n'),attempt('y','youtube','YouTube','region_limited','round:run-a:item-y'),attempt('c','chatgpt','ChatGPT','transport_error','round:run-a:item-c'),attempt('old','netflix','Netflix','unlocked','legacy-request')]
    const rounds=serviceHealthPoints({services:ref({[key(node)]:items}),serviceIds:ref(['netflix','youtube','chatgpt'])},node)
    expect(rounds).toHaveLength(2)
    const round=rounds.find(r=>r.id==='run-a')!
    expect(round.samples?.map(s=>[s.label,s.tone])).toEqual([['Netflix','good'],['YouTube','warn'],['ChatGPT','bad']])
    const details=mount(MeasurementResults,{props:{point:round,project:'service'}})
    expect(details.findAll('.measurement-result')).toHaveLength(3)
    expect(details.text()).toContain('解锁规则通过（播放未验证）');expect(details.text()).toContain('部分地区受限');expect(details.text()).toContain('connection refused')
    details.unmount()
    const w=createWorkspace();w.serviceIds.value=['chatgpt'];w.catalog.value=['netflix','youtube','chatgpt','apple'].map(service_id=>({service_id,name:service_id,category:'服务',description:'',result_kind:'availability',success_criterion:'',timeout_seconds:15}))
    expect(w.displayServiceIds.value).toEqual(['chatgpt','netflix','youtube'])
    w.project.value='service';w.openPlan([node]);w.queue.value=[{id:'queued-chatgpt',node,project:'service',serviceId:'chatgpt',state:'queued'}]
    w.displayServiceIds.value=['youtube','netflix']
    expect(w.serviceIds.value).toEqual(['chatgpt']);expect(w.plan.value?.serviceIds).toEqual(['chatgpt']);expect(w.queue.value[0].serviceId).toBe('chatgpt')
    w.displayServiceIds.value=['netflix','youtube','chatgpt','apple'];expect(w.displayServiceIds.value).toHaveLength(4)
    w.displayServiceIds.value=[];expect(w.displayServiceIds.value).toEqual(['netflix'])
    w.displayServiceIds.value=['youtube','netflix','youtube','unknown'];expect(w.displayServiceIds.value).toEqual(['youtube','netflix'])
    w.services.value[key(node)]=items
    expect(points(w,node,'service',undefined,'youtube').map(p=>p.id)).toEqual(['y'])
    expect(serviceHealthPoints(w,node,['youtube']).flatMap(p=>p.attempts!.map(a=>a.service_id))).toEqual(['youtube'])
    expect(w.latestService(node,'netflix')?.attempt_id).toBe('n')
    w.mergeAttempt('service',{...items[0],attempt_id:'netflix-new-failure',request_id:'new-failure',requested_at:'2026-10-01T00:00:02Z',finished_at:'2026-10-01T00:00:03Z',result:{outcome:'transport_error',bytes_read:0,finished_at:'2026-10-01T00:00:03Z'}})
    w.mergeAttempt('service',{...items[1],attempt_id:'youtube-pending',request_id:'new-pending',requested_at:'2026-10-01T00:00:04Z',finished_at:undefined,execution_state:'queued',persistence_state:'pending',result:undefined})
    expect(w.latestService(node,'netflix')?.attempt_id).toBe('netflix-new-failure')
    expect(w.latestService(node,'youtube')?.attempt_id).toBe('youtube-pending')
    expect(w.latestService(node,'apple')).toBeUndefined()
    expect(serviceSummary(w,node,w.displayServiceIds.value)).toEqual({passed:0,measured:1,total:2})
    expect(points(w,node,'service',undefined,'youtube').map(p=>p.id)).toEqual(['y','youtube-pending'])
    w.catalog.value=w.catalog.value.filter(rule=>rule.service_id!=='youtube')
    expect(w.displayServiceIds.value).toEqual(['netflix'])
    expect(w.serviceIds.value).toEqual(['chatgpt']);expect(w.plan.value?.serviceIds).toEqual(['chatgpt']);expect(w.queue.value[0].serviceId).toBe('chatgpt')
    w.catalog.value=[];expect(w.displayServiceIds.value).toEqual([])
    w.dispose()
  })
})

it('opens a real unexecuted round instead of disguising it as empty history',async()=>{
 const point={id:'planned-root',time:'2026-10-06T00:00:00Z',value:null,tone:'empty' as const,description:'定时 · 未执行 82 项'}
 const wrapper=mount(HealthBars,{props:{points:[point],label:'服务'}})
 const cells=wrapper.findAll('.health-cell')
 await cells.at(-1)!.trigger('click')
 expect(wrapper.emitted('open')?.[0]?.[0]).toEqual(point)
 expect(cells.at(-1)!.attributes('aria-label')).toContain('未执行 82 项')
 expect(cells[0].attributes('aria-disabled')).toBe('true')
})

it('marks manual and scheduled cells with different borders while preserving result colors',()=>{
 const points=[
  {id:'manual',time:'2026-10-01T00:00:00Z',value:null,tone:'bad' as const,trigger:'manual' as const,description:'手动 · HTTP 拒绝'},
  {id:'scheduled',time:'2026-10-01T00:01:00Z',value:8,tone:'good' as const,trigger:'scheduled' as const,description:'定时 · 达到上限'}
 ]
 const wrapper=mount(HealthBars,{props:{points,label:'下载'}}),cells=wrapper.findAll('.health-cell.has-record')
 expect(cells[0].classes()).toContain('trigger-manual');expect(cells[1].classes()).toContain('trigger-scheduled')
 expect(cells[0].find('.health-segment.bad').exists()).toBe(true);expect(cells[1].find('.health-segment.good').exists()).toBe(true)
 expect(cells[0].attributes('title')).toContain('手动');expect(cells[1].attributes('title')).toContain('定时')
 wrapper.unmount()
})

it('places 82 planned service children including cancellation and save failure in one scheduled cell',()=>{
 const round={round_id:'persistent-a',trigger_type:'scheduled' as const,started_at:'2026-10-06T00:00:00Z',state:'interrupted',items:Array.from({length:82},(_,i)=>({...node,request_id:'child-'+i,project:'service' as const,service_id:'service-'+i,execution_state:'not_executed',persistence_state:'not_started',not_executed_reason:'未获得资源'}))}
 const fixture:Attempt={...node,attempt_id:'actual-0',request_id:'child-0',service_id:'service-0',requested_at:round.started_at,execution_state:'completed',persistence_state:'failed',persistence_error:'disk full',rule:{name:'example',target_url:'https://example.test'},result:{outcome:'matched',bytes_read:1,finished_at:round.started_at}}
 round.items[0]={...round.items[0],execution_state:'completed',persistence_state:'failed',service:fixture} as typeof round.items[number]
 const w={services:ref({[key(node)]:[fixture]}),serviceIds:ref(round.items.map(i=>i.service_id)),measurementRounds:ref({[key(node)]:[round]})}
 const p=serviceHealthPoints(w,node)
 expect(p).toHaveLength(1)
 expect(p[0]).toMatchObject({id:'persistent-a',trigger:'scheduled',planned:82})
 expect(p[0].attempts).toHaveLength(82)
 expect(p[0].description).toContain('保存失败 1')
 expect(p[0].description).toContain('未执行 81')
 const wrapper=mount(HealthBars,{props:{points:p,label:'全量服务',aggregate:true}})
 expect(wrapper.findAll('.has-record')).toHaveLength(1)
 expect(wrapper.findAll('.health-segment.bad')).toHaveLength(0)
})

it('keeps every latency target in the persisted round expansion',()=>{
 const t={...base,samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:base.finished_at,success:true,latency_ms:80},{seq:2,target:'https://www.gstatic.com/generate_204',timestamp:base.finished_at,success:false,latency_ms:0,error:'timeout'}]}
 const w={latency:ref({[key(node)]:[t]}),downloads:ref({}),services:ref({}),serviceIds:ref<string[]>(['chatgpt_web']),displayTarget:ref('cloudflare'),measurementRounds:ref({[key(node)]:[{round_id:'manual-root',trigger_type:'manual' as const,started_at:base.requested_at,state:'finished',items:[{...node,request_id:'batch-id',project:'latency' as const,execution_state:'completed',persistence_state:'saved',latency:t}]}]})}
 const p=points(w,node,'latency')
 expect(p).toHaveLength(1);expect(p[0].sampleGroups).toHaveLength(2)
 expect(points(w,node,'combined')[0]).toMatchObject({id:'manual-root',value:80})
 const detail=mount(RoundResults,{props:{point:p[0]}})
 expect(detail.text()).toContain('80 ms');expect(detail.text()).toContain('timeout')
})


it('retains a download round when an independent service filter is selected',()=>{
 const a:Attempt={...node,attempt_id:'download-child',request_id:'download-request',requested_at:base.requested_at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://download.example',maximum_bytes:10485760,maximum_duration_ns:10000000000},result:{outcome:'byte_limit',bytes_read:10485760,duration_ns:1000000000,finished_at:base.finished_at}}
 const w={latency:ref({}),downloads:ref({[key(node)]:[a]}),services:ref({}),serviceIds:ref(['chatgpt_web']),displayTarget:ref('cloudflare'),measurementRounds:ref({[key(node)]:[{round_id:'download-root',trigger_type:'scheduled' as const,started_at:base.requested_at,state:'finished',items:[{...node,request_id:a.request_id,project:'download' as const,execution_state:'completed',persistence_state:'saved',download:a}]}]})}
 expect(points(w,node,'download')).toHaveLength(1)
 expect(points(w,node,'download')[0]).toMatchObject({id:'download-root',trigger:'scheduled',value:10})
})
