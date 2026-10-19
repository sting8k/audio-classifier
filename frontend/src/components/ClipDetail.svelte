<script lang="ts">
  import { OpenFileLocation } from '../../wailsjs/go/main/App.js'
  import { clock, errText, prob } from '../lib/format'
  import { langName } from '../lib/langs'
  import type { Clip } from '../lib/results'
  import Icon from './Icon.svelte'
  import Lang from './Lang.svelte'
  import StatusBadge from './StatusBadge.svelte'

  let { clip, onclose }: { clip: Clip; onclose: () => void } = $props()

  let openError = $state('')
  async function reveal() {
    openError = ''
    try {
      await OpenFileLocation(clip.path)
    } catch (e) {
      openError = errText(e)
    }
  }
</script>

<div class="detail" role="region" aria-label="Details of {clip.file}">
  <div class="top">
    <div class="who">
      <h3 class="mono">{clip.file}</h3>
      <p class="mono small muted path">{clip.path}</p>
    </div>
    <button type="button" class="icon-btn" aria-label="Close details" onclick={onclose}><Icon name="x" /></button>
  </div>

  <dl>
    <div><dt>Language</dt><dd><Lang code={clip.language} /></dd></div>
    <div><dt>Confidence</dt><dd class="mono">{prob(clip.confidence)}</dd></div>
    <div>
      <dt>2nd choice</dt>
      <dd>
        {#if clip.second_language}
          <span class="mono">{clip.second_language}</span> {langName(clip.second_language)} <span class="mono muted">· {prob(clip.second_confidence)}</span>
        {:else}—{/if}
      </dd>
    </div>
    <div><dt>Speech (in the first 30 s)</dt><dd class="mono">{clip.speech_s != null ? clock(clip.speech_s) : '—'}</dd></div>
    <div><dt>Clip duration</dt><dd class="mono">{clock(clip.duration_s)}</dd></div>
    <div><dt>Status</dt><dd><StatusBadge status={clip.status} /></dd></div>
  </dl>

  {#if clip.error}<p class="alert">{clip.error}</p>{/if}
  {#if clip.second_language}<p class="small muted">“2nd choice” is the model’s second-best language and its probability, not a share of the clip’s time.</p>{/if}

  <div class="foot">
    <button type="button" class="btn" onclick={reveal}><Icon name="folder" />Open file location</button>
    {#if openError}<span class="small err" role="alert">{openError}</span>{/if}
  </div>
</div>

<style>
  .detail {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 4px 4px 8px;
    white-space: normal;
  }
  .top {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }
  .who {
    min-width: 0;
  }
  h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .path {
    overflow-wrap: anywhere;
  }
  dl {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 10px 28px;
  }
  dt {
    font-size: 12px;
    color: var(--muted);
  }
  dd {
    margin: 0;
    font-weight: 600;
  }
  dd :global(.muted) {
    font-weight: 400;
  }
  .foot {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
  }
  .err {
    color: var(--error);
  }
</style>
