<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Airport, AirportMaintenance } from '../../types'
import { saveAirportMaintenance } from '../../api/bridge'
import UiSelect from '../common/UiSelect.vue'
const props = withDefaults(defineProps<{ airport: Airport; discoveredLinks?: { label: string; url: string }[] }>(), { discoveredLinks: () => [] })
const emit = defineEmits<{ (event: 'saved'): void }>()
const draft = ref<AirportMaintenance>({ refresh_hours: 0, links: [] })
const editing = ref(false)
const busy = ref(false)
const error = ref('')
const now = ref(new Date())
const month = computed(() => `${now.value.getFullYear()}-${String(now.value.getMonth() + 1).padStart(2, '0')}`)
const clock = setInterval(() => { now.value = new Date() }, 60000)
onBeforeUnmount(() => clearInterval(clock))
const settings = computed(() => props.airport.maintenance || { refresh_hours: 0, links: [] })
const refreshOptions = [
  { value: 0, label: '关闭' },
  { value: 1, label: '每 1 小时' },
  { value: 6, label: '每 6 小时' },
  { value: 12, label: '每 12 小时' },
  { value: 24, label: '每 24 小时' },
  { value: 72, label: '每 72 小时' },
  { value: 168, label: '每 168 小时' },
]
const unsavedDiscoveredLinks = computed(() => {
  const saved = new Set((settings.value.links || []).map(link => safeLink(link.url)).filter(Boolean))
  const seen = new Set<string>()
  return props.discoveredLinks.filter(link => {
    const url = safeLink(link.url)
    if (!url || saved.has(url) || seen.has(url)) return false
    seen.add(url)
    return true
  })
})
function reset() { draft.value = JSON.parse(JSON.stringify({ ...settings.value, links: settings.value.links || [] })); error.value = '' }
watch(() => props.airport.id, () => { editing.value = false; reset() }, { immediate: true })
function open() { reset(); editing.value = true }
function safeLink(url: string) { try { const u = new URL(url); return ['http:', 'https:'].includes(u.protocol) && !u.username && !u.password ? u.href : undefined } catch { return undefined } }
function due(link: AirportMaintenance['links'][number]) { return link.monthly_day > 0 && now.value.getDate() >= link.monthly_day && link.done_month !== month.value }
function time(value?: string) { return value && !value.startsWith('0001') ? new Date(value).toLocaleString() : '—' }
async function save(value = draft.value) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { await saveAirportMaintenance(props.airport.id, value); editing.value = false; emit('saved') }
  catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  finally { busy.value = false }
}
function complete(index: number) { const next = JSON.parse(JSON.stringify(settings.value)) as AirportMaintenance; next.links[index].done_month = next.links[index].done_month === month.value ? '' : month.value; void save(next) }
function setRefreshHours(value: string | number) { draft.value.refresh_hours = Number(value) || 0 }
function addDiscovered(link: { label: string; url: string }) {
  const url = safeLink(link.url)
  if (!url) return
  const next = JSON.parse(JSON.stringify(settings.value)) as AirportMaintenance
  next.links = [...(next.links || []), { label: link.label, url, monthly_day: 0 }]
  void save(next)
}
</script>

