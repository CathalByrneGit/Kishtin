<script lang="ts">
  import { Button, Card, Notice, TextInput } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../lib/api";
  import { pairDevice, unpairDevice } from "../lib/session.svelte";
  import FirstRun from "./FirstRun.svelte";

  let key = $state("");
  let busy = $state(false);
  let error = $state("");
  // A new install with no staff shows the setup screen instead.
  let setup = $state<Schemas["SetupStatusOutBody"] | null>(null);

  onMount(() => {
    api
      .setupStatus()
      .then((s) => (setup = s))
      .catch(() => (setup = null));
  });

  async function pair(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    busy = true;
    pairDevice(key);
    try {
      await api.staff();
    } catch (err) {
      unpairDevice();
      error = message(err);
    } finally {
      busy = false;
    }
  }
</script>

{#if setup?.needed}
  <FirstRun status={setup} />
{:else}
  <div class="page kitchen">
    <h1>Kishtin</h1>
    <p class="muted">Kishtin, from <i>cistin</i>, Irish for kitchen.</p>

    <Card title="Set up this device" padding="md">
      <form class="stack" onsubmit={pair}>
        <p>
          This tablet or phone needs a device key before staff can log in. A manager makes one in Setup › Staff and access › Device
          keys, then opens its pairing link on this device, or types the key here.
        </p>
        <label class="field">
          <span>Device key</span>
          <TextInput bind:value={key} placeholder="kishtin_key_…" autocomplete="off" required />
        </label>
        {#if error}
          <Notice tone="error" title="Not paired" message={error} />
        {/if}
        <Button type="submit" tone="info" surface="solid" label={busy ? "Checking…" : "Pair this device"} disabled={busy || !key.trim()} />
      </form>
    </Card>
  </div>
{/if}

<style>
  .page {
    max-width: 520px;
    margin: 0 auto;
    padding: var(--space-8) var(--space-6);
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  h1 {
    font-size: var(--font-size-2xl);
  }
  p {
    margin: 0;
    font-size: var(--font-size-lg);
    line-height: 1.5;
  }
</style>
