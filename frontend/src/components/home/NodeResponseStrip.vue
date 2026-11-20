<script setup lang="ts">
import {computed,inject,onBeforeUnmount,onMounted,shallowRef} from 'vue'
import {date,targets,type NodeOption} from '../../domain'
import {latencyFailureReason,latencySiteSamples} from '../../inspectorLatency'
import {latencyPointLabel,type SamplePoint,type TrendPoint} from '../../presentation'
import {homeModelKey} from './homeModel'
import {useWorkspace} from '../../workspace'
import LatencyHistoryMarks from './LatencyHistoryMarks.vue'
const props=defineProps<{node:NodeOption;detailed?:boolean;review?:{point:TrendPoint;target:string}}>()
const emit=defineEmits<{round:[point:TrendPoint,target:string];history:[point:TrendPoint,target:string]}>()
const m=inject(homeModelKey)!,w=useWorkspace()
function sampleLabel(sample:SamplePoint,index:number){
 const text=sample.value!==null?Math.round(sample.value)+' ms':sample.executionState==='cancelled'?'已取消':sample.tone==='empty'?'未执行':latencyFailureReason(sample.error||'')
 const note=sample.note?` · ${sample.note}`:''
 return `第 ${index+1} 次 · ${text}${note}`
}
function sampleShortStatus(sample:SamplePoint){
 if(sample.value!==null){
  const code=sample.note?.match(/HTTP (\d+)/)?.[1]
  return {text:`${Math.round(sample.value)} ms`,badge:code?`HTTP ${code}`:'',tone:code?'warn':sample.value>1500?'warn':'good'}
 }
 if(sample.executionState==='cancelled')return {text:'已取消',badge:'',tone:'empty'}
 if(sample.tone==='empty')return {text:'未执行',badge:'',tone:'empty'}
 return {text:latencyFailureReason(sample.error||''),badge:'',tone:'bad'}
}
function historyLabel(point:TrendPoint,target:string){
 return `${date(point.time)} · ${point.source==='monitor'?'持续监测':'手动检测'} · ${latencyPointLabel(point,target)}`
}
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)).map(t=>{
 const review=props.review&&(props.review.target==='all'||props.review.target===t.id)?props.review.point:undefined
 const point=m.latencyRowPoint(props.node,t.id,review),samples=latencySiteSamples(point,t.id)
 const history=props.detailed?m.sitePoints(props.node,t.id).filter(p=>latencySiteSamples(p,t.id).length).slice(-8):[]
 const httpCode=samples.map(s=>s.note?.match(/HTTP (\d+)/)?.[1]).find(Boolean)||''
 const httpCount=httpCode?samples.filter(s=>s.note?.includes('HTTP '+httpCode)).length:0
 return {...t,point,samples,history,review,httpCode,httpCount,
  text:props.detailed?latencyPointLabel(point,t.id):m.siteMetric(props.node,t.id),
  context:props.detailed?(point?historyLabel(point,t.id):review?date(review.time)+' · 该轮未测此站':'本轮没有该站点的样本'):m.siteContext(props.node,t.id),
  roundLabel:review?'回看':point?.source==='monitor'?'最近监测':'本轮',
  responses:samples.filter(s=>s.value!==null).length,
 }
}))
const peak=computed(()=>Math.max(1,...sites.value.flatMap(s=>s.samples.map(sample=>sample.value??0))))
function sampleHeight(sample:SamplePoint){return sample.value===null?6:5+Math.round(15*sample.value/peak.value)}
type SiteInfo=(typeof sites.value)[number]
const tip=shallowRef<{site:SiteInfo;x:number;y:number;below:boolean}|null>(null)
function showTip(e:MouseEvent|FocusEvent,site:SiteInfo){
 if(!site.samples.length)return
 const el=e.currentTarget as HTMLElement|null
 if(!el)return
 const rect=el.getBoundingClientRect()
 const vw=typeof window!=='undefined'?window.innerWidth:1280
 const x=Math.max(140,Math.min(vw-140,rect.left+rect.width/2))
 const below=rect.top<200
 tip.value={site,x,y:below?rect.bottom:rect.top,below}
}
function hideTip(){tip.value=null}
onMounted(()=>window.addEventListener('scroll',hideTip,true))
onBeforeUnmount(()=>window.removeEventListener('scroll',hideTip,true))
</script>
<template>
    <span v-for="s in sites" :key="s.id" role="cell" class="response-stop" :class="[s.point?.tone||'empty',{'response-detailed':detailed,'response-reviewing':s.review,'has-http-note':!!s.httpCode}]" :title="s.name+' · '+s.context">
      <span class="response-site">{{s.name}}</span>
      <span class="response-head-line"><strong>{{s.text}}</strong><b v-if="s.httpCode" class="response-http-tag" :title="`${s.name} ${s.httpCount}/${s.samples.length} 次返回 HTTP ${s.httpCode}（已连通，可能限流/风控）`">{{s.httpCode}}</b></span>
      <template v-if="detailed">
        <button v-if="s.point&&s.samples.length" class="response-round" type="button" :aria-label="s.name+' '+s.roundLabel+'顺序 · '+s.samples.map(sampleLabel).join('；')" @mouseenter="showTip($event,s)" @mouseleave="hideTip" @focus="showTip($event,s)" @blur="hideTip" @click.stop="hideTip();emit('round',s.point,s.id)">
          <span class="response-vertical" aria-hidden="true"><i v-for="(sample,index) in s.samples" :key="sample.id" :class="[sample.tone,{'http-note':!!sample.note}]" :style="{height:sampleHeight(sample)+'px'}" :title="sampleLabel(sample,index)"/></span>
          <small>{{s.roundLabel}} {{s.responses}}/{{s.samples.length}}</small>
        </button>
        <small v-else-if="s.review" class="response-missing">该轮未测</small>
        <span v-else class="response-unrecorded" aria-hidden="true"/>
        <LatencyHistoryMarks :points="s.history" :label="s.name+' 最近历史'" :target="s.id" :selected="s.review" @select="emit('history',$event,s.id)"/>
      </template>
      <span v-else-if="s.samples.length" class="response-samples" :aria-label="s.name+' 本轮 '+s.samples.length+' 次实际采样'" @mouseenter="showTip($event,s)" @mouseleave="hideTip"><i v-for="(sample,index) in s.samples" :key="sample.id" :class="sample.tone" :title="sampleLabel(sample,index)"/></span>
      <span v-else class="response-unrecorded" aria-hidden="true"/>
    </span>
    <Teleport to="body">
      <div v-if="tip" class="cst-chart-tip response-sample-tip" :class="{below:tip.below}" role="tooltip" :style="{left:tip.x+'px',top:tip.y+'px'}">
        <div class="cst-chart-tip-head"><span>{{tip.site.name}} · {{tip.site.roundLabel}} {{tip.site.responses}}/{{tip.site.samples.length}}</span><span v-if="tip.site.point">{{date(tip.site.point.time)}}</span></div>
        <div v-if="tip.site.httpCode" class="response-tip-http">已连通但返回 HTTP {{tip.site.httpCode}}（{{tip.site.httpCount}}/{{tip.site.samples.length}} 次，可能限流或风控）</div>
        <ol class="response-tip-list">
          <li v-for="(sample,index) in tip.site.samples" :key="sample.id">
            <span class="tip-seq">#{{index+1}}{{index===0?' 首次':''}}</span>
            <span class="tip-val" :class="sampleShortStatus(sample).tone">{{sampleShortStatus(sample).text}}</span>
            <b v-if="sampleShortStatus(sample).badge" class="tip-http">{{sampleShortStatus(sample).badge}}</b>
          </li>
        </ol>
      </div>
    </Teleport>
