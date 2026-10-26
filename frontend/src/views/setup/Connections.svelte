<script lang="ts">
  import { Button, Card, Chip, Notice, Spinner } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { dublinTime } from "../../lib/format";
  import { onChange } from "../../lib/live.svelte";
  import { navigate } from "../../lib/router.svelte";

  let list = $state<Schemas["ConnectorStatus"][] | null>(null);
  let error = $state("");

  async function load() {
    try {
      list = (await api.connectors()).connectors;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((k) => {
      if (k === "settings" || k === "order") void load();
    });
  });

  const SOURCES: Record<string, string> = {
    saved: "connected here",
    environment: "set in the server's environment",
    "kishtin.toml": "set in kishtin.toml",
  };
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Connections</h1>
      <p>
        Other systems Kishtin can work with. Nothing is sent anywhere until a manager connects it and turns it on,
        after seeing exactly what is sent. The kitchen keeps working if a connection is down.
      </p>
    </div>
  </header>

  {#if error}<Notice tone="error" message={error} />{/if}
  {#if !list}
    {#if !error}<Spinner />{/if}
  {:else}
    <div class="grid">
      {#each list as c (c.id)}
        <Card padding="md">
          <div class="stack">
            <div class="row between">
              <h2>{c.name}</h2>
              <div class="row">
                <Chip uppercase={false} dot tone={c.connected ? "success" : "neutral"}>{c.connected ? "Connected" : "Not connected"}</Chip>
                {#if c.enabled}<Chip uppercase={false} tone="info">In use</Chip>{/if}
              </div>
            </div>
            <p>{c.summary}</p>
            <p class="muted small"><strong>Sends:</strong> {c.sends}</p>
            {#if c.connected}
              <p class="muted small">
                {SOURCES[c.source ?? ""] ?? ""}{c.environment ? ` · ${c.environment}` : ""}{c.last_ok_at ? ` · last worked ${dublinTime(c.last_ok_at)}` : ""}
              </p>
            {/if}
            {#if c.last_error}<Notice tone="warning" title="Last problem" message={c.last_error} />{/if}
            <div>
              <Button tone={c.connected ? "neutral" : "info"} surface={c.connected ? "outline" : "solid"} label={c.connected ? "Open" : "Set up"} onclick={() => navigate(`/setup/connections/${c.id}`)} />
            </div>
          </div>
        </Card>
      {/each}
    </div>
  {/if}
</div>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 380px), 1fr));
    gap: var(--space-5);
  }
  h2 {
    font-size: var(--font-size-lg);
  }
  .between {
    justify-content: space-between;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
