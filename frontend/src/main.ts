import {createApp} from 'vue'
import App from './App.vue'
import './app.css'
import './native-ui.css'
import './closeout-ui.css'
import './observatory.css'
import './design/tokens.css'
import './design/base.css'
import './design/shell.css'
import {applyTheme,savedThemePreference,followSystemTheme} from './theme'
void applyTheme(savedThemePreference()).catch(error=>console.warn('更新窗口主题失败',error))
followSystemTheme()
createApp(App).mount('#app')
