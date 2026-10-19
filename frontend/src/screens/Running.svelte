<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import Lang from '../components/Lang.svelte'
  import StatusBadge from '../components/StatusBadge.svelte'
  import { baseName, clock, eta, prob, STATUS_LABEL } from '../lib/format'
  import { cancel, go, pause, resume, run } from '../lib/store.svelte'

  const p = $derived(run.progress)
  const state = $derived(p?.state ?? 'idle')
  const live = $derived(state === 'running' || state === 'paused')
  const pct = $derived(p && p.total > 0 ? Math.min(100, (p.done / p.total) * 100) : 0)
  const folders = $derived(p?.folders ?? [])
  const recent = $derived(p?.recent ?? [])
  const title = $derived(
    { running: 'Detecting languages', paused: 'Paused', done: 'Detection complete', cancelled: 'Cancelled' }[state] ?? 'Detection',
  )
  const counts = $derived(p ? ([['ok', p.summary.ok], ['review', p.summary.review], ['no_speech', p.summary.no_speech], ['error', p.summary.error]] as const) : [])

  function folderNote(f: { done: number; total: number }) {
    if (f.total > 0 && f.done >= f.total) return ' · done'
    return f.done === 0 && state === 'running' ? ' · waiting' : ''
  }
</script>

<main>
  {#if p}
    <section class="card stack" aria-labelledby="h-run">
      <div class="head">
        <div>
          <h1 id="h-run">{title}</h1>
          <p class="count mono" aria-live="polite">{p.done} <span>/ {p.total} {p.total === 1 ? 'clip' : 'clips'}</span></p>
        </div>
        <div class="actions">
          {#if live}
            {#if state === 'running'}
              <button type="button" class="btn" disabled={run.busy} onclick={pause}><Icon name="pause" />Pause</button>
            {:else}
              <button type="button" class="btn" disabled={run.busy} onclick={resume}><Icon name="play" />Resume</button>
            {/if}
            <button type="button" class="btn danger" disabled={run.busy} onclick={cancel}>Cancel</button>
          {:else}
            <button type="button" class="btn primary" onclick={() => go('results')}>View results</button>
          {/if}
        </div>
      </div>

      {#if run.error}<p class="alert" role="alert">{run.error}</p>{/if}

      <div class="bar big" role="progressbar" aria-label="Overall progress" aria-valuemin="0" aria-valuemax={p.total} aria-valuenow={p.done}><div style="width: {pct}%"></div></div>

      <div class="meta muted">
        <span>Elapsed <strong class="mono">{clock(p.elapsed_s)}</strong></span>
        {#if live && state === 'running' && p.eta_s != null}<span>Estimated time left <strong class="mono">{eta(p.eta_s)}</strong></span>{/if}
        {#each counts as [k, n] (k)}
          <span class="count-chip"><span class="dot {k}"></span>{STATUS_LABEL[k]} <strong class="mono">{n}</strong></span>
        {/each}
      </div>

      {#if live}
        <div class="current">
          {#if p.current}
            <span>Processing <strong class="mono" title={p.current}>{baseName(p.current)}</strong></span>
            <span class="muted mono small cur-path" title={p.current}>{p.current}</span>
          {:else}
            <span class="muted">{state === 'paused' ? 'Paused.' : 'Preparing…'}</span>
          {/if}
        </div>
      {/if}
    </section>

    <div class="cols">
      <section class="card stack" aria-labelledby="h-folders">
        <h2 id="h-folders">By folder</h2>
        {#each folders as f (f.path)}
          <div class="folder">
            <div class="row">
              <span class="fname" title={f.path}>{f.name}</span>
              <span class="mono muted">{f.done} / {f.total}{folderNote(f)}</span>
            </div>
            <div class="bar" role="progressbar" aria-label="Progress of {f.name}" aria-valuemin="0" aria-valuemax={f.total} aria-valuenow={f.done}><div style="width: {f.total ? Math.min(100, (f.done / f.total) * 100) : 0}%"></div></div>
          </div>
        {:else}
          <p class="muted">No folder information yet.</p>
        {/each}
      </section>

      <section class="card stack" aria-labelledby="h-log">
        <div class="log-head">
          <h2 id="h-log">Just finished</h2>
          <button type="button" class="btn link" onclick={() => go('results')}>View results so far →</button>
        </div>
        {#if recent.length}
          <div class="table-wrap">
            <table>
              <thead><tr><th>Clip</th><th>Language</th><th class="right">Confidence</th><th>Status</th></tr></thead>
              <tbody>
                {#each recent as c, i (c.path + ':' + i)}
                  <tr>
                    <td class="mono clip" title={c.path}><span class="trunc">{c.file}</span></td>
                    <td><Lang code={c.language} /></td>
                    <td class="right mono" class:low={c.status === 'review'}>{prob(c.confidence)}</td>
                    <td class="status"><StatusBadge status={c.status} error={c.error} /></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {:else}
          <p class="muted">No clips finished yet.</p>
        {/if}
      </section>
    </div>
  {/if}
</main>

<style>
  main {
    max-width: 1280px;
    margin: 0 auto;
    padding: 20px 24px;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
  }
  .count {
    margin-top: 2px;
    font-size: 28px;
    font-weight: 600;
  }
  .count span {
    font-size: 18px;
    font-weight: 400;
    color: var(--muted);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .bar.big {
    height: 12px;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 24px;
  }
  .meta strong {
    color: var(--text);
    font-weight: 500;
  }
  .count-chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }
  .dot.ok {
    background: var(--ok);
  }
  .dot.review {
    background: #d9822b;
  }
  .dot.no_speech {
    background: #9a9a95;
  }
  .dot.error {
    background: var(--error);
  }
  .current {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 2px 16px;
    padding: 12px 14px;
    background: var(--subtle);
    border: 1px solid var(--line);
    border-radius: 8px;
    min-width: 0;
  }
  .cur-path {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1 1 200px;
  }

  .cols {
    display: grid;
    grid-template-columns: minmax(280px, 1fr) minmax(0, 2fr);
    gap: 20px;
    align-items: start;
  }
  @media (max-width: 760px) {
    .cols {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  .folder {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
  }
  .fname {
    font-weight: 500;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row .mono {
    flex: none;
  }
  .log-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .btn.link {
    border: 0;
    background: none;
    color: var(--primary);
    padding: 0;
  }
  .btn.link:hover:not(:disabled) {
    background: none;
    color: var(--primary-dark);
  }
  .clip {
    max-width: 280px;
  }
  .status {
    max-width: 240px;
  }
  .low {
    color: var(--review);
  }
</style>
