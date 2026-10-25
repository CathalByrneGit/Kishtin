<script lang="ts">
  import { Button, Card, Chip, Modal, Notice, SelectDropdown, Spinner, TextInput, Toggle, showFlash } from "@kenn-io/kit-ui";
  import { ExternalLink, Pencil, Plus, Trash2, Upload } from "@lucide/svelte";
  import { onMount } from "svelte";
  import BookletEditor from "../../components/BookletEditor.svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import ReviewChanges, { type Preview } from "../../components/ReviewChanges.svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { onChange } from "../../lib/live.svelte";
  import { navigate } from "../../lib/router.svelte";
  import {
    copyStatus,
    draftBooklets,
    newCustom,
    switchBundle,
    withSetting,
    type BookSetting,
    type DraftBooklet,
    type Settings,
    type Template,
  } from "../../lib/recordbooks";
  import { isManagerPassword } from "../../lib/session.svelte";

  let info = $state<Schemas["RecordBooks"] | null>(null);
  let saved = $state<Schemas["VersionedRecordBookSettings"] | null>(null);
  let draft = $state<Settings | null>(null);
  let error = $state("");
  let editing = $state<DraftBooklet | null>(null);
  let adding = $state<{ code: string; title: string; builder: string; from: string } | null>(null);
  let uploadFor = $state<DraftBooklet | null>(null);
  let uploading = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | undefined>();
  let review = $state<{ preview: Preview; value: Settings } | null>(null);
  let saving = $state(false);
  const canEdit = $derived(isManagerPassword());

  async function load() {
    try {
      const [i, s] = await Promise.all([api.recordBooks(), api.recordBookSettings()]);
      info = i;
      saved = s;
      draft = structuredClone($state.snapshot(s.value)) as Settings;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((k) => {
      if (k === "settings" && !dirty) void load();
    });
  });

  const bundle = $derived(info?.bundles.find((b) => b.id === draft?.bundle));
  const booklets = $derived(draft ? draftBooklets(bundle, draft) : []);
  const norm = (s: Settings) => JSON.stringify({ bundle: s.bundle, books: [...(s.books ?? [])].sort((a, b) => a.template.localeCompare(b.template)), custom: s.custom ?? [] });
  const dirty = $derived(Boolean(saved && draft && norm(draft) !== norm(saved.value)));
  const bundleOptions = $derived((info?.bundles ?? []).map((b) => ({ value: b.id, label: `${b.country}: ${b.name}` })));

  function set(next: Settings) {
    draft = next;
  }

  function toggle(b: DraftBooklet, on: boolean) {
    if (!draft) return;
    set(withSetting(draft, { ...(b.setting ?? { template: b.base.id }), off: on ? undefined : true }));
  }

  function chooseBundle(id: string) {
    if (!draft || id === draft.bundle) return;
    set(switchBundle(draft, id));
  }

  function pickCopy(b: DraftBooklet) {
    uploadFor = b;
    fileInput?.click();
  }

  async function uploaded(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    const b = uploadFor;
    input.value = "";
    if (!file || !b || !draft) return;
    uploading = b.base.id;
    try {
      const m = await api.upload(file);
      if (m.mime !== "application/pdf") {
        showFlash("Upload the booklet as a PDF.", { tone: "danger", durationMs: 6000 });
        return;
      }
      set(withSetting(draft, { ...(b.setting ?? { template: b.base.id }), copy_file: m.sha256 }));
      const st = copyStatus(b.base, m.sha256);
      showFlash(st?.text ?? "Copy uploaded", { tone: st?.tone === "warning" ? "warning" : "success", durationMs: 8000 });
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      uploading = null;
    }
  }

  function saveSetting(s: BookSetting) {
    if (draft) set(withSetting(draft, s));
    editing = null;
  }

  function saveCustom(t: Template) {
    if (!draft) return;
    set({ ...draft, custom: (draft.custom ?? []).map((c) => (c.id === t.id ? t : c)) });
    editing = null;
  }

  function removeCustom(b: DraftBooklet) {
    if (!draft) return;
    set({ ...draft, custom: (draft.custom ?? []).filter((c) => c.id !== b.base.id), books: (draft.books ?? []).filter((x) => x.template !== b.base.id) });
  }

  const startFrom = $derived(
    adding ? booklets.filter((b) => b.template.builder === adding!.builder).map((b) => ({ value: b.template.id, label: `${b.template.code} ${b.template.title}` })) : [],
  );

  function openAdd() {
    const first = info?.builders[0]?.id ?? "unit_grid";
    adding = { code: "", title: "", builder: first, from: booklets.find((b) => b.template.builder === first)?.template.id ?? "" };
  }

  function add() {
    if (!draft || !adding || !adding.code.trim() || !adding.title.trim()) return;
    const from = booklets.find((b) => b.template.id === adding!.from)?.template;
    const t = newCustom(adding.code, adding.title, adding.builder, from, draft);
    set({ ...draft, custom: [...(draft.custom ?? []), t] });
    adding = null;
    editing = draftBooklets(bundle, { ...draft }).find((b) => b.base.id === t.id) ?? null;
  }

  async function reviewChanges() {
    if (!draft) return;
    const value = $state.snapshot(draft) as Settings;
    try {
      review = { preview: await api.previewRecordBookSettings({ value, reason: "" }), value };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function save(reason: string) {
    if (!review) return;
    saving = true;
    try {
      await api.saveRecordBookSettings({ value: review.value, reason });
      review = null;
      showFlash("Saved: record books use the new settings", { tone: "success" });
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
      <h1>HACCP records</h1>
      <p>Choose the record books for your country, then change what fits your kitchen. Changes are versioned and only change how records print, never the records.</p>
    </div>
    <Button label="Open Record books" onclick={() => navigate("/books")} />
  </header>

  <ManagerGate what="HACCP records" viewable>
    {#if error}
      <Notice tone="error" message={error} />
    {:else if !info || !draft || !saved}
      <Spinner />
    {:else}
      <Card padding="md" title="Bundle" meta={saved.builtin ? "Kishtin default" : `Version ${saved.version.id}`}>
        <div class="stack">
          <label class="field">
            <span>Record books for</span>
            <SelectDropdown value={draft.bundle} options={bundleOptions} onchange={chooseBundle} title="Bundle" disabled={!canEdit} />
          </label>
          {#if bundle}
            <p class="muted">
              {bundle.templates.length} booklets from {bundle.issuer}; Kishtin can fill in {bundle.templates.filter((t) => t.builder).length} of them.
              {#if bundle.url}<a href={bundle.url} target="_blank" rel="noreferrer">Official booklets <ExternalLink size={12} /></a>{/if}
            </p>
          {/if}
          <p class="muted small">
            No bundle for your country yet? Choose "Any country", then change titles and columns to match your own booklets, or add a booklet. Bundles for more countries are added to Kishtin as kitchens ask for them.
          </p>
        </div>
      </Card>

      <Card padding="md" title="Booklets">
        <ul class="books">
          {#each booklets as b (b.base.id)}
            {@const copy = copyStatus(b.base, b.setting?.copy_file)}
            <li class:off={b.off}>
              <span class="code num">{b.template.code}</span>
              <div class="about">
                <span class="title">{b.template.title}</span>
                <span class="chips">
                  {#if !b.template.builder}
                    <Chip uppercase={false} tone="neutral">Kishtin cannot fill this yet</Chip>
                  {:else if b.off}
                    <Chip uppercase={false} tone="neutral">Off</Chip>
                  {:else}
                    <Chip uppercase={false} dot tone="success">In use</Chip>
                  {/if}
                  {#if b.custom}<Chip uppercase={false} tone="info">Added by you</Chip>{/if}
                  {#if b.changed.length > 0}<Chip uppercase={false} tone="info">Changed: {b.changed.join(", ")}</Chip>{/if}
                  {#if b.base.source}<span class="muted small">Edition {b.base.source.published}</span>{/if}
                </span>
                {#if !b.template.builder && b.template.missing}<span class="muted small">{b.template.missing}</span>{/if}
                {#if copy}
                  <span class="small copy {copy.tone}">{copy.tone === "success" ? "Matches: " : copy.tone === "warning" ? "Check: " : ""}{copy.text}</span>
                {/if}
              </div>
              <div class="actions">
                <Toggle checked={!b.off} label="In use" disabled={!canEdit} onchange={(on) => toggle(b, on)} />
                {#if b.template.builder}
                  <Button size="sm" label="Change" disabled={!canEdit} onclick={() => (editing = b)}><Pencil size={14} /></Button>
                {/if}
                {#if !b.custom || b.setting?.copy_file}
                  <Button size="sm" label={uploading === b.base.id ? "Uploading…" : b.setting?.copy_file ? "Replace your copy" : "Upload your copy"} disabled={!canEdit || uploading !== null} onclick={() => pickCopy(b)}>
                    <Upload size={14} />
                  </Button>
                {/if}
                {#if b.custom}
                  <Button size="sm" tone="danger" label="Remove" disabled={!canEdit} onclick={() => removeCustom(b)}><Trash2 size={14} /></Button>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
        <div class="row add">
          <Button label="Add a booklet" disabled={!canEdit} onclick={openAdd}><Plus size={16} /></Button>
          <span class="muted small">For a booklet your country uses that is not in the bundle. It can show anything Kishtin records for that kind of booklet.</span>
        </div>
        <input bind:this={fileInput} class="kit-sr-only" type="file" accept="application/pdf" onchange={uploaded} />
        <p class="muted small">
          Uploading your copy of an official booklet keeps it with your records, and Kishtin checks whether it is the edition the bundle was made from. If it is not, compare its columns and change them here.
        </p>
      </Card>

      {#if dirty}
        <div class="savebar">
          <Notice tone="info" title="Changes not saved yet" message="Review them before saving. Nothing changes on Record books until you save." />
          <div class="row">
            <Button label="Discard" onclick={() => (draft = structuredClone($state.snapshot(saved!.value)) as Settings)} />
            <Button tone="success" surface="solid" label="Review and save" onclick={reviewChanges} />
          </div>
        </div>
      {/if}
    {/if}
  </ManagerGate>
</div>

{#if editing && info}
  <BookletEditor
    base={editing.base}
    setting={editing.setting}
    custom={editing.custom}
    builders={info.builders}
    onsetting={saveSetting}
    ontemplate={saveCustom}
    onclose={() => (editing = null)}
  />
{/if}

{#if adding && info}
  <Modal title="Add a booklet" onclose={() => (adding = null)} maxWidth="560px">
    <div class="stack">
      <div class="two">
        <label class="field"><span>Code</span><TextInput bind:value={adding.code} block placeholder="T4" autofocus /></label>
        <label class="field"><span>Title</span><TextInput bind:value={adding.title} block placeholder="Relevés de température" /></label>
      </div>
      <label class="field">
        <span>What it records</span>
        <SelectDropdown
          value={adding.builder}
          options={info.builders.map((b) => ({ value: b.id, label: b.label }))}
          onchange={(v) => adding && ((adding.builder = v), (adding.from = booklets.find((b) => b.template.builder === v)?.template.id ?? ""))}
          title="What it records"
        />
      </label>
      {#if startFrom.length > 0}
        <label class="field">
          <span>Start with the columns of</span>
          <SelectDropdown value={adding.from} options={startFrom} onchange={(v) => adding && (adding.from = v)} title="Start with the columns of" />
        </label>
      {/if}
    </div>
    {#snippet footer()}
      <div class="row">
        <Button label="Cancel" onclick={() => (adding = null)} />
        <Button tone="info" surface="solid" label="Add and set up columns" disabled={!adding?.code.trim() || !adding?.title.trim()} onclick={add} />
      </div>
    {/snippet}
  </Modal>
{/if}

{#if review}
  <ReviewChanges title="Save record book changes" preview={review.preview} busy={saving} onsave={save} onclose={() => (review = null)} />
{/if}

<style>
  .books {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .books li {
    display: grid;
    grid-template-columns: 56px minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) 0;
    border-top: var(--border-width) solid var(--border-default);
  }
  .books li:first-child {
    border-top: none;
  }
  .off .title,
  .off .code {
    color: var(--text-secondary);
  }
  .code {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-bold);
  }
  .about {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  .title {
    font-weight: var(--font-weight-semibold);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-3);
  }
  .copy.success {
    color: var(--accent-green);
  }
  .copy.warning {
    color: var(--text-primary);
    font-weight: var(--font-weight-semibold);
  }
  .add {
    margin: var(--space-4) 0;
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: var(--space-4);
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
  .muted {
    color: var(--text-secondary);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
  a {
    color: var(--accent-blue);
  }
  @media (max-width: 900px) {
    .books li {
      grid-template-columns: 48px minmax(0, 1fr);
    }
    .actions {
      grid-column: 2;
      justify-content: flex-start;
    }
    .two {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
