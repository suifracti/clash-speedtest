<script setup lang="ts">
import {computed} from 'vue'
import {date} from '../domain'
import type {SampleGroup,TrendPoint} from '../presentation'
const props=defineProps<{point:TrendPoint}>()
const groups=computed(()=>props.point.sampleGroups||[])
const samples=computed(()=>groups.value.flatMap(g=>g.samples))
const success=computed(()=>samples.value.filter(s=>s.tone==='good').length)
function failures(group:SampleGroup){const errors=new Map<string,string[]>();for(const sample of group.samples){if(!sample.error)continue;const labels=errors.get(sample.error)||[];labels.push(sample.label);errors.set(sample.error,labels)}return Array.from(errors,([error,labels])=>({error,labels:labels.join('、')}))}
</script>
<template>
  <section class="round-results" aria-label="本轮测试数据">
    <header><strong>{{point.source==='monitor'?'本轮已读采样':'本轮全部测试'}}</strong><time :datetime="point.time" :title="point.time">{{date(point.time)}}</time></header>
    <p class="round-summary">{{groups.length}} 个站点 · {{samples.length}} 次采样 · {{success}}/{{samples.length}} 成功</p>
    <div class="round-target-list">
      <article v-for="group in groups" :key="group.id" class="round-target">
        <header><strong :title="group.target">{{group.name}}</strong><span>中位数 <b>{{group.value===null?'—':group.value.toFixed(0)+' ms'}}</b></span><small>{{group.samples.filter(s=>s.tone==='good').length}}/{{group.samples.length}} 成功</small></header>
        <div class="round-result-grid"><div v-for="sample in group.samples" :key="sample.id" class="round-result" :class="sample.tone" :title="sample.description"><span>{{sample.label}}</span><strong>{{sample.value===null?'失败':sample.value.toFixed(0)+' ms'}}</strong></div></div>
        <p v-for="failure in failures(group)" :key="failure.error" class="round-failure bad">{{failure.labels}}：{{failure.error}}</p>
      </article>
    </div>
  </section>
</template>
