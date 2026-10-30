<script lang="ts">
  import { Button, Notice } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../lib/api";
  import { dublinTime } from "../lib/format";
  import { live, onChange } from "../lib/live.svelte";
  import { screen, setScreen } from "../lib/screen.svelte";

  // A screen facing customers: takeaway numbers being prepared and ready to
  // collect. It runs on a paired device with no staff login.

  let board = $state<Schemas["CollectionBoard"] | null>(null);
  let error = $state("");
  let now = $state(new Date());
  let askLeave = $state(false);

  async function load() {
    try {
      board = await api.collection();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    const off = onChange((k) => {
      if (k === "order" || k === "ready" || k === "settings") void load();
    });
    // A safety net if live updates drop.
    const poll = setInterval(load, 15_000);
    const tick = setInterval(() => (now = new Date()), 10_000);
    return () => {
      off();
      clearInterval(poll);
      clearInterval(tick);
    };
  });

  const label = (o: Schemas["CollectionOrder"]) => (screen.names && o.name ? o.name : "");
</script>

<div class="collection">
  <header>
    <h1>{board?.restaurant || "Orders"}</h1>
    <span class="clock num">{dublinTime(now)}</span>
  </header>

  {#if live.status === "offline" || error}
    <div class="offline"><Notice tone="warning" message="Not connected: these numbers may be out of date." /></div>
  {/if}

  <main>
    <section class="ready" aria-labelledby="ready-h">
      <h2 id="ready-h">Ready to collect</h2>
      {#if board && board.ready.length > 0}
        <ul>
          {#each board.ready as o (o.number)}
            <li><span class="number num">{o.number}</span>{#if label(o)}<span class="name">{label(o)}</span>{/if}</li>
          {/each}
        </ul>
      {:else}
        <p class="none">Nothing waiting</p>
      {/if}
    </section>
    <section class="preparing" aria-labelledby="prep-h">
      <h2 id="prep-h">Preparing</h2>
      {#if board && board.preparing.length > 0}
        <ul>
          {#each board.preparing as o (o.number)}
            <li><span class="number num">{o.number}</span>{#if label(o)}<span class="name">{label(o)}</span>{/if}</li>
          {/each}
        </ul>
      {:else}
        <p class="none">No orders being prepared</p>
      {/if}
    </section>
  </main>

  <footer>
    {#if askLeave}
      <span>Leave the collection screen? Staff then log in to choose another.</span>
      <Button size="sm" label="Stay" onclick={() => (askLeave = false)} />
      <Button size="sm" tone="info" surface="solid" label="Leave" onclick={() => setScreen("desktop")} />
    {:else}
      <Button size="sm" label="Staff" onclick={() => (askLeave = true)} />
    {/if}
  </footer>
</div>

<style>
  .collection {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    padding: var(--space-6) var(--space-7);
    gap: var(--space-5);
  }
  header {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
  }
  h1 {
    font-size: clamp(1.75rem, 3vw, 3rem);
  }
  .clock {
    font-size: clamp(1.5rem, 2.5vw, 2.5rem);
  }
  main {
    flex: 1;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-7);
  }
  h2 {
    font-size: clamp(1.5rem, 2.5vw, 2.5rem);
    padding-bottom: var(--space-3);
    border-bottom: 4px solid currentColor;
    margin-bottom: var(--space-5);
  }
  .ready h2 {
    color: var(--accent-green);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
  }
  li {
    display: flex;
    flex-direction: column;
    align-items: center;
    min-width: 7ch;
    padding: var(--space-3) var(--space-5);
    border-radius: var(--radius-lg, 12px);
    border: 2px solid var(--border-default, currentColor);
  }
  .ready li {
    border-color: var(--accent-green);
    border-width: 4px;
  }
  .number {
    font-size: clamp(3rem, 7vw, 7rem);
    font-weight: 800;
    line-height: 1.1;
  }
  .preparing .number {
    font-size: clamp(2rem, 4.5vw, 4.5rem);
    font-weight: 700;
  }
  .name {
    font-size: clamp(1rem, 1.6vw, 1.6rem);
  }
  .none {
    font-size: clamp(1.25rem, 2vw, 2rem);
    color: var(--text-muted, inherit);
  }
  footer {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: var(--space-3);
  }
  @media (max-width: 760px) {
    main {
      grid-template-columns: 1fr;
    }
  }
</style>
