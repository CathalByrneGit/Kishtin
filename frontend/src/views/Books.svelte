<script lang="ts">
  import { can, canWithPassword } from "../lib/can.svelte";
  import { Button, Card, Chip, MetricCard, Modal, Notice, SelectDropdown, Spinner, Table, showFlash } from "@kenn-io/kit-ui";
  import { Eye, FileText, PenLine } from "@lucide/svelte";
  import SignOffModal from "../components/SignOffModal.svelte";
  import Login from "../screens/Login.svelte";
  import { title } from "../lib/labels";
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
  let info = $state<Schemas["RecordBooks"] | null>(null);
  let filled = $state<Record<string, Book | string>>({});
  let selected = $state<string | null>(param("book"));
  let error = $state("");
  let busy = $state<string | null>(null);
  let signStatus = $state<Schemas["SignOffPreview"] | null>(null);
  let signing = $state(false);
  let managerLogin = $state(false);
  // Bumped after a sign-off, so the booklets fill in again with it.
  let version = $state(0);

  const canExport = $derived(can("haccp.export"));

  function loadSignStatus() {
    api
      .previewSignOff()
      .then((r) => (signStatus = r))
      .catch(() => (signStatus = null));
  }

  onMount(() => {
    if (!canExport) return;
    api
      .recordBooks()
      .then((r) => (info = r))
      .catch((err: unknown) => (error = message(err)));
    loadSignStatus();
  });

  function signed() {
    signing = false;
    loadSignStatus();
    version++;
  }

  // Fill in every booklet Kishtin can, for the chosen month, so each shows
  // its missed checks before anyone opens it.
  $effect(() => {
    const m = month;
    const list = shown;
    void version;
    if (!info) return;
    filled = {};
    let gone = false;
    for (const bk of list) {
      const t = bk.template;
      if (!t.builder || locked(t)) continue;
      api
        .book(t.id, m)
        .then((b) => {
          if (!gone) filled[t.id] = b;
        })
        .catch((err: unknown) => {
          if (!gone) filled[t.id] = message(err);
        });
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

  /** locked is a booklet this login may not open: fitness to work is for a manager with a password. */
  function locked(t: Template): boolean {
    return t.builder === "fitness" && !can("haccp.fitness");
  }

  const shown = $derived((info?.booklets ?? []).filter((b) => !b.off));
  const hidden = $derived((info?.booklets ?? []).filter((b) => b.off).length);
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
  {:else if !info}
    <Spinner />
  {:else}
    <Card padding="md" title="Manager sign-off">
      <div class="signoff">
        <div class="stack">
          {#if signStatus?.last}
            <span>Last signed off: <strong>{title(signStatus.last)}</strong>, by {signStatus.last.logged_by_name}.</span>
          {:else if signStatus}
            <span>No records signed off yet.</span>
          {/if}
          {#if signStatus?.up_to_date}
            <span class="small muted">Every day up to today is signed off.</span>
          {:else if signStatus && signStatus.from <= signStatus.to}
            <span class="small muted">
              Next to sign off: {longDate(signStatus.from)} to {longDate(signStatus.to)} · {signStatus.review.records} records, {signStatus.review.out_of_limit} out of limit, {signStatus.review.missed} missed checks.
            </span>
          {/if}
          <span class="small muted">A sign-off fills the manager or supervisor check line of every record book.</span>
        </div>
        <div class="stack end">
          <Button tone="info" surface="solid" label="Sign off records" disabled={!can("haccp.sign_off")} onclick={() => (signing = true)}><PenLine size={16} /></Button>
          {#if !can("haccp.sign_off") && canWithPassword("haccp.sign_off")}
            <Button size="sm" label="Log in with a password to sign" onclick={() => (managerLogin = true)} />
          {:else if !can("haccp.sign_off")}
            <span class="small muted">A manager signs off, or can allow you in Setup › Permissions.</span>
          {/if}
        </div>
      </div>
    </Card>
    {@const p = info.bundle}
      <Card padding="md" title={p.name} meta={`${p.country} · ${p.issuer}`}>
        <ul class="books">
          {#each shown as bk (bk.template.id)}
            {@const t = bk.template}
            {@const b = filled[t.id]}
            <li class:muted={!t.builder}>
              <span class="code num">{t.code}</span>
              <div class="about">
                <span class="title">{t.title}</span>
                <span class="small muted">
                  {#if bk.custom}Added by this restaurant{:else if t.source}Edition {t.source.published}{/if}{#if bk.changed.length > 0}{bk.custom || t.source ? " · " : ""}Changed here: {bk.changed.join(", ")}{/if}
                </span>
                {#if !t.builder}
                  <span class="small">{t.missing}</span>
                {:else if locked(t)}
                  <span class="small">Fitness to work records are health information: only a manager logged in with a password can open them.</span>
                {:else if typeof b === "string"}
                  <span class="small error">{b}</span>
                {/if}
              </div>
              <div class="status">
                {#if !t.builder}
                  <Chip uppercase={false} tone="neutral">Not yet</Chip>
                {:else if locked(t)}
                  <Chip uppercase={false} tone="neutral">Managers only</Chip>
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
              {#if t.builder && !locked(t)}
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
        <p class="small muted setup">
          {hidden > 0 ? `${hidden} booklet${hidden === 1 ? " is" : "s are"} turned off. ` : ""}Choose your country's bundle, turn booklets off, or change titles and columns in
          <a href="/setup/records">Setup › HACCP records</a>.
        </p>
      </Card>

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
          {#if page.sheet && (pi === 0 || book.pages[pi - 1]?.sheet !== page.sheet)}
            <h3 class="day">{page.day && book.template.builder === "daily" ? longDate(page.day) : page.sheet_label}</h3>
          {/if}
          {#if page.title}<h4>{page.title}</h4>{/if}
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
                  {#if row.heading}
                    <tr><th class="rowhead" colspan={page.columns.length}>{row.cells[0]?.text ?? ""}</th></tr>
                  {:else}
                    <tr>
                      {#each row.cells as cell, ci (ci)}
                        <td class={cell.flag ?? ""}>{cellText(cell)}</td>
                      {/each}
                    </tr>
                  {/if}
                {:else}
                  <tr><td colspan={page.columns.length} class="muted">{page.sheet ? "None." : "No records this month."}</td></tr>
                {/each}
              </tbody>
            </Table>
          </div>
        {/each}

        {#if (book.template.notes?.length ?? 0) > 0}
          <ul class="notes small muted">
            {#each book.template.notes ?? [] as n (n)}<li>{n}</li>{/each}
          </ul>
        {/if}
        {#if book.gaps.length > 0}
          <h3>Missed checks</h3>
          <ul class="gaps">
            {#each gapsByDate(book.gaps) as g (g.date)}
              <li><span class="date">{longDate(g.date)}</span><span>{g.checks.join(", ")}</span></li>
            {/each}
          </ul>
        {/if}
        {#if book.template.manager_check}
          <h3>Manager check</h3>
          {#if book.sign_offs.length > 0}
            <ul class="gaps">
              {#each book.sign_offs as so (so.seq)}
                <li><span class="date">Signed by {so.by}</span><span>{longDate(so.date)} · covers {longDate(so.from)} to {longDate(so.to)} · record #{so.seq}{so.note ? ` · ${so.note}` : ""}</span></li>
              {/each}
            </ul>
          {/if}
          {#if book.unsigned.length > 0}
            <p class="small">Not signed off: {book.unsigned.map((u) => (u.from === u.to ? longDate(u.from) : `${longDate(u.from)} to ${longDate(u.to)}`)).join(", ")}.</p>
          {:else if book.sign_offs.length === 0}
            <p class="small muted">No sign-offs for this month.</p>
          {/if}
        {/if}
        <p class="small muted">
          Layout from the {book.bundle_name}{book.template.source ? `, edition ${book.template.source.published}` : ""}{book.changed.length > 0 ? `, with this restaurant's changes to the ${book.changed.join(", ")}` : ""}. Failed and borderline readings show FAIL and CHECK. A missed check stays marked as missed: Kishtin never fills a gap in.
        </p>
      </Card>
    {/if}
  {/if}
</div>

{#if signing}
  <SignOffModal onsigned={signed} onclose={() => (signing = false)} />
{/if}

{#if managerLogin}
  <Modal title="Password login" onclose={() => (managerLogin = false)} maxWidth="520px">
    <p class="muted">This device will be logged in with a password for 30 minutes.</p>
    <Login passwordOnly password onloggedin={() => (managerLogin = false)} />
  </Modal>
{/if}

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
  .setup {
    margin-top: var(--space-4);
  }
  .signoff {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--space-5);
  }
  .end {
    align-items: flex-end;
  }
  .rowhead {
    background: var(--bg-inset);
  }
  .day {
    margin: var(--space-6) 0 0;
    padding-top: var(--space-4);
    border-top: var(--border-width) solid var(--border-default);
  }
  h4 {
    margin: var(--space-4) 0 0;
    font-size: var(--font-size-md);
  }
  .notes {
    margin: 0 0 var(--space-4);
    padding-left: var(--space-6);
  }
  a {
    color: var(--accent-blue);
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
