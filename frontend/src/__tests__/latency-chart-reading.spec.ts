import {afterEach,expect,it,onTestFinished} from 'vitest'
import {mount,type VueWrapper} from '@vue/test-utils'
import {nextTick} from 'vue'
import LatencyChart from '../components/home/LatencyChart.vue'
import InspectorLatency from '../components/home/InspectorLatency.vue'
import HoverTip from '../components/HoverTip.vue'
import type {LatencySeries} from '../latencyHistory'
import type {TrendPoint} from '../presentation'
import {hideHoverTip,hoverTip,showHoverTip} from '../hoverTip'

const wrappers:VueWrapper[]=[]
afterEach(()=>{for(const wrapper of wrappers.splice(0))wrapper.unmount()})
const at=Date.now()
function round(id:string,values:(number|null)[],offset=0):TrendPoint {
 const time=new Date(at+offset).toISOString()
 return {id,time,value:null,tone:'good',description:'synthetic',samples:values.map((value,i)=>({id:id+':'+i,time,value,tone:value===null?'bad':'good',description:'synthetic',label:String(i+1),...(value===null?{error:'synthetic timeout'}:{})}))}
}
const site=(id:string,name:string,points:TrendPoint[]):LatencySeries=>({id,name,points,context:''})
function chart(series:LatencySeries[],history=false,changes=[] as {time:string;label:string}[]){const w=mount(LatencyChart,{props:{series,history,changes}});wrappers.push(w);return w}
const points=(w:VueWrapper)=>w.findAll('.latency-combined circle').filter(c=>c.attributes('fill')!=='transparent')

it('makes sub-100ms differences readable on a zero-based scale',()=>{
 const w=chart([site('cloudflare','Cloudflare',[round('r',[20,40,68])])])
 const y=points(w).map(c=>Number(c.attributes('cy')))
 expect(Math.max(...y)-Math.min(...y)).toBeGreaterThan(40)
 expect(w.text()).toContain('自动量程')
 expect(w.findAll('.latency-combined text').map(e=>e.text())).not.toContain('1500')
})

it('uses the same scale for raw samples and station ranges without clipping slow samples',()=>{
 const series=[site('cloudflare','Cloudflare',[round('r',[1800,2400,4000])])]
 const w=mount(InspectorLatency,{props:{series}});wrappers.push(w)
 const mark=w.get('.range-track b').element as HTMLElement
 const range=w.get('.range-track i').element as HTMLElement
 expect(Number.parseFloat(mark.style.left)).toBeLessThan(90)
 expect(Number.parseFloat(range.style.right)).toBeGreaterThan(0)
 const ticks=w.findAll('.range-axis span').map(t=>t.text().replace(' ms',''))
 const labels=w.find('.latency-combined').findAll('.latency-axis-label').map(t=>t.text())
 expect(labels).toEqual(ticks)
 expect(w.get('.latency-metrics').text()).toContain('3,200 ms')
})

it('separates failures from zero latency and does not connect across missing samples',async()=>{
 const w=chart([site('cloudflare','Cloudflare',[round('r',[0,null,60])]),site('google','Google',[round('r',[20,null,40])])])
 const zeroY=Number(points(w)[0].attributes('cy'))
 expect(Number(w.get('.failed').attributes('y'))).toBeGreaterThan(zeroY+15)
 expect(w.get('.failed').text()).toContain('2')
 expect(w.findAll('path')[0].attributes('d')?.match(/M/g)).toHaveLength(2)
 await w.findAll('.hit')[1].trigger('mouseenter',{clientX:40,clientY:50})
 expect(hoverTip.text).toContain('Cloudflare')
 expect(hoverTip.text).toContain('Google')
 expect(hoverTip.text).toContain('synthetic timeout')
})

