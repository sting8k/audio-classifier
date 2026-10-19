<script lang="ts">
  import ClipDetail from '../components/ClipDetail.svelte'
  import Icon from '../components/Icon.svelte'
  import Lang from '../components/Lang.svelte'
  import StatusBadge from '../components/StatusBadge.svelte'
  import { baseName, clock, plural, prob, STATUS_LABEL } from '../lib/format'
  import { langName } from '../lib/langs'
  import { applyFilter, countBy, emptyFilter, languageShare } from '../lib/results'
  import { go, loadResults, res, run } from '../lib/store.svelte'

  const PAGE = 25
  const TOP_LANGS = 8

  const counts = $derived(countBy(res.list))
  const filtered = $derived(applyFilter(res.list, res.filter))
  const pages = $derived(Math.max(1, Math.ceil(filtered.length / PAGE)))
  const page = $derived(Math.min(res.page, pages - 1))
  const rows = $derived(filtered.slice(page * PAGE, page * PAGE + PAGE))
  const filterOn = $derived(Object.values(res.filter).some(Boolean))

  const folders = $derived([...new Set(res.list.map((c) => c.folder))])
  const folderLabel = (f: string) => (folders.filter((x) => baseName(x) === baseName(f)).length > 1 ? f : baseName(f))
  const shares = $derived(languageShare(res.list))
  const labelled = $derived(shares.reduce((s, x) => s + x.n, 0))
  const shownShares = $derived(shares.slice(0, TOP_LANGS))
  const otherShare = $derived(shares.slice(TOP_LANGS).reduce((s, x) => s + x.n, 0))
  const topN = $derived(shares[0]?.n ?? 1)

  const state = $derived(run.progress?.state ?? 'idle')
  const live = $derived(state === 'running' || state === 'paused')
  const folderCount = $derived(run.progress?.folders?.length || folders.length)

  const CARDS = [
    { key: 'total', label: 'Total clips' },
    { key: 'ok', label: 'OK' },
    { key: 'review', label: 'Needs review' },
    { key: 'no_speech', label: 'No speech' },
    { key: 'error', label: 'Errors' },
  ] as const

  const reset = () => (res.page = 0)
  function clearFilter() {
    res.filter = emptyFilter()
    reset()
  }
  function toggle(path: string) {
    res.selected = res.selected === path ? '' : path
  }
</script>

