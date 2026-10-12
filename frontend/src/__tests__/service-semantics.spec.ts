import {expect,it,vi} from 'vitest'
import {ref,nextTick} from 'vue'
import {points,serviceHealthPoints,serviceSummary} from '../presentation'
import {durableRoundPoints} from '../measurementRounds'
import {serviceComparisonGroup} from '../sharedResults'
import {createWorkspace} from '../workspace'
import {key,type Attempt,type NodeOption,type MeasurementRound} from '../domain'
vi.mock('../api',async original=>{const actual=await original<typeof import('../api')>();return {...actual,api:{...actual.api,attemptHistory:vi.fn(async()=>({attempts:[],has_more:false,complete:true}))}}})
const n:NodeOption={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',profile_name:'fixture',display_name:'fixture',type:'ss',country_code:'HK',country_flag:''}
const at='2026-10-09T18:33:25Z'
const attempt=(id:string,outcome:string):Attempt=>({...n,attempt_id:id,request_id:'round:fixture:'+id,service_id:id,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{service_id:id,name:id,target_url:'https://fixture.invalid/'},result:{outcome,bytes_read:0,finished_at:at,details:{execution_status:'executed'}}})
const unknown=()=>({...attempt('ipok_ipv4','unknown'),result:{...attempt('ipok_ipv4','unknown').result!,failure_phase:'timeout'}})
const workspace=(list:Attempt[],ids=list.map(a=>a.service_id!))=>({displayTarget:ref('cloudflare'),serviceIds:ref(ids),latency:ref({}),downloads:ref({}),services:ref({[key(n)]:list}),latestService:(_node:NodeOption,id:string)=>list.find(a=>a.service_id===id)})
const round=(list:Attempt[]):MeasurementRound=>({round_id:'fixture',started_at:at,trigger_type:'manual',state:'finished',items:list.map(a=>({...n,request_id:a.request_id,project:'service',service_id:a.service_id,execution_state:a.execution_state,persistence_state:a.persistence_state,service:a}))})
it('repairs unlinked IPOK unknown without mutating the attempt',()=>{const a=unknown(),raw=JSON.stringify(a);expect(points(workspace([a]),n,'service','cloudflare','ipok_ipv4')[0].value).toBeNull();expect(JSON.stringify(a)).toBe(raw)})
it('repairs linked IPOK unknown and preserves its child evidence',()=>{const a=unknown(),r=round([a]),raw=JSON.stringify(r),p=durableRoundPoints([r],'service','cloudflare','ipok_ipv4')[0];expect(p.value).toBeNull();expect(p.samples![0].value).toBeNull();expect(p.attempts![0]).toBe(a);expect(JSON.stringify(r)).toBe(raw)})
it('excludes successful profiles from business passes and comparison conditions',()=>{
 const a=attempt('netflix_unlock','unlocked'),profile=attempt('ipok_ipv4','profiled'),w=workspace([a,profile])
 expect(serviceHealthPoints(w,n)[0].value).toBe(1);expect(points(w,n,'service','cloudflare','ipok_ipv4')[0].value).toBeNull();expect(serviceComparisonGroup([a,profile],['netflix_unlock','ipok_ipv4'])).toEqual(serviceComparisonGroup([a],['netflix_unlock']));expect(serviceSummary(w as any,n)).toEqual({passed:1,measured:1,total:1})
})
it('counts evidenced business rejection as failure in both history paths',()=>{const a=attempt('netflix_unlock','region_blocked');expect(points(workspace([a]),n,'service')[0].value).toBe(0);const p=durableRoundPoints([round([a])],'service')[0];expect(p.value).toBe(0);expect(p.samples![0].value).toBe(0);expect(p.samples![0].description).toContain('判据未通过')})
it('keeps unknown children null without diluting confirmed partial evidence',()=>{
 const list=[attempt('netflix_unlock','unlocked'),attempt('youtube_premium','unknown'),attempt('ipok_ipv4','profiled')],p=serviceHealthPoints(workspace(list),n)[0]
 expect(p.value).toBe(1);expect(p.tone).toBe('warn');expect(p.samples!.map(s=>s.value)).toEqual([1,null,null]);expect(durableRoundPoints([round(list)],'service')[0].value).toBe(1);expect(serviceComparisonGroup(list,list.map(a=>a.service_id!)).complete).toBe(false)
})
it('leaves all unknown or profile-only histories unrankable',()=>{const list=[attempt('netflix_unlock','unknown'),unknown()];expect(serviceHealthPoints(workspace(list),n)[0].value).toBeNull();expect(durableRoundPoints([round(list)],'service')[0].value).toBeNull();expect(serviceComparisonGroup(list,list.map(a=>a.service_id!)).complete).toBe(false);expect(serviceComparisonGroup([attempt('ipok_ipv4','profiled')],['ipok_ipv4']).complete).toBe(false)})
it('does not count transport, provider quota or parser limits as business rejection',()=>{
 for(const outcome of ['transport_error','timed_out','rate_limited','reachable','auth_failed','permission_denied'])expect(points(workspace([attempt('chatgpt_web',outcome)]),n,'service')[0].value).toBeNull()
 const a=attempt('chatgpt_web','criteria_mismatch');a.result!.failure_phase='response_limit';a.result!.http_status=200;expect(points(workspace([a]),n,'service')[0].value).toBeNull()
})
it('workspace ranks confirmed outcomes ahead of unknown, excluding profiles',async()=>{
 const w=createWorkspace(),other={...n,node_key:'other',node_identity_key:'other',display_name:'other'}
 w.airports.value=[{id:'airport',name:'fixture',url_display:'',node_count:2,has_cache:true,subscriptions:[{id:'p',airport_id:'airport',name:'fixture',url_display:'',url_configured:true,node_count:2,has_cache:true}]}];w.nodes.value=[n,other];w.setAirports(['airport']);w.catalog.value=['netflix_unlock','ipok_ipv4'].map(service_id=>({service_id,name:service_id,category:'fixture',description:'',result_kind:service_id==='ipok_ipv4'?'ip_quality':'streaming_unlock',success_criterion:'',timeout_seconds:10}));w.displayServiceIds.value=['netflix_unlock','ipok_ipv4'];w.services.value={[key(n)]:[attempt('netflix_unlock','unknown'),unknown()],[key(other)]:[{...attempt('netflix_unlock','region_blocked'),...other},attempt('ipok_ipv4','unknown')]};w.sort.value='service';await nextTick();await w.refreshSort();w.sortSnapshots.value={[key(n)]:{services:w.services.value[key(n)]},[key(other)]:{services:w.services.value[key(other)]}} as any
 expect(w.filteredNodes.value.map(key)).toEqual([key(other),key(n)]);w.dispose()
})

it('keeps a mixed confirmed pass and rejection at one half despite unknown and profile children',()=>{
 const list=[attempt('netflix_unlock','unlocked'),attempt('youtube_premium','region_blocked'),attempt('chatgpt_web','unknown'),attempt('ipok_ipv4','profiled')]
 expect(serviceHealthPoints(workspace(list),n)[0].value).toBe(.5);expect(durableRoundPoints([round(list)],'service')[0].value).toBe(.5)
})
it('recognizes profile snapshot metadata beyond legacy IDs and leaves HTTP-only evidence unknown',()=>{
 const a=attempt('fixture_new_profile','unknown');a.rule={...a.rule,result_kind:'exit_profile'} as Attempt['rule'];const pass=attempt('netflix_unlock','unlocked')
 expect(serviceComparisonGroup([pass,a],['netflix_unlock','fixture_new_profile'])).toEqual(serviceComparisonGroup([pass],['netflix_unlock']))
 const rejected=attempt('chatgpt_web','http_rejected');rejected.result!.http_status=403;expect(points(workspace([rejected]),n,'service')[0].value).toBe(0)
 rejected.result!.http_status=429;expect(points(workspace([rejected]),n,'service')[0].value).toBeNull()
})
