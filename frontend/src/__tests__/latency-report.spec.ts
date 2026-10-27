import {describe,it,expect} from 'vitest'
import {manualLatencyPoint} from '../presentation'
import {latencyVerdict} from '../components/home/homeModel'
import {activeLatencyTargets} from '../latencyTargets'
import {buildLatencyReport,compareLatencyReports} from '../latencyReport'
import type {LatencyBatch,BatchItem} from '../domain'
const item=(id:string,ms:number,success=true):BatchItem=>({item_id:id,batch_id:'b',profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',display_name:id,execution_state:'completed',persistence_state:'saved',result:{profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',attempt_id:id,display_name:id,node_type:'Vless',requested_at:'2026-10-21T11:00:00Z',finished_at:'2026-10-21T11:01:00Z',status:'completed',latency_ms:ms,total_samples:24,success_samples:success?24:0,failure_samples:success?0:24,persistence_state:'saved',samples:['https://speed.cloudflare.com/__down?bytes=1','https://www.gstatic.com/generate_204','https://api.github.com/zen','https://captive.apple.com/hotspot-detect.html'].flatMap(target=>Array.from({length:6},(_,i)=>({seq:i+1,target,timestamp:'2026-10-21T11:00:00Z',latency_ms:i?ms:9000,success,error:success?'':'context deadline exceeded'})))}})
const batch=(items:BatchItem[]):LatencyBatch=>({batch_id:'b',request_id:'b',requested_at:'2026-10-21T11:00:00Z',state:'completed',target_id:'all',timeout_seconds:5,sample_count:6,item_count:items.length,items})
describe('latency run report',()=>{
 it('uses stable medians, sorts fastest and compares the same nodes',()=>{const b=batch([item('fast',300),item('slow',3000)]),r=buildLatencyReport(b);expect(r.counts).toMatchObject({usable:1,slow:1,suspected:1});expect(r.fastest5[0].stableMs).toBe(300);const old={...batch([item('fast',500),item('slow',3100)]),batch_id:'old',requested_at:'2026-10-20T11:00:00Z'};expect(compareLatencyReports(b,r,[old])).toMatchObject({comparableNodes:2,stableMedianDeltaMs:-150})})
 it('keeps an unexecuted configuration out of no-response counts',()=>{const skipped={...item('skip',0),result:undefined,error_message:'配置冲突'},r=buildLatencyReport(batch([item('failed',0,false),skipped]));expect(r.counts).toMatchObject({noResponse:1,skipped:1});expect(r.groups.find(g=>g.kind==='no-response')?.reason).toBe('连接超时')})
 it('marks low-response nodes unstable and sorts them after usable ones',()=>{const low=item('low',3000);low.result!.samples=low.result!.samples.slice(0,6).map((s,i)=>({...s,success:i<2}));low.result!.total_samples=6;low.result!.success_samples=2;const r=buildLatencyReport(batch([low,item('fast',500),item('failed',0,false)]));expect(r.rows.map(row=>row.status)).toEqual(['usable','unstable','no-response']);expect(r.counts).toMatchObject({slow:0,unstable:1});expect(r.groups.find(g=>g.kind==='unstable')?.reason).toBe('不稳定')})
 it('groups a completely blocked site separately from instability',()=>{
  const node=item('partial',300)
  node.result!.samples=node.result!.samples.map(s=>s.target?.includes('gstatic')?{...s,success:false,error:'EOF'}:s)
  node.result!.success_samples=18
  const r=buildLatencyReport(batch([node]))
  expect(r.rows[0]).toMatchObject({status:'partial',partialSites:['Google']})
  expect(r.counts).toMatchObject({unstable:0,partial:1})
  expect(r.groups.find(g=>g.kind==='partial')?.reason).toBe('部分站点不通')
  expect(latencyVerdict(activeLatencyTargets.map(t=>manualLatencyPoint(node.result!,t.id))).label).toBe('部分站点不通（Google）')
 })

})
