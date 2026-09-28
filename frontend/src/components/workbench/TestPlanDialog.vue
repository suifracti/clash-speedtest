<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
export interface TestPlan { title: string; nodes: number; rounds: number; downloadMiB: number; services: string[]; antigravity: boolean; repeatSummary?: string }
defineProps<{ plan: TestPlan }>()
const emit = defineEmits<{ (e: 'answer', value: boolean): void }>()
const dialog = ref<HTMLDialogElement>()
let previousFocus: HTMLElement | null = null
onMounted(() => { previousFocus = document.activeElement as HTMLElement; dialog.value?.showModal() })
onBeforeUnmount(() => previousFocus?.focus())
function amount(mib: number) { return mib >= 1024 ? `${(mib / 1024).toFixed(2)} GiB` : `${mib.toLocaleString()} MiB` }
</script>
<template>
  <Teleport to="body"><dialog ref="dialog" class="test-plan-dialog" aria-labelledby="test-plan-title" @cancel.prevent="emit('answer', false)">
    <header><span>开始前确认</span><button aria-label="关闭测试确认" @click="emit('answer', false)">×</button></header>
    <h2 id="test-plan-title">{{ plan.title }}</h2><p>只测试下面确认的范围，不会自动追加节点。</p>
    <div class="plan-volume" :class="{ download: plan.downloadMiB > 0 }"><span>{{ plan.downloadMiB > 0 ? '本次最多读取的下载数据（估算）' : '本次不做大文件下载' }}</span><strong>{{ plan.downloadMiB > 0 ? amount(plan.downloadMiB) : '轻量服务检查' }}</strong><small v-if="plan.downloadMiB > 0">{{ plan.nodes }} 个节点 × {{ plan.rounds }} 次 × 每次最多 {{ amount(plan.downloadMiB / plan.nodes / plan.rounds) }}</small></div>
    <dl><div><dt>测试节点</dt><dd>{{ plan.nodes }} 个独立配置</dd></div><div><dt>重复次数</dt><dd>{{ plan.repeatSummary || `${plan.rounds} 次` }}</dd></div><div><dt>服务</dt><dd>{{ plan.services.join('、') || '仅下载速度' }}</dd></div></dl>
    <p class="plan-caution">{{ plan.downloadMiB > 0 ? '上方是应用计划最多读取的数据量，不是机场实际扣费预估。测试可能提前结束；协议开销、缓冲和机场倍率还可能增加计费。' : '检查仍会产生网络流量；读取网页不等于验证账号全部功能。' }}</p>
    <p v-if="plan.antigravity" class="plan-account">Antigravity 将使用已绑定账号发起真实模型请求，可能消耗账号额度。成功仅代表这次模型回答，不保证以后一直可用。</p>
    <footer><button autofocus @click="emit('answer', false)">返回调整</button><button class="confirm-test" @click="emit('answer', true)">确认并开始测试</button></footer>
  </dialog></Teleport>
</template>
<style scoped>
.test-plan-dialog{width:min(540px,calc(100vw - 32px));max-height:90vh;overflow:auto;border:1px solid var(--border);border-radius:20px;padding:28px;color:var(--text-main);background:var(--card-bg);box-shadow:0 28px 90px #0d19274d}.test-plan-dialog::backdrop{background:#10203480;backdrop-filter:blur(4px)}header{display:flex;justify-content:space-between;color:var(--text-secondary);font-size:12px}header button{font-size:24px;line-height:1}h2{font-size:25px;font-weight:750;margin:12px 0}p{font-size:13px;color:var(--text-secondary);line-height:1.7}dl{margin:20px 0}dl div{display:grid;grid-template-columns:84px 1fr;gap:12px;padding:9px 0;font-size:13px}dt{color:var(--text-secondary)}dd{margin:0;overflow-wrap:anywhere}.plan-volume{background:var(--primary-subtle);padding:23px;border-radius:12px;margin:20px 0}.plan-volume.download{background:var(--warning-bg);color:var(--warning)}.plan-volume span,.plan-volume small{display:block;font-size:12px}.plan-volume strong{display:block;font-size:34px;font-weight:800;margin:5px 0}.plan-caution{padding-left:12px;border-left:3px solid var(--warning)}.plan-account{background:var(--warning-bg);padding:12px;border-radius:8px;color:var(--warning)}footer{display:flex;justify-content:flex-end;gap:12px;margin-top:25px}footer button{padding:10px 15px;border-radius:8px;border:1px solid var(--border);font-size:13px}.confirm-test{background:var(--primary);color:white;border-color:var(--primary)}
</style>
