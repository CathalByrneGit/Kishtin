<script lang="ts">
  import { Button } from "@kenn-io/kit-ui";
  import { type AllDay, active } from "../lib/tickets";

  interface Props {
    rows: AllDay[];
    /** the dish shown on its own, by key */
    selected: string | null;
    onselect: (key: string | null) => void;
    /** start the items the scheduler says to start now, together */
    onstart?: (row: AllDay) => void;
    busy?: boolean;
    /** count held items too (the pass) */
    showHeld?: boolean;
  }
  let { rows, selected, onselect, onstart, busy = false, showHeld = false }: Props = $props();

  const shown = $derived(rows.filter((r) => active(r) + r.ready + (showHeld ? r.held : 0) > 0));

  function parts(r: AllDay): string {
    const p: string[] = [];
    if (r.toStart) p.push(`${r.toStart} to start`);
    if (r.cooking) p.push(`${r.cooking} cooking`);
    if (r.ready) p.push(`${r.ready} ready`);
    if (showHeld && r.held) p.push(`${r.held} held`);
    return p.join(" · ");
  }
</script>

{#if shown.length > 0}
  <section class="allday" aria-label="All day">
    <span class="title">All day</span>
    <div class="dishes">
      {#each shown as r (r.key)}
        <div class="dish" class:selected={selected === r.key}>
          <button
            type="button"
            class="pick kit-control-states"
            aria-pressed={selected === r.key}
            title={selected === r.key ? "Show every dish" : `Show only ${r.name}`}
            onclick={() => onselect(selected === r.key ? null : r.key)}
          >
            <span class="count num">{active(r) + (showHeld ? r.held : 0) || r.ready}</span>
            <span class="name">{r.name}</span>
            <span class="parts">{parts(r)}</span>
          </button>
          {#if onstart && r.startNow.length > 1}
            <Button tone="success" surface="solid" label={`Start ${r.startNow.length}`} disabled={busy} onclick={() => onstart(r)} />
          {/if}
        </div>
      {/each}
    </div>
  </section>
{/if}

<style>
  .allday {
    display: flex;
    align-items: flex-start;
    gap: var(--space-4);
  }
  .title {
    flex: none;
    padding-top: var(--space-3);
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-semibold);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-secondary);
  }
  .dishes {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }
  .dish {
    display: flex;
    align-items: stretch;
    gap: var(--space-2);
  }
  .pick {
    display: grid;
    grid-template-columns: auto 1fr;
    grid-template-rows: auto auto;
    column-gap: var(--space-3);
    align-items: center;
    min-height: max(44px, var(--kit-control-height, 0px));
    padding: var(--space-2) var(--space-4);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .selected .pick {
    border-color: var(--accent-blue);
    box-shadow: inset 0 0 0 1px var(--accent-blue);
  }
  .count {
    grid-row: span 2;
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-bold);
  }
  .name {
    font-weight: var(--font-weight-semibold);
  }
  .parts {
    font-size: var(--font-size-sm);
    color: var(--text-secondary);
  }
</style>
