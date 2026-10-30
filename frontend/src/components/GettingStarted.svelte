<script lang="ts">
  import { Button, Card, Chip } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, type Schemas } from "../lib/api";
  import { can } from "../lib/can.svelte";
  import { onChange } from "../lib/live.svelte";
  import { navigate } from "../lib/router.svelte";
  import { load, save } from "../lib/storage";

  // The getting-started checklist for managers. Each step ticks itself off
  // from what is set up and recorded. Hiding it is kept on this device.

  const KEY = "kishtin-getting-started-hidden";
  let progress = $state<Schemas["SetupProgress"] | null>(null);
  let hidden = $state(load(KEY) === "1");

  async function refresh() {
    if (!can("settings.manage")) return;
    try {
      progress = await api.gettingStarted();
    } catch {
      progress = null;
    }
  }

  onMount(() => {
    void refresh();
    // Orders change all service long; none of the steps depend on them.
    return onChange((kind) => {
      if (kind !== "order" && kind !== "ready") void refresh();
    });
  });

  $effect(() => {
    if (can("settings.manage") && progress === null) void refresh();
  });

  function hide() {
    hidden = true;
    save(KEY, "1");
  }

  const allDone = $derived(progress !== null && progress.done === progress.total);
</script>

{#if progress && !hidden && !allDone}
  <Card padding="md" title={`Getting started: ${progress.done} of ${progress.total} done`}>
    <ul class="steps">
      {#each progress.steps as s (s.id)}
        <li class:done={s.done}>
          <div class="what">
            <span><strong>{s.title}</strong>{#if s.optional}<span class="muted small"> (optional)</span>{/if}</span>
            <span class="muted small">{s.detail}</span>
          </div>
          {#if s.done}
            <Chip size="sm" tone="success" uppercase={false}>Done</Chip>
          {:else}
            <Button size="sm" label="Open" ariaLabel={`Open: ${s.title}`} onclick={() => navigate(s.href)} />
          {/if}
        </li>
      {/each}
    </ul>
    <div class="foot">
      <span class="muted small">Print guides for your staff in <a href="/guides" onclick={(e) => (e.preventDefault(), navigate("/guides"))}>Staff guides</a>.</span>
      <Button size="sm" label="Hide this list" onclick={hide} />
    </div>
  </Card>
{/if}

<style>
  .steps {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  li {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-4);
  }
  .what {
    display: flex;
    flex-direction: column;
  }
  li.done strong {
    color: var(--text-muted, inherit);
  }
  .foot {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-4);
  }
</style>
