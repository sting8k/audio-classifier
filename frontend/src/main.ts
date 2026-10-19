import './fonts.css'
import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'

async function boot() {
  // Outside Wails (plain `vite` + a browser) there is no window.go: use the dev-only mock.
  // import.meta.env.DEV is false in `vite build`, so the mock never reaches the production bundle.
  if (import.meta.env.DEV && !window.go) await import('./dev/mock')
  mount(App, { target: document.getElementById('app')! })
}

boot()
