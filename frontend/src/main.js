import '@fontsource-variable/unbounded'
import '@fontsource-variable/hanken-grotesk'
import '@fontsource-variable/jetbrains-mono'
import './tailwind.css'
import './style.css'
import './app.css'
import './theme.css'
import { mount } from 'svelte'
import App from './App.svelte'

const app = mount(App, {
  target: document.getElementById('app')
})

export default app
