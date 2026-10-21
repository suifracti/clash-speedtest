import {expect,it} from 'vitest'
import {unlockGroupSummary,unlockResult} from '../unlock'
import type {Attempt} from '../domain'
const attempt=(outcome:string)=>({service_id:'chatgpt_unlock',execution_state:'completed',rule:{service_id:'chatgpt_unlock',target_url:''},result:{outcome,details:{service_region:'SG'},summary:'地区不支持'}} as unknown as Attempt)
it('shares available counts and explicit region refusal reasons',()=>{
 const catalog=[{service_id:'chatgpt_unlock',name:'ChatGPT'},{service_id:'claude_unlock',name:'Claude'}]
 const summary=unlockGroupSummary('ai',catalog,id=>attempt(id==='chatgpt_unlock'?'unlocked':'region_blocked'))
 expect(summary.label).toBe('1/2');expect(summary.tone).toBe('warn');expect(summary.reason).toContain('Claude：地区不支持');expect(unlockResult(attempt('unlocked')).label).toBe('可用 · SG')
})
it('keeps a browser challenge indeterminate',()=>{expect(unlockResult(attempt('challenge'))).toMatchObject({state:'unknown',tone:'empty',reason:'验证挑战'})})
