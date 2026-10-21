import {latencyTargetId} from './latencyTargets'
import type {SamplePoint,TrendPoint} from './presentation'
export function inspectorMedian(values:number[]){const a=values.slice().sort((a,b)=>a-b),i=Math.floor(a.length/2);return a.length?a.length%2?a[i]:(a[i-1]+a[i])/2:null}
export function inspectorSamples(point:TrendPoint,id:string):SamplePoint[]{return (point.sampleGroups?.filter(g=>latencyTargetId(g.target)===id).flatMap(g=>g.samples)||point.samples||[]).filter(s=>!s.executionState&&!(s.tone==='empty'&&s.value===null))}
export function stableLatency(series:{id:string;points:TrendPoint[]}[]){return inspectorMedian(series.flatMap(s=>{const latest=s.points.filter(p=>!p.id.startsWith('planned:')&&inspectorSamples(p,s.id).length).at(-1);const stable=inspectorMedian((latest?inspectorSamples(latest,s.id).slice(1):[]).flatMap(p=>p.value===null?[]:[p.value]));return stable===null?[]:[stable]}))}
