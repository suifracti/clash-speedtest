import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { MonitorNodeOption } from '../../../types'
import UiSelect from '../../common/UiSelect.vue'
import ServiceCatalogPicker from '../ServiceCatalogPicker.vue'

const nodes: MonitorNodeOption[] = [
  { profileId: 'airport', profileName: '机场', nodeKey: 'jp-1', nodeIdentityKey: 'jp-1', configRevisionKey: 'a', displayName: '日本 01', type: 'trojan', countryCode: 'JP', countryFlag: '🇯🇵' },
  { profileId: 'airport', profileName: '机场', nodeKey: 'jp-2', nodeIdentityKey: 'jp-2', configRevisionKey: 'b', displayName: '日本 02', type: 'trojan', countryCode: 'JP', countryFlag: '🇯🇵' },
  { profileId: 'airport', profileName: '机场', nodeKey: 'us-1', nodeIdentityKey: 'us-1', configRevisionKey: 'c', displayName: '美国 01', type: 'trojan', countryCode: 'US', countryFlag: '🇺🇸' },
]

describe('service target picker', () => {
  it('groups nodes under Chinese region names and selects the entire current scope', async () => {
    const wrapper = mount(ServiceCatalogPicker, { props: { catalog: [], modelValue: [], nodes, selectedNodeKeys: [] } })
    expect(wrapper.find('.target-picker').exists()).toBe(false)
    await wrapper.findAll('.selection-summary')[1]!.trigger('click')
    const regionSelect = wrapper.findAllComponents(UiSelect).find(select => select.props('ariaLabel') === '筛选待测节点地区')!
    expect(regionSelect.props('options')).toEqual(expect.arrayContaining([
      expect.objectContaining({ value: 'JP', label: '日本（2）' }),
      expect.objectContaining({ value: 'US', label: '美国（1）' }),
    ]))
    expect(wrapper.findAll('.node-region-group')).toHaveLength(2)
    expect(wrapper.findAll('.node-choices label')).toHaveLength(0)

    await wrapper.get('.select-range').trigger('click')
    expect(wrapper.emitted('update:selectedNodeKeys')?.[0]?.[0]).toHaveLength(3)
    await wrapper.setProps({ selectedNodeKeys: ['airport\u0000jp-1', 'airport\u0000jp-2', 'airport\u0000us-1'] })
    expect(wrapper.get('.select-range').text()).toContain('取消当前范围选择')
    await wrapper.findAll('.region-expand').find(button => button.text().includes('日本'))!.trigger('click')
    expect(wrapper.findAll('.node-choices label')).toHaveLength(2)
    await wrapper.get('.done-selection').trigger('click')
    expect(wrapper.find('.target-picker').exists()).toBe(false)
    expect(wrapper.findAll('.selection-summary')[1]!.text()).toContain('已选 3 条线路')
    wrapper.unmount()
  })
})
