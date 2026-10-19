<script lang="ts">
  import { Export, OpenFileLocation, PickExportPath } from '../../wailsjs/go/main/App.js'
  import Icon from '../components/Icon.svelte'
  import { baseName, errText } from '../lib/format'
  import { applyFilter, countBy, csvPreview, EXPORT_FIELDS, inScope, jsonPreview } from '../lib/results'
  import { exp, go, res, run } from '../lib/store.svelte'

  const folders = $derived([...new Set(res.list.map((c) => c.folder))])
  const folderLabel = (f: string) => (folders.filter((x) => baseName(x) === baseName(f)).length > 1 ? f : baseName(f))
  const clips = $derived(inScope(res.list, exp.scope, res.filter, exp.folders))
  const filterCount = $derived(applyFilter(res.list, res.filter).length)
  const fields = $derived(EXPORT_FIELDS.map((f) => f.key).filter((k) => exp.fields.includes(k)))
  const filterOn = $derived(Object.values(res.filter).some(Boolean))

  const today = new Date().toLocaleDateString('sv-SE') // YYYY-MM-DD in local time
  const defaultName = $derived(`language-results_${today}.${exp.format}`)
  const preview = $derived(
    exp.format === 'json'
      ? jsonPreview(clips.slice(0, 3), fields, clips.length, run.minMargin, countBy(clips))
      : csvPreview(clips.slice(0, 6), fields, exp.bom),
  )
  const blocked = $derived(
    clips.length === 0 ? (exp.scope === 'folders' ? 'Select at least one folder.' : 'No clips in this scope.') : fields.length === 0 ? 'Select at least one field.' : '',
  )

  function setFormat() {
    exp.out = exp.out.replace(/\.(json|csv)$/i, `.${exp.format}`)
  }

  async function pick(): Promise<string> {
    try {
      let p = await PickExportPath(defaultName)
      if (p && !/\.[^./\\]+$/.test(p)) p += `.${exp.format}`
      if (p) exp.out = p
      return p
    } catch (e) {
      exp.error = errText(e)
      return ''
    }
  }

  async function doExport() {
    exp.error = ''
    exp.done = ''
    const out = exp.out || (await pick())
    if (!out) return
    exp.busy = true
    try {
      await Export({
        format: exp.format,
        // Empty paths means "every result" to the backend, so only the "all" scope may send it empty
        // (the other scopes are blocked above when they select nothing).
        paths: exp.scope === 'all' ? [] : clips.map((c) => c.path),
        fields: [...fields],
        bom: exp.format === 'csv' && exp.bom,
        out,
      })
      exp.done = out
    } catch (e) {
      exp.error = errText(e)
    } finally {
      exp.busy = false
    }
  }

  async function reveal() {
    try {
      await OpenFileLocation(exp.done)
    } catch (e) {
      exp.error = errText(e)
    }
  }
</script>

