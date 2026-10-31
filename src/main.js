import { createApp } from 'vue'
import './styles/tokens.css'
import './styles/base.css'
import './styles/glass.css'
import './styles/motion.css'
import './styles/perf.css'
import App from './App.vue'
import { initPerf } from './lib/perf.js'

initPerf()   // 首帧前套用流畅度档位，避免闪一下

createApp(App).mount('#app')
