<script lang="ts">
  import { STATUS_LABEL } from '../lib/format'
  let { status, error = '' }: { status: string; error?: string } = $props()
  const text = $derived((STATUS_LABEL[status] ?? status) + (status === 'error' && error ? `: ${error}` : ''))
</script>

<span class="badge {status}" title={text}><span class="trunc">{text}</span></span>

<style>
  .trunc {
    min-width: 0;
  }
</style>
