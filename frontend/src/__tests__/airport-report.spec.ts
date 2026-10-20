import {expect,it} from 'vitest'
import {airportReport} from '../airportReport'
import {key,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
it('summarizes observed node rounds and leaves missing history unknown',()=>{
 const node={profile_id:'p',node_identity_key:'i',config_revision_key:'r',node_key:'n'} as NodeOption,at='2026-10-20T10:00:00Z'
 const evidence={tests:[{attempt_id:'good',samples:[{seq:1,target:'cloudflare',timestamp:at,success:true,latency_ms:10}]},{attempt_id:'bad',samples:[{seq:1,target:'cloudflare',timestamp:at,success:false,latency_ms:0,error:'timeout'}]}],monitor:[],reads:{latency:{complete:true},monitor:{complete:true}}} as unknown as QualityEvidence
 const report=airportReport([node],{[key(node)]:evidence});expect(report.rate).toBe(50);expect(report.total).toBe(2);expect(report.failures).toEqual([['超时',1]]);expect(report.trend[0].value).toBe(10)
 const missing=airportReport([node],{});expect(missing.rate).toBeNull();expect(missing.incomplete).toBe(true);expect(missing.trend).toEqual([])
})
