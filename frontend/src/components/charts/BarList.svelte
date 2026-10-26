<script lang="ts">
  // Horizontal bars for a few named counts, largest first. One hue; the
  // label and value are text, so nothing relies on colour.
  interface Row {
    label: string;
    value: number;
    /** what the value is out of, for the tooltip */
    of?: number | null;
  }
  interface Props {
    rows: Row[];
    ariaLabel: string;
    /** how a value reads, such as "3 missed" */
    format?: (v: number) => string;
    empty?: string;
  }
  let { rows, ariaLabel, format = (v: number) => String(v), empty = "None." }: Props = $props();
  const top = $derived(Math.max(1, ...rows.map((r) => r.value)));
</script>

{#if rows.length === 0}
  <p class="muted">{empty}</p>
{:else}
  <ul class="bars" aria-label={ariaLabel}>
    {#each rows as r (r.label)}
      <li title={r.of ? `${r.label}: ${format(r.value)} of ${r.of}` : `${r.label}: ${format(r.value)}`}>
        <span class="label">{r.label}</span>
        <span class="track">
          <span class="bar" style:width={`${(r.value / top) * 100}%`}></span>
        </span>
        <span class="value num">{format(r.value)}{#if r.of}<span class="muted"> of {r.of}</span>{/if}</span>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .bars {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: var(--space-3);
  }
  li {
    display: grid;
    grid-template-columns: minmax(120px, 1fr) minmax(0, 2fr) auto;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-1) 0;
  }
  li:hover .label,
  li:hover .value {
    color: var(--text-primary);
  }
  .label {
    color: var(--text-secondary);
  }
  .track {
    height: 16px;
    display: flex;
  }
  .bar {
    min-width: 4px;
    height: 100%;
    background: var(--chart-series);
    border-radius: 0 4px 4px 0;
  }
  .value {
    color: var(--text-primary);
    text-align: right;
    white-space: nowrap;
  }
  .muted {
    color: var(--text-secondary);
  }
  p {
    margin: 0;
  }
</style>
