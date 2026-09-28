import { mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import ObservationWindowSelect from '../ObservationWindowSelect.vue'
import UiSelect from '../../common/UiSelect.vue'
import { readObservationPreference, saveObservationPreference } from '../observationPreference'

afterEach(() => vi.unstubAllGlobals())
it('applies custom windows explicitly and rejects invalid values without changing the range', async () => {
  const saved = new Map<string, string>()
  vi.stubGlobal('localStorage', { getItem: (key: string) => saved.get(key) ?? null, setItem: (key: string, value: string) => saved.set(key, value), clear: () => saved.clear() })
  const wrapper = mount(ObservationWindowSelect, { props: { modelValue: '6h', label: '观察窗口' } })
  wrapper.findComponent(UiSelect).vm.$emit('update:modelValue', 'custom-h')
  await wrapper.vm.$nextTick()
  await wrapper.get('input').setValue('1.5')
  expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  await wrapper.get('button').trigger('click')
  expect(wrapper.emitted('update:modelValue')).toEqual([['1.5h']])
  wrapper.findComponent(UiSelect).vm.$emit('update:modelValue', 'custom-d')
  await wrapper.vm.$nextTick()
  await wrapper.get('input').setValue('2')
  await wrapper.get('input').trigger('keydown.enter')
  expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['2d'])
  await wrapper.get('input').setValue('0')
  await wrapper.get('button').trigger('click')
  expect(wrapper.get('[role="alert"]').text()).toContain('正数')
  expect(wrapper.emitted('update:modelValue')).toHaveLength(2)
  wrapper.unmount()
  saveObservationPreference('selected', '1.5h')
  const reopened = mount(ObservationWindowSelect, { props: { modelValue: readObservationPreference('selected'), label: '观察窗口' } })
  expect((reopened.get('input').element as HTMLInputElement).value).toBe('1.5')
  reopened.findComponent(UiSelect).vm.$emit('update:modelValue', 'custom-d')
  await reopened.vm.$nextTick()
  expect((reopened.get('input').element as HTMLInputElement).value).toBe('2')
  reopened.findComponent(UiSelect).vm.$emit('update:modelValue', 'custom-h')
  await reopened.vm.$nextTick()
  expect((reopened.get('input').element as HTMLInputElement).value).toBe('1.5')
  localStorage.clear()
})
