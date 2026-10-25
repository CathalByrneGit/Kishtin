<script lang="ts">
  import { Button, Chip, EmptyState, Notice, SegmentedControl, Spinner, showFlash } from "@kenn-io/kit-ui";
  import { Play } from "@lucide/svelte";
  import { api, message, type Schemas } from "../lib/api";
  import { dublinTime } from "../lib/format";
  import { countdown, itemState, nextLabel } from "../lib/kitchen";
  import { canWorkStation, kitchenFeed } from "../lib/kitchenfeed.svelte";
  import { param, setParam } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  interface Props {
    /** large type and buttons, for a station tablet */
    big?: boolean;
    /** show only this station, with no station picker */
    station?: string | null;
  }
  let { big = false, station: locked = null }: Props = $props();

  const feed = kitchenFeed();
  const cook = $derived(canWorkStation(session.user?.role));
  let busy = $state<string | null>(null);

  const stations = $derived(feed.view?.plan.stations ?? []);
  const selectedId = $derived(locked ?? param("station") ?? stations[0]?.station_id ?? "");
  const station = $derived(stations.find((s) => s.station_id === selectedId) ?? null);
  const startNow = $derived(station?.actions.filter((a) => a.now) ?? []);
  const comingUp = $derived(station?.actions.filter((a) => !a.now) ?? []);

  /** Items in progress, grouped when they are the same dish in the same state and finish together. */
  const progress = $derived.by(() => {
    const out: { key: string; items: Schemas["Progress"][] }[] = [];
    for (const p of station?.in_progress ?? []) {
      const key = `${p.dish_name}|${(p.modifiers ?? []).join(",")}|${p.state}|${p.phase_ends_at}`;
      const g = out.find((x) => x.key === key);
      if (g) g.items.push(p);
      else out.push({ key, items: [p] });
    }
    return out;
  });

  // from is the state the cook saw. If someone else moved the items first,
  // the server refuses and the screen reloads to show the latest.
  async function move(key: string, ids: string[], from: string, done: string) {
    busy = key;
    try {
      await api.moveItems({ item_ids: ids, from });
      showFlash(done, { tone: "success" });
      await feed.reload();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
      await feed.reload().catch(() => {});
    } finally {
      busy = null;
    }
  }

  function dish(name: string, mods: string[] | null | undefined): string {
    return mods && mods.length > 0 ? `${name} (${mods.join(", ")})` : name;
  }

  function labels(items: Schemas["Progress"][]): string {
    return [...new Set(items.map((p) => p.label))].join(", ");
  }
</script>

