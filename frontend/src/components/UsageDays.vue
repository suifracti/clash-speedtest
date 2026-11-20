<script setup lang="ts">
import {computed,onBeforeUnmount,ref,shallowRef,watch} from 'vue'
import {bytes} from '../domain'
import {registerTipDismiss} from '../chartTip'
import {accountDailyUsage,shanghaiDay,shiftDay,type DayUsage,type UsageSnapshot} from '../usageDaily'
const props=withDefaults(defineProps<{airportId:string;subscriptionId:string;days?:number;chart?:boolean;summary?:boolean;refreshVersion?:number}>(),{days:14,chart:false,summary:false,refreshVersion:0})
const emit=defineEmits<{account:[value:UsageSnapshot|undefined]}>()
const series=ref<DayUsage[]>([]),account=ref<UsageSnapshot>(),error=ref(''),loading=ref(false)
let generation=0
async function load(){
 const current=++generation;loading.value=true;error.value=''
 try{const r=await accountDailyUsage(props.airportId,props.subscriptionId,props.days);if(current!==generation)return;series.value=r.days;account.value=r.account;emit('account',r.account)}
 catch(e){if(current===generation)error.value=e instanceof Error?e.message:String(e)}
 finally{if(current===generation)loading.value=false}
}
watch(()=>[props.airportId,props.subscriptionId,props.days,props.refreshVersion],()=>void load(),{immediate:true})
const statusText:Record<string,string>={ok:'机场已报告',baseline:'首次基线',reset:'计数重置',partial:'用量字段不完整',missing:'机场未提供用量',local_file:'本地配置，无用量',refresh_failed:'更新失败',observed:'已观测',cross_period:'跨期区间'}
const when=(v?:string)=>!v||v.startsWith('0001')?'':new Date(v).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})
const hm=(value?:string)=>!value||value.startsWith('0001')?'':new Date(value).toLocaleTimeString('zh-CN',{timeZone:'Asia/Shanghai',hour:'2-digit',minute:'2-digit',hour12:false})
function valueText(d:DayUsage){return d.bytes!=null?(d.estimated?'约 ':'')+(d.lowerBound?'≥':'')+bytes(d.bytes):d.recorded?'未知':'未记录'}
function text(d:DayUsage){return d.lowerBound&&d.since?`${valueText(d)}（${hm(d.since)} 起）`:valueText(d)}
function title(d:DayUsage){return `${d.date} · ${d.bytes!=null?bytes(d.bytes)+(d.estimated?`（${d.sourceKind==='notice_remaining'?'按公告余额变化估算':'区间估算'}）`:''):d.recorded?'有更新记录但无法归属增量':'当天没有更新记录'}${d.lowerBound?` · 今天从 ${hm(d.since)} 开始记录，实际用量不低于此值`:''}${d.message?' · '+d.message:''}`}
const summaryDays=computed<DayUsage[]>(()=>series.value.length?series.value.slice(0,3):['今天','昨天','前天'].map((label,index)=>({date:shiftDay(shanghaiDay(),-index),label,bytes:null,estimated:false,recorded:false})))
const max=computed(()=>Math.max(1,...(props.summary?summaryDays.value:series.value).map(d=>d.bytes||0)))
const summaryLabel=computed(()=>`近 3 日用量：${summaryDays.value.map(d=>`${d.label} ${text(d)}`).join('；')}`)
const chartDays=computed(()=>[...series.value].reverse())
const dayTip=shallowRef<{day:DayUsage;x:number;y:number;below:boolean}|null>(null)
let clearTipDismiss:(()=>void)|undefined
function showDayTip(e:MouseEvent|FocusEvent,day:DayUsage){
 const el=e.currentTarget as HTMLElement|null
 if(!el)return
 const rect=el.getBoundingClientRect()
 const vw=typeof window!=='undefined'?window.innerWidth:1280
 const x=Math.max(115,Math.min(vw-115,rect.left+rect.width/2))
 const below=rect.top<96
 dayTip.value={day,x,y:below?rect.bottom:rect.top,below}
 if(!clearTipDismiss)clearTipDismiss=registerTipDismiss(hideDayTip)
}
function hideDayTip(){dayTip.value=null;clearTipDismiss?.();clearTipDismiss=undefined}
function dayDetail(d:DayUsage){
 const notes:string[]=[]
 if(d.bytes!=null){
  if(d.estimated)notes.push(d.sourceKind==='notice_remaining'?'按公告余额变化估算':'区间估算')
  if(d.lowerBound&&d.since)notes.push(`从 ${hm(d.since)} 起记录，实际用量不低于此值`)
 }else{
  notes.push(d.recorded?'有更新记录但无法归属增量':'当天没有更新记录')
 }
 if(d.message)notes.push(d.message)
 return notes.join(' · ')
}
onBeforeUnmount(()=>hideDayTip())
const caption=computed(()=>{
 if(error.value)return '用量读取失败：'+error.value
 if(!account.value)return loading.value?'正在读取用量…':`近 ${props.days} 天没有用量记录`
 const s=account.value,status=statusText[s.status]||s.status
 return `截至 ${when(s.captured_at)||'—'} · ${s.counter_kind==='notice_remaining'?'公告余额':status}`
})
</script>
<template>
  <div class="cst-usage" :class="{chart}" aria-label="机场账户每日用量">
    <div v-if="summary" class="cst-usage-summary" role="img" :aria-label="summaryLabel" @mouseleave="hideDayTip">
      <div v-for="d in summaryDays" :key="d.date" class="cst-usage-day" :class="{unknown:d.bytes==null,estimated:d.estimated,active:dayTip?.day.date===d.date}" tabindex="0" :title="title(d)" @mouseenter="showDayTip($event,d)" @focus="showDayTip($event,d)" @blur="hideDayTip">
        <span class="cst-usage-day-value">{{valueText(d)}}</span>
        <span class="cst-usage-day-track"><i :class="{unknown:d.bytes==null,estimated:d.estimated}" :style="{height:d.bytes!=null?Math.max(6,d.bytes/max*100)+'%':'100%'}"/></span>
        <span class="cst-usage-day-label">{{d.label}}</span>
      </div>
    </div>
    <dl v-else class="cst-usage-days" @mouseleave="hideDayTip">
      <div v-for="d in series.slice(0,3)" :key="d.date" :title="title(d)" @mouseenter="showDayTip($event,d)"><dt>{{d.label}}</dt><dd :class="{unknown:d.bytes==null}">{{valueText(d)}}<small v-if="d.lowerBound&&d.since" class="cst-usage-since">（{{hm(d.since)}} 起）</small></dd></div>
      <div v-if="!series.length" class="cst-usage-none"><dt>今天</dt><dd class="unknown">{{loading?'…':'未记录'}}</dd></div>
    </dl>
    <div v-if="chart&&series.length" class="cst-usage-bars" role="img" :aria-label="`近 ${series.length} 天账户用量；空白表示没有可归属的数据`" @mouseleave="hideDayTip">
      <span v-for="d in chartDays" :key="d.date" :class="{unknown:d.bytes==null,estimated:d.estimated,active:dayTip?.day.date===d.date}" tabindex="0" :title="title(d)" @mouseenter="showDayTip($event,d)" @focus="showDayTip($event,d)" @blur="hideDayTip"><i :style="{height:d.bytes!=null?Math.max(16,d.bytes/max*100)+'%':'22%'}"/></span>
    </div>
    <div v-if="dayTip" class="cst-chart-tip" :class="{below:dayTip.below}" role="tooltip" :style="{left:dayTip.x+'px',top:dayTip.y+'px'}">
      <div class="cst-chart-tip-head"><span>{{dayTip.day.date}}</span><span>{{dayTip.day.label}}</span></div>
      <strong class="cst-chart-tip-val" :class="{unknown:dayTip.day.bytes==null}">{{valueText(dayTip.day)}}</strong>
      <span v-if="dayDetail(dayTip.day)" class="cst-chart-tip-meta">{{dayDetail(dayTip.day)}}</span>
    </div>
    <small class="cst-usage-caption" :class="{bad:!!error}" :title="account?.raw_error||account?.message||''">{{caption}}</small>
  </div>
