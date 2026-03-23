import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from '@/App.vue'
import i18n, { initializeAppLocale } from '@/app/i18n'
import router from '@/app/router'
import { useAuthStore } from '@/entities/auth/model/auth.store'
import AppToastViewport from '@/shared/ui/app-toast/AppToastViewport.vue'

import '@/style.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(i18n)
useAuthStore(pinia).hydrate()
app.use(router)
app.component('AppToastViewport', AppToastViewport)
initializeAppLocale()
app.mount('#app')
