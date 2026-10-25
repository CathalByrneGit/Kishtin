<script lang="ts">
  import { Button, IconButton, Modal, Notice, SelectDropdown, TextInput, Toggle } from "@kenn-io/kit-ui";
  import { ArrowDown, ArrowUp, Plus, Trash2 } from "@lucide/svelte";
  import { untrack } from "svelte";
  import { moved, type BookSetting, type Builder, type Column, type Template } from "../lib/recordbooks";

  interface Props {
    /** the bundle's booklet, or the added booklet as saved in the draft */
    base: Template;
    /** the restaurant's changes to a bundle booklet */
    setting?: BookSetting;
    /** an added booklet: every part is the restaurant's own */
    custom: boolean;
    builders: Builder[];
    /** a bundle booklet's changes */
    onsetting?: (s: BookSetting) => void;
    /** an added booklet, edited */
    ontemplate?: (t: Template) => void;
    onclose: () => void;
  }
  let { base, setting, custom, builders, onsetting, ontemplate, onclose }: Props = $props();

  interface Row {
    col: Column;
    widthText: string;
  }

  const builder = $derived(builders.find((b) => b.id === base.builder));
  // Text inputs bind to strings, so a column with no group heading gets "".
  function toRow(c: Column): Row {
    return { col: { ...c, group: c.group ?? "" }, widthText: String(Math.round(c.width * 10) / 10) };
  }

  // The editor starts from the booklet it was opened for; the page closes
  // and reopens it to edit another.
  const init = untrack(() => {
    const cols = setting?.columns?.length ? setting.columns : (base.columns ?? []);
    return {
      code: base.code,
      title: setting?.title || base.title,
      units: String(setting?.units_per_page || base.units_per_page || 6),
      managerCheck: setting?.manager_check ?? base.manager_check ?? false,
      notes: (custom ? base.notes : setting?.notes)?.join("\n") ?? "",
      rows: cols.map(toRow),
    };
  });
  let code = $state(init.code);
  let title = $state(init.title);
  let unitsText = $state(init.units);
  let managerCheck = $state(init.managerCheck);
  let notesText = $state(init.notes);
  let rows = $state<Row[]>(init.rows);

  const fieldOptions = $derived([
    ...(builder?.fields ?? []).map((f) => ({ value: f.key, label: f.label })),
    { value: "", label: "Blank: filled in by hand" },
  ]);
  const grid = $derived(base.builder === "unit_grid");

  const problem = $derived.by((): string => {
    if (custom && !code.trim()) return "Give the booklet a code, such as T4.";
    if (!title.trim()) return "Give the booklet a title.";
    if (rows.length === 0) return "A booklet needs at least one column.";
    if (rows.some((r) => !r.col.label.trim())) return "Every column needs a heading.";
    if (rows.some((r) => !(Number(r.widthText) > 0))) return "Every width must be a number above 0.";
    if (grid && rows.filter((r) => r.col.field === "unit_checks").length !== 1) return "A fridge and freezer grid needs exactly one readings column.";
    if (grid && !(Number.isInteger(Number(unitsText)) && Number(unitsText) >= 1 && Number(unitsText) <= 12)) return "Fridges and freezers per sheet must be a whole number from 1 to 12.";
    return "";
  });

  const columns = (): Column[] => rows.map((r) => ({ ...r.col, label: r.col.label.trim(), group: r.col.group?.trim() || undefined, field: r.col.field || undefined, width: Number(r.widthText) }));
  const notes = () => notesText.split("\n").map((n) => n.trim()).filter(Boolean);
  const sameColumns = (a: Column[], b: Column[]) =>
    JSON.stringify(a.map((c) => [c.label, c.group ?? "", c.field ?? "", c.fixed ?? "", c.width])) === JSON.stringify(b.map((c) => [c.label, c.group ?? "", c.field ?? "", c.fixed ?? "", c.width]));

  function done() {
    if (problem) return;
    const units = grid ? Number(unitsText) : undefined;
    if (custom) {
      ontemplate?.({ ...base, code: code.trim(), title: title.trim(), columns: columns(), notes: notes(), units_per_page: units, manager_check: managerCheck });
      return;
    }
    const cols = columns();
    onsetting?.({
      template: base.id,
      off: setting?.off,
      copy_file: setting?.copy_file,
      title: title.trim() !== base.title ? title.trim() : undefined,
      columns: sameColumns(cols, base.columns ?? []) ? undefined : cols,
      notes: notes().length ? notes() : undefined,
      units_per_page: grid && units !== base.units_per_page ? units : undefined,
      manager_check: managerCheck !== Boolean(base.manager_check) ? managerCheck : undefined,
    });
  }

  function resetColumns() {
    rows = (base.columns ?? []).map(toRow);
  }
</script>

