import {onBeforeUnmount,ref} from 'vue'
import type {Airport} from './domain'

// Refresh limits belong to the airport, including its other subscriptions.
export function useUsageRefreshWindow(){
 const now=ref(Date.now())
 const timer=setInterval(()=>now.value=Date.now(),15_000)
 onBeforeUnmount(()=>clearInterval(timer))
 return (airport:Airport)=>{
  const last=Math.max(0,...(airport.subscriptions||[]).map(s=>Date.parse(s.last_attempt_at||'')||0))
  return last>0&&now.value-last<30*60*1000
 }
}
