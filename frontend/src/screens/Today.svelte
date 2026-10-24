<script lang="ts">
  import { Button, Card, Checkbox, Chip, EmptyState, Notice, Spinner, showFlash, type ChipTone } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import Ring from "../components/Ring.svelte";
  import { api, message, type Schemas, type Today } from "../lib/api";
  import { dublinTime, temp } from "../lib/format";
  import { details, limitText, outcomeStyle, title, typeLabel } from "../lib/labels";
  import { onChange } from "../lib/live.svelte";
  import type { LogPreset } from "./LogCheck.svelte";

  interface Props {
    onlog: (preset: LogPreset) => void;
  }
  let { onlog }: Props = $props();

  let today = $state<Today | null>(null);
  let error = $state("");
  let ticking = $state<string | null>(null);

  async function load() {
    try {
      today = await api.today();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    const off = onChange((kind) => {
      if (kind !== "staff") void load();
    });
    // Checks become missed as the day goes on.
    const tick = setInterval(load, 60_000);
    return () => {
      off();
      clearInterval(tick);
    };
  });

  function slotChip(s: Schemas["SlotToday"]): { label: string; tone: ChipTone } {
    if (s.record) {
      return { label: `${s.label} ${temp(s.record.reading.temp_c)}`, tone: outcomeStyle(s.record.outcome).tone };
    }
    if (s.status === "missed") return { label: `${s.label} missed`, tone: "warning" };
    return { label: `${s.label} due`, tone: "neutral" };
  }

  async function tick(task: string) {
    ticking = task;
    try {
      await api.log({ type: "cleaning", reading: { task, done: true } });
      showFlash("Saved", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      ticking = null;
    }
  }

  const summary = $derived.by(() => {
    if (!today) return "";
    if (today.out_of_limit === 0) return "No out-of-limit readings today.";
    const n = today.out_of_limit;
    const what = n === 1 ? "1 out-of-limit reading today" : `${n} out-of-limit readings today`;
    if (today.without_action === 0) return `${what}, ${n === 1 ? "with" : "each with"} an action recorded.`;
    return `${what}, ${today.without_action} without an action.`;
  });
</script>

{#if error}
  <Notice tone="error" title="Could not load today" message={error} />
{/if}

{#if !today}
  {#if !error}<Spinner />{/if}
{:else}
  <div class="stack">
    <Card padding="md">
      <div class="summary">
        <Ring percent={today.percent} state={today.ring} label={`${today.done} of ${today.total} checks done`} />
        <div class="stack">
          <strong class="big">{today.done} of {today.total} checks done</strong>
          <span class:alert={today.out_of_limit > 0}>{summary}</span>
        </div>
      </div>
    </Card>

    <h2 class="section-title">Fridge &amp; freezer temperatures</h2>
    {#if today.units.length === 0}
      <EmptyState title="No fridges or freezers" description="A manager adds them in Setup." />
    {/if}
    {#each today.units as u (u.name)}
      <Card padding="md">
        <div class="unit">
          <div class="unit-name">
            <strong>{u.name}</strong>
            <span class="muted">{typeLabel(u.kind)} · {limitText(u.kind, u.limit.max_c)}</span>
          </div>
          <div class="row slots">
            {#each u.slots as s (s.label)}
              {@const c = slotChip(s)}
              <Chip tone={c.tone} size="md" uppercase={false} title={s.record ? `Logged ${dublinTime(s.record.logged_at)} by ${s.record.logged_by_name}` : ""}>
                <span class="num">{c.label}</span>
              </Chip>
            {/each}
          </div>
          <Button tone="info" surface="soft" label="Log" onclick={() => onlog({ type: u.kind, unit: u.name })} />
        </div>
      </Card>
    {/each}

    <h2 class="section-title">Daily cleaning</h2>
    <Card padding="md">
      <ul class="checklist">
        {#each today.cleaning as c (c.task)}
          <li>
            <Checkbox
              checked={Boolean(c.record)}
              disabled={Boolean(c.record) || ticking !== null}
              label={c.task}
              onchange={(on) => {
                if (on) void tick(c.task);
              }}
            />
            {#if c.record}
              <span class="muted num">{dublinTime(c.record.logged_at)} · {c.record.logged_by_name}</span>
            {/if}
          </li>
        {:else}
          <li class="muted">No cleaning tasks. A manager adds them in Setup.</li>
        {/each}
      </ul>
    </Card>

    {#if today.needs_attention.length > 0}
      <h2 class="section-title">Needs attention</h2>
      {#each today.needs_attention as r (r.id)}
        {@const o = outcomeStyle(r.outcome)}
        <Card padding="md">
          <div class="stack">
            <div class="row">
              <Chip tone={o.tone} uppercase={false}>{o.label}</Chip>
              <strong>{title(r)}</strong>
              <span class="muted num">{dublinTime(r.logged_at)} · {r.logged_by_name}</span>
            </div>
            <span>{r.outcome_reason}</span>
            {#if r.corrective_action}
              <span><strong>Action:</strong> {r.corrective_action}</span>
            {:else}
              <span class="alert">No corrective action recorded.</span>
            {/if}
          </div>
        </Card>
      {/each}
    {/if}

    {#if today.batches.length > 0}
      <h2 class="section-title">Batches in use</h2>
      <Card padding="md">
        <ul class="batches">
          {#each today.batches as b (b.record.id)}
            <li>
              <div class="unit-name">
                <strong>{b.record.reading.product}</strong>
                <span class="muted">{details(b.record)}</span>
              </div>
              {#if b.status === "expired"}
                <Chip tone="danger" uppercase={false}>Expired</Chip>
              {:else if b.status === "use_today"}
                <Chip tone="warning" uppercase={false}>Use today</Chip>
              {:else}
                <Chip tone="success" uppercase={false}>OK</Chip>
              {/if}
            </li>
          {/each}
        </ul>
      </Card>
    {/if}
  </div>
{/if}

<style>
  .summary {
    display: flex;
    align-items: center;
    gap: var(--space-6);
  }
  .big {
    font-size: var(--font-size-xl);
  }
  .alert {
    color: var(--accent-red);
    font-weight: var(--font-weight-medium);
  }
  .unit {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: var(--space-4);
    align-items: center;
  }
  .unit-name {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: var(--font-size-lg);
  }
  .slots {
    grid-column: 1;
  }
  .unit :global(.kit-button) {
    grid-column: 2;
    grid-row: 1 / span 2;
  }
  .checklist,
  .batches {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    font-size: var(--font-size-lg);
  }
  .checklist li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    min-height: 44px;
  }
  .batches li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }
</style>