<Modal title={custom ? `Edit ${base.code}` : `Change ${base.code}`} {onclose} maxWidth="920px">
  <div class="stack">
    {#if !custom}
      <p class="muted">Changes apply to how {base.code} prints for this restaurant. The bundle's own booklet stays as it is, and you can go back to it at any time.</p>
    {/if}
    <div class="two">
      {#if custom}
        <label class="field"><span>Code</span><TextInput bind:value={code} block placeholder="T4" /></label>
      {/if}
      <label class="field"><span>Title</span><TextInput bind:value={title} block placeholder={base.title} /></label>
      {#if grid}
        <label class="field"><span>Fridges and freezers per sheet</span><TextInput bind:value={unitsText} block /></label>
      {/if}
    </div>
    <Toggle checked={managerCheck} label="Print a manager or supervisor check line" onchange={(on) => (managerCheck = on)} />

    <div>
      <div class="cols-head">
        <strong>Columns</strong>
        <span class="muted small">{builder?.label}. Widths are shares of the page.</span>
      </div>
      <ol class="cols">
        {#each rows as r, i (i)}
          <li>
            <span class="num pos">{i + 1}</span>
            <TextInput bind:value={r.col.label} ariaLabel={`Column ${i + 1} heading`} placeholder="Heading" size="sm" />
            <TextInput bind:value={r.col.group} ariaLabel={`Column ${i + 1} group heading`} placeholder="Group heading (optional)" size="sm" />
            <SelectDropdown value={r.col.field ?? ""} options={fieldOptions} onchange={(v) => (r.col.field = v || undefined)} title={`Column ${i + 1} shows`} />
            <TextInput bind:value={r.widthText} ariaLabel={`Column ${i + 1} width`} size="sm" />
            <span class="moves">
              <IconButton ariaLabel={`Move column ${i + 1} up`} disabled={i === 0} onclick={() => (rows = moved(rows, i, -1))}><ArrowUp size={14} /></IconButton>
              <IconButton ariaLabel={`Move column ${i + 1} down`} disabled={i === rows.length - 1} onclick={() => (rows = moved(rows, i, 1))}><ArrowDown size={14} /></IconButton>
              <IconButton ariaLabel={`Remove column ${i + 1}`} onclick={() => rows.splice(i, 1)}><Trash2 size={14} /></IconButton>
            </span>
            {#if r.col.fixed}
              <span class="muted small fixed">Prints "{r.col.fixed}"{r.col.fixed_when ? " when that row has a value in " + r.col.fixed_when : ""}.</span>
            {/if}
          </li>
        {/each}
      </ol>
      <div class="row">
        <Button size="sm" label="Add a column" onclick={() => rows.push(toRow({ label: "", width: 20 }))}><Plus size={14} /></Button>
        {#if !custom}<Button size="sm" label="Use the bundle's columns" onclick={resetColumns} />{/if}
      </div>
    </div>

    <label class="field">
      <span>{custom ? "Notes printed under the table, one per line" : "Your notes, one per line. They print under the booklet's own notes."}</span>
      <textarea class="text-area" bind:value={notesText} aria-label="Notes" placeholder="For example: see our HACCP plan, section 4"></textarea>
    </label>
    {#if !custom && (base.notes?.length ?? 0) > 0}
      <div class="muted small">
        The booklet's own notes:
        <ul>{#each base.notes ?? [] as n (n)}<li>{n}</li>{/each}</ul>
      </div>
    {/if}
    {#if problem}<Notice tone="warning" message={problem} />{/if}
  </div>
  {#snippet footer()}
    <div class="row">
      <Button label="Cancel" onclick={onclose} />
      <Button tone="info" surface="solid" label="Done" disabled={Boolean(problem)} onclick={done} />
    </div>
  {/snippet}
</Modal>

<style>
  .two {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--space-4);
  }
  .cols-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-3);
    margin-bottom: var(--space-2);
  }
  .cols {
    list-style: none;
    margin: 0 0 var(--space-3);
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .cols li {
    display: grid;
    grid-template-columns: 24px minmax(120px, 1.4fr) minmax(110px, 1fr) minmax(160px, 1.6fr) 64px auto;
    gap: var(--space-2);
    align-items: center;
  }
  .pos {
    color: var(--text-secondary);
    text-align: right;
  }
  .moves {
    display: flex;
    gap: var(--space-1);
  }
  .fixed {
    grid-column: 2 / -1;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  .muted {
    color: var(--text-secondary);
  }
  ul {
    margin: var(--space-1) 0 0;
    padding-left: var(--space-6);
  }
  p {
    margin: 0;
  }
  @media (max-width: 760px) {
    .cols li {
      grid-template-columns: 24px minmax(0, 1fr);
    }
    .cols li > :global(*:not(.pos)) {
      grid-column: 2;
    }
  }
</style>
