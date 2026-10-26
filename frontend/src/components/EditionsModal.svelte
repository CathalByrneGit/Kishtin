<script lang="ts">
  import { Button, Chip, Modal, Notice, Spinner, Table } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../lib/api";

  interface Props {
    canCheck: boolean;
    /** use a downloaded new edition as the booklet's copy */
    onuse: (booklet: string, file: string) => void;
    onclose: () => void;
  }
  let { canCheck, onuse, onclose }: Props = $props();

  let targets = $state<Schemas["EditionTarget"][] | null>(null);
  let results = $state<Schemas["EditionResult"][] | null>(null);
  let error = $state("");
  let busy = $state(false);

  onMount(async () => {
    try {
      targets = (await api.editionTargets()).targets;
    } catch (err) {
      error = message(err);
    }
  });

  async function check() {
    busy = true;
    try {
      results = (await api.checkEditions()).results;
      error = "";
    } catch (err) {
      error = message(err);
    } finally {
      busy = false;
    }
  }

  const hosts = $derived([...new Set((targets ?? []).map((t) => new URL(t.url).host))]);
  const tone = (s: string) => (s === "same" ? "success" : s === "changed" ? "warning" : "danger");
  const words: Record<string, string> = { same: "Same edition", changed: "New edition", unreachable: "Could not download" };
</script>

<Modal title="Check for new editions" {onclose} maxWidth="820px">
  <div class="stack">
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if !targets}
      {#if !error}<Spinner />{/if}
    {:else if !results}
      <p>
        Kishtin will download {targets.length} official PDFs from {hosts.join(", ")} and compare each with the edition its booklet was made from. Only these public files are downloaded. Nothing about your restaurant is sent.
      </p>
      <Table bare unsorted class="ed" ariaLabel="Files to download">
        <thead><tr><th>Booklet</th><th>Edition in Kishtin</th><th>Downloaded from</th></tr></thead>
        <tbody>
          {#each targets as t (t.booklet)}
            <tr><td>{t.code} {t.title}</td><td>{t.published}</td><td class="url">{t.url}</td></tr>
          {/each}
        </tbody>
      </Table>
      {#if !canCheck}<Notice tone="info" message="Log in as a manager with a password to check." />{/if}
    {:else}
      <Table bare unsorted class="ed" ariaLabel="Results">
        <thead><tr><th>Booklet</th><th>Result</th><th></th></tr></thead>
        <tbody>
          {#each results as r (r.booklet)}
            <tr>
              <td>{r.code} {r.title}</td>
              <td><Chip uppercase={false} dot tone={tone(r.status)}>{words[r.status] ?? r.status}</Chip>{#if r.error}<span class="small muted"> {r.error}</span>{/if}</td>
              <td>{#if r.file}<Button size="sm" label="Compare with the booklet" onclick={() => r.file && onuse(r.booklet, r.file)} />{/if}</td>
            </tr>
          {/each}
        </tbody>
      </Table>
      <p class="muted small">A new edition is kept as your copy of the booklet once you compare it. Comparing shows which headings changed.</p>
    {/if}
  </div>
  {#snippet footer()}
    <div class="row">
      <Button label="Close" onclick={onclose} />
      {#if targets && !results}
        <Button tone="info" surface="solid" label={busy ? "Downloading…" : `Download ${targets.length} PDFs and compare`} disabled={busy || !canCheck || targets.length === 0} onclick={check} />
      {/if}
    </div>
  {/snippet}
</Modal>

<style>
  :global(table.ed) {
    border-collapse: collapse;
    width: 100%;
    font-size: var(--font-size-sm);
  }
  :global(table.ed) th,
  :global(table.ed) td {
    text-align: left;
    padding: var(--space-2);
    border-bottom: var(--border-width) solid var(--border-default);
    vertical-align: top;
  }
  .url {
    word-break: break-all;
    color: var(--text-secondary);
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
