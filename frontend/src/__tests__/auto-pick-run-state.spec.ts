import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import AutoPickCard from '../components/AutoPickCard.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'

vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),removeItem:(k:string)=>items.delete(k),clear:()=>items.clear()}})})
vi.mock('../api',()=>({api:{autoPick:vi.fn(),saveAutoPick:vi.fn(),runAutoPick:vi.fn()}}))

const baseStatus:any={
  config:{enabled:false,schedule:'6h',daily_hour:6,airport_ids:['a1'],region_filter:'',include_ai:false,include_streaming:false,ai_required:['chatgpt_unlock','claude_unlock'],streaming_required:['netflix_unlock'],min_response_rate:90,max_latency_ms:800,sample_count:6},
  running:false,
  estimate:{nodes:10,projects:1,requests:10,warn:false},
  subscription:{url:'http://127.0.0.1:17891/sub/t.yaml',path:'',exists:false,node_count:0}
}
const clone=(value:any)=>JSON.parse(JSON.stringify(value))

function mountCard(){
  const workspace=createWorkspace()
  workspace.airports.value=[{id:'a1',name:'机场A',node_count:10} as any]
  return mount(AutoPickCard,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:workspace},stubs:{Icon:true}}})
}
const runButton=(wrapper:any)=>wrapper.findAll('button.button').find((button:any)=>button.text().includes('立即运行一次'))!

beforeEach(()=>{
  vi.resetAllMocks()
  vi.mocked(api.autoPick).mockResolvedValue(clone(baseStatus))
  vi.mocked(api.saveAutoPick).mockResolvedValue(clone(baseStatus))
})
afterEach(()=>{
  vi.useRealTimers()
  document.body.innerHTML=''
})

describe('auto-pick run guard',()=>{
  it('refuses to start while the form has unsaved changes',async()=>{
    const wrapper=mountCard()
    await flushPromises()
    expect(runButton(wrapper).attributes('disabled')).toBeUndefined()

    const aiToggle=wrapper.findAll('label.check-field').find(label=>label.text().includes('AI 组'))!
    await aiToggle.get('input').setValue(true)
    await flushPromises()

    expect(wrapper.text()).toContain('设置有修改，请先保存')
    expect(runButton(wrapper).attributes('disabled')).toBeDefined()
    await runButton(wrapper).trigger('click')
    expect(wrapper.find('.auto-pick-confirm').exists()).toBe(false)
    expect(api.runAutoPick).not.toHaveBeenCalled()

    vi.mocked(api.saveAutoPick).mockRejectedValueOnce(new Error('写入失败'))
    const save=wrapper.findAll('button.button').find(button=>button.text().includes('保存设置'))!
    await save.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('写入失败')
    expect(wrapper.find('.auto-pick-dirty').exists()).toBe(true)
    expect(runButton(wrapper).attributes('disabled')).toBeDefined()
    expect(api.runAutoPick).not.toHaveBeenCalled()
  })

  it('resumes polling when the card opens during a running round',async()=>{
    vi.useFakeTimers({toFake:['setInterval','clearInterval']})
    const running=clone(baseStatus)
    running.running=true
    const finished=clone(baseStatus)
    finished.running=false
    finished.last_report={report_id:'ap-1',created_at:'2026-10-23T03:00:00Z',trigger:'manual',status:'selected',summary:'10 个节点入选',selected_count:10,daily_count:10,streaming_count:0,ai_count:0,kept_previous:false}
    finished.subscription={url:'http://127.0.0.1:17891/sub/t.yaml',path:'',exists:true,updated_at:'2026-10-23T03:05:00Z',node_count:10}
    vi.mocked(api.autoPick).mockResolvedValueOnce(running).mockResolvedValue(finished)

    const wrapper=mountCard()
    await flushPromises()
    expect(wrapper.text()).toContain('正在运行本轮精选')
    expect(api.autoPick).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(api.autoPick).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.auto-pick-running').exists()).toBe(false)
    expect(wrapper.text()).toContain('本轮已结束')
    expect(wrapper.text()).toContain('10 个节点入选')
    expect(wrapper.text()).toContain('查看报告')
    expect(wrapper.text()).toContain('文件更新于')

    await vi.advanceTimersByTimeAsync(9000)
    expect(api.autoPick).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
