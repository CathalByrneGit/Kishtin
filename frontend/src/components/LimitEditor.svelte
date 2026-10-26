<script lang="ts">
  import {
    Button,
    Checkbox,
    Chip,
    IconButton,
    Modal,
    Notice,
    SegmentedControl,
    SelectDropdown,
    Table, TableHeaderCell,
    TextInput,
  } from "@kenn-io/kit-ui";
  import { Plus, X } from "@lucide/svelte";
  import { untrack } from "svelte";
  import { api, message, type Lists, type Schemas } from "../lib/api";
  import { dublinTime, parseTemp, shortDate, dublinDate } from "../lib/format";
  import { CHECK_LABELS, limitWords, outcomeStyle } from "../lib/labels";

  interface Props {
    /** the limit to change, or null for a new one */
    start: Schemas["LimitVersion"] | null;
    lists: Lists;
    onsaved: () => void;
    onclose: () => void;
  }
  let { start, lists, onsaved, onclose }: Props = $props();

  // The form starts from the limit once; later prop changes do not reset it.
  const s0 = untrack(() => start);
  const str = (n: number | undefined | null) => (n === undefined || n === null ? "" : String(n));
  let check = $state(s0?.check ?? "fridge");
  let scopeKind = $state(s0?.scope.kind ?? "default");
  let scopeRef = $state(s0?.scope.ref ?? "");
  let maxC = $state(str(s0?.max_c));
  let borderline = $state(str(s0?.borderline_max_c));
  let minC = $state(str(s0?.min_c));
  let combos = $state((s0?.combos ?? [{ min_c: 75, hold_seconds: 0 }]).map((c) => ({ temp: String(c.min_c), mins: String(c.hold_seconds / 60) })));
  let fridgeWithin = $state(String((s0?.cooling?.into_fridge_within_seconds ?? 7200) / 60));
  let cutoffAfter = $state(String((s0?.cooling?.cutoff_after_seconds ?? 9000) / 60));
  let cutoffMax = $state(str(s0?.cooling?.cutoff_max_c ?? 10));
  let nonFood = $state(s0?.non_food ?? false);
  let reason = $state("");
  let planRef = $state("");

  let preview = $state<Schemas["LimitPreview"] | null>(null);
  let error = $state("");
  let busy = $state(false);

  const isUnitCheck = $derived(check === "fridge" || check === "freezer");
  $effect(() => {
    if (scopeKind === "unit" && !isUnitCheck) scopeKind = "default";
  });

  const scopeOptions = $derived([
    { value: "default", label: "Every one" },
    ...(isUnitCheck ? [{ value: "unit", label: "One unit" }] : []),
    { value: "category", label: "Food category" },
    { value: "dish", label: "Dish" },
  ]);

  const unitOptions = $derived(lists.units.filter((u) => u.kind === check).map((u) => ({ value: u.name, label: u.name })));
  const categoryOptions = $derived(lists.food_categories.map((c) => ({ value: c, label: c })));

  /** body builds the proposed limit, or says what is missing. */
  const body = $derived.by((): Schemas["NewLimit"] | string => {
    const nl: Schemas["NewLimit"] = { check, scope: { kind: scopeKind }, reason: reason.trim() };
    if (scopeKind !== "default") {
      if (!scopeRef.trim()) return "Choose what the limit applies to.";
      nl.scope.ref = scopeRef.trim();
    }
    const num = (s: string, what: string): number | string => parseTemp(s) ?? `${what} must be a number.`;
    if (check === "fridge" || check === "freezer" || check.startsWith("delivery_")) {
      const m = num(maxC, "The maximum");
      if (typeof m === "string") return m;
      nl.max_c = m;
    }
    if (check.startsWith("delivery_")) {
      const b = num(borderline, "The borderline maximum");
      if (typeof b === "string") return b;
      nl.borderline_max_c = b;
    }
    if (check === "hot_hold") {
      const m = num(minC, "The minimum");
      if (typeof m === "string") return m;
      nl.min_c = m;
    }
    if (check === "cook" || check === "reheat") {
      nl.combos = [];
      for (const c of combos) {
        const t = num(c.temp, "Each core temperature");
        if (typeof t === "string") return t;
        if (!/^\d+(\.\d+)?$/.test(c.mins.trim())) return "Each hold time must be a number of minutes (0 for instant).";
        nl.combos.push({ min_c: t, hold_seconds: Math.round(Number(c.mins) * 60) });
      }
      if (nl.combos.length === 0) return "Add at least one temperature.";
    }
    if (check === "cooling") {
      const cm = num(cutoffMax, "The cutoff temperature");
      if (typeof cm === "string") return cm;
      if (!/^\d+$/.test(fridgeWithin.trim()) || !/^\d+$/.test(cutoffAfter.trim())) return "Cooling times must be whole minutes.";
      nl.cooling = {
        into_fridge_within_seconds: Number(fridgeWithin) * 60,
        cutoff_after_seconds: Number(cutoffAfter) * 60,
        cutoff_max_c: cm,
      };
    }
    if (nonFood && scopeKind === "unit") nl.non_food = true;
    if (planRef.trim()) nl.plan_ref = planRef.trim();
    return nl;
  });

  // Any edit makes the last preview out of date.
  $effect(() => {
    void body;
    preview = null;
  });

  async function runPreview() {
    if (typeof body === "string") return;
    busy = true;
    try {
      preview = await api.previewLimit(body);
      error = "";
    } catch (err) {
      error = message(err);
    } finally {
      busy = false;
    }
  }

  async function save() {
    if (typeof body === "string") return;
    busy = true;
    try {
      await api.createLimit(body);
      onsaved();
    } catch (err) {
      error = message(err);
    } finally {
      busy = false;
    }
  }

  const changed = $derived(preview?.recent.filter((r) => r.changed).length ?? 0);
  const below = $derived([...(preview?.below_guidance ?? [])]);
  // FSAI warnings that say the same as a guidance warning are shown once.
  const belowFSAI = $derived((preview?.below_fsai ?? []).filter((b) => !below.includes(b)));
  // The preview is asked with whatever reason is typed; a missing reason
  // or plan reference is shown next to its field instead of blocking.
  const blocking = $derived(preview?.problem && !/needs a reason|plan section/.test(preview.problem) ? preview.problem : "");
  const needsPlan = $derived(below.length > 0 && !nonFood);
  const canSave = $derived(Boolean(preview) && !blocking && reason.trim() !== "" && (!needsPlan || planRef.trim() !== "") && !busy);
