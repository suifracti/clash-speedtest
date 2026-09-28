<script setup lang="ts">
import { ref, watch } from 'vue'
import UiSelect from '../common/UiSelect.vue'
import { readObservationPreference, saveObservationPreference } from './observationPreference'
import { freezeLatencyWindow, type LatencyWindowMode } from './latencyRequestGuard'

const props = defineProps<{ modelValue: LatencyWindowMode; label: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: LatencyWindowMode] }>()
const choice = ref('6h')
const amount = ref<number | string>(6)
const error = ref('')
const options = [
  { value: '6h', label: '最近 6h' },
  { value: '24h', label: '最近 24h' },
  { value: 'custom-h', label: '自定义 h' },
  { value: 'custom-d', label: '自定义 d' },
]
watch(() => props.modelValue, value => {
  choice.value = value === '6h' || value === '24h' ? value : `custom-${value.slice(-1)}`
  amount.value = Number(value.slice(0, -1))
  error.value = ''
}, { immediate: true })
function select(value: string | number) {
  choice.value = String(value)
  error.value = ''
  if (value === '6h' || value === '24h') emit('update:modelValue', value)
  else amount.value = Number(readObservationPreference(value === 'custom-d' ? 'd' : 'h').slice(0, -1))
}
function apply() {
  const value = `${Number(amount.value)}${choice.value === 'custom-d' ? 'd' : 'h'}` as LatencyWindowMode
  try {
    freezeLatencyWindow(value)
    saveObservationPreference(choice.value === 'custom-d' ? 'd' : 'h', value)
    error.value = ''
    emit('update:modelValue', value)
  } catch {
    error.value = '请输入有效的正数'
  }
}
</script>

<template>
  <span class="window-select">
    <UiSelect :model-value="choice" :aria-label="label" variant="compact" :options="options" @update:model-value="select" />
    <template v-if="choice.startsWith('custom-')">
      <input v-model="amount" type="number" min="0" step="any" :aria-label="choice === 'custom-d' ? '自定义天数' : '自定义小时数'" @keydown.enter.prevent="apply">
      <span>{{ choice === 'custom-d' ? 'd' : 'h' }}</span>
      <button type="button" @click="apply">应用</button>
    </template>
    <span v-if="error" class="window-error" role="alert">{{ error }}</span>
  </span>
</template>

<style scoped>
.window-select { display: inline-flex; align-items: center; flex-wrap: wrap; gap: 6px; }
input { width: 65px; padding: 5px 7px; border: 1px solid var(--border); border-radius: 5px; background: var(--card-bg); color: var(--text-main); }
button { padding: 5px 9px; border: 1px solid var(--border); border-radius: 5px; background: var(--card-bg); color: var(--primary); cursor: pointer; }
.window-error { color: var(--danger); font-size: 11px; }
</style>
