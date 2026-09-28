<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import * as bridgeApi from '../../api/bridge'
import type { MonitorNodeOption, WorkbenchPublicServiceRule } from '../../types'
import type { LogicalProfileChoice } from '../../utils/logicalProfiles'
import { serviceEvidence, serviceGroup, serviceTitle } from '../../utils/servicePresentation'
import UiSelect from '../common/UiSelect.vue'
const props = withDefaults(defineProps<{ catalog: WorkbenchPublicServiceRule[]; modelValue: string[]; disabled?: boolean; showTargets?: boolean; sources?: LogicalProfileChoice[]; selectedSourceIds?: string[]; nodes?: MonitorNodeOption[]; selectedNodeKeys?: string[]; nodeRegion?: string; nodeSearch?: string; showAlternateConfigs?: boolean; hiddenAlternateCount?: number; repeatCount?: number; timeoutSeconds?: number }>(), { showTargets: true, sources: () => [], selectedSourceIds: () => [], nodes: () => [], selectedNodeKeys: () => [], nodeRegion: '全部地区', nodeSearch: '', showAlternateConfigs: false, hiddenAlternateCount: 0, repeatCount: 1, timeoutSeconds: 10 })
const emit = defineEmits<{ (e: 'update:modelValue', ids: string[]): void; (e: 'update:selectedSourceIds', ids: string[]): void; (e: 'update:selectedNodeKeys', keys: string[]): void; (e: 'update:nodeRegion', value: string): void; (e: 'update:nodeSearch', value: string): void; (e: 'update:showAlternateConfigs', value: boolean): void; (e: 'update:repeatCount', value: number): void; (e: 'update:timeoutSeconds', value: number): void; (e: 'run'): void }>()
const editor = ref<'services' | 'nodes' | null>(null)
function edit(section: 'services' | 'nodes') { editor.value = editor.value === section ? null : section }
const group = ref('推荐'), region = ref('全部地区'), search = ref('')
const nodeKey = (node: MonitorNodeOption) => `${node.profileId}\u0000${node.nodeKey}`
const countryNames: Record<string, string> = { JP: '日本', US: '美国', HK: '香港', TW: '台湾', SG: '新加坡', KR: '韩国', CN: '中国大陆', GB: '英国', DE: '德国', FR: '法国', CA: '加拿大', AU: '澳大利亚', NL: '荷兰', IN: '印度', RU: '俄罗斯', VN: '越南', OTHER: '其他地区' }
const countryCode = (node: MonitorNodeOption) => node.countryCode?.trim().toUpperCase() || 'OTHER'
const countryName = (code: string) => countryNames[code] || code
const sourceNodes = computed(() => props.nodes.filter(node => !props.selectedSourceIds.length || props.selectedSourceIds.includes(node.profileId)))
const nodeRegions = computed(() => {
  const counts = new Map<string, number>()
  for (const node of sourceNodes.value) counts.set(countryCode(node), (counts.get(countryCode(node)) || 0) + 1)
  return [{ value: '全部地区', label: `全部地区（${sourceNodes.value.length}）` }, ...[...counts].sort(([a], [b]) => countryName(a).localeCompare(countryName(b), 'zh-CN')).map(([value, count]) => ({ value, label: `${countryName(value)}（${count}）` }))]
})
const scopedNodes = computed(() => sourceNodes.value.filter(node =>
  (props.nodeRegion === '全部地区' || countryCode(node) === props.nodeRegion)
  && (!props.nodeSearch.trim() || `${node.displayName} ${node.profileName}`.toLowerCase().includes(props.nodeSearch.trim().toLowerCase()))))