<div class="page" class:big class:kitchen={big}>
  {#if !big}
  <header class="head">
    <div>
      <h1>{station?.name ?? "Stations"}</h1>
      <p>What this station should start, in the order the scheduler recommends, and why.</p>
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
        onchange={(v) => setParam("station", v)}
        options={stations.map((s) => ({ value: s.station_id, label: `${s.name}${s.actions.some((a) => a.now) ? " •" : ""}` }))}
      />
    </div>
    {/if}

    {#if !station}
      <Notice tone="warning" title="Station not found" message="This screen's station is no longer in use. Choose another from the menu at the top right." />
    {:else}
      <div class="columns">
        <section>
          <h2 class="section-title">Start now</h2>
          {#each startNow as a (a.item_ids.join(","))}
            <article class="action" class:late={a.late}>
              <div class="action-head">
                <strong class="big">{a.quantity}× {dish(a.dish_name, a.modifiers)}</strong>
                {#if a.late}<Chip tone="danger" uppercase={false}>Late</Chip>{/if}
              </div>
              <span class="for">{a.equipment_name ? `On the ${a.equipment_name.toLowerCase()} · ` : ""}for {a.labels.join(" and ")}</span>
              <p class="why">{a.reason}</p>
              {#if cook}
                <Button tone="success" surface="solid" size={big ? undefined : "lg"} label={busy === a.item_ids.join(",") ? "Starting…" : a.quantity > 1 ? `Start all ${a.quantity}` : "Start"} disabled={busy !== null} onclick={() => move(a.item_ids.join(","), a.item_ids, "fired", `Started ${a.quantity}× ${a.dish_name}`)}>
                  <Play size={18} />
                </Button>
              {/if}
            </article>
          {:else}
            <p class="muted">Nothing to start right now.</p>
          {/each}
        </section>

        <section>
          <h2 class="section-title">In progress</h2>
          {#each progress as g (g.key)}
            {@const p = g.items[0]!}
            {@const s = itemState(p.state)}
            <article class="progress" class:overdue={new Date(p.phase_ends_at) < feed.now}>
              <div class="action-head">
                <strong>{g.items.length}× {dish(p.dish_name, p.modifiers)}</strong>
                <Chip size="sm" uppercase={false} tone={s.tone}>{s.label}</Chip>
              </div>
              <span class="for">for {labels(g.items)}</span>
              <div class="timer">
                <span class="clock num">{countdown(p.phase_ends_at, feed.now)}</span>
                <span class="muted">{new Date(p.phase_ends_at) < feed.now ? "overdue" : `until ${dublinTime(p.phase_ends_at)}`}</span>
              </div>
              {#if cook}
                <Button tone="info" surface="solid" label={nextLabel(p.state)} disabled={busy !== null} onclick={() => move(g.key, g.items.map((x) => x.item_id), p.state, `${nextLabel(p.state)}: ${p.dish_name}`)} />
              {/if}
            </article>
          {:else}
            <p class="muted">Nothing cooking here.</p>
          {/each}
        </section>

        <section>
          <h2 class="section-title">Coming up</h2>
          {#each comingUp as a (a.item_ids.join(","))}
            <article class="upcoming">
              <div class="action-head">
                <strong>{a.quantity}× {dish(a.dish_name, a.modifiers)}</strong>
                <span class="num">{dublinTime(a.start_at)} · in {countdown(a.start_at, feed.now)}</span>
              </div>
              <span class="for">for {a.labels.join(" and ")}</span>
              <p class="why">{a.reason}</p>
              {#if cook}
                <Button size={big ? undefined : "sm"} label="Start early" disabled={busy !== null} onclick={() => move(a.item_ids.join(","), a.item_ids, "fired", `Started ${a.quantity}× ${a.dish_name}`)} />
              {/if}
            </article>
          {:else}
            <p class="muted">Nothing planned yet.</p>
          {/each}
        </section>
      </div>
    {/if}
  {/if}
</div>

<style>
  .columns {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-6);
    align-items: start;
  }
  section {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .section-title {
    margin: 0;
  }
  .action,
  .progress,
  .upcoming {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-5);
    background: var(--bg-surface);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-lg);
  }
  .action {
    border-color: var(--accent-green);
    box-shadow: inset 4px 0 0 var(--accent-green);
  }
  .action.late,
  .progress.overdue {
    border-color: var(--accent-red);
    box-shadow: inset 4px 0 0 var(--accent-red);
  }
  .action-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .big {
    font-size: var(--font-size-xl);
  }
  .for {
    color: var(--text-secondary);
  }
  .why {
    margin: 0;
    font-size: var(--font-size-sm);
    color: var(--text-secondary);
  }
  .timer {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
  }
  .clock {
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-bold);
  }
  .overdue .clock {
    color: var(--accent-red);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
  @media (max-width: 900px) {
    .columns {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  /* Station tablet: bigger type; columns wrap on a portrait tablet. */
  .big .columns {
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr));
  }
  .big .why {
    font-size: var(--font-size-md);
  }
  .big .for,
  .big .action-head strong {
    font-size: var(--font-size-lg);
  }
  .big .big {
    font-size: var(--font-size-2xl);
  }
</style>
