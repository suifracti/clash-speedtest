import {expect,it,vi} from 'vitest'
import {mount} from '@vue/test-utils'
import {createWorkspace,workspaceKey} from '../workspace'
import QueueModal from '../components/QueueModal.vue'
import {downloadGroup,servicePassed,serviceExecuted,serviceComparisonGroup} from '../sharedResults'
import {durableRoundPoints,serviceConclusion,attemptTone} from '../measurementRounds'
import {downloadSpeed,type Attempt,type NodeOption} from '../domain'
const n:NodeOption={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',profile_name:'p',display_name:'n',type:'ss',country_code:'',country_flag:''}
const a:Attempt={...n,attempt_id:'a',request_id:'q',requested_at:'2026-10-09T00:00:00Z',execution_state:'completed',persistence_state:'failed',rule:{target_url:'https://example.test/data',method:'GET',maximum_bytes:10485760},result:{outcome:'time_limit',bytes_read:5242880,duration_ns:1e9,finished_at:'2026-10-09T00:00:01Z'}}
it('retains staged effective partial speed and separates it from full and invalid measurements',()=>{
 expect(downloadSpeed(a)).toBe(5)
 expect(downloadGroup(a).kind).toBe('partial')
 expect(downloadGroup({...a,result:{...a.result!,outcome:'byte_limit'}}).kind).toBe('full')
 expect(downloadGroup({...a,result:{...a.result!,http_status:429,outcome:'source_rate_limited'}}).kind).toBe('invalid')
 expect(downloadGroup({...a,result:{...a.result!,bytes_read:0}}).kind).toBe('invalid')
 expect(downloadGroup({...a,rule:{...a.rule,target_url:'https://other.test/data'}}).key).not.toBe(downloadGroup(a).key)
})
it('counts endpoint profiles and explicitly skipped records separately from strict rule passes',()=>{
 expect(servicePassed({...a,result:{...a.result!,outcome:'profiled'}})).toBe(false)
 expect(serviceExecuted({...a,result:{...a.result!,outcome:'profiled'}})).toBe(true)
 expect(servicePassed({...a,result:{...a.result!,outcome:'unlocked'}})).toBe(true)
 expect(servicePassed({...a,execution_state:'not_executed',result:{...a.result!,outcome:'unlocked'}})).toBe(false)
 expect(serviceExecuted({...a,result:{...a.result!,details:{execution_status:'not_executed'}}})).toBe(false)
})
it('only offers retry-save when the original staged result is recoverable',()=>{
 HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()
 const w=createWorkspace();w.queue.value=[{id:'q',node:n,project:'download',state:'completed',error:'保存失败：暂存失败',attempt:{...a,result:undefined}}]
 const ui=mount(QueueModal,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 expect(document.body.textContent).not.toContain('重试保存')
 expect(document.body.textContent).toContain('原结果无法恢复')
 ui.unmount();w.dispose()
})

it('shows an executed six-site failure as failed rather than unmeasured and retains children',()=>{
 const latency={...n,attempt_id:'all-failed',display_name:'n',node_type:'ss',requested_at:a.requested_at,finished_at:a.requested_at,status:'failed',latency_ms:0,total_samples:6,success_samples:0,failure_samples:6,persistence_state:'saved',samples:['speed.cloudflare.com','gstatic.com','api.github.com','captive.apple.com','msftconnecttest.com','detectportal.firefox.com'].map((target,i)=>({seq:i,target:'https://'+target,timestamp:a.requested_at,latency_ms:0,success:false,error:'context deadline exceeded'}))}
 const points=durableRoundPoints([{round_id:'failed',trigger_type:'manual',started_at:a.requested_at,state:'finished',items:[{...n,project:'latency',request_id:'q',execution_state:'failed',persistence_state:'saved',latency}]}],'latency','all')
 expect(points[0].tone).toBe('bad');expect(points[0].sampleGroups).toHaveLength(6);expect(points[0].samples).toHaveLength(6)
})
it('retains a measured strict service pass when its persistence fails',()=>{
 const attempt={...a,service_id:'one',result:{...a.result!,outcome:'unlocked'}}
 const points=durableRoundPoints([{round_id:'save-failed',trigger_type:'manual',started_at:a.requested_at,state:'finished',items:[{...n,project:'service',service_id:'one',request_id:'q',execution_state:'completed',persistence_state:'failed',service:attempt}]}],'service','all')
 expect(points[0].value).toBe(1);expect(points[0].description).toContain('保存失败 1')
})
it('uses explicit skipped or cancelled evidence even when an older execution state disagrees',()=>{
 expect(serviceConclusion({...a,execution_state:'failed',result:{...a.result!,outcome:'transport_error',details:{execution_status:'not_executed'}}})).toBe('未执行')
 expect(serviceConclusion({...a,result:{...a.result!,outcome:'cancelled'}})).toBe('已取消')
 expect(attemptTone({...a,result:{...a.result!,outcome:'credentials_required'}})).toBe('empty')
})

it('separates service comparisons when rule conditions or execution coverage differ',()=>{
 const one={...a,service_id:'one',result:{...a.result!,outcome:'matched'}}
 expect(serviceComparisonGroup([one],['one']).complete).toBe(true)
 expect(serviceComparisonGroup([one],['one','two']).complete).toBe(false)
 expect(serviceComparisonGroup([one],['one']).key).not.toBe(serviceComparisonGroup([{...one,rule:{...one.rule,rule_version:2}}],['one']).key)
})
