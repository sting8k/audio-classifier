/// <reference types="svelte" />
/// <reference types="vite/client" />

// Injected by Wails before the page loads; absent in a plain browser.
interface Window {
  go?: unknown
}
