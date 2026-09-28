import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { MonitorNodeOption } from '../../../types'
import DownloadComparison from '../DownloadComparison.vue'
import ServiceComparison from '../ServiceComparison.vue'

const node: MonitorNodeOption = {
  profileId: 'airport-a', profileName: '机场 A', nodeKey: 'node-a',
  nodeIdentityKey: 'line-a', configRevisionKey: 'revision-a',
  displayName: '日本 01', type: 'trojan', countryCode: 'JP', countryFlag: '🇯🇵',
}
const rows = [{ key: 'airport-a\u0000node-a', node }]

describe('unmeasured comparison views', () => {
  it('keeps unmeasured download lines out of the result list until the user opens selection', async () => {
    const wrapper = mount(DownloadComparison, { props: { rows, records: {}, selected: [], states: {}, partial: {} } })
    expect(wrapper.find('.download-spotlight').exists()).toBe(false)
    expect(wrapper.findAll('.download-result-row')).toHaveLength(0)
    await wrapper.get('.choose-download-nodes').trigger('click')
    expect(wrapper.findAll('.download-result-row')).toHaveLength(1)
    wrapper.unmount()
  })

  it('shows service results only for measured or selected lines by default', async () => {
    const wrapper = mount(ServiceComparison, { props: { rows, services: [{ value: 'cloudflare_204', label: 'Cloudflare' }], records: {}, selected: [], states: {}, partial: {} } })
    expect(wrapper.findAll('.overview-row')).toHaveLength(0)
    await wrapper.get('input[type="checkbox"]').setValue(true)
    expect(wrapper.findAll('.overview-row')).toHaveLength(1)
    wrapper.unmount()
  })
})
