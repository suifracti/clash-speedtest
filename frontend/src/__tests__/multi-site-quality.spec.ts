import {describe,expect,it,vi} from 'vitest'
import {multiSiteQuality,compareSiteQuality,qualityGroups,type QualityEvidence} from '../nodeQuality'
import {latencyTargets} from '../latencyTargets'
import type {LatencyTest,MonitorSample,NetworkPath,NodeScope} from '../domain'

const node:NodeScope={profile_id:'airport-a/sub',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'rev-a'}
const since='2026-10-09T00:00:00Z',until='2026-10-11T00:00:00Z',at='2026-10-10T23:00:00Z'
const path:NetworkPath={method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:'IPv4',address_source:'A'}
function test(id:string,site=0,count=5,override:Partial<LatencyTest>={}):LatencyTest{
 const target=latencyTargets[site].urls[0]
 return {...node,attempt_id:id,display_name:'同名香港',node_type:'vless',source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,network_path:{...path},target,requested_at:at,finished_at:at,status:'completed',latency_ms:30,total_samples:count,success_samples:count,failure_samples:0,persistence_state:'saved',samples:Array.from({length:count},(_,i)=>({seq:i+1,target,timestamp:at,latency_ms:(i+1)*10,success:true})),...override}
}
function evidence(tests:LatencyTest[]=[],monitor:MonitorSample[]=[],override:Partial<QualityEvidence>={}):QualityEvidence{return {tests,monitor,downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since,until,...override}}
const site=(e:QualityEvidence,index=0,n=node)=>multiSiteQuality(n,e)[index]