</script>

<Modal title={start ? "Change a limit" : "New limit"} {onclose} maxWidth="760px">
  <div class="form">
    <div class="two">
      <label class="field">
        <span>Check</span>
        <SelectDropdown
          value={check}
          options={Object.entries(CHECK_LABELS).map(([value, label]) => ({ value, label }))}
          onchange={(v) => {
            check = v;
            scopeRef = "";
          }}
          title="Check"
          disabled={Boolean(start)}
        />
      </label>
      <div class="field">
        <span>Applies to</span>
        <SegmentedControl
          value={scopeKind}
          options={scopeOptions}
          onchange={(v) => {
            scopeKind = v;
            scopeRef = "";
          }}
          ariaLabel="Applies to"
        />
      </div>
    </div>

    {#if scopeKind === "unit"}
      <label class="field">
        <span>Unit</span>
        <SelectDropdown value={scopeRef} options={[{ value: "", label: "Choose…" }, ...unitOptions]} onchange={(v) => (scopeRef = v)} title="Unit" />
      </label>
      <Checkbox bind:checked={nonFood} label="Non-food unit, such as a drinks fridge (needs only a reason to loosen)" />
    {:else if scopeKind === "category"}
      <label class="field">
        <span>Food category</span>
        <SelectDropdown value={scopeRef} options={[{ value: "", label: "Choose…" }, ...categoryOptions]} onchange={(v) => (scopeRef = v)} title="Food category" />
      </label>
    {:else if scopeKind === "dish"}
      <label class="field"><span>Dish</span><TextInput bind:value={scopeRef} placeholder="For example: chicken supreme" /></label>
    {/if}

    {#if check === "fridge" || check === "freezer" || check.startsWith("delivery_")}
      <div class="two">
        <label class="field"><span>Maximum (°C)</span><TextInput bind:value={maxC} placeholder="5.0" /></label>
        {#if check.startsWith("delivery_")}
          <label class="field"><span>Borderline up to (°C)</span><TextInput bind:value={borderline} placeholder="8.0" /></label>
        {/if}
      </div>
    {:else if check === "hot_hold"}
      <label class="field"><span>Minimum (°C)</span><TextInput bind:value={minC} placeholder="63.0" /></label>
    {:else if check === "cook" || check === "reheat"}
      <div class="field">
        <span>Passes if any of these is met</span>
        {#each combos as c, i (i)}
          <div class="combo">
            <TextInput bind:value={c.temp} ariaLabel="Core temperature (°C)" placeholder="75" />
            <span class="muted">°C held for</span>
            <TextInput bind:value={c.mins} ariaLabel="Minutes" placeholder="0" />
            <span class="muted">min</span>
            {#if combos.length > 1}
              <IconButton ariaLabel="Remove this combination" onclick={() => combos.splice(i, 1)}><X size={14} /></IconButton>
            {/if}
          </div>
        {/each}
        <div><Button size="sm" label="Add a time and temperature" onclick={() => combos.push({ temp: "", mins: "" })}><Plus size={14} /></Button></div>
      </div>
    {:else if check === "cooling"}
      <div class="three">
        <label class="field"><span>Into the fridge within (min)</span><TextInput bind:value={fridgeWithin} placeholder="120" /></label>
        <label class="field"><span>Cutoff after (min)</span><TextInput bind:value={cutoffAfter} placeholder="150" /></label>
        <label class="field"><span>Serve or discard above (°C)</span><TextInput bind:value={cutoffMax} placeholder="10.0" /></label>
      </div>
    {/if}

    <label class="field"><span>Reason for the change (required)</span><TextInput bind:value={reason} block /></label>
    <label class="field">
      <span>HACCP plan section or validation {needsPlan ? "(required: this is below your guidance)" : "(needed only if less strict than your guidance)"}</span>
      <TextInput bind:value={planRef} block />
    </label>

    {#if typeof body === "string"}
      <Notice tone="info" message={body} />
    {/if}
    {#if error}<Notice tone="error" message={error} />{/if}

    {#if preview}
      <section class="preview">
        <h3>What changes</h3>
        <p>
          {preview.replaces ? `Replaces: ${limitWords(preview.replaces)}.` : "A new limit at this level; nothing is replaced."}
          New: <strong>{limitWords(preview.limit)}</strong>.
        </p>
        {#each below as b (b)}<Notice tone="warning" title={`Below ${preview.guidance}`} message={b} />{/each}
        {#each belowFSAI as b (b)}<Notice tone="warning" title="Below FSAI guidance" message={b} />{/each}
        {#if blocking}<Notice tone="error" title="Cannot save" message={blocking} />{/if}
        <p class="muted">
          Records already saved keep their result.
          {preview.recent.length === 0
            ? "There are no recent records this limit would apply to."
            : `Of the last ${preview.recent.length} records it would apply to, ${changed} would have scored differently.`}
        </p>
        {#if preview.recent.length > 0}
          <div class="regrade">
            <Table unsorted ariaLabel="Recent records under the new limit" stickyHeader>
              {#snippet header()}
                <TableHeaderCell label="When" />
                <TableHeaderCell label="Record" />
                <TableHeaderCell label="Saved as" />
                <TableHeaderCell label="Would be" />
              {/snippet}
              {#each preview.recent as r (r.record_id)}
                {@const a = outcomeStyle(r.saved)}
                {@const b = outcomeStyle(r.proposed)}
                <tr class:changed={r.changed}>
                  <td class="num nowrap">{shortDate(dublinDate(new Date(r.logged_at)))}, {dublinTime(r.logged_at)}</td>
                  <td>{r.subject}</td>
                  <td><Chip size="sm" tone={a.tone} uppercase={false}>{a.label}</Chip></td>
                  <td><Chip size="sm" tone={b.tone} uppercase={false}>{b.label}</Chip>{#if r.changed}<span class="muted"> changed</span>{/if}</td>
                </tr>
              {/each}
            </Table>
          </div>
        {/if}
      </section>
    {/if}
  </div>

  {#snippet footer()}
    <div class="row">
      <Button label="Cancel" onclick={onclose} />
      {#if !preview}
        <Button tone="info" surface="solid" label={busy ? "Checking…" : "Preview the change"} disabled={busy || typeof body === "string"} onclick={runPreview} />
      {:else}
        <Button tone="success" surface="solid" label={busy ? "Saving…" : "Save limit"} disabled={!canSave} onclick={save} />
      {/if}
    </div>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-5);
  }
  .three {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: var(--space-5);
  }
  .combo {
    display: grid;
    grid-template-columns: 100px auto 100px auto auto;
    gap: var(--space-3);
    align-items: center;
    justify-content: start;
  }
  .preview {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding-top: var(--space-5);
    border-top: var(--border-width) solid var(--border-muted);
  }
  h3 {
    font-size: var(--font-size-md);
  }
  p {
    margin: 0;
  }
  .regrade {
    max-height: 260px;
    overflow-y: auto;
  }
  .changed td {
    font-weight: var(--font-weight-semibold);
  }
  .nowrap {
    white-space: nowrap;
  }
</style>
