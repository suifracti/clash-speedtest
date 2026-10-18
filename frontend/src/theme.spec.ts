import {afterEach,expect,it,vi} from 'vitest'
import {applyTheme,followSystemTheme,savedTheme,savedThemePreference} from './theme'
vi.mock('./api',()=>({desktop:()=>false}))
afterEach(()=>{localStorage.clear();vi.unstubAllGlobals()})
it('persists the system preference and follows actual system changes',async()=>{
 let callback:()=>void=()=>{};const media={matches:true,addEventListener:vi.fn((_:string,fn:()=>void)=>callback=fn),removeEventListener:vi.fn()}
 vi.stubGlobal('matchMedia',()=>media)
 const stop=followSystemTheme();await applyTheme('system')
 expect(savedThemePreference()).toBe('system');expect(savedTheme()).toBe('dark');expect(document.documentElement.dataset.theme).toBe('dark')
 media.matches=false;callback();expect(document.documentElement.dataset.theme).toBe('light')
 await applyTheme('dark');callback();expect(document.documentElement.dataset.theme).toBe('dark')
 stop();expect(media.removeEventListener).toHaveBeenCalled()
})
