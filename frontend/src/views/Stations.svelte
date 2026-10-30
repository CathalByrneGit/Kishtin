<script lang="ts">
  import { allergenLine } from "../lib/allergens.svelte";
  import { Button, Chip, EmptyState, IconButton, Notice, SegmentedControl, Spinner, showFlash } from "@kenn-io/kit-ui";
  import { Info } from "@lucide/svelte";
  import AllDayStrip from "../components/AllDayStrip.svelte";
  import { api, message, type OrderItem } from "../lib/api";
  import { dublinTime } from "../lib/format";
  import { countdown, courseLabel, courseStatus, itemName, itemState, nextLabel, orderPlace, orderTitle } from "../lib/kitchen";
  import { canWorkStation, kitchenFeed } from "../lib/kitchenfeed.svelte";
  import { param, setParam } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";
  import { type AllDay, type TicketLine, allDay, lineKey, recentlyDone, stationTickets, stepBack } from "../lib/tickets";

  interface Props {
    /** large type and buttons, for a station tablet */
    big?: boolean;
    /** show only this station, with no station picker */
    station?: string | null;
  }
  let { big = false, station: locked = null }: Props = $props();

  const feed = kitchenFeed();
  const cook = $derived(canWorkStation());
  let busy = $state<string | null>(null);
  /** the dish picked in the all-day strip, to show on its own */
  let only = $state<string | null>(null);
  /** lines whose reason is open */
  let why = $state<Record<string, boolean>>({});

  const stations = $derived(feed.view?.plan.stations ?? []);
  const selectedId = $derived(locked ?? param("station") ?? stations[0]?.station_id ?? "");
  const station = $derived(stations.find((s) => s.station_id === selectedId) ?? null);
  const tickets = $derived(feed.view && station ? stationTickets(feed.view, station.station_id) : []);
  const shown = $derived(only ? tickets.filter((t) => t.lines.some((l) => lineKey(l.item) === only)) : tickets);
  const counts = $derived(feed.view && station ? allDay(feed.view, station.station_id) : []);
  const done = $derived(feed.view && station ? recentlyDone(feed.view, station.station_id, feed.now) : []);

  // from is the state the cook saw. If someone else moved the items first,
  // the server refuses and the screen reloads to show the latest.
  async function move(key: string, ids: string[], from: string, said: string, to?: string) {
    busy = key;
    try {
      await api.moveItems({ item_ids: ids, from, to });
      showFlash(said, { tone: "success" });
      await feed.reload();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
      await feed.reload().catch(() => {});
    } finally {
      busy = null;
    }
  }

  function startTogether(r: AllDay) {
    void move(`all|${r.key}`, r.startNow, "fired", `Started ${r.startNow.length}× ${r.name}`);
  }

  /** hint says when a line needs a hand: start now, start at, or how long left. */
  function hint(l: TicketLine): { text: string; tone: "success" | "danger" | "neutral" | "warning" } | null {
    const it = l.item;
    if (it.state === "fired" && l.action) {
      if (l.action.now) return l.action.late ? { text: "Start now: late", tone: "danger" } : { text: "Start now", tone: "success" };
      return { text: `Start ${dublinTime(l.action.start_at)}`, tone: "neutral" };
    }
    if (l.progress) {
      const over = new Date(l.progress.phase_ends_at) < feed.now;
      return { text: over ? `${countdown(l.progress.phase_ends_at, feed.now).replace("−", "")} over` : `${countdown(l.progress.phase_ends_at, feed.now)} left`, tone: over ? "danger" : "warning" };
    }
    return null;
  }

  function elapsed(since: string | undefined): string {
    if (!since) return "";
    const s = Math.max(0, Math.round((feed.now.getTime() - new Date(since).getTime()) / 1000));
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
  }

  function undo(it: OrderItem, label: string) {
    const to = stepBack(it);
    void move(`undo|${it.id}`, [it.id], "ready", `${itemName(it)} for ${label}: back to ${itemState(to).label.toLowerCase()}`, to);
  }
</script>

