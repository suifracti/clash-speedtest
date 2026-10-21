import {expect,it} from 'vitest'
import {unlockGroupSummary,unlockResult,unlockServiceAllowed} from '../unlock'
import type {Attempt} from '../domain'
const attempt=(outcome:string)=>({service_id:'chatgpt_unlock',execution_state:'completed',rule:{service_id:'chatgpt_unlock',target_url:''},result:{outcome,details:{service_region:'SG'},summary:'地区不支持'}} as unknown as Attempt)
it('shares available counts and explicit region refusal reasons',()=>{
 const catalog=[{service_id:'chatgpt_unlock',name:'ChatGPT'},{service_id:'claude_unlock',name:'Claude'}]
 const summary=unlockGroupSummary('ai',catalog,id=>attempt(id==='chatgpt_unlock'?'unlocked':'region_blocked'))
 expect(summary.label).toBe('1/2');expect(summary.tone).toBe('warn');expect(summary.reason).toContain('Claude：地区不支持');expect(unlockResult(attempt('unlocked')).label).toBe('可用 · SG')
})
it('keeps a browser challenge indeterminate',()=>{expect(unlockResult(attempt('challenge'))).toMatchObject({state:'unknown',tone:'empty',reason:'验证挑战'})})

it('separates untested from inconclusive and keeps old Netflix restrictions indeterminate',()=>{
 expect(unlockResult()).toMatchObject({label:'未测',reason:'',tone:'empty'})
 const old={...attempt('region_blocked'),service_id:'netflix_unlock'}
 expect(unlockResult(old)).toMatchObject({state:'unknown',tone:'empty',label:'仅自制剧或不可用（旧检测）'})
 const current={...old,result:{...old.result!,outcome:'originals_only',details:{service_region:'SG',title_check_version:'dual_title_v1'}}}
 expect(unlockResult(current)).toMatchObject({state:'available',tone:'warn',label:'仅自制剧（SG）'})
})

it('uses saved service regions, empty overrides and disabled skipping',()=>{
 const rules={chatgpt_unlock:['HK','TW'],abema_unlock:[]}
 expect(unlockServiceAllowed('chatgpt_unlock','HK',true,rules)).toBe(true)
 expect(unlockServiceAllowed('chatgpt_unlock','US',true,rules)).toBe(false)
 expect(unlockServiceAllowed('abema_unlock','US',true,rules)).toBe(true)
 expect(unlockServiceAllowed('chatgpt_unlock','US',false,rules)).toBe(true)
})
