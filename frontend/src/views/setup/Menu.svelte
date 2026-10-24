<script lang="ts">
  import { Button, Card, Chip, EmptyState, Modal, Notice, SelectDropdown, Spinner, Table, TableHeaderCell, TableSort, TextInput, Toggle, showFlash } from "@kenn-io/kit-ui";
  import { Plus } from "@lucide/svelte";
  import { onMount } from "svelte";
  import DishEditor from "../../components/DishEditor.svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import { api, message, type Dish, type Schemas, type Setup } from "../../lib/api";
  import { courseLabel, seconds } from "../../lib/kitchen";
  import { onChange } from "../../lib/live.svelte";
  import { isManagerPassword, session } from "../../lib/session.svelte";

  let menu = $state<Setup | null>(null);
  let categories = $state<string[]>([]);
  let error = $state("");
  let editing = $state<{ dish: Dish | null } | null>(null);
  let stationForm = $state<{ id: string | null; name: string; position: string; active: boolean } | null>(null);
  let equipmentForm = $state<{ id: string | null; stationId: string; name: string; slots: string; active: boolean } | null>(null);
  let formError = $state("");
  let busy = $state(false);

  const canEdit = $derived(isManagerPassword());
  const can86 = $derived(["head_chef", "line_cook", "manager"].includes(session.user?.role ?? ""));

  async function load() {
    try {
      const [m, l] = await Promise.all([api.menu(), api.lists()]);
      menu = m;
      categories = l.value.food_categories;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((k) => {
      if (k === "menu" || k === "settings") void load();
    });
  });

  const stationName = (id: string) => menu?.stations.find((s) => s.id === id)?.name ?? "";
  const equipmentOf = (id: string | null | undefined) => menu?.equipment.find((e) => e.id === id);

  const sorter = new TableSort<Dish>(
    {
      name: (d) => d.name.toLowerCase(),
      course: (d) => ["starter", "main", "dessert"].indexOf(d.course),
      station: (d) => stationName(d.station_id),
      equipment: (d) => equipmentOf(d.equipment_id)?.name ?? "",
      cook: (d) => d.cook_seconds,
      total: (d) => d.cook_seconds + d.rest_seconds + d.plate_seconds,
      hold: (d) => d.hold_seconds,
      status: (d) => (d.active ? (d.available ? 0 : 1) : 2),
    },
    { key: "course" },
  );
  let showInactive = $state(false);
  const dishes = $derived(sorter.sort((menu?.dishes ?? []).filter((d) => showInactive || d.active)));

  async function set86(d: Dish) {
    try {
      await api.setAvailability(d.id, !d.available);
      showFlash(d.available ? `${d.name} is 86: it cannot be ordered` : `${d.name} is back on`, { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function saveStation() {
    if (!stationForm) return;
    const f = stationForm;
    busy = true;
    formError = "";
    try {
      const body: Schemas["StationInput"] = { name: f.name, position: Number(f.position) || 0 };
      if (f.id) await api.updateStation(f.id, { ...body, active: f.active });
      else await api.createStation(body);
      stationForm = null;
      showFlash("Station saved", { tone: "success" });
      await load();
    } catch (err) {
      formError = message(err);
    } finally {
      busy = false;
    }
  }

  async function saveEquipment() {
    if (!equipmentForm) return;
    const f = equipmentForm;
    busy = true;
    formError = "";
    try {
      const body: Schemas["EquipmentInput"] = { station_id: f.stationId, name: f.name, slots: Number(f.slots) };
      if (f.id) await api.updateEquipment(f.id, { ...body, active: f.active });
      else await api.createEquipment(body);
      equipmentForm = null;
      showFlash("Equipment saved", { tone: "success" });
      await load();
    } catch (err) {
      formError = message(err);
    } finally {
      busy = false;
    }
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Menu</h1>
      <p>Stations, the equipment in them, and every dish with its timings. The scheduler plans from these. Orders already in the kitchen keep the times they were ordered with.</p>
    </div>
    {#if canEdit && menu}
      <div class="row">
        <Button label="Add station" onclick={() => ((formError = ""), (stationForm = { id: null, name: "", position: String(menu?.stations.length ?? 0), active: true }))}><Plus size={16} /></Button>
        <Button tone="info" surface="solid" label="Add dish" disabled={menu.stations.length === 0} onclick={() => (editing = { dish: null })}><Plus size={16} /></Button>
      </div>
    {/if}
  </header>

  <ManagerGate what="The menu" viewable>
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if !menu}
      {#if !error}<Spinner />{/if}
    {:else}
      {#if menu.stations.length === 0}
        <EmptyState title="No stations yet" description="Start with your stations (grill, fryer, cold section), then their equipment, then dishes." />
      {/if}

      <div class="stations">
        {#each menu.stations as s (s.id)}
          <Card padding="md">
            <div class="station-head">
              <strong>{s.name}</strong>
              {#if !s.active}<Chip size="sm" tone="muted" uppercase={false}>Not in use</Chip>{/if}
              <span class="grow"></span>
              {#if canEdit}
                <Button size="sm" label="Edit" onclick={() => ((formError = ""), (stationForm = { id: s.id, name: s.name, position: String(s.position), active: s.active }))} />
              {/if}
            </div>
            <ul class="equipment">
              {#each menu.equipment.filter((e) => e.station_id === s.id) as e (e.id)}
                <li>
                  <span>{e.name}{e.active ? "" : " (not in use)"}</span>
                  <span class="row">
                    <span class="muted num">{e.slots} at once</span>
                    {#if canEdit}
                      <Button size="sm" label="Edit" onclick={() => ((formError = ""), (equipmentForm = { id: e.id, stationId: e.station_id, name: e.name, slots: String(e.slots), active: e.active }))} />
                    {/if}
                  </span>
                </li>
              {:else}
                <li class="muted">No equipment: dishes here have no capacity limit.</li>
              {/each}
            </ul>
            {#if canEdit && s.active}
              <Button size="sm" label="Add equipment" onclick={() => ((formError = ""), (equipmentForm = { id: null, stationId: s.id, name: "", slots: "1", active: true }))}><Plus size={14} /></Button>
            {/if}
          </Card>
        {/each}
      </div>

      {#if menu.dishes.length > 0}
        <Card padding="none">
          <div class="table-head">
            <h2>Dishes</h2>
            <Toggle checked={showInactive} label="Show dishes off the menu" onchange={(on) => (showInactive = on)} />
          </div>
          <Table ariaLabel="Dishes">
            {#snippet header()}
              <TableHeaderCell label="Dish" sort={sorter} column="name" />
              <TableHeaderCell label="Course" sort={sorter} column="course" />
              <TableHeaderCell label="Station" sort={sorter} column="station" />
              <TableHeaderCell label="Cooks on" sort={sorter} column="equipment" />
              <TableHeaderCell label="Cook" numeric sort={sorter} column="cook" />
              <TableHeaderCell label="Cook + rest + plate" numeric sort={sorter} column="total" />
              <TableHeaderCell label="Can wait" numeric sort={sorter} column="hold" />
              <TableHeaderCell label="Status" sort={sorter} column="status" />
              <TableHeaderCell><span class="kit-sr-only">Actions</span></TableHeaderCell>
            {/snippet}
            {#each dishes as d (d.id)}
              {@const eq = equipmentOf(d.equipment_id)}
              <tr class:inactive={!d.active}>
                <td>
                  <strong>{d.name}</strong>
                  {#if d.modifiers.length > 0}
                    <div class="muted small">{d.modifiers.map((m) => `${m.name} ${m.add_seconds >= 0 ? "+" : "−"}${seconds(Math.abs(m.add_seconds))}`).join(" · ")}</div>
                  {/if}
                </td>
                <td>{courseLabel(d.course, false)}</td>
                <td>{stationName(d.station_id)}</td>
                <td>{eq ? `${eq.name}, ${d.batch_capacity || eq.slots} at once${d.batchable ? ", batched" : ""}` : "–"}</td>
                <td class="num">{seconds(d.cook_seconds)}</td>
                <td class="num">{seconds(d.cook_seconds + d.rest_seconds + d.plate_seconds)}</td>
                <td class="num">{seconds(d.hold_seconds)}</td>
                <td>
                  {#if !d.active}
                    <Chip size="sm" tone="muted" uppercase={false}>Off the menu</Chip>
                  {:else if d.available}
                    <Chip size="sm" tone="success" uppercase={false}>Available</Chip>
                  {:else}
                    <Chip size="sm" tone="danger" uppercase={false}>86</Chip>
                  {/if}
                </td>
                <td class="end">
                  <div class="row nowrap">
                    {#if can86 && d.active}
                      <Button size="sm" label={d.available ? "86" : "Back on"} tone={d.available ? "danger" : "neutral"} onclick={() => set86(d)} />
                    {/if}
                    {#if canEdit}<Button size="sm" label="Edit" onclick={() => (editing = { dish: d })} />{/if}
                  </div>
                </td>
              </tr>
            {/each}
          </Table>
        </Card>
      {/if}
    {/if}
  </ManagerGate>
</div>

{#if editing && menu}
  <DishEditor
    {menu}
    dish={editing.dish}
    {categories}
    onclose={() => (editing = null)}
    onsaved={(d) => {
      editing = null;
      showFlash(`${d.name} saved`, { tone: "success" });
      void load();
    }}
  />
{/if}

{#if stationForm}
  <Modal title={stationForm.id ? "Change station" : "New station"} onclose={() => (stationForm = null)} maxWidth="480px">
    <div class="stack">
      <label class="field"><span>Name</span><TextInput bind:value={stationForm.name} placeholder="Grill" block autofocus /></label>
      <label class="field"><span>Position on screens</span><TextInput bind:value={stationForm.position} block /></label>
      {#if stationForm.id}
        <Toggle checked={stationForm.active} label="In use" onchange={(on) => stationForm && (stationForm.active = on)} />
      {/if}
      {#if formError}<Notice tone="error" message={formError} />{/if}
    </div>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (stationForm = null)} />
        <Button tone="success" surface="solid" label={busy ? "Saving…" : "Save station"} disabled={busy || !stationForm?.name.trim()} onclick={saveStation} />
      </div>
    {/snippet}
  </Modal>
{/if}

{#if equipmentForm && menu}
  <Modal title={equipmentForm.id ? "Change equipment" : "New equipment"} onclose={() => (equipmentForm = null)} maxWidth="480px">
    <div class="stack">
      <label class="field">
        <span>Station</span>
        <SelectDropdown
          value={equipmentForm.stationId}
          options={menu.stations.filter((s) => s.active).map((s) => ({ value: s.id, label: s.name }))}
          onchange={(v) => equipmentForm && (equipmentForm.stationId = v)}
          title="Station"
        />
      </label>
      <label class="field"><span>Name</span><TextInput bind:value={equipmentForm.name} placeholder="Flat-top" block autofocus /></label>
      <label class="field"><span>How many it holds at once</span><TextInput bind:value={equipmentForm.slots} placeholder="3" block /></label>
      <p class="muted small">For example 3 burgers on the flat-top, or 2 fryer baskets. A dish can say it fits a different number.</p>
      {#if equipmentForm.id}
        <Toggle checked={equipmentForm.active} label="In use" onchange={(on) => equipmentForm && (equipmentForm.active = on)} />
      {/if}
      {#if formError}<Notice tone="error" message={formError} />{/if}
    </div>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (equipmentForm = null)} />
        <Button tone="success" surface="solid" label={busy ? "Saving…" : "Save equipment"} disabled={busy || !equipmentForm?.name.trim() || !/^\d+$/.test(equipmentForm?.slots ?? "")} onclick={saveEquipment} />
      </div>
    {/snippet}
  </Modal>
{/if}

<style>
  .stations {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: var(--space-5);
  }
  .station-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }
  .grow {
    flex: 1;
  }
  .equipment {
    list-style: none;
    margin: 0 0 var(--space-4);
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .equipment li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .inactive td {
    color: var(--text-secondary);
  }
  .end {
    text-align: right;
  }
  .nowrap {
    flex-wrap: nowrap;
    justify-content: flex-end;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