const selectedNodeCount = computed(() => props.selectedNodeKeys.filter(key => props.nodes.some(node => nodeKey(node) === key)).length)
const selectedScopedCount = computed(() => scopedNodes.value.filter(node => props.selectedNodeKeys.includes(nodeKey(node))).length)
const allScopedSelected = computed(() => scopedNodes.value.length > 0 && selectedScopedCount.value === scopedNodes.value.length)
const expandedRegions = ref<string[]>([])
const nodeGroups = computed(() => {
  const groups = new Map<string, MonitorNodeOption[]>()
  for (const node of scopedNodes.value) {
    const code = countryCode(node)
    const entries = groups.get(code) || []
    entries.push(node)
    groups.set(code, entries)
  }
  return [...groups].sort(([a], [b]) => countryName(a).localeCompare(countryName(b), 'zh-CN')).map(([code, nodes]) => ({ code, name: countryName(code), nodes, selected: nodes.filter(node => props.selectedNodeKeys.includes(nodeKey(node))).length }))
})
watch(() => [props.nodeRegion, props.nodeSearch], () => {
  expandedRegions.value = props.nodeRegion !== '全部地区' || props.nodeSearch.trim() ? nodeGroups.value.map(area => area.code) : []
}, { immediate: true })
function regionExpanded(code: string) { return expandedRegions.value.includes(code) }
function toggleRegion(code: string) { expandedRegions.value = expandedRegions.value.includes(code) ? expandedRegions.value.filter(item => item !== code) : [...expandedRegions.value, code] }
function toggleSource(source: LogicalProfileChoice) {
  if (props.disabled) return
  const current = props.selectedSourceIds.length ? props.selectedSourceIds : []
  const selected = source.profileIds.every(id => current.includes(id))
  const next = !current.length ? source.profileIds : selected ? current.filter(id => !source.profileIds.includes(id)) : [...new Set([...current, ...source.profileIds])]
  emit('update:selectedSourceIds', next)
}
function toggleNode(key: string) { if (!props.disabled) emit('update:selectedNodeKeys', props.selectedNodeKeys.includes(key) ? props.selectedNodeKeys.filter(item => item !== key) : [...props.selectedNodeKeys, key]) }
function toggleScopedNodes() {
  if (props.disabled) return
  const keys = new Set(scopedNodes.value.map(nodeKey))
  emit('update:selectedNodeKeys', allScopedSelected.value ? props.selectedNodeKeys.filter(key => !keys.has(key)) : [...new Set([...props.selectedNodeKeys, ...keys])])
}
function toggleGroupNodes(nodes: MonitorNodeOption[]) {
  if (props.disabled) return
  const keys = new Set(nodes.map(nodeKey))
  const allSelected = nodes.every(node => props.selectedNodeKeys.includes(nodeKey(node)))
  emit('update:selectedNodeKeys', allSelected ? props.selectedNodeKeys.filter(key => !keys.has(key)) : [...new Set([...props.selectedNodeKeys, ...keys])])
}
function clearNodes() { if (!props.disabled) emit('update:selectedNodeKeys', []) }
const isTestingEnv = typeof process !== 'undefined' && (process.env?.NODE_ENV === 'test' || Boolean(process.env?.VITEST))
const favoriteKey = 'speedtest.favorite-services.v1'
function readFavorites(): string[] {
  try {
    const value = typeof localStorage !== 'undefined' && !isTestingEnv ? JSON.parse(localStorage.getItem(favoriteKey) || '[]') : []
    return Array.isArray(value) ? value.filter(v => typeof v === 'string') : []
  } catch {
    return []
  }
}
const favorites = ref(readFavorites())
onMounted(async () => {
  if (!isTestingEnv) {
    try {
      if (typeof (bridgeApi as any).fetchSettings === 'function') {
        const settings = await bridgeApi.fetchSettings()
        if (Array.isArray(settings?.favorite_services) && settings.favorite_services.length > 0) {
          favorites.value = Array.from(new Set([...favorites.value, ...settings.favorite_services]))
        }
      }
    } catch {}
  }
})
watch(favorites, ids => {
  try {
    if (typeof localStorage !== 'undefined' && !isTestingEnv) {
      localStorage.setItem(favoriteKey, JSON.stringify(ids))
    }
  } catch {}
  if (!isTestingEnv) {
    try {
      if (typeof (bridgeApi as any).fetchSettings === 'function') {
        void bridgeApi.fetchSettings().then(s => bridgeApi.saveSettings({ ...s, favorite_services: ids })).catch(() => {})
      }
    } catch {}
  }
}, { deep: true })
const groups = computed(() => ['推荐', '我的常用', ...new Set(props.catalog.map(serviceGroup))])
const regions = computed(() => ['全部地区', ...new Set(props.catalog.map(r => r.region || '全球'))].map(value => ({ value, label: value })))
const filtered = computed(() => props.catalog.filter(r =>
  (search.value.trim() || (group.value === '推荐' ? r.batch_default : group.value === '我的常用' ? favorites.value.includes(r.service_id) : serviceGroup(r) === group.value))
  && (region.value === '全部地区' || region.value === r.region)
  && `${r.name} ${r.description || ''}`.toLowerCase().includes(search.value.toLowerCase())))
