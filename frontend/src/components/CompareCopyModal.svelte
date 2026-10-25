<script lang="ts">
  import { Button, Chip, MetricCard, Modal, Notice, Spinner, Table } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../lib/api";

  interface Props {
    book: string;
    code: string;
    file: string;
    /** a local model is set up, so columns can be suggested */
    ai: boolean;
    /** whether this login may ask for suggestions (manager password) */
    canSuggest: boolean;
    onuse: (s: Schemas["Suggestion"]) => void;
    onclose: () => void;
  }
  let { book, code, file, ai, canSuggest, onuse, onclose }: Props = $props();

  let cmp = $state<Schemas["CopyComparison"] | null>(null);
  let error = $state("");
  let suggestion = $state<Schemas["Suggestion"] | null>(null);
  let suggesting = $state(false);
  let suggestError = $state("");
  let showText = $state(false);

  onMount(async () => {
    try {
      cmp = await api.compareCopy(book, file);
    } catch (err) {
      error = message(err);
    }
  });

  async function suggest() {
    suggesting = true;
    suggestError = "";
    try {
      suggestion = await api.suggestColumns(book, file);
    } catch (err) {
      suggestError = message(err);
    } finally {
      suggesting = false;
    }
  }

  const missing = $derived(cmp ? [...cmp.report.columns, ...cmp.report.questions].filter((l) => !l.found) : []);
</script>

<Modal title={`${code} compared with your copy`} {onclose} maxWidth="820px">
  <div class="stack">
    <p class="muted">Kishtin read the text of your copy on this machine. Nothing was sent anywhere. A heading counts as found when all its words are in the copy.</p>
    {#if error}
      <Notice tone="error" message={error} />
    {:else if !cmp}
      <Spinner />
    {:else}
      {#if cmp.edition}
        <Notice
          tone={cmp.edition.matches ? "success" : "warning"}
          title={cmp.edition.matches ? "Same edition" : "Different edition"}
          message={cmp.edition.matches ? "Your copy is the edition this booklet was made from." : "Your copy is not the edition this booklet was made from. Check the headings below."}
        />
      {/if}
      <div class="metrics">
        <MetricCard label="Found in the copy" value={`${cmp.report.found} of ${cmp.report.total}`} tone={cmp.report.found === cmp.report.total ? "success" : "warning"} level="inset" />
        <MetricCard label="Pages read" value={String(cmp.report.pages)} level="inset" />
      </div>
      {#if missing.length > 0}
        <div>
          <strong>Not found in your copy</strong>
          <ul>
            {#each missing as l (l.label)}<li>{l.label}</li>{/each}
          </ul>
          <p class="muted small">These may have been renamed, moved or dropped in your copy. Change the booklet's columns or questions to match, or keep them if your HACCP plan needs them.</p>
        </div>
      {:else}
        <p>Every column and question of the booklet is in your copy.</p>
      {/if}
      <Button size="sm" label={showText ? "Hide the copy's text" : "Show the copy's text"} onclick={() => (showText = !showText)} />
      {#if showText}<p class="excerpt small">{cmp.report.excerpt}</p>{/if}

      {#if ai}
        <div class="stack">
          <strong>Suggest columns from the copy</strong>
          <p class="muted small">The local model reads the copy's headings and matches them to what Kishtin records. It runs on this machine or your network. Check every column before saving.</p>
          {#if !canSuggest}<Notice tone="info" message="Log in as a manager with a password to ask for suggestions." />{/if}
          <div class="row"><Button label={suggesting ? "Reading…" : "Suggest columns"} disabled={suggesting || !canSuggest} onclick={suggest} /></div>
          {#if suggestError}<Notice tone="error" message={suggestError} />{/if}
          {#if suggestion}
            <Table bare unsorted class="sugg" ariaLabel="Suggested columns">
              <thead><tr><th>Heading</th><th>Group</th><th>Shows</th></tr></thead>
              <tbody>
                {#each suggestion.columns as c, i (i)}
                  <tr><td>{c.label}</td><td>{c.group ?? ""}</td><td>{#if c.field}{c.field}{:else}<Chip uppercase={false} tone="neutral">Blank</Chip>{/if}</td></tr>
                {/each}
              </tbody>
            </Table>
            {#if (suggestion.checklist?.length ?? 0) > 0}
              <p class="small">And {suggestion.checklist?.reduce((n, s) => n + s.items.length, 0)} questions or topics in {suggestion.checklist?.length} sections.</p>
            {/if}
            {#each suggestion.notes as n (n)}<p class="muted small">{n}</p>{/each}
          {/if}
        </div>
      {:else}
        <p class="muted small">A local model can suggest columns from the copy. Set [ai] url in kishtin.toml to a model server on this machine to turn it on.</p>
      {/if}
    {/if}
  </div>
  {#snippet footer()}
    <div class="row">
      <Button label="Close" onclick={onclose} />
      {#if suggestion}
        <Button tone="info" surface="solid" label="Use these columns" onclick={() => suggestion && onuse(suggestion)} />
      {/if}
    </div>
  {/snippet}
</Modal>

<style>
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--space-3);
  }
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
  }
  ul {
    margin: var(--space-2) 0;
    padding-left: var(--space-6);
  }
  .excerpt {
    max-height: 200px;
    overflow: auto;
    padding: var(--space-3);
    background: var(--bg-inset);
    border-radius: var(--radius-md);
    white-space: pre-wrap;
  }
  :global(table.sugg) {
    border-collapse: collapse;
    width: 100%;
  }
  :global(table.sugg) th,
  :global(table.sugg) td {
    text-align: left;
    padding: var(--space-1) var(--space-2);
    border-bottom: var(--border-width) solid var(--border-default);
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
</style>
