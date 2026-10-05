import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import MonitorCreateModal from '../components/MonitorCreateModal.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import type {Airport,MonitorJob,NodeOption} from '../domain'

vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),clear:()=>items.clear()}})})
vi.mock('../api',()=>({api:{airports:vi.fn(),nodes:vi.fn(),jobs:vi.fn(),createJob:vi.fn(),jobAction:vi.fn()},events:vi.fn(),mapLimited:vi.fn(),saveFile:vi.fn()}))

const a:NodeOption={profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A',display_name:'Node A',type:'trojan',country_code:'HK',country_flag:''}
const b:NodeOption={profile_id:'sub-b',node_key:'node-b',node_identity_key:'identity-b',config_revision_key:'revision-b',profile_name:'Airport B',display_name:'Node B',type:'ss',country_code:'SG',country_flag:''}
const airports:Airport[]=[a,b].map((n,i)=>({id:`airport-${i?'b':'a'}`,name:n.profile_name,node_count:1,has_cache:true,url_display:'',subscriptions:[{id:n.profile_id,airport_id:`airport-${i?'b':'a'}`,name:'Default',url_display:'',url_configured:true,node_count:1,has_cache:true}]}))
const jobA={id:'job-a',profile_id:'sub-a'} as MonitorJob
let wrapper:VueWrapper|undefined

function attach(){const w=createWorkspace();w.airports.value=airports;w.nodes.value=[a,b];w.setAirports(['airport-a','airport-b']);w.toggleNode(a);w.toggleNode(b);w.monitorNodes.value=w.selectedNodes.value.map(n=>({...n}));wrapper=mount(MonitorCreateModal,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});return w}
function submit(){(document.querySelector('dialog .modal-footer .primary') as HTMLButtonElement).click()}

beforeEach(()=>{window.localStorage.clear();vi.resetAllMocks();vi.mocked(api.jobs).mockResolvedValue([]);vi.mocked(api.jobAction).mockResolvedValue(undefined);HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;document.body.innerHTML=''})

describe('monitor selection recovery',()=>{
  it.each(['airport deselected','configuration refreshed'])('requires a new selection when %s after the dialog opens',async reason=>{
    const w=attach()
    if(reason==='airport deselected')w.setAirports(['airport-a'])
    else{vi.mocked(api.airports).mockResolvedValue(airports);vi.mocked(api.nodes).mockResolvedValue([a,{...b,config_revision_key:'revision-b-new'}]);await w.refreshNodes()}
    await flushPromises();submit();await flushPromises()
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('所选节点范围或配置已变化，请返回重新选择')
    expect(api.createJob).not.toHaveBeenCalled()
    expect(api.jobAction).not.toHaveBeenCalled()
    expect(w.monitorNodes.value?.map(n=>n.node_key)).toEqual(['node-a','node-b'])
    expect(w.page.value).toBe('home')
  })

  it('retains an already created job and stops the remaining work when refresh completes during creation',async()=>{
    const w=attach();let resolveCreate!:(job:MonitorJob)=>void
    vi.mocked(api.createJob).mockImplementationOnce(()=>new Promise(resolve=>resolveCreate=resolve))
    submit();await flushPromises()
    vi.mocked(api.airports).mockResolvedValue(airports);vi.mocked(api.nodes).mockResolvedValue([a,{...b,config_revision_key:'revision-b-new'}]);await w.refreshNodes()
    resolveCreate(jobA);await flushPromises()
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('所选节点范围或配置已变化，请返回重新选择')
    expect(document.body.textContent).toContain('已创建')
    expect(api.createJob).toHaveBeenCalledTimes(1)
    expect(api.jobAction).not.toHaveBeenCalled()
    submit();await flushPromises()
    expect(api.createJob).toHaveBeenCalledTimes(1)
    expect(api.jobAction).not.toHaveBeenCalled()
    expect(w.monitorNodes.value).not.toBeNull()
  })
})
