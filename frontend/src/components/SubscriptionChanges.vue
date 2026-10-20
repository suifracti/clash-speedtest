<script setup lang="ts">
import {ref} from 'vue'
import {request} from '../api'
import type {NodeChanges,Subscription} from '../domain'
import Modal from './Modal.vue'
const props=defineProps<{airportId:string;subscription:Subscription}>()
const opened=ref(false),loading=ref(false),error=ref(''),changes=ref<NodeChanges|null>(null)
async function open(){opened.value=true;loading.value=true;error.value='';changes.value=null;try{changes.value=await request<NodeChanges|null>(`/api/airports/${props.airportId}/subscriptions/${props.subscription.id}/changes`,'GET',undefined,'GetSubscriptionNodeChanges',[props.airportId,props.subscription.id])}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}}
</script>
<template>
  <button v-if="subscription.node_changes" class="text-button small" @click="open">新增 {{subscription.node_changes.added.length}} / 下线 {{subscription.node_changes.removed.length}} / 改名 {{subscription.node_changes.renamed.length}}</button>
  <Modal v-if="opened" :title="subscription.name+' · 节点变化'" @close="opened=false">
    <p v-if="loading">读取变化中…</p><p v-else-if="error" class="inline-error" role="alert">{{error}}</p>
    <template v-else-if="changes"><p class="muted small">与上一次成功更新的节点列表对比。</p>
      <h3>新增 {{changes.added.length}}</h3><ul v-if="changes.added.length"><li v-for="(name,i) in changes.added" :key="i">{{name}}</li></ul>
      <h3>下线 {{changes.removed.length}}</h3><ul v-if="changes.removed.length"><li v-for="(name,i) in changes.removed" :key="i">{{name}}</li></ul>
      <h3>改名 {{changes.renamed.length}}</h3><ul v-if="changes.renamed.length"><li v-for="(item,i) in changes.renamed" :key="i">{{item.from}} → {{item.to}}</li></ul>
    </template><p v-else class="muted">尚无上一版节点列表，下次更新后可对比。</p>
  </Modal>
</template>
