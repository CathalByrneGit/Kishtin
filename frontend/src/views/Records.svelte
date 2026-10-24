<script lang="ts">
  import {
    Chip,
    EmptyState,
    Notice,
    SearchInput,
    SegmentedControl,
    SelectDropdown,
    Spinner,
    Table,
    TableHeaderCell,
    TableSort,
  } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import RecordDetails from "../components/RecordDetails.svelte";
  import { api, message, type HaccpRecord, type RecordFilter } from "../lib/api";
  import { addDays, dublinDate, dublinTime, shortDate } from "../lib/format";
  import { details, outcomeStyle, title, TYPE_LABELS, typeLabel } from "../lib/labels";
  import { onChange } from "../lib/live.svelte";
  import { param, setParam } from "../lib/router.svelte";

  const LIMIT = 1000;
  const range = $derived(param("range") ?? "7");
  const type = $derived(param("type") ?? "all");
  const selectedId = $derived(param("id"));
  let query = $state(param("q") ?? "");

  let records = $state<HaccpRecord[] | null>(null);
  let fetched = $state<HaccpRecord | null>(null);
  let error = $state("");

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

  const sorter = new TableSort<HaccpRecord>(
    {
      seq: (r) => r.seq,
      time: (r) => r.logged_at,
      type: (r) => typeLabel(r.type),
      record: (r) => title(r).toLowerCase(),
      result: (r) => ({ fail: 0, borderline: 1, pass: 2 })[r.outcome] ?? 3,
      by: (r) => r.logged_by_name.toLowerCase(),
    },
    { key: "time", direction: "desc" },
  );

  const visible = $derived.by(() => {
    const q = query.trim().toLowerCase();
    const rows = (records ?? []).filter(
      (r) =>
        !q ||
        [title(r), details(r), typeLabel(r.type), r.logged_by_name, r.corrective_action ?? "", r.outcome_reason, String(r.seq), r.id]
          .join(" ")
          .toLowerCase()
          .includes(q),
    );
    return sorter.sort(rows);
  });

  // The selected record: from the list when it is there, otherwise fetched.
  const selected = $derived(records?.find((r) => r.id === selectedId) ?? (fetched?.id === selectedId ? fetched : null));

  $effect(() => {
    const id = selectedId;
    if (!id || !records || records.some((r) => r.id === id)) return;
    api
      .record(id)
      .then((r) => (fetched = r))
      .catch((err: unknown) => (error = message(err)));
  });

  const correctedBy = $derived(selected ? (records ?? []).filter((r) => r.corrects === selected.id) : []);
  const correctedIds = $derived(new Set((records ?? []).map((r) => r.corrects).filter(Boolean)));

  const typeOptions = [
    { value: "all", label: "All checks" },
    { value: "fails", label: "Borderline and fails" },
    ...Object.entries(TYPE_LABELS).map(([value, label]) => ({ value, label })),
  ];

  function select(r: HaccpRecord) {
    setParam("id", selectedId === r.id ? null : r.id);
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Records</h1>
      <p>Every HACCP record, newest first. Select a record to see its details, docket and hashes.</p>
    </div>
  </header>

  <div class="toolbar">
    <div class="search">
      <SearchInput
        bind:value={query}
        placeholder="Search records: fridge, food, supplier, staff, action"
        ariaLabel="Search records"
        block
        onchange={(v) => setParam("q", v)}
        onclear={() => setParam("q", null)}
      />
    </div>
    <SegmentedControl
      ariaLabel="Date range"
      value={range}
      onchange={(v) => setParam("range", v === "7" ? null : v)}
      options={[
        { value: "1", label: "Today" },
        { value: "7", label: "7 days" },
        { value: "31", label: "31 days" },
        { value: "all", label: "All" },
      ]}
    />
    <SelectDropdown value={type} options={typeOptions} onchange={(v) => setParam("type", v === "all" ? null : v)} title="Type" />
  </div>

  {#if error}<Notice tone="error" message={error} />{/if}

  <div class="split" class:open={Boolean(selected)}>
    <div class="list">
      {#if records === null}
        {#if !error}<Spinner />{/if}
      {:else if visible.length === 0}
        <EmptyState title="No records" description={query ? "Nothing matches that search." : "Nothing logged for this filter."} />
      {:else}
        {#if records.length === LIMIT}
          <Notice tone="info" message={`Showing the newest ${LIMIT} records. Narrow the dates, or export, to see the rest.`} />
        {/if}
        <Table ariaLabel="HACCP records" stickyHeader>
          {#snippet header()}
            <TableHeaderCell label="#" numeric sort={sorter} column="seq" />
            <TableHeaderCell label="When" sort={sorter} column="time" />
            <TableHeaderCell label="Type" sort={sorter} column="type" />
            <TableHeaderCell label="Record" sort={sorter} column="record" />
            <TableHeaderCell label="Result" sort={sorter} column="result" />
            <TableHeaderCell label="By" sort={sorter} column="by" />
          {/snippet}
          {#each visible as r (r.id)}
            {@const o = outcomeStyle(r.outcome)}
            <tr class="clickable" class:selected={r.id === selectedId} onclick={() => select(r)}>
              <td class="num muted">{r.seq}</td>
              <td class="num nowrap">
                {shortDate(dublinDate(new Date(r.logged_at)))}, {dublinTime(r.logged_at)}
              </td>
              <td>{typeLabel(r.type)}</td>
              <td>
                <a
                  href={`/records?id=${r.id}`}
                  onclick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    select(r);
                  }}><strong>{title(r)}</strong></a
                >
                {#if details(r)}<span class="muted"> · {details(r)}</span>{/if}
                {#if r.corrects}<Chip size="sm" uppercase={false} tone="info">Correction</Chip>{/if}
                {#if correctedIds.has(r.id)}<Chip size="sm" uppercase={false} tone="muted">Corrected</Chip>{/if}
                {#if r.corrective_action}<div class="muted small">Action: {r.corrective_action}</div>{/if}
              </td>
              <td><Chip tone={o.tone} uppercase={false}>{o.label}</Chip></td>
              <td class="nowrap">{r.logged_by_name}</td>
            </tr>
          {/each}
        </Table>
        <span class="muted small">{visible.length} of {records.length} records shown.</span>
      {/if}
    </div>

    {#if selected}
      <RecordDetails record={selected} {correctedBy} onclose={() => setParam("id", null)} />
    {/if}
  </div>
</div>

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex-wrap: wrap;
  }
  .search {
    flex: 1;
    min-width: 260px;
    max-width: 520px;
  }
  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-6);
    align-items: start;
  }
  .split.open {
    grid-template-columns: minmax(0, 1fr) minmax(340px, 420px);
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
  }
  .clickable {
    cursor: pointer;
  }
  .selected td {
    background: var(--bg-selected, var(--bg-inset));
  }
  .nowrap {
    white-space: nowrap;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  a {
    color: inherit;
    text-decoration: none;
  }
  a:hover {
    text-decoration: underline;
  }
  @media (max-width: 900px) {
    .split.open {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
