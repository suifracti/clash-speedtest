<script setup lang="ts">
import {computed,inject,onMounted,ref} from 'vue'
import {useWorkspace} from '../../workspace'
import {date,downloadSpeed,key,median,type LatencyTest,type NodeOption} from '../../domain'
import {activeLatencyTargets,latencyTargets,latencyTargetId} from '../../latencyTargets'
import {downloadEvidence,latestServices,qualityGroups} from '../../nodeQuality'
import {nodeDisplayName} from '../../nodePresentation'
import {exitLanding} from '../../exitLanding'
import {latencySiteSamples,stableLatency} from '../../inspectorLatency'
import {unlockResult} from '../../unlock'
import QualityMetrics from '../QualityMetrics.vue'
import UiSelect from '../UiSelect.vue'
import Icon from '../Icon.vue'
import {homeModelKey,weeklyHistoryKey} from './homeModel'
const w=useWorkspace(),m=inject(homeModelKey)!
const section=ref<'all'|'latency'|'download'|'service'>('all')
const onlyDiffs=ref(false)
const sections=[{id:'all',label:'全部指标'},{id:'latency',label:'延迟'},{id:'download',label:'下载'},{id:'service',label:'服务与出口'}] as const
const services=computed(()=>Object.fromEntries(m.compareNodes.value.map(n=>[key(n),latestServices(m.qualityData.value[key(n)]?.services||[])])))
const exitSummaries=computed(()=>Object.fromEntries(m.compareNodes.value.map(n=>[key(n),exitLanding(n,m.qualityData.value[key(n)]?.services||[])])))

interface FourSiteChip {id:string;name:string;shortName:string;p50:number|null;success:number;total:number;tone:'good'|'warn'|'bad'|'empty';label:string}
interface FourSiteDigest {chips:FourSiteChip[];med:number|null;responding:number;measured:number;barPct:number;deltaFromBest:number|null;isBest:boolean}
const fourSiteMap=computed<Record<string,FourSiteDigest>>(()=>{
 const raw=m.compareNodes.value.map(n=>{
  const ev=m.qualityData.value[key(n)]
  const chips:FourSiteChip[]=activeLatencyTargets.map(t=>{
   const g=ev?qualityGroups(ev.tests,ev.monitor,t.id)[0]:undefined
   const p50=g&&g.success>0?g.p50:null
   const total=g?.total??0,success=g?.success??0
   const tone:FourSiteChip['tone']=!total?'empty':success===0?'bad':success<total||(p50!==null&&p50>=1200)?'warn':'good'
   const label=p50!==null?`${Math.round(p50)}ms`:total>0?'超时':'未测'
   return {id:t.id,name:t.name,shortName:t.id==='cloudflare'?'CF':t.name,p50,success,total,tone,label}
  })
  const vals=chips.map(c=>c.p50).filter((v):v is number=>v!==null&&Number.isFinite(v)).sort((a,b)=>a-b)
  const med=median(vals)
  return {k:key(n),chips,med,responding:vals.length,measured:chips.filter(c=>c.total>0).length}
 })
 const validMeds=raw.map(r=>r.med).filter((v):v is number=>v!==null)
 const minMed=validMeds.length>=2?Math.min(...validMeds):null
 const maxMed=validMeds.length?Math.max(...validMeds,1):1
 const hasDistinct=validMeds.length>=2&&Math.max(...validMeds)-Math.min(...validMeds)>=1
 return Object.fromEntries(raw.map(r=>[r.k,{
  chips:r.chips,
  med:r.med,
  responding:r.responding,
  measured:r.measured,
  barPct:r.med!==null?Math.max(12,Math.min(100,Math.round(r.med/maxMed*100))):0,
  deltaFromBest:r.med!==null&&minMed!==null&&hasDistinct?Math.round(r.med-minMed):null,
  isBest:r.med!==null&&minMed!==null&&hasDistinct&&r.med===minMed,
 }]))
})

