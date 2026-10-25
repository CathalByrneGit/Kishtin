<script lang="ts">
  import { Button, Chip, IconButton, Modal, Notice, ThemeToggle, Toggle, getHighContrast, setHighContrast } from "@kenn-io/kit-ui";
  import { ChefHat, ClipboardPlus, House, Menu, ReceiptText } from "@lucide/svelte";
  import { onMount } from "svelte";
  import LogForm, { type LogPreset } from "../components/LogForm.svelte";
  import NewOrderForm from "../components/NewOrderForm.svelte";
  import PhoneToday from "../components/PhoneToday.svelte";
  import ScreenPicker from "../components/ScreenPicker.svelte";
  import VoiceBar from "../components/VoiceBar.svelte";
  import { api } from "../lib/api";
  import { dublinTime } from "../lib/format";
  import { canTakeOrders } from "../lib/kitchenfeed.svelte";
  import { live } from "../lib/live.svelte";
  import { screen } from "../lib/screen.svelte";
  import { endSession, session } from "../lib/session.svelte";
  import Pass from "../views/Pass.svelte";
  import Stations from "../views/Stations.svelte";

  interface Props {
    restaurant: string;
  }
  let { restaurant }: Props = $props();

  let now = $state(new Date());
  let menuOpen = $state(false);
  let highContrast = $state(getHighContrast());
  let stationName = $state("");

  type Tab = "today" | "log" | "order" | "pass";
  let tab = $state<Tab>("today");
  let preset = $state<LogPreset | null>(null);

  onMount(() => {
    const t = setInterval(() => (now = new Date()), 1000);
    return () => clearInterval(t);
  });

  $effect(() => {
    const id = screen.station;
    if (screen.kind !== "station" || !id) return;
    api
      .menu()
      .then((m) => (stationName = m.stations.find((s) => s.id === id)?.name ?? ""))
      .catch(() => {});
  });

  const tabs = $derived(
    [
      { id: "today" as Tab, label: "Today", icon: House },
      { id: "log" as Tab, label: "Log check", icon: ClipboardPlus },
      ...(canTakeOrders(session.user?.role) ? [{ id: "order" as Tab, label: "New order", icon: ReceiptText }] : []),
      { id: "pass" as Tab, label: "Pass", icon: ChefHat },
    ],
  );

  const heading = $derived(
    screen.kind === "pass" ? "Pass" : screen.kind === "station" ? stationName : (tabs.find((t) => t.id === tab)?.label ?? ""),
  );

  async function switchStaff() {
    try {
      await api.logout();
    } catch {
      // The session ends on this device either way.
    }
    menuOpen = false;
    endSession();
  }

  const connection = $derived(
    live.status === "synced" && !live.saveFailed
      ? { label: "Synced", tone: "success" as const }
      : live.status === "connecting"
        ? { label: "Connecting…", tone: "info" as const }
        : { label: "Not saving", tone: "danger" as const },
  );
</script>

