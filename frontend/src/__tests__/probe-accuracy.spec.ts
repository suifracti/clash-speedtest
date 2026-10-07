import {expect,it} from 'vitest'
import {ref} from 'vue'
import {mount} from '@vue/test-utils'
import RoundResults from '../components/RoundResults.vue'
import {attemptStatus,downloadCondition} from '../measurementRounds'
import {createPresentationCache} from '../presentation'
import {key,type Attempt,type MonitorSample,type NodeOption} from '../domain'
const n:NodeOption={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',profile_name:'p',display_name:'n',type:'ss',country_code:'',country_flag:''}
const a:Attempt={...n,attempt_id:'a',request_id:'q',display_name:'n',requested_at:'2026-10-07T00:00:00Z',execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://speed.cloudflare.com/__down?bytes=10485761',method:'GET',maximum_bytes:10485760,network_path_method:'physical_socket_v1'},result:{outcome:'byte_limit',bytes_read:10485760,duration_ns:1e9,finished_at:'2026-10-07T00:00:01Z'}}
it('labels a full bounded download as normal completion and includes partial conditions',()=>{
 expect(attemptStatus(a,true)).toBe('完成·10 MiB')
 expect(attemptStatus({...a,result:{...a.result!,outcome:'time_limit',bytes_read:5242880,duration_ns:10e9}},true)).toContain('5.0 MiB / 10.00 秒')
})
it('does not merge declared physical routing with actually verified socket routing',()=>{
 const verified={...a,result:{...a.result!,network_path:{method:'physical_socket_v1',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0}}}
 expect(downloadCondition(a)).not.toBe(downloadCondition(verified))
})
it('projects six-target scheduled records per node without leaking Cloudflare into Google',()=>{
 const base:MonitorSample={...n,sample_id:'s',run_id:'round',display_name_snapshot:'n',timestamp:'2026-10-07T00:00:00Z',target:'https://www.gstatic.com/generate_204',probe_type:'rtt',success:true,latency:120e6,ttfb:120e6,sampling_tier:'regular',trigger_type:'scheduled'}
 const w={displayTarget:ref('cloudflare'),serviceIds:ref([] as string[]),latency:ref({}),downloads:ref({}),services:ref({})}
 const monitor=ref({[key(n)]:[base]});const cache=createPresentationCache(w,monitor,ref({}))
 expect(cache.site(n,'google')).toHaveLength(1)
 expect(cache.site(n,'google')[0].value).toBe(120)
 expect(cache.site(n,'cloudflare')).toHaveLength(0)
 monitor.value[key(n)][0].latency=300e6
 expect(cache.site(n,'google')[0].value).toBe(300)
})
it('separates service uncertainty from node failure and actual business access',()=>{
 expect(attemptStatus({...a,execution_state:'failed',result:{...a.result!,outcome:'timed_out'}},false)).toContain('已执行 · 未确认（超时）')
 expect(attemptStatus({...a,result:{...a.result!,outcome:'reachable'}},false)).toContain('HTTP 可达（业务／解锁未验证）')
 expect(attemptStatus({...a,execution_state:'not_executed',result:undefined},false)).toBe('未执行')
})
it('keeps planned but unexecuted six-site targets out of failure and successful coverage',()=>{
 const s:MonitorSample={...n,sample_id:'skip',run_id:'six',display_name_snapshot:'n',timestamp:'2026-10-07T00:00:00Z',target:'https://www.gstatic.com/generate_204',probe_type:'rtt',success:false,latency:0,ttfb:0,sampling_tier:'regular',error_class:'not_executed',error_detail:'proxy setup failed'}
 const w={displayTarget:ref('cloudflare'),serviceIds:ref([] as string[]),latency:ref({}),downloads:ref({}),services:ref({})};const cache=createPresentationCache(w,ref({[key(n)]:[s]}),ref({}))
 expect(cache.site(n,'google')[0].tone).toBe('empty')
 expect(cache.site(n,'google')[0].sampleGroups![0].samples[0].tone).toBe('empty')
 const ui=mount(RoundResults,{props:{point:cache.site(n,'google')[0]}});expect(ui.find('.round-summary').text()).toContain('实际执行 0 / 6 站点');ui.unmount()
})
it('keeps all target children available from a scheduled site cell while its value stays site-specific',()=>{
 const base:MonitorSample={...n,sample_id:'cf',run_id:'run-six',display_name_snapshot:'n',timestamp:'2026-10-07T00:00:00Z',target:'https://speed.cloudflare.com/__down?bytes=1',probe_type:'rtt',success:true,latency:100e6,ttfb:100e6,sampling_tier:'regular'}
 const w={displayTarget:ref('cloudflare'),serviceIds:ref([] as string[]),latency:ref({}),downloads:ref({}),services:ref({})}
 const cache=createPresentationCache(w,ref({[key(n)]:[base,{...base,sample_id:'google',target:'https://www.gstatic.com/generate_204',latency:300e6}]}),ref({}))
 const google=cache.site(n,'google')[0];expect(google.value).toBe(300);expect(google.sampleGroups).toHaveLength(2)
})
it('does not count an explicitly unexecuted persisted result as an executed service',()=>{
 const unexecuted={...a,service_id:'cloudflare_204',execution_state:'not_executed',result:{...a.result!,outcome:'transport_error',details:{execution_status:'not_executed'}}}
 const w={displayTarget:ref('cloudflare'),serviceIds:ref(['cloudflare_204']),latency:ref({}),downloads:ref({}),services:ref({}),measurementRounds:ref({[key(n)]:[{round_id:'skip',trigger_type:'manual' as const,started_at:a.requested_at,state:'finished',items:[{...n,project:'service' as const,service_id:'cloudflare_204',request_id:'q',execution_state:'not_executed',persistence_state:'saved',service:unexecuted}]}]})}
 const cache=createPresentationCache(w,ref({}),ref({}));expect(cache.services(n)[0].description).toContain('实际执行 0 · 未执行 1')
})

it('retains HTTP refusal latency as a warning with a reason rather than a node failure',()=>{
 const sample:MonitorSample={...n,sample_id:'http',sampling_tier:'regular',run_id:'six-http',display_name_snapshot:'n',timestamp:'2026-10-07T00:00:00Z',target:'https://api.github.com/zen',probe_type:'rtt',success:true,latency:368e6,ttfb:368e6,error_detail:'HTTP 403；已测得响应时延，业务／解锁未确认'}
 const w={displayTarget:ref('github'),serviceIds:ref([] as string[]),latency:ref({}),downloads:ref({}),services:ref({})};const cache=createPresentationCache(w,ref({[key(n)]:[sample]}),ref({}));const p=cache.site(n,'github')[0]
 expect(p.value).toBe(368);expect(p.tone).toBe('warn');expect(p.sampleGroups![0].samples[0].note).toContain('403')
})
