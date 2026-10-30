<script lang="ts">
  import { Button, Checkbox, Notice, SelectDropdown } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../lib/api";
  import { SCREEN_LABELS, screen, setScreen, type ScreenKind } from "../lib/screen.svelte";

  interface Props {
    onchanged?: () => void;
  }
  let { onchanged }: Props = $props();

  let kind = $state<ScreenKind>(screen.kind);
  let station = $state(screen.station ?? "");
  let names = $state(screen.names ?? false);
  let stations = $state<Schemas["Station"][]>([]);
  let error = $state("");

  onMount(async () => {
    try {
      stations = (await api.menu()).stations.filter((s) => s.active);
      if (!station && stations[0]) station = stations[0].id;
    } catch (err) {
      error = message(err);
    }
  });

  const HELP: Record<ScreenKind, string> = {
    desktop: "The full app with the sidebar, for the office computer.",
    pass: "Every open order in large type, for a wall-mounted tablet or TV at the pass.",
    station: "One station's queue in large type, for a tablet at that station.",
    phone: "Today's checks, logging, new orders and the pass, for a phone.",
    collection:
      "Takeaway numbers being prepared and ready to collect, in large type, for a screen customers can see. It needs no staff login and shows no items, notes or staff.",
  };

  const unchanged = $derived(
    kind === screen.kind && (kind !== "station" || station === screen.station) && (kind !== "collection" || names === (screen.names ?? false)),
  );
</script>

<div class="stack">
  <label class="field">
    <span>This device shows</span>
    <SelectDropdown
      value={kind}
      options={(Object.keys(SCREEN_LABELS) as ScreenKind[]).map((k) => ({ value: k, label: SCREEN_LABELS[k] }))}
      onchange={(v) => (kind = v as ScreenKind)}
      title="Screen"
    />
  </label>
  {#if kind === "station"}
    <label class="field">
      <span>Station</span>
      <SelectDropdown value={station} options={stations.map((s) => ({ value: s.id, label: s.name }))} onchange={(v) => (station = v)} title="Station" />
    </label>
    {#if stations.length === 0 && !error}
      <Notice tone="info" message="There are no stations yet. A manager adds them in Setup, Menu." />
    {/if}
  {/if}
  {#if kind === "collection"}
    <Checkbox bind:checked={names} label="Show order names as well as numbers (customers can see them)" />
  {/if}
  <p class="muted small">{HELP[kind]} The choice is kept on this device only.</p>
  {#if error}<Notice tone="error" message={error} />{/if}
  <div>
    <Button
      tone="info"
      surface="solid"
      label="Use this screen"
      disabled={unchanged || (kind === "station" && !station)}
      onclick={() => {
        setScreen(kind, kind === "station" ? station : null, names);
        onchanged?.();
      }}
    />
  </div>
</div>

<style>
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