</template>
<style scoped>
.response-stop{display:flex;flex-direction:column;justify-content:center;align-items:flex-start;gap:5px;min-width:0;position:relative;min-height:44px;padding-left:10px;border-left:1px solid var(--hd-line);color:var(--hd-text)}
.response-site{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%);white-space:nowrap}
.response-head-line{display:inline-flex;align-items:center;gap:5px;max-width:100%}
.response-stop strong{font-size:13px;font-weight:550;font-variant-numeric:tabular-nums;white-space:nowrap}
.response-http-tag{display:inline-flex;align-items:center;padding:0 5px;border-radius:999px;background:var(--hd-warn-bg);border:1px solid color-mix(in srgb,var(--hd-warn) 34%,transparent);color:var(--hd-warn);font-size:9.5px;font-weight:700;line-height:1.45;font-variant-numeric:tabular-nums}
.response-samples{display:flex;gap:2px;width:100%;height:4px;margin-top:2px}.response-samples i{flex:1;min-width:1px;background:var(--hd-accent);border-radius:1px}.response-samples i.bad{background:var(--hd-bad)}.response-samples i.warn{background:var(--hd-warn)}.response-samples i.empty{background:transparent;border:1px solid var(--hd-line)}
.response-unrecorded{height:4px;width:100%;border-bottom:1px dotted var(--hd-line);margin-top:2px}
.response-stop.warn strong{color:var(--hd-warn)}.response-stop.bad strong{color:var(--hd-bad)}.response-stop.empty strong{color:var(--hd-muted);font-weight:400}
.response-detailed{gap:4px;min-height:75px;padding-block:4px;justify-content:flex-start}
.response-round{appearance:none;padding:2px 3px;margin:-2px -3px;border:0;border-radius:6px;background:none;display:grid;gap:2px;width:calc(100% + 6px);text-align:left;cursor:pointer;color:var(--hd-muted);transition:background .12s}
.response-round:hover{background:var(--cst-accent-soft)}
.response-round small{font-size:10px;line-height:1.3;white-space:nowrap}
.response-vertical{height:21px;display:flex;align-items:flex-end;gap:2px}
.response-vertical i{flex:1;min-width:1px;max-width:9px;border-radius:2px 2px 1px 1px;background:var(--cst-aurora-bar-v,var(--hd-accent))}
.response-vertical i.http-note{background:var(--cst-aurora-warn,var(--hd-warn))}
.response-vertical i.bad{background:var(--hd-bad)}
.response-vertical i.warn{background:var(--hd-warn)}
.response-vertical i.empty{background:var(--hd-panel-2);border:1px solid var(--hd-line)}
.response-reviewing .response-round small{color:var(--hd-accent)}
.response-missing{font-size:10px;color:var(--hd-muted);margin-top:7px}
.response-round:hover small{color:var(--hd-accent)}
.response-round:focus-visible{outline:2px solid var(--hd-accent);outline-offset:1px}
</style>
<style>
.response-sample-tip{min-width:196px;max-width:270px;gap:5px}
.response-tip-http{padding:3px 7px;border-radius:6px;background:var(--cst-warn-bg);color:var(--cst-warn);font-size:10.5px;font-weight:600;line-height:1.4}
.response-tip-list{list-style:none;margin:0;padding:0;display:grid;gap:3px}
.response-tip-list li{display:flex;align-items:center;gap:6px;font-size:11px;font-variant-numeric:tabular-nums}
.response-tip-list .tip-seq{color:var(--cst-muted);min-width:44px}
.response-tip-list .tip-val{font-weight:650;color:var(--cst-text)}
.response-tip-list .tip-val.good{color:var(--cst-good)}
.response-tip-list .tip-val.warn{color:var(--cst-warn)}
.response-tip-list .tip-val.bad{color:var(--cst-bad)}
.response-tip-list .tip-http{margin-left:auto;padding:0 5px;border-radius:999px;background:var(--cst-warn-bg);color:var(--cst-warn);font-size:9.5px;font-weight:700}
</style>
