<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from './components/Icon.svelte'
  import Folders from './screens/Folders.svelte'
  import Running from './screens/Running.svelte'
  import Results from './screens/Results.svelte'
  import Export from './screens/Export.svelte'
  import { errText } from './lib/format'
  import { go, hasRun, init, input, nav, type Screen } from './lib/store.svelte'

  const STEPS: { id: Screen; label: string }[] = [
    { id: 'folders', label: 'Input' },
    { id: 'running', label: 'Detection' },
    { id: 'results', label: 'Results' },
    { id: 'export', label: 'Export' },
  ]

  let bootError = $state('')
  onMount(() => {
    init().catch((e) => (bootError = errText(e)))
  })
</script>

<header>
  <div class="bar-inner">
    <div class="brand">
      <Icon name="logo" size={20} />
      <span>Audio Language Classifier</span>
    </div>
    <nav aria-label="Steps">
      {#each STEPS as s, i (s.id)}
        {@const locked = s.id !== 'folders' && !hasRun()}
        <button type="button" class="step" aria-current={nav.screen === s.id ? 'page' : undefined} disabled={locked} title={locked ? 'Run detection first' : undefined} onclick={() => go(s.id)}>
          <span class="num">{i + 1}</span>{s.label}
        </button>
      {/each}
    </nav>
    <span class="model mono">desert ant labs · ear 0.2.1</span>
  </div>
</header>

{#if bootError}
  <div class="boot alert" role="alert">Could not start: {bootError}</div>
{:else if input.ready}
  {#if nav.screen === 'folders'}
    <Folders />
  {:else if nav.screen === 'running'}
    <Running />
  {:else if nav.screen === 'results'}
    <Results />
  {:else}
    <Export />
  {/if}
{/if}

<style>
  header {
    position: sticky;
    top: 0;
    z-index: 10;
    background: var(--card);
    border-bottom: 1px solid var(--border);
  }
  .bar-inner {
    max-width: 1280px;
    margin: 0 auto;
    padding: 6px 24px;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 20px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    font-weight: 600;
    font-size: 15px;
  }
  nav {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    flex: 1 1 auto;
  }
  .step {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    height: 40px;
    padding: 0 14px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
  }
  .step:hover:not(:disabled):not([aria-current]) {
    background: var(--bg);
    color: var(--text);
  }
  .step:disabled {
    opacity: 0.55;
  }
  .step[aria-current] {
    background: var(--primary-bg);
    color: var(--primary);
    font-weight: 600;
  }
  .num {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    border: 1px solid currentColor;
    font-size: 12px;
  }
  .model {
    padding: 5px 10px;
    border: 1px solid var(--border);
    border-radius: 999px;
    font-size: 12px;
    color: var(--muted);
  }
  .boot {
    margin: 24px;
  }
</style>
