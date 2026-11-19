<script setup lang="ts">
import {computed,inject} from 'vue'
import {date,targets,type NodeOption} from '../../domain'
import {latencyFailureReason,latencySiteSamples} from '../../inspectorLatency'
import {latencyPointLabel,type SamplePoint,type TrendPoint} from '../../presentation'
import {homeModelKey} from './homeModel'
import {useWorkspace} from '../../workspace'
const props=defineProps<{node:NodeOption;detailed?:boolean}>()
const emit=defineEmits<{round:[point:TrendPoint,target:string]}>()
const m=inject(homeModelKey)!,w=useWorkspace()
function sampleLabel(sample:SamplePoint,index:number){
 const text=sample.value!==null?Math.round(sample.value)+' ms':sample.executionState==='cancelled'?'已取消':sample.tone==='empty'?'未执行':latencyFailureReason(sample.error||'')
 return `第 ${index+1} 次 · ${text}`
}
function historyLabel(point:TrendPoint,target:string){
 return `${date(point.time)} · ${point.source==='monitor'?'持续监测':'手动检测'} · ${latencyPointLabel(point,target)}`
}
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)).map(t=>{
 const point=m.latencyRowPoint(props.node,t.id),samples=latencySiteSamples(point,t.id)
 const history=props.detailed?m.sitePoints(props.node,t.id).filter(p=>latencySiteSamples(p,t.id).length).slice(-8):[]
 return {...t,point,samples,history,
  text:props.detailed?latencyPointLabel(point,t.id):m.siteMetric(props.node,t.id),
  context:props.detailed?(point?historyLabel(point,t.id):'本轮没有该站点的样本'):m.siteContext(props.node,t.id),
  responses:samples.filter(s=>s.value!==null).length,
 }
}))
const peak=computed(()=>Math.max(1,...sites.value.flatMap(s=>s.samples.map(sample=>sample.value??0))))
function sampleHeight(sample:SamplePoint){return sample.value===null?6:5+Math.round(15*sample.value/peak.value)}
function moveHistory(event:KeyboardEvent,direction:number){
 const button=event.currentTarget as HTMLButtonElement
 const next=(direction<0?button.previousElementSibling:button.nextElementSibling) as HTMLButtonElement|null
 next?.focus()
}
</script>
<template>
    <span v-for="s in sites" :key="s.id" role="cell" class="response-stop" :class="[s.point?.tone||'empty',{'response-detailed':detailed}]" :title="s.name+' · '+s.context">
      <span class="response-site">{{s.name}}</span><strong>{{s.text}}</strong>
      <template v-if="detailed">
        <button v-if="s.point&&s.samples.length" class="response-round" type="button" :aria-label="s.name+(s.point.source==='monitor'?' 最近监测顺序 · ':' 本轮顺序 · ')+s.samples.map(sampleLabel).join('；')" @click.stop="emit('round',s.point,s.id)">
          <span class="response-vertical" aria-hidden="true"><i v-for="(sample,index) in s.samples" :key="sample.id" :class="sample.tone" :style="{height:sampleHeight(sample)+'px'}" :title="sampleLabel(sample,index)"/></span>
          <small>{{s.point.source==='monitor'?'最近监测':'本轮'}} {{s.responses}}/{{s.samples.length}}</small>
        </button>
        <span v-else class="response-unrecorded" aria-hidden="true"/>
        <span v-if="s.history.length" class="response-history" role="group" :aria-label="s.name+' 最近历史'">
          <button v-for="(p,index) in s.history" :key="p.source+':'+p.id" type="button" :tabindex="index===s.history.length-1?0:-1" :class="[p.tone,{monitor:p.source==='monitor'}]" :title="historyLabel(p,s.id)" :aria-label="s.name+' · '+historyLabel(p,s.id)" @keydown.left.stop.prevent="moveHistory($event,-1)" @keydown.right.stop.prevent="moveHistory($event,1)" @click.stop="emit('round',p,s.id)"/>
        </span>
      </template>
      <span v-else-if="s.samples.length" class="response-samples" :aria-label="s.name+' 本轮 '+s.samples.length+' 次实际采样'"><i v-for="(sample,index) in s.samples" :key="sample.id" :class="sample.tone" :title="sampleLabel(sample,index)"/></span>
      <span v-else class="response-unrecorded" aria-hidden="true"/>
    </span>
</template>
<style scoped>
.response-stop{display:flex;flex-direction:column;justify-content:center;align-items:flex-start;gap:5px;min-width:0;position:relative;min-height:44px;padding-left:10px;border-left:1px solid var(--hd-line);color:var(--hd-text)}
.response-site{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%);white-space:nowrap}.response-stop strong{font-size:13px;font-weight:550;font-variant-numeric:tabular-nums;white-space:nowrap}
.response-samples{display:flex;gap:2px;width:100%;height:4px;margin-top:2px}.response-samples i{flex:1;min-width:1px;background:var(--hd-accent);border-radius:1px}.response-samples i.bad{background:var(--hd-bad)}.response-samples i.warn{background:var(--hd-warn)}.response-samples i.empty{background:transparent;border:1px solid var(--hd-line)}
.response-unrecorded{height:4px;width:100%;border-bottom:1px dotted var(--hd-line);margin-top:2px}
.response-stop.warn strong{color:var(--hd-warn)}.response-stop.bad strong{color:var(--hd-bad)}.response-stop.empty strong{color:var(--hd-muted);font-weight:400}
.response-detailed{gap:4px;min-height:75px;padding-block:4px;justify-content:flex-start}
.response-round{appearance:none;padding:0;border:0;background:none;display:grid;gap:2px;width:100%;text-align:left;cursor:pointer;color:var(--hd-muted)}
.response-round small{font-size:10px;line-height:1.3;white-space:nowrap}
.response-vertical{height:21px;display:flex;align-items:flex-end;gap:2px}
.response-vertical i{flex:1;min-width:1px;max-width:9px;border-radius:1px;background:var(--hd-accent)}
.response-vertical i.bad,.response-history button.bad::before{background:var(--hd-bad)}
.response-vertical i.warn,.response-history button.warn::before{background:var(--hd-warn)}
.response-vertical i.empty,.response-history button.empty::before{background:var(--hd-panel-2);border:1px solid var(--hd-line)}
.response-history{display:flex;gap:2px;width:100%;align-items:center}
.response-history button{position:relative;padding:0;flex:1 1 0;min-width:0;max-width:14px;height:18px;border:0;border-radius:2px;background:none;cursor:pointer}
.response-history button::before{content:"";position:absolute;inset:3px 1px;border:1px solid transparent;border-radius:2px;background:var(--hd-good)}
.response-history button.monitor::before{border-color:var(--hd-text);border-style:dotted}
.response-round:hover small{color:var(--hd-accent)}
.response-history button:hover,.response-history button:focus-visible,.response-round:focus-visible{outline:2px solid var(--hd-accent);outline-offset:1px}
</style>
