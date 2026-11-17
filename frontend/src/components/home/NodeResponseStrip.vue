<script setup lang="ts">
import {computed,inject} from 'vue'
import type {NodeOption} from '../../domain'
import {homeModelKey} from './homeModel'
import {useWorkspace} from '../../workspace'
import {targets} from '../../domain'
const props=defineProps<{node:NodeOption}>()
const m=inject(homeModelKey)!,w=useWorkspace()
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)).map(t=>({...t,point:m.metricPoint(props.node,t.id),text:m.siteMetric(props.node,t.id),context:m.siteContext(props.node,t.id)})))
</script>
<template>
  <div class="response-strip" aria-label="各站点已保存延迟">
    <div v-for="s in sites" :key="s.id" class="response-stop" :class="s.point?.tone||'empty'" :title="s.context">
      <span class="response-site">{{s.name}}</span><strong>{{s.text}}</strong>
      <span v-if="s.point?.samples?.length" class="response-samples" :aria-label="s.name+' 本轮 '+s.point.samples.length+' 次实际采样'"><i v-for="sample in s.point.samples" :key="sample.id" :class="sample.tone" :title="sample.description"/></span>
      <span v-else class="response-unrecorded" aria-hidden="true"/>
    </div>
  </div>
</template>
<style scoped>
.response-strip{display:flex;align-items:stretch;gap:0;width:100%;position:relative}
.response-stop{display:flex;flex-direction:column;align-items:flex-start;gap:5px;min-width:0;flex:1;position:relative;padding:0 10px 0 0;color:var(--hd-text)}
.response-site{font-size:10px;color:var(--hd-muted);white-space:nowrap}.response-stop strong{font-size:13px;font-weight:550;font-variant-numeric:tabular-nums;white-space:nowrap}
.response-samples{display:flex;gap:2px;width:100%;height:4px;margin-top:2px}.response-samples i{flex:1;min-width:1px;background:var(--hd-accent);border-radius:1px}.response-samples i.bad{background:var(--hd-bad)}.response-samples i.warn{background:var(--hd-warn)}.response-samples i.empty{background:transparent;border:1px solid var(--hd-line)}
.response-unrecorded{height:4px;width:100%;border-bottom:1px dotted var(--hd-line);margin-top:2px}
.response-stop.warn strong{color:var(--hd-warn)}.response-stop.bad strong{color:var(--hd-bad)}.response-stop.empty strong{color:var(--hd-muted);font-weight:400}
</style>