<main>
  <section class="card stack form" aria-labelledby="h-export">
    <h1 id="h-export">Export results</h1>

    <fieldset>
      <legend>Format</legend>
      <div class="formats">
        <label class="fmt" class:on={exp.format === 'json'}>
          <input type="radio" name="fmt" value="json" bind:group={exp.format} onchange={setFormat} />
          <span><span class="fmt-name mono">JSON</span><span class="small muted">Structured, with a header: time, settings and summary counts.</span></span>
        </label>
        <label class="fmt" class:on={exp.format === 'csv'}>
          <input type="radio" name="fmt" value="csv" bind:group={exp.format} onchange={setFormat} />
          <span><span class="fmt-name mono">CSV</span><span class="small muted">One row per clip; opens in Excel, Numbers, Google Sheets.</span></span>
        </label>
      </div>
    </fieldset>

    <fieldset>
      <legend>Scope</legend>
      <label class="check"><input type="radio" name="scope" value="all" bind:group={exp.scope} />All clips <span class="muted">({res.list.length})</span></label>
      <label class="check"><input type="radio" name="scope" value="filter" bind:group={exp.scope} />Current filter on the Results screen <span class="muted">({filterCount}{filterOn ? '' : ', no filter set'})</span></label>
      <label class="check"><input type="radio" name="scope" value="folders" bind:group={exp.scope} />Selected folders only</label>
      <div class="folder-pick" class:off={exp.scope !== 'folders'}>
        {#each folders as f (f)}
          <label class="check small-check" title={f}><input type="checkbox" value={f} bind:group={exp.folders} disabled={exp.scope !== 'folders'} /><span class="trunc">{folderLabel(f)}</span></label>
        {/each}
      </div>
    </fieldset>

    <fieldset>
      <legend>Fields</legend>
      <div class="fields">
        {#each EXPORT_FIELDS as f (f.key)}
          <label class="check small-check"><input type="checkbox" value={f.key} bind:group={exp.fields} />{f.label}</label>
        {/each}
      </div>
    </fieldset>

    {#if exp.format === 'csv'}
      <label class="bom">
        <input type="checkbox" bind:checked={exp.bom} />
        <span><span class="bom-t">CSV for Excel</span><span class="small muted">UTF-8 with a BOM and “;” as the separator, so Excel on Windows shows accented names and splits the columns correctly. Turn off for plain UTF-8 with commas.</span></span>
      </label>
    {/if}

    <div class="field">
      <label for="out-path">Save to</label>
      <div class="pathrow">
        <input id="out-path" class="input mono" type="text" readonly value={exp.out} placeholder={defaultName} />
        <button type="button" class="btn" onclick={pick}>Browse…</button>
      </div>
    </div>

    {#if exp.error}<p class="alert" role="alert">{exp.error}</p>{/if}
    {#if exp.done}
      <p class="ok-note" role="status">
        Saved <span class="mono">{baseName(exp.done)}</span>.
        <button type="button" class="linkbtn" onclick={reveal}>Open file location</button>
      </p>
    {/if}

    <div class="foot">
      {#if blocked}<span class="small muted">{blocked}</span>{/if}
      <button type="button" class="btn" onclick={() => go('results')}>Back to results</button>
      <button type="button" class="btn primary" disabled={!!blocked || exp.busy} onclick={doExport}><Icon name="download" />Export {exp.format === 'json' ? 'JSON' : 'CSV'} <span class="mono">({clips.length})</span></button>
    </div>
  </section>

  <section class="card stack" aria-labelledby="h-preview">
    <div>
      <h2 id="h-preview">Preview</h2>
      <p class="small muted">The first few clips with the current options. Illustrates the layout; the app writes the real file.</p>
    </div>
    <div class="pv">
      <div class="small muted mono">{exp.out ? baseName(exp.out) : defaultName}</div>
      {#if clips.length && fields.length}
        <pre>{preview}</pre>
      {:else}
        <p class="muted">Nothing to preview.</p>
      {/if}
    </div>
  </section>
</main>

<style>
  main {
    max-width: 1280px;
    margin: 0 auto;
    padding: 20px 24px;
    display: grid;
    grid-template-columns: minmax(380px, 440px) minmax(0, 1fr);
    gap: 20px;
    align-items: start;
  }
  @media (max-width: 880px) {
    main {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  .form {
    gap: 18px;
  }
  .formats {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }
  .fmt {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    padding: 12px;
    border: 1px solid var(--input-border);
    border-radius: 10px;
    cursor: pointer;
  }
  .fmt.on {
    border: 2px solid var(--primary);
    padding: 11px;
    background: #f5f8fe;
  }
  .fmt input {
    margin-top: 2px;
  }
  .fmt-name {
    display: block;
    font-weight: 600;
  }
  .fmt span.small {
    display: block;
  }
  .folder-pick {
    display: flex;
    flex-wrap: wrap;
    gap: 0 16px;
    padding-left: 28px;
  }
  .folder-pick.off {
    color: var(--muted);
  }
  .small-check {
    min-height: 32px;
    min-width: 0;
  }
  .fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0 12px;
  }
  .bom {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--subtle);
    cursor: pointer;
  }
  .bom input {
    margin-top: 2px;
  }
  .bom-t {
    display: block;
    font-weight: 500;
  }
  .bom .small {
    display: block;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .field > label {
    font-weight: 500;
  }
  .pathrow {
    display: flex;
    gap: 8px;
  }
  .pathrow .input {
    flex: 1 1 auto;
    font-size: 13px;
  }
  .ok-note {
    padding: 10px 12px;
    border-radius: 8px;
    background: var(--ok-bg);
    color: var(--ok);
  }
  .linkbtn {
    border: 0;
    background: none;
    padding: 0 4px;
    color: var(--primary);
    text-decoration: underline;
  }
  /* Stays at the bottom of the window so Export is reachable on a 640px-high window. */
  .foot {
    position: sticky;
    bottom: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    margin: 0 -20px -20px;
    padding: 12px 20px;
    background: var(--card);
    border-top: 1px solid var(--line);
    border-radius: 0 0 12px 12px;
  }
  .foot .small {
    margin-right: auto;
  }
  .pv {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }
  pre {
    margin: 0;
    padding: 14px 16px;
    background: #f7f7f4;
    border: 1px solid var(--line);
    border-radius: 8px;
    overflow: auto;
    max-height: max(320px, calc(100vh - 230px));
    font-family: var(--mono);
    font-size: 12.5px;
    line-height: 1.55;
  }
</style>
