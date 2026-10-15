import {expect,it} from 'vitest'
import {manualLatencyPoint,latencyPointLabel} from '../presentation'
import {qualityGroups} from '../nodeQuality'
import type {LatencyTest} from '../domain'

it('shows a local physical binding rejection as unexecuted history, never a failed or zero-latency node',()=>{
 const target='https://speed.cloudflare.com/__down?bytes=1',at='2026-10-13T08:00:00Z'
 const test:LatencyTest={profile_id:'fixture',node_key:'n',node_identity_key:'i',config_revision_key:'r',attempt_id:'local-binding',display_name:'Fixture',node_type:'ss',requested_at:at,finished_at:at,status:'not_executed',persistence_state:'saved',latency_ms:0,total_samples:0,success_samples:0,failure_samples:0,method:'http_get_via_proxy_first_byte',method_version:2,target,samples:[{seq:1,target,timestamp:at,success:false,latency_ms:0,error:'未执行：物理出口约束未核实'}]}
 const point=manualLatencyPoint(test,'cloudflare')
 expect(point.value).toBe(null);expect(point.tone).toBe('empty');expect(latencyPointLabel(point,'cloudflare')).toBe('未执行')
 const groups=qualityGroups([test],[],'cloudflare')
 expect(groups[0].total).toBe(0);expect(groups[0].excluded).toBe(1);expect(groups[0].successRate).toBe(null)
 const failed=manualLatencyPoint({...test,status:'failed',total_samples:1,failure_samples:1,samples:[{...test.samples[0],error:'EOF'}]},'cloudflare')
 expect(failed.tone).toBe('bad');expect(latencyPointLabel(failed)).toBe('失败')
})

it('does not count saved IP-version preparation errors as node failures or rewrite their raw records',()=>{
 const at='2026-10-15T08:33:30Z',urls=['https://speed.cloudflare.com/__down?bytes=1','https://www.gstatic.com/generate_204','https://api.github.com/zen','https://captive.apple.com/hotspot-detect.html','http://www.msftconnecttest.com/connecttest.txt','https://detectportal.firefox.com/success.txt']
 const saved:LatencyTest={profile_id:'fixture',node_key:'n',node_identity_key:'i',config_revision_key:'r',attempt_id:'saved-ip-version',display_name:'Fixture',node_type:'Hysteria2',requested_at:at,finished_at:at,status:'failed',persistence_state:'saved',latency_ms:0,total_samples:36,success_samples:0,failure_samples:36,method:'http_get_via_proxy_first_byte',method_version:2,target:'multi://latency-v1',network_path:{method:'physical_socket_v1',interface:'en1',address_family:'ipv6',address_source:'physical_ipv6_dns',dns_mode:'physical_interface_dns_v1',dns_bind_verified:true,socket_bind_verified:false,tcp_bindings:0,udp_bindings:0},samples:urls.flatMap((target,i)=>Array.from({length:6},(_,j)=>({seq:i*6+j+1,target,timestamp:at,success:false,latency_ms:0,error:`Get "${target}": ip version error`})))}
 const raw=JSON.stringify(saved),point=manualLatencyPoint(saved,'cloudflare'),group=qualityGroups([saved],[],'cloudflare')[0]
 expect(point.tone).toBe('empty');expect(latencyPointLabel(point,'cloudflare')).toBe('未执行')
 expect(group.total).toBe(0);expect(group.excluded).toBe(6);expect(group.successRate).toBe(null)
 expect(JSON.stringify(saved)).toBe(raw)
 const realFailure={...saved,attempt_id:'real-timeout',samples:[{...saved.samples[0],error:'context deadline exceeded'}]}
 expect(manualLatencyPoint(realFailure,'cloudflare').tone).toBe('bad')
 expect(qualityGroups([realFailure],[],'cloudflare')[0].total).toBe(1)
})
