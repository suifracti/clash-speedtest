import {shallowRef} from 'vue'

export interface LatencySampleTipRow {
 seq:string
 text:string
 badge?:string
 tone:string
}

export interface OverviewLatencyTipSite {
 name:string
 value:string
 count:string
 badge?:string
 tone:string
}

export interface UnlockTipItem {
 name:string
 label:string
 tone:string
 reason?:string
}

export interface UnlockTipGroup {
 name:string
 ratio:string
 items:UnlockTipItem[]
}

export type ChartTipPayload=
 | {kind:'latency-samples';title:string;time?:string;httpNote?:string;samples:LatencySampleTipRow[];x:number;y:number;below:boolean}
 | {kind:'overview-latency';title:string;sub?:string;time?:string;sites:OverviewLatencyTipSite[];compare?:{text:string;tone:string};x:number;y:number;below:boolean}
 | {kind:'unlock-detail';title:string;groups:UnlockTipGroup[];x:number;y:number;below:boolean}

export const activeChartTip=shallowRef<ChartTipPayload|null>(null)

const dismissCallbacks=new Set<()=>void>()
let scrollBound=false

function onGlobalScroll(){
 if(activeChartTip.value)activeChartTip.value=null
 if(dismissCallbacks.size){
  for(const fn of [...dismissCallbacks])fn()
 }
 syncScrollListener()
}

function syncScrollListener(){
 if(typeof window==='undefined')return
 const needed=!!activeChartTip.value||dismissCallbacks.size>0
 if(needed&&!scrollBound){
  window.addEventListener('scroll',onGlobalScroll,{capture:true,passive:true})
  scrollBound=true
 }else if(!needed&&scrollBound){
  window.removeEventListener('scroll',onGlobalScroll,true)
  scrollBound=false
 }
}

export function computeTipAnchor(el:HTMLElement|null,halfWidth=140,flipThreshold=160){
 if(!el)return null
 const rect=el.getBoundingClientRect()
 const vw=typeof window!=='undefined'?window.innerWidth:1280
 const x=Math.max(halfWidth,Math.min(vw-halfWidth,rect.left+rect.width/2))
 const below=rect.top<flipThreshold
 return {x,y:below?rect.bottom:rect.top,below}
}

export function showChartTip(payload:ChartTipPayload){
 activeChartTip.value=payload
 syncScrollListener()
}

export function hideChartTip(){
 if(!activeChartTip.value)return
 activeChartTip.value=null
 syncScrollListener()
}

export function registerTipDismiss(fn:()=>void){
 dismissCallbacks.add(fn)
 syncScrollListener()
 return ()=>{
  dismissCallbacks.delete(fn)
  syncScrollListener()
 }
}
