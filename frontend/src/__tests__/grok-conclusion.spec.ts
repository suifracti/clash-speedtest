import {expect,it} from 'vitest'
import {serviceAvailability,servicePassed} from '../sharedResults'
import {outcomeLabel,outcomeTone,type Attempt} from '../domain'
it('keeps suspected challenge and entry reachability unconfirmed without promoting business access',()=>{
 const attempt=(outcome:string)=>({execution_state:'completed',persistence_state:'saved',service_id:'grok_web',rule:{service_id:'grok_web'},result:{outcome,http_status:403}} as Attempt)
 expect(serviceAvailability(attempt('suspected_challenge'))).toBeNull();expect(servicePassed(attempt('reachable'))).toBe(false)
 expect(serviceAvailability(attempt('challenge'))).toBe(0);expect(serviceAvailability(attempt('rate_limited'))).toBeNull()
 expect(outcomeLabel('suspected_challenge')).toBe('疑似挑战');expect(outcomeTone('suspected_challenge')).toBe('warn')
})
