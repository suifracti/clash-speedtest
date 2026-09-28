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
      <div class="custom-window-input-group">
        <input
          v-model="amount"
          type="number"
          min="0"
          step="any"
          class="custom-amount-input"
          :aria-label="choice === 'custom-d' ? '自定义天数' : '自定义小时数'"
          @keydown.enter.prevent="apply"
        >
        <span class="custom-unit-badge">{{ choice === 'custom-d' ? '天' : '小时' }}</span>
        <button type="button" class="custom-apply-btn" @click="apply">应用</button>
      </div>
    </template>
    <span v-if="error" class="window-error" role="alert">{{ error }}</span>
  </span>
</template>

<style scoped>
.window-select { display: inline-flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.custom-window-input-group {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 4px 2px 8px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card-subtle);
  transition: all 0.15s ease;
}
.custom-window-input-group:focus-within {
  border-color: var(--primary);
  background: var(--card-bg);
  box-shadow: 0 0 0 2px var(--primary-subtle, rgba(99, 102, 241, 0.2));
}
.custom-amount-input {
  width: 44px;
  padding: 3px 0;
  border: none;
  background: transparent;
  color: var(--text-main);
  font-size: 12px;
  font-family: var(--font-mono, monospace);
  font-weight: 600;
  outline: none;
  -moz-appearance: textfield;
}
.custom-amount-input::-webkit-outer-spin-button,
.custom-amount-input::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.custom-unit-badge {
  font-size: 11px;
  color: var(--text-secondary);
  font-weight: 550;
  user-select: none;
}
.custom-apply-btn {
  padding: 3px 8px;
  border: 1px solid transparent;
  border-radius: 4px;
  background: var(--primary);
  color: white;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
}
.custom-apply-btn:hover {
  filter: brightness(1.1);
  transform: translateY(-0.5px);
}
.custom-apply-btn:active {
  transform: translateY(0.5px);
}
.window-error { color: var(--danger); font-size: 11px; }
</style>
