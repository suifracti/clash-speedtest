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
    expect(details.find('.round-summary').text()).toBe('2 个站点 · 3 次采样 · 2/3 成功')
    const groups=details.findAll('.round-target')
    expect(groups[0].text()).toContain('Cloudflare')
    expect(groups[0].findAll('.round-result strong').map(s=>s.text())).toEqual(['80 ms','失败'])
    expect(groups[0].text()).toContain('timeout')
    expect(groups[1].text()).toContain('Google')
    expect(groups[1].find('.round-result strong').text()).toBe('900 ms')
    expect(details.text()).not.toContain('70 ms')
    const google=mount(HealthBars,{props:{points:points(workspace,node,'latency','google'),label:'Google'}})
    const unmeasured=google.findAll('.health-cell')[14]
    expect(unmeasured.attributes('aria-disabled')).toBe('true')
    expect(unmeasured.attributes('aria-label')).toContain('暂无检测记录')
    await unmeasured.trigger('mouseenter');await unmeasured.trigger('focus');await unmeasured.trigger('click')
    expect(google.emitted('inspect')).toBeUndefined();expect(google.emitted('open')).toBeUndefined()
    await google.findAll('.health-cell')[15].trigger('click')
    expect(google.emitted('open')?.[0]?.[0]).toMatchObject({id:'round-a',value:900})
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
    expect(details.text()).toContain('已解锁');expect(details.text()).toContain('部分地区受限');expect(details.text()).toContain('connection refused')
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
