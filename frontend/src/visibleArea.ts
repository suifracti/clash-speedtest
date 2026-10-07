// One observer for all deferred bars; navigation/unmount releases every target.
const callbacks=new Map<Element,(visible:boolean)=>void>()
let observer:IntersectionObserver|undefined
export function observeVisibleArea(element:Element,change:(visible:boolean)=>void){
  if(typeof IntersectionObserver==='undefined'){change(true);return ()=>{}}
  if(!observer)observer=new IntersectionObserver(entries=>{for(const entry of entries)callbacks.get(entry.target)?.(entry.isIntersecting)},{rootMargin:'240px 0px',threshold:0})
  callbacks.set(element,change);observer.observe(element)
  return ()=>{
    callbacks.delete(element);observer?.unobserve(element)
    if(!callbacks.size){observer?.disconnect();observer=undefined}
  }
}
