<script lang="ts">
  import GettingStarted from "../components/GettingStarted.svelte";
  import { dueToday, scheduleStatus } from "../lib/cleaning";
  import { can, canWithPassword } from "../lib/can.svelte";
  import { Button, Card, Checkbox, Chip, EmptyState, MetricCard, Notice, Spinner, Table, TableHeaderCell, showFlash, type ChipTone } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, type Schemas, type Today } from "../lib/api";
  import { dublinTime, longDate, temp } from "../lib/format";
  import { details, doneAt, doneBy, limitText, outcomeStyle, title, typeLabel } from "../lib/labels";
  import { onChange } from "../lib/live.svelte";
  import { openLog } from "../lib/logdrawer.svelte";
  import { navigate } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  /** SIGN_OFF_DAYS is how many unsigned days before Today reminds the head chef and manager. */
  const SIGN_OFF_DAYS = 7;

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
    if (s.record) return { label: temp(s.record.reading.temp_c), tone: outcomeStyle(s.record.outcome).tone };
    if (s.status === "missed") return { label: "Missed", tone: "warning" };
    return { label: "Due", tone: "neutral" };
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

  // Every unit's check labels, in order, so the table has one column per check.
  const slotLabels = $derived.by(() => {
    const seen: string[] = [];
    for (const u of today?.units ?? []) for (const s of u.slots) if (!seen.includes(s.label)) seen.push(s.label);
    return seen;
  });

  const cleaningToday = $derived((today?.cleaning ?? []).filter(dueToday));
  const cleaningDone = $derived(cleaningToday.filter((c) => c.record).length);
  const daily = $derived((today?.cleaning ?? []).filter((c) => !c.schedule));
  const longer = $derived((today?.cleaning ?? []).filter((c) => c.schedule));
  const batchAlerts = $derived(today?.batches.filter((b) => b.status !== "ok").length ?? 0);
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Today</h1>
      {#if today}<span class="muted">{longDate(today.date)}</span>{/if}
    </div>
  </header>

  <GettingStarted />

  {#if error}
    <Notice tone="error" title="Could not load today" message={error} />
  {/if}

  {#if !today}
    {#if !error}<Spinner />{/if}
  {:else}
    {#if today.sign_off.unsigned_days >= SIGN_OFF_DAYS && canWithPassword("haccp.sign_off") && today.sign_off.unsigned_from}
      <Notice
        tone="warning"
        title="Records not signed off"
        message={`Nothing is signed off from ${longDate(today.sign_off.unsigned_from)}: ${today.sign_off.unsigned_days} days. A manager signs off in Record books.`}
        actionLabel="Open Record books"
        onaction={() => navigate("/books")}
      />
    {/if}
    <div class="metrics">
      <MetricCard
        label="Checks done"
        value={`${today.done} of ${today.total}`}

        tone={today.ring === "attention" ? "danger" : today.ring === "complete" ? "success" : "info"}
      >
        <span class="muted small">{today.percent}% of today's fridge, freezer and cleaning checks</span>
      </MetricCard>
      <MetricCard
        label="Out of limit"
        value={String(today.out_of_limit)}

        tone={today.out_of_limit > 0 ? "warning" : "success"}
      >
        <span class="muted small">{today.out_of_limit === 0 ? "No borderline or failed readings today" : "Borderline or failed readings today"}</span>
      </MetricCard>
      <MetricCard
        label="Without an action"
        value={String(today.without_action)}

        tone={today.without_action > 0 ? "danger" : "success"}
      >
        <span class="muted small">{today.without_action === 0 ? "Every out-of-limit reading has a corrective action" : "Out-of-limit readings need a corrective action"}</span>
      </MetricCard>
      <MetricCard
        label="Cleaning"
        value={`${cleaningDone} of ${cleaningToday.length}`}

        tone={cleaningToday.length > 0 && cleaningDone === cleaningToday.length ? "success" : "neutral"}
      >
        <span class="muted small">Cleaning tasks due today, done</span>
      </MetricCard>
      <MetricCard
        label="Batches"
        value={String(today.batches.length)}

        tone={batchAlerts > 0 ? "warning" : "neutral"}
      >
        <span class="muted small">{batchAlerts === 0 ? "Batch labels in use, all in date" : `${batchAlerts} to use today or expired`}</span>
      </MetricCard>
    </div>

    <div class="grid">
      <section class="wide">
        <Card padding="none">
          <div class="table-head"><h2>Fridge and freezer temperatures</h2></div>
          {#if today.units.length === 0}
            <EmptyState title="No fridges or freezers" description="A manager adds them in Setup, Fridges and tasks." />
          {:else}
            <Table unsorted ariaLabel="Fridge and freezer checks">
              {#snippet header()}
                <TableHeaderCell label="Unit" />
                <TableHeaderCell label="Limit" />
                {#each slotLabels as l (l)}<TableHeaderCell label={l} />{/each}
                <TableHeaderCell><span class="kit-sr-only">Action</span></TableHeaderCell>
              {/snippet}
              {#each today.units as u (u.name)}
                <tr>
                  <td>
                    <strong>{u.name}</strong>
                    <span class="muted"> · {typeLabel(u.kind)}{u.non_food ? ", non-food" : ""}</span>
                  </td>
                  <td class="num muted">{limitText(u.kind, u.limit.max_c)}</td>
                  {#each slotLabels as l (l)}
                    {@const s = u.slots.find((x) => x.label === l)}
                    <td>
                      {#if s}
                        {@const c = slotChip(s)}
                        <Chip
                          tone={c.tone}
                          uppercase={false}
                          title={s.record ? `Done ${dublinTime(doneAt(s.record))} by ${doneBy(s.record)}` : `From ${s.from}${s.until ? ` until ${s.until}` : ""}`}
                        >
                          <span class="num">{c.label}</span>
                        </Chip>
                        {#if s.record}<span class="muted small num"> {dublinTime(doneAt(s.record))}</span>{/if}
                      {:else}
                        <span class="muted">–</span>
                      {/if}
                    </td>
                  {/each}
                  <td class="end">
                    <Button size="sm" label="Log" onclick={() => openLog({ type: u.kind, unit: u.name })} />
                  </td>
                </tr>
              {/each}
            </Table>
          {/if}
        </Card>
      </section>

      <section>
        <Card padding="md" title="Daily cleaning" meta={`${daily.filter((c) => c.record).length} of ${daily.length} done`}>
          <ul class="checklist">
            {#each daily as c (c.task)}
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
                  <span class="muted small"><span class="num">{dublinTime(doneAt(c.record))}</span> · {doneBy(c.record)}</span>
                {/if}
              </li>
            {:else}
              <li class="muted">{longer.length > 0 ? "No daily cleaning tasks." : "No cleaning tasks. A manager adds them in Setup."}</li>
            {/each}
          </ul>
        </Card>
        {#if longer.length > 0}
          <Card padding="md" title="Weekly and longer">
            <ul class="checklist">
              {#each longer as c (c.task)}
                {@const st = scheduleStatus(c)}
                <li class="longer">
                  <span class="top">
                    <Checkbox
                      checked={Boolean(c.record)}
                      disabled={Boolean(c.record) || ticking !== null}
                      label={c.task}
                      onchange={(on) => {
                        if (on) void tick(c.task);
                      }}
                    />
                    {#if st}<Chip size="sm" uppercase={false} tone={st.tone}>{st.label}</Chip>{/if}
                  </span>
                  <span class="muted small">{#if c.record}<span class="num">{dublinTime(doneAt(c.record))}</span> · {doneBy(c.record)} · {/if}{st?.text}</span>
                </li>
              {/each}
            </ul>
          </Card>
        {/if}
      </section>

      <section class="wide">
        <Card padding="none">
          <div class="table-head"><h2>Needs attention</h2><span class="muted">Today's borderline and failed readings</span></div>
          {#if today.needs_attention.length === 0}
            <p class="empty muted">Nothing out of limit today.</p>
          {:else}
            <Table unsorted ariaLabel="Readings that need attention">
              {#snippet header()}
                <TableHeaderCell label="Time" />
                <TableHeaderCell label="Result" />
                <TableHeaderCell label="Record" />
                <TableHeaderCell label="Why" />
                <TableHeaderCell label="Corrective action" />
              {/snippet}
              {#each today.needs_attention as r (r.id)}
                {@const o = outcomeStyle(r.outcome)}
                <tr>
                  <td class="num">{dublinTime(doneAt(r))}</td>
                  <td><Chip tone={o.tone} uppercase={false}>{o.label}</Chip></td>
                  <td><a href={`/records?id=${r.id}`}>{title(r)}</a>&nbsp;<span class="muted">· {r.logged_by_name}</span></td>
                  <td>{r.outcome_reason}</td>
                  <td>
                    {#if r.corrective_action}{r.corrective_action}{:else}<Chip tone="danger" uppercase={false}>No action recorded</Chip>{/if}
                  </td>
                </tr>
              {/each}
            </Table>
          {/if}
        </Card>
      </section>

      <section>
        <Card padding="md" title="Batches in use">
          {#if today.batches.length === 0}
            <p class="muted">No batch labels in date.</p>
          {:else}
            <ul class="batches">
              {#each today.batches as b (b.record.id)}
                <li>
                  <div class="name">
                    <strong>{b.record.reading.product}</strong>
                    <span class="muted small">{details(b.record)}</span>
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
          {/if}
        </Card>
      </section>
    </div>
  {/if}
</div>

<style>
  .checklist li.longer {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-1);
  }
  .longer .top {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
    gap: var(--space-2);
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--space-5);
  }
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(280px, 1fr);
    gap: var(--space-6);
    align-items: start;
  }
  .checklist,
  .batches {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .checklist li,
  .batches li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }
  .name {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  .end {
    text-align: right;
  }
  .empty {
    margin: 0;
    padding: var(--space-5) var(--space-6);
  }
  p {
    margin: 0;
  }
  @media (max-width: 900px) {
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
