import {expect,it} from 'vitest'
import {unlockGroup,unlockGroupDetail,unlockGroupSummary,unlockResult,unlockServiceAllowed,unlockSupported} from '../unlock'
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
it('skips only excluded regions when a service uses an exclusion list',()=>{
 const rules={antigravity:['!CN','!HK']}
 expect(unlockServiceAllowed('antigravity','HK',true,rules)).toBe(false)
 expect(unlockServiceAllowed('antigravity','US',true,rules)).toBe(true)
 expect(unlockServiceAllowed('antigravity','HK',false,rules)).toBe(true)
 const agOk={...attempt('matched'),service_id:'antigravity',result:{...attempt('matched').result!,outcome:'matched',details:{antigravity_progress:'真实模型已回答',checked_model:'gemini-2.5-flash',reply_preview:'READY',google_region:'JP',country_code:'JP',network_path:JSON.stringify({interface:'WLAN',socket_bind_verified:true})}}} as Attempt
 expect(unlockResult(agOk)).toMatchObject({state:'available',tone:'good',label:'模型可用（gemini-2.5-flash）'})
 expect(unlockResult(agOk).reason).toContain('WLAN 绕过 TUN')
 expect(unlockResult(agOk).reason).toContain('回答“READY”')
 expect(unlockResult(agOk).reason).toContain('Google 判定 JP')
 const agCn={...attempt('region_blocked'),service_id:'antigravity',result:{...attempt('region_blocked').result!,outcome:'region_blocked',details:{google_region:'CN',country_code:'JP'}}} as Attempt
 expect(unlockResult(agCn)).toMatchObject({state:'unavailable',tone:'warn',label:'出口送中（CN）'})
 const agBlocked={...attempt('region_blocked'),service_id:'antigravity'} as Attempt
 expect(unlockResult(agBlocked)).toMatchObject({state:'unavailable',tone:'warn',label:'地区不支持'})
 const cfClean={...attempt('profiled'),service_id:'cloudflare_challenge',result:{...attempt('profiled').result!,outcome:'profiled',details:{cf_challenge_detail:'ChatGPT: 直通(200) · Grok: 直通(200) · Claude: 直通(200)'}}} as Attempt
 expect(unlockResult(cfClean)).toMatchObject({state:'available',tone:'good',label:'未触发 CF 盾'})
 const cfPart={...attempt('verification_required'),service_id:'cloudflare_challenge',result:{...attempt('verification_required').result!,outcome:'verification_required',details:{cf_challenge_detail:'ChatGPT: 直通(200) · Grok: 直通(200) · Claude: 弹盾(403 cf-mitigated)'}}} as Attempt
 expect(unlockResult(cfPart)).toMatchObject({state:'verification',tone:'warn',label:'部分弹 CF 盾'})
 const cfBad={...attempt('challenge'),service_id:'cloudflare_challenge',result:{...attempt('challenge').result!,outcome:'challenge',details:{cf_challenge_detail:'ChatGPT: 弹盾(403) · Grok: 弹盾(403) · Claude: 弹盾(403)'}}} as Attempt
 expect(unlockResult(cfBad)).toMatchObject({state:'unavailable',tone:'bad',label:'易弹 CF 人机验证'})
 expect(unlockGroup('dlsite_unlock')).toBe('japan')
 expect(unlockServiceAllowed('dlsite_unlock','HK',true)).toBe(false)
 expect(unlockServiceAllowed('dlsite_unlock','JP',true)).toBe(true)
 const dlsiteOk={...attempt('unlocked'),service_id:'dlsite_unlock',result:{...attempt('unlocked').result!,outcome:'unlocked',details:{service_region:'JP',hidden_by_country:'false'}}} as Attempt
 expect(unlockResult(dlsiteOk)).toMatchObject({state:'available',tone:'good',label:'可用 · JP'})
 const dlsiteBlocked={...attempt('region_blocked'),service_id:'dlsite_unlock',result:{...attempt('region_blocked').result!,outcome:'region_blocked',summary:'DLsite 按当前出口地区隐藏锁区作品（RJ01571432）',details:{hidden_by_country:'true'}}} as Attempt
 expect(unlockResult(dlsiteBlocked)).toMatchObject({state:'unavailable',tone:'warn',label:'不可用'})
})

it('builds per-service detail items with failures prioritized before passing services',()=>{
 const catalog=[
  {service_id:'chatgpt_unlock',name:'ChatGPT'},
  {service_id:'claude_unlock',name:'Claude'},
  {service_id:'gemini_unlock',name:'Gemini'},
  {service_id:'antigravity',name:'Antigravity'},
 ]
 const map:Record<string,Attempt>={
  chatgpt_unlock:{...attempt('unlocked'),service_id:'chatgpt_unlock',result:{...attempt('unlocked').result!,outcome:'unlocked',details:{service_region:'US'}}} as Attempt,
  claude_unlock:{...attempt('region_blocked'),service_id:'claude_unlock',result:{...attempt('region_blocked').result!,outcome:'region_blocked',summary:'地区不支持'}} as Attempt,
  gemini_unlock:{...attempt('region_blocked'),service_id:'gemini_unlock',result:{...attempt('region_blocked').result!,outcome:'region_blocked',details:{service_region:'CN'},summary:'Google 判定为中国大陆'}} as Attempt,
 }
 const detail=unlockGroupDetail('ai',catalog,id=>map[id],'JP')
 expect(detail.label).toBe('1/3')
 expect(detail.attemptedItems.map(i=>`${i.shortName}:${i.shortStatus}`)).toEqual(['Gemini:送中','Claude:不可用 SG','ChatGPT:✓ US'])
 expect(detail.attemptedItems.map(i=>i.tone)).toEqual(['bad','warn','good'])
})

it('surfaces exact error messages and failure phases on transport or timeout failures',()=>{
 const agErr={service_id:'antigravity',execution_state:'completed',rule:{service_id:'antigravity',target_url:''},result:{outcome:'transport_error',failure_phase:'loadCodeAssist',error_message:'TLS 握手失败：connection reset by peer'}} as unknown as Attempt
 const res=unlockResult(agErr,'antigravity')
 expect(res.state).toBe('unknown')
 expect(res.reason).toContain('TLS 握手失败：connection reset by peer')
 expect(res.reason).toContain('loadCodeAssist')
})
