import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from '@/App.vue'
import i18n, { initializeAppLocale } from '@/app/i18n'
import router from '@/app/router'
import AppToastViewport from '@/shared/ui/app-toast/AppToastViewport.vue'

import '@/style.css'

const app = createApp(App)

app.use(createPinia())
app.use(i18n)
app.use(router)
app.component('AppToastViewport', AppToastViewport)
initializeAppLocale()
app.mount('#app')
