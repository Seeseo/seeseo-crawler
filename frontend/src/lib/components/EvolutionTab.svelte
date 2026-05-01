<script>
  import { onMount } from 'svelte';
  import { getProjectEvolution } from '../api.js';
  import AreaChart from './charts/AreaChart.svelte';
  import { fmtN } from '../utils.js';

  let { project } = $props();

  let points = $state([]);
  let loading = $state(true);
  let error = $state('');
  let metric = $state('total_pages');

  const METRICS = [
    { key: 'total_pages', label: 'Pages totales', color: '#FF6B00' },
    { key: 'not_found', label: 'Erreurs 404', color: '#dc2626' },
    { key: 'client_errors', label: 'Erreurs 4xx', color: '#f59e0b' },
    { key: 'server_errors', label: 'Erreurs 5xx', color: '#7c2d12' },
    { key: 'redirects', label: 'Redirections 3xx', color: '#0891b2' },
    { key: 'internal_links', label: 'Liens internes', color: '#16a34a' },
    { key: 'external_links', label: 'Liens externes', color: '#9333ea' },
    { key: 'avg_fetch_ms', label: 'Temps de réponse moyen (ms)', color: '#2563eb' },
  ];

  async function load() {
    loading = true;
    error = '';
    try {
      const res = await getProjectEvolution(project.id);
      points = res.points || [];
    } catch (e) {
      error = e.message || String(e);
    } finally {
      loading = false;
    }
  }

  onMount(load);
  $effect(() => {
    if (project?.id) load();
  });

  const currentMetric = $derived(METRICS.find((m) => m.key === metric) || METRICS[0]);

  const chartData = $derived.by(() => {
    if (!points.length) return { series: [], labels: [] };
    const values = points.map((p) => Number(p[metric] || 0));
    const labels = points.map((p) => {
      const d = new Date(p.started_at);
      return d.toLocaleDateString('fr-FR', { day: '2-digit', month: '2-digit', year: '2-digit' });
    });
    return {
      series: [
        {
          key: currentMetric.key,
          label: currentMetric.label,
          color: currentMetric.color,
          values,
        },
      ],
      labels,
    };
  });

  function fmtDate(iso) {
    const d = new Date(iso);
    return d.toLocaleDateString('fr-FR', { day: '2-digit', month: '2-digit', year: 'numeric' });
  }

  function fmtMetric(value, key) {
    if (key === 'avg_fetch_ms') return Math.round(Number(value)) + ' ms';
    return fmtN(Number(value));
  }
</script>

<div class="evolution-tab">
  {#if loading}
    <div class="evolution-state">Chargement…</div>
  {:else if error}
    <div class="evolution-state evolution-error">Erreur : {error}</div>
  {:else if points.length === 0}
    <div class="evolution-state">
      <p>Aucune session de crawl pour ce projet pour l'instant.</p>
      <p class="evolution-hint">
        Lancez au moins un crawl pour faire apparaître ce projet dans la timeline.
      </p>
    </div>
  {:else if points.length === 1}
    <div class="evolution-state">
      <p>Une seule session pour ce projet.</p>
      <p class="evolution-hint">
        La timeline d'évolution s'active dès le 2<sup>e</sup> crawl — relancez un crawl plus tard
        pour comparer les métriques dans le temps.
      </p>
      <div class="evolution-single-session">
        <div class="evo-stat"><span>Pages</span><strong>{fmtN(points[0].total_pages)}</strong></div>
        <div class="evo-stat"><span>404</span><strong>{fmtN(points[0].not_found)}</strong></div>
        <div class="evo-stat"><span>4xx</span><strong>{fmtN(points[0].client_errors)}</strong></div>
        <div class="evo-stat"><span>5xx</span><strong>{fmtN(points[0].server_errors)}</strong></div>
        <div class="evo-stat">
          <span>Liens internes</span><strong>{fmtN(points[0].internal_links)}</strong>
        </div>
        <div class="evo-stat">
          <span>Liens externes</span><strong>{fmtN(points[0].external_links)}</strong>
        </div>
      </div>
    </div>
  {:else}
    <div class="evolution-controls">
      <label>
        Métrique :
        <select bind:value={metric}>
          {#each METRICS as m}
            <option value={m.key}>{m.label}</option>
          {/each}
        </select>
      </label>
      <span class="evolution-summary">
        {points.length} sessions · du {fmtDate(points[0].started_at)} au {fmtDate(
          points[points.length - 1].started_at
        )}
      </span>
    </div>

    <div class="evolution-chart">
      <AreaChart series={chartData.series} labels={chartData.labels} height={280} />
    </div>

    <table class="evolution-table">
      <thead>
        <tr>
          <th>Date</th>
          <th>Label</th>
          <th class="num">Pages</th>
          <th class="num">404</th>
          <th class="num">4xx</th>
          <th class="num">5xx</th>
          <th class="num">Liens int.</th>
          <th class="num">Liens ext.</th>
          <th class="num">Fetch ms</th>
        </tr>
      </thead>
      <tbody>
        {#each points as p}
          <tr>
            <td>{fmtDate(p.started_at)}</td>
            <td class="evo-label">{p.label || '—'}</td>
            <td class="num">{fmtN(p.total_pages)}</td>
            <td class="num">{fmtN(p.not_found)}</td>
            <td class="num">{fmtN(p.client_errors)}</td>
            <td class="num">{fmtN(p.server_errors)}</td>
            <td class="num">{fmtN(p.internal_links)}</td>
            <td class="num">{fmtN(p.external_links)}</td>
            <td class="num">{Math.round(p.avg_fetch_ms)}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  .evolution-tab {
    padding: 16px 4px;
  }
  .evolution-state {
    padding: 32px;
    text-align: center;
    color: var(--text-muted);
  }
  .evolution-error {
    color: #dc2626;
  }
  .evolution-hint {
    margin-top: 8px;
    font-size: 13px;
  }
  .evolution-single-session {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: 12px;
    margin-top: 24px;
    max-width: 720px;
    margin-left: auto;
    margin-right: auto;
  }
  .evo-stat {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 12px 16px;
    background: var(--bg-hover);
    border-radius: var(--radius-sm);
  }
  .evo-stat span {
    font-size: 12px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .evo-stat strong {
    font-size: 18px;
    font-weight: 600;
    color: var(--text);
  }
  .evolution-controls {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 16px;
    padding: 0 8px;
    flex-wrap: wrap;
  }
  .evolution-controls label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 500;
    color: var(--text-secondary);
  }
  .evolution-controls select {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font-size: 14px;
  }
  .evolution-summary {
    font-size: 13px;
    color: var(--text-muted);
  }
  .evolution-chart {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 16px 8px;
    margin-bottom: 24px;
  }
  .evolution-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .evolution-table th,
  .evolution-table td {
    padding: 8px 10px;
    border-bottom: 1px solid var(--border);
    text-align: left;
  }
  .evolution-table th {
    font-weight: 600;
    color: var(--text-secondary);
    background: var(--bg-hover);
  }
  .evolution-table td.num,
  .evolution-table th.num {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .evo-label {
    color: var(--text-muted);
    font-style: italic;
  }
</style>
