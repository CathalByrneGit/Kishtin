<script lang="ts">
  import {
    AppSidebar,
    Button,
    Chip,
    CommandPalette,
    DetailDrawer,
    FlashBanner,
    IconButton,
    KbdBadge,
    Notice,
    ThemeToggle,
    TopBar,
    appShortcuts,
    formatShortcutKeys,
    initShortcuts,
    type PaletteCommand,
    type SidebarGroup,
  } from "@kenn-io/kit-ui";
  import {
    Archive,
    ChefHat,
    ClipboardPlus,
    CookingPot,
    BookMarked,
    ClipboardCheck,
    BookOpen,
    Download,
    House,
    KeyRound,
    List,
    ListChecks,
    LogOut,
    Mic,
    ReceiptText,
    Timer,
    UtensilsCrossed,
    Search,
    Settings,
    ShieldCheck,
    SlidersHorizontal,
    Plug,
    Users,
  } from "@lucide/svelte";
  import { onMount } from "svelte";
  import LogForm from "./components/LogForm.svelte";
  import { api } from "./lib/api";
  import { dublinDate, dublinTime, longDate } from "./lib/format";
  import { connect, disconnect, live, onChange } from "./lib/live.svelte";
  import { closeLog, logDrawer, openLog } from "./lib/logdrawer.svelte";
  import { initRouter, navigate, route } from "./lib/router.svelte";
  import { screen } from "./lib/screen.svelte";
  import { endSession, session, takePairingFromURL } from "./lib/session.svelte";
  import KitchenShell from "./screens/KitchenShell.svelte";
  import Login from "./screens/Login.svelte";
  import Pair from "./screens/Pair.svelte";
  import Exports from "./views/Exports.svelte";
  import Books from "./views/Books.svelte";
  import Inspection from "./views/Inspection.svelte";
  import OrdersView from "./views/Orders.svelte";
  import Pass from "./views/Pass.svelte";
  import Stations from "./views/Stations.svelte";
  import MenuSetup from "./views/setup/Menu.svelte";
  import SchedulerSetup from "./views/setup/Scheduler.svelte";
  import ConnectionsSetup from "./views/setup/Connections.svelte";
  import SquareSetup from "./views/setup/Square.svelte";
  import Records from "./views/Records.svelte";
  import Backups from "./views/setup/Backups.svelte";
  import Keys from "./views/setup/Keys.svelte";
  import Limits from "./views/setup/Limits.svelte";
  import Lists from "./views/setup/Lists.svelte";
  import RecordBooksSetup from "./views/setup/RecordBooks.svelte";
  import SettingsView from "./views/setup/Settings.svelte";
  import Staff from "./views/setup/Staff.svelte";
  import Today from "./views/Today.svelte";
  import Verify from "./views/Verify.svelte";
  import VoiceView from "./views/Voice.svelte";

  interface Page {
    path: string;
    label: string;
    section: string;
    view: string | null;
    group: string;
    /** sub-pages are reached from their parent page, not the sidebar */
    sub?: boolean;
  }

  const pages: Page[] = [
    { path: "/pass", label: "Pass", section: "pass", view: null, group: "Service" },
    { path: "/stations", label: "Stations", section: "stations", view: null, group: "Service" },
    { path: "/orders", label: "Orders", section: "orders", view: null, group: "Service" },
    { path: "/voice", label: "Voice", section: "voice", view: null, group: "Service" },
    { path: "/today", label: "Today", section: "today", view: null, group: "HACCP" },
    { path: "/records", label: "Records", section: "records", view: null, group: "HACCP" },
    { path: "/books", label: "Record books", section: "books", view: null, group: "HACCP" },
    { path: "/inspection", label: "Inspection", section: "inspection", view: null, group: "HACCP" },
    { path: "/exports", label: "Exports", section: "exports", view: null, group: "HACCP" },
    { path: "/verify", label: "Verify", section: "verify", view: null, group: "HACCP" },
    { path: "/setup/menu", label: "Menu", section: "setup", view: "menu", group: "Setup" },
    { path: "/setup/scheduler", label: "Scheduler", section: "setup", view: "scheduler", group: "Setup" },
    { path: "/setup/connections", label: "Connections", section: "setup", view: "connections", group: "Setup" },
    { path: "/setup/connections/square", label: "Square", section: "setup", view: "connections", group: "Setup / Connections", sub: true },
    { path: "/setup/lists", label: "Fridges and tasks", section: "setup", view: "lists", group: "Setup" },
    { path: "/setup/limits", label: "Limits and rules", section: "setup", view: "limits", group: "Setup" },
    { path: "/setup/records", label: "HACCP records", section: "setup", view: "records", group: "Setup" },
    { path: "/setup/staff", label: "Staff", section: "setup", view: "staff", group: "Setup" },
    { path: "/setup/keys", label: "Device keys", section: "setup", view: "keys", group: "Setup" },
    { path: "/setup/backups", label: "Backups", section: "setup", view: "backups", group: "Setup" },
    { path: "/setup/settings", label: "Settings", section: "settings", view: null, group: "This device" },
  ];

  const setupIcons: Record<string, typeof House> = {
    menu: UtensilsCrossed,
    scheduler: Timer,
    connections: Plug,
    lists: ListChecks,
    limits: SlidersHorizontal,
    records: BookMarked,
    staff: Users,
    keys: KeyRound,
    backups: Archive,
  };

  const groups: SidebarGroup[] = [
    {
      label: "Service",
      sections: [
        { id: "pass", label: "Pass", href: "/pass", icon: ChefHat, viewRuns: [] },
        { id: "stations", label: "Stations", href: "/stations", icon: CookingPot, viewRuns: [] },
        { id: "orders", label: "Orders", href: "/orders", icon: ReceiptText, viewRuns: [] },
        { id: "voice", label: "Voice", href: "/voice", icon: Mic, viewRuns: [] },
      ],
    },
    {
      label: "HACCP",
      sections: [
        { id: "today", label: "Today", href: "/today", icon: House, viewRuns: [] },
        { id: "records", label: "Records", href: "/records", icon: List, viewRuns: [] },
        { id: "books", label: "Record books", href: "/books", icon: BookOpen, viewRuns: [] },
        { id: "inspection", label: "Inspection", href: "/inspection", icon: ClipboardCheck, viewRuns: [] },
        { id: "exports", label: "Exports", href: "/exports", icon: Download, viewRuns: [] },
        { id: "verify", label: "Verify", href: "/verify", icon: ShieldCheck, viewRuns: [] },
      ],
    },
    {
      label: "Setup",
      sections: pages
        .filter((p) => p.section === "setup" && !p.sub)
        .map((p) => ({ id: `setup-${p.view}`, label: p.label, href: p.path, icon: setupIcons[p.view ?? ""] ?? Settings, viewRuns: [] })),
    },
    {
      label: null,
      end: true,
      sections: [{ id: "settings", label: "Settings", href: "/setup/settings", icon: Settings, viewRuns: [] }],
    },
  ];

  let restaurant = $state("");
  let today = $state(dublinDate(new Date()));
  let paletteOpen = $state(false);

  takePairingFromURL();

  const loggedIn = $derived(Boolean(session.deviceKey && session.token));
  const current = $derived(pages.find((p) => p.path === route.path) ?? null);
  const location = $derived(
    current
      ? { section: current.section === "setup" ? `setup-${current.view}` : current.section, view: null }
      : { section: null, view: null },
  );

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

  $effect(() => {
    if (!current) navigate("/pass", true);
  });

  $effect(() => {
    document.title = current ? `${current.label} · Kishtin` : "Kishtin";
  });

  onMount(() => {
    const offRouter = initRouter();
    const offKeys = initShortcuts();
    const offPalette = appShortcuts.register("mod+k", () => (paletteOpen = true), { description: "Open command palette" });
    const offLog = appShortcuts.register("mod+l", () => openLog(), { description: "Log a check" });
    const off = onChange((kind) => {
      if ((kind === "settings" || kind === "ready") && session.token) void loadName();
    });
    const tick = setInterval(() => (today = dublinDate(new Date())), 30_000);

    // Keep a wall-mounted screen awake during service.
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
      offRouter();
      offPalette();
      offLog();
      offKeys();
      off();
      clearInterval(tick);
      document.removeEventListener("visibilitychange", wake);
      void lock?.release();
    };
  });

  async function switchStaff() {
    try {
      await api.logout();
    } catch {
      // The session ends on this device either way.
    }
    endSession();
  }

  const commands = $derived<PaletteCommand[]>([
    { id: "new-order", label: "New order", section: "Actions", keywords: "table takeaway ticket" },
    { id: "log", label: "Log a check", section: "Actions", keywords: "record temperature fridge delivery", combo: "mod+l" },
    { id: "log-fridge", label: "Log a fridge or freezer temperature", section: "Actions", keywords: "temp" },
    { id: "log-cleaning", label: "Log a cleaning task", section: "Actions", keywords: "clean" },
    { id: "switch", label: "Switch staff", section: "Actions", keywords: "log out logout" },
    ...pages.map((p) => ({ id: `go:${p.path}`, label: `Go to ${p.label}`, section: p.group, keywords: p.path })),
  ]);

  function run(c: PaletteCommand) {
    if (c.id.startsWith("go:")) navigate(c.id.slice(3));
    else if (c.id === "new-order") navigate("/orders");
    else if (c.id === "log") openLog();
    else if (c.id === "log-fridge") openLog({ type: "fridge" });
    else if (c.id === "log-cleaning") openLog({ type: "cleaning" });
    else if (c.id === "switch") void switchStaff();
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

{#snippet brandName(rail: boolean)}
  <a class="brand" href="/pass" aria-label="Kishtin home">
    {#if rail}
      <span class="brand-mark">K</span>
    {:else}
      <span class="brand-mark">K</span>
      <span class="brand-text">
        <strong>{restaurant || "Kishtin"}</strong>
        <span class="muted">{longDate(today)}</span>
      </span>
    {/if}
  </a>
{/snippet}

{#if !session.deviceKey}
  <Pair />
{:else if !session.token}
  <Login />
{:else if screen.kind !== "desktop"}
  <KitchenShell {restaurant} />
{:else}
  <div class="app">
    <AppSidebar {groups} {location} page={route.path + route.search} storageKey="kishtin-sidebar">
      {#snippet brand({ rail })}
        {@render brandName(rail)}
      {/snippet}
      {#snippet narrowBar()}
        {@render brandName(false)}
        <span class="grow"></span>
        <Button tone="info" surface="solid" size="sm" label="Log check" onclick={() => openLog()} />
      {/snippet}
      {#snippet footer({ rail })}
        <div class="side-footer" class:rail>
          {#if !rail}
            <div class="who">
              <strong>{session.user?.name}</strong>
              <span class="muted">{session.method === "password" ? "Manager password" : "PIN"} login</span>
            </div>
          {/if}
          <ThemeToggle size={rail ? "md" : "sm"} />
          <IconButton ariaLabel="Switch staff" title="Switch staff" onclick={switchStaff}>
            <LogOut size={16} />
          </IconButton>
        </div>
      {/snippet}
    </AppSidebar>

    <div class="column">
      <div class="top-bar">
        <TopBar>
          {#snippet left()}
            <span class="where">{current && current.group !== "This device" ? `${current.group} / ` : ""}<strong>{current?.label ?? ""}</strong></span>
          {/snippet}
          {#snippet search()}
            <button type="button" class="search-trigger kit-control-states" onclick={() => (paletteOpen = true)}>
              <Search size={16} />
              <span>Search or jump to…</span>
              <KbdBadge keys={formatShortcutKeys("mod+k")} />
            </button>
          {/snippet}
          {#snippet right()}
            <Chip tone={connection.tone} dot uppercase={false}>{connection.label}</Chip>
            <Button label="New order" onclick={() => navigate("/orders")}>
              <ReceiptText size={16} />
            </Button>
            <Button tone="info" surface="solid" label="Log check" onclick={() => openLog()}>
              <ClipboardPlus size={16} />
            </Button>
          {/snippet}
        </TopBar>
      </div>

      {#if live.status === "offline"}
        <div class="offline">
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
        {#if route.path === "/pass"}
          <Pass />
        {:else if route.path === "/stations"}
          <Stations />
        {:else if route.path === "/orders"}
          <OrdersView />
        {:else if route.path === "/voice"}
          <VoiceView />
        {:else if route.path === "/setup/menu"}
          <MenuSetup />
        {:else if route.path === "/setup/scheduler"}
          <SchedulerSetup />
        {:else if route.path === "/setup/connections"}
          <ConnectionsSetup />
        {:else if route.path === "/setup/connections/square"}
          <SquareSetup />
        {:else if route.path === "/today"}
          <Today />
        {:else if route.path === "/records"}
          <Records />
        {:else if route.path === "/inspection"}
          <Inspection />
        {:else if route.path === "/books"}
          <Books />
        {:else if route.path === "/exports"}
          <Exports />
        {:else if route.path === "/verify"}
          <Verify />
        {:else if route.path === "/setup/lists"}
          <Lists />
        {:else if route.path === "/setup/records"}
          <RecordBooksSetup />
        {:else if route.path === "/setup/limits"}
          <Limits />
        {:else if route.path === "/setup/staff"}
          <Staff />
        {:else if route.path === "/setup/keys"}
          <Keys />
        {:else if route.path === "/setup/backups"}
          <Backups />
        {:else if route.path === "/setup/settings"}
          <SettingsView />
        {/if}
      </main>
    </div>
  </div>

  <CommandPalette bind:open={paletteOpen} {commands} onrun={run} placeholder="Search pages and actions" />

  {#if logDrawer.open}
    <DetailDrawer
      title={logDrawer.preset?.corrects ? "Correct a record" : "Log a check"}
      onclose={closeLog}
      width="min(620px, 100vw)"
    >
      <LogForm preset={logDrawer.preset} onsaved={closeLog} />
    </DetailDrawer>
  {/if}
{/if}

<style>
  .app {
    display: flex;
    min-height: 100vh;
    background: var(--bg-primary);
  }

  .column {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .top-bar {
    position: sticky;
    top: 0;
    z-index: 10;
  }

  main {
    flex: 1;
    padding: var(--space-7);
    width: 100%;
    max-width: 1600px;
  }

  .offline {
    padding: var(--space-5) var(--space-7) 0;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-width: 0;
    color: var(--text-primary);
    text-decoration: none;
  }

  .brand-mark {
    display: grid;
    place-items: center;
    flex: none;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-md);
    background: var(--accent-blue);
    color: var(--text-on-accent, var(--bg-surface));
    font-weight: var(--font-weight-bold);
  }

  .brand-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
    font-size: var(--font-size-sm);
  }

  .brand-text strong {
    font-size: var(--font-size-md);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .grow {
    flex: 1;
  }

  .side-footer {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
  }

  .side-footer.rail {
    flex-direction: column;
  }

  .who {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    font-size: var(--font-size-sm);
  }

  .where {
    color: var(--text-secondary);
  }

  .where strong {
    color: var(--text-primary);
  }

  .search-trigger {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 320px;
    height: var(--kit-control-height, 32px);
    padding: 0 var(--space-4);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--bg-inset);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--font-size-sm);
    cursor: pointer;
  }

  .search-trigger span {
    flex: 1;
    text-align: left;
  }

  @media (max-width: 900px) {
    .app {
      flex-direction: column;
    }

    .top-bar {
      display: none;
    }

    main {
      padding: var(--space-5);
    }

    .offline {
      padding: var(--space-5) var(--space-5) 0;
    }
  }
</style>
