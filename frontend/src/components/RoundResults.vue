<script setup lang="ts">
import {computed} from 'vue'
import {triggerLabel} from '../measurementRounds'
import {date} from '../domain'
import type {SampleGroup,TrendPoint} from '../presentation'
const props=defineProps<{point:TrendPoint}>()
const groups=computed(()=>props.point.sampleGroups||[])
const samples=computed(()=>groups.value.flatMap(g=>g.samples))
const success=computed(()=>samples.value.filter(s=>s.value!==null).length)
const executedSites=computed(()=>groups.value.filter(g=>g.samples.some(s=>s.tone!=='empty'||s.executionState==='cancelled')).length)
const cancelled=computed(()=>samples.value.filter(s=>s.executionState==='cancelled').length)
function failures(group:SampleGroup){const errors=new Map<string,string[]>();for(const sample of group.samples){if(!sample.error)continue;const labels=errors.get(sample.error)||[];labels.push(sample.label);errors.set(sample.error,labels)}return Array.from(errors,([error,labels])=>({error,labels:labels.join('、')}))}
</script>
<template>
  <section class="round-results" aria-label="本轮测试数据">
    <header><strong>{{point.source==='monitor'?'本轮已读采样':'本轮全部测试'}}</strong><time :datetime="point.time" :title="point.time">{{date(point.time)}} · {{triggerLabel(point.trigger)}}</time></header>
    <p class="round-summary">本节点本轮已读 {{groups.length}} / 6 个默认站点记录 · 实际执行 {{executedSites}} / 6 站点 · {{samples.length}} 次采样 · {{success}}/{{samples.length}} 成功<span v-if="cancelled"> · 取消 {{cancelled}}</span></p>
    <p class="small muted">这是本节点的已读覆盖，不代表全局覆盖；旧单站记录保持原范围。响应时延或 HTTP 成功不证明业务／地区解锁。</p>
    <details v-for="test in point.latencyTests||[]" :key="test.attempt_id" class="small"><summary>方法与出口证据 · {{test.network_path?.interface||'未记录'}} · {{test.attempt_id}}</summary><p>{{test.source||'来源未知'}} · {{test.method||'方法未知'}} / v{{test.method_version||'?'}} · {{test.target||'目标见子样本'}}</p><p>A 记录优先；只有 A 查询明确返回 NOERROR 且无 A 记录时才查询 AAAA。地址族选定后绑定并核验物理接口，不跨地址族重试；socket 绑定核验不代表已证明绕过 TUN。旧结果不补造 DNS 或出口证据。</p><pre>{{JSON.stringify(test.network_path||{evidence:'旧结果无出口证据'},null,2)}}</pre></details>
    <div class="round-target-list">
      <article v-for="group in groups" :key="group.id" class="round-target">
        <header><strong :title="group.target">{{group.name}}</strong><span>中位数 <b>{{group.value===null?'—':group.value.toFixed(0)+' ms'}}</b></span><small>{{group.samples.filter(s=>s.value!==null).length}}/{{group.samples.length}} 成功</small></header>
        <div class="round-result-grid"><div v-for="sample in group.samples" :key="sample.id" class="round-result" :class="sample.tone" :title="sample.description"><span>{{sample.label}}</span><strong>{{sample.value===null?(sample.executionState==='cancelled'?'已取消':sample.tone==='empty'?'未执行':'未确认'):sample.value.toFixed(0)+' ms'}}</strong></div></div>
        <p v-for="sample in group.samples.filter(s=>s.note)" :key="sample.id+':note'" class="small warn">{{sample.label}}：{{sample.note}}</p>
        <p v-for="failure in failures(group)" :key="failure.error" class="round-failure" :class="group.samples.every(s=>s.executionState==='cancelled')?'muted':'bad'">{{failure.labels}}：{{failure.error}}</p>
      </article>
    </div>
  </section>
</template>
