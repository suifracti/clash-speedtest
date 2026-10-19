<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {bytes} from '../domain'
import {accountDailyUsage,type DayUsage,type UsageSnapshot} from '../usageDaily'
const props=withDefaults(defineProps<{airportId:string;subscriptionId:string;days?:number;chart?:boolean}>(),{days:14,chart:false})
const series=ref<DayUsage[]>([]),account=ref<UsageSnapshot>(),error=ref(''),loading=ref(false)
let generation=0
async function load(){
 const current=++generation;loading.value=true;error.value=''
 try{const r=await accountDailyUsage(props.airportId,props.subscriptionId,props.days);if(current!==generation)return;series.value=r.days;account.value=r.account}
 catch(e){if(current===generation)error.value=e instanceof Error?e.message:String(e)}
 finally{if(current===generation)loading.value=false}
}
watch(()=>[props.airportId,props.subscriptionId,props.days],()=>void load(),{immediate:true})
const statusText:Record<string,string>={ok:'机场已报告',baseline:'首次基线',reset:'计数重置',missing:'机场未提供用量',local_file:'本地配置，无用量',refresh_failed:'更新失败',observed:'已观测',cross_period:'跨期区间'}
const when=(v?:string)=>!v||v.startsWith('0001')?'':new Date(v).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})
function text(d:DayUsage){return d.bytes!=null?(d.estimated?'约 ':'')+bytes(d.bytes):d.recorded?'未知':'未记录'}
function title(d:DayUsage){return `${d.date} · ${d.bytes!=null?bytes(d.bytes)+(d.estimated?'（含近午夜估算）':''):d.recorded?'有更新记录但无法归属增量':'当天没有更新记录'}`}
const max=computed(()=>Math.max(1,...series.value.map(d=>d.bytes||0)))
const caption=computed(()=>{
 if(error.value)return '用量读取失败：'+error.value
 if(!account.value)return loading.value?'正在读取用量…':`近 ${props.days} 天没有用量记录`
 const s=account.value,status=statusText[s.status]||s.status
 return `截至 ${when(s.captured_at)||'—'} · ${status}`
})
</script>
<template>
  <div class="cst-usage" :class="{chart}" aria-label="机场账户每日用量">
    <dl class="cst-usage-days">
      <div v-for="d in series.slice(0,3)" :key="d.date" :title="title(d)"><dt>{{d.label}}</dt><dd :class="{unknown:d.bytes==null}">{{text(d)}}</dd></div>
      <div v-if="!series.length" class="cst-usage-none"><dt>今天</dt><dd class="unknown">{{loading?'…':'未记录'}}</dd></div>
    </dl>
    <div v-if="chart&&series.length" class="cst-usage-bars" role="img" :aria-label="`近 ${series.length} 天账户用量；空白表示没有可归属的数据`">
      <span v-for="d in [...series].reverse()" :key="d.date" :class="{unknown:d.bytes==null,estimated:d.estimated}" :title="title(d)"><i :style="{height:d.bytes!=null?Math.max(6,d.bytes/max*100)+'%':'100%'}"/></span>
    </div>
    <small class="cst-usage-caption" :class="{bad:!!error}" :title="account?.message||''">{{caption}}</small>
  </div>
</template>
<style>
.cst-usage{display:flex;flex-direction:column;gap:4px;min-width:0}
.cst-usage-days{display:flex;gap:14px;margin:0}
.cst-usage-days div{display:flex;flex-direction:column;min-width:0}
.cst-usage-days dt{font-size:11px;color:var(--cst-muted)}
.cst-usage-days dd{margin:0;font-weight:600;font-variant-numeric:tabular-nums;white-space:nowrap}
.cst-usage-days dd.unknown{font-weight:400;color:var(--cst-muted)}
.cst-usage-bars{display:grid;grid-auto-flow:column;grid-auto-columns:1fr;gap:2px;height:28px;align-items:end}
.cst-usage-bars span{height:100%;display:flex;align-items:flex-end}
.cst-usage-bars i{display:block;width:100%;border-radius:2px;background:var(--cst-accent)}
.cst-usage-bars .estimated i{opacity:.55}
.cst-usage-bars .unknown i{background:repeating-linear-gradient(-45deg,var(--cst-line) 0 2px,transparent 2px 4px);border:1px dashed var(--cst-line-strong);box-sizing:border-box}
.cst-usage-caption{color:var(--cst-muted);font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.cst-usage-caption.bad{color:var(--cst-bad)}
</style>
