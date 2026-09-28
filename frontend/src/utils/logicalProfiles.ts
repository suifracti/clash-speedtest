import type { MonitorNodeOption } from '../types'

export interface LogicalProfileChoice {
  id: string
  name: string
  airportName: string
  count: number
  configCount: number
  profileIds: string[]
  profileNames: string[]
  mergedSourceCount: number
}

interface RawProfile {
  id: string
  name: string
  nodes: MonitorNodeOption[]
  allIdentities: Set<string>
  identities: Set<string>
}

function airportName(profileName: string): string {
  return profileName.split(' · ')[0]?.trim() || profileName
}

function singleSourceDisplayName(profileName: string): string {
  const [source, subscription] = profileName.split(' · ').map(part => part.trim())
  // A single imported source can have a generic parent and a more specific
  // child, e.g. "github · github免费". Keep the parent for merge identity.
  if (source && subscription && subscription.toLocaleLowerCase().startsWith(source.toLocaleLowerCase())) return subscription
  if (source && subscription && /^(默认订阅|默认|订阅)$/.test(subscription)) return source
  return source && subscription ? `${source}（${subscription}）` : profileName
}

function sourcePriority(profile: RawProfile): number {
  if (/默认订阅/.test(profile.name)) return 0
  if (!profile.name.includes(' · ')) return 1
  return 2
}

/**
 * Subscriptions from the same airport are one logical source when their
 * full node identity sets are equal. A notice may be classified differently
 * between two cache revisions; that must not split otherwise identical sources.
 */
export function buildLogicalProfileChoices(
  options: MonitorNodeOption[],
  includeNode: (node: MonitorNodeOption) => boolean = () => true,
): LogicalProfileChoice[] {
  const profiles = new Map<string, RawProfile>()
  const proxyIdentitiesByAirport = new Map<string, Set<string>>()
  for (const node of options) {
    const profile = profiles.get(node.profileId) || { id: node.profileId, name: node.profileName, nodes: [], allIdentities: new Set<string>(), identities: new Set<string>() }
    const identity = node.nodeIdentityKey || node.nodeKey
    profile.allIdentities.add(identity)
    if (includeNode(node)) {
      profile.nodes.push(node)
      const airport = airportName(node.profileName)
      const knownProxies = proxyIdentitiesByAirport.get(airport) || new Set<string>()
      knownProxies.add(identity)
      proxyIdentitiesByAirport.set(airport, knownProxies)
    }
    profiles.set(node.profileId, profile)
  }

  const grouped = new Map<string, RawProfile[]>()
  for (const profile of profiles.values()) {
    if (!profile.nodes.length) continue
    const knownProxies = proxyIdentitiesByAirport.get(airportName(profile.name)) || new Set<string>()
    profile.identities = new Set([...profile.allIdentities].filter(identity => knownProxies.has(identity)))
    const signature = [...profile.identities].sort().join('\u0001')
    const key = `${airportName(profile.name)}\u0000${signature || `empty:${profile.id}`}`
    const group = grouped.get(key) || []
    group.push(profile)
    grouped.set(key, group)
  }

  return Array.from(grouped.values()).map(group => {
    const ordered = [...group].sort((a, b) => sourcePriority(a) - sourcePriority(b) || a.name.localeCompare(b.name, 'zh-CN'))
    const canonical = ordered[0]
    const name = airportName(canonical.name)
    return {
      id: canonical.id,
      name: ordered.length > 1 ? name : singleSourceDisplayName(canonical.name),
      airportName: name,
      count: new Set(group.flatMap(profile => profile.nodes.map(node => node.nodeIdentityKey || node.nodeKey))).size,
      configCount: new Set(group.flatMap(profile => profile.nodes.map(node => `${node.nodeIdentityKey || node.nodeKey}\u0000${node.configRevisionKey || node.nodeKey}`))).size,
      profileIds: ordered.map(profile => profile.id),
      profileNames: ordered.map(profile => profile.name),
      mergedSourceCount: ordered.length,
    }
  }).sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
}

export function logicalChoiceForProfile(choices: LogicalProfileChoice[], profileId: string): LogicalProfileChoice | undefined {
  return choices.find(choice => choice.profileIds.includes(profileId))
}
