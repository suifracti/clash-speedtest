import {expect,it} from 'vitest'
import {unlockGroupSummary,unlockResult,unlockServiceAllowed,unlockSupported} from '../unlock'
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
 expect(unlockResult(current)).toMatchObject({state:'partial',tone:'warn',label:'仅自制剧（SG）'})
})

it('uses saved service regions, empty overrides and disabled skipping',()=>{
 const rules={chatgpt_unlock:['HK','TW'],abema_unlock:[]}
 expect(unlockServiceAllowed('chatgpt_unlock','HK',true,rules)).toBe(true)
 expect(unlockServiceAllowed('chatgpt_unlock','US',true,rules)).toBe(false)
 expect(unlockServiceAllowed('abema_unlock','US',true,rules)).toBe(true)
 expect(unlockServiceAllowed('chatgpt_unlock','US',false,rules)).toBe(true)
})

it('shows Google verification separately without counting it as unavailable',()=>{
 const a={...attempt('verification_required'),service_id:'gemini_unlock'}
 expect(unlockResult(a)).toMatchObject({state:'verification',tone:'warn',label:'需人机验证'})
 expect(unlockGroupSummary('ai',[{service_id:'gemini_unlock',name:'Gemini'}],()=>a)).toMatchObject({total:1,label:'0/1',tone:'warn'})
 const cn={...a,result:{...a.result!,outcome:'region_blocked',details:{service_region:'CN'}}}
 expect(unlockResult(cn)).toMatchObject({label:'Google 判定为中国大陆',tone:'bad'})
})

it('counts verification in 2/3 and shows Spotify registration restriction in orange',()=>{ const catalog=['chatgpt_unlock','claude_unlock','gemini_unlock'].map(service_id=>({service_id,name:service_id}))
 expect(unlockGroupSummary('ai',catalog,id=>({...attempt(id==='gemini_unlock'?'verification_required':'unlocked'),service_id:id}))).toMatchObject({label:'2/3',tone:'warn'})
 expect(unlockResult({...attempt('service_rejected'),service_id:'spotify_unlock',result:{...attempt('service_rejected').result!,summary:'Spotify 拒绝代理 IP 注册'}})).toMatchObject({state:'partial',tone:'warn',label:'限注册',reason:'识别为代理 IP，已有账号通常不受影响'})
})

it('marks an IP ban red and a blocked Gemini region amber',()=>{
 const banned={...attempt('banned'),service_id:'netflix_unlock',result:{...attempt('banned').result!,summary:'Netflix 封禁该 IP'}}
 expect(unlockResult(banned)).toMatchObject({state:'unavailable',tone:'bad',label:'封禁'})
 const gemini={...attempt('region_blocked'),service_id:'gemini_unlock',result:{...attempt('region_blocked').result!,details:{service_region:'HK'},summary:'Gemini 在该地区不可用（HK）'}}
 expect(unlockResult(gemini)).toMatchObject({state:'unavailable',tone:'warn',label:'不可用（HK）'})
})

it('disables new Disney checks while preserving successful old results',()=>{
 expect(unlockSupported('disney_unlock')).toBe(false)
 expect(unlockResult(undefined,'disney_unlock')).toMatchObject({label:'暂不支持',tone:'empty'})
 expect(unlockResult({...attempt('unlocked'),service_id:'disney_unlock'}).state).toBe('available')
})

it('keeps YouTube not-available amber and only CN red',()=>{
 const blocked={...attempt('region_blocked'),service_id:'youtube_premium',result:{...attempt('region_blocked').result!,details:{},summary:'YouTube 不向该 IP 提供 Premium（常见于机房/代理 IP）'}}
 expect(unlockResult(blocked)).toMatchObject({state:'unavailable',tone:'warn',label:'不可用',reason:'YouTube 不向该 IP 提供 Premium（常见于机房/代理 IP）'})
 const cn={...blocked,result:{...blocked.result!,details:{service_region:'CN'},summary:'Google 判定为中国大陆'}}
 expect(unlockResult(cn)).toMatchObject({state:'unavailable',tone:'bad',label:'Google 判定为中国大陆'})
})
