import {expect,it} from 'vitest'
import {exitLanding} from '../exitLanding'
import {detailFields,toExitRecord} from '../exitSummaryPresentation'
import {qualityMatchesGate,type QualityEvidence} from '../nodeQuality'
import {manualRoundLabel} from '../measurementRounds'
import type {Attempt,ManualRoundSummary} from '../domain'
let serial=0
const record=(id:string,details:Record<string,string>)=>({attempt_id:String(++serial),service_id:id,execution_state:'completed',persistence_state:'saved',rule:{service_id:id},result:{outcome:'profiled',details,finished_at:new Date(Date.UTC(2026,9,21,0,serial)).toISOString()}} as unknown as Attempt)
const records=[record('ippure_ip_quality',{ip:'1.2.3.4',country_code:'SG',risk_score:'18',risk_scale_max:'100',risk_direction:'lower_is_better',ip_type:'机房 IDC',origin_type:'原生 IP',asn_bot_percent:'94.22'}),record('cloudflare_trace',{country_code:'SG'}),record('google_region',{service_region:'SG'})]
it('combines the three region sources and masks the exit IP',()=>{expect(exitLanding({country_code:'SG'},records)).toMatchObject({ip:'1.*.*.4',country:'SG',label:'落地一致',tone:'good',riskLabel:'机器 94%',riskTone:'bad'});expect(exitLanding({country_code:'SG'},records).purity).toMatchObject({score:82,label:'82 · 机房',tone:'good'});expect(exitLanding({country_code:'SG'},records).purity.title).toContain('由 IPPure 风险分 18 换算 · 越高越干净');expect(exitLanding({country_code:'SG'},records).purity.summary).toContain('纯净度 82 · 机房 IP · 原生 SG');expect(exitLanding({country_code:'SG'},[]).purity.label).toBe('—')})
it('highlights a Google mismatch and makes any mainland source red',()=>{
 expect(exitLanding({country_code:'SG'},[...records,record('google_region',{service_region:'HK'})]).label).toContain('Google 判定 HK')
 expect(exitLanding({country_code:'SG'},[record('cloudflare_trace',{country_code:'CN'})])).toMatchObject({tone:'bad',label:'Cloudflare 判定 CN（中国大陆）'})
})

it('uses IP library, Cloudflare and name consensus instead of Google',()=>{
 expect(exitLanding({country_code:'SG'},[...records,record('google_region',{service_region:'US'})])).toMatchObject({consensusRegion:'SG',googleNote:'Google 判定 US，与落地 SG 不一致'})
 expect(exitLanding({country_code:'JP'},[record('ippure_ip_quality',{country_code:'SG'}),record('cloudflare_trace',{country_code:'HK'})]).consensusRegion).toBe('SG')
})

it('projects cloudflare_challenge status and rank in exitLanding and exitSummaryPresentation',()=>{
 const cf=(outcome:string,detail:string)=>({attempt_id:String(++serial),service_id:'cloudflare_challenge',execution_state:'completed',persistence_state:'saved',rule:{service_id:'cloudflare_challenge'},result:{outcome,details:{cf_challenge_detail:detail},finished_at:new Date(Date.UTC(2026,9,21,1,serial)).toISOString()}} as unknown as Attempt)
 expect(exitLanding({country_code:'SG'},[]).cfChallenge).toMatchObject({shortLabel:'未测',tone:'empty',rank:null})
 expect(exitLanding({country_code:'SG'},[cf('profiled','ChatGPT: 直通(200)')]).cfChallenge).toMatchObject({shortLabel:'直通',tone:'good',rank:0})
 expect(exitLanding({country_code:'SG'},[cf('verification_required','Claude: 弹盾(403)')]).cfChallenge).toMatchObject({shortLabel:'部分弹盾',tone:'warn',rank:1})
 const badAttempt=cf('challenge','ChatGPT: 弹盾(403) · Grok: 弹盾(403)')
 expect(exitLanding({country_code:'SG'},[badAttempt]).cfChallenge).toMatchObject({shortLabel:'易弹盾',tone:'bad',rank:2})
 const exitRec=toExitRecord(badAttempt,'2026-10-21T01:00:00Z')
 expect(exitRec).toMatchObject({provider:'Cloudflare 人机验证',status:'易弹 CF 人机验证'})
 expect(detailFields(exitRec).find(f=>f.label==='CF 盾站点明细')?.value).toContain('ChatGPT: 弹盾(403)')
})

