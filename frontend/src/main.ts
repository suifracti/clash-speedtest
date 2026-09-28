import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import './style.css'
import './workspace-v4.css'

const savedTheme = localStorage.getItem('theme')
if (savedTheme === 'dark') {
  document.documentElement.classList.add('dark', 'dark-theme')
  document.body.classList.add('dark', 'dark-theme')
}

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.mount('#app')