<div class="page" class:big class:kitchen={big}>
  {#if !big}
    <header class="head">
      <div>
        <h1>{station?.name ?? "Stations"}</h1>
        <p>Every ticket with work for this station, soonest first. Each item has its own button; the i says why the scheduler chose its time.</p>
      </div>
      {#if feed.view}<span class="muted small">Plan at {dublinTime(feed.view.plan.at)}</span>{/if}
    </header>
  {/if}

  {#if feed.error}<Notice tone="error" title="Could not load the kitchen" message={feed.error} />{/if}

  {#if !feed.view}
    {#if !feed.error}<Spinner />{/if}
  {:else if stations.length === 0}
    <EmptyState title="No stations" description="A manager adds stations, equipment and dishes in Setup, Menu." />
  {:else}
    {#if !locked}
      <div>
        <SegmentedControl
          ariaLabel="Station"
          value={selectedId}
          onchange={(v) => {
            only = null;
            setParam("station", v);
          }}
          options={stations.map((s) => ({ value: s.station_id, label: `${s.name}${s.actions.some((a) => a.now) ? " •" : ""}` }))}
        />
      </div>
    {/if}

    {#if !station}
      <Notice tone="warning" title="Station not found" message="This screen's station is no longer in use. Choose another from the menu at the top right." />
    {:else}
      <AllDayStrip rows={counts} selected={only} onselect={(k) => (only = k)} onstart={cook ? startTogether : undefined} busy={busy !== null} />

      <div class="layout">
        <div class="tickets">
          {#each shown as t (t.key)}
            {@const st = t.status ? courseStatus(t.status.status) : null}
            <article class="ticket" class:late={t.status?.status === "late"} class:risk={t.status?.status === "at_risk"} class:allergy={t.order.allergy}>
              <header>
                <div class="who">
                  <strong class="name">{orderTitle(t.order)}</strong>
                  <span class="muted small">{t.order.name ? `${orderPlace(t.order)} · ` : ""}#{t.order.number} · {courseLabel(t.course)}</span>
                </div>
                <div class="when">
                  <span class="elapsed num" title="Since fired">{elapsed(t.firedAt)}</span>
                  {#if st}<Chip size="sm" uppercase={false} tone={st.tone} dot>{st.label}</Chip>{/if}
                </div>
              </header>
              {#if t.order.allergy}<div class="flag"><Chip size="sm" tone="danger" uppercase={false}>Allergy</Chip>{#if t.order.notes}<span class="small">{t.order.notes}</span>{/if}</div>{/if}
              <ul>
                {#each t.lines as l (l.item.id)}
                  {@const it = l.item}
                  {@const h = hint(l)}
                  {@const finished = it.state === "ready" || it.state === "served"}
                  <li class:finished class:picked={only !== null && lineKey(it) === only}>
                    <div class="line">
                      <div class="what">
                        <span class="dish">{itemName(it)}</span>
                        {#if it.notes}<span class="note">{it.notes}</span>{/if}
                        {#if t.order.allergy}
                          {@const al = allergenLine(it.dish_id)}
                          {#if al}<span class="allergens" class:unchecked={al.unchecked}>{al.text}</span>{/if}
                        {/if}
                      </div>
                      {#if finished}
                        <Chip size="sm" uppercase={false} tone="success">Ready</Chip>
                      {:else}
                        {#if h}<span class={`hint ${h.tone}`}>{h.text}</span>{/if}
                        {#if l.action}
                          <IconButton
                            ariaLabel={why[it.id] ? "Hide why" : `Why this time for ${itemName(it)}`}
                            title="Why?"
                            size={big ? undefined : "sm"}
                            onclick={() => (why[it.id] = !why[it.id])}
                          >
                            <Info size={big ? 20 : 16} />
                          </IconButton>
                        {/if}
                        {#if it.state !== "fired"}<Chip size="sm" uppercase={false} tone={itemState(it.state).tone}>{itemState(it.state).label}</Chip>{/if}
                        {#if cook}
                          <Button
                            tone={it.state === "fired" ? "success" : "info"}
                            surface="solid"
                            size={big ? undefined : "sm"}
                            label={busy === it.id ? "…" : nextLabel(it.state)}
                            disabled={busy !== null}
                            onclick={() => move(it.id, [it.id], it.state, `${nextLabel(it.state)}: ${itemName(it)} for ${orderTitle(t.order)}`)}
                          />
                        {/if}
                      {/if}
                    </div>
                    {#if l.action && !finished && why[it.id]}<p class="why">{l.action.reason}</p>{/if}
                  </li>
                {/each}
              </ul>
              {#if t.targetAt}<footer class="muted small">Due at the pass {dublinTime(t.targetAt)}</footer>{/if}
            </article>
          {:else}
            <p class="muted">{only ? "No tickets with that dish." : "Nothing for this station right now."}</p>
          {/each}
        </div>

        <aside class="done">
          <h2 class="section-title">Recently done</h2>
          {#each done as d (d.item.id)}
            <div class="done-row">
              <div>
                <span>{itemName(d.item)}</span>
                <span class="muted small">{orderTitle(d.order)} · {dublinTime(d.item.state_at)}</span>
              </div>
              {#if cook}<Button size={big ? undefined : "sm"} label="Undo" disabled={busy !== null} onclick={() => undo(d.item, orderTitle(d.order))} />{/if}
            </div>
          {:else}
            <p class="muted small">Items made ready here in the last 15 minutes show here, to undo a tap.</p>
          {/each}
        </aside>
      </div>
    {/if}
  {/if}
</div>

<style>
  .allergens {
    display: block;
    font-weight: 600;
    font-size: var(--font-size-sm);
  }
  .allergens.unchecked {
    color: var(--accent-red);
  }
  .layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(220px, 280px);
    gap: var(--space-6);
    align-items: start;
  }
  .tickets {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr));
    gap: var(--space-4);
    align-items: start;
  }
  .ticket {
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-lg);
    box-shadow: inset 0 4px 0 var(--accent-green);
    overflow: hidden;
  }
  .ticket.risk {
    box-shadow: inset 0 4px 0 var(--accent-amber, var(--accent-orange));
  }
  .ticket.late {
    border-color: var(--accent-red);
    box-shadow: inset 0 4px 0 var(--accent-red);
  }
  .ticket header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-4) var(--space-2);
  }
  .who {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .name {
    font-size: var(--font-size-lg);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .big .name {
    font-size: var(--font-size-xl);
  }
  .when {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--space-1);
  }
  .elapsed {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-bold);
  }
  .late .elapsed {
    color: var(--accent-red);
  }
  .flag {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 0 var(--space-4) var(--space-2);
    color: var(--accent-red);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    padding: var(--space-3) var(--space-4);
    border-top: var(--border-width) solid var(--border-muted);
  }
  li.picked {
    background: var(--bg-inset);
  }
  li.finished .dish {
    text-decoration: line-through;
    color: var(--text-secondary);
  }
  .line {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .what {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .dish {
    font-weight: var(--font-weight-semibold);
  }
  .big .dish {
    font-size: var(--font-size-lg);
  }
  .note {
    font-size: var(--font-size-sm);
    color: var(--accent-red);
  }
  .hint {
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-semibold);
    white-space: nowrap;
  }
  .hint.success {
    color: var(--accent-green);
  }
  .hint.danger {
    color: var(--accent-red);
  }
  .hint.warning {
    font-variant-numeric: tabular-nums;
  }
  .hint.neutral {
    color: var(--text-secondary);
  }
  .why {
    margin: var(--space-1) 0 0;
    font-size: var(--font-size-sm);
    color: var(--text-secondary);
  }
  footer {
    padding: var(--space-2) var(--space-4) var(--space-3);
    border-top: var(--border-width) solid var(--border-muted);
  }
  .done {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .done-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3);
    background: var(--bg-surface);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
  }
  .done-row > div {
    display: flex;
    flex-direction: column;
  }
  .section-title {
    margin: 0;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  @media (max-width: 900px) {
    .layout {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
