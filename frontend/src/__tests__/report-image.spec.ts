import {afterEach,expect,it,vi} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import html2canvas from 'html2canvas'
import LatencyReport from '../components/LatencyReport.vue'
import type {LatencyBatch} from '../domain'
vi.mock('html2canvas',()=>({default:vi.fn()}))
afterEach(()=>{delete window.go;document.body.innerHTML='';vi.restoreAllMocks()})
it('exports the current report without private fields and reports a cancelled save',async()=>{
 const save=vi.fn().mockResolvedValueOnce(true).mockResolvedValueOnce(false)
 window.go={desktop:{App:{SavePNGFile:save}}}
 const batch:LatencyBatch={batch_id:'b',request_id:'b',requested_at:'2026-10-21T11:00:00Z',state:'completed',target_id:'all',timeout_seconds:5,sample_count:6,item_count:1,items:[{item_id:'n',batch_id:'b',profile_id:'p',node_key:'n',node_identity_key:'n',config_revision_key:'r',display_name:'香港 01 192.0.2.1',execution_state:'completed',persistence_state:'saved',error_message:'connect 192.0.2.2 private.example.net https://example.net/sub?token=secret'}]}
 let exported=''
 vi.mocked(html2canvas).mockImplementation(async element=>{exported=element.textContent||'';return {toDataURL:()=> 'data:image/png;base64,cG5n'} as HTMLCanvasElement})
 const wrapper=mount(LatencyReport,{props:{batch,airportNames:{p:'示例机场'}},attachTo:document.body})
 const button=()=>wrapper.findAll('button').find(b=>b.text()==='导出图片')!
 await button().trigger('click');await flushPromises()
 expect(exported).toContain('示例机场');expect(exported).toContain('最快 5 个');expect(exported).toContain('问题分组');expect(exported).toContain('节点结果');expect(exported).toContain('CST')
 expect(exported).not.toMatch(/192\.0\.2|private\.example|token=secret/)
 expect(save).toHaveBeenCalledWith('cG5n','CST-report-20261021110000.png');expect(wrapper.get('[role=status]').text()).toBe('图片已保存')
 await button().trigger('click');await flushPromises();expect(wrapper.get('[role=status]').text()).toBe('已取消保存')
 wrapper.unmount()
})
