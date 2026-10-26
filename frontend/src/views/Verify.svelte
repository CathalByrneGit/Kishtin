<script lang="ts">
  import { Button, Card, Chip, CopyButton, Notice, Spinner } from "@kenn-io/kit-ui";
  import { RefreshCw } from "@lucide/svelte";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../lib/api";
  import { dublinDate, dublinTime, longDate } from "../lib/format";

  let report = $state<Schemas["VerifyReport"] | null>(null);
  let checkedAt = $state<Date | null>(null);
  let error = $state("");
  let busy = $state(false);

  async function run() {
    busy = true;
    try {
      report = await api.verify();
      checkedAt = new Date();
      error = "";
    } catch (err) {
      error = message(err);
    } finally {
      busy = false;
    }
  }

  onMount(() => void run());

  const fileProblems = $derived(report?.file_problems ?? []);
  const unknownLimits = $derived(report?.unknown_limits ?? []);
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Verify</h1>
      <p>Re-checks every record's hash from the first one, every attached file against its SHA-256, and that every limit version records point to exists.</p>
    </div>
    <Button label={busy ? "Checking…" : "Check again"} disabled={busy} onclick={run}><RefreshCw size={16} /></Button>
  </header>

  {#if error}<Notice tone="error" title="Could not verify" message={error} />{/if}

  {#if !report}
    {#if !error}<Spinner />{/if}
  {:else}
    <Notice
      tone={report.ok ? "success" : "error"}
      title={report.ok ? "Everything checks out" : "Something does not match"}
      message={report.ok
        ? `${report.chain.checked} records and ${report.files_checked} attached files checked${checkedAt ? ` at ${dublinTime(checkedAt)} on ${longDate(dublinDate(checkedAt))}` : ""}. Nothing has been changed since it was saved.`
        : "Details are below. Keep this device and the database file as they are, and make a backup before anything else."}
    />

    <div class="grid">
      <Card padding="md" title="Hash chain">
        <dl>
          <dt>Result</dt>
          <dd><Chip tone={report.chain.ok ? "success" : "danger"} uppercase={false}>{report.chain.ok ? "Pass" : "Fail"}</Chip></dd>
          <dt>Records checked</dt>
          <dd class="num">{report.chain.checked}</dd>
          <dt>Newest record</dt>
          <dd class="num">{report.chain.head_seq || "none yet"}</dd>
          <dt>Chain head</dt>
          <dd class="hash">
            <code>{report.chain.head_hash || "none yet"}</code>
            {#if report.chain.head_hash}<CopyButton text={report.chain.head_hash} ariaLabel="Copy chain head" />{/if}
          </dd>
        </dl>
        {#if report.chain.break}
          <Notice
            tone="error"
            title={`First break at record ${report.chain.break.seq}`}
            message={report.chain.break.problem}
          />
          {#if report.chain.break.record_id}
            <a href={`/records?range=all&id=${report.chain.break.record_id}`}>Show record {report.chain.break.seq}</a>
          {/if}
        {/if}
      </Card>

      <Card padding="md" title="Attached files">
        <dl>
          <dt>Result</dt>
          <dd><Chip tone={fileProblems.length === 0 ? "success" : "danger"} uppercase={false}>{fileProblems.length === 0 ? "Pass" : "Fail"}</Chip></dd>
          <dt>Files checked</dt>
          <dd class="num">{report.files_checked}</dd>
        </dl>
        {#each fileProblems as p (p.sha256)}
          <div class="problem">
            <code>{p.sha256}</code>
            <span>{p.problem}</span>
          </div>
        {/each}
      </Card>

      <Card padding="md" title="Limit versions">
        <dl>
          <dt>Result</dt>
          <dd><Chip tone={unknownLimits.length === 0 ? "success" : "danger"} uppercase={false}>{unknownLimits.length === 0 ? "Pass" : "Fail"}</Chip></dd>
        </dl>
        {#if unknownLimits.length > 0}
          <p>Records refer to limit versions that do not exist:</p>
          {#each unknownLimits as id (id)}<code class="block">{id}</code>{/each}
        {:else}
          <p class="muted">Every record's limit version exists, so it is clear which rule each record was checked against.</p>
        {/if}
      </Card>
    </div>

    <Card padding="md" title="What this cannot catch">
      <p>
        If someone removed the newest records from the end of the chain, what is left would still verify. To catch that, compare the
        chain head above with one written down earlier, on an export or a backup. The chain head only ever moves forward.
      </p>
    </Card>
  {/if}
</div>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: var(--space-6);
    align-items: start;
  }
  dl {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: var(--space-3) var(--space-5);
    margin: 0 0 var(--space-4);
    align-items: center;
  }
  dt {
    color: var(--text-secondary);
  }
  dd {
    margin: 0;
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
  .block {
    display: block;
  }
  .problem {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-3) 0;
    border-top: var(--border-width) solid var(--border-muted);
  }
  p {
    margin: 0;
  }
</style>
