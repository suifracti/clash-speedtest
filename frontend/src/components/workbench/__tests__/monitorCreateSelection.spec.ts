import { describe, expect, it } from 'vitest'
import type { MonitorNodeOption } from '../../../types'
import { decideMonitorSelection } from '../monitorCreateSelection'

const nodeA: MonitorNodeOption = {
  profileId: 'profile-a',
  profileName: 'Profile A',
  nodeKey: 'node-a',
  nodeIdentityKey: 'identity-a',
  configRevisionKey: 'revision-a',
  displayName: 'Node A',
  type: 'ss',
  countryCode: 'JP',
  countryFlag: '🇯🇵',
}

const nodeA2: MonitorNodeOption = { ...nodeA, nodeKey: 'node-a2', nodeIdentityKey: 'identity-a2', configRevisionKey: 'revision-a2', displayName: 'Node A2' }
const nodeB: MonitorNodeOption = { ...nodeA, profileId: 'profile-b', profileName: 'Profile B', nodeKey: 'node-b', nodeIdentityKey: 'identity-b', configRevisionKey: 'revision-b', displayName: 'Node B' }
const key = (node: MonitorNodeOption) => `${node.profileId}\u0000${node.nodeKey}`

describe('Workbench Monitor creation selection', () => {
  it('requires a stable selection and preserves the single-profile scope', () => {
    expect(decideMonitorSelection([nodeA], []).prefill).toBeNull()

    expect(decideMonitorSelection([nodeA, nodeA2], [key(nodeA), key(nodeA2)])).toEqual({
      reason: '',
      prefill: {
        profileId: 'profile-a',
        nodeKeys: ['node-a', 'node-a2'],
        nodeContexts: [
          { node_key: 'node-a', node_identity_key: 'identity-a', config_revision_key: 'revision-a' },
          { node_key: 'node-a2', node_identity_key: 'identity-a2', config_revision_key: 'revision-a2' },
        ],
      },
    })
  })

  it('rejects mixed profiles instead of merging scopes into one job', () => {
    const result = decideMonitorSelection([nodeA, nodeB], [key(nodeA), key(nodeB)])
    expect(result.prefill).toBeNull()
    expect(result.reason).toContain('一个订阅')
  })

  it('rejects a selection without stable identity or revision', () => {
    const result = decideMonitorSelection([{ ...nodeA, configRevisionKey: '' }], [key(nodeA)])
    expect(result.prefill).toBeNull()
    expect(result.reason).toContain('稳定身份')
  })
})
