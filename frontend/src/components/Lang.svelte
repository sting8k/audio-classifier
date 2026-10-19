<script lang="ts">
  import { langName } from '../lib/langs'
  let { code, names = true }: { code: string | null | undefined; names?: boolean } = $props()
</script>

{#if code}
  <span class="mono code">{code}</span>
  {#if names && langName(code)}<span class="muted">{langName(code)}</span>{/if}
{:else}
  <span class="muted">—</span>
{/if}

<style>
  .code {
    font-weight: 500;
  }
</style>