it('shows a first-only history round as insufficient samples rather than failure',async()=>{
 const w=chart([site('cloudflare','Cloudflare',[round('r',[40])]),site('google','Google',[])],true)
 expect(w.find('.failed').exists()).toBe(false)
 expect(w.text()).toContain('样本不足')
 expect(w.findAll('path')[0].attributes('d')).not.toMatch(/[ML]/)
 await w.get('.hit').trigger('mouseenter',{clientX:40,clientY:50})
 expect(hoverTip.text).toContain('首次 40 ms')
 expect(hoverTip.text).toContain('1/1')
 expect(hoverTip.text).toContain('Google · 0/0 响应 · 未测')
})

it('keeps partial history responses and failure evidence when no stable value exists',async()=>{
 const first=round('r',[25,null]);first.samples![0].note='已连通（HTTP 403）'
 const w=chart([site('cloudflare','Cloudflare',[first])],true)
 expect(w.text()).toContain('样本不足')
 expect(w.find('.failed').exists()).toBe(false)
 await w.get('.hit').trigger('mouseenter',{clientX:40,clientY:50})
 expect(hoverTip.text).toContain('1/2')
 expect(hoverTip.text).toContain('synthetic timeout')
})

it('renders an all-failed history round without claiming a zero response time',async()=>{
 const w=chart([site('cloudflare','Cloudflare',[round('r',[null,null])])],true)
 expect(w.find('.failed').exists()).toBe(true)
 expect(points(w)).toHaveLength(0)
 expect(w.text()).toContain('无响应')
 await w.get('.hit').trigger('mouseenter',{clientX:40,clientY:50})
 expect(hoverTip.text).toContain('0/2')
})

it('keeps saved configuration changes on the history timeline',()=>{
 const time=new Date(at-1800000).toISOString()
 const w=chart([site('cloudflare','Cloudflare',[round('old',[50,60],-3600000),round('new',[70,80])])],true,[{time,label:'配置已变化'}])
 expect(w.findAll('.config-change')).toHaveLength(1)
 expect(w.get('.config-change title').text()).toBe('配置已变化')
 expect(points(w)).toHaveLength(2)
})

it('describes the selected round beside its value instead of reusing whole-period statistics',async()=>{
 const s=site('cloudflare','Cloudflare',[round('old',[1800,2400],-3600000),round('new',[30,40])]);s.context='24 小时内稳定中位 1220 ms'
 const w=mount(InspectorLatency,{props:{series:[s]}});wrappers.push(w)
 expect(w.get('.hd-site-value').attributes('title')).not.toContain('1220')
 expect(w.get('.hd-site-value').attributes('title')).toContain('本轮')
 expect(w.get('.hd-site-name').attributes('title')).toBe('查看范围：24 小时内稳定中位 1220 ms')
 await w.findAll('.round-row')[1].trigger('click')
 expect(w.get('.hd-site-value').text()).toBe('2,400 ms')
 expect(w.get('.hd-site-value').attributes('title')).toContain('本轮')
 expect(w.get('.hd-site-name').attributes('title')).toContain('1220 ms')
})

it('renders chart readouts inside their native dialog and returns outside readouts to the page',async()=>{
 const dialog=document.createElement('dialog'),inside=document.createElement('button'),outside=document.createElement('button')
 dialog.open=true;dialog.append(inside);document.body.append(dialog,outside)
 const w=mount(HoverTip);wrappers.push(w)
 onTestFinished(()=>{hideHoverTip();dialog.remove();outside.remove()})
 for(const target of [inside,outside])target.addEventListener('mouseenter',e=>showHoverTip(e,'saved latency evidence'))
 inside.dispatchEvent(new MouseEvent('mouseenter',{clientX:80,clientY:90}));await nextTick()
 expect(dialog.querySelector('[role="tooltip"]')?.textContent).toBe('saved latency evidence')
 outside.dispatchEvent(new MouseEvent('mouseenter',{clientX:80,clientY:90}));await nextTick()
 expect(dialog.querySelector('[role="tooltip"]')).toBeNull()
 expect(document.body.querySelector(':scope > [role="tooltip"]')?.textContent).toBe('saved latency evidence')
})
