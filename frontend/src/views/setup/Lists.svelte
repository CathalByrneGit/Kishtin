<script lang="ts">
  import {
    Button,
    Card,
    Checkbox,
    Chip,
    IconButton,
    Notice,
    SegmentedControl,
    SelectDropdown,
    Spinner,
    Table, TableHeaderCell,
    TextInput,
    showFlash,
  } from "@kenn-io/kit-ui";
  import { Plus, Trash2, X } from "@lucide/svelte";
  import { onMount } from "svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import ReviewChanges, { type Preview } from "../../components/ReviewChanges.svelte";
  import { api, message, type Lists, type Schemas } from "../../lib/api";
  import { everyOptions } from "../../lib/cleaning";
  import { LOGGABLE_TYPES, TYPE_LABELS, typeLabel } from "../../lib/labels";
  import { onChange } from "../../lib/live.svelte";
  import { can } from "../../lib/can.svelte";

  let lists = $state<Lists | null>(null);
  let version = $state<Schemas["SettingsVersion"] | null>(null);
  let savedBy = $state("");
  let error = $state("");

  // Drafts the manager edits before reviewing.
  let units = $state<Schemas["Unit"][]>([]);
  let tasks = $state<Schemas["CleaningTask"][]>([]);
  let categories = $state<string[]>([]);
  let actions = $state<Schemas["ActionPreset"][]>([]);

  let newUnit = $state("");
  let newUnitKind = $state("fridge");
  let newTask = $state("");
  let newTaskEvery = $state("0");
  let newTaskArea = $state("");
  let newCategory = $state("");
  let newAction = $state("");
  let newActionType = $state("fridge");

  let review = $state<{ preview: Preview; value: Lists } | null>(null);
  let saving = $state(false);

  const canEdit = $derived(can("haccp.limits"));

  function normalise(l: Lists) {
    return {
      units: l.units.map((u) => ({ ...u, checks: (u.checks ?? []).map((c) => ({ ...c })) })),
      tasks: l.cleaning_tasks.map((t) => ({ ...t })),
      categories: [...l.food_categories],
      actions: (l.corrective_actions ?? []).map((a) => ({ ...a })),
    };
  }

  function reset() {
    if (!lists) return;
    const n = normalise(lists);
    units = n.units;
    tasks = n.tasks;
    categories = n.categories;
    actions = n.actions;
  }

  async function load() {
    try {
      const [l, st] = await Promise.all([api.lists(), api.staff()]);
      lists = l.value;
      version = l.builtin ? null : l.version;
      savedBy = st.staff.find((s) => s.id === l.version.created_by)?.name ?? "a manager";
      reset();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((kind) => {
      if (kind === "settings") void load();
    });
  });

  const dirty = $derived(
    lists !== null && JSON.stringify(normalise(lists)) !== JSON.stringify({ units, tasks, categories, actions }),
  );

  function addUnit() {
    if (!newUnit.trim()) return;
    units.push({ name: newUnit.trim(), kind: newUnitKind, checks: [] });
    newUnit = "";
  }

  function addTask() {
    if (!newTask.trim()) return;
    const every = Number(newTaskEvery);
    tasks.push({ task: newTask.trim(), ...(newTaskArea.trim() ? { area: newTaskArea.trim() } : {}), ...(every > 1 ? { every_days: every } : {}) });
    newTask = "";
    newTaskEvery = "0";
    newTaskArea = "";
  }

  function addCategory() {
    const c = newCategory.trim();
    if (c && !categories.includes(c)) categories.push(c);
    newCategory = "";
  }

  function addAction() {
    if (!newAction.trim()) return;
    actions.push({ type: newActionType, text: newAction.trim() });
    newAction = "";
  }

  async function reviewChanges() {
    if (!lists) return;
    const value: Lists = { ...lists, units, cleaning_tasks: tasks, food_categories: categories, corrective_actions: actions };
    try {
      review = { preview: await api.previewLists({ value, reason: "" }), value };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function save(reason: string, planRef: string) {
    if (!review) return;
    saving = true;
    try {
      await api.saveLists({ value: review.value, reason, plan_ref: planRef || undefined });
      review = null;
      showFlash("Saved", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }

  const typeOptions = LOGGABLE_TYPES.map((value) => ({ value, label: TYPE_LABELS[value] ?? value }));
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Fridges and tasks</h1>
      <p>The fridges and freezers staff check, their daily check times, cleaning tasks, food categories and quick-pick corrective actions.</p>
    </div>
    {#if version}<span class="muted small">Last changed by {savedBy}: {version.reason}</span>{/if}
  </header>

  <ManagerGate what="This list" action="haccp.limits" viewable>
    {#if error}<Notice tone="error" message={error} />{/if}

    {#if !lists}
      {#if !error}<Spinner />{/if}
    {:else}
      <Card padding="none">
          <div class="table-head"><h2>Fridges and freezers</h2><span class="muted">Each check runs from its time until the next one starts. A check not logged by then shows as missed.</span></div>
        <Table unsorted ariaLabel="Fridges and freezers">
          {#snippet header()}
            <TableHeaderCell label="Name" />
            <TableHeaderCell label="Type" />
            <TableHeaderCell label="Daily checks" />
            <TableHeaderCell><span class="kit-sr-only">Remove</span></TableHeaderCell>
          {/snippet}
          {#each units as u, i (i)}
            <tr>
              <td>
                <strong>{u.name}</strong>
                {#if canEdit && u.kind === "fridge"}
                  <div class="nonfood"><Checkbox checked={Boolean(u.non_food)} label="Non-food (drinks only)" onchange={(on) => (u.non_food = on || undefined)} /></div>
                {:else if u.non_food}
                  <Chip size="sm" uppercase={false} tone="muted">Non-food</Chip>
                {/if}
              </td>
              <td>{typeLabel(u.kind)}</td>
              <td>
                {#if !u.checks || u.checks.length === 0}
                  <div class="row">
                    <span class="muted">AM (until 12:00) and PM</span>
                    {#if canEdit}
                      <Button size="sm" label="Set check times" onclick={() => (u.checks = [{ label: "AM", from: "00:00" }, { label: "PM", from: "12:00" }])} />
                    {/if}
                  </div>
                {:else}
                  <div class="checks">
                    {#each u.checks as c, ci (ci)}
                      <div class="check-row">
                        <TextInput bind:value={c.label} ariaLabel="Check name" size="sm" disabled={!canEdit} />
                        <TextInput bind:value={c.from} ariaLabel="From (HH:MM)" placeholder="08:00" size="sm" disabled={!canEdit} />
                        {#if canEdit}
                          <IconButton ariaLabel={`Remove check ${c.label}`} onclick={() => u.checks?.splice(ci, 1)}><X size={14} /></IconButton>
                        {/if}
                      </div>
                    {/each}
                    {#if canEdit}
                      <div><Button size="sm" label="Add a check" onclick={() => u.checks?.push({ label: "", from: "" })}><Plus size={14} /></Button></div>
                    {/if}
                  </div>
                {/if}
              </td>
              <td class="end">
                {#if canEdit}
                  <IconButton ariaLabel={`Remove ${u.name}`} onclick={() => units.splice(i, 1)}><Trash2 size={16} /></IconButton>
                {/if}
              </td>
            </tr>
          {/each}
        </Table>
        {#if canEdit}
          <div class="add">
            <TextInput bind:value={newUnit} placeholder="New fridge or freezer" ariaLabel="New fridge or freezer" />
            <SegmentedControl
              value={newUnitKind}
              onchange={(v) => (newUnitKind = v)}
              ariaLabel="Type"
              options={[
                { value: "fridge", label: "Fridge" },
                { value: "freezer", label: "Freezer" },
              ]}
            />
            <Button label="Add" onclick={addUnit} disabled={!newUnit.trim()}><Plus size={16} /></Button>
          </div>
        {/if}
      </Card>

      <div class="grid">
        <Card padding="none">
          <div class="table-head"><h2>Cleaning tasks</h2></div>
          <Table unsorted ariaLabel="Cleaning tasks">
            {#snippet header()}
              <TableHeaderCell label="Task" />
              <TableHeaderCell label="Area" />
              <TableHeaderCell label="How often" />
              <TableHeaderCell><span class="kit-sr-only">Remove</span></TableHeaderCell>
            {/snippet}
            {#each tasks as t, i (i)}
              <tr>
                <td>{t.task}</td>
                <td class="muted">{t.area ?? ""}</td>
                <td>
                  <SelectDropdown
                    value={String(t.every_days ?? 0)}
                    options={everyOptions(t.every_days)}
                    disabled={!canEdit}
                    title={`How often: ${t.task}`}
                    onchange={(v) => (t.every_days = Number(v) > 1 ? Number(v) : undefined)}
                  />
                </td>
                <td class="end">
                  {#if canEdit}
                    <IconButton ariaLabel={`Remove ${t.task}`} onclick={() => tasks.splice(i, 1)}><Trash2 size={16} /></IconButton>
                  {/if}
                </td>
              </tr>
            {/each}
          </Table>
          {#if canEdit}
            <div class="add">
              <TextInput bind:value={newTask} placeholder="New cleaning task" ariaLabel="New cleaning task" />
              <TextInput bind:value={newTaskArea} placeholder="Area (optional)" ariaLabel="Area" />
              <SelectDropdown value={newTaskEvery} options={everyOptions(0)} title="How often" onchange={(v) => (newTaskEvery = v)} />
              <Button label="Add" onclick={addTask} disabled={!newTask.trim()}><Plus size={16} /></Button>
            </div>
          {/if}
        </Card>

        <Card padding="md" title="Food categories" meta="A category can have its own cooking limit.">
          <div class="row">
            {#each categories as c, i (c)}
              <Chip uppercase={false}>
                {c}
                {#if canEdit}
                  <button type="button" class="chip-x" aria-label={`Remove ${c}`} onclick={() => categories.splice(i, 1)}><X size={12} /></button>
                {/if}
              </Chip>
            {:else}
              <span class="muted">No categories yet.</span>
            {/each}
          </div>
          {#if canEdit}
            <div class="add inset">
              <TextInput bind:value={newCategory} placeholder="New category, such as poultry" ariaLabel="New food category" />
              <Button label="Add" onclick={addCategory} disabled={!newCategory.trim()}><Plus size={16} /></Button>
            </div>
          {/if}
        </Card>
      </div>

      <Card padding="none">
          <div class="table-head"><h2>Quick-pick corrective actions</h2><span class="muted">Offered when a reading is not a pass. Staff can always write their own.</span></div>
        <Table unsorted ariaLabel="Corrective actions">
          {#snippet header()}
            <TableHeaderCell label="Record type" />
            <TableHeaderCell label="Action" />
            <TableHeaderCell><span class="kit-sr-only">Remove</span></TableHeaderCell>
          {/snippet}
          {#each actions as a, i (i)}
            <tr>
              <td class="nowrap">{typeLabel(a.type)}</td>
              <td>{a.text}</td>
              <td class="end">
                {#if canEdit}
                  <IconButton ariaLabel={`Remove ${a.text}`} onclick={() => actions.splice(i, 1)}><Trash2 size={16} /></IconButton>
                {/if}
              </td>
            </tr>
          {/each}
        </Table>
        {#if canEdit}
          <div class="add">
            <SelectDropdown value={newActionType} options={typeOptions} onchange={(v) => (newActionType = v)} title="Record type" />
            <TextInput bind:value={newAction} placeholder="New corrective action" ariaLabel="New corrective action" />
            <Button label="Add" onclick={addAction} disabled={!newAction.trim()}><Plus size={16} /></Button>
          </div>
        {/if}
      </Card>

      {#if canEdit && dirty}
        <div class="savebar">
          <span>You have unsaved changes. Review them to see what changes before saving.</span>
          <Button label="Undo changes" onclick={reset} />
          <Button tone="info" surface="solid" label="Review changes" onclick={reviewChanges} />
        </div>
      {/if}
    {/if}
  </ManagerGate>
</div>

{#if review}
  <ReviewChanges title="Review list changes" preview={review.preview} busy={saving} onsave={save} onclose={() => (review = null)} />
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
    gap: var(--space-6);
    align-items: start;
  }
  .checks {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .check-row {
    display: grid;
    grid-template-columns: 160px 90px auto;
    gap: var(--space-3);
    align-items: center;
  }
  .nonfood {
    margin-top: var(--space-2);
    font-size: var(--font-size-sm);
  }
  .add {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
    align-items: center;
    padding: var(--space-4) var(--space-6);
    border-top: var(--border-width) solid var(--border-muted);
  }
  .add.inset {
    padding: var(--space-5) 0 0;
    border-top: 0;
  }
  .add :global(.kit-text-input) {
    flex: 1;
    min-width: 180px;
  }
  .end {
    text-align: right;
  }
  .nowrap {
    white-space: nowrap;
  }
  .chip-x {
    display: inline-grid;
    place-items: center;
    margin-left: var(--space-1);
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    cursor: pointer;
  }
  .savebar {
    position: sticky;
    bottom: var(--space-5);
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-4);
    padding: var(--space-4) var(--space-5);
    background: var(--bg-surface);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
  }
  .savebar span {
    flex: 1;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  @media (max-width: 900px) {
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
