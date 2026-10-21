import {computed,ref} from 'vue'
import {mount,flushPromises} from '@vue/test-utils'
import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import UnlockServiceMatrix from '../components/home/UnlockServiceMatrix.vue'
import {homeModelKey,type HomeModel} from '../components/home/homeModel'
import {createWorkspace,workspaceKey} from '../workspace'
import {api,saveFile} from '../api'
import {key,type Attempt,type NodeOption,type ServiceRule} from '../domain'
vi.mock('../api',async original=>{const actual=await original<typeof import('../api')>();return {...actual,saveFile:vi.fn(async()=>true),api:{...actual.api,exportClash:vi.fn()}}})
let w:ReturnType<typeof createWorkspace>,view:ReturnType<typeof mount>,scope:ReturnType<typeof ref<NodeOption[]>>,nodes:NodeOption[]
beforeEach(()=>{
 vi.clearAllMocks();window.localStorage.clear();w=createWorkspace()
 nodes=['SG','SG','SG','JP'].map((country_code,i)=>({profile_id:'p',node_key:String(i),node_identity_key:String(i),config_revision_key:'r',profile_name:'fixture',display_name:country_code+' '+i,type:'ss',country_code,country_flag:''}))
 scope=ref(nodes)
 const records=new Map(nodes.slice(0,2).map((n,i)=>[key(n),{...n,attempt_id:String(i),request_id:String(i),requested_at:'2026-10-21T01:00:00Z',execution_state:'completed',persistence_state:'saved',service_id:'chatgpt_unlock',started_at:'2026-10-21T01:00:00Z',rule:{service_id:'chatgpt_unlock',target_url:'https://example.test'},result:{outcome:i?'region_blocked':'unlocked',summary:i?'地区不支持':'',bytes_read:0,finished_at:'2026-10-21T02:00:00Z'}} as Attempt]))
 const model={runNodes:computed(()=>scope.value),qualityCutoff:ref('2026-10-21T03:00:00Z'),qualityErrors:ref({}),qualityLoading:ref(false),readQualityNodes:vi.fn(async()=>{}),displayServices:computed(()=>[{service_id:'chatgpt_unlock',name:'ChatGPT'},{service_id:'abema_unlock',name:'ABEMA'}]),shownService:(n:NodeOption,id:string)=>id==='chatgpt_unlock'?records.get(key(n)):undefined} as unknown as HomeModel
 w.catalog.value=model.displayServices.value as ServiceRule[]
 vi.mocked(api.exportClash).mockResolvedValue({yaml_content:'fixture',node_count:1})
 view=mount(UnlockServiceMatrix,{global:{provide:{[workspaceKey as symbol]:w,[homeModelKey as symbol]:model}}})
})
afterEach(()=>{view.unmount();w.dispose()})
it('shares scope, confirmed counts, service-only untested plan and available export',async()=>{
 await flushPromises();const sg=view.get('button[aria-label="ChatGPT · 新加坡 · 1/2"]');await sg.trigger('click')
 expect(sg.classes()).toContain('warn');expect(view.get('button[aria-label="ABEMA · 新加坡 · —"]').attributes('title')).toBe('非日本节点，不检测')
 expect(view.text()).toContain('地区不支持');expect(view.text()).toContain('检测未测的 1 个')
 await view.findAll('button').find(b=>b.text()==='检测未测的 1 个')!.trigger('click')
 expect(w.plan.value).toMatchObject({nodes:[nodes[2]],projects:['service'],serviceIds:['chatgpt_unlock'],includeReputation:false,reputationServiceIds:[]})
 await view.findAll('button').find(b=>b.text()==='导出可用节点')!.trigger('click');await flushPromises()
 expect(api.exportClash).toHaveBeenCalledWith([nodes[0]],'ChatGPT · 新加坡');expect(saveFile).toHaveBeenCalledOnce()
 scope.value=[nodes[3]!];await flushPromises();expect(view.find('[aria-label="ChatGPT · 新加坡 · 1/2"]').exists()).toBe(false)
})
it('reports a failed export without claiming success',async()=>{
 await view.get('button[aria-label="ChatGPT · 新加坡 · 1/2"]').trigger('click');vi.mocked(api.exportClash).mockRejectedValueOnce(new Error('导出失败'))
 await view.findAll('button').find(b=>b.text()==='导出可用节点')!.trigger('click');await flushPromises()
 expect(w.error.value).toBe('导出失败');expect(saveFile).not.toHaveBeenCalled()
})
