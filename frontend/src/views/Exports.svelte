<script lang="ts">
  import {
    Button,
    Card,
    Checkbox,
    CopyButton,
    DatePicker,
    MetricCard,
    Notice,
    SelectDropdown,
    Spinner,
    showFlash,
  } from "@kenn-io/kit-ui";
  import { Download, FileText, History } from "@lucide/svelte";
  import { api, download, message, type ExportResult, type HaccpRecord, type RecordFilter } from "../lib/api";
  import { addDays, dublinDate, longDate } from "../lib/format";
  import { TYPE_LABELS, typeLabel } from "../lib/labels";
  import { session } from "../lib/session.svelte";

  // The API returns at most 1000 records; the export itself has no limit.
  const PREVIEW_LIMIT = 1000;
  const today = dublinDate(new Date());
  let from = $state(addDays(today, -30));
  let to = $state(today);
  let everything = $state(false);
  let type = $state("all");
  let failsOnly = $state(false);

  let preview = $state<HaccpRecord[] | null>(null);
  let error = $state("");
  let busy = $state<string | null>(null);
  let last = $state<(Omit<ExportResult, "blob"> & { kind: string }) | null>(null);

  const canExport = $derived(session.user?.role === "manager" || session.user?.role === "head_chef");
  const badRange = $derived(!everything && from > to);

  const filter = $derived.by((): RecordFilter => {
    const f: RecordFilter = {};
    if (!everything) {
      f.from = from;
      f.to = to;
    }
    if (type !== "all") f.type = [type];
    if (failsOnly) f.outcome = ["borderline", "fail"];
    return f;
  });

  $effect(() => {
    const f = filter;
    if (badRange) {
      preview = [];
      return;
    }
    preview = null;
    let gone = false;
    api
      .records({ ...f, limit: PREVIEW_LIMIT })
      .then((r) => {
        if (!gone) {
          preview = r.records;
          error = "";
        }
      })
      .catch((err: unknown) => {
        if (!gone) error = message(err);
      });
    return () => {
      gone = true;
    };
  });

  const counts = $derived.by(() => {
    const c = { fail: 0, borderline: 0, withoutAction: 0, corrections: 0, dockets: 0, byType: new Map<string, number>() };
    for (const r of preview ?? []) {
      if (r.outcome === "fail") c.fail++;
      if (r.outcome === "borderline") c.borderline++;
      if ((r.outcome === "fail" || r.outcome === "borderline") && !r.corrective_action) c.withoutAction++;
      if (r.corrects) c.corrections++;
      if (r.reading.docket_file) c.dockets++;
      c.byType.set(r.type, (c.byType.get(r.type) ?? 0) + 1);
    }
    return c;
  });

  async function exportAs(kind: "csv" | "pdf") {
    busy = kind;
    try {
      const { blob, ...rest } = await api.exportFile(kind, filter);
      download(blob, rest.name);
      last = { ...rest, kind: kind === "csv" ? "CSV" : "Inspection pack" };
      showFlash(kind === "csv" ? "CSV downloaded" : "Inspection pack downloaded", { tone: "success" });
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      busy = null;
    }
  }

  async function exportLimits() {
    busy = "limits";
    try {
      const { blob, name } = await api.exportLimits();
      download(blob, name);
      showFlash("Limit history downloaded", { tone: "success" });
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      busy = null;
    }
  }

  const typeOptions = [{ value: "all", label: "All record types" }, ...Object.entries(TYPE_LABELS).map(([value, label]) => ({ value, label }))];
  const rangeText = $derived(everything ? "every record" : from === to ? longDate(from) : `${longDate(from)} to ${longDate(to)}`);
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Exports</h1>
      <p>Choose what to export and check the preview, then download. Every export lists record IDs and hashes so an inspector can check them.</p>
    </div>
  </header>

  {#if !canExport}
    <Notice tone="info" title="Head chef or manager only" message="Exports are for the head chef and managers. Ask one of them to log in on this device." />
  {:else}
    <div class="grid">
      <Card padding="md" title="What to export">
        <div class="stack">
          <Checkbox bind:checked={everything} label="Every record, from the first" />
          {#if !everything}
            <div class="two">
              <label class="field"><span>From</span><DatePicker value={from} onchange={(d) => (from = d)} ariaLabel="From" block /></label>
              <label class="field"><span>To (inclusive)</span><DatePicker value={to} onchange={(d) => (to = d)} ariaLabel="To" block /></label>
            </div>
            {#if badRange}<Notice tone="error" message="The start date is after the end date." />{/if}
          {/if}
          <label class="field">
            <span>Record type</span>
            <SelectDropdown value={type} options={typeOptions} onchange={(v) => (type = v)} title="Record type" />
          </label>
          <Checkbox bind:checked={failsOnly} label="Only borderline and failed records" />
          <span class="muted small">Dates are Dublin time.</span>
        </div>
      </Card>

      <div class="stack">
        <Card padding="md" title="Preview" meta={rangeText}>
          {#if error}
            <Notice tone="error" message={error} />
          {:else if preview === null}
            <Spinner />
          {:else}
            <div class="metrics">
              <MetricCard label="Records" value={preview.length === PREVIEW_LIMIT ? `${PREVIEW_LIMIT}+` : String(preview.length)} level="inset" />
              <MetricCard label="Failed" value={String(counts.fail)} tone={counts.fail > 0 ? "danger" : "neutral"} level="inset" />
              <MetricCard label="Borderline" value={String(counts.borderline)} tone={counts.borderline > 0 ? "warning" : "neutral"} level="inset" />
              <MetricCard label="Without an action" value={String(counts.withoutAction)} tone={counts.withoutAction > 0 ? "danger" : "neutral"} level="inset" />
            </div>
            {#if preview.length === PREVIEW_LIMIT}
              <p class="muted small capped">More than {PREVIEW_LIMIT} records match. The counts cover the first {PREVIEW_LIMIT}; the export includes them all.</p>
            {/if}
            {#if counts.byType.size > 0}
              <ul class="types">
                {#each [...counts.byType] as [t, n] (t)}
                  <li><span>{typeLabel(t)}</span><span class="num">{n}</span></li>
                {/each}
                <li class="muted"><span>Corrections</span><span class="num">{counts.corrections}</span></li>
                <li class="muted"><span>Dockets attached</span><span class="num">{counts.dockets}</span></li>
              </ul>
            {/if}
          {/if}
        </Card>

        <Card padding="md" title="Download">
          <div class="downloads">
            <div class="download">
              <Button tone="info" surface="solid" label={busy === "pdf" ? "Making the pack…" : "Inspection pack (PDF)"} disabled={busy !== null || badRange || preview?.length === 0} onclick={() => exportAs("pdf")}>
                <FileText size={16} />
              </Button>
              <span class="muted small">Summary, failures with their actions, every record by day, and the limits used.</span>
            </div>
            <div class="download">
              <Button label={busy === "csv" ? "Exporting…" : "Records (CSV)"} disabled={busy !== null || badRange || preview?.length === 0} onclick={() => exportAs("csv")}>
                <Download size={16} />
              </Button>
              <span class="muted small">One row per record, with IDs, times, results, limit versions and both hashes.</span>
            </div>
            <div class="download">
              <Button label={busy === "limits" ? "Exporting…" : "Limit history (CSV)"} disabled={busy !== null} onclick={exportLimits}>
                <History size={16} />
              </Button>
              <span class="muted small">Every limit version ever saved, with who, when and why. Not filtered by date.</span>
            </div>
          </div>
        </Card>

        {#if last}
          <Card padding="md" title="Last export" meta={last.name}>
            <dl>
              <dt>File</dt>
              <dd>{last.kind}</dd>
              <dt>Records</dt>
              <dd class="num">{last.records}</dd>
              <dt>Hash chain</dt>
              <dd>{last.chainOK ? "Verified when exported" : "Did not verify. See Verify."}</dd>
              <dt>Chain head</dt>
              <dd class="hash"><code>{last.chainHead}</code><CopyButton text={last.chainHead} ariaLabel="Copy chain head" /></dd>
            </dl>
            <p class="muted small">Keep the chain head with the file. If later records are ever removed from the end of the chain, comparing it with a newer export shows it.</p>
          </Card>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .grid {
    display: grid;
    grid-template-columns: minmax(280px, 380px) minmax(0, 1fr);
    gap: var(--space-6);
    align-items: start;
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-4);
  }
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: var(--space-4);
  }
  .types {
    list-style: none;
    margin: var(--space-5) 0 0;
    padding: 0;
    columns: 2;
    column-gap: var(--space-7);
  }
  .types li {
    display: flex;
    justify-content: space-between;
    padding: var(--space-1) 0;
    break-inside: avoid;
  }
  .downloads {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  .download {
    display: grid;
    grid-template-columns: 220px 1fr;
    align-items: center;
    gap: var(--space-5);
  }
  .download :global(.kit-button) {
    width: 100%;
  }
  dl {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: var(--space-2) var(--space-5);
    margin: 0 0 var(--space-4);
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
  .small {
    font-size: var(--font-size-sm);
  }
  .capped {
    margin-top: var(--space-4);
  }
  p {
    margin: 0;
  }
  @media (max-width: 900px) {
    .grid,
    .download {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
