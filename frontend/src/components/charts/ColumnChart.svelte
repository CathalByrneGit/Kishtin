<script lang="ts">
  // Columns for a value per ordered group (days of the week, hours). One
  // hue, a hairline grid, hover for the exact value, and a table view.
  interface Col {
    label: string;
    /** short label under the column */
    short?: string;
    value: number;
    /** the tooltip's second line */
    note?: string;
  }
  interface Props {
    cols: Col[];
    ariaLabel: string;
    format?: (v: number) => string;
    /** fix the top of the scale, such as 100 for percentages */
    max?: number;
    unit?: string;
  }
  let { cols, ariaLabel, format = (v: number) => String(v), max, unit = "" }: Props = $props();

  // Drawn at the width it is shown, so marks and text keep their sizes.
  let width = $state(560);
  const W = $derived(Math.max(240, width));
  const H = 200;
  const pad = { top: 16, right: 8, bottom: 28, left: 36 };
  const plotW = $derived(W - pad.left - pad.right);
  const plotH = H - pad.top - pad.bottom;

  function niceTop(v: number): number {
    if (v <= 0) return 1;
    const step = 10 ** Math.floor(Math.log10(v));
    for (const m of [1, 2, 2.5, 5, 10]) if (m * step >= v) return m * step;
    return 10 * step;
  }
  const top = $derived(max ?? niceTop(Math.max(...cols.map((c) => c.value), 0)));
  const ticks = $derived([0, top / 2, top]);
  const band = $derived(plotW / Math.max(1, cols.length));
  const barW = $derived(Math.min(24, band * 0.6));
  const y = (v: number) => pad.top + plotH - (v / top) * plotH;

  let hover = $state<number | null>(null);
  const hovered = $derived(hover === null ? undefined : cols[hover]);
</script>

<figure class="chart" bind:clientWidth={width}>
  <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={ariaLabel}>
    {#each ticks as t (t)}
      <line class="grid" x1={pad.left} x2={W - pad.right} y1={y(t)} y2={y(t)} />
      <text class="tick" x={pad.left - 6} y={y(t)} dy="0.32em" text-anchor="end">{format(t)}{unit}</text>
    {/each}
    {#each cols as c, i (c.label)}
      {@const x = pad.left + band * i + (band - barW) / 2}
      {@const h = Math.max(0, (c.value / top) * plotH)}
      <g
        role="presentation"
        onpointerenter={() => (hover = i)}
        onpointerleave={() => (hover = null)}
      >
        <rect class="hit" x={pad.left + band * i} y={pad.top} width={band} height={plotH} />
        {#if h > 0}
          <path
            class="bar"
            d={`M${x},${pad.top + plotH} V${pad.top + plotH - h + Math.min(4, h)} q0,-${Math.min(4, h)} ${Math.min(4, h)},-${Math.min(4, h)} H${x + barW - Math.min(4, h)} q${Math.min(4, h)},0 ${Math.min(4, h)},${Math.min(4, h)} V${pad.top + plotH} Z`}
          />
        {/if}
        <text class="tick" x={pad.left + band * i + band / 2} y={H - 8} text-anchor="middle">{c.short ?? c.label}</text>
      </g>
    {/each}
    <line class="axis" x1={pad.left} x2={W - pad.right} y1={pad.top + plotH} y2={pad.top + plotH} />
  </svg>
  {#if hover !== null && hovered}
    {@const c = hovered}
    <div class="tip" style:left={`${((pad.left + band * hover + band / 2) / W) * 100}%`}>
      <strong>{c.label}</strong>
      <span class="num">{format(c.value)}{unit}</span>
      {#if c.note}<span class="muted">{c.note}</span>{/if}
    </div>
  {/if}
  <details>
    <summary>Show as a table</summary>
    <table>
      <tbody>
        {#each cols as c (c.label)}
          <tr><th scope="row">{c.label}</th><td class="num">{format(c.value)}{unit}</td><td class="muted">{c.note ?? ""}</td></tr>
        {/each}
      </tbody>
    </table>
  </details>
</figure>

<style>
  .chart {
    position: relative;
    margin: 0;
  }
  svg {
    width: 100%;
    height: auto;
    display: block;
  }
  .grid {
    stroke: var(--chart-grid);
    stroke-width: 1;
  }
  .axis {
    stroke: var(--border-default);
    stroke-width: 1;
  }
  .bar {
    fill: var(--chart-series);
  }
  .hit {
    fill: transparent;
  }
  .tick {
    fill: var(--text-secondary);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }
  .tip {
    position: absolute;
    top: 0;
    transform: translateX(-50%);
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-surface);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
    font-size: var(--font-size-sm);
    pointer-events: none;
    white-space: nowrap;
  }
  .muted {
    color: var(--text-secondary);
  }
  details {
    margin-top: var(--space-2);
    font-size: var(--font-size-sm);
  }
  summary {
    cursor: pointer;
    color: var(--text-secondary);
  }
  table {
    border-collapse: collapse;
    margin-top: var(--space-2);
  }
  th,
  td {
    text-align: left;
    padding: 2px var(--space-4) 2px 0;
    font-weight: normal;
  }
</style>