interface RoundSnapshot {atMs:number;time:string;med:number|null;success:number;total:number;tone:'good'|'warn'|'bad'}
interface TrendDigest {rounds:RoundSnapshot[];badge:string;tone:'good'|'warn'|'bad'|'muted';detail:string;weeklyRate:number|null}
function buildNodeRounds(tests:LatencyTest[]):RoundSnapshot[]{
 const dated=tests.filter(t=>(t.samples?.length||0)>0).map(t=>{
  const time=t.finished_at||t.requested_at||''
  const atMs=Date.parse(time)
  return {t,time,atMs:Number.isFinite(atMs)?atMs:0}
 }).sort((a,b)=>a.atMs-b.atMs)
 const buckets:Array<{atMs:number;time:string;tests:LatencyTest[]}>=[]
 for(const item of dated){
  const prev=buckets[buckets.length-1]
  if(prev&&Math.abs(item.atMs-prev.atMs)<=60_000){prev.tests.push(item.t);if(item.atMs>=prev.atMs){prev.atMs=item.atMs;prev.time=item.time}}
  else buckets.push({atMs:item.atMs,time:item.time,tests:[item.t]})
 }
 return buckets.map(b=>{
  const bySite=new Map<string,number[]>()
  let success=0,total=0
  for(const t of b.tests){
   for(const s of t.samples||[]){
    total++
    if(s.success&&Number.isFinite(s.latency_ms)){
     success++
     const id=latencyTargetId(s.target||t.target||'')||'other'
     const list=bySite.get(id)||[]
     list.push(s.latency_ms)
     bySite.set(id,list)
    }
   }
  }
  const siteMeds=[...bySite.values()].map(arr=>median([...arr].sort((x,y)=>x-y))).filter((v):v is number=>v!==null).sort((x,y)=>x-y)
  const med=median(siteMeds)
  const tone:RoundSnapshot['tone']=success===0?'bad':success<total||(med!==null&&med>=1200)?'warn':'good'
  return {atMs:b.atMs,time:b.time,med,success,total,tone}
 })
}
function buildSitePointRounds(n:NodeOption):RoundSnapshot[]{
 const pts=m.sitePoints(n,'all').filter(p=>activeLatencyTargets.some(t=>latencySiteSamples(p,t.id).length))
 return pts.map(p=>{
  const siteMeds:number[]=[]
  let success=0,total=0
  for(const t of activeLatencyTargets){
   const samples=latencySiteSamples(p,t.id)
   if(!samples.length)continue
   total+=samples.length
   success+=samples.filter(s=>s.value!==null).length
   const ms=stableLatency([{id:t.id,points:[{...p,samples}]}])
   if(ms!==null&&Number.isFinite(ms))siteMeds.push(ms)
  }
  const med=median(siteMeds)
  const atMs=Date.parse(p.time)
  const tone:RoundSnapshot['tone']=success===0?'bad':success<total||(med!==null&&med>=1200)?'warn':'good'
  return {atMs:Number.isFinite(atMs)?atMs:0,time:p.time,med,success,total,tone}
 }).filter(r=>r.total>0).sort((a,b)=>a.atMs-b.atMs)
}
const trendMap=computed<Record<string,TrendDigest>>(()=>Object.fromEntries(m.compareNodes.value.map(n=>{
 const ev=m.qualityData.value[key(n)]
 const evRounds=ev?buildNodeRounds(ev.tests):[]
 const merged=[...evRounds]
 if(merged.length<2){
  for(const ptRound of buildSitePointRounds(n)){
   if(!merged.some(r=>Math.abs(r.atMs-ptRound.atMs)<=60_000))merged.push(ptRound)
  }
  merged.sort((a,b)=>a.atMs-b.atMs)
 }
 const rounds=merged.slice(-6)
 const weekly=m.weeklyStats.value[weeklyHistoryKey(n)]
 const weeklyRate=weekly&&!weekly.allOld&&weekly.all.responseRate!=null?Math.round(weekly.all.responseRate):null
 if(!rounds.length)return [key(n),{rounds,badge:'',tone:'muted' as const,detail:'未测',weeklyRate}]
 const cur=rounds[rounds.length-1]
 if(rounds.length===1){
  const curText=cur.med!==null?`${Math.round(cur.med)} ms`:'无响应'
  return [key(n),{rounds,badge:'仅 1 轮',tone:'muted' as const,detail:`${date(cur.time)} · ${curText}，暂无上轮可对比`,weeklyRate}]
 }
 const prev=rounds[rounds.length-2]
 if(prev.success>0&&cur.success===0){
  return [key(n),{rounds,badge:'转离线',tone:'bad' as const,detail:`上轮 ${date(prev.time)} ${prev.med!==null?Math.round(prev.med)+' ms':'有响应'} → 本轮无响应`,weeklyRate}]
 }
 if(prev.success===0&&cur.success>0){
  return [key(n),{rounds,badge:'已恢复',tone:'good' as const,detail:`上轮 ${date(prev.time)} 无响应 → 本轮 ${cur.med!==null?Math.round(cur.med)+' ms':'已恢复'}`,weeklyRate}]
 }
 if(cur.med!==null&&prev.med!==null){
  const diff=Math.round(cur.med-prev.med)
  const ratio=prev.med>0?Math.abs(diff)/prev.med:0
  const sign=diff>0?'+':''
  const pct=prev.med>0&&Math.abs(diff)>=40?` (${sign}${Math.round(diff/prev.med*100)}%)`:''
  const badge=diff>=80||(diff>=40&&ratio>=0.25)?`↑+${diff}ms`:diff<=-80||(diff<=-40&&ratio>=0.25)?`↓${diff}ms`:Math.abs(diff)<20?'持平':`${sign}${diff}ms`
  const tone:TrendDigest['tone']=diff>=300||ratio>=0.7?'bad':diff>=80||(diff>=40&&ratio>=0.25)?'warn':diff<=-80||(diff<=-40&&ratio>=0.25)?'good':'muted'
  return [key(n),{rounds,badge,tone,detail:`上轮 ${date(prev.time)} ${Math.round(prev.med)} ms → ${Math.round(cur.med)} ms${pct}`,weeklyRate}]
 }
 return [key(n),{rounds,badge:`共 ${rounds.length} 轮`,tone:'muted' as const,detail:`最近 ${date(cur.time)}`,weeklyRate}]
})))

