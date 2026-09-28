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

  it('allows clicking the whole service card and provides selected services overview with clear all', async () => {
    const catalog = [
      { service_id: 'antigravity', name: '反重力', description: '反重力工具测试', category: 'ai', region: '全球', batch_default: true, enabled: true },
      { service_id: 'chatgpt', name: 'ChatGPT', description: 'OpenAI 官网访问', category: 'ai', region: '全球', batch_default: true, enabled: true },
      { service_id: 'netflix', name: 'Netflix', description: '网飞流媒体', category: 'media', region: '全球', batch_default: true, enabled: true },
    ]
    const wrapper = mount(ServiceCatalogPicker, {
      props: {
        catalog: catalog as any,
        modelValue: ['antigravity', 'chatgpt'],
        nodes,
        selectedNodeKeys: [],
      },
    })

    // Open services editor
    await wrapper.findAll('.selection-summary')[0]!.trigger('click')

    // Category navigation should include "已选服务 (2)"
    const navButtons = wrapper.findAll('nav button')
    const selectedNav = navButtons.find(b => b.text().includes('已选服务'))
    expect(selectedNav).toBeDefined()
    expect(selectedNav!.text()).toContain('2')

    // Selected chips bar should display selected services
    const chips = wrapper.findAll('.selected-chip')
    expect(chips).toHaveLength(2)
    expect(chips[0]!.text()).toContain('反重力')

    // Clicking a chip removes it
    await chips[0]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['chatgpt'])

    // Clicking the clear all button clears selection
    const clearBtn = wrapper.find('.btn-clear-services')
    expect(clearBtn.exists()).toBe(true)
    await clearBtn.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[1]?.[0]).toEqual([])

    // Clicking a whole service card toggles selection
    const cards = wrapper.findAll('.service-cards article')
    expect(cards.length).toBeGreaterThanOrEqual(1)
    await cards[0]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()

    wrapper.unmount()
  })
})
