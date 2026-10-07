import {describe,expect,it} from 'vitest'
import {createWorkspace} from '../workspace'
import {key,type Attempt,type LatencyTest} from '../domain'
const n={profile_id:'p',node_key:'node',node_identity_key:'nid',config_revision_key:'rev'}
const at='2026-10-01T00:00:00Z'
const a:Attempt={...n,attempt_id:'download',request_id:'request',display_name:'Node',requested_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://example.com'},result:{outcome:'byte_limit',bytes_read:10485760,duration_ns:1000000000,finished_at:at}}
const t:LatencyTest={...n,attempt_id:'latency',display_name:'Node',node_type:'ss',requested_at:at,finished_at:at,status:'completed',latency_ms:80,total_samples:1,success_samples:1,failure_samples:0,persistence_state:'saved',samples:[{seq:1,timestamp:at,success:true,latency_ms:80}]}
describe('unchanged history response reuse',()=>{
 it('preserves download references for identical reads but accepts updated result details',()=>{
  const w=createWorkspace();w.mergeAttempt('download',a);const list=w.downloads.value[key(n)],record=list[0]
  w.mergeAttempt('download',JSON.parse(JSON.stringify(a)));expect(w.downloads.value[key(n)]).toBe(list);expect(w.downloads.value[key(n)][0]).toBe(record)
  w.mergeAttempt('download',{...a,result:{...a.result!,duration_ns:2000000000,details:{phase:'new'}}});expect(w.downloads.value[key(n)]).not.toBe(list);expect(w.downloads.value[key(n)][0].result?.duration_ns).toBe(2000000000);expect(w.downloads.value[key(n)][0].result?.details).toEqual({phase:'new'})
  const updated=w.downloads.value[key(n)];w.mergeAttempt('download',{...a,execution_state:'running',persistence_state:'pending',result:undefined});expect(w.downloads.value[key(n)]).toBe(updated);w.dispose()
 })
 it('preserves identical latency reads without ignoring corrected samples and parent target',()=>{
  const w=createWorkspace();w.mergeLatency(t);const list=w.latency.value[key(n)];w.mergeLatency(JSON.parse(JSON.stringify(t)));expect(w.latency.value[key(n)]).toBe(list)
  w.mergeLatency({...t,target:'https://speed.cloudflare.com/__down?bytes=1',samples:[{...t.samples[0],latency_ms:120}]});expect(w.latency.value[key(n)]).not.toBe(list);expect(w.latency.value[key(n)][0].samples[0].latency_ms).toBe(120);expect(w.latency.value[key(n)][0].target).toContain('cloudflare');w.dispose()
 })
})