const selected = computed(() => props.catalog.filter(r => props.modelValue.includes(r.service_id)))
function toggle(id: string) { if (!props.disabled) emit('update:modelValue', props.modelValue.includes(id) ? props.modelValue.filter(v => v !== id) : [...props.modelValue, id]) }
function favorite(id: string) { favorites.value = favorites.value.includes(id) ? favorites.value.filter(v => v !== id) : [...favorites.value, id] }
</script>
<template>
<section class="service-library" aria-label="选择要检测的服务">
<div v-if="showTargets" class="setup-summary">
<button type="button" class="selection-summary" :class="{ editing: editor === 'services' }" :aria-expanded="editor === 'services'" @click="edit('services')"><span class="selection-icon">01</span><span><small>检测服务 · 已选 {{ selected.length }} 项</small><strong>{{ selected.length ? selected.slice(0, 3).map(serviceTitle).join('、') + (selected.length > 3 ? ' 等' : '') : '选择你想使用的服务' }}</strong></span><b>{{ editor === 'services' ? '收起' : '选择服务' }} ⌄</b></button>
<button type="button" class="selection-summary" :class="{ editing: editor === 'nodes' }" :aria-expanded="editor === 'nodes'" @click="edit('nodes')"><span class="selection-icon">02</span><span><small>检测节点</small><strong>{{ selectedNodeCount ? '已选 ' + selectedNodeCount + (showAlternateConfigs ? ' 个配置' : ' 条线路') : '选择需要检测的线路' }}</strong></span><b>{{ editor === 'nodes' ? '收起' : '选择节点' }} ⌄</b></button>
</div>
    <div v-if="showTargets" class="library-run"><div><strong>{{ selected.length }} 项服务 × {{ selectedNodeCount }} 个节点</strong><small>{{ !selectedNodeCount ? '请先点击“选择节点”' : !selected.length ? '请先选择服务' : '开始前可确认检测范围与消耗' }}</small></div><label>重复 <UiSelect :model-value="repeatCount" :options="[{ value: 1, label: '1 次' }, { value: 2, label: '2 次' }, { value: 3, label: '3 次' }]" aria-label="服务检测次数" @update:model-value="emit('update:repeatCount', Number($event))" /></label><label>超时 <UiSelect :model-value="timeoutSeconds" :options="[{ value: 5, label: '5 秒' }, { value: 10, label: '10 秒' }, { value: 20, label: '20 秒' }, { value: 30, label: '30 秒' }]" aria-label="服务单项超时" @update:model-value="emit('update:timeoutSeconds', Number($event))" /></label><button type="button" class="run-service" :disabled="disabled || !selected.length || !selectedNodeCount" @click="editor = null; emit('run')">开始检测</button></div>

