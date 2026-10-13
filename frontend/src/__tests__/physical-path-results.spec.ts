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
