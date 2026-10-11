import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
// Self-hosted: these bytes ship inside the Go binary, no font CDN.
import '@fontsource-variable/geist/wght.css'
import '@fontsource-variable/geist-mono/wght.css'
import './assets/styles.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)

app.mount('#app')
