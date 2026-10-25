<script lang="ts">
  import { Button, Card, Chip, IconButton, Notice, SelectDropdown, Spinner, Table, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { Camera, Plus, Trash2 } from "@lucide/svelte";
  import { onMount } from "svelte";
  import { api, message, type Lists, type Schemas } from "../lib/api";
  import { dublinDate, dublinTime } from "../lib/format";
  import { emptyRow, fromRecord, PAPER_KINDS, toRecord, type PaperRow } from "../lib/paper";
  import { session } from "../lib/session.svelte";

  let lists = $state<Lists | null>(null);
  let ai = $state(false);
  let photo = $state<{ sha: string; url: string; type: string; name: string } | null>(null);
  let uploading = $state(false);
  let fileInput = $state<HTMLInputElement | undefined>();
  let rows = $state<PaperRow[]>([emptyRow()]);
  let checks = $state<Schemas["PaperCheck"][] | null>(null);
  let checkedFor = $state("");
  let reading = $state(false);
  let busy = $state(false);
  let readProblems = $state<string[]>([]);

  const canImport = $derived(session.user?.role === "manager" || session.user?.role === "head_chef");

  onMount(async () => {
    if (!canImport) return;
    try {
      lists = (await api.lists()).value;
      ai = (await api.recordBooks()).ai;
    } catch (err) {
      showFlash(message(err), { tone: "danger" });
    }
  });

  async function pick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file) return;
    uploading = true;
    try {
      const m = await api.upload(file);
      const f = await api.file(m.sha256);
      if (photo) URL.revokeObjectURL(photo.url);
      photo = { sha: m.sha256, url: f.url, type: f.type, name: file.name };
      checks = null;
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      uploading = false;
    }
  }

  async function readPhoto() {
    if (!photo) return;
    reading = true;
    try {
      const draft = await api.readPaper(photo.sha);
      rows = draft.rows.length ? draft.rows.map((r) => fromRecord(r, dublinDate, dublinTime)) : [emptyRow()];
      readProblems = draft.problems;
      checks = null;
      showFlash(`The local model read ${draft.rows.length} rows. Check each one against the photo.`, { tone: "info", durationMs: 8000 });
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      reading = false;
    }
  }

  const records = $derived(rows.map(toRecord));
  const typed = $derived(records.filter((r) => typeof r !== "string").length);
  const key = $derived(JSON.stringify(rows) + (photo?.sha ?? ""));
  const ready = $derived(checks !== null && checkedFor === key && checks.every((c) => !c.problem));

  async function check() {
    if (!photo) return;
    const bad = records.findIndex((r) => typeof r === "string");
    if (bad >= 0) {
      showFlash(`Row ${bad + 1}: ${records[bad]}`, { tone: "danger", durationMs: 6000 });
      return;
    }
    busy = true;
    try {
      checks = (await api.previewPaper({ file: photo.sha, rows: records as Schemas["NewRecord"][] })).rows;
      checkedFor = key;
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  async function save() {
    if (!photo || !ready) return;
    busy = true;
    try {
      const res = await api.importPaper({ file: photo.sha, rows: records as Schemas["NewRecord"][] });
      showFlash(`${res.records.length} records saved from the photo`, { tone: "success" });
      rows = [emptyRow(rows.at(-1))];
      checks = null;
      readProblems = [];
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  const unitOptions = $derived((lists?.units ?? []).map((u) => ({ value: u.name, label: u.name })));
  const outcome = (c: Schemas["PaperCheck"] | undefined) => c?.check?.result.outcome;
  const tone = (o: string | undefined) => (o === "pass" ? "success" : o === "fail" ? "danger" : o === "borderline" ? "warning" : "neutral");
  const words: Record<string, string> = { pass: "Pass", fail: "Fail", borderline: "Check", recorded: "Recorded" };
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Paper import</h1>
      <p>Bring old paper record book pages into Kishtin. Photograph the page, type its rows next to the photo, check them, then save. Each record keeps the photo, when the check was done and the name on the paper.</p>
    </div>
  </header>

  {#if !canImport}
    <Notice tone="info" title="Head chef or manager only" message="Paper import is for the head chef and managers." />
  {:else}
    <div class="grid">
      <Card padding="md" title="Photo of the page">
        <div class="stack">
          <input bind:this={fileInput} class="kit-sr-only" type="file" accept="image/*,application/pdf" capture="environment" onchange={pick} />
          <div class="row">
            <Button label={uploading ? "Uploading…" : photo ? "Use another photo" : "Add a photo"} disabled={uploading} onclick={() => fileInput?.click()}><Camera size={16} /></Button>
            {#if photo && ai}
              <Button label={reading ? "Reading…" : "Read the photo"} disabled={reading} onclick={readPhoto} />
            {/if}
          </div>
          {#if photo?.type.startsWith("image/")}
            <a href={photo.url} target="_blank" rel="noopener"><img src={photo.url} alt="The paper page" /></a>
          {:else if photo}
            <a href={photo.url} target="_blank" rel="noopener">Open {photo.name}</a>
          {:else}
            <p class="muted">A clear, flat photo in good light, one page at a time.</p>
          {/if}
          {#if ai}
            <p class="muted small">"Read the photo" asks the local model on this machine or your network to fill in the rows. Check every row against the photo: it can misread handwriting.</p>
          {:else}
            <p class="muted small">Type the rows by hand. A local model can read photos for you: set [ai] url in kishtin.toml.</p>
          {/if}
          {#each readProblems as p (p)}<Notice tone="warning" message={p} />{/each}
        </div>
      </Card>

      <Card padding="md" title="Rows" meta={`${typed} of ${rows.length} filled in`}>
        <div class="stack">
          <p class="muted small">Temperature readings from fridge, freezer, cooking, reheating and hot holding pages. Copy the action written on the paper for any reading out of limit.</p>
          <div class="scroll">
            <Table bare unsorted class="rows" ariaLabel="Rows from the paper page">
              <thead>
                <tr><th class="date">Date</th><th class="time">Time</th><th class="kind">Kind</th><th>Fridge or food</th><th class="temp">°C</th><th class="signed">Signed</th><th>Action</th><th class="result">Result</th><th class="remove"><span class="kit-sr-only">Remove</span></th></tr>
              </thead>
              <tbody>
                {#each rows as r, i (i)}
                  {@const c = checks && checkedFor === key ? checks[i] : undefined}
                  <tr>
                    <td><TextInput bind:value={r.date} block size="sm" placeholder="2026-04-28" ariaLabel={`Row ${i + 1} date`} /></td>
                    <td><TextInput bind:value={r.time} block size="sm" placeholder="09:00" ariaLabel={`Row ${i + 1} time`} /></td>
                    <td><SelectDropdown value={r.type} options={PAPER_KINDS} onchange={(v) => (r.type = v)} title={`Row ${i + 1} kind`} /></td>
                    <td>
                      {#if r.type === "fridge" || r.type === "freezer"}
                        <SelectDropdown value={r.what} options={unitOptions} onchange={(v) => (r.what = v)} title={`Row ${i + 1} fridge or freezer`} />
                      {:else}
                        <TextInput bind:value={r.what} block size="sm" placeholder="Food" ariaLabel={`Row ${i + 1} food`} />
                      {/if}
                    </td>
                    <td><TextInput bind:value={r.temp} block size="sm" ariaLabel={`Row ${i + 1} temperature`} /></td>
                    <td><TextInput bind:value={r.signedBy} block size="sm" placeholder="As written" ariaLabel={`Row ${i + 1} signed`} /></td>
                    <td><TextInput bind:value={r.action} block size="sm" ariaLabel={`Row ${i + 1} action`} /></td>
                    <td>
                      {#if c}
                        <Chip uppercase={false} tone={c.problem ? "danger" : tone(outcome(c))}>{c.problem ? "Fix" : (words[outcome(c) ?? ""] ?? "")}</Chip>
                      {/if}
                    </td>
                    <td><IconButton ariaLabel={`Remove row ${i + 1}`} onclick={() => (rows = rows.length > 1 ? rows.filter((_, j) => j !== i) : [emptyRow()])}><Trash2 size={14} /></IconButton></td>
                  </tr>
                  {#if c?.problem}
                    <tr><td colspan="9" class="problem">Row {i + 1}: {c.problem}</td></tr>
                  {/if}
                {/each}
              </tbody>
            </Table>
          </div>
          <div class="row">
            <Button size="sm" label="Add a row" onclick={() => rows.push(emptyRow(rows.at(-1)))}><Plus size={14} /></Button>
          </div>
        </div>
      </Card>
    </div>

    <div class="savebar">
      <span class="muted">
        {#if !photo}Add the photo first: every record keeps it.{:else if ready}Every row checks out. Saving adds {rows.length} records, logged by {session.user?.name}.{:else}Check the rows to see each result before saving.{/if}
      </span>
      <div class="row">
        <Button label={busy && !ready ? "Checking…" : "Check rows"} disabled={!photo || busy} onclick={check} />
        <Button tone="success" surface="solid" label={busy && ready ? "Saving…" : `Save ${rows.length} records`} disabled={!ready || busy} onclick={save} />
      </div>
    </div>
  {/if}
</div>

{#if uploading}<Spinner />{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: minmax(220px, 1fr) minmax(0, 4fr);
    gap: var(--space-5);
    align-items: start;
  }
  img {
    width: 100%;
    border-radius: var(--radius-md);
    border: var(--border-width) solid var(--border-default);
  }
  .scroll {
    overflow-x: auto;
  }
  :global(table.rows) {
    border-collapse: collapse;
    table-layout: fixed;
    width: 100%;
    min-width: 820px;
  }
  :global(table.rows) th {
    text-align: left;
    font-size: var(--font-size-sm);
    color: var(--text-secondary);
    padding: var(--space-1);
  }
  :global(table.rows) td {
    padding: var(--space-1);
    vertical-align: middle;
  }
  :global(table.rows) td :global(.kit-select-dropdown) {
    min-width: 0;
  }
  .date {
    width: 112px;
  }
  .time {
    width: 68px;
  }
  .kind {
    width: 136px;
  }
  .temp {
    width: 64px;
  }
  .signed {
    width: 96px;
  }
  .result {
    width: 84px;
  }
  .remove {
    width: 40px;
  }
  .problem {
    color: var(--accent-red);
    font-size: var(--font-size-sm);
  }
  .savebar {
    position: sticky;
    bottom: var(--space-4);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-4);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  .muted {
    color: var(--text-secondary);
  }
  p {
    margin: 0;
  }
  @media (max-width: 900px) {
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
