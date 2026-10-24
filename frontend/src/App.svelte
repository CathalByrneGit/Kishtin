<script lang="ts">
  import { Chip, FlashBanner, IconButton, Notice } from "@kenn-io/kit-ui";
  import { ClipboardPlus, House, List, LogOut, Settings } from "@lucide/svelte";
  import { onMount } from "svelte";
  import { api } from "./lib/api";
  import { dublinDate, dublinTime, longDate } from "./lib/format";
  import { connect, disconnect, live, onChange } from "./lib/live.svelte";
  import { endSession, session, takePairingFromURL } from "./lib/session.svelte";
  import LogCheck, { type LogPreset } from "./screens/LogCheck.svelte";
  import Login from "./screens/Login.svelte";
  import Pair from "./screens/Pair.svelte";
  import Records from "./screens/Records.svelte";
  import Setup from "./screens/Setup.svelte";
  import Today from "./screens/Today.svelte";

  type Tab = "today" | "log" | "records" | "setup";
  const tabs: { id: Tab; label: string; icon: typeof House }[] = [
    { id: "today", label: "Today", icon: House },
    { id: "log", label: "Log check", icon: ClipboardPlus },
    { id: "records", label: "Records", icon: List },
    { id: "setup", label: "Setup", icon: Settings },
  ];

  let tab = $state<Tab>("today");
  let preset = $state<LogPreset | null>(null);
  let restaurant = $state("");
  let today = $state(dublinDate(new Date()));

  takePairingFromURL();

  const loggedIn = $derived(Boolean(session.deviceKey && session.token));

  async function loadName() {
    try {
      restaurant = (await api.system()).value.restaurant_name;
    } catch {
      // Keeps the last known name.
    }
  }

  $effect(() => {
    if (!session.deviceKey) {
      disconnect();
      return;
    }
    connect();
    return disconnect;
  });

  $effect(() => {
    if (loggedIn) void loadName();
  });

  onMount(() => {
    const off = onChange((kind) => {
      if ((kind === "settings" || kind === "ready") && session.token) void loadName();
    });
    const tick = setInterval(() => (today = dublinDate(new Date())), 30_000);

    // Keep kitchen tablets awake during service.
    let lock: WakeLockSentinel | null = null;
    const wake = async () => {
      try {
        if (document.visibilityState === "visible" && "wakeLock" in navigator) {
          lock = await navigator.wakeLock.request("screen");
        }
      } catch {
        // The browser refused (low battery or not allowed); the screen may sleep.
      }
    };
    void wake();
    document.addEventListener("visibilitychange", wake);
    return () => {
      off();
      clearInterval(tick);
      document.removeEventListener("visibilitychange", wake);
      void lock?.release();
    };
  });

  function logFrom(p: LogPreset) {
    preset = p;
    tab = "log";
  }

  async function switchStaff() {
    try {
      await api.logout();
    } catch {
      // The session ends on this device either way.
    }
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

<FlashBanner />

{#if !session.deviceKey}
  <Pair />
{:else if !session.token}
  <Login />
{:else}
  <div class="shell kitchen">
    <header class="top">
      <div class="title">
        <h1>{restaurant || "Kishtin"}</h1>
        <span class="muted">{longDate(today)}</span>
      </div>
      <Chip tone={connection.tone} dot uppercase={false}>{connection.label}</Chip>
      <div class="who">
        <span class="muted">{session.user?.name}</span>
        <IconButton ariaLabel="Switch staff" title="Switch staff" onclick={switchStaff}>
          <LogOut size={20} />
        </IconButton>
      </div>
    </header>

    {#if live.status === "offline"}
      <div class="notice">
        <Notice
          tone="warning"
          title="Not connected"
          message={live.lastSeen
            ? `Showing what was on screen at ${dublinTime(live.lastSeen)}. New records cannot be saved until the connection is back.`
            : "Cannot reach the Kishtin server. New records cannot be saved until the connection is back."}
        />
      </div>
    {/if}

    <main>
      {#if tab === "today"}
        <Today onlog={logFrom} />
      {:else if tab === "log"}
        <LogCheck {preset} onsaved={() => (preset = null)} />
      {:else if tab === "records"}
        <Records />
      {:else}
        <Setup />
      {/if}
    </main>

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
  </div>
{/if}

<style>
  .shell {
    max-width: 640px;
    margin: 0 auto;
    min-height: 100dvh;
    padding-bottom: calc(80px + env(safe-area-inset-bottom));
  }

  .top {
    position: sticky;
    top: 0;
    z-index: 10;
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-5) var(--space-6);
    background: var(--bg-surface);
    border-bottom: var(--border-width) solid var(--border-default);
  }

  .title {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .title h1 {
    font-size: var(--font-size-xl);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .notice {
    padding: var(--space-5) var(--space-6) 0;
  }

  main {
    padding: var(--space-5) var(--space-6);
  }

  .tabs {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    z-index: 10;
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    max-width: 640px;
    margin: 0 auto;
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

  @media (max-width: 640px) {
    .who span {
      display: none;
    }
  }
</style>
