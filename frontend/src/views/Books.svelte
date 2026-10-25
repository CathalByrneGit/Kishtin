<script lang="ts">
  import { Button, Card, Chip, MetricCard, Notice, SelectDropdown, Spinner, Table, showFlash } from "@kenn-io/kit-ui";
  import { Eye, FileText } from "@lucide/svelte";
  import { onMount, tick } from "svelte";
  import { api, download, message, type Schemas } from "../lib/api";
  import { cellText, gapsByDate, headerGroups, monthLabel, recentMonths } from "../lib/books";
  import { longDate } from "../lib/format";
  import { param, setParam } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  type Book = Schemas["Book"];
  type Template = Schemas["Template"];

  const months = recentMonths(new Date(), 13);
  let month = $state(param("month") ?? months[0]!);
  let packs = $state<Schemas["Pack"][] | null>(null);
  let filled = $state<Record<string, Book | string>>({});
  let selected = $state<string | null>(param("book"));
  let error = $state("");
  let busy = $state<string | null>(null);

  const canExport = $derived(session.user?.role === "manager" || session.user?.role === "head_chef");

  onMount(() => {
    if (!canExport) return;
    api
      .bookPacks()
      .then((r) => (packs = r.packs))
      .catch((err: unknown) => (error = message(err)));
  });

  // Fill in every booklet Kishtin can, for the chosen month, so each shows
  // its missed checks before anyone opens it.
  $effect(() => {
    const m = month;
    const list = packs;
    if (!list) return;
    filled = {};
    let gone = false;
    for (const p of list) {
      for (const t of p.templates) {
        if (!t.builder) continue;
        api
          .book(t.id, m)
          .then((b) => {
            if (!gone) filled[t.id] = b;
          })
          .catch((err: unknown) => {
            if (!gone) filled[t.id] = message(err);
          });
      }
    }
    return () => {
      gone = true;
    };
  });

  function choose(m: string) {
    month = m;
    setParam("month", m === months[0] ? null : m);
  }

  async function open(t: Template) {
    selected = selected === t.id ? null : t.id;
    setParam("book", selected);
    await tick();
    if (selected) document.getElementById("book-preview")?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  async function downloadPDF(t: Template) {
    busy = t.id;
    try {
      const res = await api.bookPDF(t.id, month);
      download(res.blob, res.name);
      showFlash(`${t.code} for ${monthLabel(month)} downloaded`, { tone: "success" });
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      busy = null;
    }
  }

  const current = $derived(selected ? filled[selected] : undefined);
  const book = $derived(typeof current === "object" ? current : null);
  const monthOptions = months.map((m) => ({ value: m, label: monthLabel(m) }));
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Record books</h1>
      <p>Kishtin fills in the official HACCP record books from the records staff log. Check the preview and the missed checks, then download and print.</p>
    </div>
    <label class="month">
      <span>Month</span>
      <SelectDropdown value={month} options={monthOptions} onchange={choose} title="Month" />
    </label>
  </header>

  {#if !canExport}
    <Notice tone="info" title="Head chef or manager only" message="Record books are for the head chef and managers. Ask one of them to log in on this device." />
  {:else if error}
    <Notice tone="error" message={error} />
  {:else if !packs}
    <Spinner />
  {:else}
    {#each packs as p (p.id)}
      <Card padding="md" title={p.name} meta={p.issuer}>
        <ul class="books">
          {#each p.templates as t (t.id)}
            {@const b = filled[t.id]}
            <li class:muted={!t.builder}>
              <span class="code num">{t.code}</span>
              <div class="about">
                <span class="title">{t.title}</span>
                <span class="small muted">Edition {t.source.published}</span>
                {#if !t.builder}
                  <span class="small">{t.missing}</span>
                {:else if typeof b === "string"}
                  <span class="small error">{b}</span>
                {/if}
              </div>
              <div class="status">
                {#if !t.builder}
                  <Chip uppercase={false} tone="neutral">Not yet</Chip>
                {:else if b === undefined}
                  <Spinner />
                {:else if typeof b === "object"}
                  {#if b.gaps.length > 0}
                    <Chip uppercase={false} dot tone="warning">{b.gaps.length} missed {b.gaps.length === 1 ? "check" : "checks"}</Chip>
                  {:else}
                    <Chip uppercase={false} dot tone="success">No missed checks</Chip>
                  {/if}
                  <span class="small muted">{b.records} {b.records === 1 ? "record" : "records"}</span>
                {/if}
              </div>
              {#if t.builder}
                <div class="actions">
                  <Button label={selected === t.id ? "Hide preview" : "Preview"} onclick={() => open(t)}><Eye size={16} /></Button>
                  <Button tone="info" surface="solid" label={busy === t.id ? "Making the PDF…" : "Download PDF"} disabled={busy !== null || typeof b !== "object"} onclick={() => downloadPDF(t)}>
                    <FileText size={16} />
                  </Button>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      </Card>
    {/each}

    {#if book}
      <div id="book-preview"></div>
      <Card padding="md" title={`${book.template.code} ${book.template.title}`} meta={monthLabel(book.month)}>
        <div class="metrics">
          <MetricCard label="Records" value={String(book.records)} level="inset" />
          <MetricCard label="Missed checks" value={String(book.gaps.length)} tone={book.gaps.length > 0 ? "warning" : "neutral"} level="inset" />
          <MetricCard label="Corrected" value={String(book.corrected)} level="inset" />
        </div>
        {#if book.unfilled.length > 0}
          <Notice tone="info" title="Left blank" message={`Kishtin does not record: ${book.unfilled.join(", ")}. These boxes print blank.`} />
        {/if}

        {#each book.pages as page, pi (pi)}
          <div class="sheet" role="region" aria-label={`Sheet ${pi + 1}`}>
            <!-- A booklet page keeps the booklet's own order, so it does not sort. -->
            <Table bare unsorted class="booklet" ariaLabel={`${book.template.code} sheet ${pi + 1}`}>
              <thead>
                {#if page.columns.some((c) => c.group)}
                  <tr>
                    {#each headerGroups(page.columns) as g, gi (gi)}<th colspan={g.span} class="group">{g.label}</th>{/each}
                  </tr>
                {/if}
                <tr>
                  {#each page.columns as c, ci (ci)}<th>{c.label}</th>{/each}
                </tr>
              </thead>
              <tbody>
                {#each page.rows as row, ri (ri)}
                  <tr>
                    {#each row.cells as cell, ci (ci)}
                      <td class={cell.flag ?? ""}>{cellText(cell)}</td>
                    {/each}
                  </tr>
                {:else}
                  <tr><td colspan={page.columns.length} class="muted">No records this month.</td></tr>
                {/each}
              </tbody>
            </Table>
          </div>
        {/each}

        {#if book.gaps.length > 0}
          <h3>Missed checks</h3>
          <ul class="gaps">
            {#each gapsByDate(book.gaps) as g (g.date)}
              <li><span class="date">{longDate(g.date)}</span><span>{g.checks.join(", ")}</span></li>
            {/each}
          </ul>
        {/if}
        <p class="small muted">
          Layout from the {book.pack_name}, edition {book.template.source.published}. Failed and borderline readings show FAIL and CHECK. A missed check stays marked as missed: Kishtin never fills a gap in.
        </p>
      </Card>
    {/if}
  {/if}
</div>

<style>
  .head {
    align-items: flex-end;
  }
  .month {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 200px;
  }
  .month span {
    font-size: var(--font-size-sm);
    color: var(--text-secondary);
  }
  .books {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .books li {
    display: grid;
    grid-template-columns: 56px minmax(0, 1fr) auto auto;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) 0;
    border-top: var(--border-width) solid var(--border-default);
  }
  .books li:first-child {
    border-top: none;
  }
  .code {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-bold);
  }
  .about,
  .status {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  .status {
    align-items: flex-end;
  }
  .title {
    font-weight: var(--font-weight-semibold);
  }
  .actions {
    display: flex;
    gap: var(--space-3);
  }
  .muted {
    color: var(--text-secondary);
  }
  .error {
    color: var(--accent-red);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: var(--space-4);
    margin-bottom: var(--space-5);
  }
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
  }
  .sheet {
    overflow-x: auto;
    margin: var(--space-5) 0;
  }
  .sheet :global(table.booklet) {
    border-collapse: collapse;
    width: 100%;
    font-size: var(--font-size-sm);
  }
  th,
  td {
    border: var(--border-width) solid var(--border-default);
    padding: var(--space-1) var(--space-2);
    text-align: left;
    vertical-align: top;
    white-space: pre-line;
  }
  th {
    background: var(--bg-inset);
    font-weight: var(--font-weight-semibold);
  }
  td.fail {
    background: color-mix(in srgb, var(--accent-red) 14%, transparent);
    font-weight: var(--font-weight-semibold);
  }
  td.borderline {
    background: color-mix(in srgb, var(--accent-amber) 18%, transparent);
    font-weight: var(--font-weight-semibold);
  }
  td.missed {
    background: var(--bg-inset);
    color: var(--text-secondary);
    font-style: italic;
  }
  h3 {
    margin: var(--space-5) 0 var(--space-3);
    font-size: var(--font-size-md);
  }
  .gaps {
    list-style: none;
    margin: 0 0 var(--space-4);
    padding: 0;
  }
  .gaps li {
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr);
    gap: var(--space-4);
    padding: var(--space-1) 0;
  }
  .date {
    font-weight: var(--font-weight-semibold);
  }
  p {
    margin: 0;
  }
  @media (max-width: 900px) {
    .books li {
      grid-template-columns: 48px minmax(0, 1fr);
    }
    .status {
      align-items: flex-start;
    }
    .status,
    .actions {
      grid-column: 2;
    }
    .gaps li {
      grid-template-columns: minmax(0, 1fr);
      gap: 0;
    }
  }
</style>
