import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import AutoPickCard from '../components/AutoPickCard.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'

vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),removeItem:(k:string)=>items.delete(k),clear:()=>items.clear()}})})
vi.mock('../api',()=>({api:{autoPick:vi.fn(),saveAutoPick:vi.fn(),runAutoPick:vi.fn()}}))

const status:any={
  config:{enabled:false,schedule:'6h',daily_hour:6,airport_ids:['a1'],region_filter:'',include_ai:false,include_streaming:false,ai_required:['chatgpt_unlock','claude_unlock'],streaming_required:['netflix_unlock'],min_response_rate:90,max_latency_ms:800,sample_count:6},
  running:false,
  estimate:{nodes:10,projects:1,requests:10,warn:false},
  subscription:{url:'http://127.0.0.1:17891/sub/t.yaml',path:'',exists:false,node_count:0}
}

function mountCard(){
  const workspace=createWorkspace()
  workspace.airports.value=[{id:'a1',name:'良心云',node_count:10} as any]
  return mount(AutoPickCard,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:workspace},stubs:{Icon:true}}})
}

beforeEach(()=>{
  vi.resetAllMocks()
  vi.mocked(api.autoPick).mockResolvedValue(status)
  vi.mocked(api.saveAutoPick).mockResolvedValue(status)
})
afterEach(()=>{document.body.innerHTML=''})

describe('auto-pick required services',()=>{
  it('only counts and saves the services the user keeps as required',async()=>{
    const wrapper=mountCard()
    await flushPromises()
    expect(wrapper.get('.auto-pick-estimate').text()).toContain('10 个节点 × 1 个项目')

    const aiToggle=wrapper.findAll('label.check-field').find(label=>label.text().includes('AI 组'))!
    await aiToggle.get('input').setValue(true)
    await flushPromises()
    expect(wrapper.get('.auto-pick-estimate').text()).toContain('10 个节点 × 3 个项目')

    const expand=wrapper.findAll('button.text-button').find(button=>button.text().includes('必需'))!
    expect(expand.text()).toContain('必需 2 项')
    await expand.trigger('click')
    const claude=wrapper.findAll('label.check-field').find(label=>label.text().trim()==='Claude')!
    await claude.get('input').setValue(false)
    await flushPromises()
    expect(wrapper.get('.auto-pick-estimate').text()).toContain('10 个节点 × 2 个项目')

    const save=wrapper.findAll('button.button').find(button=>button.text().includes('保存设置'))!
    await save.trigger('click')
    await flushPromises()
    expect(api.saveAutoPick).toHaveBeenCalledTimes(1)
    const payload=vi.mocked(api.saveAutoPick).mock.calls[0][0] as any
    expect(payload.include_ai).toBe(true)
    expect(payload.ai_required).toEqual(['chatgpt_unlock'])
    expect(payload.streaming_required).toEqual(['netflix_unlock'])
  })

  it('says the schedule and the subscription only live while CST runs',async()=>{
    const wrapper=mountCard()
    await flushPromises()
    expect(wrapper.text()).toContain('定时任务和订阅地址只在 CST 运行时有效。')
    expect(wrapper.text()).toContain('解锁没通过只影响用途组，节点仍可进日常组。')
  })
})

describe('auto-pick rounds in node history',()=>{
  it('labels auto_pick rounds as 自动精选',async()=>{
    const {triggerLabel}=await import('../measurementRounds')
    expect(triggerLabel('auto_pick')).toBe('自动精选')
    expect(triggerLabel('manual')).toBe('手动')
  })
})
