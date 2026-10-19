<script setup lang="ts">
import {inject,onMounted,ref} from 'vue'
import {useWorkspace} from '../../workspace'
import {date,key} from '../../domain'
import {nodeDisplayName} from '../../nodePresentation'
import QualityMetrics from '../QualityMetrics.vue'
import Icon from '../Icon.vue'
import {homeModelKey} from './homeModel'
const w=useWorkspace(),m=inject(homeModelKey)!
const rows=[{id:'distribution',label:'延迟分布'},{id:'response',label:'成功率 / 样本'},{id:'download',label:'下载'},{id:'service',label:'服务结论'},{id:'time',label:'测量时间'},{id:'details',label:'完整证据'}] as const
const root=ref<HTMLElement>()
function history(n:typeof m.compareNodes.value[number]){m.closeComparison();w.inspectNode.value=n}
onMounted(()=>root.value?.querySelector<HTMLElement>('.hd-compare-head button')?.focus({preventScroll:true}))
</script>
<template>
  <aside ref="root" class="hd-compare" aria-label="节点比较" @keydown.esc.stop="m.closeComparison()">
    <header class="hd-compare-head">
      <div><h2>比较 {{m.compareNodes.value.length}} 个已选节点</h2><p class="hd-muted quality-compare-meta">截止 {{date(m.qualityCutoff.value)}} · 只查看，不改变检测范围和勾选</p></div>
      <button class="hd-btn" type="button" @click="m.closeComparison()"><Icon name="close"/>退出比较</button>
    </header>
    <p class="quality-compare-status hd-compare-verdict" :class="m.compareCompatible.value?'good':'warn'">{{m.compareCompatible.value?'延迟最新组条件一致，可比较描述统计。':'延迟不可比较：条件不同、样本不足、未测或读取不完整。'}}</p>
    <p class="comparison-download-status hd-muted">{{m.comparisonDownloadStatus.value}}</p>
    <details class="quality-compare-boundaries hd-compare-rules"><summary>比较口径与边界</summary><p>延迟按所选站点的最新条件组展示；P95 至少 20 个成功样本，少于 5 个成功样本不排名，缺少出口或地址族/来源证据时只展示原值。下载仅同源、方法、上限、结束类型及已记录的出口地址条件相同时可比较，缺少必要出口证据时不做数值排序；历史有效速度不代表最近检测成功。服务逐项核对同一规则、方法、地址条件和时间，HTTP响应不等于业务可用。完整依据可在各节点的证据行展开。</p></details>
    <p v-if="m.compareNodes.value.length>2" id="comparison-scroll-hint" class="comparison-scroll-hint hd-muted">← 超出视野时横向滚动；左侧指标固定。聚焦表格可用左右方向键。 →</p>
    <div class="quality-compare hd-compare-table" role="region" aria-label="节点逐项比较表" tabindex="0" :aria-describedby="m.compareNodes.value.length>2?'comparison-scroll-hint':undefined" :style="{'--compare-count':m.compareNodes.value.length}">
      <table><colgroup><col class="comparison-label-column"><col v-for="n in m.compareNodes.value" :key="key(n)"></colgroup>
        <thead><tr><th scope="col">指标</th><th v-for="n in m.compareNodes.value" :key="key(n)" scope="col"><strong>{{nodeDisplayName(n)}}</strong><small>{{m.sourceLabel(n)}}</small><small>{{n.type}}</small></th></tr></thead>
        <tbody><tr v-for="row in rows" :key="row.id"><th scope="row">{{row.label}}</th><td v-for="n in m.compareNodes.value" :key="key(n)"><QualityMetrics :comparison-field="row.id" :evidence="m.qualityData.value[key(n)]" :error="m.qualityErrors.value[key(n)]" :target="w.displayTarget.value" :catalog="w.catalog.value"/></td></tr>
        <tr><th scope="row">原始历史</th><td v-for="n in m.compareNodes.value" :key="key(n)"><button class="hd-btn" @click="history(n)">查看原始历史</button></td></tr></tbody>
      </table>
    </div>
  </aside>
</template>
