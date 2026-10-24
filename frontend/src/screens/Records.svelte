<script lang="ts">
  import { Button, Card, Chip, EmptyState, Notice, SegmentedControl, SelectDropdown, Spinner, showFlash } from "@kenn-io/kit-ui";
  import { Download, FileText } from "@lucide/svelte";
  import { onMount } from "svelte";
  import { api, download, message, type HaccpRecord, type RecordFilter } from "../lib/api";
  import { addDays, dublinDate, dublinTime, longDate } from "../lib/format";
  import { details, outcomeStyle, title, TYPE_LABELS, typeLabel } from "../lib/labels";
  import { onChange } from "../lib/live.svelte";
  import { session } from "../lib/session.svelte";

  const LIMIT = 1000;
  let range = $state("7");
  let type = $state("all");
  let records = $state<HaccpRecord[] | null>(null);
  let error = $state("");
  let exporting = $state(false);

  const canExport = $derived(session.user?.role === "manager" || session.user?.role === "head_chef");

  function filter(): RecordFilter {
    const today = dublinDate(new Date());
    const f: RecordFilter = {};
    if (range !== "all") f.from = addDays(today, 1 - Number(range));
    if (type === "fails") f.outcome = ["borderline", "fail"];
    else if (type !== "all") f.type = [type];
    return f;
  }

  async function load() {
    try {
      records = (await api.records({ ...filter(), newest: true, limit: LIMIT })).records;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  $effect(() => {
    void range;
    void type;
    records = null;
    void load();
  });

  onMount(() =>
    onChange((kind) => {
      if (kind === "record") void load();
    }),
  );

  const days = $derived.by(() => {
    const groups: { date: string; items: HaccpRecord[] }[] = [];
    for (const r of records ?? []) {
      const d = dublinDate(new Date(r.logged_at));
      const last = groups.at(-1);
      if (last?.date === d) last.items.push(r);
      else groups.push({ date: d, items: [r] });
    }
    return groups;
  });

  async function exportAs(kind: "csv" | "pdf") {
    exporting = true;
    try {
      const { blob, name } = await api.exportFile(kind, filter());
      download(blob, name);
      showFlash(kind === "csv" ? "CSV downloaded" : "Inspection pack downloaded", { tone: "success" });
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      exporting = false;
    }
  }

  const typeOptions = [
    { value: "all", label: "All checks" },
    { value: "fails", label: "Fails only" },
    ...Object.entries(TYPE_LABELS).map(([value, label]) => ({ value, label })),
  ];
</script>

<div class="stack">
  <SegmentedControl
    block
    ariaLabel="Date range"
    value={range}
    onchange={(v) => (range = v)}
    options={[
      { value: "1", label: "Today" },
      { value: "7", label: "7 days" },
      { value: "31", label: "31 days" },
      { value: "all", label: "All" },
    ]}
  />
  <div class="row">
    <SelectDropdown value={type} options={typeOptions} onchange={(v) => (type = v)} title="Type" />
    {#if canExport}
      <Button label="Export CSV" disabled={exporting} onclick={() => exportAs("csv")}><Download size={20} /></Button>
      <Button label="Inspection pack (PDF)" disabled={exporting} onclick={() => exportAs("pdf")}><FileText size={20} /></Button>
    {/if}
  </div>

  {#if error}<Notice tone="error" message={error} />{/if}

  {#if records === null}
    {#if !error}<Spinner />{/if}
  {:else if records.length === 0}
    <EmptyState title="No records" description="Nothing logged for this filter." />
  {:else}
    {#if records.length === LIMIT}
      <Notice tone="info" message={`Showing the newest ${LIMIT} records. Narrow the filter, or export, to see the rest.`} />
    {/if}
    {#each days as day (day.date)}
      <h2 class="section-title">{longDate(day.date)}</h2>
      <Card padding="none">
        <ul class="list">
          {#each day.items as r (r.id)}
            {@const o = outcomeStyle(r.outcome)}
            <li>
              <span class="time num">{dublinTime(r.logged_at)}</span>
              <div class="body">
                <div class="row">
                  <strong>{title(r)}</strong>
                  <Chip tone={o.tone} uppercase={false}>{o.label}</Chip>
                </div>
                <span class="muted">{typeLabel(r.type)}{details(r) ? ` · ${details(r)}` : ""} · {r.logged_by_name}</span>
                {#if r.corrective_action}<span>Action: {r.corrective_action}</span>{/if}
                {#if r.corrects}<span class="muted">Corrects an earlier record: {r.correction_reason}</span>{/if}
              </div>
            </li>
          {/each}
        </ul>
      </Card>
    {/each}
  {/if}
</div>

<style>
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: flex;
    gap: var(--space-5);
    padding: var(--space-5) var(--space-6);
    font-size: var(--font-size-lg);
  }
  li + li {
    border-top: 1px solid var(--border-muted);
  }
  .time {
    flex: none;
    width: 3.5em;
    font-weight: var(--font-weight-semibold);
  }
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }
</style>