</template>
<style>
.cst-usage{display:flex;flex-direction:column;gap:5px;min-width:0}
.cst-usage-days{display:flex;gap:14px;margin:0;flex-wrap:wrap}
.cst-usage-days div{display:flex;flex-direction:column;gap:1px;min-width:0}
.cst-usage-days dt{font-size:11px;color:var(--cst-muted)}
.cst-usage-days dd{margin:0;font-size:12.5px;font-weight:650;font-variant-numeric:tabular-nums;white-space:nowrap;color:var(--cst-text)}
.cst-usage-since{display:inline;margin-left:2px;font-size:10.5px;font-weight:400;color:var(--cst-muted)}
.cst-usage-days dd.unknown{font-weight:400;color:var(--cst-muted)}
.cst-usage-bars{display:grid;grid-auto-flow:column;grid-auto-columns:1fr;gap:3px;height:30px;align-items:end;padding:2px 0}
.cst-usage-bars span{height:100%;display:flex;align-items:flex-end;padding:2px 1px 0;border-radius:4px;cursor:pointer;transition:background .12s;outline:none}
.cst-usage-bars span:hover,.cst-usage-bars span.active,.cst-usage-bars span:focus-visible{background:var(--cst-accent-soft)}
.cst-usage-bars i{display:block;width:100%;border-radius:3px 3px 1px 1px;background:var(--cst-aurora-bar-v,var(--cst-accent));transition:filter .12s,box-shadow .12s}
.cst-usage-bars span:hover i,.cst-usage-bars span.active i{filter:brightness(1.1);box-shadow:0 0 10px -1px rgba(79,110,247,.5)}
.cst-usage-bars .estimated i{opacity:.62}
.cst-usage-bars .unknown i{background:var(--cst-empty-bg);border:1px dashed var(--cst-line-strong);box-sizing:border-box;box-shadow:none}
.cst-usage-bars .unknown:hover i,.cst-usage-bars .unknown.active i{border-color:var(--cst-accent)}
.cst-usage-caption{color:var(--cst-muted);font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.cst-usage-caption.bad{color:var(--cst-bad)}
.cst-usage-summary{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));align-items:end;gap:10px;height:78px;padding-bottom:6px;border-bottom:1px solid var(--cst-line)}
.cst-usage-day{display:grid;grid-template-rows:15px 1fr 15px;justify-items:center;align-items:end;gap:4px;min-width:0;height:100%;padding:2px;border-radius:7px;cursor:pointer;transition:background .12s;outline:none}
.cst-usage-day:hover,.cst-usage-day.active,.cst-usage-day:focus-visible{background:var(--cst-accent-soft)}
.cst-usage-day-value,.cst-usage-day-label{max-width:100%;font-size:11px;line-height:1.25;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.cst-usage-day-value{color:var(--cst-muted)}.cst-usage-day-label{color:var(--cst-text);font-weight:500}
.cst-usage-day-track{display:flex;align-items:flex-end;justify-content:center;width:28px;height:100%;overflow:hidden;border-radius:7px 7px 3px 3px;background:color-mix(in srgb,var(--cst-panel-2) 65%,transparent)}
.cst-usage-day-track i{display:block;width:100%;min-height:6px;border-radius:inherit;background:var(--cst-aurora-bar-v,var(--cst-accent))}
.cst-usage-day-track i.estimated{opacity:.55}
.cst-usage-day-track i.unknown{height:100%!important;background:repeating-linear-gradient(-45deg,var(--cst-line) 0 3px,transparent 3px 6px);border:1px dashed var(--cst-line-strong);box-sizing:border-box}
</style>
