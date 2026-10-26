<script lang="ts">
  import { Button, Card, Chip, Modal, Notice, SelectDropdown, Spinner, Table, TableHeaderCell, TableSort, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { Plus } from "@lucide/svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { onChange } from "../../lib/live.svelte";
  import { can } from "../../lib/can.svelte";
  import { session } from "../../lib/session.svelte";

  type User = Schemas["User"];

  const ROLES: Record<string, string> = {
    head_chef: "Head chef",
    line_cook: "Line cook",
    front_of_house: "Front of house",
    manager: "Manager",
  };
  const roleOptions = Object.entries(ROLES).map(([value, label]) => ({ value, label }));

  let users = $state<User[] | null>(null);
  let error = $state("");
  let busy = $state(false);

  // Add form
  let adding = $state(false);
  let name = $state("");
  let role = $state("line_cook");
  let pin = $state("");
  let password = $state("");

  // Anonymise a leaver
  let anonymising = $state<Schemas["AnonymisePreview"] | null>(null);

  async function previewAnonymise(u: User) {
    try {
      anonymising = await api.previewAnonymise(u.id);
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function anonymise() {
    if (!anonymising) return;
    busy = true;
    try {
      const res = await api.anonymise(anonymising.user.id);
      showFlash(`Anonymised: now ${res.user.name}`, { tone: "success" });
      anonymising = null;
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  // Reset PIN or password
  let resetting = $state<User | null>(null);
  let newPin = $state("");
  let newPassword = $state("");

  async function load() {
    try {
      users = (await api.users()).users;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  $effect(() => {
    if (!can("users.manage")) return;
    void load();
    return onChange((kind) => {
      if (kind === "staff") void load();
    });
  });

  const sorter = new TableSort<User>(
    { name: (u) => u.name.toLowerCase(), role: (u) => ROLES[u.role] ?? u.role, active: (u) => (u.active ? 0 : 1) },
    { key: "active" },
  );
  const rows = $derived(sorter.sort(users ?? []));

  const pinOK = (p: string) => /^\d{4,8}$/.test(p);
  const canAdd = $derived(
    name.trim() !== "" && (pin === "" || pinOK(pin)) && (pin !== "" || password !== "") && (password === "" || password.length >= 10) && (role !== "manager" || password !== ""),
  );

  async function add() {
    busy = true;
    try {
      const body: Schemas["NewUser"] = { name: name.trim(), role };
      if (pin) body.pin = pin;
      if (password) body.password = password;
      await api.createUser(body);
      showFlash(`${name.trim()} added`, { tone: "success" });
      adding = false;
      name = "";
      pin = "";
      password = "";
      role = "line_cook";
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  async function update(u: User, change: Schemas["UserChange"], done: string) {
    busy = true;
    try {
      await api.updateUser(u.id, change);
      showFlash(done, { tone: "success" });
      await load();
      return true;
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
      return false;
    } finally {
      busy = false;
    }
  }

  async function saveReset() {
    if (!resetting) return;
    const change: Schemas["UserChange"] = {};
    if (newPin) change.pin = newPin;
    if (newPassword) change.password = newPassword;
    if (await update(resetting, change, `Login changed for ${resetting.name}`)) {
      resetting = null;
      newPin = "";
      newPassword = "";
    }
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Staff</h1>
      <p>Who can log in. Staff use a 4–8 digit PIN on kitchen devices. Managers also need a password to change setup. Kishtin keeps names and roles only.</p>
    </div>
    {#if can("users.manage")}
      <Button tone="info" surface="solid" label="Add staff" onclick={() => (adding = true)}><Plus size={16} /></Button>
    {/if}
  </header>

  <ManagerGate what="Staff" action="users.manage">
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if users === null}
      {#if !error}<Spinner />{/if}
    {:else}
      <Card padding="none">
        <Table ariaLabel="Staff">
          {#snippet header()}
            <TableHeaderCell label="Name" sort={sorter} column="name" />
            <TableHeaderCell label="Role" sort={sorter} column="role" />
            <TableHeaderCell label="Status" sort={sorter} column="active" />
            <TableHeaderCell><span class="kit-sr-only">Actions</span></TableHeaderCell>
          {/snippet}
          {#each rows as u (u.id)}
            <tr class:inactive={!u.active}>
              <td><strong>{u.name}</strong>{#if u.id === session.user?.id}&nbsp;<span class="muted">(you)</span>{/if}</td>
              <td>
                <SelectDropdown
                  value={u.role}
                  options={roleOptions}
                  title={`Role for ${u.name}`}
                  disabled={busy || u.id === session.user?.id || Boolean(u.anonymised_at)}
                  onchange={(v) => update(u, { role: v }, `${u.name} is now ${ROLES[v] ?? v}`)}
                />
              </td>
              <td><Chip tone={u.active ? "success" : "muted"} uppercase={false}>{u.active ? "Active" : u.anonymised_at ? "Anonymised" : "Deactivated"}</Chip></td>
              <td class="end">
                <div class="actions">
                  {#if !u.anonymised_at}
                    <Button size="sm" label="Change PIN or password" disabled={busy} onclick={() => (resetting = u)} />
                  {/if}
                  {#if !u.active && !u.anonymised_at}
                    <Button size="sm" label="Anonymise" disabled={busy} onclick={() => previewAnonymise(u)} />
                  {/if}
                  {#if u.id !== session.user?.id && !u.anonymised_at}
                    <Button
                      size="sm"
                      tone={u.active ? "danger" : "neutral"}
                      label={u.active ? "Deactivate" : "Reactivate"}
                      disabled={busy}
                      onclick={() => update(u, { active: !u.active }, u.active ? `${u.name} deactivated` : `${u.name} reactivated`)}
                    />
                  {/if}
                </div>
              </td>
            </tr>
          {/each}
        </Table>
      </Card>
      <p class="muted small">Staff are never deleted, because HACCP records name who logged them. Deactivate someone who has left; once deactivated, Anonymise replaces their name in the kitchen's other logs.</p>
    {/if}
  </ManagerGate>
</div>

{#if adding}
  <Modal title="Add staff" onclose={() => (adding = false)} maxWidth="520px">
    <div class="stack">
      <label class="field"><span>Name</span><TextInput bind:value={name} block autofocus /></label>
      <label class="field"><span>Role</span><SelectDropdown value={role} options={roleOptions} onchange={(v) => (role = v)} title="Role" /></label>
      <label class="field"><span>PIN (4–8 digits)</span><TextInput bind:value={pin} block autocomplete="off" invalid={pin !== "" && !pinOK(pin)} /></label>
      <label class="field">
        <span>Password {role === "manager" ? "(at least 10 characters; needed to change setup)" : "(optional, at least 10 characters; for actions a manager allows with a password in Setup › Staff and access › Permissions)"}</span>
        <TextInput type="password" bind:value={password} block autocomplete="new-password" />
      </label>
      <span class="muted small">A PIN or a password is needed to log in.</span>
    </div>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (adding = false)} />
        <Button tone="success" surface="solid" label={busy ? "Adding…" : "Add staff"} disabled={busy || !canAdd} onclick={add} />
      </div>
    {/snippet}
  </Modal>
{/if}

{#if anonymising}
  <Modal title={`Anonymise ${anonymising.user.name}`} onclose={() => (anonymising = null)} maxWidth="560px">
    <div class="stack">
      {#if anonymising.problem}
        <Notice tone="error" message={`${anonymising.user.name} cannot be anonymised: ${anonymising.problem}.`} />
      {:else}
        <Notice tone="warning" title="This cannot be undone" message={`${anonymising.user.name} will be called ${anonymising.name}.`} />
      {/if}
      <ul>
        {#each anonymising.notes as n (n)}<li>{n}</li>{/each}
      </ul>
      <div class="row end">
        <Button label="Cancel" onclick={() => (anonymising = null)} />
        <Button tone="danger" surface="solid" label={busy ? "Anonymising…" : `Anonymise ${anonymising.user.name}`} disabled={busy || Boolean(anonymising.problem)} onclick={anonymise} />
      </div>
    </div>
  </Modal>
{/if}

{#if resetting}
  <Modal title={`Change login for ${resetting.name}`} onclose={() => (resetting = null)} maxWidth="520px">
    <div class="stack">
      <label class="field"><span>New PIN (4–8 digits)</span><TextInput bind:value={newPin} block autocomplete="off" invalid={newPin !== "" && !pinOK(newPin)} /></label>
      <label class="field"><span>New password (leave empty to keep)</span><TextInput type="password" bind:value={newPassword} block autocomplete="new-password" /></label>
    </div>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (resetting = null)} />
        <Button
          tone="success"
          surface="solid"
          label={busy ? "Saving…" : "Save"}
          disabled={busy || (newPin === "" && newPassword === "") || (newPin !== "" && !pinOK(newPin))}
          onclick={saveReset}
        />
      </div>
    {/snippet}
  </Modal>
{/if}

<style>
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-3);
  }
  .end {
    text-align: right;
  }
  .inactive td {
    color: var(--text-secondary);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
