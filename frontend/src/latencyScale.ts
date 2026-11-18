// Display-only scale. Latency statistics and verdict thresholds stay in their
// existing modules; the chart and the station ranges share these same ticks.
export interface LatencyScale {maximum:number;ticks:number[]}
export function latencyScale(values:readonly (number|null)[]):LatencyScale {
 const highest=values.reduce<number>((max,value)=>value!==null&&Number.isFinite(value)&&value>=0?Math.max(max,value):max,0)
 const target=Math.max(1,highest*1.1),rough=target/4
 const magnitude=10**Math.floor(Math.log10(rough))
 const step=([1,2,5,10].find(n=>n*magnitude>=rough)||10)*magnitude
 const count=Math.ceil(target/step)
 return {maximum:count*step,ticks:Array.from({length:count+1},(_,i)=>Number((i*step).toPrecision(10)))}
}
export const latencyTick=(value:number)=>value.toLocaleString('en-US',{maximumFractionDigits:2,useGrouping:false})