describe('six-site evidence without aggregate scoring',()=>{
 it('keeps independent 4/5/19/20 sample gates and hand-calculated nearest-rank P95',()=>{
  const summaries=multiSiteQuality(node,evidence([test('four',0,4),test('five',1,5),test('nineteen',2,19),test('twenty',3,20)]))
  expect(summaries.map(s=>s.targetId)).toEqual(['cloudflare','google','github','apple','microsoft','firefox'])
  expect(summaries.map(s=>s.success)).toEqual([4,5,19,20,0,0])
  expect(summaries.map(s=>s.rankable.p50)).toEqual([false,true,true,true,false,false])
  expect(summaries.map(s=>s.rankable.p95)).toEqual([false,false,false,true,false,false])
  expect(summaries.slice(0,4).map(s=>[s.p50,s.p95])).toEqual([[25,40],[30,50],[100,190],[105,190]])
  expect(summaries[0].reasons).toContain('insufficient_samples')
  expect(summaries[4].state).toBe('unmeasured')
 })
 it('returns six unknown/unmeasured sites for no records, never all-site failure',()=>{
  const summaries=multiSiteQuality(node,evidence())
  expect(summaries).toHaveLength(6)
  expect(summaries.every(s=>s.state==='unmeasured'&&s.p50===null&&s.successRate===null&&!s.rankable.rate)).toBe(true)
 })
 it('counts real partial success, cancellation and nonexecution independently; HTTP 429 remains a first-byte response',()=>{
  const t=test('mixed',0,8)
  t.samples[4]={...t.samples[4],success:false,error:'dial timeout'}
  t.samples[5]={...t.samples[5],success:false,error:'context canceled'}
  t.samples[6]={...t.samples[6],success:false,error:'not executed: local physical path not confirmed'}
  t.samples[7]={...t.samples[7],error:'HTTP 429: source rate limited'}
  const s=site(evidence([t]))
  expect([s.total,s.success,s.excluded]).toEqual([6,5,2]);expect(s.successRate).toBeCloseTo(83.3333333333)
  expect(s.observations).toMatchObject({failed:1,cancelled:1,notExecuted:1,sourceLimited:1})
  expect(s.state).toBe('partial')
  expect(s.gates.response).toBe(false)
  expect(multiSiteQuality(node,evidence([t]))[1].state).toBe('unmeasured')
 })
 it('retains attempt-level cancelled/nonexecuted with no fabricated zero samples',()=>{
  const summaries=multiSiteQuality(node,evidence([test('cancel',0,0,{status:'cancelled'}),test('notrun',1,0,{status:'not_executed'})]))
  expect(summaries[0].state).toBe('cancelled');expect(summaries[1].state).toBe('not_executed')
  expect(summaries[0].total).toBe(0);expect(summaries[1].p50).toBeNull()
 })
 it('blocks missing pages and read errors while keeping available partial numbers visible',()=>{
  const e=evidence([test('page')]);e.reads.latency={complete:false,error:'missing page'};e.complete=false
  const s=site(e)
  expect(s.p50).toBe(30);expect(s.readComplete).toBe(false)
  expect(s.readError).toBe('missing page');expect(s.reasons).toContain('incomplete_read');expect(s.rankable.p50).toBe(false)
 })
 it('does not let unrelated failed download/monitor reads invalidate complete current HTTP evidence',()=>{
  const e=evidence([test('http')]);e.reads.monitor={complete:false};e.reads.download={complete:false};e.complete=false
  expect(site(e).rankable.p50).toBe(true)
 })
 it('marks stale groups and enforces the exact 24-hour freshness boundary',()=>{
  const stale=test('stale',0,5);stale.samples=stale.samples.map(s=>({...s,timestamp:'2026-10-09T23:59:59Z'}))
  expect(site(evidence([stale])).reasons).toContain('stale');expect(site(evidence([stale])).rankable.p50).toBe(false)
  stale.samples=stale.samples.map(s=>({...s,timestamp:'2026-10-10T00:00:00Z'}))
  expect(site(evidence([stale])).rankable.p50).toBe(true)
 })
 it('enforces a complete explicit window and filters individual observations at its boundaries',()=>{
  const t=test('window',0,7);t.samples[0].timestamp='2026-10-08T23:59:59Z';t.samples[1].timestamp='2026-10-11T00:00:01Z'
  const s=site(evidence([t]));expect(s.total).toBe(5);expect(s.p50).toBe(50)
  expect(site(evidence([t],[],{since:''})).reasons).toContain('invalid_window')
  expect(site(evidence([t],[],{since:until,until:since})).rankable.p50).toBe(false)
 })
 it('does not place undated samples into a known window or rank around them',()=>{
  const t=test('undated',0,6);t.samples[0].timestamp='0001-01-01T00:00:00Z'
  const s=site(evidence([t]));expect(s.success).toBe(5);expect(s.unplacedSamples).toBe(1)
  expect(s.reasons).toContain('unknown_observation_time');expect(s.rankable.p50).toBe(false)
 })
 it('uses the latest condition group even if insufficient; never backfills from older prettier conditions',()=>{
  const old=test('older',0,20),latest=test('latest',0,4,{network_path:{...path,address_family:'IPv6',address_source:'AAAA'}})
  old.samples=old.samples.map(s=>({...s,timestamp:'2026-10-10T01:00:00Z'}))
  const s=site(evidence([old,latest]));expect(s.groups).toHaveLength(2);expect(s.success).toBe(4)
  expect(s.group?.condition).toContain('IPv6');expect(s.rankable.p50).toBe(false)
 })
 it('cannot refresh old success with a newer cancellation in the same group',()=>{
  const old=test('old',0,5);old.samples=old.samples.map(s=>({...s,timestamp:'2026-10-09T01:00:00Z'}))
  const cancelled=test('cancelled',0,1);cancelled.samples[0]={...cancelled.samples[0],success:false,error:'context canceled'}
  const s=site(evidence([old,cancelled]));expect(s.lastAt).toBe('2026-10-09T01:00:00Z');expect(s.gates.fresh).toBe(false)
 })
 it('separates stable identities/revisions across airports even when display names and node keys coincide',()=>{
  const other={...node,profile_id:'airport-b/sub',node_identity_key:'identity-b',config_revision_key:'rev-b'}
  const a=test('same-id',0,5),b=test('same-id',0,20,{...other,samples:test('other',0,20).samples.map(s=>({...s,latency_ms:1}))})
  expect(site(evidence([a,b])).success).toBe(5)
  expect(site(evidence([a,b]),0,other).success).toBe(20)
  expect(site(evidence([test('revision',0,20,{config_revision_key:'old'})])).state).toBe('unmeasured')
  expect(site(evidence([a]),0,{...node,node_identity_key:''}).rankable.p50).toBe(false)
 })
 it('never equates recent legacy Monitor RTT with older HTTP first-byte evidence',()=>{
  const legacy:MonitorSample={...node,sample_id:'legacy',run_id:'run',display_name_snapshot:'同名香港',timestamp:until,target:latencyTargets[0].urls[0],probe_type:'rtt',success:true,latency:1e6,ttfb:0,sampling_tier:'fast'}
  const s=site(evidence([test('http')],[legacy]));expect(s.group?.source).toBe('workbench');expect(s.p50).toBe(30)
  const only=site(evidence([],[legacy]));expect(only.p50).toBe(1);expect(only.reasons).toContain('unknown_conditions');expect(only.rankable.p50).toBe(false)
 })
 it('blocks known paths with unknown node type/method/family, without inventing conditions',()=>{
  for(const override of [{source:'unknown'},{node_type:''},{method:'unknown'},{method:''},{network_path:{...path,address_family:'unknown'}},{network_path:{...path,socket_bind_verified:false}}]){
   const s=site(evidence([test('unknown',0,20,override)]));expect(s.rankable.p95).toBe(false);expect(s.reasons).toContain('unknown_conditions')
  }
 })
 it('keeps missing node types visible but out of comparable quality groups',()=>{
  const groups=qualityGroups([test('missing-type',0,20,{node_type:''})],[],'cloudflare')
  expect(groups).toHaveLength(1)
  expect(groups[0]).toMatchObject({comparable:false,total:20,success:20,p50:105,p95:190})
 })
 it('applies the same known-condition requirements to quick comparison',()=>{
  for(const override of [{source:'unknown'},{method:'unknown'},{method_version:0},{network_path:{...path,interface:'unknown'}}]){
   const groups=qualityGroups([test('quick-unknown',0,20,override)],[],'cloudflare')
   expect(groups[0].comparable).toBe(false)
  }
 })
 it('rejects simultaneous different conditions instead of choosing a hidden lexical winner',()=>{
  const s=site(evidence([test('a'),test('b',0,20,{network_path:{...path,address_family:'IPv6',address_source:'AAAA'}})]))
  expect(s.groups).toHaveLength(2);expect(s.reasons).toContain('ambiguous_latest_group');expect(s.reasons).not.toContain('unknown_conditions');expect(s.rankable.p50).toBe(false)
 })
 it('keeps the P50/P95 values but blocks tied latest condition groups from comparison',()=>{
  const groups=qualityGroups([
   test('same-time-ipv4',0,20),
   test('same-time-ipv6',0,20,{network_path:{...path,address_family:'IPv6',address_source:'AAAA'}}),
  ],[],'cloudflare')
  expect(groups).toHaveLength(2)
  expect(groups.map(group=>group.comparable)).toEqual([false,false])
  expect(groups.map(group=>[group.total,group.success,group.p50,group.p95])).toEqual([[20,20,105,190],[20,20,105,190]])
 })
 it('retains successful samples from a cancelled attempt while marking partial execution',()=>{
  const s=site(evidence([test('partial-cancel',0,5,{status:'cancelled'})]))
  expect(s.success).toBe(5);expect(s.state).toBe('partial');expect(s.attemptStates.cancelled).toBe(1)
 })
 it('does not convert invalid successful measurements into failure rates or ranking evidence',()=>{
  const t=test('invalid',0,6);t.samples[0].latency_ms=NaN
  const s=site(evidence([t]));expect(s.total).toBe(5);expect(s.successRate).toBe(100)
  expect(s.invalidSamples).toBe(1);expect(s.reasons).toContain('invalid_samples');expect(s.rankable.p50).toBe(false)
 })
 it('keeps different raw target URLs within one attempt in separate groups',()=>{
  const t=test('two-urls',0,10)
  t.samples=t.samples.map((s,i)=>i<5?s:{...s,target:'https://cp.cloudflare.com/generate_204',timestamp:until,error:i===5?'HTTP 429':''})
  const s=site(evidence([t]));expect(s.groups).toHaveLength(2);expect(s.group?.target).toBe('https://cp.cloudflare.com/generate_204')
  expect(s.success).toBe(5);expect(s.p50).toBe(80);expect(s.observations.sourceLimited).toBe(1)
 })
 it.each(['cancelled','not_executed'])('does not rank older path evidence after a newer empty %s attempt',status=>{
  const old=test('old-path',0,20);old.samples=old.samples.map(s=>({...s,timestamp:'2026-10-10T22:00:00Z'}))
  const latest=test('new-path',0,0,{status,network_path:{...path,interface:'en2',address_family:'IPv6',address_source:'AAAA'}})
  const e=evidence([old,latest]);e.reads.monitor={complete:false,error:'unrelated legacy read'};e.complete=false
  const s=site(e);expect(s.groups[0].success).toBe(20);expect(s.readComplete).toBe(true);expect(s.readError).toBeUndefined()
  expect(s.group).toBeUndefined();expect(s.state).toBe(status);expect(s.p50).toBeNull()
  expect(s.reasons).toContain('latest_unobserved_attempt')
  expect(s.rankable).toEqual({observed:false,p50:false,p95:false,rate:false})
 })
 it('treats contradictory copies of one observation as unknown in either input order',()=>{
  const a=test('same-id',0,20),b=test('same-id',0,20,{network_path:{...path,interface:'en2',address_family:'IPv6',address_source:'AAAA'}})
  for(const tests of [[a,b],[b,a]]){
   const s=site(evidence(tests));expect(s.conflictingSamples).toBe(20)
   expect(s.reasons).toContain('conflicting_observations');expect(s.group).toBeUndefined()
   expect(s.rankable).toEqual({observed:false,p50:false,p95:false,rate:false});expect(s.p50).toBeNull()
  }
 })
 it('does not block a measured site with an older empty attempt or another site cancellation',()=>{
  const oldEmpty=test('older-empty',0,0,{status:'not_executed',requested_at:'2026-10-10T21:00:00Z',finished_at:'2026-10-10T21:00:00Z'})
  const measured=test('measured',0,20),otherSite=test('other-empty',1,0,{status:'cancelled'})
  const summaries=multiSiteQuality(node,evidence([oldEmpty,measured,otherSite]))
  expect(summaries[0].rankable.p95).toBe(true);expect(summaries[0].success).toBe(20)
  expect(summaries[1].state).toBe('cancelled');expect(summaries[1].rankable.p50).toBe(false)
 })
 it('rejects contradictory numeric values even when the condition key agrees',()=>{
  const a=test('same-values',0,20),b={...a,samples:a.samples.map(s=>({...s,latency_ms:s.seq===1?1:s.latency_ms}))}
  for(const tests of [[a,b],[b,a]]){
   const s=site(evidence(tests));expect(s.success).toBe(19);expect(s.conflictingSamples).toBe(1)
   expect(s.reasons).toContain('conflicting_observations');expect(s.rankable.p50).toBe(false)
  }
 })
 it('keeps legacy stream conflicts diagnostic without blocking complete current HTTP',()=>{
  const legacy:MonitorSample={...node,sample_id:'legacy-conflict',run_id:'run',display_name_snapshot:'same',timestamp:at,target:latencyTargets[0].urls[0],probe_type:'rtt',success:true,latency:1e6,ttfb:0,sampling_tier:'fast'}
  const conflicting={...legacy,latency:2e6}
  for(const monitor of [[legacy,conflicting],[conflicting,legacy]]){
   const s=site(evidence([test('http',0,20)],monitor))
   expect(s.conflictingSamples).toBe(1);expect(s.success).toBe(20)
   expect(s.reasons).not.toContain('conflicting_observations')
   expect(s.rankable).toEqual({observed:true,p50:true,p95:true,rate:true})
   expect(site(evidence([],monitor)).rankable.p50).toBe(false)
  }
 })
 it('deduplicates repeated pages without mutating saved input or executing any API',()=>{
  const t=test('duplicate'),e=evidence([t,t]);const snapshot=JSON.stringify(e),fetch=vi.fn(()=>{throw Error('network forbidden')});vi.stubGlobal('fetch',fetch)
  try{expect(site(e).success).toBe(5);expect(JSON.stringify(e)).toBe(snapshot);expect(fetch).not.toHaveBeenCalled()}finally{vi.unstubAllGlobals()}
 })
})

