import { describe, expect, it } from 'vitest'
import type { MonitorNodeOption } from '../../types'
import { buildLogicalProfileChoices } from '../logicalProfiles'

function node(profileId: string, profileName: string, nodeKey: string, identity: string, revision: string, displayName = nodeKey): MonitorNodeOption {
  return {
    profileId,
    profileName,
    nodeKey,
    nodeIdentityKey: identity,
    configRevisionKey: revision,
    displayName,
    type: 'hysteria2',
    countryCode: 'US',
    countryFlag: '🇺🇸',
  }
}

describe('logical subscription sources', () => {
  it('merges subscriptions from one airport with the same node identities and keeps a different package independent', () => {
    const choices = buildLogicalProfileChoices([
      node('default', '飞鸟云 · 默认订阅', 'a1', 'identity-1', 'revision-default'),
      node('default', '飞鸟云 · 默认订阅', 'a2', 'identity-2', 'revision-default'),
      node('second', '飞鸟云 · 2', 'b1', 'identity-1', 'revision-second'),
      node('second', '飞鸟云 · 2', 'b2', 'identity-2', 'revision-second'),
      node('package', '飞鸟云 · 高级套餐', 'c1', 'identity-3', 'revision-package'),
      node('other', '另一机场', 'd1', 'identity-1', 'revision-other'),
    ])

    expect(choices).toHaveLength(3)
    expect(choices.find(choice => choice.id === 'default')).toMatchObject({
      name: '飞鸟云',
      count: 2,
      configCount: 4,
      profileIds: ['default', 'second'],
      mergedSourceCount: 2,
    })
    expect(choices.find(choice => choice.id === 'package')).toMatchObject({ name: '飞鸟云（高级套餐）', mergedSourceCount: 1 })
    expect(choices.find(choice => choice.id === 'other')).toMatchObject({ name: '另一机场', mergedSourceCount: 1 })
  })

  it('does not split identical subscriptions when a notice is classified differently', () => {
    const same = [
      node('default', '飞鸟云 · 默认订阅', 'a1', 'identity-1', 'old'),
      node('default', '飞鸟云 · 默认订阅', 'a2', 'identity-2', 'old'),
      node('second', '飞鸟云 · 2', 'b1', 'identity-1', 'new'),
      node('second', '飞鸟云 · 2', 'b2', 'identity-2', 'new'),
    ]
    const choices = buildLogicalProfileChoices(same, item => item.nodeKey !== 'a2')
    expect(choices).toHaveLength(1)
    expect(choices[0]).toMatchObject({ name: '飞鸟云', count: 2, profileIds: ['default', 'second'] })
  })

  it('ignores subscription-only announcements when comparing actual proxy nodes', () => {
    const choices = buildLogicalProfileChoices([
      node('default', '飞鸟云 · 默认订阅', 'proxy-a', 'identity-1', 'old'),
      node('default', '飞鸟云 · 默认订阅', 'notice', 'notice-only', 'old', '电报群 https://t.me/example'),
      node('second', '飞鸟云 · 2', 'proxy-b', 'identity-1', 'new'),
    ], item => item.nodeKey !== 'notice')
    expect(choices).toHaveLength(1)
    expect(choices[0]).toMatchObject({ name: '飞鸟云', count: 1, profileIds: ['default', 'second'] })
  })

  it('uses the specific source name instead of repeating a generic parent name', () => {
    const choices = buildLogicalProfileChoices([node('github', 'github · github免费', 'a1', 'identity-1', 'revision')])
    expect(choices[0]?.name).toBe('github免费')
  })
})
