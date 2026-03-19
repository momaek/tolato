import { createApp } from 'vue'
import './style.css'
import '@/app/styles/tokens.css'
import App from './App.vue'
import { createPinia } from 'pinia'
import { router } from '@/app/router'

createApp(App).use(createPinia()).use(router).mount('#app')