describe('explicit single-site pairwise comparison only',()=>{
 it('compares P50 at five but requires twenty successes on BOTH sides for P95',()=>{
  const a=site(evidence([test('a',0,19)])),b=site(evidence([test('b',0,20)]))
  expect(compareSiteQuality(a,b,'p50')).toMatchObject({comparable:true,order:-1})
  expect(compareSiteQuality(a,b,'p95')).toMatchObject({comparable:false,order:null})
  const c=site(evidence([test('c',0,20,{samples:test('c',0,20).samples.map(s=>({...s,latency_ms:1}))})]))
  expect(compareSiteQuality(b,c,'p95')).toMatchObject({comparable:true,order:1})
 })
 it.each([
  ['method',{method:'tcp_connect'}],['version',{method_version:2}],['family',{network_path:{...path,address_family:'IPv6',address_source:'AAAA'}}],['exit',{network_path:{...path,interface:'en2'}}],['URL',{target:'https://cp.cloudflare.com/generate_204',samples:test('url').samples.map(s=>({...s,target:'https://cp.cloudflare.com/generate_204'}))}],
 ] as [string,Partial<LatencyTest>][])('rejects different %s rather than ordering group keys',(_,override)=>{
  const a=site(evidence([test('a')])),b=site(evidence([test('b',0,5,override)]))
  expect(compareSiteQuality(a,b,'p50')).toMatchObject({comparable:false,order:null})
  expect(compareSiteQuality(a,b,'p50').reasons).toContain('condition_mismatch')
 })
 it('orders response rate descending only within identical recorded conditions',()=>{
  const t=test('partial',0,6);t.samples[5]={...t.samples[5],success:false,error:'timeout'}
  expect(compareSiteQuality(site(evidence([t])),site(evidence([test('full')])),'rate')).toMatchObject({comparable:true,order:1})
 })
 it('rejects different sites/windows; equivalent timezone representations remain comparable',()=>{
  const a=site(evidence([test('a')])),b=site(evidence([test('b',1)]),1)
  expect(compareSiteQuality(a,b,'p50').reasons).toContain('target_mismatch')
  const window=site(evidence([test('c')],[],{since:'2026-10-10T00:00:00Z'}))
  expect(compareSiteQuality(a,window,'p50').reasons).toContain('window_mismatch')
  const equivalent=site(evidence([test('d')],[],{since:'2026-10-09T08:00:00+08:00',until:'2026-10-11T08:00:00+08:00'}))
  expect(compareSiteQuality(a,equivalent,'p50')).toMatchObject({comparable:true,order:0})
 })
})

it('admits a single complete same-condition observation without claiming stability/P95 ranking',()=>{
 const a=site(evidence([test('one-a',0,1)])),b=site(evidence([test('one-b',0,1)]))
 expect(compareSiteQuality(a,b,'observed').comparable).toBe(true)
 expect(a.rankable.p50).toBe(false);expect(a.rankable.p95).toBe(false)
 const other=site(evidence([test('other',0,1,{network_path:{...path,address_family:'IPv6',address_source:'AAAA'}})]))
 expect(compareSiteQuality(a,other,'observed').comparable).toBe(false)
 const missing=site(evidence([test('missing',0,1)],[],{reads:{latency:{complete:false},monitor:{complete:true},service:{complete:true},download:{complete:true}}}))
 expect(compareSiteQuality(a,missing,'observed').comparable).toBe(false)
})
