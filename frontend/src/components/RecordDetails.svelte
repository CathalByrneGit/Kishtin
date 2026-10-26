<script lang="ts">
  import { Button, Chip, CopyButton, IconButton, Notice } from "@kenn-io/kit-ui";
  import { FileText, PenLine, X } from "@lucide/svelte";
  import { api, message, type HaccpRecord } from "../lib/api";
  import { dublinDate, dublinTime, longDate } from "../lib/format";
  import { doneAt, doneBy, outcomeStyle, readingRows, title, typeLabel } from "../lib/labels";
  import { openLog } from "../lib/logdrawer.svelte";

  interface Props {
    record: HaccpRecord;
    /** records in the current list that correct this one */
    correctedBy: HaccpRecord[];
    onclose: () => void;
  }
  let { record, correctedBy, onclose }: Props = $props();

  let docket = $state<{ url: string; type: string } | null>(null);
  let docketError = $state("");

  $effect(() => {
    const sha = record.reading.docket_file ?? record.reading.paper_file;
    docket = null;
    docketError = "";
    if (!sha) return;
    let url = "";
    let gone = false;
    api
      .file(sha)
      .then((f) => {
        url = f.url;
        if (gone) URL.revokeObjectURL(url);
        else docket = f;
      })
      .catch((err: unknown) => {
        if (!gone) docketError = message(err);
      });
    return () => {
      gone = true;
      if (url) URL.revokeObjectURL(url);
    };
  });

  const o = $derived(outcomeStyle(record.outcome));
  const rows = $derived(readingRows(record));
  const at = $derived(new Date(doneAt(record)));
  const saved = $derived(new Date(record.logged_at));
</script>

<aside class="panel" aria-label="Record details">
  <header>
    <div class="titles">
      <span class="muted small">{typeLabel(record.type)} · record {record.seq}</span>
      <h2>{title(record)}</h2>
    </div>
    <IconButton ariaLabel="Close details" title="Close" onclick={onclose}><X size={16} /></IconButton>
  </header>

  <div class="body">
    <div class="row">
      <Chip tone={o.tone} uppercase={false}>{o.label}</Chip>
      <span>{record.outcome_reason}</span>
    </div>

    {#if correctedBy.length > 0}
      <Notice
        tone="info"
        title="This record was corrected"
        message={correctedBy.map((c) => `Record ${c.seq} on ${longDate(dublinDate(new Date(c.logged_at)))}: ${c.correction_reason ?? ""}`).join(" ")}
      />
    {/if}

    {#if record.corrects}
      <Notice tone="info" title="This record corrects an earlier one" message={`Why: ${record.correction_reason ?? ""}`} />
      <a class="small" href={`/records?id=${record.corrects}`}>Show the original record</a>
    {/if}

    <dl>
      <dt>When</dt>
      <dd class="num">{longDate(dublinDate(at))}, {dublinTime(at)}</dd>
      <dt>By</dt>
      <dd>{doneBy(record)}</dd>
      {#each rows as [label, value] (label)}
        <dt>{label}</dt>
        <dd>{value}</dd>
      {/each}
      {#if record.corrective_action}
        <dt>Corrective action</dt>
        <dd>{record.corrective_action}</dd>
      {/if}
      {#if record.limit_scope}
        <dt>Limit from</dt>
        <dd>{record.limit_scope.kind === "default" ? "Check type default" : `${record.limit_scope.kind}: ${record.limit_scope.ref}`}</dd>
      {/if}
    </dl>

    {#if record.reading.docket_file || record.reading.paper_file}
      <section>
        <h3>{record.reading.paper_file ? "Paper page this was transcribed from" : "Docket"}</h3>
        {#if docketError}
          <Notice tone="error" message={docketError} />
        {:else if docket?.type.startsWith("image/")}
          <a href={docket.url} target="_blank" rel="noopener"><img src={docket.url} alt="Supplier docket" /></a>
        {:else if docket}
          <a href={docket.url} target="_blank" rel="noopener" class="row"><FileText size={16} /> Open the docket ({docket.type || "file"})</a>
        {:else}
          <span class="muted small">Loading…</span>
        {/if}
      </section>
    {/if}

    <section>
      <h3>Evidence</h3>
      <dl class="evidence">
        <dt>Record ID</dt>
        <dd class="hash"><code>{record.id}</code><CopyButton text={record.id} ariaLabel="Copy record ID" /></dd>
        <dt>Saved</dt>
        <dd class="num">{longDate(dublinDate(saved))}, {dublinTime(saved)} by {record.logged_by_name}</dd>
        <dt>Hash</dt>
        <dd class="hash"><code>{record.hash}</code><CopyButton text={record.hash} ariaLabel="Copy hash" /></dd>
        <dt>Previous hash</dt>
        <dd class="hash"><code>{record.prev_hash || "none (first record)"}</code></dd>
        {#if record.limit_version_id}
          <dt>Limit version</dt>
          <dd class="hash"><code>{record.limit_version_id}</code></dd>
        {/if}
        {#if record.rules_version_id}
          <dt>Rules version</dt>
          <dd class="hash"><code>{record.rules_version_id}</code></dd>
        {/if}
        {#if record.reading.docket_file}
          <dt>Docket SHA-256</dt>
          <dd class="hash"><code>{record.reading.docket_file}</code></dd>
        {/if}
      </dl>
    </section>
  </div>

  <footer>
    <Button label="Correct this record" onclick={() => openLog({ type: record.type, corrects: record })}>
      <PenLine size={16} />
    </Button>
    <span class="muted small">Records are never edited. A correction is a new record that points to this one.</span>
  </footer>
</aside>

<style>
  .panel {
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-lg);
    position: sticky;
    top: calc(var(--space-7) + 48px);
    max-height: calc(100vh - 2 * var(--space-7) - 48px);
    overflow: hidden;
  }
  header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-5) var(--space-6);
    border-bottom: var(--border-width) solid var(--border-muted);
  }
  .titles {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }
  h2 {
    font-size: var(--font-size-lg);
  }
  h3 {
    font-size: var(--font-size-sm);
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin-bottom: var(--space-3);
  }
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    padding: var(--space-5) var(--space-6);
    overflow-y: auto;
  }
  dl {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: var(--space-2) var(--space-5);
    margin: 0;
  }
  dt {
    color: var(--text-secondary);
  }
  dd {
    margin: 0;
  }
  .evidence {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-1);
  }
  .evidence dd {
    margin-bottom: var(--space-3);
  }
  .hash {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  code {
    font-family: var(--font-mono);
    font-size: var(--font-size-xs);
    word-break: break-all;
  }
  img {
    max-width: 100%;
    border-radius: var(--radius-md);
    border: var(--border-width) solid var(--border-muted);
  }
  footer {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-5) var(--space-6);
    border-top: var(--border-width) solid var(--border-muted);
  }
  .small {
    font-size: var(--font-size-sm);
  }
</style>
