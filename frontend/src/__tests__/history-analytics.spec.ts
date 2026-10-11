import {describe,it,expect} from 'vitest'
import {analyzeHistory} from '../historyAnalytics'
import type {MonitorSample,LatencyTest} from '../domain'
const context={since:'2026-10-01T00:00:00Z',until:'2026-10-03T00:00:00Z',hasMore:false}
const sample=(id:string,patch:Partial<MonitorSample>={}):MonitorSample=>({sample_id:id,run_id:'round-1',profile_id:'p',node_key:'n',node_identity_key:'old-id',config_revision_key:'r',display_name_snapshot:'deleted',timestamp:'2026-10-01T01:00:00Z',target:'site',probe_type:'rtt',sampling_tier:'regular',success:true,latency:10e6,ttfb:0,...patch})
const test=(patch:Partial<LatencyTest>={}):LatencyTest=>({profile_id:'p',node_key:'n',node_identity_key:'id',config_revision_key:'r',attempt_id:'attempt',display_name:'node',node_type:'ss',requested_at:context.since,finished_at:context.until,status:'completed',persistence_state:'saved',method:'http',method_version:1,source:'manual',latency_ms:10,total_samples:2,success_samples:2,failure_samples:0,samples:[{seq:1,target:'site',timestamp:'2026-10-01T01:00:00Z',latency_ms:10,success:true},{seq:2,target:'other',timestamp:'2026-10-01T02:00:00Z',latency_ms:20,success:true}],network_path:{method:'bound',interface:'en0',dns_mode:'bound',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:1,address_family:'ipv4',address_source:'dns'},...patch})
describe('loaded history evidence',()=>{
 it('keeps empty and excluded-only quality untested',()=>{
  expect(analyzeHistory([],context).groups).toEqual([])
  const a=analyzeHistory(['cancelled','not_executed','local_path','rate_limited'].map((error_class,i)=>sample(String(i),{success:false,error_class})),context)
  expect(a.groups[0]).toMatchObject({count:4,eligible:0,excluded:4,successRate:null,p50:null,p95:null})
 })
 it('deduplicates, filters numeric time and computes successful-only percentiles with explicit denominator',()=>{
  const records=[sample('one',{timestamp:'2026-10-01T09:00:00+08:00'}),sample('two',{latency:30e6,timestamp:'2026-10-02T02:00:00Z',run_id:'round-2'}),sample('timeout',{success:false,error_class:'timeout'}),sample('cancel',{success:false,error_class:'cancelled'}),sample('429',{error_detail:'HTTP 429',success:true}),sample('future',{timestamp:'2026-10-04T00:00:00Z'}),sample('invalid',{timestamp:'0001-01-01T00:00:00Z'})]
  const a=analyzeHistory([...records,records[0]!],{...context,hasMore:true})
  expect(a).toMatchObject({loaded:8,inWindow:5,complete:false})
  expect(a.groups[0]).toMatchObject({count:5,eligible:3,success:2,timeout:1,failed:0,excluded:2,p50:20,p95:30,rounds:2,days:2,comparable:false,firstAt:'2026-10-01T09:00:00+08:00',lastAt:'2026-10-02T02:00:00Z'})
  expect(a.groups[0]!.successRate).toBeCloseTo(66.6666667)
  expect(a.points.map(p=>p.id)).toEqual(['monitor:one','monitor:timeout','monitor:cancel','monitor:429','monitor:two'])
 })
 it('does not merge old identity, revision, probe or tier and ignores current-scope mismatches',()=>{
  const records=[sample('old'),sample('new',{node_identity_key:'new-id'}),sample('revision',{config_revision_key:'r2'}),sample('probe',{probe_type:'ttfb'}),sample('tier',{sampling_tier:'diagnostic'})]
  expect(analyzeHistory(records,context).groups).toHaveLength(5)
  expect(analyzeHistory(records,{...context,identity:'new-id',profile:'p'}).inWindow).toBe(1)
  expect(analyzeHistory(records,{...context,profile:'other'}).groups).toHaveLength(0)
 })
 it('keeps failures and invalid successful timings separate from percentiles',()=>{
  const g=analyzeHistory([sample('bad',{success:false,error_class:'conn_error'}),sample('nan',{latency:NaN}),sample('http',{probe_type:'service_github',latency:0})],context).groups
  expect(g[0]).toMatchObject({eligible:2,success:1,failed:1,invalidTiming:1,p50:null})
  expect(g[1]?.p50).toBeNull()
 })
 it('separates method/version/address family and never counts multi-site samples as independent rounds',()=>{
  const a=analyzeHistory([],context,[test(),test({attempt_id:'v6',network_path:{...test().network_path!,address_family:'ipv6'}}),test({attempt_id:'old',method_version:undefined,network_path:undefined}),test({attempt_id:'method',method:'tcp'})])
  expect(a.groups).toHaveLength(8)
  expect(a.groups.filter(g=>g.comparable)).toHaveLength(6)
  expect(a.groups.every(g=>g.rounds===1&&g.days===1)).toBe(true)
  expect(analyzeHistory([],context,[test({source:'other'})]).groups[0]?.condition).not.toBe(a.groups[0]?.condition)
 })
})
it('uses nearest-rank P95 for twenty successful readings and marks read failures incomplete',()=>{
 const samples=Array.from({length:20},(_,i)=>sample(String(i),{latency:(i+1)*1e6}))
 expect(analyzeHistory(samples,context).groups[0]).toMatchObject({success:20,eligible:20,successRate:100,p50:10.5,p95:19,rounds:1,days:1})
 expect(analyzeHistory(samples,{...context,error:'offline'}).complete).toBe(false)
})
it('recognizes structured nonexecution and target throttling without treating them as successful quality',()=>{
 const records=[sample('structured',{metadata:{execution_status:'not_executed'} as MonitorSample['metadata']}),sample('429',{metadata:{http_status:429} as MonitorSample['metadata']})]
 expect(analyzeHistory(records,context).groups[0]).toMatchObject({eligible:0,success:0,excluded:2})
})
it('does not confuse a target port or diagnostic duration with HTTP 429 throttling',()=>{
 const records=[sample('port',{success:false,error_class:'conn_refused',error_detail:'dial tcp 203.0.113.1:429: connect: connection refused'}),sample('duration',{success:false,error_class:'conn_error',error_detail:'transport failed after 429 ms'}),sample('status',{success:false,error_class:'http_status_error',error_detail:'HTTP status 429'})]
 expect(analyzeHistory(records,context).groups[0]).toMatchObject({eligible:2,failed:2,excluded:1,successRate:0})
})
