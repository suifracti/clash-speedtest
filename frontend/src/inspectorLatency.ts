import {latencyTargetId} from './latencyTargets'
import type {SamplePoint,TrendPoint} from './presentation'
export function inspectorMedian(values:number[]){const a=values.slice().sort((a,b)=>a-b),i=Math.floor(a.length/2);return a.length?a.length%2?a[i]:(a[i-1]+a[i])/2:null}
export function inspectorSamples(point:TrendPoint,id:string):SamplePoint[]{return (point.sampleGroups?.filter(g=>latencyTargetId(g.target)===id).flatMap(g=>g.samples)||point.samples||[]).filter(s=>!s.executionState&&!(s.tone==='empty'&&s.value===null))}
export function stableLatency(series:{id:string;points:TrendPoint[]}[]){return inspectorMedian(series.flatMap(s=>{const latest=s.points.filter(p=>!p.id.startsWith('planned:')&&inspectorSamples(p,s.id).length).at(-1);const stable=inspectorMedian((latest?inspectorSamples(latest,s.id).slice(1):[]).flatMap(p=>p.value===null?[]:[p.value]));return stable===null?[]:[stable]}))}

export function latencyFailureReason(error:string){return /timeout|timed out|deadline|超时/i.test(error)?'超时':/refused|拒绝/i.test(error)?'连接被拒绝':/reset|重置/i.test(error)?'连接被重置':/eof|closed|关闭/i.test(error)?'连接被关闭':'未响应'}
// The most frequent known reason; empty when no error says why.
export function mainLatencyFailure(errors:string[]){
 const counts=new Map<string,number>()
 for(const error of errors){const reason=latencyFailureReason(error);if(reason!=='未响应')counts.set(reason,(counts.get(reason)||0)+1)}
 return [...counts].sort((a,b)=>b[1]-a[1])[0]?.[0]||''
}
export function partialLatencyFailures(total:number,responses:number,errors:string[]){
 if(responses>=total)return ''
 const counts=new Map<string,number>()
 for(const error of errors){const reason=latencyFailureReason(error);counts.set(reason,(counts.get(reason)||0)+1)}
 if(!counts.size)counts.set('未响应',total-responses)
 return `${total} 次中 `+[...counts].map(([reason,count])=>`${count} 次${reason}`).join('，')
}
export function partialLatencyError(test?:import('./domain').LatencyTest){return test?partialLatencyFailures(test.total_samples,test.success_samples,(test.samples||[]).filter(s=>!s.success).map(s=>s.error||'')):''}
