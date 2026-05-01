<script>
  import { onMount } from 'svelte';
  import {
    getHaloscanStatus,
    syncHaloscan,
    getHaloscanOverview,
    getHaloscanPositions,
    getHaloscanCompetitors,
    getHaloscanTrends,
    getHaloscanGap,
  } from '../api.js';
  import { fmtN } from '../utils.js';

  let { projectId, projectName, onerror } = $props();

  let status = $state(null);
  let overview = $state(null);
  let positions = $state([]);
  let competitors = $state([]);
  let trends = $state([]);
  let gapMissing = $state([]);
  let gapBested = $state([]);
  let loading = $state(true);
  let syncing = $state(false);
  let domainInput = $state(guessDomain(projectName));
  let syncError = $state('');
  let pollTimer = null;
  let pollStartedAt = 0;

  function guessDomain(name) {
    if (!name) return '';
    let d = String(name).trim().toLowerCase();
    d = d.replace(/^sc-domain:/, '');
    d = d.replace(/^https?:\/\//, '');
    d = d.replace(/^www\./, '');
    d = d.replace(/\/.*$/, '');
    return d;
  }

  const PALETTE = ['#2563EB', '#10B981', '#8B5CF6', '#F59E0B'];

  function intentLabel(p) {
    if (p.si_brand) return { label: 'Marque', color: '#6B21A8' };
    if (p.si_local) return { label: 'Locale', color: '#0F766E' };
    if (p.si_trans) return { label: 'Transactionnelle', color: '#B45309' };
    if (p.si_comm) return { label: 'Commerciale', color: '#A16207' };
    if (p.si_nav) return { label: 'Navigationnelle', color: '#475569' };
    if (p.si_info) return { label: 'Informationnelle', color: '#1D4ED8' };
    return { label: '—', color: '#94A3B8' };
  }

  function positionPill(pos) {
    if (pos <= 3) return '#15803D';
    if (pos <= 10) return '#65A30D';
    if (pos <= 30) return '#D97706';
    return '#64748B';
  }

  function shorten(url, n = 50) {
    if (!url) return '';
    if (url.length <= n) return url;
    return url.slice(0, n - 1) + '…';
  }

  function urlPath(url) {
    try {
      const u = new URL(url);
      return decodeURIComponent(u.pathname + u.search);
    } catch (_) {
      return url;
    }
  }

  async function loadAll() {
    loading = true;
    try {
      status = await getHaloscanStatus(projectId);
      if (status?.has_data) {
        const [ov, pos, cp, tr, gm, gb] = await Promise.all([
          getHaloscanOverview(projectId).catch(() => null),
          getHaloscanPositions(projectId, 5000, 100).catch(() => ({ positions: [] })),
          getHaloscanCompetitors(projectId, 20).catch(() => ({ competitors: [] })),
          getHaloscanTrends(projectId).catch(() => ({ trends: [] })),
          getHaloscanGap(projectId, 'missing', 200).catch(() => ({ keywords: [] })),
          getHaloscanGap(projectId, 'bested', 200).catch(() => ({ keywords: [] })),
        ]);
        overview = ov;
        positions = pos.positions || [];
        competitors = cp.competitors || [];
        trends = tr.trends || [];
        gapMissing = gm.keywords || [];
        gapBested = gb.keywords || [];
        // Use the stored domain as the default for resync.
        if (ov?.domain) domainInput = ov.domain;
      }
    } catch (e) {
      onerror?.(e.message);
    } finally {
      loading = false;
    }
  }

  const SYNC_TIMEOUT_MS = 120_000; // 2 min — assez pour 5 endpoints Haloscan + 392+ rows ClickHouse

  async function startSync() {
    if (!domainInput || !domainInput.trim()) {
      onerror?.('Renseignez un domaine avant de synchroniser');
      return;
    }
    syncing = true;
    syncError = '';
    pollStartedAt = Date.now();
    try {
      await syncHaloscan(projectId, { domain: domainInput.trim(), positionMax: 100 });
      pollTimer = setInterval(async () => {
        try {
          const s = await getHaloscanStatus(projectId);
          if (s?.has_data) {
            clearInterval(pollTimer);
            pollTimer = null;
            syncing = false;
            await loadAll();
            return;
          }
        } catch (_) {
          // ignore transient errors during sync
        }
        if (Date.now() - pollStartedAt > SYNC_TIMEOUT_MS) {
          clearInterval(pollTimer);
          pollTimer = null;
          syncing = false;
          syncError =
            'Synchronisation lente ou échouée après 2 min. Vérifiez la clé Haloscan et le domaine, puis consultez les logs serveur (onglet Logs).';
        }
      }, 3000);
    } catch (e) {
      syncing = false;
      onerror?.(e.message);
    }
  }

  // ---- Aggregation: top 10 URLs × top 5 KW ----
  const topUrls = $derived.by(() => {
    if (!positions.length) return [];
    const byUrl = new Map();
    for (const p of positions) {
      const e = byUrl.get(p.url) || { url: p.url, kws: [], total_kw: 0, top10: 0, traffic: 0 };
      e.kws.push(p);
      e.total_kw++;
      if (p.position <= 10) e.top10++;
      e.traffic += Number(p.traffic) || 0;
      byUrl.set(p.url, e);
    }
    const all = [...byUrl.values()];
    for (const e of all) {
      e.kws.sort((a, b) => (Number(b.traffic) || 0) - (Number(a.traffic) || 0));
      e.top_kws = e.kws.slice(0, 5);
      e.dominant = dominantIntent(e.kws);
    }
    all.sort((a, b) => b.traffic - a.traffic);
    return all.slice(0, 10);
  });

  function dominantIntent(kws) {
    const counts = {};
    for (const p of kws) {
      const i = intentLabel(p).label;
      counts[i] = (counts[i] || 0) + 1;
    }
    const sorted = Object.entries(counts).sort((a, b) => b[1] - a[1]);
    return sorted.length ? sorted[0][0] : '—';
  }

  // ---- Visibility chart helpers ----
  const trendSeries = $derived.by(() => {
    if (!trends.length) return [];
    const byDomain = new Map();
    for (const p of trends) {
      const arr = byDomain.get(p.series_domain) || [];
      arr.push(p);
      byDomain.set(p.series_domain, arr);
    }
    return [...byDomain.entries()].map(([d, pts], i) => ({
      domain: d,
      color: i === 0 ? '#F97316' : PALETTE[(i - 1) % PALETTE.length],
      points: pts.sort((a, b) => new Date(a.agg_date) - new Date(b.agg_date)),
    }));
  });

  const trendSVG = $derived.by(() => {
    if (!trendSeries.length) return null;
    const W = 960;
    const H = 280;
    const PADL = 56;
    const PADR = 16;
    const PADT = 16;
    const PADB = 36;
    let allDates = [];
    let allVals = [];
    for (const s of trendSeries) {
      for (const p of s.points) {
        allDates.push(new Date(p.agg_date).getTime());
        allVals.push(Number(p.visibility_index) || 0);
      }
    }
    if (!allDates.length) return null;
    const xMin = Math.min(...allDates);
    const xMax = Math.max(...allDates);
    let yMax = Math.max(1, ...allVals);
    // round yMax up to a "nice" tick
    const niceCeil = (v) => {
      const exp = Math.pow(10, Math.floor(Math.log10(v)));
      const f = v / exp;
      const niced = f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10;
      return niced * exp;
    };
    yMax = niceCeil(yMax * 1.1);
    const yTicks = 5;
    const x = (t) => PADL + ((t - xMin) / (xMax - xMin)) * (W - PADL - PADR);
    const y = (v) => H - PADB - (v / yMax) * (H - PADT - PADB);
    const paths = trendSeries.map((s) => {
      const d = s.points
        .map(
          (p, i) =>
            `${i === 0 ? 'M' : 'L'}${x(new Date(p.agg_date).getTime()).toFixed(1)},${y(Number(p.visibility_index) || 0).toFixed(1)}`,
        )
        .join(' ');
      return { ...s, path: d };
    });
    const yLabels = [];
    for (let i = 0; i <= yTicks; i++) {
      const v = (yMax / yTicks) * i;
      yLabels.push({ v, y: y(v) });
    }
    // X-axis: ~6 evenly spaced date ticks
    const xTicks = 6;
    const xLabels = [];
    for (let i = 0; i <= xTicks; i++) {
      const t = xMin + ((xMax - xMin) / xTicks) * i;
      const date = new Date(t);
      const label = date.toLocaleDateString('fr-FR', { month: 'short', year: '2-digit' });
      xLabels.push({ t, x: x(t), label });
    }
    return { W, H, PADL, PADR, PADT, PADB, paths, xMin, xMax, yMax, yLabels, xLabels };
  });

  onMount(() => {
    loadAll();
    return () => {
      if (pollTimer) clearInterval(pollTimer);
    };
  });
</script>

<div class="haloscan-root">
  {#if loading}
    <div class="empty-state"><p>Chargement…</p></div>
  {:else if !status?.connected}
    <div class="empty-state">
      <h3>API Haloscan non connectée</h3>
      <p class="text-muted text-sm">
        Aucune clé API détectée. Définissez <code>haloscan.api_key</code> dans
        <code>config.yaml</code>, ou la variable d'environnement <code>HALOSCAN_API_KEY</code>, ou
        configurez le MCP
        <code>haloscan-lucky</code> dans Claude Desktop.
      </p>
    </div>
  {:else if !status?.has_data}
    <div class="empty-state">
      <h3>Aucune donnée Haloscan pour ce projet</h3>
      <p class="text-muted text-sm">
        Source de la clé : <strong>{status.key_source}</strong>. Saisissez le domaine racine (sans
        <code>https://</code>, sans <code>www.</code>, ex&nbsp;:
        <code>singular-is-future.com</code>) puis lancez la synchronisation : positions ≤100, top 10
        concurrents, courbe visibilité benchmarkée sur ~2,5 ans, gap analysis (manqués + bested).
      </p>
      <div class="domain-row">
        <input
          type="text"
          class="domain-input"
          bind:value={domainInput}
          placeholder="exemple.com"
          disabled={syncing}
        />
        <button
          class="btn btn-primary"
          disabled={syncing || !domainInput.trim()}
          onclick={startSync}
        >
          {syncing ? 'Synchronisation en cours…' : 'Synchroniser depuis Haloscan'}
        </button>
      </div>
      {#if syncError}
        <p class="sync-error">{syncError}</p>
      {/if}
    </div>
  {:else}
    <!-- KPI strip -->
    <div class="kpi-row">
      <div class="kpi">
        <div class="kpi-label">Indice de visibilité</div>
        <div class="kpi-value">{(overview?.visibility_index || 0).toFixed(1)}</div>
      </div>
      <div class="kpi">
        <div class="kpi-label">Mots-clés positionnés</div>
        <div class="kpi-value">{fmtN(overview?.total_keyword_count || 0)}</div>
      </div>
      <div class="kpi">
        <div class="kpi-label">Top 10</div>
        <div class="kpi-value">{fmtN(overview?.top_10_positions || 0)}</div>
      </div>
      <div class="kpi">
        <div class="kpi-label">Concurrents identifiés</div>
        <div class="kpi-value">{competitors.length}</div>
      </div>
      <div class="kpi-actions">
        <button class="btn btn-sm" disabled={syncing} onclick={startSync}>
          {syncing ? 'Sync…' : 'Resynchroniser'}
        </button>
      </div>
    </div>

    <!-- Visibility benchmark chart -->
    {#if trendSVG}
      <section class="block">
        <h3>Visibilité benchmarkée vs concurrents</h3>
        <div class="chart-wrap">
          <svg
            viewBox="0 0 {trendSVG.W} {trendSVG.H}"
            class="trend-svg"
            preserveAspectRatio="xMidYMid meet"
          >
            <!-- Y gridlines + labels -->
            {#each trendSVG.yLabels as t}
              <line
                x1={trendSVG.PADL}
                x2={trendSVG.W - trendSVG.PADR}
                y1={t.y}
                y2={t.y}
                class="grid-y"
              />
              <text x={trendSVG.PADL - 8} y={t.y + 4} class="axis-label" text-anchor="end"
                >{t.v.toFixed(t.v < 10 ? 1 : 0)}</text
              >
            {/each}
            <!-- X axis labels -->
            {#each trendSVG.xLabels as t}
              <line
                x1={t.x}
                x2={t.x}
                y1={trendSVG.H - trendSVG.PADB}
                y2={trendSVG.H - trendSVG.PADB + 4}
                class="grid-x-tick"
              />
              <text
                x={t.x}
                y={trendSVG.H - trendSVG.PADB + 18}
                class="axis-label"
                text-anchor="middle">{t.label}</text
              >
            {/each}
            <!-- Lines -->
            {#each trendSVG.paths as s}
              <path
                d={s.path}
                fill="none"
                stroke={s.color}
                stroke-width="2"
                stroke-linejoin="round"
                stroke-linecap="round"
              />
            {/each}
          </svg>
          <div class="legend">
            {#each trendSVG.paths as s}
              <span class="legend-item"
                ><span class="legend-dot" style="background:{s.color}"></span>{s.domain}</span
              >
            {/each}
          </div>
        </div>
      </section>
    {/if}

    <!-- Top 10 URLs × top 5 KW -->
    {#if topUrls.length > 0}
      <section class="block">
        <h3>Top 10 URLs (× top 5 mots-clés par URL)</h3>
        <div class="url-grid">
          {#each topUrls as u}
            <article class="url-card">
              <header class="url-header">
                <a href={u.url} target="_blank" rel="noopener" class="url-link" title={u.url}
                  >{shorten(urlPath(u.url), 60)}</a
                >
                <span class="pill" title="Intention dominante">{u.dominant}</span>
              </header>
              <div class="url-meta">
                <span>{u.total_kw} KW</span>
                <span>{u.top10} top 10</span>
                <span>{fmtN(Math.round(u.traffic))} traffic est.</span>
              </div>
              <ul class="kw-list">
                {#each u.top_kws as p}
                  <li class="kw-row">
                    <span class="kw-text" title={p.keyword}>{p.keyword}</span>
                    <span class="kw-pos" style="background:{positionPill(p.position)}"
                      >#{p.position}</span
                    >
                    <span class="kw-vol">{fmtN(p.volume || 0)}</span>
                  </li>
                {/each}
              </ul>
              {#if u.kws.length > 5}
                <div class="kw-rest">+ {u.kws.length - 5} autres</div>
              {/if}
            </article>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Competitors -->
    {#if competitors.length > 0}
      <section class="block">
        <h3>Top concurrents organiques</h3>
        <table>
          <thead>
            <tr>
              <th></th>
              <th>Domaine</th>
              <th class="num">Visibilité</th>
              <th class="num">Mots-clés</th>
              <th class="num">Positions</th>
              <th class="num">Trafic</th>
              <th class="num">Bested</th>
            </tr>
          </thead>
          <tbody>
            {#each competitors as c, i}
              <tr>
                <td>{i + 1}</td>
                <td>{c.competitor_domain}</td>
                <td class="num">{(c.visibility_index || 0).toFixed(1)}</td>
                <td class="num">{fmtN(c.keywords)}</td>
                <td class="num">{fmtN(c.positions)}</td>
                <td class="num">{fmtN(c.total_traffic)}</td>
                <td class="num">{fmtN(c.bested)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>
    {/if}

    <!-- Gap analysis -->
    {#if gapMissing.length > 0 || gapBested.length > 0}
      <section class="block">
        <h3>Gap analysis</h3>
        <div class="gap-grid">
          <div>
            <h4>Mots-clés manqués (top 25)</h4>
            <table class="gap-table">
              <thead>
                <tr>
                  <th>Mot-clé</th>
                  <th class="num">Volume</th>
                  <th>URL concurrente</th>
                  <th class="num">Pos.</th>
                </tr>
              </thead>
              <tbody>
                {#each gapMissing.slice(0, 25) as k}
                  <tr>
                    <td>{k.keyword}</td>
                    <td class="num">{fmtN(k.volume || 0)}</td>
                    <td>
                      <a
                        href={k.best_competitor_url}
                        target="_blank"
                        rel="noopener"
                        title={k.best_competitor_url}
                        >{shorten(urlPath(k.best_competitor_url), 38)}</a
                      >
                      <span class="text-muted text-sm"> · {k.best_competitor_domain}</span>
                    </td>
                    <td class="num">{Math.round(k.best_competitor_position) || '—'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          <div>
            <h4>Concurrents mieux positionnés (top 25)</h4>
            <table class="gap-table">
              <thead>
                <tr>
                  <th>Mot-clé</th>
                  <th class="num">Volume</th>
                  <th>Votre URL</th>
                  <th class="num">Vous</th>
                  <th class="num">Eux</th>
                </tr>
              </thead>
              <tbody>
                {#each gapBested.slice(0, 25) as k}
                  <tr>
                    <td>{k.keyword}</td>
                    <td class="num">{fmtN(k.volume || 0)}</td>
                    <td>
                      <a
                        href={k.best_reference_url}
                        target="_blank"
                        rel="noopener"
                        title={k.best_reference_url}>{shorten(urlPath(k.best_reference_url), 38)}</a
                      >
                    </td>
                    <td class="num">{Math.round(k.best_reference_position) || '—'}</td>
                    <td class="num">{Math.round(k.best_competitor_position) || '—'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      </section>
    {/if}
  {/if}
</div>

<style>
  .haloscan-root {
    display: flex;
    flex-direction: column;
    gap: 24px;
    color: var(--text);
  }
  .empty-state {
    padding: 32px;
    text-align: center;
  }
  .domain-row {
    display: flex;
    gap: 8px;
    align-items: center;
    justify-content: center;
    margin-top: 16px;
  }
  .domain-input {
    padding: 8px 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 14px;
    min-width: 280px;
    background: var(--bg-input);
    color: var(--text);
  }
  .sync-error {
    margin-top: 16px;
    padding: 12px 16px;
    background: rgba(239, 68, 68, 0.12);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.35);
    border-radius: 6px;
    font-size: 13px;
    max-width: 600px;
    margin-left: auto;
    margin-right: auto;
  }
  .kpi-row {
    display: flex;
    gap: 16px;
    align-items: stretch;
    flex-wrap: wrap;
  }
  .kpi {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 14px 18px;
    min-width: 150px;
  }
  .kpi-label {
    font-size: 11px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    font-weight: 500;
  }
  .kpi-value {
    font-size: 24px;
    font-weight: 600;
    margin-top: 4px;
    color: var(--text);
  }
  .kpi-actions {
    margin-left: auto;
    align-self: center;
  }
  .block {
    border-top: 1px solid var(--border);
    padding-top: 20px;
  }
  .block h3 {
    margin: 0 0 14px 0;
    font-size: 15px;
    font-weight: 600;
    color: var(--text);
  }
  .block h4 {
    margin: 0 0 10px 0;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .chart-wrap {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
  }
  .trend-svg {
    width: 100%;
    height: auto;
    display: block;
  }
  .grid-y,
  .grid-x-tick {
    stroke: var(--border);
    stroke-width: 1;
    opacity: 0.5;
  }
  .axis-label {
    fill: var(--text-muted);
    font-size: 11px;
    font-family: inherit;
  }
  .legend {
    display: flex;
    gap: 18px;
    flex-wrap: wrap;
    margin-top: 12px;
    font-size: 12px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }
  .legend-item {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--text-secondary);
  }
  .legend-dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
  }
  .url-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
    gap: 12px;
  }
  .url-card {
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 14px;
    background: var(--bg-card);
    transition: border-color 0.15s;
  }
  .url-card:hover {
    border-color: var(--text-muted);
  }
  .url-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }
  .url-link {
    font-weight: 600;
    text-decoration: none;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 13px;
  }
  .url-link:hover {
    text-decoration: underline;
  }
  .pill {
    background: color-mix(in srgb, var(--accent, #3b82f6) 18%, transparent);
    color: var(--accent, #93c5fd);
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 11px;
    white-space: nowrap;
  }
  .url-meta {
    display: flex;
    gap: 12px;
    font-size: 11px;
    color: var(--text-muted);
    margin-bottom: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .kw-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .kw-row {
    display: grid;
    grid-template-columns: 1fr auto auto;
    gap: 10px;
    align-items: center;
    font-size: 13px;
    color: var(--text-secondary);
  }
  .kw-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .kw-pos {
    color: white;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    min-width: 32px;
    text-align: center;
  }
  .kw-vol {
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    min-width: 48px;
    text-align: right;
    font-size: 12px;
  }
  .kw-rest {
    margin-top: 8px;
    font-size: 11px;
    color: var(--text-muted);
    font-style: italic;
  }
  .gap-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 24px;
  }
  @media (max-width: 1024px) {
    .gap-grid {
      grid-template-columns: 1fr;
    }
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
    color: var(--text-secondary);
  }
  th {
    padding: 8px;
    border-bottom: 1px solid var(--border);
    text-align: left;
    font-weight: 500;
    color: var(--text-muted);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  td {
    padding: 8px;
    border-bottom: 1px solid var(--border);
    text-align: left;
  }
  tbody tr:hover {
    background: var(--bg-hover);
  }
  .num {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .gap-table td a {
    color: var(--text);
    text-decoration: none;
  }
  .gap-table td a:hover {
    text-decoration: underline;
  }
</style>
