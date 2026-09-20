<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'

export interface UiSelectOption {
  value: string | number
  label: string
  disabled?: boolean
}

const props = withDefaults(defineProps<{
  modelValue: string | number
  options: UiSelectOption[]
  ariaLabel?: string
  disabled?: boolean
  placeholder?: string
  variant?: 'default' | 'scope' | 'toolbar'
}>(), {
  ariaLabel: '选择',
  disabled: false,
  placeholder: '请选择',
  variant: 'default',
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: string | number): void
}>()

const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const menu = ref<HTMLElement | null>(null)
const isOpen = ref(false)
const highlightedIndex = ref(-1)
const menuStyle = ref<Record<string, string>>({})
const menuId = `ui-select-${Math.random().toString(36).slice(2, 10)}`

const selectedIndex = computed(() => props.options.findIndex((option) => sameValue(option.value, props.modelValue)))
const selectedOption = computed(() => selectedIndex.value >= 0 ? props.options[selectedIndex.value] : undefined)
const displayLabel = computed(() => selectedOption.value?.label || props.placeholder)

function sameValue(left: string | number, right: string | number): boolean {
  return Object.is(left, right) || String(left) === String(right)
}

function firstEnabledIndex(start = 0, step = 1): number {
  let index = start
  while (index >= 0 && index < props.options.length) {
    if (!props.options[index].disabled) return index
    index += step
  }
  return -1
}

function moveHighlight(step: number): void {
  if (!props.options.length) return
  const start = highlightedIndex.value < 0
    ? (step > 0 ? 0 : props.options.length - 1)
    : highlightedIndex.value + step
  let index = start
  for (let count = 0; count < props.options.length; count += 1) {
    if (index < 0) index = props.options.length - 1
    if (index >= props.options.length) index = 0
    if (!props.options[index].disabled) {
      highlightedIndex.value = index
      return
    }
    index += step
  }
}

function updateMenuPosition(): void {
  if (!trigger.value || !menu.value) return
  const triggerRect = trigger.value.getBoundingClientRect()
  const menuRect = menu.value.getBoundingClientRect()
  const gap = 6
  const canOpenAbove = triggerRect.top > menuRect.height + gap
  const top = !canOpenAbove && window.innerHeight - triggerRect.bottom >= menuRect.height + gap
    ? triggerRect.bottom + gap
    : Math.max(8, triggerRect.top - menuRect.height - gap)
  menuStyle.value = {
    left: `${Math.max(8, triggerRect.left)}px`,
    top: `${top}px`,
    minWidth: `${Math.max(triggerRect.width, 164)}px`,
    maxWidth: `${Math.max(triggerRect.width, 280)}px`,
  }
}

function addViewportListeners(): void {
  document.addEventListener('pointerdown', onDocumentPointerDown)
  window.addEventListener('resize', updateMenuPosition)
  window.addEventListener('scroll', updateMenuPosition, true)
}

function removeViewportListeners(): void {
  document.removeEventListener('pointerdown', onDocumentPointerDown)
  window.removeEventListener('resize', updateMenuPosition)
  window.removeEventListener('scroll', updateMenuPosition, true)
}

async function openMenu(): Promise<void> {
  if (props.disabled || isOpen.value) return
  highlightedIndex.value = selectedIndex.value >= 0 && !props.options[selectedIndex.value].disabled
    ? selectedIndex.value
    : firstEnabledIndex()
  isOpen.value = true
  addViewportListeners()
  await nextTick()
  updateMenuPosition()
}

function closeMenu(): void {
  if (!isOpen.value) return
  isOpen.value = false
  removeViewportListeners()
}

function toggleMenu(): void {
  if (isOpen.value) closeMenu()
  else void openMenu()
}

function choose(index: number): void {
  const option = props.options[index]
  if (!option || option.disabled) return
  emit('update:modelValue', option.value)
  closeMenu()
  trigger.value?.focus()
}

function onTriggerKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    if (isOpen.value) {
      event.preventDefault()
      closeMenu()
    }
    return
  }
  if (!isOpen.value && ['Enter', ' ', 'ArrowDown', 'ArrowUp'].includes(event.key)) {
    event.preventDefault()
    void openMenu()
    return
  }
  if (!isOpen.value) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    moveHighlight(event.key === 'ArrowDown' ? 1 : -1)
  } else if (event.key === 'Home' || event.key === 'End') {
    event.preventDefault()
    highlightedIndex.value = event.key === 'Home' ? firstEnabledIndex() : firstEnabledIndex(props.options.length - 1, -1)
  } else if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    if (highlightedIndex.value >= 0) choose(highlightedIndex.value)
  } else if (event.key === 'Tab') {
    closeMenu()
  }
}

function onDocumentPointerDown(event: PointerEvent): void {
  const target = event.target as Node | null
  if (target && (root.value?.contains(target) || menu.value?.contains(target))) return
  closeMenu()
}

onBeforeUnmount(removeViewportListeners)
</script>

<template>
  <span ref="root" class="ui-select" :class="`ui-select--${variant}`">
    <button
      ref="trigger"
      type="button"
      class="ui-select__trigger"
      role="combobox"
      :aria-label="ariaLabel"
      :aria-expanded="isOpen"
      :aria-controls="menuId"
      :aria-haspopup="'listbox'"
      :disabled="disabled"
      @click="toggleMenu"
      @keydown="onTriggerKeydown"
    >
      <span class="ui-select__label" :title="displayLabel">{{ displayLabel }}</span>
      <span class="ui-select__chevron" aria-hidden="true">⌄</span>
    </button>

    <Teleport to="body">
      <div
        v-if="isOpen"
        ref="menu"
        :id="menuId"
        class="ui-select__menu"
        :style="menuStyle"
        role="listbox"
        :aria-label="ariaLabel"
      >
        <button
          v-for="(option, index) in options"
          :key="`${String(option.value)}-${index}`"
          type="button"
          role="option"
          class="ui-select__option"
          :class="{ highlighted: highlightedIndex === index, selected: sameValue(option.value, modelValue) }"
          :aria-selected="sameValue(option.value, modelValue)"
          :disabled="option.disabled"
          @mouseenter="highlightedIndex = index"
          @click="choose(index)"
        >
          <span>{{ option.label }}</span>
          <span v-if="sameValue(option.value, modelValue)" class="ui-select__check" aria-hidden="true">✓</span>
        </button>
      </div>
    </Teleport>
  </span>
</template>