const downloadComparable=computed(()=>m.comparisonDownloadStatus.value.includes('条件一致'))
const downloadRankMap=computed<Record<string,{speed:number|null;barPct:number;isBest:boolean}>>(()=>{
 const entries=m.compareNodes.value.map(n=>{
  const ev=m.qualityData.value[key(n)]
  const valid=downloadEvidence(ev?.downloads||[]).valid
  const speed=downloadSpeed(valid)
  return {k:key(n),speed:speed!==null&&speed>0?speed:null}
 })
 const validSpeeds=entries.map(e=>e.speed).filter((v):v is number=>v!==null)
 const maxSpd=validSpeeds.length?Math.max(...validSpeeds):0
 const hasDistinct=validSpeeds.length>=2&&Math.max(...validSpeeds)-Math.min(...validSpeeds)>=0.05
 return Object.fromEntries(entries.map(e=>[e.k,{
  speed:e.speed,
  barPct:e.speed!==null&&maxSpd>0?Math.max(10,Math.min(100,Math.round(e.speed/maxSpd*100))):0,
  isBest:e.speed!==null&&hasDistinct&&e.speed===maxSpd,
 }]))
})

type Field='fourSite'|'trend'|'distribution'|'response'|'download'|'service'|'reputation'|'cfChallenge'|'time'|'details'
interface Row {id:string;field:Field;label:string;serviceId?:string}
const serviceRows=computed<Row[]>(()=>{
 const names=new Map<string,string>()
 for(const attempts of Object.values(services.value))for(const a of attempts){const id=a.service_id||a.rule.service_id!;names.set(id,w.catalog.value.find(s=>s.service_id===id)?.name||a.rule.name||id)}
 const order=new Map(w.catalog.value.map((s,i)=>[s.service_id,i]))
 return [...names].sort(([a,an],[b,bn])=>(order.get(a)??Infinity)-(order.get(b)??Infinity)||an.localeCompare(bn)).map(([id,label])=>({id:'service:'+id,field:'service',serviceId:id,label}))
})
const allRows=computed<Row[]>(()=>[
 ...(section.value==='all'||section.value==='latency'?[
  {id:'fourSite',field:'fourSite' as const,label:'四站综合延迟'},
  {id:'trend',field:'trend' as const,label:'较上次变化'},
  {id:'distribution',field:'distribution' as const,label:'延迟分布'},
  {id:'response',field:'response' as const,label:'成功率 / 样本'},
 ]:[]),
 ...(section.value==='all'||section.value==='download'?[{id:'download',field:'download' as const,label:'下载'}]:[]),
 ...(section.value==='all'||section.value==='service'?[{id:'reputation',field:'reputation' as const,label:'多源信誉'},{id:'cfChallenge',field:'cfChallenge' as const,label:'CF 人机验证'}]:[]),
 ...(section.value==='all'||section.value==='service'?serviceRows.value.length?serviceRows.value:[{id:'service',field:'service' as const,label:'服务结论'}]:[]),
 ...(section.value!=='service'?[{id:'time',field:'time' as const,label:'测量时间'}]:[]),{id:'details',field:'details',label:'完整证据'}
])
function rowSignature(row:Row,n:NodeOption){
 const k=key(n)
 if(row.field==='service'){
  if(!row.serviceId)return services.value[k]?.length?services.value[k].map(a=>`${a.service_id||a.rule.service_id}:${unlockResult(a)?.label||a.result?.outcome||''}`).join('|'):'untested'
  const a=services.value[k]?.find(item=>(item.service_id||item.rule.service_id)===row.serviceId)
  if(!a)return 'untested'
  const u=unlockResult(a)
  return u?`${u.state}:${u.label}`:(a.result?.outcome||a.execution_state)
 }
 if(row.field==='reputation'){
  const ex=exitSummaries.value[k]
  const srcs=ex?.reputationSources.filter(s=>s.risk!==null).map(s=>`${s.id}:${s.risk}`).join(',')||''
  const iss=ex?.reputationIssues.join(',')||''
  return srcs||iss?`${srcs}|${iss}|${ex?.ipKind.tag||''}`:'untested'
 }
 if(row.field==='cfChallenge')return exitSummaries.value[k]?.cfChallenge.shortLabel||'untested'
 if(row.field==='download'){
  const dl=downloadEvidence(m.qualityData.value[k]?.downloads||[])
  const spd=downloadSpeed(dl.valid)
  return spd!==null?`${spd.toFixed(1)}`:(dl.latest?.result?.outcome||dl.latest?.execution_state||'untested')
 }
 if(row.field==='fourSite'){
  const fs=fourSiteMap.value[k]
  return fs?.measured?fs.chips.map(c=>`${c.id}:${c.label}`).join('|'):'untested'
 }
 if(row.field==='trend'){
  const tr=trendMap.value[k]
  return tr?.rounds.length?`${tr.badge}|${tr.detail}`:'untested'
 }
 return null
}
function rowDiffers(row:Row){
 if(m.compareNodes.value.length<2)return true
 if(['distribution','response','time','details'].includes(row.field))return true
 const sigs=m.compareNodes.value.map(n=>rowSignature(row,n))
 if(sigs.some(s=>s===null))return true
 return new Set(sigs).size>1
}
const rows=computed<Row[]>(()=>onlyDiffs.value?allRows.value.filter(rowDiffers):allRows.value)
const hiddenDiffCount=computed(()=>onlyDiffs.value?allRows.value.length-rows.value.length:0)

