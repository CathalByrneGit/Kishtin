<script lang="ts">
  import {
    Button,
    Card,
    Chip,
    IconButton,
    Modal,
    Notice,
    SegmentedControl,
    Spinner,
    Table,
    TextInput,
    ThemeToggle,
    Toggle,
    getHighContrast,
    setHighContrast,
    showFlash,
  } from "@kenn-io/kit-ui";
  import { Plus, Trash2 } from "@lucide/svelte";
  import { onMount } from "svelte";
  import ReviewChanges, { type Preview } from "../components/ReviewChanges.svelte";
  import { api, message, type Lists, type Schemas } from "../lib/api";
  import { temp } from "../lib/format";
  import { typeLabel } from "../lib/labels";
  import { onChange } from "../lib/live.svelte";
  import { isManagerPassword, session, unpairDevice } from "../lib/session.svelte";
  import Login from "./Login.svelte";

  let lists = $state<Lists | null>(null);
  let system = $state<Schemas["System"] | null>(null);
  let limits = $state<Schemas["Limits"] | null>(null);
  let guidance = $state("");
  let error = $state("");

  // Drafts the manager edits before reviewing.
  let name = $state("");
  let units = $state<Schemas["Unit"][]>([]);
  let tasks = $state<Schemas["CleaningTask"][]>([]);
  let newUnit = $state("");
  let newUnitKind = $state("fridge");
  let newTask = $state("");

  let review = $state<{ title: string; preview: Preview; save: (reason: string, planRef: string) => Promise<void> } | null>(null);
  let saving = $state(false);
  let managerLogin = $state(false);

  const canEdit = $derived(isManagerPassword());

  async function load() {
    try {
      const [l, s, lim, g] = await Promise.all([api.lists(), api.system(), api.limits(), api.guidance()]);
      lists = l.value;
      system = s.value;
      limits = lim;
      guidance = g.value.name;
      reset();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  function reset() {
    if (!lists || !system) return;
    name = system.restaurant_name;
    units = lists.units.map((u) => ({ ...u, checks: (u.checks ?? []).map((c) => ({ ...c })) }));
    tasks = lists.cleaning_tasks.map((t) => ({ ...t }));
  }

  onMount(() => {
    void load();
    return onChange((kind) => {
      if (kind === "settings" || kind === "limit") void load();
    });
  });

  const listsDirty = $derived(
    lists !== null && (JSON.stringify(units) !== JSON.stringify(lists.units.map((u) => ({ ...u, checks: u.checks ?? [] }))) || JSON.stringify(tasks) !== JSON.stringify(lists.cleaning_tasks)),
  );

  function addUnit() {
    if (!newUnit.trim()) return;
    units.push({ name: newUnit.trim(), kind: newUnitKind, checks: [] });
    newUnit = "";
  }

  function addTask() {
    if (!newTask.trim()) return;
    tasks.push({ task: newTask.trim() });
    newTask = "";
  }

  function editChecks(u: Schemas["Unit"]) {
    if (!u.checks || u.checks.length === 0) u.checks = [{ label: "AM", from: "00:00" }, { label: "PM", from: "12:00" }];
  }

  async function reviewLists() {
    if (!lists) return;
    const value: Lists = { ...lists, units, cleaning_tasks: tasks };
    try {
      const preview = await api.previewLists({ value, reason: "" });
      review = {
        title: "Review list changes",
        preview,
        save: async (reason, planRef) => {
          await api.saveLists({ value, reason, plan_ref: planRef || undefined });
        },
      };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function reviewName() {
    if (!system) return;
    const value = { ...system, restaurant_name: name.trim() };
    try {
      const preview = await api.previewSystem({ value, reason: "" });
      review = {
        title: "Review name change",
        preview,
        save: async (reason) => {
          await api.saveSystem({ value, reason });
        },
      };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function saveReviewed(reason: string, planRef: string) {
    if (!review) return;
    saving = true;
    try {
      await review.save(reason, planRef);
      review = null;
      showFlash("Saved", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }

  function limitWords(l: Schemas["LimitVersion"]): string {
    const parts: string[] = [];
    if (l.max_c !== undefined && l.max_c !== null) parts.push(`${temp(l.max_c)} or ${l.check === "freezer" || l.check === "delivery_frozen" ? "colder" : "below"}`);
    if (l.borderline_max_c !== undefined && l.borderline_max_c !== null) parts.push(`borderline to ${temp(l.borderline_max_c)}`);
    if (l.min_c !== undefined && l.min_c !== null) parts.push(`${temp(l.min_c)} or above`);
    for (const c of l.combos ?? []) parts.push(c.hold_seconds ? `core ${temp(c.min_c)} for ${Math.round(c.hold_seconds / 60)} min` : `core ${temp(c.min_c)} or above`);
    if (l.cooling) {
      parts.push(`into fridge within ${l.cooling.into_fridge_within_seconds / 3600} h; serve or discard if above ${temp(l.cooling.cutoff_max_c)} at ${l.cooling.cutoff_after_seconds / 3600} h`);
    }
    return parts.join(", ");
  }

  const checkNames: Record<string, string> = {
    fridge: "Fridge", freezer: "Freezer", cook: "Cooking, core", reheat: "Reheating, core", hot_hold: "Hot holding",
    cooling: "Cooling", delivery_chilled: "Chilled delivery", delivery_frozen: "Frozen delivery",
  };
</script>

<div class="stack">
  {#if error}<Notice tone="error" message={error} />{/if}

  {#if !canEdit}
    <Notice
      tone="info"
      title="Setup is shared by every device"
      message="Only a manager logged in with their password can change it. Everyone can see it."
      actionLabel="Manager login"
      onaction={() => (managerLogin = true)}
    />
  {/if}

  {#if !lists || !system}
    {#if !error}<Spinner />{/if}
  {:else}
    <h2 class="section-title">Restaurant</h2>
    <Card padding="md">
      <div class="stack">
        <label class="field">
          <span>Restaurant name</span>
          <TextInput bind:value={name} size="lg" disabled={!canEdit} />
        </label>
        {#if canEdit && name.trim() !== system.restaurant_name}
          <Button tone="info" surface="solid" label="Review name change" onclick={reviewName} />
        {/if}
      </div>
    </Card>

    <h2 class="section-title">Fridges &amp; freezers</h2>
    <Card padding="md">
      <ul class="items">
        {#each units as u, i (i)}
          <li>
            <div class="row grow">
              <strong>{u.name}</strong>
              <Chip uppercase={false}>{typeLabel(u.kind)}</Chip>
              {#if u.non_food}<Chip uppercase={false} tone="muted">Non-food</Chip>{/if}
              {#if canEdit}
                <IconButton ariaLabel={`Remove ${u.name}`} onclick={() => units.splice(i, 1)}><Trash2 size={20} /></IconButton>
              {/if}
            </div>
            <div class="checks">
              <span class="muted">Daily checks:</span>
              {#if !u.checks || u.checks.length === 0}
                <span class="muted">AM (until 12:00) and PM</span>
                {#if canEdit}<Button label="Change check times" onclick={() => editChecks(u)} />{/if}
              {:else}
                {#each u.checks as c, ci (ci)}
                  <div class="check-row">
                    <TextInput bind:value={c.label} ariaLabel="Check name" disabled={!canEdit} />
                    <TextInput bind:value={c.from} ariaLabel="From (HH:MM)" placeholder="08:00" disabled={!canEdit} />
                    {#if canEdit}
                      <IconButton ariaLabel={`Remove check ${c.label}`} onclick={() => u.checks?.splice(ci, 1)}><Trash2 size={18} /></IconButton>
                    {/if}
                  </div>
                {/each}
                {#if canEdit}
                  <Button label="Add a check" onclick={() => u.checks?.push({ label: "", from: "" })}><Plus size={18} /></Button>
                {/if}
                <span class="muted">Each check runs from its time until the next one starts. A check not logged by then shows as missed.</span>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
      {#if canEdit}
        <div class="add">
          <TextInput bind:value={newUnit} placeholder="New fridge or freezer" size="lg" />
          <SegmentedControl
            value={newUnitKind}
            onchange={(v) => (newUnitKind = v)}
            ariaLabel="Type"
            options={[
              { value: "fridge", label: "Fridge" },
              { value: "freezer", label: "Freezer" },
            ]}
          />
          <Button label="Add" onclick={addUnit} disabled={!newUnit.trim()}><Plus size={18} /></Button>
        </div>
      {/if}
    </Card>

    <h2 class="section-title">Daily cleaning tasks</h2>
    <Card padding="md">
      <ul class="items">
        {#each tasks as t, i (i)}
          <li class="row grow">
            <span>{t.task}</span>
            {#if canEdit}
              <IconButton ariaLabel={`Remove ${t.task}`} onclick={() => tasks.splice(i, 1)}><Trash2 size={20} /></IconButton>
            {/if}
          </li>
        {/each}
      </ul>
      {#if canEdit}
        <div class="add">
          <TextInput bind:value={newTask} placeholder="New cleaning task" size="lg" />
          <Button label="Add" onclick={addTask} disabled={!newTask.trim()}><Plus size={18} /></Button>
        </div>
      {/if}
    </Card>

    {#if canEdit && listsDirty}
      <div class="row sticky">
        <Button label="Undo changes" onclick={reset} />
        <Button tone="info" surface="solid" label="Review changes" onclick={reviewLists} />
      </div>
    {/if}

    <h2 class="section-title">Limits in use</h2>
    <Card padding="md">
      <div class="stack">
        <Table unsorted ariaLabel="Limits in use" scroll="page">
          {#snippet header()}
            <th scope="col">Check</th>
            <th scope="col">Applies to</th>
            <th scope="col">Limit</th>
          {/snippet}
          {#each limits?.current ?? [] as l (l.id)}
            <tr>
              <td>{checkNames[l.check] ?? l.check}</td>
              <td>{l.scope.kind === "default" ? "All" : l.scope.ref}</td>
              <td class="num">{limitWords(l)}</td>
            </tr>
          {/each}
        </Table>
        <p class="muted">
          Measured against: {guidance}. Based on FSAI Safe Catering guidance. Check these match your own HACCP plan. A manager
          changes limits with <code>kishtin limits set</code>.
        </p>
      </div>
    </Card>
  {/if}

  <h2 class="section-title">This device</h2>
  <Card padding="md">
    <div class="stack">
      <ThemeToggle variant="segmented" block />
      <Toggle checked={getHighContrast()} label="High contrast" onchange={(on) => setHighContrast(on)} />
      <span class="muted">Logged in as {session.user?.name}{session.method === "password" ? " (manager password)" : ""}.</span>
      <Button tone="danger" label="Unpair this device" onclick={unpairDevice} />
    </div>
  </Card>
</div>

{#if review}
  <ReviewChanges title={review.title} preview={review.preview} busy={saving} onsave={saveReviewed} onclose={() => (review = null)} />
{/if}

{#if managerLogin}
  <Modal title="Manager login" onclose={() => (managerLogin = false)} maxWidth="520px">
    <p class="muted">This device will be logged in as the manager for 30 minutes.</p>
    <Login managersOnly password onloggedin={() => (managerLogin = false)} />
  </Modal>
{/if}

<style>
  .items {
    list-style: none;
    margin: 0 0 var(--space-5);
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    font-size: var(--font-size-lg);
  }
  .grow {
    justify-content: space-between;
  }
  .checks {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-3);
    font-size: var(--font-size-md);
  }
  .check-row {
    display: grid;
    grid-template-columns: 1fr 120px auto;
    gap: var(--space-4);
    align-items: center;
  }
  .add {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
    align-items: center;
  }
  .add :global(.kit-text-input) {
    flex: 1;
    min-width: 200px;
  }
  .sticky {
    position: sticky;
    bottom: 88px;
    justify-content: flex-end;
    padding: var(--space-4);
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
  }
  p {
    margin: 0;
  }
  code {
    font-family: var(--font-mono);
  }
</style>
