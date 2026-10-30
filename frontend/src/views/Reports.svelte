<script lang="ts">
  import { Button, Card, DatePicker, MetricCard, Notice, SegmentedControl, Spinner, Table, showFlash } from "@kenn-io/kit-ui";
  import { onMount, untrack } from "svelte";
  import BarList from "../components/charts/BarList.svelte";
  import ColumnChart from "../components/charts/ColumnChart.svelte";
  import TempChart from "../components/charts/TempChart.svelte";
  import { api, message, type Schemas } from "../lib/api";
  import { can } from "../lib/can.svelte";
  import { dublinDate, dublinTime, longDate, shortDate } from "../lib/format";
  import { courseLabel } from "../lib/kitchen";
  import { details, title, typeLabel } from "../lib/labels";
  import { duration, hourLabel, lastDays, overText } from "../lib/reports";
  import { param, setParam } from "../lib/router.svelte";

  const canPass = $derived(can("orders.pass"));
  const canHACCP = $derived(can("haccp.export"));
  let tab = $state(param("tab") ?? "pass");
  const today = dublinDate(new Date());
  const initial = lastDays(28, today);
  let from = $state(param("from") ?? initial.from);
  let to = $state(param("to") ?? initial.to);

  let pass = $state<Schemas["PassReport"] | null>(null);
  let haccp = $state<Schemas["HACCPReport"] | null>(null);
  let loading = $state(false);
  let error = $state("");

  const shown = $derived(tab === "haccp" ? (canHACCP ? "haccp" : canPass ? "pass" : "") : canPass ? "pass" : canHACCP ? "haccp" : "");

  async function load() {
    if (!shown) return;
    loading = true;
    try {
      if (shown === "pass") pass = await api.reportPass(from, to);
      else haccp = await api.reportHACCP(from, to);
      error = "";
    } catch (err) {
      error = message(err);
    } finally {
      loading = false;
    }
  }

  function choose(range: number) {
    const r = lastDays(range, today);
    from = r.from;
    to = r.to;
    void load();
  }

  $effect(() => {
    void shown;
    untrack(() => {
      setParam("tab", shown || null);
      void load();
    });
  });

  onMount(() => {
    void load();
  });

  function apply() {
    if (from > to) {
      showFlash("The period ends before it starts.", { tone: "danger" });
      return;
    }
    setParam("from", from);
    setParam("to", to);
    void load();
  }

  let allLate = $state(false);
  const causeWords: Record<string, string> = {
    equipment_full: "Equipment full",
    started_late: "Started late",
    cooking_long: "Took longer than set",
    waiting_at_pass: "Waited too long at the pass",
    busy: "Equipment busy",
    unknown: "No cause recorded",
  };
  const weekdayShort: Record<string, string> = { mon: "Mon", tue: "Tue", wed: "Wed", thu: "Thu", fri: "Fri", sat: "Sat", sun: "Sun" };
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Reports</h1>
      <p>How the kitchen did over a period, worked out from what Kishtin records. Choose a period, then a report.</p>
    </div>
  </header>

  {#if !canPass && !canHACCP}
    <Notice tone="info" title="Not allowed on this login" message="Reports are for the head chef and managers, or staff a manager allows in Setup › Staff and access › Permissions." />
  {:else}
    <div class="filters">
      {#if canPass && canHACCP}
        <SegmentedControl
          ariaLabel="Report"
          value={shown}
          onchange={(v) => (tab = v)}
          options={[
            { value: "pass", label: "Pass times" },
            { value: "haccp", label: "HACCP checks" },
          ]}
        />
      {/if}
      <label class="field"><span>From</span><DatePicker value={from} onchange={(d) => (from = d)} ariaLabel="From" /></label>
      <label class="field"><span>To</span><DatePicker value={to} onchange={(d) => (to = d)} ariaLabel="To" /></label>
      <Button label="Show" onclick={apply} />
      <div class="quick">
        <Button size="sm" label="Last 7 days" onclick={() => choose(7)} />
        <Button size="sm" label="Last 28 days" onclick={() => choose(28)} />
        <Button size="sm" label="Last 90 days" onclick={() => choose(90)} />
      </div>
    </div>

    {#if error}<Notice tone="error" message={error} />{/if}

    {#if shown === "pass"}
      {#if !pass}
        {#if loading}<Spinner />{/if}
      {:else}
        {@const p = pass}
        <p class="muted small">Courses fired from {longDate(p.from)} to {longDate(p.to)} that reached the pass. On time means at the pass within a minute of the target. Times are medians: half were quicker. "9 in 10" is the time nine in ten were quicker than.</p>
        <div class="metrics">
          <MetricCard label="Courses" value={String(p.courses)} level="inset" />
          <MetricCard label="On time" value={p.courses ? `${p.on_time_percent}%` : "–"} meta={`${p.late} late`} level="inset" />
          <MetricCard label="Fire to pass" value={p.ticket.count ? duration(p.ticket.median_seconds) : "–"} meta={p.ticket.count ? `9 in 10: ${duration(p.ticket.p90_seconds)}` : ""} level="inset" />
          <MetricCard label="Waiting at the pass" value={p.pass_wait.count ? duration(p.pass_wait.median_seconds) : "–"} meta={p.pass_wait.count ? `9 in 10: ${duration(p.pass_wait.p90_seconds)}` : ""} level="inset" />
        </div>

        {#if p.courses === 0}
          <Notice tone="info" message="No course was fired and finished in this period." />
        {:else}
          <div class="grid">
            <Card padding="md" title="Why courses were late" meta={`${p.late} late`}>
              <BarList
                rows={p.late_by_cause.map((c) => ({ label: c.label, value: c.count }))}
                ariaLabel="Late courses by cause"
                format={(v) => `${v} ${v === 1 ? "course" : "courses"}`}
                empty="No course was late."
              />
              <p class="muted small note">The cause the scheduler gave the last time it saw the course at risk or late.</p>
            </Card>
            <Card padding="md" title="On time by hour fired">
              <ColumnChart
                cols={p.by_hour.map((h) => ({
                  label: hourLabel(h.hour),
                  short: String(h.hour),
                  value: h.courses ? Math.round((h.on_time * 100) / h.courses) : 0,
                  note: `${h.on_time} of ${h.courses} courses on time`,
                }))}
                ariaLabel="Share of courses on time by hour fired"
                max={100}
                unit="%"
              />
            </Card>
          </div>

          <Card padding="md" title="By course">
            <Table bare unsorted ariaLabel="Pass times by course">
              <thead><tr><th>Course</th><th>Courses</th><th>On time</th><th>Fire to pass (median)</th><th>9 in 10 within</th><th>Waiting at the pass (median)</th></tr></thead>
              <tbody>
                {#each p.by_course as c (c.course)}
                  <tr>
                    <td>{courseLabel(c.course)}</td>
                    <td class="num">{c.courses}</td>
                    <td class="num">{c.on_time_percent}%</td>
                    <td class="num">{duration(c.ticket.median_seconds)}</td>
                    <td class="num">{duration(c.ticket.p90_seconds)}</td>
                    <td class="num">{c.pass_wait.count ? duration(c.pass_wait.median_seconds) : "–"}</td>
                  </tr>
                {/each}
              </tbody>
            </Table>
          </Card>

          <Card padding="md" title="Dishes: actual time against set time" meta="From start to ready">
            {#if p.dishes.length === 0}
              <p class="muted">No dish was started and finished on a station screen in this period.</p>
            {:else}
              <Table bare unsorted ariaLabel="Dish times">
                <thead><tr><th>Dish</th><th>Set</th><th>Actual (median)</th><th>9 in 10 within</th><th>Compared with set</th><th>Cooked</th></tr></thead>
                <tbody>
                  {#each p.dishes as d (d.dish_id)}
                    <tr>
                      <td>{d.dish_name}</td>
                      <td class="num">{duration(d.set_seconds)}</td>
                      <td class="num">{duration(d.actual.median_seconds)}</td>
                      <td class="num">{duration(d.actual.p90_seconds)}</td>
                      <td>{overText(d.over_seconds)}</td>
                      <td class="num">{d.actual.count}</td>
                    </tr>
                  {/each}
                </tbody>
              </Table>
              <p class="muted small note">Set time is the dish's cook, rest and plate time. A dish much slower than set makes the scheduler plan too late; change its times in Setup › Kitchen › Menu.</p>
            {/if}
          </Card>

          {#if p.late_courses.length > 0}
            <Card padding="md" title="Late courses" meta={`${p.late} late${p.late_courses.length < p.late ? `; the latest ${p.late_courses.length} listed` : ""}`}>
              <Table bare unsorted ariaLabel="Late courses">
                <thead><tr><th>Day</th><th>Order</th><th>Course</th><th>Due</th><th>At the pass</th><th>Late by</th><th>Why</th></tr></thead>
                <tbody>
                  {#each allLate ? p.late_courses : p.late_courses.slice(0, 10) as l (l.order_id + l.course)}
                    <tr>
                      <td>{shortDate(dublinDate(new Date(l.fired_at)))}</td>
                      <td>{l.label}</td>
                      <td>{courseLabel(l.course, false)}</td>
                      <td class="num">{dublinTime(l.target)}</td>
                      <td class="num">{dublinTime(l.ready_at)}</td>
                      <td class="num">{duration(l.late_by_seconds)}</td>
                      <td title={l.reason ?? ""}>{causeWords[l.cause] ?? l.cause}</td>
                    </tr>
                  {/each}
                </tbody>
              </Table>
              {#if p.late_courses.length > 10}
                <div class="more">
                  <Button size="sm" label={allLate ? "Show the latest 10" : `Show all ${p.late_courses.length}`} onclick={() => (allLate = !allLate)} />
                </div>
              {/if}
            </Card>
          {/if}
        {/if}
      {/if}
    {:else if shown === "haccp"}
      {#if !haccp}
        {#if loading}<Spinner />{/if}
      {:else}
        {@const h = haccp}
        <p class="muted small">{longDate(h.from)} to {longDate(h.to)}: {h.checks_done} of {h.checks_due} fridge and freezer checks done. Missed checks are counted the way the record books count them, from the first day records were kept. Cleaning counts days that are over.</p>
        <div class="metrics">
          <MetricCard label="Fridge and freezer checks" value={h.checks_due ? `${h.checks_percent}%` : "–"} meta={`${h.checks_missed} missed`} level="inset" />
          <MetricCard label="Cleaning tasks done" value={h.cleaning_due ? `${h.cleaning_percent}%` : "–"} meta={`${h.cleaning_done} of ${h.cleaning_due}`} level="inset" />
          <MetricCard label="Records" value={String(h.records)} meta={`${h.out_of_limit} out of limit`} level="inset" />
          <MetricCard label="Out of limit, no action" value={String(h.without_action)} level="inset" />
        </div>

        <div class="grid">
          <Card padding="md" title="Missed checks by day of the week">
            <ColumnChart
              cols={h.missed_by_weekday.map((d) => ({ label: d.label, short: weekdayShort[d.key] ?? d.label, value: d.count, note: d.of ? `${d.count} of ${d.of} checks due` : "No checks due" }))}
              ariaLabel="Missed fridge and freezer checks by day of the week"
            />
          </Card>
          <Card padding="md" title="Missed checks by fridge and freezer">
            <BarList rows={h.missed_by_unit.map((u) => ({ label: u.label, value: u.count }))} ariaLabel="Missed checks by unit" format={(v) => `${v} missed`} empty="No check was missed." />
          </Card>
          <Card padding="md" title="Daily cleaning tasks not done">
            <BarList
              rows={h.cleaning_missed_by_task.map((t) => ({ label: t.label, value: t.count }))}
              ariaLabel="Cleaning tasks not done by task"
              format={(v) => `${v} ${v === 1 ? "day" : "days"}`}
              empty="Every cleaning task was done."
            />
          </Card>
          {#if h.cleaning_overdue_by_task.length > 0}
            <Card padding="md" title="Weekly and longer tasks overdue">
              <BarList
                rows={h.cleaning_overdue_by_task.map((t) => ({ label: t.label, value: t.count }))}
                ariaLabel="Days each weekly or longer cleaning task was overdue"
                format={(v) => `${v} ${v === 1 ? "day" : "days"} overdue`}
                empty=""
              />
            </Card>
          {/if}
          <Card padding="md" title="Out of limit by kind">
            <BarList rows={h.out_of_limit_by_type.map((t) => ({ label: typeLabel(t.key), value: t.count }))} ariaLabel="Out-of-limit records by kind" empty="No reading was out of limit." />
          </Card>
        </div>

        <Card padding="md" title="Highest reading each day">
          {#if h.temperatures.length === 0}
            <p class="muted">No fridges or freezers are on the list.</p>
          {:else}
            <div class="multiples">
              {#each h.temperatures as u (u.unit)}
                <TempChart unit={u.unit} days={u.days} limit={u.max_c ?? null} />
              {/each}
            </div>
          {/if}
        </Card>

        {#if h.recent.length > 0}
          <Card padding="md" title="Out-of-limit records" meta={h.recent.length < h.out_of_limit ? `Latest ${h.recent.length} of ${h.out_of_limit}` : String(h.out_of_limit)}>
            <Table bare unsorted ariaLabel="Out-of-limit records">
              <thead><tr><th>When</th><th>Record</th><th>Reading</th><th>Result</th><th>Corrective action</th></tr></thead>
              <tbody>
                {#each h.recent as r (r.id)}
                  {@const when = new Date(r.reading.taken_at ?? r.logged_at)}
                  <tr>
                    <td class="num">{shortDate(dublinDate(when))}, {dublinTime(when)}</td>
                    <td><a href={`/records?id=${r.id}`}>{title(r)}</a></td>
                    <td>{details(r)}</td>
                    <td>{r.outcome === "fail" ? "Fail" : "Borderline"}</td>
                    <td>{r.corrective_action || "None"}</td>
                  </tr>
                {/each}
              </tbody>
            </Table>
          </Card>
        {/if}
      {/if}
    {/if}
  {/if}
</div>

<style>
  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--space-4);
  }
  .quick {
    display: flex;
    gap: var(--space-2);
    margin-left: auto;
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--space-4);
  }
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
    gap: var(--space-5);
    align-items: start;
  }
  .multiples {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: var(--space-6);
  }
  .more {
    margin-top: var(--space-3);
  }
  .note {
    margin-top: var(--space-3);
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
  @media (max-width: 640px) {
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
    .quick {
      margin-left: 0;
    }
  }
</style>
