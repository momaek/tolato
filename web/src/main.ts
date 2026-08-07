import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n from './i18n'
import { useAppStore } from './stores/app'
import './assets/styles/index.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n)

// Re-read the identity behind the stored token before the first route resolves,
// so a role changed while the user was away is reflected in this session rather
// than at token expiry.
void useAppStore().refreshUser()

app.mount('#app')
