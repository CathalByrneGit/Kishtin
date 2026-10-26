<script lang="ts">
  import { Button, Card, Checkbox, Chip, IconButton, Notice, SegmentedControl, SelectDropdown, Spinner, showFlash } from "@kenn-io/kit-ui";
  import { Lock, RotateCcw, X } from "@lucide/svelte";
  import { onMount } from "svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import ReviewChanges, { type Preview } from "../../components/ReviewChanges.svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { can } from "../../lib/can.svelte";
  import { onChange } from "../../lib/live.svelte";
  import { DELEGABLE_ROLES, draftFrom, grantsFrom, isDefault, type Draft } from "../../lib/permissions";

  let actions = $state<Schemas["ActionInfo"][] | null>(null);
  let current = $state<Schemas["VersionedPermissions"] | null>(null);
  let staff = $state<Schemas["StaffEntry"][]>([]);
  let draft = $state<Record<string, Draft>>({});
  let error = $state("");
  let review = $state<{ preview: Preview; value: Schemas["Permissions"] } | null>(null);
  let saving = $state(false);
  const canEdit = $derived(can("permissions.manage"));

  function reset() {
    if (actions && current) draft = draftFrom(actions, current.value);
  }

  async function load() {
    try {
      const [a, p, s] = await Promise.all([api.actions(), api.permissions(), api.staff()]);
      actions = a;
      current = p;
      staff = s.staff;
      reset();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((k) => {
      if (k === "settings" || k === "user") void load();
    });
  });

  const value = $derived(actions ? { grants: grantsFrom(actions, draft) } : null);
  const dirty = $derived(
    value !== null && current !== null && JSON.stringify(value.grants) !== JSON.stringify(grantsFrom(actions ?? [], draftFrom(actions ?? [], current.value))),
  );
  const groups = $derived([...new Set((actions ?? []).map((a) => a.group))]);
  const names = $derived(Object.fromEntries(staff.map((s) => [s.id, s.name])));
  const roleLabel: Record<string, string> = { head_chef: "Head chefs", line_cook: "Line cooks", front_of_house: "Front of house" };

  function toggleRole(a: string, role: string, on: boolean) {
    const d = draft[a];
    if (!d) return;
    d.roles = on ? [...d.roles, role] : d.roles.filter((r) => r !== role);
  }

  async function reviewChanges() {
    if (!value) return;
    try {
      review = { preview: await api.previewPermissions({ value, reason: "" }), value };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function save(reason: string) {
    if (!review) return;
    saving = true;
    try {
      await api.savePermissions({ value: review.value, reason });
      review = null;
      showFlash("Saved: the new permissions apply straight away", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Permissions</h1>
      <p>Who may do what. Give a role, or named staff, an action a manager usually does, such as signing off records. Managers may always do everything. Every change is saved as a new version with a reason.</p>
    </div>
    {#if canEdit && dirty}
      <div class="row">
        <Button label="Undo changes" onclick={reset} />
        <Button tone="info" surface="solid" label="Review changes" onclick={reviewChanges} />
      </div>
    {/if}
  </header>

  <ManagerGate what="Permissions" action="permissions.manage" viewable>
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if !actions || !current}
      {#if !error}<Spinner />{/if}
    {:else}
      <Notice
        tone="info"
        title={current.builtin ? "Kishtin's default permissions" : `Version saved: ${current.version.reason}`}
        message="A password login lasts 30 minutes; a PIN login lasts a shift. Staff need a password (Setup › Staff) for actions that need one."
      />
      {#each groups as g (g)}
        <Card padding="md" title={g}>
          <ul class="actions">
            {#each actions.filter((a) => a.group === g) as a (a.action)}
              {@const d = draft[a.action]}
              <li>
                <div class="name">
                  <strong>{a.label}</strong>
                  {#if a.fixed}
                    <span class="muted small">{a.why}</span>
                  {:else if d && !isDefault(a, d)}
                    <Chip uppercase={false} tone="info">Changed</Chip>
                  {/if}
                </div>
                {#if a.fixed || !d}
                  <div class="fixed muted"><Lock size={14} /> Managers with a password, always</div>
                {:else}
                  <div class="who">
                    <Chip uppercase={false} tone="neutral">Managers</Chip>
                    {#each DELEGABLE_ROLES as r (r)}
                      <Checkbox
                        checked={d.roles.includes(r)}
                        disabled={!canEdit}
                        label={roleLabel[r]}
                        onchange={(on) => toggleRole(a.action, r, on)}
                      />
                    {/each}
                  </div>
                  <div class="people">
                    {#each d.users as u (u)}
                      <span class="person">
                        {names[u] ?? "Someone no longer on staff"}
                        {#if canEdit}
                          <IconButton ariaLabel={`Remove ${names[u] ?? "this person"} from ${a.label}`} onclick={() => (d.users = d.users.filter((x) => x !== u))}><X size={14} /></IconButton>
                        {/if}
                      </span>
                    {/each}
                    {#if canEdit}
                      {@const others = staff.filter((s) => s.role !== "manager" && !d.users.includes(s.id) && !d.roles.includes(s.role))}
                      {#if others.length > 0}
                        <SelectDropdown
                          value=""
                          options={[{ value: "", label: "Add a person…" }, ...others.map((s) => ({ value: s.id, label: s.name }))]}
                          onchange={(v) => v && (d.users = [...d.users, v])}
                          title={`Add a person to ${a.label}`}
                        />
                      {/if}
                    {/if}
                  </div>
                  <div class="login">
                    <SegmentedControl
                      ariaLabel={`Login for ${a.label}`}
                      value={d.pin ? "pin" : "password"}
                      disabled={!canEdit}
                      onchange={(v) => (d.pin = v === "pin")}
                      options={[
                        { value: "pin", label: "PIN is enough" },
                        { value: "password", label: "Needs a password" },
                      ]}
                    />
                    {#if canEdit && !isDefault(a, d)}
                      <Button size="sm" label="Reset to default" onclick={() => (draft[a.action] = { roles: [...a.roles.filter((r) => r !== "manager")], users: [], pin: a.pin })}><RotateCcw size={14} /></Button>
                    {/if}
                  </div>
                {/if}
              </li>
            {/each}
          </ul>
        </Card>
      {/each}
    {/if}
  </ManagerGate>
</div>

{#if review}
  <ReviewChanges title="Review permission changes" preview={review.preview} busy={saving} onsave={(reason) => save(reason)} onclose={() => (review = null)} />
{/if}

<style>
  .actions {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: grid;
    grid-template-columns: minmax(200px, 1.2fr) minmax(0, 2fr);
    gap: var(--space-2) var(--space-5);
    padding: var(--space-4) 0;
    border-bottom: var(--border-width) solid var(--border-default);
  }
  li:last-child {
    border-bottom: 0;
  }
  .name {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    align-items: flex-start;
  }
  .who,
  .people,
  .login {
    grid-column: 2;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-4);
  }
  .who {
    grid-row: 1;
  }
  .fixed {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .person {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 0 var(--space-1) 0 var(--space-3);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  .muted {
    color: var(--text-secondary);
  }
  @media (max-width: 760px) {
    li {
      grid-template-columns: minmax(0, 1fr);
    }
    .who,
    .people,
    .login {
      grid-column: 1;
      grid-row: auto;
    }
  }
</style>
