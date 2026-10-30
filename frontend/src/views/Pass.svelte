<script lang="ts">
  import { allergenLine } from "../lib/allergens.svelte";
  import { Button, Chip, EmptyState, IconButton, MetricCard, Modal, Notice, SegmentedControl, Spinner, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { Flame, History, Pause, TimerReset } from "@lucide/svelte";
  import OrderDrawer from "../components/OrderDrawer.svelte";
  import { api, message, type Course, type Order, type OrderItem, type Schemas } from "../lib/api";
  import { dublinTime, dublinToDate, dublinDate, isClock, normaliseClock } from "../lib/format";
  import AllDayStrip from "../components/AllDayStrip.svelte";
  import { allDay, lineKey } from "../lib/tickets";
  import { countdown, courseLabel, courseStatus, itemName, itemState, orderLabel, orderPlace, orderTitle } from "../lib/kitchen";
  import { canRunPass, kitchenFeed } from "../lib/kitchenfeed.svelte";
  import { param, setParam } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  interface Props {
    /** large type and buttons, for a wall-mounted screen */
    big?: boolean;
    /** no heading or figures, for a phone */
    compact?: boolean;
  }
  let { big = false, compact = false }: Props = $props();

  const feed = kitchenFeed();
  const expo = $derived(canRunPass());
  const filter = $derived(param("show") ?? "all");
  let busy = $state<string | null>(null);
  let drawer = $state<string | null>(null);
  let target = $state<{ order: Order; course: Course; time: string; reason: string } | null>(null);

  const statusOf = $derived.by(() => {
    const m = new Map<string, Schemas["CourseStatus"]>();
    for (const c of feed.view?.plan.courses ?? []) m.set(`${c.order_id}|${c.course}`, c);
    return m;
  });

  const counts = $derived.by(() => {
    const c = { on_track: 0, at_risk: 0, late: 0, ready: 0, held: 0 };
    for (const s of feed.view?.plan.courses ?? []) c[s.status as keyof typeof c]++;
    for (const o of feed.view?.orders ?? []) c.held += o.courses.filter((x) => x.state === "held").length;
    return c;
  });

  function needsAction(o: Order): boolean {
    return o.courses.some((c) => {
      const s = statusOf.get(`${o.id}|${c.course}`)?.status;
      return s === "at_risk" || s === "late" || s === "ready";
    });
  }

  /** the dish picked in the all-day strip, to show only orders with it */
  let only = $state<string | null>(null);
  const counts2 = $derived(feed.view ? allDay(feed.view) : []);

  const orders = $derived.by(() => {
    let list = feed.view?.orders ?? [];
    if (filter === "action") list = list.filter(needsAction);
    if (filter === "held") list = list.filter((o) => o.courses.some((c) => c.state === "held"));
    if (only) list = list.filter((o) => o.items.some((it) => lineKey(it) === only && it.state !== "served" && it.state !== "voided"));
    return list;
  });

  /** group shows identical items once: "2× Burger (Well done)". */
  function group(items: OrderItem[]): { key: string; n: number; item: OrderItem }[] {
    const out: { key: string; n: number; item: OrderItem }[] = [];
    for (const it of items) {
      const key = `${itemName(it)}|${it.notes ?? ""}|${it.state}`;
      const g = out.find((x) => x.key === key);
      if (g) g.n++;
      else out.push({ key, n: 1, item: it });
    }
    return out;
  }

  async function act(key: string, fn: () => Promise<unknown>, done: string) {
    busy = key;
    try {
      await fn();
      showFlash(done, { tone: "success" });
      await feed.reload();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = null;
    }
  }

  function openTarget(o: Order, c: Schemas["Course"]) {
    target = { order: o, course: c.course as Course, time: c.target_at ? dublinTime(c.target_at) : "", reason: "" };
  }

  async function saveTarget() {
    if (!target) return;
    const t = target;
    const hhmm = normaliseClock(t.time);
    let at = dublinToDate(dublinDate(new Date()), hhmm);
    if (at.getTime() < Date.now() - 12 * 3600_000) at = new Date(at.getTime() + 24 * 3600_000);
    await act("target", () => api.setTarget(t.order.id, t.course, at.toISOString(), t.reason), "Target changed");
    target = null;
  }
</script>

<div class="page" class:big class:kitchen={big}>
  {#if !big && !compact}
  <header class="head">
    <div>
      <h1>Pass</h1>
      <p>Every open order. Fire courses, watch them come together, and serve them when every item is ready.</p>
    </div>
    {#if feed.view}
      <span class="muted small">Plan at {dublinTime(feed.view.plan.at)} · {feed.view.plan.strategy === "fifo" ? "first in, first out" : "backward scheduling"}</span>
    {/if}
  </header>
  {/if}

  {#if feed.error}<Notice tone="error" title="Could not load the kitchen" message={feed.error} />{/if}

  {#if !feed.view}
    {#if !feed.error}<Spinner />{/if}
  {:else}
    {#if !compact}
    <div class="metrics">
      <MetricCard label="Open orders" value={String(feed.view.orders.length)} />
      <MetricCard label="On track" value={String(counts.on_track)} tone="success" />
      <MetricCard label="At risk" value={String(counts.at_risk)} tone={counts.at_risk > 0 ? "warning" : "neutral"} />
      <MetricCard label="Late" value={String(counts.late)} tone={counts.late > 0 ? "danger" : "neutral"} />
      <MetricCard label="Ready to serve" value={String(counts.ready)} tone={counts.ready > 0 ? "info" : "neutral"} />
      <MetricCard label="Held courses" value={String(counts.held)} />
    </div>
    {/if}

    <div>
      <SegmentedControl
        ariaLabel="Show"
        value={filter}
        onchange={(v) => setParam("show", v === "all" ? null : v)}
        options={[
          { value: "all", label: "All orders" },
          { value: "action", label: "Needs action" },
          { value: "held", label: "Held courses" },
        ]}
      />
    </div>

    <AllDayStrip rows={counts2} selected={only} onselect={(k) => (only = k)} showHeld />

    {#if feed.view.orders.length === 0}
      <EmptyState title="No open orders" description="New orders appear here as soon as they are entered." />
    {:else if orders.length === 0}
      <EmptyState title="Nothing here" description="No open orders match this filter." />
    {/if}

    <div class="cards">
      {#each orders as o (o.id)}
        <article class="card" class:allergy={o.allergy}>
          <header>
            <div class="title">
              <h2>{orderTitle(o)}</h2>
              <span class="muted num">{o.name ? `${orderPlace(o)} · ` : ""}#{o.number} · {dublinTime(o.created_at)}{o.covers ? ` · ${o.covers} covers` : ""}</span>
            </div>
            {#if o.created_by === "pos:square"}<Chip uppercase={false}>Square</Chip>{/if}
            {#if o.allergy}<Chip tone="danger" uppercase={false}>Allergy</Chip>{/if}
            <IconButton ariaLabel={`History for ${orderLabel(o)}`} title="Order and history" onclick={() => (drawer = o.id)}><History size={16} /></IconButton>
          </header>
          {#if o.notes}<p class="notes">{o.notes}</p>{/if}

          {#each o.courses as c (c.course)}
            {@const items = o.items.filter((it) => it.course === c.course)}
            {@const st = statusOf.get(`${o.id}|${c.course}`)}
            {@const allReady = items.every((it) => it.state === "ready" || it.state === "served" || it.state === "voided")}
            {@const canHold = items.every((it) => it.state === "fired" || it.state === "held" || it.state === "voided")}
            {@const canReady = !allReady && items.every((it) => ["cooking", "resting", "plating", "ready", "served", "voided"].includes(it.state))}
            <section class="course" class:served={c.state === "served"}>
              <div class="course-head">
                <strong>{courseLabel(c.course)}</strong>
                {#if c.state === "held"}
                  <Chip uppercase={false}>Held</Chip>
                {:else if c.state === "served"}
                  <Chip uppercase={false} tone="muted">Served {dublinTime(c.served_at ?? "")}</Chip>
                {:else if st}
                  {@const s = courseStatus(st.status)}
                  <Chip uppercase={false} tone={s.tone} dot>{s.label}</Chip>
                  {#if c.urgent}<Chip uppercase={false} tone="danger">Urgent</Chip>{/if}
                {/if}
                <span class="grow"></span>
                {#if c.state === "fired" && c.target_at}
                  <span class="due num" class:past={new Date(c.target_at) < feed.now}>
                    Due {dublinTime(c.target_at)} · {countdown(c.target_at, feed.now)}
                  </span>
                {/if}
              </div>
              {#if st && c.state === "fired"}
                <p class="reason muted">{st.reason}</p>
              {/if}
              <ul class="items">
                {#each group(items) as g (g.key)}
                  {@const s = itemState(g.item.state)}
                  <li class:done={g.item.state === "served" || g.item.state === "voided"} class:picked={only !== null && lineKey(g.item) === only}>
                    <span>
                      <span class="num">{g.n}×</span> {itemName(g.item)}{#if g.item.notes}<span class="muted">: {g.item.notes}</span>{/if}
                      {#if o.allergy}
                        {@const al = allergenLine(g.item.dish_id)}
                        {#if al}<span class="allergens" class:unchecked={al.unchecked}>{al.text}</span>{/if}
                      {/if}
                    </span>
                    <Chip size="sm" uppercase={false} tone={s.tone}>{s.label}</Chip>
                  </li>
                {/each}
              </ul>
              {#if expo}
                <div class="actions">
                  {#if c.state === "held"}
                    <Button tone="info" surface="solid" label={busy === `${o.id}|${c.course}` ? "Firing…" : `Fire ${courseLabel(c.course).toLowerCase()}`} disabled={busy !== null} onclick={() => act(`${o.id}|${c.course}`, () => api.fire(o.id, c.course as Course), `${orderLabel(o)} ${courseLabel(c.course).toLowerCase()} fired`)}>
                      <Flame size={16} />
                    </Button>
                  {:else if c.state === "fired"}
                    {#if canReady}
                      <Button tone="info" surface="solid" label="Mark ready" disabled={busy !== null} onclick={() => act(`${o.id}|${c.course}`, () => api.ready(o.id, c.course as Course), `${orderLabel(o)} ${courseLabel(c.course).toLowerCase()} ready at the pass`)} />
                    {/if}
                    <Button tone="success" surface="solid" label={`Serve ${courseLabel(c.course).toLowerCase()}`} disabled={busy !== null || !allReady} onclick={() => act(`${o.id}|${c.course}`, () => api.serve(o.id, c.course as Course), `${orderLabel(o)} ${courseLabel(c.course).toLowerCase()} served`)} />
                    <Button label="Change time" disabled={busy !== null} onclick={() => openTarget(o, c)}><TimerReset size={16} /></Button>
                    {#if canHold}
                      <Button label="Hold again" disabled={busy !== null} onclick={() => act(`${o.id}|${c.course}`, () => api.hold(o.id, c.course as Course), `${orderLabel(o)} ${courseLabel(c.course).toLowerCase()} held again`)}><Pause size={16} /></Button>
                    {/if}
                    <Button label={c.urgent ? "Not urgent" : "Urgent"} tone={c.urgent ? "neutral" : "danger"} disabled={busy !== null} onclick={() => act(`${o.id}|${c.course}`, () => api.setUrgent(o.id, c.course as Course, !c.urgent), c.urgent ? "Urgent flag cleared" : "Flagged urgent")} />
                  {/if}
                </div>
              {/if}
            </section>
          {/each}

          {#if expo && o.items.every((it) => it.state === "served" || it.state === "voided")}
            <footer>
              <Button label="Close order" disabled={busy !== null} onclick={() => act(o.id, () => api.closeOrder(o.id), `${orderLabel(o)} closed`)} />
            </footer>
          {/if}
        </article>
      {/each}
    </div>
  {/if}
</div>

{#if drawer}
  <OrderDrawer orderId={drawer} onclose={() => (drawer = null)} />
{/if}

{#if target}
  <Modal title={`Change when ${orderLabel(target.order)} ${courseLabel(target.course).toLowerCase()} reach the pass`} onclose={() => (target = null)} maxWidth="480px">
    <div class="stack">
      <label class="field">
        <span>New time (HH:MM)</span>
        <TextInput bind:value={target.time} placeholder="19:30" block autofocus />
      </label>
      <label class="field">
        <span>Reason (for the log)</span>
        <TextInput bind:value={target.reason} placeholder="For example: table not ready" block />
      </label>
      <p class="muted small">The scheduler re-plans every station around the new time. The change is logged with your name.</p>
    </div>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (target = null)} />
        <Button tone="info" surface="solid" label="Save time" disabled={!isClock(normaliseClock(target?.time ?? "")) || busy !== null} onclick={saveTarget} />
      </div>
    {/snippet}
  </Modal>
{/if}

<style>
  .allergens {
    display: block;
    font-weight: 600;
    font-size: var(--font-size-sm);
  }
  .allergens.unchecked {
    color: var(--accent-red);
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-4);
  }
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
  }
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 360px), 1fr));
    gap: var(--space-5);
    align-items: start;
  }
  .card {
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }
  .card.allergy {
    border-color: var(--accent-red);
    box-shadow: inset 4px 0 0 var(--accent-red);
  }
  .card > header {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
    border-bottom: var(--border-width) solid var(--border-muted);
  }
  .title {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .title h2 {
    font-size: var(--font-size-xl);
  }
  .notes {
    margin: 0;
    padding: var(--space-3) var(--space-5);
    background: var(--bg-inset);
    font-weight: var(--font-weight-medium);
  }
  .course {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
  }
  .course + .course {
    border-top: var(--border-width) solid var(--border-muted);
  }
  .course.served {
    opacity: 0.65;
  }
  .course-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .grow {
    flex: 1;
  }
  .due {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
  }
  .due.past {
    color: var(--accent-red);
  }
  .reason {
    margin: 0;
    font-size: var(--font-size-sm);
  }
  .items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .items li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .items li.done {
    color: var(--text-secondary);
  }
  .items li.picked {
    font-weight: var(--font-weight-semibold);
    background: var(--bg-inset);
    border-radius: var(--radius-sm);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }
  .card > footer {
    padding: var(--space-4) var(--space-5);
    border-top: var(--border-width) solid var(--border-muted);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }

  /* Wall-mounted pass screen: bigger type, readable at arm's length. */
  .big .cards {
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 420px), 1fr));
  }
  .big .title h2 {
    font-size: var(--font-size-2xl);
  }
  .big .items li,
  .big .course-head strong {
    font-size: var(--font-size-lg);
  }
  .big .due {
    font-size: var(--font-size-xl);
  }
  .big .reason {
    font-size: var(--font-size-md);
  }
</style>
