import {describe,expect,it} from 'vitest'
import {sortSiteRows} from '../siteQualityView'
import {multiSiteQuality,type QualityEvidence} from '../nodeQuality'
import type {LatencyTest,NodeOption} from '../domain'
const nodes=[0,1,2].map(i=>({profile_id:'synthetic',node_key:'node'+i,node_identity_key:'identity'+i,config_revision_key:'rev',profile_name:'Synthetic',display_name:'Node'+i,type:'ss',country_code:'HK',country_flag:''} as NodeOption))
function summary(i:number,ms:number,family='IPv4',count=20){
 const at='2026-10-11T10:00:00Z',target='https://speed.cloudflare.com/__down?bytes=1'
 const t:LatencyTest={...nodes[i],attempt_id:'attempt'+i,display_name:'Synthetic',node_type:'ss',source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,network_path:{method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:family,address_source:family==='IPv4'?'A':'AAAA'},target,requested_at:at,finished_at:at,status:'completed',latency_ms:ms,total_samples:count,success_samples:count,failure_samples:0,persistence_state:'saved',samples:Array.from({length:count},(_,seq)=>({seq:seq+1,target,timestamp:at,latency_ms:ms,success:true}))}
 const e:QualityEvidence={tests:[t],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'2026-10-10T12:00:00Z',until:'2026-10-11T12:00:00Z'}
 return multiSiteQuality(nodes[i],e)
}
describe('six-site UI conservative comparison',()=>{
 it('sorts a whole eligible same-condition set by P50 and rate without mutating input',()=>{
  const rows=nodes.slice(0,2).map((node,i)=>({node,summary:summary(i,40-i*20)[0]}))
  expect(sortSiteRows(rows,'p50').rows.map(r=>r.node.node_key)).toEqual(['node1','node0'])
  expect(rows.map(r=>r.node.node_key)).toEqual(['node0','node1'])
  expect(sortSiteRows(rows,'rate').comparable).toBe(true)
 })
 it('keeps the entire original order for mixed conditions or insufficient P95, never null-as-tie',()=>{
  const rows=nodes.map((node,i)=>({node,summary:summary(i,30-i*10,i===2?'IPv6':'IPv4')[0]}))
  expect(sortSiteRows(rows,'p50')).toMatchObject({rows,comparable:false})
  rows[2].summary=summary(2,10,'IPv4',5)[0]
  expect(sortSiteRows(rows,'p95')).toMatchObject({rows,comparable:false})
  expect(sortSiteRows(rows,'p50').comparable).toBe(true)
 })
})
