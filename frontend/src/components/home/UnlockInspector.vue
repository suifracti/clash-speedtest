<script setup lang="ts">
import {computed,inject} from 'vue'
import {useWorkspace} from '../../workspace'
import {date,isReputationRule,key,type NodeOption} from '../../domain'
import {attemptObservedAt} from '../../nodeQuality'
import {unlockGroups,unlockGroup,unlockResult,unlockGroupSkipReason} from '../../unlock'
import {homeModelKey} from './homeModel'
import RecordedBars from '../RecordedBars.vue'
const props=defineProps<{node:NodeOption}>(),w=useWorkspace(),m=inject(homeModelKey)!
const items=computed(()=>{
 const records=[...(m.qualityData.value[key(props.node)]?.services||[]),...(w.services.value[key(props.node)]||[])],names=new Map(w.catalog.value.filter(s=>!isReputationRule(s)).map(s=>[s.service_id,s.name]))
 for(const a of records)if(a.service_id&&!isReputationRule({service_id:a.service_id})&&!names.has(a.service_id))names.set(a.service_id,a.rule.name||a.service_id)
 return [...names].map(([id,name])=>{const attempt=m.shownService(props.node,id);return {id,name,attempt,result:unlockResult(attempt),at:attempt?date(attemptObservedAt(attempt)):''}}).sort((a,b)=>(a.result.state==='unavailable'?0:1)-(b.result.state==='unavailable'?0:1))
})
const groups=computed(()=>[...unlockGroups,{id:'legacy',name:'其他（旧检测项）'}].map(g=>({...g,skip:unlockGroupSkipReason(g.id,w.nodeUnlockRegion(props.node),w.japanOnly.value),items:items.value.filter(s=>unlockGroup(s.id)===g.id)})).filter(g=>g.items.length))
function records(id:string){return m.servicePoints(props.node,id).filter(p=>!p.id.startsWith('planned:')&&p.attempts?.some(a=>(a.service_id||a.rule.service_id)===id&&!!a.result)).slice(-12)}
</script>
<template><div class="unlock-inspector" aria-label="解锁检测明细">
 <template v-for="group in groups" :key="group.id"><p v-if="group.skip" class="unlock-skipped">{{group.name}} · {{group.skip}}</p><component v-else :is="group.id==='legacy'?'details':'section'" class="unlock-group" :class="{legacy:group.id==='legacy'}">
  <component :is="group.id==='legacy'?'summary':'h4'">{{group.name}}<small>{{group.items.some(s=>s.result.state!=='unknown')?group.items.filter(s=>s.result.state==='available').length+'/'+group.items.filter(s=>s.result.state!=='unknown').length+' 可用':'未测'}}</small></component>
  <div v-for="s in group.items" :key="s.id" class="hd-service-row unlock-row">
   <button class="hd-site-name unlock-name" @click="m.showService(node,s.id)">{{s.name}}</button>
   <button class="hd-site-value unlock-state" :class="s.result.tone" :title="s.result.reason" @click="m.showService(node,s.id)">{{s.result.label}}<small v-if="s.attempt?.result&&(s.id==='netflix_unlock'||s.id==='netflix_unlock_ipv4')">未验证账号播放</small></button>
   <p v-if="s.result.reason&&s.result.reason!=='未验证账号播放'&&s.result.state!=='available'" class="unlock-reason">{{s.result.reason}}</p>
   <div class="unlock-meta"><time v-if="s.at">{{s.at}}</time><RecordedBars :service-id="s.id" :points="records(s.id)" :label="s.name+' 实际检测记录'"/></div>
  </div>
 </component></template><p v-if="!groups.length" class="hd-muted">还没有解锁检测记录。</p>
</div></template>
<style scoped>
.unlock-skipped{margin:12px 0;color:var(--hd-muted);font-size:12px}.unlock-state small{display:block;font-size:9px;color:var(--hd-muted);font-weight:400;margin-top:2px}.unlock-state.bad{color:#c6343e}
.unlock-group+ .unlock-group{margin-top:14px}.unlock-group h4,.unlock-group summary{margin:0 0 6px;font-size:12px;font-weight:600}.unlock-group summary{cursor:pointer}.unlock-group h4 small,.unlock-group summary small{margin-left:8px;font-size:10px;color:var(--hd-muted);font-weight:400}.unlock-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(70px,auto) minmax(55px,115px);align-items:center;gap:5px 8px;padding:9px 0;border-top:1px solid var(--hd-line)}.unlock-name.hd-site-name{font-size:12px;font-weight:500;line-height:1.5;white-space:normal;overflow:visible;text-overflow:clip}.unlock-state.hd-site-value{font-size:11px;font-weight:500;text-align:left;white-space:normal;line-height:1.4;padding:3px 6px;border-radius:4px;background:var(--hd-bg)}.unlock-reason{grid-row:2;grid-column:1/-1;font-size:11px;line-height:1.5;margin:0;color:var(--hd-muted)}.unlock-state.good{color:#16864c}.unlock-state.warn{color:#a7670b}.unlock-state.empty{color:var(--hd-muted)}.unlock-meta{grid-column:3;grid-row:1;font-size:9px;color:var(--hd-muted);display:flex;flex-direction:column;align-items:flex-end;gap:4px;min-width:0}.unlock-meta :deep(.recorded-bars){max-width:115px}.legacy{color:var(--hd-muted)}
</style>
