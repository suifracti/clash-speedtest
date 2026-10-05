<script setup lang="ts">
import UiSelect from './UiSelect.vue'
import {computed,ref,watch} from 'vue'
import {bytes,date,downloadSpeed,downloadTone,outcomeLabel,outcomeTone,stateLabel} from '../domain'
import {monitorHTTPServices,type TrendPoint} from '../presentation'
const props=defineProps<{point:TrendPoint;project:'download'|'service'}>()
const search=ref(''),status=ref('all'),page=ref(1),pageSize=12
const attempts=computed(()=>props.point.attempts||[])
const counts=computed(()=>({good:attempts.value.filter(a=>a.result&&outcomeTone(a.result.outcome)==='good').length,warn:attempts.value.filter(a=>a.result&&outcomeTone(a.result.outcome)==='warn').length,bad:attempts.value.filter(a=>a.result&&outcomeTone(a.result.outcome)==='bad').length,empty:attempts.value.filter(a=>!a.result).length}))
const filtered=computed(()=>attempts.value.filter(a=>(status.value==='all'||(a.result?outcomeTone(a.result.outcome):'empty')===status.value)&&(!search.value||(a.rule.name+' '+a.service_id+' '+(a.result?.summary||'')+' '+(a.result?.error_message||'')).toLowerCase().includes(search.value.trim().toLowerCase()))))
const pageCount=computed(()=>Math.max(1,Math.ceil(filtered.value.length/pageSize))),visible=computed(()=>filtered.value.slice((page.value-1)*pageSize,page.value*pageSize))
watch([search,status],()=>page.value=1)
watch(()=>props.point.id,()=>{search.value='';status.value='all';page.value=1})
watch(pageCount,count=>page.value=Math.min(page.value,count))
</script>
<template>
<section class="measurement-results" :aria-label="project==='service'?'本轮服务结果':'本次测速数据'">
  <header><strong>{{project==='service'?'本轮服务结果':'本次测速数据'}}</strong><small>{{date(point.time)}}</small></header>
  <template v-if="point.source==='monitor'&&point.monitorSamples?.length"><p class="small muted">持续监测 · 基础 HTTP：响应 2xx／3xx 记为成功，不代表解锁或工作台的严格服务规则通过。</p><details v-for="s in point.monitorSamples" :key="s.sample_id" class="measurement-result" open><summary><strong>{{monitorHTTPServices.find(p=>p.probeType===s.probe_type)?.name||s.probe_type}}</strong><span :class="s.success?'good':'bad'">{{s.success?'HTTP 可达':'失败'}}</span></summary><p v-if="!s.success" class="bad">{{s.error_detail||s.error_class||'失败'}}</p><p class="small muted">{{date(s.timestamp)}} · 持续监测</p><p class="small muted break-text">目标：{{s.target}}</p></details></template>
  <template v-else-if="project==='service'"><p class="service-round-counts"><span class="good">通过 {{counts.good}}</span><span class="warn">受限 {{counts.warn}}</span><span class="bad">失败 {{counts.bad}}</span><span v-if="counts.empty" class="muted">未完成 {{counts.empty}}</span><span class="muted">共 {{attempts.length}} 项</span></p><div v-if="attempts.length>8" class="service-result-tools"><input v-model="search" type="search" aria-label="搜索本轮服务结果" placeholder="搜索服务或失败原因"><UiSelect v-model="status" aria-label="筛选本轮服务状态"><option value="all">全部结果</option><option value="good">只看通过</option><option value="warn">只看受限</option><option value="bad">只看失败</option><option value="empty">未完成</option></UiSelect></div></template>
  <details v-for="a in visible" :key="a.attempt_id" class="measurement-result" :open="project==='download'||attempts.length<=8">
    <summary><strong>{{project==='service'?a.rule.name||a.service_id:'下载测速'}}</strong><span :class="project==='download'?downloadTone(a):a.result?outcomeTone(a.result.outcome):'empty'">{{a.result?outcomeLabel(a.result.outcome):stateLabel(a.execution_state)}}</span></summary>
    <dl v-if="project==='download'"><div><dt>平均速度</dt><dd>{{downloadSpeed(a)===null?'—':downloadSpeed(a)?.toFixed(2)+' MiB/s'}}</dd></div><div><dt>读取数据</dt><dd>{{bytes(a.result?.bytes_read)}}</dd></div><div><dt>持续时间</dt><dd>{{a.result?.duration_ns?(a.result.duration_ns/1e9).toFixed(2)+' 秒':'—'}}</dd></div></dl>
    <p v-if="a.result?.summary">{{a.result.summary}}</p><p v-if="a.result?.error_message" class="bad">{{a.result.error_message}}</p><p v-if="a.result?.http_status" class="small muted">HTTP {{a.result.http_status}}</p><p v-if="a.rule.success_criterion" class="small muted">判断依据：{{a.rule.success_criterion}}</p><p v-if="a.rule.target_url" class="small muted break-text">目标：{{a.rule.target_url}}</p>
    <details v-if="a.result?.samples?.length"><summary>下载采样 · {{a.result.samples.length}} 次</summary><div class="table-scroll"><table><thead><tr><th>已持续</th><th>瞬时速度</th><th>累计数据</th></tr></thead><tbody><tr v-for="(s,i) in a.result.samples" :key="i"><td>{{(s.elapsed_ns/1e9).toFixed(1)}} 秒</td><td>{{s.speed_mbps===undefined?'—':(s.speed_mbps/8).toFixed(2)+' MB/s'}}</td><td>{{bytes(s.cumulative_bytes)}}</td></tr></tbody></table></div></details>
    <details v-if="a.result?.details"><summary>服务详细返回</summary><pre>{{JSON.stringify(a.result.details,null,2)}}</pre></details>
  </details>
  <p v-if="!point.monitorSamples?.length&&!filtered.length" class="small muted">没有匹配的结果。</p><footer v-if="pageCount>1" class="result-pagination"><span>{{filtered.length}} 项 · 每页 {{pageSize}} 项</span><button class="button ghost" :disabled="page===1" @click="page--">上一页</button><span>{{page}} / {{pageCount}}</span><button class="button ghost" :disabled="page===pageCount" @click="page++">下一页</button></footer>
</section>
</template>