const root=ref<HTMLElement>()
function history(n:NodeOption){
 const evidence=m.qualityData.value[key(n)],project=section.value==='all'?'latency':section.value
 const source=project==='latency'&&evidence&&qualityGroups(evidence.tests,evidence.monitor,m.compareTarget.value)[0]?.source==='monitor'?'monitor':'manual'
 m.closeComparison()
 w.inspectNode.value={...n,historyTargetId:m.compareTarget.value,historyProject:project,historySource:source,historyUntil:evidence?.until||m.qualityCutoff.value,...(project==='service'?{historyServiceId:''}:{})}
}
function serviceEvidence(n:NodeOption,id:string){const a=services.value[key(n)]?.find(a=>(a.service_id||a.rule.service_id)===id);if(a)m.showServiceEvidence(a)}
onMounted(()=>root.value?.querySelector<HTMLElement>('.hd-compare-head button')?.focus({preventScroll:true}))
</script>
<template>
 <aside ref="root" class="hd-compare" aria-label="节点比较" @keydown.esc.stop="m.closeComparison()">
  <header class="hd-compare-head">
   <div><h2>比较 {{m.compareNodes.value.length}} 个已选节点</h2><p class="hd-muted quality-compare-meta">截止 {{date(m.qualityCutoff.value)}} · 已保存记录</p></div>
   <button class="hd-btn" type="button" @click="m.closeComparison()"><Icon name="close"/>退出比较</button>
  </header>
  <div class="comparison-tools">
   <div class="comparison-sections" role="group" aria-label="比较内容"><button v-for="item in sections" :key="item.id" type="button" :aria-pressed="section===item.id" @click="section=item.id">{{item.label}}</button></div>
   <div class="comparison-tools-right">
    <button v-if="m.compareNodes.value.length>=2" type="button" class="compare-diff-toggle" :aria-pressed="onlyDiffs" @click="onlyDiffs=!onlyDiffs">仅看有差异<span v-if="hiddenDiffCount>0" class="compare-diff-count">已隐藏 {{hiddenDiffCount}}</span></button>
    <label v-if="section==='all'||section==='latency'">延迟站点<UiSelect v-model="m.compareTarget.value" aria-label="比较延迟站点"><option v-for="target in latencyTargets" :key="target.id" :value="target.id">{{target.name}}{{['microsoft','firefox'].includes(target.id)?'（历史）':''}}</option></UiSelect></label>
   </div>
  </div>
  <div class="comparison-verdicts">
   <p v-if="section==='all'||section==='latency'" class="quality-compare-status" :class="m.compareCompatible.value?'good':'warn'"><span>延迟</span>{{m.compareCompatible.value?'最新组条件一致，可比较描述统计。':'不可比较：条件不同、样本不足、未测或读取不完整。'}}</p>
   <p v-if="section==='all'||section==='download'" class="comparison-download-status hd-muted"><span>下载</span>{{m.comparisonDownloadStatus.value.replace(/^下载/,'')}}</p>
   <p v-if="section==='service'" class="hd-muted">每行对应一个服务；点击结论查看该条保存证据。HTTP 响应不等于业务可用。</p>
  </div>
  <details class="quality-compare-boundaries hd-compare-rules"><summary>比较口径与边界</summary><p>延迟按所选站点的最新条件组展示；P95 至少 20 个成功样本，少于 5 个成功样本不排名，缺少出口或地址族/来源证据时只展示原值。下载仅同源、方法、上限、结束类型及已记录的出口地址条件相同时可比较，缺少必要出口证据时不做数值排序；历史有效速度不代表最近检测成功。服务逐项核对同一规则、方法、地址条件和时间，HTTP 响应不等于业务可用。切换比较内容和站点不改变检测范围、勾选和工作台站点。</p></details>
  <p v-if="m.compareNodes.value.length>2" id="comparison-scroll-hint" class="comparison-scroll-hint hd-muted">← 超出视野时横向滚动；左侧指标固定。聚焦表格可用左右方向键。 →</p>
  <div class="quality-compare hd-compare-table" role="region" aria-label="节点逐项比较表" tabindex="0" :aria-describedby="m.compareNodes.value.length>2?'comparison-scroll-hint':undefined" :style="{'--compare-count':m.compareNodes.value.length}">
   <table><colgroup><col class="comparison-label-column"><col v-for="n in m.compareNodes.value" :key="key(n)"></colgroup>
    <thead><tr><th scope="col">指标 / 服务</th><th v-for="(n,i) in m.compareNodes.value" :key="key(n)" scope="col"><span class="comparison-node-number">{{String(i+1).padStart(2,'0')}}</span><strong>{{nodeDisplayName(n)}}</strong><small>{{m.sourceLabel(n)}}</small><small>{{n.type}}</small></th></tr></thead>
    <tbody>
     <tr v-for="row in rows" :key="row.id"><th scope="row">{{row.label}}</th><td v-for="n in m.compareNodes.value" :key="key(n)" :class="{'compare-cell-best':(row.field==='fourSite'&&fourSiteMap[key(n)]?.isBest&&m.compareCompatible.value)||(row.field==='download'&&downloadRankMap[key(n)]?.isBest&&downloadComparable)}">
      <div v-if="row.field==='fourSite'" class="compare-foursite-cell">
       <template v-if="fourSiteMap[key(n)]?.measured">
        <div class="compare-foursite-head">
         <strong class="compare-foursite-ms">{{fourSiteMap[key(n)].med!==null?`${Math.round(fourSiteMap[key(n)].med!)} ms`:'全部超时'}}</strong>
         <span v-if="fourSiteMap[key(n)].isBest" class="compare-winner-pill" :class="m.compareCompatible.value?'good':'warn'">{{m.compareCompatible.value?'最快':'参考最低'}}</span>
         <span v-else-if="fourSiteMap[key(n)].deltaFromBest!==null&&fourSiteMap[key(n)].deltaFromBest!>0" class="compare-diff-ms">慢 +{{fourSiteMap[key(n)].deltaFromBest}} ms</span>
        </div>
        <div v-if="fourSiteMap[key(n)].med!==null" class="compare-bar"><i :class="fourSiteMap[key(n)].isBest&&m.compareCompatible.value?'good':fourSiteMap[key(n)].med!>=1200?'bad':fourSiteMap[key(n)].med!>=600?'warn':'accent'" :style="{width:fourSiteMap[key(n)].barPct+'%'}"/></div>
        <div class="compare-site-chips">
         <button v-for="c in fourSiteMap[key(n)].chips" :key="c.id" type="button" class="compare-site-chip" :class="[c.tone,{active:m.compareTarget.value===c.id}]" :title="`${c.name}：${c.label}（${c.success}/${c.total}），点击下方聚焦此站点`" @click="m.compareTarget.value=c.id"><b>{{c.shortName}}</b><span>{{c.label}}</span></button>
        </div>
       </template>
       <span v-else class="hd-muted">未测</span>
      </div>
      <div v-else-if="row.field==='trend'" class="compare-trend-cell">
       <template v-if="trendMap[key(n)]?.rounds.length">
        <div class="compare-trend-head">
         <span class="compare-trend-badge" :class="trendMap[key(n)].tone">{{trendMap[key(n)].badge}}</span>
         <span class="compare-trend-dots" :aria-label="`近期 ${trendMap[key(n)].rounds.length} 轮记录`"><i v-for="(r,idx) in trendMap[key(n)].rounds" :key="idx" :class="r.tone" :title="`${date(r.time)} · ${r.med!==null?Math.round(r.med)+' ms':'无响应'} (${r.success}/${r.total})`"/></span>
        </div>
        <small class="compare-exit-meta">{{trendMap[key(n)].detail}}</small>
        <small v-if="trendMap[key(n)].weeklyRate!==null" class="compare-exit-meta">近 7 天响应率 {{trendMap[key(n)].weeklyRate}}%</small>
       </template>
       <span v-else class="hd-muted">未测</span>
      </div>
      <div v-else-if="row.field==='reputation'" class="compare-exit-cell" :title="exitSummaries[key(n)]?.purity.title">
       <template v-if="exitSummaries[key(n)]?.reputationSources.length||exitSummaries[key(n)]?.reputationIssues.length">
        <span class="hd-rep-grid">
         <span v-for="src in exitSummaries[key(n)].reputationSources.filter(s=>s.risk!==null)" :key="src.id" class="hd-rep-chip" :class="src.tone" :title="src.detail"><b>{{src.name}}</b><i>{{src.risk}}</i></span>
         <span v-for="iss in exitSummaries[key(n)].reputationIssues" :key="iss" class="hd-rep-chip issue">{{iss}}</span>
        </span>
        <small class="compare-exit-meta">{{exitSummaries[key(n)].ipKind.tag}} · {{exitSummaries[key(n)].label}}</small>
       </template>
       <span v-else class="hd-muted">未测</span>
      </div>
      <div v-else-if="row.field==='cfChallenge'" class="compare-exit-cell" :title="exitSummaries[key(n)]?.cfChallenge.detail">
       <span class="hd-cf-tag" :class="exitSummaries[key(n)]?.cfChallenge.tone||'empty'">{{exitSummaries[key(n)]?.cfChallenge.shortLabel||'未测'}}</span>
       <small v-if="exitSummaries[key(n)]?.cfChallenge.tone!=='empty'" class="compare-exit-meta">{{exitSummaries[key(n)].cfChallenge.detail}}</small>
      </div>
      <div v-else-if="row.field==='download'" class="compare-download-wrap">
       <div v-if="downloadRankMap[key(n)]?.speed!==null" class="compare-dl-bar-row">
        <div class="compare-bar"><i :class="downloadRankMap[key(n)].isBest&&downloadComparable?'good':'accent'" :style="{width:downloadRankMap[key(n)].barPct+'%'}"/></div>
        <span v-if="downloadRankMap[key(n)].isBest" class="compare-winner-pill" :class="downloadComparable?'good':'warn'">{{downloadComparable?'最快下载':'参考最快'}}</span>
       </div>
       <QualityMetrics :comparison-field="row.field" :project="section==='all'?'combined':section" :evidence="m.qualityData.value[key(n)]" :error="m.qualityErrors.value[key(n)]" :target="m.compareTarget.value" :catalog="w.catalog.value" @select-service="serviceEvidence(n,$event)"/>
      </div>
      <QualityMetrics v-else :comparison-field="row.field" :project="section==='all'?'combined':section" :visible-service-ids="row.serviceId?[row.serviceId]:row.field==='time'?[]:undefined" :evidence="m.qualityData.value[key(n)]" :error="m.qualityErrors.value[key(n)]" :target="m.compareTarget.value" :catalog="w.catalog.value" @select-service="serviceEvidence(n,$event)"/>
     </td></tr>
     <tr v-if="!rows.length&&onlyDiffs"><td :colspan="m.compareNodes.value.length+1" class="compare-empty-diffs">当前标签下所选节点各项结论一致（关闭「仅看有差异」可查看全部）。</td></tr>
     <tr><th scope="row">原始历史</th><td v-for="n in m.compareNodes.value" :key="key(n)"><button class="hd-btn" @click="history(n)">查看原始历史</button></td></tr>
    </tbody>
   </table>
  </div>
 </aside>