<template>
  <section class="airport-maintenance">
    <header><div><strong>日常管理</strong><span>订阅自动更新 · 常用入口 · 月度待办</span></div><button :disabled="busy" @click="open">管理</button></header>
    <div class="refresh-summary"><span class="schedule-badge">{{ settings.refresh_hours ? `每 ${settings.refresh_hours} 小时刷新` : '定时刷新未开启' }}</span><span v-if="settings.refresh_hours">下次 {{ time(settings.next_refresh) }}</span><small v-if="settings.last_result">{{ settings.last_result }} · {{ time(settings.last_attempt) }}</small></div>
    <div class="shortcut-list">
      <article v-for="(link, index) in settings.links" :key="index" :class="{ due: due(link) }">
        <a :href="safeLink(link.url)" target="_blank" rel="noopener noreferrer">{{ link.label }} ↗</a>
        <span v-if="link.monthly_day">{{ link.done_month === month ? '本月已完成' : due(link) ? '本月待处理' : `每月 ${link.monthly_day} 日` }}</span>
        <button v-if="link.monthly_day" :disabled="busy" @click="complete(index)">{{ link.done_month === month ? '撤销' : '标记完成' }}</button>
      </article>
      <article v-for="link in unsavedDiscoveredLinks" :key="link.url" class="discovered-link">
        <span><b>订阅发现</b>{{ link.label }}</span>
        <a :href="safeLink(link.url)" target="_blank" rel="noopener noreferrer">打开 ↗</a>
        <button :disabled="busy" @click="addDiscovered(link)">加入日常管理</button>
      </article>
      <p v-if="!settings.links?.length && !unsavedDiscoveredLinks.length">订阅中暂未发现入口；也可以手动加入公告、签到或兑换页面。</p>
    </div>
    <form v-if="editing" class="maintenance-editor" @submit.prevent="save()">
      <label>订阅定时刷新<UiSelect :model-value="draft.refresh_hours" :options="refreshOptions" :disabled="busy" aria-label="订阅定时刷新" @update:model-value="setRefreshHours" /></label>
      <p>刷新此机场全部订阅。仅程序运行时执行，关闭网页不影响；保存不会立即刷新。</p>
      <div v-for="(link, index) in draft.links" :key="index" class="link-editor">
        <input v-model="link.label" required maxlength="100" placeholder="名称，例如：月初领兑换码" aria-label="入口名称" :disabled="busy" />
        <input v-model="link.url" required type="url" placeholder="https://… 公告或活动地址" aria-label="入口网址" :disabled="busy" />
        <label>每月提醒日<input v-model.number="link.monthly_day" type="number" min="0" max="28" :disabled="busy" /></label>
        <button type="button" :disabled="busy" @click="draft.links.splice(index, 1)">移除</button>
      </div>
      <p>提醒日填 0 只保存入口；1–28 在当月到期后显示待办，不读取公告、不自动兑换。</p>
      <footer><button type="button" :disabled="busy || draft.links.length >= 20" @click="draft.links.push({ label: '', url: '', monthly_day: 0 })">＋入口 / 待办</button><button type="button" :disabled="busy" @click="editing = false">取消</button><button class="primary" :disabled="busy">{{ busy ? '保存中…' : '保存设置' }}</button></footer>
    </form>
    <p v-if="error" role="alert" class="error">{{ error }}</p>
  </section>
</template>

<style scoped>
.airport-maintenance { margin: 18px 24px; padding: 18px; background: #f0f5fc; border: 1px solid #ccd9ed; border-left: 3px solid #5475ac; border-radius: 8px; font-size: 12px; }
header, header > div, .refresh-summary, footer { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
header { justify-content: space-between; margin-bottom: 14px; } header strong { color: #2a4269; } header span, p, small { color: var(--text-secondary); }
button, input { border: 1px solid var(--border); border-radius: 6px; background: white; padding: 6px 9px; font-size: 12px; } button { cursor: pointer; } button:disabled { opacity: .5; }
.schedule-badge { background: #dce7f7; color: #315788; padding: 5px 8px; border-radius: 4px; }
.shortcut-list { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 12px; }
article { display: flex; gap: 10px; align-items: center; padding: 9px 12px; background: white; border: 1px solid #d6dfeb; border-radius: 6px; } article.due { border-color: #d6a861; background: #fff8e9; } article span { font-size: 11px; color: #8b6428; } a { color: #244e88; font-weight: 600; }
.discovered-link { border-style: dashed; background: #f8fbff; } .discovered-link > span { display: grid; color: var(--text-main); } .discovered-link b { color: var(--primary); font-size: 9px; letter-spacing: .08em; text-transform: uppercase; }
.maintenance-editor { display: grid; gap: 12px; margin-top: 16px; border-top: 1px solid #ccd9ed; padding-top: 16px; } label { display: flex; align-items: center; gap: 8px; }
.link-editor { display: grid; grid-template-columns: 1fr 2fr auto auto; gap: 8px; } input { min-width: 0; } label input { width: 62px; } footer { justify-content: flex-end; } .primary { background: var(--primary); color: white; } .error { color: #a33c32; }
@media(max-width: 900px) { .link-editor { grid-template-columns: 1fr; } }
</style>
