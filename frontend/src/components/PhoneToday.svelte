<script lang="ts">
  import { t, tAny } from "../lib/i18n.svelte";
  import { dueToday, scheduleStatus } from "../lib/cleaning";
  import { Button, Card, Checkbox, Chip, EmptyState, Notice, Spinner, showFlash, type ChipTone } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import Ring from "./Ring.svelte";
  import { api, message, type Schemas, type Today } from "../lib/api";
  import { dublinTime, temp } from "../lib/format";
  import { details, doneAt, doneBy, limitText, outcomeStyle, title, typeLabel } from "../lib/labels";
  import { onChange } from "../lib/live.svelte";
  import type { LogPreset } from "./LogForm.svelte";

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
    if (s.status === "missed") return { label: `${s.label} ${t("missed")}`, tone: "warning" };
    if (s.status === "closed") return { label: `${s.label} ${t("closed")}`, tone: "neutral" };
    return { label: `${s.label} ${t("due")}`, tone: "neutral" };
  }

  async function tick(task: string) {
    ticking = task;
    try {
      await api.log({ type: "cleaning", reading: { task, done: true } });
      showFlash(t("Saved"), { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      ticking = null;
    }
  }

  const summary = $derived.by(() => {
    if (!today) return "";
    if (today.out_of_limit === 0) return t("No out-of-limit readings today.");
    const n = today.out_of_limit;
    if (today.without_action === 0) return t("Out-of-limit readings today: {n}.", { n });
    return t("Out-of-limit readings today: {n}. Without an action: {m}.", { n, m: today.without_action });
  });
</script>

{#if error}
  <Notice tone="error" title={t("Could not load today")} message={error} />
{/if}

{#if !today}
  {#if !error}<Spinner />{/if}
{:else}
  <div class="stack">
    <Card padding="md">
      <div class="summary">
        <Ring percent={today.percent} state={today.ring} label={t("{done} of {total} checks done", { done: today.done, total: today.total })} />
        <div class="stack">
          <strong class="big">{t("{done} of {total} checks done", { done: today.done, total: today.total })}</strong>
          <span class:alert={today.out_of_limit > 0}>{summary}</span>
        </div>
      </div>
    </Card>

    {#if today.closed}
      <Notice tone="info" message={t("Closed today: no checks are due.")} />
    {/if}

    <h2 class="section-title">{t("Fridge & freezer temperatures")}</h2>
    {#if today.units.length === 0}
      <EmptyState title={t("No fridges or freezers")} description={t("A manager adds them in Setup.")} />
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
              <Chip tone={c.tone} size="md" uppercase={false} title={s.record ? `Done ${dublinTime(doneAt(s.record))} by ${doneBy(s.record)}` : ""}>
                <span class="num">{c.label}</span>
              </Chip>
            {/each}
          </div>
          <Button tone="info" surface="soft" label={t("Log")} onclick={() => onlog({ type: u.kind, unit: u.name })} />
        </div>
      </Card>
    {/each}

    <h2 class="section-title">{t("Cleaning due today")}</h2>
    <Card padding="md">
      <ul class="checklist">
        {#each today.cleaning.filter(dueToday) as c (c.task)}
          {@const st = scheduleStatus(c)}
          <li>
            <Checkbox
              checked={Boolean(c.record)}
              disabled={Boolean(c.record) || ticking !== null}
              label={c.task}
              onchange={(on) => {
                if (on) void tick(c.task);
              }}
            />
            {#if st}<Chip size="sm" uppercase={false} tone={st.tone}>{st.label}</Chip>{/if}
            {#if c.record}
              <span class="muted num">{dublinTime(doneAt(c.record))} · {doneBy(c.record)}</span>
            {/if}
          </li>
        {:else}
          <li class="muted">{today.cleaning.length > 0 ? t("No cleaning due today.") : t("No cleaning tasks. A manager adds them in Setup.")}</li>
        {/each}
      </ul>
    </Card>

    {#if today.needs_attention.length > 0}
      <h2 class="section-title">{t("Needs attention")}</h2>
      {#each today.needs_attention as r (r.id)}
        {@const o = outcomeStyle(r.outcome)}
        <Card padding="md">
          <div class="stack">
            <div class="row">
              <Chip tone={o.tone} uppercase={false}>{tAny(o.label)}</Chip>
              <strong>{title(r)}</strong>
              <span class="muted num">{dublinTime(doneAt(r))} · {doneBy(r)}</span>
            </div>
            <span>{r.outcome_reason}</span>
            {#if r.corrective_action}
              <span><strong>{t("Action:")}</strong> {r.corrective_action}</span>
            {:else}
              <span class="alert">{t("No corrective action recorded.")}</span>
            {/if}
          </div>
        </Card>
      {/each}
    {/if}

    {#if today.batches.length > 0}
      <h2 class="section-title">{t("Batches in use")}</h2>
      <Card padding="md">
        <ul class="batches">
          {#each today.batches as b (b.record.id)}
            <li>
              <div class="unit-name">
                <strong>{b.record.reading.product}</strong>
                <span class="muted">{details(b.record)}</span>
              </div>
              {#if b.status === "expired"}
                <Chip tone="danger" uppercase={false}>{t("Expired")}</Chip>
              {:else if b.status === "use_today"}
                <Chip tone="warning" uppercase={false}>{t("Use today")}</Chip>
              {:else}
                <Chip tone="success" uppercase={false}>{t("OK")}</Chip>
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
