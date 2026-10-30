<script lang="ts">
  import { Button, Checkbox, IconButton, Modal, Notice, SelectDropdown, TextInput, Toggle } from "@kenn-io/kit-ui";
  import { Plus, X } from "@lucide/svelte";
  import { untrack } from "svelte";
  import { api, message, type Dish, type Schemas, type Setup } from "../lib/api";
  import { COURSES, courseLabel, durationSeconds, minutesText } from "../lib/kitchen";

  interface Props {
    menu: Setup;
    /** the dish to change, or null for a new one */
    dish: Dish | null;
    categories: string[];
    onsaved: (d: Dish) => void;
    onclose: () => void;
  }
  let { menu, dish, categories, onsaved, onclose }: Props = $props();

  const d0 = untrack(() => dish);
  let name = $state(d0?.name ?? "");
  let course = $state<string>(d0?.course ?? "main");
  let stationId = $state(d0?.station_id ?? untrack(() => menu.stations.find((s) => s.active)?.id) ?? "");
  let equipmentId = $state(d0?.equipment_id ?? "");
  let capacity = $state(d0?.batch_capacity ? String(d0.batch_capacity) : "");
  let cook = $state(minutesText(d0?.cook_seconds));
  let rest = $state(minutesText(d0?.rest_seconds));
  let plate = $state(minutesText(d0?.plate_seconds));
  let hold = $state(minutesText(d0?.hold_seconds));
  let batchable = $state(d0?.batchable ?? false);
  let category = $state(d0?.food_category ?? "");
  let active = $state(d0?.active ?? true);
  let allergens = $state<string[]>([...(d0?.allergens ?? [])]);
  let allergensChecked = $state(d0?.allergens_checked ?? false);
  let allergenList = $state<Schemas["Allergen"][]>([]);
  void api
    .allergens()
    .then((c) => (allergenList = c.allergens))
    .catch(() => {});

  function toggleAllergen(code: string, on: boolean) {
    allergens = on ? [...allergens, code] : allergens.filter((a) => a !== code);
    // Choosing allergens is checking them.
    allergensChecked = true;
  }
  let mods = $state((d0?.modifiers ?? []).map((m) => ({ name: m.name, text: (m.add_seconds < 0 ? "-" : "") + minutesText(Math.abs(m.add_seconds)) })));
  let error = $state("");
  let busy = $state(false);

  const equipment = $derived(menu.equipment.filter((e) => e.station_id === stationId && (e.active || e.id === equipmentId)));
  $effect(() => {
    if (equipmentId && !equipment.some((e) => e.id === equipmentId)) equipmentId = "";
  });
  const slots = $derived(menu.equipment.find((e) => e.id === equipmentId)?.slots ?? 0);

  /** body builds the dish, or says what is wrong. */
  const body = $derived.by((): Schemas["DishInput"] | string => {
    if (!name.trim()) return "Enter the dish's name.";
    if (!stationId) return "Choose a station.";
    const times: Record<string, number | null> = {
      cook: durationSeconds(cook), rest: durationSeconds(rest), plate: durationSeconds(plate), hold: durationSeconds(hold),
    };
    for (const [k, v] of Object.entries(times)) if (v === null) return `The ${k} time must be minutes, such as 8 or 1:30.`;
    const cap = capacity.trim() === "" ? 0 : Number(capacity);
    if (!Number.isInteger(cap) || cap < 0) return "Cook at once must be a whole number.";
    const modifiers: Schemas["Modifier"][] = [];
    for (const m of mods) {
      if (!m.name.trim()) return "Every modifier needs a name.";
      const neg = m.text.trim().startsWith("-");
      const secs = durationSeconds(m.text.trim().replace(/^[-+]/, ""));
      if (secs === null) return `${m.name}: extra cook time must be minutes, such as 2 or -1.`;
      modifiers.push({ name: m.name.trim(), add_seconds: neg ? -secs : secs });
    }
    return {
      name: name.trim(), course, station_id: stationId, equipment_id: equipmentId || undefined,
      batch_capacity: equipmentId ? cap : undefined, cook_seconds: times.cook ?? 0, rest_seconds: times.rest ?? 0,
      plate_seconds: times.plate ?? 0, hold_seconds: times.hold ?? 0, batchable, modifiers,
      food_category: category || undefined, active: d0 ? active : undefined,
      allergens: allergenList.length > 0 ? allergens : undefined, allergens_checked: allergenList.length > 0 ? allergensChecked : undefined,
    };
  });

  async function save() {
    if (typeof body === "string") return;
    busy = true;
    try {
      onsaved(d0 ? await api.updateDish(d0.id, body) : await api.createDish(body));
    } catch (err) {
      error = message(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={d0 ? `Change ${d0.name}` : "New dish"} {onclose} maxWidth="720px">
  <div class="form">
    <div class="two">
      <label class="field"><span>Name</span><TextInput bind:value={name} block autofocus /></label>
      <label class="field">
        <span>Course</span>
        <SelectDropdown value={course} options={COURSES.map((c) => ({ value: c, label: courseLabel(c, false) }))} onchange={(v) => (course = v)} title="Course" />
      </label>
    </div>
    <div class="three">
      <label class="field">
        <span>Station</span>
        <SelectDropdown
          value={stationId}
          options={menu.stations.filter((s) => s.active || s.id === stationId).map((s) => ({ value: s.id, label: s.name }))}
          onchange={(v) => (stationId = v)}
          title="Station"
        />
      </label>
      <label class="field">
        <span>Cooks on</span>
        <SelectDropdown
          value={equipmentId}
          options={[{ value: "", label: "No equipment" }, ...equipment.map((e) => ({ value: e.id, label: `${e.name} (${e.slots})` }))]}
          onchange={(v) => (equipmentId = v)}
          title="Equipment"
        />
      </label>
      <label class="field">
        <span>Cook at once {slots ? `(blank: ${slots})` : ""}</span>
        <TextInput bind:value={capacity} disabled={!equipmentId} placeholder={slots ? String(slots) : ""} block />
      </label>
    </div>
    <div class="four">
      <label class="field"><span>Cook (min)</span><TextInput bind:value={cook} placeholder="8" block /></label>
      <label class="field"><span>Rest (min)</span><TextInput bind:value={rest} placeholder="0" block /></label>
      <label class="field"><span>Plate (min)</span><TextInput bind:value={plate} placeholder="1" block /></label>
      <label class="field"><span>Can wait (min)</span><TextInput bind:value={hold} placeholder="3" block /></label>
    </div>
    <p class="muted small">Minutes, or minutes:seconds such as 1:30. "Can wait" is how long the dish keeps at the pass before quality drops; 0 means it must go straight out.</p>
    <Checkbox bind:checked={batchable} label="Can be cooked for several tables at once" />
    <label class="field">
      <span>HACCP food category (sets its cooking limit)</span>
      {#if categories.length > 0}
        <SelectDropdown value={category} options={[{ value: "", label: "None" }, ...categories.map((c) => ({ value: c, label: c }))]} onchange={(v) => (category = v)} title="Food category" />
      {:else}
        <TextInput bind:value={category} placeholder="Add categories in Fridges and tasks" disabled block />
      {/if}
    </label>

    <div class="field">
      <span>Modifiers that change the cook time</span>
      {#each mods as m, i (i)}
        <div class="mod">
          <TextInput bind:value={m.name} placeholder="Well done" ariaLabel="Modifier name" block />
          <TextInput bind:value={m.text} placeholder="2 or -1" ariaLabel="Extra cook minutes" block />
          <span class="muted small">min</span>
          <IconButton ariaLabel={`Remove ${m.name || "modifier"}`} size="sm" onclick={() => mods.splice(i, 1)}><X size={14} /></IconButton>
        </div>
      {/each}
      <div><Button size="sm" label="Add a modifier" onclick={() => mods.push({ name: "", text: "" })}><Plus size={14} /></Button></div>
    </div>

    <div class="field">
      <span>Allergens it contains</span>
      <div class="allergens">
        {#each allergenList as a (a.code)}
          <Checkbox checked={allergens.includes(a.code)} label={a.name} onchange={(on) => toggleAllergen(a.code, on)} />
        {/each}
      </div>
      <Checkbox bind:checked={allergensChecked} label="These allergens are checked, including that the dish has none of the others" />
      {#if !allergensChecked}<span class="muted small">Unchecked, the allergen chart shows “Not checked”: customers must not be told it is safe.</span>{/if}
    </div>

    {#if d0}
      <Toggle checked={active} label="On the menu" onchange={(on) => (active = on)} />
    {/if}

    {#if typeof body === "string"}<Notice tone="info" message={body} />{/if}
    {#if error}<Notice tone="error" message={error} />{/if}
  </div>
  {#snippet footer()}
    <div class="row">
      <Button label="Cancel" onclick={onclose} />
      <Button tone="success" surface="solid" label={busy ? "Saving…" : d0 ? "Save dish" : "Add dish"} disabled={busy || typeof body === "string"} onclick={save} />
    </div>
  {/snippet}
</Modal>

<style>
  .allergens {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: var(--space-2) var(--space-4);
    margin-bottom: var(--space-3);
  }
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  .two {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
    gap: var(--space-4);
  }
  .three {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-4);
  }
  .four {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--space-4);
  }
  .mod {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 120px auto auto;
    gap: var(--space-3);
    align-items: center;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
