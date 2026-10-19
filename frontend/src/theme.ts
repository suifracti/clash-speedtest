import {desktop} from './api'
export type Theme = 'light' | 'dark'
export type ThemePreference = Theme | 'system'
export function savedThemePreference():ThemePreference {
 try { const value=window.localStorage.getItem('theme');return value==='dark'||value==='system'?value:'light' } catch { return 'light' }
}
export function savedTheme():Theme {return resolveTheme(savedThemePreference())}
function resolveTheme(theme:ThemePreference):Theme {return theme==='system'?(window.matchMedia?.('(prefers-color-scheme: dark)').matches?'dark':'light'):theme}
async function renderTheme(theme:Theme){
 document.documentElement.dataset.theme=theme
 document.querySelector('meta[name="theme-color"]')?.setAttribute('content',theme==='dark'?'#1c1c1e':'#f5f5f7')
 if(desktop()){const update=window.go?.desktop?.App?.SetWindowTheme;if(update)await update(theme)}
}
export async function applyTheme(theme:ThemePreference){
 try{window.localStorage.setItem('theme',theme)}catch{/* Apply in this window even without storage. */}
 await renderTheme(resolveTheme(theme))
}
export function followSystemTheme(){
 const media=window.matchMedia?.('(prefers-color-scheme: dark)')
 const change=()=>{if(savedThemePreference()==='system')void renderTheme(resolveTheme('system')).catch(error=>console.warn('更新窗口主题失败',error))}
 media?.addEventListener('change',change)
 return ()=>media?.removeEventListener('change',change)
}