<main>
  <div class="top">
    <div>
      <h1>Detection results</h1>
      <p class="muted">
        {plural(folderCount, 'folder')} ·
        {#if state === 'done'}complete · ran for {clock(run.progress?.elapsed_s)}
        {:else if state === 'cancelled'}cancelled, showing the results so far
        {:else if live}still running, showing the results so far{/if}
        {#if live}<button type="button" class="linkbtn" onclick={loadResults} disabled={res.loading}>Refresh</button>{/if}
      </p>
    </div>
    <button type="button" class="btn primary big" onclick={() => go('export')}><Icon name="download" />Export JSON / CSV</button>
  </div>

  {#if res.error}<p class="alert" role="alert">{res.error}</p>{/if}

  <div class="cards">
    {#each CARDS as c (c.key)}
      <div class="card stat {c.key}">
        <div class="muted label">{c.label}</div>
        <div class="num mono">{counts[c.key]}</div>
      </div>
    {/each}
  </div>

  <div class="cols">
    <section class="card stack" aria-labelledby="h-list">
      <h2 id="h-list">Clips</h2>

      <div class="filters">
        <label class="search">
          <Icon name="search" />
          <span class="sr-only">Search clips</span>
          <input type="search" placeholder="Search by file name…" bind:value={res.filter.q} oninput={reset} />
        </label>
        <label class="sel">Folder
          <select class="input" bind:value={res.filter.folder} onchange={reset}>
            <option value="">All</option>
            {#each folders as f (f)}<option value={f}>{folderLabel(f)}</option>{/each}
          </select>
        </label>
        <label class="sel">Language
          <select class="input" bind:value={res.filter.lang} onchange={reset}>
            <option value="">All</option>
            {#each shares as s (s.code)}<option value={s.code}>{s.code}{langName(s.code) ? ` — ${langName(s.code)}` : ''}</option>{/each}
          </select>
        </label>
        <label class="sel">Status
          <select class="input" bind:value={res.filter.status} onchange={reset}>
            <option value="">All</option>
            {#each ['ok', 'review', 'no_speech', 'error'] as s (s)}<option value={s}>{STATUS_LABEL[s]}</option>{/each}
          </select>
        </label>
        {#if filterOn}<button type="button" class="linkbtn" onclick={clearFilter}>Clear filters</button>{/if}
      </div>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Clip</th>
              <th class="right c-dur">Duration</th>
              <th class="c-lang">Language</th>
              <th class="right c-conf">Confidence</th>
              <th class="c-2nd" title="The model’s second-best language and its probability (not a share of the clip’s time)">2nd choice</th>
              <th class="c-status">Status</th>
            </tr>
          </thead>
          <tbody>
            {#each rows as c, i (c.path + ':' + i)}
              {@const open = res.selected === c.path}
              <!-- The clip name is a real button (keyboard); the row click is only a convenience. -->
              <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
              <tr class:open class="clickable" onclick={() => toggle(c.path)}>
                <td title={c.path}>
                  <button type="button" class="rowbtn mono" aria-expanded={open} onclick={(e) => { e.stopPropagation(); toggle(c.path) }}><span class="trunc">{c.file}</span></button>
                  <span class="trunc small muted" title={c.folder}>{baseName(c.folder)}</span>
                </td>
                <td class="right mono">{clock(c.duration_s)}</td>
                <td><span class="trunc"><Lang code={c.language} /></span></td>
                <td class="right mono" class:low={c.status === 'review'}>{prob(c.confidence)}</td>
                <td>
                  {#if c.second_language}
                    <span class="mono">{c.second_language}</span> <span class="mono muted">{prob(c.second_confidence)}</span>
                  {:else}<span class="muted">—</span>{/if}
                </td>
                <td><StatusBadge status={c.status} error={c.error} /></td>
              </tr>
              {#if open}
                <tr class="detail-row"><td colspan="6"><ClipDetail clip={c} onclose={() => (res.selected = '')} /></td></tr>
              {/if}
            {:else}
              <tr><td colspan="6" class="empty muted">{res.list.length ? 'No clips match the filters.' : res.loading ? 'Loading…' : 'No results yet.'}</td></tr>
            {/each}
          </tbody>
        </table>
      </div>

      <div class="pager muted">
        <span>
          {#if filtered.length}Showing {page * PAGE + 1}–{page * PAGE + rows.length} of {plural(filtered.length, 'clip')}{:else}0 clips{/if}
          {#if filterOn} (filtered from {res.list.length}){/if}
        </span>
        <div class="pager-btns">
          <button type="button" class="btn sq" aria-label="Previous page" disabled={page === 0} onclick={() => (res.page = page - 1)}><Icon name="prev" /></button>
          <span class="mono cur">{page + 1} / {pages}</span>
          <button type="button" class="btn sq" aria-label="Next page" disabled={page >= pages - 1} onclick={() => (res.page = page + 1)}><Icon name="next" /></button>
        </div>
      </div>
    </section>

    <section class="card stack dist" aria-labelledby="h-dist">
      <div>
        <h2 id="h-dist">Language distribution</h2>
        <p class="small muted">{plural(labelled, 'clip')} with a language (OK + Needs review)</p>
      </div>
      {#if shares.length}
        <div class="bars">
          {#each shownShares as s (s.code)}
            <span class="mono code" title={langName(s.code)}>{s.code}</span>
            <div class="bar thin"><div style="width: {(s.n / topN) * 100}%"></div></div>
            <span class="mono n">{s.n} · {Math.round((s.n / labelled) * 100)}%</span>
          {/each}
          {#if otherShare}
            <span class="muted code">other</span>
            <div class="bar thin"><div style="width: {(otherShare / topN) * 100}%"></div></div>
            <span class="mono n">{otherShare} · {Math.round((otherShare / labelled) * 100)}%</span>
          {/if}
        </div>
      {:else}
        <p class="muted">No clip has a language yet.</p>
      {/if}
    </section>
  </div>
</main>

<style>
  main {
    max-width: 1280px;
    margin: 0 auto;
    padding: 20px 24px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .top {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    justify-content: space-between;
    gap: 12px;
  }
  .top p {
    margin-top: 4px;
  }
  .linkbtn {
    border: 0;
    background: none;
    padding: 0 4px;
    color: var(--primary);
    text-decoration: underline;
  }
  .linkbtn:disabled {
    opacity: 0.5;
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 12px;
  }
  .stat {
    padding: 12px 16px;
  }
  .label {
    font-size: 13px;
  }
  .num {
    font-size: 26px;
    font-weight: 600;
    line-height: 1.3;
  }
  .stat.ok .num {
    color: var(--ok);
  }
  .stat.review .num {
    color: var(--review);
  }
  .stat.no_speech .num {
    color: var(--nospeech);
  }
  .stat.error .num {
    color: var(--error);
  }

  .cols {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 300px;
    gap: 20px;
    align-items: start;
  }
  @media (max-width: 1100px) {
    .cols {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 10px;
  }
  .search {
    flex: 1 1 150px;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 8px;
    height: 40px;
    padding: 0 12px;
    border: 1px solid var(--input-border);
    border-radius: 8px;
    color: var(--muted);
  }
  .search input {
    flex: 1 1 auto;
    min-width: 0;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--text);
  }
  .search:focus-within {
    outline: 2px solid var(--primary);
    outline-offset: 2px;
  }
  .sel {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--muted);
  }
  .sel .input {
    width: 140px;
    padding: 0 6px;
    color: var(--text);
  }

  table {
    table-layout: fixed;
  }
  th,
  td {
    padding: 7px 10px;
  }
  .c-dur {
    width: 80px;
  }
  .c-lang {
    width: 160px;
  }
  .c-conf {
    width: 68px;
  }
  .c-2nd {
    width: 108px;
  }
  .c-status {
    width: 156px;
  }
  .clickable {
    cursor: pointer;
  }
  .clickable:hover {
    background: var(--subtle);
  }
  tr.open,
  tr.open:hover {
    background: var(--primary-bg);
  }
  .rowbtn {
    display: block;
    max-width: 100%;
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    font-size: 13px;
  }
  tr.open .rowbtn {
    font-weight: 500;
  }
  .detail-row td {
    background: var(--subtle);
    white-space: normal;
  }
  .low {
    color: var(--review);
  }
  .empty {
    text-align: center;
    padding: 28px;
  }

  .pager {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .pager-btns {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .btn.sq {
    width: 40px;
    padding: 0;
  }
  .cur {
    color: var(--text);
    min-width: 56px;
    text-align: center;
  }

  .bars {
    display: grid;
    grid-template-columns: 38px minmax(0, 1fr) 72px;
    align-items: center;
    gap: 8px 10px;
  }
  .code {
    font-weight: 500;
  }
  .bar.thin {
    height: 10px;
    background: #f0f0ec;
  }
  .n {
    text-align: right;
    font-size: 13px;
  }
</style>
