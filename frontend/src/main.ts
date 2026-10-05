import {createApp} from 'vue'
import App from './App.vue'
import './app.css'
import './native-ui.css'
import {applyTheme,savedTheme} from './theme'
void applyTheme(savedTheme()).catch(error=>console.warn('更新窗口主题失败',error))
createApp(App).mount('#app')
