import type { MonitorJobPrefill, MonitorNodeOption } from '../../types'

export interface MonitorSelectionDecision {
  prefill: MonitorJobPrefill | null
  reason: string
}

/**
 * Converts the current Workbench selection into one safe, single-profile
 * Monitor creation context. It never carries raw node configuration.
 */
export function decideMonitorSelection(options: MonitorNodeOption[], selectedKeys: string[]): MonitorSelectionDecision {
  if (selectedKeys.length === 0) {
    return { prefill: null, reason: '先选择至少一个节点，再加入持续监测。' }
  }

  const byKey = new Map(options.map((option) => [`${option.profileId}\u0000${option.nodeKey}`, option]))
  const selected = selectedKeys.map((key) => byKey.get(key)).filter((option): option is MonitorNodeOption => !!option)
  if (selected.length !== selectedKeys.length) {
    return { prefill: null, reason: '部分节点已不在当前缓存中，请重新读取后选择。' }
  }

  const profileId = selected[0].profileId
  if (selected.some((option) => option.profileId !== profileId)) {
    return { prefill: null, reason: '一次 Monitor 任务只能属于一个订阅，请只选择同一订阅的节点。' }
  }
  if (selected.some((option) => !option.nodeIdentityKey || !option.configRevisionKey)) {
    return { prefill: null, reason: '所选节点缺少稳定身份或配置 revision，请重新读取后选择。' }
  }

  return {
    reason: '',
    prefill: {
      profileId,
      nodeKeys: selected.map((option) => option.nodeKey),
      nodeContexts: selected.map((option) => ({
        node_key: option.nodeKey,
        node_identity_key: option.nodeIdentityKey,
        config_revision_key: option.configRevisionKey,
      })),
    },
  }
}