<div class="shell kitchen" class:phone={screen.kind === "phone"}>
  <header class="top">
    <div class="title">
      <span class="brand-mark" aria-hidden="true">K</span>
      <div class="names">
        <strong class="heading">{heading}</strong>
        <span class="muted restaurant">{restaurant || "Kishtin"}</span>
      </div>
    </div>
    {#if screen.kind !== "phone"}
      <span class="clock num" aria-label="Time now">{dublinTime(now)}</span>
    {/if}
    <Chip tone={connection.tone} dot uppercase={false}>{connection.label}</Chip>
    <span class="who muted">{session.user?.name}</span>
    <IconButton ariaLabel="Screen menu" title="Switch staff, change screen" onclick={() => (menuOpen = true)}><Menu size={22} /></IconButton>
  </header>

  {#if live.status === "offline"}
    <div class="notice">
      <Notice
        tone="warning"
        title="Not connected"
        message={live.lastSeen
          ? `Showing what was on screen at ${dublinTime(live.lastSeen)}. Nothing can be saved until the connection is back.`
          : "Cannot reach the Kishtin server. Nothing can be saved until the connection is back."}
      />
    </div>
  {/if}

  <div class="voicebar">
    <VoiceBar big={screen.kind !== "phone"} touch keys wake />
  </div>

  <main>
    {#if screen.kind === "pass"}
      <Pass big />
    {:else if screen.kind === "station"}
      <Stations big station={screen.station} />
    {:else if tab === "today"}
      <PhoneToday
        onlog={(p) => {
          preset = p;
          tab = "log";
        }}
      />
    {:else if tab === "log"}
      <LogForm {preset} onsaved={() => (preset = null)} />
    {:else if tab === "order"}
      <NewOrderForm touch />
    {:else}
      <Pass compact />
    {/if}
  </main>

  {#if screen.kind === "phone"}
    <nav class="tabs" aria-label="Screens">
      {#each tabs as t (t.id)}
        <button
          type="button"
          class="tab kit-control-states"
          class:active={tab === t.id}
          aria-current={tab === t.id ? "page" : undefined}
          onclick={() => {
            if (t.id !== "log") preset = null;
            tab = t.id;
          }}
        >
          <t.icon size={24} />
          <span>{t.label}</span>
        </button>
      {/each}
    </nav>
  {/if}
</div>

{#if menuOpen}
  <Modal title="This screen" onclose={() => (menuOpen = false)} maxWidth="520px">
    <div class="stack menu">
      <div class="row between">
        <span>Logged in as <strong>{session.user?.name}</strong></span>
        <Button label="Switch staff" onclick={switchStaff} />
      </div>
      <ScreenPicker onchanged={() => (menuOpen = false)} />
      <div class="field">
        <span>Theme</span>
        <ThemeToggle variant="segmented" block />
      </div>
      <Toggle
        checked={highContrast}
        label="High contrast"
        onchange={(on) => {
          highContrast = on;
          setHighContrast(on);
        }}
      />
    </div>
  </Modal>
{/if}

<style>
  .shell {
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    background: var(--bg-primary);
  }
  .top {
    position: sticky;
    top: 0;
    z-index: 10;
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) var(--space-6);
    background: var(--bg-surface);
    border-bottom: var(--border-width) solid var(--border-default);
  }
  .title {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }
  .brand-mark {
    display: grid;
    place-items: center;
    flex: none;
    width: 40px;
    height: 40px;
    border-radius: var(--radius-md);
    background: var(--accent-blue);
    color: var(--text-on-accent, var(--bg-surface));
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-bold);
  }
  .names {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .heading {
    font-size: var(--font-size-xl);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .restaurant {
    font-size: var(--font-size-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .clock {
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-bold);
  }
  .notice {
    padding: var(--space-5) var(--space-6) 0;
  }
  .voicebar {
    padding: var(--space-4) var(--space-6) 0;
  }
  .phone .voicebar {
    padding: var(--space-4) var(--space-5) 0;
  }
  main {
    flex: 1;
    padding: var(--space-6);
  }
  .phone main {
    padding: var(--space-5) var(--space-5) calc(96px + env(safe-area-inset-bottom));
  }
  .tabs {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    z-index: 10;
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: 1fr;
    padding-bottom: env(safe-area-inset-bottom);
    background: var(--bg-surface);
    border-top: var(--border-width) solid var(--border-default);
  }
  .tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-1);
    min-height: 72px;
    border: 0;
    background: none;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-medium);
    cursor: pointer;
  }
  .tab.active {
    color: var(--accent-blue);
    box-shadow: inset 0 3px 0 var(--accent-blue);
  }
  .between {
    justify-content: space-between;
  }
  @media (max-width: 640px) {
    .top {
      padding: var(--space-3) var(--space-5);
      gap: var(--space-3);
    }
    .who {
      display: none;
    }
  }
</style>
