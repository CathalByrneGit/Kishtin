<script lang="ts">
  import { Button, Card, Chip, CopyButton, Modal, Notice, SegmentedControl, SelectDropdown, Spinner, Table, TableHeaderCell, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { Plus, QrCode as QrCodeIcon } from "@lucide/svelte";
  import ConnectTablet from "../../components/ConnectTablet.svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { dublinDate, dublinTime, shortDate } from "../../lib/format";
  import { can } from "../../lib/can.svelte";

  type Key = Schemas["APIKey"];

  let keys = $state<Key[] | null>(null);
  let users = $state<Schemas["User"][]>([]);
  let thisKey = $state("");
  let error = $state("");
  let busy = $state(false);

  let creating = $state(false);
  let kind = $state<"device" | "user">("device");
  let name = $state("");
  let userId = $state("");
  let made = $state<Schemas["NewKeyResult"] | null>(null);
  let revoking = $state<Key | null>(null);
  let connecting = $state(false);

  async function load() {
    try {
      const [k, u, me] = await Promise.all([api.keys(), api.users(), api.me()]);
      keys = k.keys;
      thisKey = me.key.id;
      users = u.users;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  $effect(() => {
    if (can("keys.manage")) void load();
  });

  const userName = (id: string | undefined) => users.find((u) => u.id === id)?.name ?? id ?? "";
  const rows = $derived([...(keys ?? [])].sort((a, b) => Number(Boolean(a.revoked_at)) - Number(Boolean(b.revoked_at)) || b.created_at.localeCompare(a.created_at)));
  const when = (t: string) => `${shortDate(dublinDate(new Date(t)))} ${new Date(t).getFullYear()}, ${dublinTime(t)}`;
  const pairLink = $derived(made && made.key.kind === "device" ? `${location.origin}/#pair=${encodeURIComponent(made.secret)}` : "");

  async function create() {
    busy = true;
    try {
      const body: Schemas["NewKeyInBody"] = { kind, name: name.trim() };
      if (kind === "user") body.user_id = userId;
      made = await api.createKey(body);
      creating = false;
      name = "";
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  async function revoke() {
    if (!revoking) return;
    busy = true;
    try {
      await api.revokeKey(revoking.id);
      showFlash(`${revoking.name} revoked`, { tone: "success" });
      revoking = null;
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Device keys</h1>
      <p>
        A device key lets a kitchen tablet, phone or computer reach Kishtin; staff then log in on it with their PIN. <strong>Connect a
        tablet</strong> shows a QR code that sets a tablet up and makes its key. A staff key lets the <code>kishtin</code> CLI or a script act as
        one person.
      </p>
    </div>
    {#if can("keys.manage")}
      <div class="head-actions">
        <Button label="New key" onclick={() => (creating = true)}><Plus size={16} /></Button>
        <Button tone="info" surface="solid" label="Connect a tablet" onclick={() => (connecting = true)}><QrCodeIcon size={16} /></Button>
      </div>
    {/if}
  </header>

  <ManagerGate what="Device keys" action="keys.manage">
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if keys === null}
      {#if !error}<Spinner />{/if}
    {:else}
      <Card padding="none">
        <Table unsorted ariaLabel="Keys">
          {#snippet header()}
            <TableHeaderCell label="Name" />
            <TableHeaderCell label="Kind" />
            <TableHeaderCell label="Created" />
            <TableHeaderCell label="Status" />
            <TableHeaderCell><span class="kit-sr-only">Revoke</span></TableHeaderCell>
          {/snippet}
          {#each rows as k (k.id)}
            <tr class:revoked={Boolean(k.revoked_at)}>
              <td><strong>{k.name}</strong>{#if k.id === thisKey}&nbsp;<span class="muted">(this device)</span>{/if}</td>
              <td>{k.kind === "device" ? "Device" : `Staff key for ${userName(k.user_id)}`}</td>
              <td class="small nowrap">{when(k.created_at)} by {userName(k.created_by)}</td>
              <td>
                {#if k.revoked_at}
                  <Chip tone="muted" uppercase={false}>Revoked {shortDate(dublinDate(new Date(k.revoked_at)))}</Chip>
                {:else}
                  <Chip tone="success" uppercase={false}>Active</Chip>
                {/if}
              </td>
              <td class="end">
                {#if !k.revoked_at}<Button size="sm" tone="danger" label="Revoke" disabled={busy} onclick={() => (revoking = k)} />{/if}
              </td>
            </tr>
          {/each}
        </Table>
      </Card>
    {/if}
  </ManagerGate>
</div>

{#if creating}
  <Modal title="New key" onclose={() => (creating = false)} maxWidth="520px">
    <div class="stack">
      <SegmentedControl
        value={kind}
        onchange={(v) => (kind = v as "device" | "user")}
        ariaLabel="Kind"
        block
        options={[
          { value: "device", label: "Device" },
          { value: "user", label: "Staff key for the CLI" },
        ]}
      />
      <label class="field">
        <span>Name</span>
        <TextInput bind:value={name} block placeholder={kind === "device" ? "For example: Pass tablet" : "For example: Laptop CLI"} autofocus />
      </label>
      {#if kind === "user"}
        <label class="field">
          <span>Acts as</span>
          <SelectDropdown
            value={userId}
            options={[{ value: "", label: "Choose…" }, ...users.filter((u) => u.active).map((u) => ({ value: u.id, label: u.name }))]}
            onchange={(v) => (userId = v)}
            title="Staff member"
          />
        </label>
      {/if}
    </div>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (creating = false)} />
        <Button tone="success" surface="solid" label={busy ? "Making…" : "Make key"} disabled={busy || !name.trim() || (kind === "user" && !userId)} onclick={create} />
      </div>
    {/snippet}
  </Modal>
{/if}

{#if made}
  <Modal title={`Key made: ${made.key.name}`} onclose={() => (made = null)} maxWidth="640px">
    <div class="stack">
      <Notice tone="warning" title="Shown once" message="Copy it now. Kishtin stores only a hash of the key and cannot show it again." />
      {#if pairLink}
        <div class="field">
          <span>Pairing link: open it on the device</span>
          <div class="secret"><code>{pairLink}</code><CopyButton text={pairLink} ariaLabel="Copy pairing link" /></div>
        </div>
      {/if}
      <div class="field">
        <span>Key</span>
        <div class="secret"><code>{made.secret}</code><CopyButton text={made.secret} ariaLabel="Copy key" /></div>
      </div>
      {#if made.key.kind === "user"}
        <p class="muted">Use it with the CLI: <code>export KISHTIN_API_KEY=…</code></p>
      {/if}
    </div>
    {#snippet footer()}
      <Button tone="info" surface="solid" label="Done" onclick={() => (made = null)} />
    {/snippet}
  </Modal>
{/if}

{#if revoking}
  <Modal title={`Revoke ${revoking.name}?`} onclose={() => (revoking = null)} maxWidth="480px">
    <p>Anything using this key stops working straight away. This cannot be undone; make a new key to replace it.</p>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (revoking = null)} />
        <Button tone="danger" surface="solid" label={busy ? "Revoking…" : "Revoke key"} disabled={busy} onclick={revoke} />
      </div>
    {/snippet}
  </Modal>
{/if}

{#if connecting}
  <ConnectTablet onclose={() => (connecting = false)} onpaired={() => void load()} />
{/if}

<style>
  .head-actions {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .secret {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    background: var(--bg-inset);
    border-radius: var(--radius-md);
  }
  code {
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
    word-break: break-all;
  }
  .revoked td {
    color: var(--text-secondary);
  }
  .end {
    text-align: right;
  }
  .nowrap {
    white-space: nowrap;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