<section v-if="!showTargets || editor === 'services'" class="service-editor" aria-label="服务目录">
<header class="editor-heading"><div><h2>选择服务</h2><p>跨分类多选；标记说明这项检查能验证到哪一步。</p></div><div class="library-search"><input v-model="search" type="search" placeholder="搜索服务" aria-label="搜索服务"><UiSelect v-model="region" :options="regions" aria-label="服务所属地区" /></div></header>
    <div class="library-body">
      <nav aria-label="服务分类"><button v-for="item in groups" :key="item" :aria-pressed="group === item" @click="group = item">{{ item }}<span>{{ item === '我的常用' ? favorites.length : item === '推荐' ? catalog.filter(r => r.batch_default).length : catalog.filter(r => serviceGroup(r) === item).length }}</span></button></nav>
      <div class="service-cards"><article v-for="rule in filtered" :key="rule.service_id" :class="{ picked: modelValue.includes(rule.service_id) }"><label><input type="checkbox" :checked="modelValue.includes(rule.service_id)" :disabled="disabled" @change="toggle(rule.service_id)"><strong>{{ serviceTitle(rule) }}</strong></label><button class="favorite-service" :aria-label="`${favorites.includes(rule.service_id) ? '取消常用' : '设为常用'} ${serviceTitle(rule)}`" :aria-pressed="favorites.includes(rule.service_id)" @click="favorite(rule.service_id)">{{ favorites.includes(rule.service_id) ? '★' : '☆' }}</button><p>{{ rule.description }}</p><footer><span :class="{ verified: rule.service_id === 'antigravity' }">{{ serviceEvidence(rule) }}</span><small>{{ rule.region || '全球' }}</small></footer></article><p v-if="!filtered.length" class="library-empty">{{ group === '我的常用' ? '点服务卡片右上角的 ☆，把常用服务放在这里。' : '没有匹配的服务，试试其他分类或地区。' }}</p></div>
    </div>

<div class="editor-footer"><span>已选 {{ selected.length }} 项服务</span><button v-if="selected.length" :disabled="disabled" @click="emit('update:modelValue', [])">清空服务</button><button v-if="showTargets" class="done-selection" @click="editor = selectedNodeCount ? null : 'nodes'">{{ selectedNodeCount ? '完成选择' : '下一步：选择节点' }}</button></div>
</section>
    <section v-if="showTargets && editor === 'nodes'" class="target-picker" aria-label="选择要检测的节点">
      <div class="source-pills"><button type="button" :class="{ active: !selectedSourceIds.length }" @click="emit('update:selectedSourceIds', [])">全部来源</button><button v-for="source in sources" :key="source.id" type="button" :class="{ active: selectedSourceIds.length > 0 && source.profileIds.every(id => selectedSourceIds.includes(id)) }" @click="toggleSource(source)">{{ source.name }} <small>{{ source.count }} 条线路<template v-if="source.mergedSourceCount > 1"> · {{ source.mergedSourceCount }} 个订阅</template></small></button><button v-if="hiddenAlternateCount" type="button" class="alternate-source-button" :disabled="disabled" :aria-pressed="showAlternateConfigs" @click="emit('update:showAlternateConfigs', !showAlternateConfigs)">{{ showAlternateConfigs ? '收起其他配置' : `查看其他配置 (${hiddenAlternateCount})` }}</button></div>
      <div class="node-tools"><input :value="nodeSearch" type="search" placeholder="搜索节点名称" aria-label="搜索待测节点" @input="emit('update:nodeSearch', ($event.target as HTMLInputElement).value)"><UiSelect :model-value="nodeRegion" :options="nodeRegions" aria-label="筛选待测节点地区" @update:model-value="emit('update:nodeRegion', String($event))" /><span class="scope-count">当前范围 {{ scopedNodes.length }} 条 · 已选 {{ selectedScopedCount }} 条</span><button type="button" class="select-range" :disabled="disabled || !scopedNodes.length" :aria-pressed="allScopedSelected" @click="toggleScopedNodes">{{ allScopedSelected ? '取消当前范围选择' : `全选当前范围（${scopedNodes.length}）` }}</button><button v-if="selectedNodeCount" type="button" class="clear-all-nodes" :disabled="disabled" @click="clearNodes">清空全部已选</button></div>
      <div class="node-choices"><section v-for="area in nodeGroups" :key="area.code" class="node-region-group"><div class="node-region-heading"><button type="button" class="region-expand" :aria-expanded="regionExpanded(area.code)" @click="toggleRegion(area.code)"><span aria-hidden="true">{{ regionExpanded(area.code) ? '▾' : '▸' }}</span><strong>{{ area.name }}</strong><small>{{ area.nodes.length }} 条线路 · 已选 {{ area.selected }} 条</small></button><button type="button" class="region-select" :disabled="disabled" @click="toggleGroupNodes(area.nodes)">{{ area.selected === area.nodes.length ? '取消本地区' : '全选本地区' }}</button></div><div v-if="regionExpanded(area.code)" class="region-node-grid"><label v-for="node in area.nodes" :key="nodeKey(node)" :class="{ checked: selectedNodeKeys.includes(nodeKey(node)) }" :title="node.displayName"><input type="checkbox" :checked="selectedNodeKeys.includes(nodeKey(node))" :disabled="disabled" @change="toggleNode(nodeKey(node))"><span>{{ node.displayName }}</span></label></div></section><p v-if="!scopedNodes.length">这个范围没有可检测节点，试试其他来源或地区。</p></div>
    </section>

