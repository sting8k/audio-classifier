<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import { hm, parentDir, plural, skippedText } from '../lib/format'
  import { clearFolders, folderSeconds, hasRun, input, isActive, pickFiles, pickFolder, plan, removeFolder, rescanSoon, runnable, start } from '../lib/store.svelte'

  let starting = $state(false)
  let startError = $state('')

  const total = $derived(plan())
  const validFolders = $derived(runnable().length)
  const thresholdOk = $derived(input.settings.min_confidence >= 0 && input.settings.min_confidence <= 1)
  const reason = $derived(
    isActive()
      ? 'A run is in progress — see step 2.'
      : !input.paths.length
        ? 'Add at least one folder or file.'
        : input.settings.extensions.length === 0
          ? 'Select at least one file format.'
          : !thresholdOk
            ? 'The confidence threshold must be between 0 and 1.'
            : input.scanning
              ? 'Scanning…'
              : total.clips === 0
                ? 'No clips found with the selected formats.'
                : '',
  )

  const planText = $derived(plural(total.clips, 'clip') + (total.seconds > 0 ? ` · ${total.unknown ? '≥' : '~'}${hm(total.seconds)}` : ''))

  async function begin() {
    starting = true
    startError = await start()
    starting = false
  }
</script>

<!-- --wails-drop-target makes this whole screen a file-drop target (read by the Wails runtime). -->
<main style="--wails-drop-target: drop">
  <section class="card stack" aria-labelledby="h-input">
    <div class="head">
      <div>
        <h1 id="h-input">Input</h1>
        <p class="muted">Add folders or single files. Folders are scanned for the audio and video clips whose language will be detected.</p>
      </div>
      <div class="actions">
        <button type="button" class="btn" disabled={!input.paths.length} onclick={clearFolders}>Clear all</button>
        <button type="button" class="btn outline" onclick={pickFiles}><Icon name="plus" />Add files</button>
        <button type="button" class="btn outline" onclick={pickFolder}><Icon name="plus" />Add folder</button>
      </div>
    </div>

    <div class="dropzone" class:compact={input.paths.length > 0}>
      <Icon name="folder" size={input.paths.length ? 20 : 32} />
      <div class="drop-title">Drag and drop folders or audio/video files here</div>
      <div class="drop-sub">or click “Add folder” / “Add files” to pick them in Finder / File Explorer</div>
    </div>

    {#if input.error}<p class="alert" role="alert">{input.error}</p>{/if}

    {#if input.folders.length}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Folder or file</th>
              <th class="right c-clips" title="Clips matching the selected formats">Clips</th>
              <th class="c-skip">Skipped</th>
              <th class="right c-dur">Total duration</th>
              <th class="c-x"><span class="sr-only">Remove</span></th>
            </tr>
          </thead>
          <tbody>
            {#each input.folders as f (f.path)}
              {@const d = folderSeconds(f)}
              <tr>
                <td title={f.error ? `${f.path}\n${f.error}` : f.path}>
                  <span class="trunc name">{f.name}</span>
                  {#if f.error}<span class="trunc path err">{f.error}</span>{:else}<span class="trunc path mono">{parentDir(f.path)}</span>{/if}
                </td>
                <td class="right mono">{f.clips}</td>
                <td class="muted" title={skippedText(f.skipped)}><span class="trunc">{skippedText(f.skipped)}</span></td>
                <td class="right mono">
                  {#if !f.clips}<span class="muted">—</span>{:else if d == null}<span class="muted pending">calculating…</span>{:else}{hm(d)}{/if}
                </td>
                <td>
                  <button type="button" class="icon-btn" aria-label="Remove folder {f.name}" onclick={() => removeFolder(f.path)}><Icon name="x" /></button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <p class="muted">
        {plural(validFolders, 'item')} · <strong class="strong">{plural(total.clips, 'clip')}</strong>
        {#if total.seconds > 0}· {total.unknown ? 'at least ' : 'about '}{hm(total.seconds)} of audio{:else if total.unknown}· calculating duration…{/if}
        {#if input.scanning}<span class="pending">· scanning…</span>{/if}
      </p>
    {/if}
  </section>

  <aside class="card stack settings" aria-labelledby="h-settings">
    <h2 id="h-settings">Detection settings</h2>

    <fieldset>
      <legend>File formats</legend>
      <div class="chips">
        {#each input.offered as ext (ext)}
          <label class="chip mono"><input type="checkbox" value={ext} bind:group={input.settings.extensions} onchange={rescanSoon} />{ext}</label>
        {/each}
      </div>
    </fieldset>

    <label class="check strong-label"><input type="checkbox" bind:checked={input.settings.recursive} onchange={rescanSoon} />Include subfolders</label>

    <div class="field">
      <label for="conf">Confidence threshold</label>
      <input id="conf" class="input mono" type="number" min="0" max="1" step="0.05" bind:value={input.settings.min_confidence} aria-invalid={!thresholdOk} aria-describedby="conf-hint" />
      <p id="conf-hint" class="small muted">Clips below the threshold are marked “Needs review”. The 0.60 default is a guess, not measured on real data.</p>
    </div>

    <div class="go">
      <div class="plan"><span class="muted">Will process</span><span class="strong">{planText}</span></div>
      <button type="button" class="btn primary big" disabled={!!reason || starting} onclick={begin}><Icon name="play" />Start detection</button>
      {#if startError}<p class="alert" role="alert">{startError}</p>{/if}
      {#if reason}
        <p class="small muted">{reason}</p>
      {:else if hasRun()}
        <p class="small muted">A new run replaces the results of the previous run.</p>
      {/if}
      <p class="small muted">Only the first 30 seconds of each clip are analysed.</p>
    </div>
  </aside>
</main>

<style>
  main {
    max-width: 1280px;
    margin: 0 auto;
    padding: 20px 24px;
    display: grid;
    grid-template-columns: minmax(0, 1fr) clamp(280px, 26vw, 320px);
    gap: 20px;
    align-items: start;
  }
  @media (max-width: 880px) {
    main {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }
  .head p {
    margin-top: 4px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .dropzone {
    border: 2px dashed #b9b9b3;
    border-radius: 10px;
    background: var(--subtle);
    padding: 28px 20px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    text-align: center;
    color: var(--muted);
  }
  .dropzone.compact {
    flex-direction: row;
    justify-content: center;
    padding: 10px 20px;
    gap: 10px;
  }
  .dropzone.compact .drop-sub {
    display: none;
  }
  .drop-title {
    font-size: 15px;
    font-weight: 500;
    color: var(--text);
  }
  /* Class added by the Wails runtime to the drop target while a file is dragged over it. */
  main:global(.wails-drop-target-active) .dropzone {
    border-color: var(--primary);
    background: var(--primary-bg);
    color: var(--primary);
  }

  .table-wrap {
    max-height: max(168px, calc(100vh - 400px));
  }
  table {
    table-layout: fixed;
  }
  th {
    position: sticky;
    top: 0;
    z-index: 1;
  }
  th,
  td {
    padding: 7px 8px;
  }
  .c-clips {
    width: 92px;
  }
  .c-skip {
    width: 150px;
  }
  .c-dur {
    width: 112px;
  }
  .c-x {
    width: 52px;
  }
  .name {
    font-weight: 500;
  }
  .path {
    font-size: 12px;
    color: var(--muted);
  }
  .path.err {
    color: var(--error);
  }
  .pending {
    font-style: italic;
  }
  .strong {
    color: var(--text);
    font-weight: 600;
  }

  .settings {
    gap: 16px;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 36px;
    padding: 0 12px;
    border: 1px solid var(--border);
    border-radius: 999px;
    font-size: 13px;
    cursor: pointer;
  }
  .chip:has(input:checked) {
    border-color: var(--primary);
    background: var(--primary-bg);
  }
  .strong-label {
    font-weight: 500;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .field label {
    font-weight: 500;
  }
  .field .input {
    width: 140px;
  }
  .field .input[aria-invalid='true'] {
    border-color: var(--error);
  }
  .go {
    border-top: 1px solid var(--line);
    padding-top: 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .plan {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }
  .plan .strong {
    font-weight: 500;
  }
</style>
