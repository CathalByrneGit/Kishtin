<script lang="ts">
  // One fridge or freezer: the highest reading each day as a line, with
  // its limit as a labelled reference line. Days over the limit get a
  // marker; days with no reading leave a gap. Hover for the day's readings.
  import { Table } from "@kenn-io/kit-ui";
  import { shortDate } from "../../lib/format";

  interface Day {
    date: string;
    count: number;
    max_c?: number | null;
    min_c?: number | null;
    avg_c?: number | null;
  }
  interface Props {
    unit: string;
    days: Day[];
    limit?: number | null;
  }
  let { unit, days, limit = null }: Props = $props();

  // Drawn at the width it is shown, so lines and text keep their sizes.
  let width = $state(360);
  const W = $derived(Math.max(240, width));
  const H = 150;
  const pad = { top: 12, right: 76, bottom: 22, left: 36 };
  const plotW = $derived(W - pad.left - pad.right);
  const plotH = H - pad.top - pad.bottom;

  const values = $derived(days.flatMap((d) => (d.max_c != null ? [d.max_c] : [])).concat(limit != null ? [limit] : []));
  const lo = $derived(values.length ? Math.floor(Math.min(...values) - 1) : 0);
  const hi = $derived(values.length ? Math.ceil(Math.max(...values) + 1) : 1);
  const x = (i: number) => pad.left + (days.length <= 1 ? plotW / 2 : (i / (days.length - 1)) * plotW);
  const y = (v: number) => pad.top + plotH - ((v - lo) / Math.max(1, hi - lo)) * plotH;

  // Line segments, broken where a day has no reading.
  const segments = $derived.by(() => {
    const out: string[] = [];
    let cur = "";
    days.forEach((d, i) => {
      if (d.max_c == null) {
        if (cur) out.push(cur);
        cur = "";
        return;
      }
      cur += `${cur ? "L" : "M"}${x(i).toFixed(1)},${y(d.max_c).toFixed(1)}`;
    });
    if (cur) out.push(cur);
    return out;
  });
  const over = (d: Day) => limit != null && d.max_c != null && d.max_c > limit;
  const overDays = $derived(days.filter(over).length);
  const read = $derived(days.filter((d) => d.count > 0).length);

  let hover = $state<number | null>(null);
  let svgEl = $state<SVGSVGElement | undefined>();
  function move(e: PointerEvent) {
    if (!svgEl || days.length === 0) return;
    const r = svgEl.getBoundingClientRect();
    const px = ((e.clientX - r.left) / r.width) * W;
    const i = days.length <= 1 ? 0 : Math.round(((px - pad.left) / plotW) * (days.length - 1));
    hover = Math.max(0, Math.min(days.length - 1, i));
  }
  const hovered = $derived(hover === null ? undefined : days[hover]);
  const fmt = (v: number | null | undefined) => (v == null ? "–" : `${v.toFixed(1)}°C`);
</script>

<figure class="chart" bind:clientWidth={width}>
  <figcaption>
    <strong>{unit}</strong>
    <span class="muted small">{read} of {days.length} days read{#if limit != null}&nbsp;· {overDays} {overDays === 1 ? "day" : "days"} over the limit{/if}</span>
  </figcaption>
  <svg
    bind:this={svgEl}
    viewBox={`0 0 ${W} ${H}`}
    role="img"
    aria-label={`${unit}: highest reading each day${limit != null ? `, limit ${limit.toFixed(1)}°C` : ""}`}
    onpointermove={move}
    onpointerleave={() => (hover = null)}
  >
    {#each [lo, hi] as t (t)}
      <line class="grid" x1={pad.left} x2={W - pad.right} y1={y(t)} y2={y(t)} />
      <text class="tick" x={pad.left - 6} y={y(t)} dy="0.32em" text-anchor="end">{t}°</text>
    {/each}
    {#if limit != null}
      <line class="limit" x1={pad.left} x2={W - pad.right} y1={y(limit)} y2={y(limit)} />
      <text class="tick" x={W - pad.right + 6} y={y(limit)} dy="0.32em">Limit {limit.toFixed(1)}°C</text>
    {/if}
    {#each segments as d (d)}
      <path class="line" {d} />
    {/each}
    {#each days as d, i (d.date)}
      {#if over(d) && d.max_c != null}
        <circle class="mark" cx={x(i)} cy={y(d.max_c)} r="4" />
      {/if}
    {/each}
    {#if days.length > 0}
      <text class="tick" x={x(0)} y={H - 6} text-anchor="start">{shortDate(days[0]!.date)}</text>
      <text class="tick" x={x(days.length - 1)} y={H - 6} text-anchor="end">{shortDate(days[days.length - 1]!.date)}</text>
    {/if}
    {#if hover !== null}
      <line class="cross" x1={x(hover)} x2={x(hover)} y1={pad.top} y2={pad.top + plotH} />
      {#if days[hover]?.max_c != null}
        <circle class="mark" cx={x(hover)} cy={y(days[hover]!.max_c!)} r="4" />
      {/if}
    {/if}
  </svg>
  {#if hover !== null && hovered}
    {@const d = hovered}
    <div class="tip" style:left={`${(x(hover) / W) * 100}%`}>
      <strong>{shortDate(d.date)}</strong>
      {#if d.count === 0}
        <span class="muted">No reading</span>
      {:else}
        <span class="num">Highest {fmt(d.max_c)}{#if over(d)} · over the limit{/if}</span>
        <span class="num muted">Lowest {fmt(d.min_c)} · {d.count} {d.count === 1 ? "reading" : "readings"}</span>
      {/if}
    </div>
  {/if}
  <details>
    <summary>Show as a table</summary>
    <Table bare unsorted ariaLabel={`${unit} readings by day`}>
      <thead><tr><th>Day</th><th>Highest</th><th>Lowest</th><th>Readings</th></tr></thead>
      <tbody>
        {#each days as d (d.date)}
          <tr><td>{shortDate(d.date)}</td><td class="num">{fmt(d.max_c)}{#if over(d)} (over){/if}</td><td class="num">{fmt(d.min_c)}</td><td class="num">{d.count}</td></tr>
        {/each}
      </tbody>
    </Table>
  </details>
</figure>

<style>
  .chart {
    position: relative;
    margin: 0;
  }
  figcaption {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }
  svg {
    width: 100%;
    height: auto;
    display: block;
    touch-action: pan-y;
  }
  .grid {
    stroke: var(--chart-grid);
    stroke-width: 1;
  }
  .limit {
    stroke: var(--text-muted);
    stroke-width: 1;
  }
  .cross {
    stroke: var(--border-default);
    stroke-width: 1;
  }
  .line {
    fill: none;
    stroke: var(--chart-series);
    stroke-width: 2;
    stroke-linejoin: round;
    stroke-linecap: round;
  }
  .mark {
    fill: var(--chart-series);
    stroke: var(--bg-surface);
    stroke-width: 2;
  }
  .tick {
    fill: var(--text-secondary);
    font-size: 10px;
    font-variant-numeric: tabular-nums;
  }
  .tip {
    position: absolute;
    top: 28px;
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
    z-index: 1;
  }
  .muted {
    color: var(--text-secondary);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  details {
    margin-top: var(--space-2);
    font-size: var(--font-size-sm);
  }
  summary {
    cursor: pointer;
    color: var(--text-secondary);
  }
</style>
