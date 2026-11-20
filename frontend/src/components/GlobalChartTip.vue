<script setup lang="ts">
import {activeChartTip} from '../chartTip'
</script>

<template>
  <Teleport to="body">
    <div v-if="activeChartTip" class="cst-chart-tip" :class="[`tip-${activeChartTip.kind}`,{below:activeChartTip.below}]" role="tooltip" :style="{left:activeChartTip.x+'px',top:activeChartTip.y+'px'}">
      <template v-if="activeChartTip.kind==='latency-samples'">
        <div class="cst-chart-tip-head"><span>{{activeChartTip.title}}</span><span v-if="activeChartTip.time">{{activeChartTip.time}}</span></div>
        <div v-if="activeChartTip.httpNote" class="response-tip-http">{{activeChartTip.httpNote}}</div>
        <ol class="response-tip-list">
          <li v-for="(sample,index) in activeChartTip.samples" :key="index">
            <span class="tip-seq">{{sample.seq}}</span>
            <span class="tip-val" :class="sample.tone">{{sample.text}}</span>
            <b v-if="sample.badge" class="tip-http">{{sample.badge}}</b>
          </li>
        </ol>
      </template>
      <template v-else-if="activeChartTip.kind==='overview-latency'">
        <div class="cst-chart-tip-head"><span>{{activeChartTip.title}}</span><span v-if="activeChartTip.sub">{{activeChartTip.sub}}</span></div>
        <ul class="ov-tip-list">
          <li v-for="s in activeChartTip.sites" :key="s.name">
            <span class="ov-tip-name">{{s.name}}</span>
            <span class="ov-tip-val" :class="s.tone">{{s.value}}</span>
            <span class="ov-tip-count">{{s.count}}</span>
            <b v-if="s.badge" class="tip-http" :class="s.tone">{{s.badge}}</b>
          </li>
        </ul>
      </template>
      <template v-else-if="activeChartTip.kind==='unlock-detail'">
        <div class="cst-chart-tip-head"><span>{{activeChartTip.title}}</span><span>已保存解锁明细</span></div>
        <div v-for="g in activeChartTip.groups" :key="g.name" class="unlock-tip-group">
          <div class="unlock-tip-ghead"><strong>{{g.name}}</strong><span>{{g.ratio}}</span></div>
          <ul class="unlock-tip-items">
            <li v-for="item in g.items" :key="item.name">
              <div class="unlock-tip-row">
                <span class="unlock-tip-name">{{item.name}}</span>
                <b class="unlock-tip-state" :class="item.tone">{{item.label}}</b>
              </div>
              <small v-if="item.reason" class="unlock-tip-reason">{{item.reason}}</small>
            </li>
          </ul>
        </div>
      </template>
    </div>
  </Teleport>
</template>

<style>
.tip-latency-samples{min-width:196px;max-width:276px;gap:5px}
.tip-overview-latency{min-width:228px;max-width:308px;gap:6px}
.tip-unlock-detail{min-width:236px;max-width:336px;gap:7px}
.response-tip-http{padding:3px 7px;border-radius:6px;background:var(--cst-warn-bg);color:var(--cst-warn);font-size:10.5px;font-weight:600;line-height:1.4}
.response-tip-list,.ov-tip-list,.unlock-tip-items{list-style:none;margin:0;padding:0;display:grid;gap:3px}
.response-tip-list li,.ov-tip-list li{display:flex;align-items:center;gap:6px;font-size:11px;font-variant-numeric:tabular-nums}
.response-tip-list .tip-seq{color:var(--cst-muted);min-width:44px}
.response-tip-list .tip-val,.ov-tip-val{font-weight:650;color:var(--cst-text)}
.response-tip-list .tip-val.good,.ov-tip-val.good,.unlock-tip-state.good{color:var(--cst-good)}
.response-tip-list .tip-val.warn,.ov-tip-val.warn,.unlock-tip-state.warn{color:var(--cst-warn)}
.response-tip-list .tip-val.bad,.ov-tip-val.bad,.unlock-tip-state.bad{color:var(--cst-bad)}
.response-tip-list .tip-val.empty,.ov-tip-val.empty,.unlock-tip-state.empty{color:var(--cst-muted)}
.tip-http{margin-left:auto;padding:0 5px;border-radius:999px;background:var(--cst-warn-bg);color:var(--cst-warn);font-size:9.5px;font-weight:700;white-space:nowrap}
.tip-http.bad{background:var(--cst-bad-bg);color:var(--cst-bad)}
.ov-tip-name{color:var(--cst-muted);min-width:66px;font-weight:500}
.ov-tip-count{color:var(--cst-muted);font-size:10.5px}
.unlock-tip-group{display:grid;gap:4px;padding-top:4px;border-top:1px solid color-mix(in srgb,var(--cst-line) 75%,transparent)}
.unlock-tip-group:first-of-type{padding-top:0;border-top:0}
.unlock-tip-ghead{display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:10.5px;color:var(--cst-muted)}
.unlock-tip-ghead strong{color:var(--cst-text);font-weight:700}
.unlock-tip-items li{display:grid;gap:1px}
.unlock-tip-row{display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:11px}
.unlock-tip-name{font-weight:600;color:var(--cst-text)}
.unlock-tip-state{font-size:10.5px;font-weight:650;white-space:nowrap}
.unlock-tip-reason{font-size:10px;line-height:1.4;color:var(--cst-muted);white-space:normal;overflow-wrap:anywhere}
</style>
