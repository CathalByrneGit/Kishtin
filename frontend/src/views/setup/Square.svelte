<script lang="ts">
  import {
    Button,
    Card,
    Chip,
    Modal,
    Notice,
    SegmentedControl,
    SelectDropdown,
    Spinner,
    Table,
    TableHeaderCell,
    TextInput,
    Toggle,
    showFlash,
  } from "@kenn-io/kit-ui";
  import { RefreshCw } from "@lucide/svelte";
  import { onMount } from "svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import ReviewChanges, { type Preview } from "../../components/ReviewChanges.svelte";
  import { api, message, type Schemas, type Setup } from "../../lib/api";
  import { dublinTime } from "../../lib/format";
  import { canRunPass } from "../../lib/kitchenfeed.svelte";
  import { onChange } from "../../lib/live.svelte";
  import { can } from "../../lib/can.svelte";
  import { session } from "../../lib/session.svelte";

  type P = Schemas["POSSettings"];

  let status = $state<Schemas["SquareStatus"] | null>(null);
  let menu = $state<Setup | null>(null);
  let error = $state("");

  // Connection.
  let connector = $state<Schemas["ConnectorStatus"] | null>(null);
  let locations = $state<Schemas["Location"][] | null>(null);
  let environment = $state<"production" | "sandbox">("production");
  let token = $state("");
  let connecting = $state(false);
  let askDisconnect = $state(false);

  // Import settings form.
  let enabled = $state(false);
  let location = $state("");
  let pollText = $state("5");
  let review = $state<{ preview: Preview; value: P } | null>(null);
  let saving = $state(false);

  // Catalog mapping.
  let askCatalog = $state(false);
  let catalog = $state<Schemas["CatalogRow"][] | null>(null);
  let loadingCatalog = $state(false);
  /** choice per catalog ID: "" not mapped, "ignore", or a dish ID / modifier name */
  let choice = $state<Record<string, string>>({});
  let syncing = $state(false);

  const canEdit = $derived(can("settings.manage"));

  function resetForm() {
    if (!status) return;
    const v = status.settings.value;
    enabled = v.enabled;
    location = v.location_id;
    pollText = String(v.poll_seconds);
  }

  async function load() {
    try {
      const [s, m, c] = await Promise.all([api.square(), api.menu(), api.connectors()]);
      const first = status === null;
      status = s;
      menu = m;
      connector = c.connectors.find((x) => x.id === "square") ?? null;
      if (first) resetForm();
      if (connector?.connected && canEdit && locations === null) void loadLocations();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  async function loadLocations() {
    try {
      locations = (await api.squareLocations()).locations;
    } catch {
      // The location can still be typed in.
      locations = [];
    }
  }

  async function connect() {
    connecting = true;
    try {
      const r = await api.connectSquare({ environment, token: token.trim() });
      token = "";
      locations = r.locations;
      if (!location && r.locations.length === 1) location = r.locations[0]?.id ?? "";
      showFlash("Connected to Square", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 10000 });
    } finally {
      connecting = false;
    }
  }

  async function disconnect() {
    askDisconnect = false;
    try {
      await api.disconnectSquare();
      locations = null;
      showFlash("Disconnected from Square. The saved token is deleted.", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    }
  }

  const SOURCES: Record<string, string> = {
    saved: "Connected here. The token is kept on the server, outside the database and its backups.",
    environment: "The token is set in the server's environment (KISHTIN_SQUARE_TOKEN). Change it there and restart kishtind.",
    "kishtin.toml": "The token is set in kishtin.toml. Change it there and restart kishtind.",
  };

  onMount(() => {
    void load();
    return onChange((k) => {
      if (k === "settings" || k === "order" || k === "menu") void load();
    });
  });

  const draft = $derived.by((): P | string => {
    if (!/^\d+$/.test(pollText.trim())) return "How often must be whole seconds.";
    return { enabled, location_id: location.trim(), poll_seconds: Number(pollText) };
  });
  const dirty = $derived(
    status !== null &&
      typeof draft !== "string" &&
      (draft.enabled !== status.settings.value.enabled ||
        draft.location_id !== status.settings.value.location_id ||
        draft.poll_seconds !== status.settings.value.poll_seconds),
  );

  async function reviewChanges() {
    if (typeof draft === "string") return;
    try {
      review = { preview: await api.previewSquare({ value: draft, reason: "" }), value: draft };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function save(reason: string) {
    if (!review) return;
    saving = true;
    try {
      await api.saveSquare({ value: review.value, reason });
      review = null;
      showFlash(enabled ? "Order import from Square is on" : "Order import from Square is off", { tone: "success" });
      status = null;
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }

  async function loadCatalog() {
    askCatalog = false;
    loadingCatalog = true;
    try {
      catalog = (await api.squareCatalog()).entries;
      choice = Object.fromEntries(
        catalog.map((r) => [
          r.entry.id,
          r.mapping ? (r.mapping.ignore ? "ignore" : (r.mapping.dish_id ?? r.mapping.modifier ?? "")) : "",
        ]),
      );
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      loadingCatalog = false;
    }
  }

  function useSuggestions() {
    for (const r of catalog ?? []) {
      if (!choice[r.entry.id] && r.suggested) choice[r.entry.id] = r.suggested;
    }
  }

  function saved(r: Schemas["CatalogRow"]): string {
    return r.mapping ? (r.mapping.ignore ? "ignore" : (r.mapping.dish_id ?? r.mapping.modifier ?? "")) : "";
  }
  const changed = $derived((catalog ?? []).filter((r) => (choice[r.entry.id] ?? "") !== saved(r) && choice[r.entry.id] !== ""));

  async function saveMappings() {
    saving = true;
    try {
      await api.saveSquareMappings(
        changed.map((r) => {
          const c = choice[r.entry.id] ?? "";
          const base = { external_id: r.entry.id, kind: r.entry.kind as "item" | "modifier", name: r.label };
          if (c === "ignore") return { ...base, ignore: true };
          return r.entry.kind === "item" ? { ...base, dish_id: c } : { ...base, modifier: c };
        }),
      );
      showFlash(`Saved ${changed.length} mapping${changed.length === 1 ? "" : "s"}`, { tone: "success" });
      await loadCatalog();
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }

  async function syncNow() {
    syncing = true;
    try {
      const r = await api.squareSync();
      showFlash(`${r.seen} changed orders: ${r.created} entered, ${r.updated} added to`, { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      syncing = false;
    }
  }

  const dishOptions = $derived([
    { value: "", label: "Not mapped" },
    { value: "ignore", label: "Not for the kitchen" },
    ...(menu?.dishes ?? []).filter((d) => d.active).map((d) => ({ value: d.id, label: d.name })),
  ]);
  const modifierOptions = $derived([
    { value: "", label: "Not mapped" },
    { value: "ignore", label: "Not for the kitchen" },
    ...[...new Set((menu?.dishes ?? []).flatMap((d) => d.modifiers.map((m) => m.name)))].sort().map((n) => ({ value: n, label: n })),
  ]);
  const dishName = (id: string) => menu?.dishes.find((d) => d.id === id)?.name ?? id;
  const needsChecking = $derived((status?.recent ?? []).filter((o) => o.unmapped.length > 0 || o.notices.length > 0).length);
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Square</h1>
      <p>
        Orders taken on the Square till come into the kitchen on their own. Kishtin only reads from Square: it never
        changes orders there, and never sees card details. Payments stay in Square.
      </p>
    </div>
    {#if status?.settings.value.enabled && canRunPass()}
      <Button label={syncing ? "Asking Square…" : "Sync now"} disabled={syncing} onclick={syncNow}><RefreshCw size={16} /></Button>
    {/if}
  </header>

  {#if error}<Notice tone="error" message={error} />{/if}
  {#if !status}
    {#if !error}<Spinner />{/if}
  {:else}
    <Card padding="md" title="1. Connect your Square account">
      {#if connector?.connected}
        <div class="row between">
          <div class="stack tight">
            <div class="row">
              <Chip uppercase={false} dot tone="success">Connected</Chip>
              {#if connector.environment}<Chip uppercase={false}>{connector.environment === "sandbox" ? "Sandbox" : "Production"}</Chip>{/if}
            </div>
            <p class="muted small">{SOURCES[connector.source ?? ""] ?? ""}</p>
          </div>
          {#if canEdit && connector.source === "saved"}
            <Button label="Disconnect" onclick={() => (askDisconnect = true)} />
          {/if}
        </div>
      {:else}
        <ManagerGate what="Connecting Square" action="settings.manage">
          <div class="stack">
            <p>
              In the Square Developer Dashboard, create an application and copy an access token for the restaurant's
              account that can read orders and items (ORDERS_READ, ITEMS_READ). Paste it here.
            </p>
            <SegmentedControl
              ariaLabel="Square account"
              value={environment}
              onchange={(v) => (environment = v as "production" | "sandbox")}
              options={[
                { value: "production", label: "Production (the real till)" },
                { value: "sandbox", label: "Sandbox (for testing)" },
              ]}
            />
            <label class="field">
              <span>Access token</span>
              <TextInput bind:value={token} type="password" autocomplete="off" placeholder="EAAA…" block />
            </label>
            <p class="muted small">
              Check and connect sends the token to Square to check it, and asks for your locations. If Square accepts it,
              Kishtin keeps it in a file only the server can read, outside the database, so it is never in a backup or
              export. Nothing is imported until you turn import on below.
            </p>
            <div>
              <Button tone="info" surface="solid" label={connecting ? "Checking with Square…" : "Check and connect"} disabled={connecting || !token.trim()} onclick={connect} />
            </div>
          </div>
        </ManagerGate>
      {/if}
    </Card>

    <section class="stack">
      <div class="row between">
        <h2 class="section-title">2. Map till items to dishes</h2>
        {#if canEdit}
          <div class="row">
            {#if catalog}
              <Button label="Use suggestions" onclick={useSuggestions} />
              <Button tone="info" surface="solid" label={saving ? "Saving…" : changed.length ? `Save ${changed.length} change${changed.length === 1 ? "" : "s"}` : "Save changes"} disabled={saving || changed.length === 0} onclick={saveMappings} />
            {/if}
            <Button label={loadingCatalog ? "Loading…" : catalog ? "Load again" : "Load from Square"} disabled={loadingCatalog || !status.connected} onclick={() => (askCatalog = true)} />
          </div>
        {/if}
      </div>
      <p class="muted">
        Each item on the till needs the dish it makes, so the kitchen knows its station and times. Drinks and anything else
        not cooked are "Not for the kitchen". Items not mapped are listed on the order, not cooked. Saving a mapping adds
        the item to open orders that were waiting for it.
      </p>
      {#if !canEdit}
        <p class="muted small">A manager logged in with their password loads the catalog and maps till items.</p>
      {/if}
      {#if catalog}
        <Table ariaLabel="Till items and their kitchen mapping" unsorted>
          {#snippet header()}
            <TableHeaderCell label="On the till" />
            <TableHeaderCell label="Kind" />
            <TableHeaderCell label="In the kitchen" />
            <TableHeaderCell label="Suggested" />
          {/snippet}
          {#each catalog as r (r.entry.id)}
            <tr>
              <td>{r.label}</td>
              <td>{r.entry.kind === "item" ? "Item" : "Modifier"}</td>
              <td>
                <SelectDropdown
                  value={choice[r.entry.id] ?? ""}
                  options={r.entry.kind === "item" ? dishOptions : modifierOptions}
                  onchange={(v) => (choice[r.entry.id] = v)}
                  title={`Kitchen mapping for ${r.label}`}
                />
              </td>
              <td class="muted">{r.suggested ? (r.entry.kind === "item" ? dishName(r.suggested) : r.suggested) : ""}</td>
            </tr>
          {/each}
        </Table>
      {:else if status.mappings.length > 0}
        <p class="muted">{status.mappings.length} till items and modifiers are mapped. Load from Square to see and change them.</p>
      {/if}
    </section>

    <div class="grid">
      <Card padding="md" title="Import status">
        <dl>
          <dt>Square account</dt>
          <dd>
            <Chip uppercase={false} dot tone={status.connected ? "success" : "warning"}>{status.connected ? "Connected" : "Not connected"}</Chip>
          </dd>
          <dt>Order import</dt>
          <dd>
            <Chip uppercase={false} dot tone={status.settings.value.enabled ? "success" : "neutral"}>{status.settings.value.enabled ? "On" : "Off"}</Chip>
          </dd>
          {#if status.settings.value.enabled}
            <dt>Location</dt>
            <dd class="num">{status.settings.value.location_id}</dd>
            <dt>Asks every</dt>
            <dd>{status.settings.value.poll_seconds} seconds</dd>
          {/if}
          {#if status.state?.last_ok_at}
            <dt>Last worked</dt>
            <dd class="num">{dublinTime(status.state.last_ok_at)}</dd>
          {/if}
        </dl>
        {#if status.state?.last_error}
          <Notice tone="warning" title="Square did not answer" message={`${status.state.last_error}. Orders can still be entered by hand; import carries on from the same point when Square answers.`} />
        {/if}
      </Card>

      <Card padding="md" title="3. Turn import on or off">
        <ManagerGate what="Square import" action="settings.manage" viewable>
          <div class="stack">
            <Toggle checked={enabled} label="Import orders from Square" disabled={!canEdit} onchange={(on) => (enabled = on)} />
            <label class="field">
              <span>Location</span>
              {#if locations && locations.length > 0}
                <SelectDropdown
                  value={location}
                  options={[
                    { value: "", label: "Choose a location" },
                    ...locations.map((l) => ({ value: l.id, label: `${l.name}${l.active ? "" : " (closed)"}` })),
                    ...(location && !locations.some((l) => l.id === location) ? [{ value: location, label: location }] : []),
                  ]}
                  onchange={(v) => (location = v)}
                  disabled={!canEdit}
                  title="Square location"
                />
              {:else}
                <TextInput bind:value={location} placeholder="Location ID from the Square Dashboard, such as L1234ABCD" disabled={!canEdit} block />
              {/if}
            </label>
            <label class="field">
              <span>Ask for new orders every (seconds)</span>
              <TextInput bind:value={pollText} disabled={!canEdit} block />
            </label>
            {#if typeof draft === "string"}<Notice tone="error" message={draft} />{/if}
            {#if canEdit}
              <div class="row">
                <Button label="Undo changes" disabled={!dirty} onclick={resetForm} />
                <Button tone="info" surface="solid" label="Review changes" disabled={!dirty} onclick={reviewChanges} />
              </div>
              <p class="muted small">Review changes shows exactly what Kishtin will send to Square before anything is saved.</p>
            {/if}
          </div>
        </ManagerGate>
      </Card>
    </div>

    <section class="stack">
      <h2 class="section-title">Recent till orders{needsChecking ? ` · ${needsChecking} need checking` : ""}</h2>
      {#if status.recent.length === 0}
        <p class="muted">No orders from Square yet.</p>
      {:else}
        <Table ariaLabel="Recent till orders" unsorted>
          {#snippet header()}
            <TableHeaderCell label="Seen" />
            <TableHeaderCell label="Order" />
            <TableHeaderCell label="On the till" />
            <TableHeaderCell label="Needs checking" />
          {/snippet}
          {#each status.recent as o (o.external_id)}
            <tr>
              <td class="num">{dublinTime(o.last_seen)}</td>
              <td>{o.label}{o.order_id ? "" : " (nothing for the kitchen)"}</td>
              <td>
                <Chip size="sm" uppercase={false} tone={o.state === "canceled" ? "danger" : o.state === "completed" ? "muted" : "info"}>
                  {o.state === "canceled" ? "Cancelled" : o.state === "completed" ? "Completed" : "Open"}
                </Chip>
                <span class="muted small">v{o.version}</span>
              </td>
              <td>
                {#if o.unmapped.length > 0}<div><strong>Not mapped:</strong> {o.unmapped.join(", ")}</div>{/if}
                {#each o.notices as n (n)}<div>{n}</div>{/each}
              </td>
            </tr>
          {/each}
        </Table>
      {/if}
    </section>
  {/if}
</div>

{#if askDisconnect}
  <Modal title="Disconnect Square" onclose={() => (askDisconnect = false)} maxWidth="480px">
    <p>The saved token is deleted from this server. Mappings and orders already imported stay. You can connect again at any time.</p>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (askDisconnect = false)} />
        <Button tone="danger" surface="solid" label="Disconnect" onclick={disconnect} />
      </div>
    {/snippet}
  </Modal>
{/if}

{#if askCatalog}
  <Modal title="Load the catalog from Square" onclose={() => (askCatalog = false)} maxWidth="520px">
    <div class="stack">
      <p>Kishtin will send Square the access token, to ask for the items and modifiers on the till.</p>
      <p class="muted">Nothing about the kitchen, staff or HACCP records is sent. Nothing is saved until you save mappings.</p>
    </div>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (askCatalog = false)} />
        <Button tone="info" surface="solid" label="Load from Square" onclick={loadCatalog} />
      </div>
    {/snippet}
  </Modal>
{/if}

{#if review}
  <ReviewChanges title="Square import" preview={review.preview} busy={saving} onsave={(reason) => save(reason)} onclose={() => (review = null)} />
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr));
    gap: var(--space-5);
  }
  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: var(--space-3) var(--space-5);
    align-items: center;
    margin: 0 0 var(--space-4);
  }
  dt {
    color: var(--text-secondary);
  }
  dd {
    margin: 0;
  }
  .between {
    justify-content: space-between;
  }
  .tight {
    gap: var(--space-2);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
