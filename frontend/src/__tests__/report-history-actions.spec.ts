import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {mount,flushPromises,type VueWrapper} from '@vue/test-utils'
import QueueModal from '../components/QueueModal.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {api,saveFile} from '../api'
import type {BatchItem,LatencyBatch,NodeOption} from '../domain'
vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',()=>({api:{exportClash:vi.fn()},saveFile:vi.fn()}))
vi.mock('../latencyReportData',()=>({readLatencyBatches:vi.fn(async()=>[])}))
const node:NodeOption={profile_id:'p',profile_name:'Current account',node_key:'n',node_identity_key:'i',config_revision_key:'r',display_name:'Node',type:'ss',country_code:'HK',country_flag:''}
const old:BatchItem={...node,item_id:'old',batch_id:'b',config_revision_key:'old',execution_state:'completed',persistence_state:'saved',result:{...node,config_revision_key:'old',attempt_id:'old',requested_at:'2026-10-01T08:00:00Z',finished_at:'2026-10-01T08:01:00Z',display_name:'Node',node_type:'ss',status:'completed',latency_ms:80,total_samples:1,success_samples:1,failure_samples:0,persistence_state:'saved',samples:[{seq:1,target:'https://www.gstatic.com/generate_204',timestamp:'2026-10-01T08:00:00Z',success:true,latency_ms:80}]}}
const current={...old,item_id:'current',config_revision_key:'r'}
const batch=(items:BatchItem[]):LatencyBatch=>({batch_id:'b',request_id:'b',requested_at:'2026-10-01T08:00:00Z',state:'completed',target_id:'google',item_count:items.length,items})
let wrapper:VueWrapper|undefined,w:Workspace
function attach(items:BatchItem[],monitor=false){w.openLatencyReport(batch(items),monitor,[batch(items)]);wrapper=mount(QueueModal,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w},stubs:{LatencyReport:{props:['batch'],emits:['inspect','export'],template:'<div><button data-test="inspect" @click="$emit(\'inspect\',batch.items[0])">Inspect</button><button data-test="export" @click="$emit(\'export\',batch.items)">Export</button></div>'}}}})}
beforeEach(()=>{vi.resetAllMocks();HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn();w=createWorkspace();w.nodes.value=[node];vi.mocked(api.exportClash).mockResolvedValue({yaml_content:'proxies: []',node_count:1});vi.mocked(saveFile).mockResolvedValue(true)})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML=''})
it.each([false,true])('opens removed report configuration with the correct source and report time: monitor=%s',async monitor=>{
 attach([old],monitor);(document.querySelector('[data-test="inspect"]') as HTMLButtonElement).click();await flushPromises()
 expect(w.inspectNode.value).toMatchObject({profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'old',historySource:monitor?'monitor':'manual',historyProject:'latency',historyTargetId:'google',historyUntil:'2026-10-01T08:01:01.000Z'})
 expect(w.queueOpen.value).toBe(false);expect(w.plan.value).toBeNull()
})
it('does not silently export only the still-current subset of a saved report',async()=>{
 attach([current,old]);(document.querySelector('[data-test="export"]') as HTMLButtonElement).click();await flushPromises()
 expect(api.exportClash).not.toHaveBeenCalled();expect(saveFile).not.toHaveBeenCalled();expect(w.toast.value).toContain('1')
})
it('does not substitute another key sharing the same identity and revision',async()=>{
 attach([{...current,node_key:'different-key'}]);(document.querySelector('[data-test="export"]') as HTMLButtonElement).click();await flushPromises()
 expect(api.exportClash).not.toHaveBeenCalled()
})

it('continues to export the complete exact current selection',async()=>{
 attach([current]);(document.querySelector('[data-test="export"]') as HTMLButtonElement).click();await flushPromises()
 expect(api.exportClash).toHaveBeenCalledWith([node]);expect(saveFile).toHaveBeenCalledWith('proxies: []','cst-usable.yaml','application/x-yaml')
})