<div v-if="showTargets && editor === 'nodes'" class="editor-footer"><span>已选 {{ selectedNodeCount }} 条线路 · 相同线路已合并</span><button class="done-selection" @click="editor = null">完成选择</button></div>
<p v-if="editor === 'services' || !showTargets" class="library-boundary">“仅网页访问”不代表能登录或播放。Antigravity 检测会消耗已绑定账号的少量模型额度。</p>
</section>
</template>
<style scoped>
.service-library{background:var(--card-bg);border:1px solid var(--border);border-radius:12px;margin:0 0 20px;overflow:hidden}
.setup-summary{display:grid;grid-template-columns:1fr 1fr;gap:12px;padding:18px}
.selection-summary{display:flex;align-items:center;gap:12px;min-width:0;text-align:left;padding:14px;border:1px solid var(--border);border-radius:9px;background:var(--card-bg)}
.selection-summary:hover,.selection-summary.editing{border-color:var(--primary);background:var(--primary-subtle)}
.selection-summary>span:nth-child(2){flex:1;min-width:0}
.selection-summary small{display:block;font-size:12px;color:var(--text-secondary);margin-bottom:5px}
.selection-summary strong{display:block;font-size:14px;font-weight:650;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.selection-summary b{font-size:13px;color:var(--primary);white-space:nowrap}
.selection-icon{font-size:12px;color:var(--text-muted);font-weight:700}
.library-run{display:flex;align-items:center;flex-wrap:wrap;gap:16px;padding:12px 18px;background:var(--card-subtle);border-top:1px solid var(--border)}
.library-run>div{margin-right:auto;display:grid;gap:4px}
.library-run strong{font-size:14px}
.library-run small{font-size:12px;color:var(--text-secondary)}
.library-run label{display:flex;align-items:center;gap:7px;font-size:13px}
.library-run :deep(.ui-select){min-width:85px}
.run-service,.done-selection{background:var(--primary);color:white;border-radius:7px;padding:10px 18px;font-size:14px;font-weight:650}
.run-service:disabled{opacity:.45;cursor:not-allowed}
.editor-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:16px 18px}
h2{font-size:16px;margin:0 0 4px}
.editor-heading p{font-size:12px;color:var(--text-secondary);margin:0}
.service-editor,.target-picker{border-top:1px solid var(--border)}
.library-search{display:flex;align-items:center;gap:8px}
.library-search input,.node-tools input{border:1px solid var(--border);background:var(--card-bg);border-radius:7px;padding:8px 10px;font-size:13px;min-width:0}
.library-body{display:grid;grid-template-columns:144px minmax(0,1fr);border-top:1px solid var(--border)}
nav{display:flex;flex-direction:column;gap:3px;padding:10px;background:var(--card-subtle)}
nav button{display:flex;align-items:center;justify-content:space-between;text-align:left;padding:8px;border-radius:6px;font-size:13px}
nav button[aria-pressed=true]{background:var(--primary-subtle);color:var(--primary);font-weight:700}
nav span{font-size:11px;color:var(--text-secondary)}
.service-cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(245px,1fr));align-content:start;gap:8px;padding:12px;max-height:330px;overflow:auto}
.service-cards article{position:relative;padding:12px;border:1px solid var(--border);border-radius:8px;background:var(--card-bg)}
.service-cards article.picked{border-color:var(--primary);background:var(--primary-subtle)}
article label{display:flex;align-items:center;gap:8px;padding-right:24px;font-size:14px;cursor:pointer}
article p{font-size:12px;line-height:1.5;color:var(--text-secondary);margin:8px 0}
article footer{display:flex;justify-content:space-between;gap:6px;font-size:11px;color:var(--text-secondary)}
article footer .verified{color:var(--success)}
.favorite-service{position:absolute;right:10px;top:9px;font-size:20px;color:var(--text-muted)}
.favorite-service[aria-pressed=true]{color:#a97411}
.library-empty{padding:20px;font-size:13px;color:var(--text-secondary);grid-column:1/-1}
.editor-footer{display:flex;align-items:center;gap:12px;padding:12px 18px;border-top:1px solid var(--border);font-size:13px}
.editor-footer>span{color:var(--text-secondary)}
.editor-footer>.done-selection{margin-left:auto}
.library-boundary{font-size:12px;line-height:1.6;color:var(--text-secondary);margin:0;padding:10px 18px;background:var(--card-subtle)}
.target-picker{padding:16px 18px}
.source-pills{display:flex;flex-wrap:wrap;align-items:center;gap:7px;margin-bottom:12px}
.source-pills button{display:flex;align-items:center;gap:6px;padding:7px 10px;border:1px solid var(--border);border-radius:6px;font-size:13px}
.source-pills button.active{background:var(--primary-subtle);border-color:var(--primary);color:var(--primary)}
.source-pills small{font-size:11px;color:var(--text-secondary)}
.source-pills .alternate-source-button{margin-left:auto;font-size:12px;border:0;color:var(--text-secondary)}
.node-tools{display:flex;align-items:center;flex-wrap:wrap;gap:8px;margin-bottom:12px}
.scope-count{font-size:12px;color:var(--text-secondary)}
.select-range{padding:8px 10px;background:var(--primary-subtle);color:var(--primary);border:1px solid var(--primary);border-radius:6px;font-size:13px}
.clear-all-nodes{font-size:12px;color:var(--text-secondary)}
.node-choices{max-height:290px;overflow:auto;border:1px solid var(--border);border-radius:8px}
.node-region-group+.node-region-group{border-top:1px solid var(--border)}
.node-region-heading{display:flex;align-items:center;padding:0 12px;gap:12px}
.region-expand{display:flex;align-items:center;gap:10px;flex:1;text-align:left;padding:10px 0}
.region-expand strong{font-size:14px}
.region-expand small{font-size:12px;color:var(--text-secondary)}
.region-expand>span{font-size:12px;color:var(--text-muted)}
.region-select{font-size:12px;color:var(--primary);padding:6px}
.region-node-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:5px;padding:8px;background:var(--card-subtle)}
.node-choices label{display:flex;align-items:center;gap:8px;min-width:0;padding:9px;border-radius:6px;font-size:13px;cursor:pointer}
.node-choices label span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.node-choices label:hover,.node-choices label.checked{background:var(--primary-subtle)}
.node-choices>p{padding:16px;font-size:13px;color:var(--text-secondary)}
@media(max-width:1000px){.setup-summary{grid-template-columns:1fr}.region-node-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.editor-heading{flex-wrap:wrap}.library-run{gap:10px}}
@media(max-width:650px){.library-body{display:block}nav{flex-direction:row;overflow:auto}nav button{white-space:nowrap;gap:8px}.region-node-grid{grid-template-columns:1fr}.library-run>div{width:100%}.library-search{flex-wrap:wrap}.selection-summary{padding:10px}.selection-icon{display:none}.source-pills button{flex-wrap:wrap}}
</style>