</template>
<style scoped>
.hd-compare{gap:12px}
.hd-compare-head h2{font-size:18px;letter-spacing:-.3px}
.comparison-tools{display:flex;justify-content:space-between;gap:10px 12px;align-items:center;flex-wrap:wrap}
.comparison-tools-right{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.comparison-sections{display:inline-flex;gap:3px;padding:3px;border-radius:10px;border:1px solid var(--cst-line,var(--hd-line));background:var(--cst-panel-2,var(--hd-panel-2))}
.comparison-sections button{background:none;border:1px solid transparent;border-radius:7px;padding:6px 11px;font-size:12px;color:var(--hd-muted);cursor:pointer;white-space:nowrap;transition:background .14s,color .14s,border-color .14s,box-shadow .14s}
.comparison-sections button:hover{color:var(--hd-text)}
.comparison-sections button[aria-pressed=true]{background:var(--cst-panel-solid,var(--hd-panel));border-color:color-mix(in srgb,var(--hd-accent) 26%,var(--hd-line));color:var(--hd-accent);font-weight:650;box-shadow:var(--cst-shadow-xs,0 1px 2px rgba(0,0,0,.06))}
.compare-diff-toggle{display:inline-flex;align-items:center;gap:6px;height:30px;padding:0 10px;border-radius:999px;border:1px solid var(--hd-line);background:var(--hd-panel);color:var(--hd-muted);font-size:12px;font-weight:600;cursor:pointer;white-space:nowrap;transition:border-color .14s,background .14s,color .14s}
.compare-diff-toggle:hover{border-color:var(--hd-accent);color:var(--hd-text)}
.compare-diff-toggle[aria-pressed=true]{background:var(--cst-accent-soft,color-mix(in srgb,var(--hd-accent) 12%,var(--hd-panel)));border-color:color-mix(in srgb,var(--hd-accent) 40%,var(--hd-line));color:var(--hd-accent)}
.compare-diff-count{padding:1px 6px;border-radius:999px;background:var(--hd-panel);border:1px solid color-mix(in srgb,var(--hd-accent) 28%,var(--hd-line));font-size:10.5px;font-variant-numeric:tabular-nums}
.comparison-tools label{display:flex;align-items:center;gap:8px;font-size:12px;color:var(--hd-muted)}
.comparison-tools .ui-select{width:150px}
.comparison-verdicts{display:flex;flex-direction:column;gap:5px;font-size:12px;line-height:1.5}
.comparison-verdicts p{display:flex;gap:12px;align-items:baseline}
.comparison-verdicts p>span{flex:0 0 26px;color:var(--hd-muted);font-size:11px}
.hd-compare-table{max-height:66vh;border-radius:var(--cst-radius,12px);border:1px solid var(--cst-line,var(--hd-line));box-shadow:var(--cst-shadow-xs,none)}
.hd-compare-table table{table-layout:fixed;min-width:calc(118px + var(--compare-count,2) * 246px)}
.hd-compare-table .comparison-label-column{width:118px}
.hd-compare-table th,.hd-compare-table td{padding:12px 15px}
.hd-compare-table thead th{vertical-align:bottom;z-index:3}
.hd-compare-table thead th:first-child{z-index:4}
.hd-compare-table thead strong{display:block;margin-bottom:4px;font-size:14px;color:var(--hd-text);font-weight:650}
.comparison-node-number{display:block;font-size:10px;color:var(--hd-accent);font-variant-numeric:tabular-nums;margin-bottom:6px;letter-spacing:1px}
.hd-compare-table tbody th{font-size:12px;line-height:1.6;color:var(--hd-muted);font-weight:600}
.compare-cell-best{background:color-mix(in srgb,var(--hd-good) 6%,transparent)}
.compare-foursite-cell,.compare-trend-cell,.compare-download-wrap{display:flex;flex-direction:column;gap:6px;min-width:0}
.compare-foursite-head,.compare-trend-head,.compare-dl-bar-row{display:flex;align-items:center;gap:6px;flex-wrap:wrap}
.compare-foursite-ms{font-size:15px;font-weight:750;font-variant-numeric:tabular-nums;color:var(--hd-text)}
.compare-winner-pill{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;font-size:10.5px;font-weight:700;line-height:1.4;white-space:nowrap;border:1px solid transparent}
.compare-winner-pill.good{color:var(--hd-good);background:var(--hd-good-bg);border-color:color-mix(in srgb,var(--hd-good) 30%,transparent)}
.compare-winner-pill.warn{color:var(--hd-warn);background:var(--hd-warn-bg);border-color:color-mix(in srgb,var(--hd-warn) 34%,transparent)}
.compare-diff-ms{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;font-size:10.5px;font-weight:650;line-height:1.4;color:var(--hd-text);background:var(--hd-panel-2);border:1px solid var(--hd-line);white-space:nowrap;font-variant-numeric:tabular-nums}
.compare-bar{flex:none;width:100%;height:6px;border-radius:999px;background:var(--cst-aurora-track,var(--hd-line));overflow:hidden}
.compare-dl-bar-row .compare-bar{flex:1 1 80px;width:auto;min-width:64px}
.compare-bar>i{display:block;height:100%;border-radius:inherit}
.compare-bar>i.good{background:var(--cst-aurora-good,var(--hd-good))}
.compare-bar>i.accent{background:var(--cst-aurora-bar,var(--hd-accent))}
.compare-bar>i.warn{background:var(--cst-aurora-warn,var(--hd-warn))}
.compare-bar>i.bad{background:linear-gradient(90deg,#e11d48 0%,#fb7185 100%)}
.compare-site-chips{display:flex;flex-wrap:wrap;gap:4px}
.compare-site-chip{display:inline-flex;align-items:center;gap:4px;padding:2px 6px;border-radius:6px;border:1px solid var(--hd-line);background:var(--hd-panel-2);font-size:10.5px;line-height:1.35;cursor:pointer;font-variant-numeric:tabular-nums;transition:border-color .12s,background .12s}
.compare-site-chip b{font-weight:600;color:var(--hd-muted);font-size:10px}
.compare-site-chip span{font-weight:650;color:var(--hd-text)}
.compare-site-chip.warn{background:var(--hd-warn-bg);border-color:color-mix(in srgb,var(--hd-warn) 30%,transparent)}.compare-site-chip.warn span{color:var(--hd-warn)}
.compare-site-chip.bad{background:var(--hd-bad-bg);border-color:color-mix(in srgb,var(--hd-bad) 30%,transparent)}.compare-site-chip.bad span{color:var(--hd-bad)}
.compare-site-chip.empty span{color:var(--hd-muted);font-weight:500}
.compare-site-chip.active,.compare-site-chip:hover{border-color:var(--hd-accent);box-shadow:0 0 0 1px color-mix(in srgb,var(--hd-accent) 28%,transparent)}
.compare-trend-badge{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;font-size:11px;font-weight:700;font-variant-numeric:tabular-nums;border:1px solid var(--hd-line);background:var(--hd-panel-2);color:var(--hd-muted)}
.compare-trend-badge.good{color:var(--hd-good);background:var(--hd-good-bg);border-color:color-mix(in srgb,var(--hd-good) 28%,transparent)}
.compare-trend-badge.warn{color:var(--hd-warn);background:var(--hd-warn-bg);border-color:color-mix(in srgb,var(--hd-warn) 32%,transparent)}
.compare-trend-badge.bad{color:var(--hd-bad);background:var(--hd-bad-bg);border-color:color-mix(in srgb,var(--hd-bad) 30%,transparent)}
.compare-trend-dots{display:inline-flex;align-items:center;gap:3px}
.compare-trend-dots i{display:block;width:8px;height:8px;border-radius:2px;background:var(--hd-good)}
.compare-trend-dots i.warn{background:var(--hd-warn)}
.compare-trend-dots i.bad{background:var(--hd-bad)}
.compare-exit-cell{display:flex;flex-direction:column;align-items:flex-start;gap:5px}
.compare-exit-meta{font-size:11px;color:var(--hd-muted);line-height:1.45;overflow-wrap:anywhere}
.compare-empty-diffs{text-align:center;color:var(--hd-muted);font-size:12px;padding:18px 16px!important}
@media(max-width:600px){.comparison-tools{align-items:flex-start}.hd-compare-head h2{font-size:16px}.comparison-sections button{padding:6px 8px}.hd-compare-table th,.hd-compare-table td{padding:10px 12px}.hd-compare-table{max-height:60vh}}
</style>