it('compares multiple independent reputation stations and flags single-source fallback on timeout',()=>{
 const multi=exitLanding({country_code:'US'},[
  record('ippure_ip_quality',{ip:'23.1.2.3',country_code:'US',risk_score:'90',risk_scale_max:'100',risk_direction:'lower_is_better',cf_status:'已取得',cf_risk_score:'96'}),
  record('ping0_ipv4',{ip:'23.1.2.3',country_code:'US',risk_score:'29',ip_type:'机房 IDC'}),
  record('ipquery_ip_quality',{ip:'23.1.2.3',country_code:'US',risk_score:'12',ip_type:'机房 IDC'}),
  record('iplogs_ip_quality',{ip:'23.1.2.3',country_code:'US',risk_score:'85',ip_type:'机房 IDC'}),
 ])
 expect(multi.purity.sourceCount).toBe(4)
 expect(multi.purity.divergent).toBe(true)
 expect(multi.reputationSources.map(s=>`${s.name}:${s.risk}`)).toEqual(['IPPure:90','Ping0:29','IPQuery:12','IPLogs:85','CF系数:96'])

 const timedOutPure={attempt_id:'timeout-pure',service_id:'ippure_ip_quality',execution_state:'completed',persistence_state:'saved',rule:{service_id:'ippure_ip_quality'},result:{outcome:'timed_out',error_message:'请求超过设定超时',finished_at:'2026-11-20T08:00:00Z'}} as unknown as Attempt
 const fallback=exitLanding({country_code:'US'},[timedOutPure,record('ping0_ipv4',{ip:'23.1.2.3',country_code:'US',risk_score:'15',ip_type:'机房 IDC'})])
 expect(fallback.purity.singleFallback).toBe(true)
 expect(fallback.purity.tone).toBe('warn')
 expect(fallback.reputationIssues).toContain('IPPure 查询超时')

 const ev=(services:Attempt[]):QualityEvidence=>({tests:[],monitor:[],downloads:[],services,reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'2026-11-20T00:00:00Z',until:'2026-11-20T12:00:00Z'})
 // Clean multi-source consensus passes purity_clean gate.
 expect(qualityMatchesGate(ev([record('ippure_ip_quality',{risk_score:'12',risk_scale_max:'100',risk_direction:'lower_is_better'}),record('ping0_ipv4',{risk_score:'18'})]),undefined,'purity_clean','2026-11-20T12:00:00Z')).toBe(true)
 // Single-source fallback after another station timed out must NOT pass purity_clean.
 expect(qualityMatchesGate(ev([timedOutPure,record('ping0_ipv4',{risk_score:'15'})]),undefined,'purity_clean','2026-11-20T12:00:00Z')).toBe(false)
 // Strongly divergent stations (0 vs 36 -> avg risk 18 -> purity 82, spread 36>=30) must NOT pass purity_clean or render green.
 const divergentHigh=[record('ipquery_ip_quality',{risk_score:'0'}),record('iplogs_ip_quality',{risk_score:'36'})]
 expect(exitLanding({country_code:'SG'},divergentHigh).purity).toMatchObject({score:82,divergent:true,tone:'warn'})
 expect(qualityMatchesGate(ev(divergentHigh),undefined,'purity_clean','2026-11-20T12:00:00Z')).toBe(false)

 // Pure reputation rounds including cloudflare_challenge label cleanly as 出口与信誉.
 const repRound={round_id:'r1',started_at:'2026-11-20T12:00:00Z',state:'finished',trigger_type:'manual',node_count:4,latency_count:0,download_count:0,service_count:24,service_ids:['ippure_ip_quality','ipquery_ip_quality','iplogs_ip_quality','cloudflare_challenge']} as ManualRoundSummary
 expect(manualRoundLabel(repRound)).toBe('出口与信誉')
})
