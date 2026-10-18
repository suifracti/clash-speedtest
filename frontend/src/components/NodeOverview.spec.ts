import {mount} from '@vue/test-utils'
import {expect,it} from 'vitest'
import NodeOverview from './NodeOverview.vue'
import {multiSiteQuality,type QualityEvidence} from '../nodeQuality'
import type {Attempt,NodeOption} from '../domain'
const node:NodeOption={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',profile_name:'fixture',display_name:'fixture',type:'ss',country_code:'SG',country_flag:''}
const at='2026-10-18T08:00:00Z'
const attempt=(id:string,outcome:string,time=at):Attempt=>({...node,attempt_id:id+time,request_id:id,service_id:id,requested_at:time,finished_at:time,execution_state:'completed',persistence_state:'saved',rule:{service_id:id,target_url:'https://example.test'},result:{outcome,bytes_read:0,finished_at:time}})
it('keeps the latest service failure and latest download status visible beside clearly historical speed',()=>{
 const old=attempt('netflix_unlock','matched','2026-10-18T07:00:00Z'),latest=attempt('netflix_unlock','challenge')
 const valid=attempt('download','byte_limit','2026-10-18T07:00:00Z');valid.result={...valid.result!,bytes_read:1048576,duration_ns:1e9};const failed=attempt('download','failed')
 const evidence:QualityEvidence={tests:[],monitor:[],services:[old,latest],downloads:[valid,failed],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'2026-10-18T00:00:00Z',until:at}
 const w=mount(NodeOverview,{props:{evidence,summaries:multiSiteQuality(node,evidence),target:'cloudflare',targetIds:['cloudflare'],serviceIds:['netflix_unlock'],catalog:[{service_id:'netflix_unlock',name:'Netflix 片目与地区检查',category:'test',description:'',result_kind:'unlock',success_criterion:'fixture',timeout_seconds:10}]}})
 expect(w.get('.overview-services').text()).toContain('验证挑战');expect(w.get('.overview-services').text()).not.toContain('规则通过')
 expect(w.get('.overview-download').text()).toContain('失败');expect(w.get('.overview-download').text()).toContain('历史有效 1.00 MiB/s')
 expect(w.get('.overview-latency').text()).toContain('未测');w.unmount()
})
