import {describe,expect,it} from 'vitest'
import {ref} from 'vue'
import * as presentation from '../presentation'
import {key,type Attempt,type LatencyTest,type MeasurementRound,type MonitorSample,type NodeOption,type ServiceRule} from '../domain'

const a:NodeOption={profile_id:'s',node_key:'a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'机场',display_name:'A',type:'ss',country_code:'CA',country_flag:''}
const b:NodeOption={...a,node_key:'b',node_identity_key:'identity-b',display_name:'B'}
const at='2026-10-01T00:00:00Z'
function latency(id='test',value=80):LatencyTest{return {...a,attempt_id:id,node_type:'ss',requested_at:at,finished_at:at,status:'completed',latency_ms:value,total_samples:2,success_samples:2,failure_samples:0,persistence_state:'saved',samples:[{seq:1,timestamp:at,target:'https://speed.cloudflare.com/__down?bytes=1',success:true,latency_ms:value},{seq:2,timestamp:at,target:'https://www.gstatic.com/generate_204',success:true,latency_ms:900}]}}
function download(id='download'):Attempt{return {...a,attempt_id:id,request_id:id,display_name:'A',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://speed.cloudflare.com/__down',maximum_bytes:10485760},result:{outcome:'byte_limit',bytes_read:10485760,duration_ns:1000000000,finished_at:at}}}
function fixture(){return {displayTarget:ref('cloudflare'),serviceIds:ref(['one']),latency:ref<Record<string,LatencyTest[]>>({[key(a)]:[latency()]}),downloads:ref<Record<string,Attempt[]>>({[key(a)]:[download()]}),services:ref<Record<string,Attempt[]>>({}),measurementRounds:ref<Record<string,MeasurementRound[]>>({}),catalog:ref<ServiceRule[]>([])} }
function cache(w=fixture(),monitor=ref<Record<string,MonitorSample[]>>({}),http=ref<Record<string,MonitorSample[]>>({})){return presentation.createPresentationCache(w,monitor,http)}

describe('bounded reactive presentation cache',()=>{
 it('reuses unchanged point references across hover renders and keeps site queries distinct',()=>{
  const w=fixture(),c=cache(w);c.retain([a]);const cloudflare=c.site(a,'cloudflare'),google=c.site(a,'google')
  expect(cloudflare[0].value).toBe(80);expect(google[0].value).toBe(900)
  for(let i=0;i<20;i++){expect(c.site(a,'cloudflare')).toBe(cloudflare);expect(c.site(a,'google')).toBe(google)}
  c.clear()
 })
 it('updates a nested result and older-page additions without borrowing another node or revision',()=>{
  const w=fixture(),c=cache(w);c.retain([a,b]);const first=c.points(a,'download');expect(first[0].value).toBe(10)
  w.downloads.value[key(b)]=[{...download('b'),...b,result:{...download().result!,bytes_read:5242880}}]
  expect(c.points(a,'download')).toBe(first);expect(c.points(b,'download')[0].value).toBe(5)
  w.downloads.value[key(a)][0].result!.duration_ns=2000000000
  expect(c.points(a,'download')[0].value).toBe(5);expect(c.points(b,'download')[0].value).toBe(5)
  w.downloads.value[key(a)].push({...download('older'),requested_at:'2026-09-30T23:00:00Z',finished_at:'2026-09-30T23:00:00Z'})
  expect(c.points(a,'download').map(p=>p.id)).toEqual(['older','download'])
  const revision={...a,config_revision_key:'next'};expect(c.points(revision,'download')).toEqual([])
  w.downloads.value={};expect(c.points(a,'download')).toEqual([]);c.clear()
 })
 it('uses a newer explicitly linked result from history while preserving one round and its origin',()=>{
  const w=fixture(),c=cache(w);c.retain([a]);const embedded=download()
  w.measurementRounds.value[key(a)]=[{round_id:'round',trigger_type:'scheduled',started_at:at,state:'finished',items:[{...a,request_id:embedded.request_id,project:'download',execution_state:'completed',persistence_state:'saved',download:embedded}]}]
  expect(c.points(a,'download').map(p=>p.id)).toEqual(['round']);expect(c.points(a,'download')[0].value).toBe(10)
  w.downloads.value[key(a)]=[{...embedded,result:{...embedded.result!,duration_ns:2000000000}}]
  expect(c.points(a,'download')[0].value).toBe(5);expect(c.points(a,'download')[0].trigger).toBe('scheduled');expect(c.points(a,'download')).toHaveLength(1)
  expect(w.measurementRounds.value[key(a)][0].items[0].download?.result?.duration_ns).toBe(1000000000)
  c.clear()
 })
 it('refreshes round saving failure and newly loaded service catalog labels',()=>{
  const w=fixture(),c=cache(w);c.retain([a]);w.measurementRounds.value[key(a)]=[{round_id:'round',trigger_type:'scheduled',started_at:at,state:'running',items:[{...a,request_id:'one',project:'service',service_id:'one',execution_state:'queued',persistence_state:'pending'}]}]
  w.catalog.value=[{service_id:'one',name:'服务一',category:'',description:'',result_kind:'',success_criterion:'',timeout_seconds:10}]
  expect(c.points(a,'service','cloudflare','one')[0].samples?.[0].label).toBe('服务一')
  w.catalog.value[0].name='更新名称';expect(c.points(a,'service','cloudflare','one')[0].samples?.[0].label).toBe('更新名称')
  const item=w.measurementRounds.value[key(a)][0].items[0];item.service={...download('one'),service_id:'one',rule:{name:'服务一',target_url:'https://example.com'},result:{outcome:'reachable',bytes_read:0,finished_at:at}};item.execution_state='completed';item.persistence_state='saved'
  expect(c.points(a,'service','cloudflare','one')[0].value).toBe(1)
  item.service.persistence_state='failed';item.persistence_state='failed'
  expect(c.points(a,'service','cloudflare','one')[0].tone).toBe('warn');expect(c.points(a,'service','cloudflare','one')[0].description).toContain('保存失败 1')
  expect(c.points(a,'service','cloudflare','missing')).toEqual([]);c.clear()
 })
 it('refreshes monitor replacements and full metadata while retaining unchanged download projections',()=>{
  const w=fixture(),m=ref<Record<string,MonitorSample[]>>({}),h=ref<Record<string,MonitorSample[]>>({}),c=cache(w,m,h);c.retain([a]);const prior=c.points(a,'download')
  expect(c.monitor(a)).toEqual([])
  const sample:MonitorSample={...a,sample_id:'sample',run_id:'run',display_name_snapshot:'A',timestamp:at,target:'https://cp.cloudflare.com/generate_204',probe_type:'rtt',success:true,latency:80000000,ttfb:0,sampling_tier:'basic',trigger_type:'scheduled'}
  m.value[key(a)]=[sample];expect(c.site(a,'cloudflare').at(-1)?.value).toBe(80)
  m.value[key(a)][0].success=false;m.value[key(a)][0].error_detail='updated timeout'
  expect(c.monitor(a)[0].tone).toBe('bad');expect(c.monitor(a)[0].samples?.[0].description).toContain('updated timeout')
  h.value[key(a)]=[{...sample,success:true,probe_type:'service_google',target:'https://www.google.com/generate_204'}];expect(c.http(a,'monitor_google_http')[0].tone).toBe('good');expect(c.http(a,'monitor_github_http')).toEqual([])
  expect(c.points(a,'download')).toBe(prior);c.clear()
 })
 it('does not reuse mutated selection arrays when returning to a previous service filter',()=>{
  const w=fixture(),c=cache(w);c.retain([a]);w.services.value[key(a)]=[{...download('one'),service_id:'one'},{...download('two'),service_id:'two'}]
  expect(c.services(a).flatMap(p=>p.attempts!.map(t=>t.service_id))).toEqual(['one'])
  w.serviceIds.value.splice(0,1,'two');expect(c.services(a).flatMap(p=>p.attempts!.map(t=>t.service_id))).toEqual(['two'])
  w.serviceIds.value=['one'];expect(c.services(a).flatMap(p=>p.attempts!.map(t=>t.service_id))).toEqual(['one']);c.clear()
 })
 it('evicts presentations when a page or filter removes the node, and releases them on clear',()=>{
  const w=fixture(),c=cache(w);c.retain([a]);const prior=c.points(a,'download');c.retain([b]);c.retain([a]);expect(c.points(a,'download')).not.toBe(prior)
  const next=c.points(a,'download');c.clear();c.retain([a]);expect(c.points(a,'download')).not.toBe(next)
  for(let i=0;i<60;i++)c.points(a,'service','cloudflare','service-'+i)
  const evicted=c.points(a,'download');expect(evicted).not.toBe(next);expect(evicted[0].value).toBe(10);c.clear()
 })
})
