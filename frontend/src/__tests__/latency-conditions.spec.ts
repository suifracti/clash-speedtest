import {expect,it} from 'vitest'
import {downloadCondition} from '../measurementRounds'
import {qualityGroups} from '../nodeQuality'
import {manualLatencyPoint} from '../presentation'
import {downloadGroup,latencyComparisonGroup,serviceComparisonGroup} from '../sharedResults'
import type {Attempt,LatencyTest,NetworkPath} from '../domain'

const url='https://speed.cloudflare.com/__down?bytes=1'
const route=(address_family?:string,address_source?:string):NetworkPath=>({method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family,address_source})
const latency=(id:string,network_path?:NetworkPath):LatencyTest=>({profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',attempt_id:id,display_name:'n',node_type:'vless',target:url,requested_at:'2026-10-09T00:00:00Z',finished_at:'2026-10-09T00:00:01Z',status:'completed',latency_ms:25,total_samples:1,success_samples:1,failure_samples:0,samples:[{seq:1,target:url,timestamp:'2026-10-09T00:00:01Z',latency_ms:25,success:true}],persistence_state:'saved',source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,network_path})
const attempt=(id:string,path:NetworkPath):Attempt=>({profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',attempt_id:id,request_id:id,display_name:'n',node_type:'vless',service_id:'cloudflare_204',requested_at:'2026-10-09T00:00:00Z',execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://cp.cloudflare.com/generate_204',method:'GET',rule_version:1,success_criterion:'status 204'},result:{outcome:'matched',bytes_read:0,finished_at:'2026-10-09T00:00:01Z',network_path:path,details:{execution_status:'executed'}}})

it('keeps a legacy latency value visible while excluding it from comparable ranking',()=>{
 const legacy=latency('legacy'),point=manualLatencyPoint(legacy)
 expect(point.value).toBe(25)
 const group=latencyComparisonGroup(point)
 expect(group.valid).toBe(false)
 expect(group.label).toContain('条件证据不足')
 expect(group.label).not.toContain('未测')
 expect(qualityGroups([legacy],[],'cloudflare')[0].comparable).toBe(false)
})

it('separates latency and quality conditions by recorded address family and source',()=>{
 const observations=[latency('v4-a',route('IPv4','A')),latency('v6-a',route('IPv6','AAAA')),latency('v4-aaaa',route('IPv4','AAAA'))]
 const points=observations.map(test=>manualLatencyPoint(test))
 const groups=points.map(latencyComparisonGroup)
 expect(groups.every(group=>group.valid)).toBe(true)
 expect(new Set(groups.map(group=>group.key)).size).toBe(3)
 const quality=qualityGroups(observations,[],'cloudflare')
 expect(quality).toHaveLength(3)
 expect(quality.every(group=>group.comparable)).toBe(true)
 expect(new Set(quality.map(group=>group.key)).size).toBe(3)
 const unknown=latency('unknown-address-fields',route())
 expect(latencyComparisonGroup(manualLatencyPoint(unknown)).valid).toBe(false)
 expect(qualityGroups([unknown],[],'cloudflare')[0].comparable).toBe(false)
 expect(qualityGroups([unknown],[],'cloudflare')[0].condition).toContain('unknown')
})

it('includes recorded address family and source in download and service comparison conditions',()=>{
 const ipv4=route('IPv4','A'),ipv6=route('IPv6','AAAA'),sameFamilyOtherSource=route('IPv4','AAAA')
 const download=(path:NetworkPath)=>({rule:{target_url:url,method:'GET',rule_version:1,maximum_bytes:1048576,maximum_duration_ns:1000000000,sample_every_bytes:65536,sample_every_ns:100000000,network_path_method:path.method,physical_interface:path.interface,dns_mode:path.dns_mode},result:{network_path:path}} as unknown as Attempt)
 const downloads=[ipv4,ipv6,sameFamilyOtherSource].map(path=>download(path))
 expect(new Set(downloads.map(downloadCondition)).size).toBe(3)
 const changedMeasuredPath:Attempt={...downloads[0],result:{...downloads[0].result!,network_path:{...ipv4,method:'another-path'}}}
 expect(downloadCondition(downloads[0])).not.toBe(downloadCondition(changedMeasuredPath))
 const services=[attempt('v4-a',ipv4),attempt('v6-a',ipv6),attempt('v4-aaaa',sameFamilyOtherSource)]
 const keys=services.map(service=>serviceComparisonGroup([service],['cloudflare_204']).key)
 expect(new Set(keys).size).toBe(3)
})

it('preserves download values but excludes unknown route evidence from numeric ranking',()=>{
 const download=(path:NetworkPath):Attempt=>({...attempt('download',path),rule:{target_url:url,method:'GET',rule_version:1,maximum_bytes:1048576,maximum_duration_ns:1000000000,network_path_method:path.method,physical_interface:path.interface,dns_mode:path.dns_mode},result:{outcome:'byte_limit',bytes_read:1048576,duration_ns:1000000000,finished_at:'2026-10-09T00:00:01Z',network_path:path}})
 expect(downloadGroup(download(route('IPv4','A'))).comparable).toBe(true)
 expect(downloadGroup(download(route())).comparable).toBe(false)
 expect(downloadGroup(download({...route('IPv4','A'),method:''})).comparable).toBe(false)
})
