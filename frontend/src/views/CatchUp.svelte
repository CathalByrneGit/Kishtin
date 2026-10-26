<script lang="ts">
  import { Button, Card, Checkbox, Chip, DatePicker, Notice, Spinner, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { untrack } from "svelte";
  import { api, message, OfflineError, type NewRecord, type Schemas } from "../lib/api";
  import { can } from "../lib/can.svelte";
  import { entryFor, filled, gapLabel, toRecord, type Entry } from "../lib/catchup";
  import { longDate } from "../lib/format";
  import { live, onChange } from "../lib/live.svelte";
  import { session } from "../lib/session.svelte";

  let view = $state<Schemas["CatchUp"] | null>(null);
  let from = $state("");
  let to = $state("");
  let entries = $state<Entry[]>([]);
  let doneBy = $state("");
  let error = $state("");
  let busy = $state(false);
  let checks = $state<Map<Entry, Schemas["PaperCheck"]>>(new Map());
  let checkedFor = $state("");

  const allowed = $derived(can("haccp.backdate"));
  const intoSignedOff = $derived(can("haccp.signed_off"));

  async function load(f = from, t = to) {
    try {
      view = await api.catchUp(f || undefined, t || undefined);
      from = view.from;
      to = view.to;
      entries = view.gaps.map(entryFor);
      checks = new Map();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  // Loads once permissions are known: on a page reload they arrive after
  // the page opens.
  $effect(() => {
    if (!allowed) return;
    untrack(() => void load());
    return onChange((k) => {
      // Someone else logging a check fills a gap; reload only when nothing is typed.
      if ((k === "record" || k === "settings") && !entries.some(filled)) void load();
    });
  });

  const days = $derived([...new Set(entries.map((e) => e.gap.date))]);
  const toSave = $derived(entries.filter((e) => filled(e) && (!e.gap.signed_off || intoSignedOff)));
  const built = $derived(toSave.map((e) => ({ entry: e, rec: toRecord(e, doneBy) })));
  const key = $derived(JSON.stringify([doneBy, toSave.map((e) => [e.gap, e.temp, e.done, e.time, e.action])]));
  const firstProblem = $derived(built.find((b) => typeof b.rec === "string"));
  const ready = $derived(toSave.length > 0 && checkedFor === key && [...checks.values()].every((c) => !c.problem));

  async function check() {
    if (firstProblem) {
      showFlash(`${longDate(firstProblem.entry.gap.date)}, ${gapLabel(firstProblem.entry.gap)}: ${firstProblem.rec}`, { tone: "danger", durationMs: 6000 });
      return;
    }
    busy = true;
    try {
      const res = await api.previewCatchUp({ rows: built.map((b) => b.rec as NewRecord) });
      checks = new Map(res.rows.map((r, i) => [built[i]!.entry, r]));
      checkedFor = key;
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  async function save() {
    if (!ready) return;
    busy = true;
    try {
      const res = await api.saveCatchUp({ rows: built.map((b) => b.rec as NewRecord) });
      live.saveFailed = false;
      showFlash(`${res.records.length} ${res.records.length === 1 ? "record" : "records"} saved`, { tone: "success" });
      await load();
    } catch (err) {
      if (err instanceof OfflineError) live.saveFailed = true;
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  const resultTone = (c: Schemas["PaperCheck"] | undefined) => {
    const o = c?.check?.result.outcome;
    return c?.problem ? "danger" : o === "pass" ? "success" : o === "fail" ? "danger" : o === "borderline" ? "warning" : "neutral";
  };
  const resultText = (c: Schemas["PaperCheck"]) =>
    c.problem ? "Fix" : ({ pass: "Pass", fail: "Fail", borderline: "Check", recorded: "Done" } as Record<string, string>)[c.check?.result.outcome ?? ""] ?? "";
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Catch up</h1>
      <p>Daily checks that were due and have no record. Fill in the ones that were done, with the time they were done, then save them together. Leave the rest blank.</p>
    </div>
  </header>

  {#if !allowed}
    <Notice tone="info" title="Not allowed on this login" message="Logging checks done earlier is turned off for you. A manager can change this in Setup › Staff and access › Permissions." />
  {:else}
    <div class="filters">
      <label class="field"><span>From</span><DatePicker value={from} onchange={(d) => (from = d)} ariaLabel="From" /></label>
      <label class="field"><span>To</span><DatePicker value={to} onchange={(d) => (to = d)} ariaLabel="To" /></label>
      <Button label="Show" onclick={() => load(from, to)} />
      <label class="field grow"><span>Done by (if not you)</span><TextInput bind:value={doneBy} block placeholder="Name or initials, for every entry" /></label>
    </div>

    {#if error}<Notice tone="error" message={error} />{/if}
    {#if !view}
      {#if !error}<Spinner />{/if}
    {:else if entries.length === 0}
      <Notice
        tone="success"
        title="Nothing to catch up"
        message={`Every fridge and freezer check and cleaning task from ${longDate(view.from)} to ${longDate(view.to)} has a record.`}
      />
    {:else}
      {#if view.since && view.since > view.from}
        <p class="muted small">Records started on {longDate(view.since)}: days before that are not listed. Bring those in with Paper import.</p>
      {/if}
      {#each days as day (day)}
        {@const dayEntries = entries.filter((e) => e.gap.date === day)}
        {@const signed = dayEntries.some((e) => e.gap.signed_off)}
        <Card padding="md" title={longDate(day)} meta={`${dayEntries.length} with no record`}>
          {#if signed}
            <Notice
              tone="info"
              message={intoSignedOff
                ? "This day is already signed off. Records added now still count."
                : "This day is already signed off. Adding to it needs permission: a manager can log in, or allow it in Setup › Staff and access › Permissions."}
            />
          {/if}
          <ul class="entries">
            {#each dayEntries as e (gapLabel(e.gap))}
              {@const c = checkedFor === key ? checks.get(e) : undefined}
              {@const locked = e.gap.signed_off && !intoSignedOff}
              <li class:done={filled(e)}>
                <span class="what">
                  <strong>{gapLabel(e.gap)}</strong>
                  <span class="muted small">{e.gap.type === "cleaning" ? "Cleaning" : e.gap.type === "freezer" ? "Freezer" : "Fridge"}</span>
                </span>
                {#if e.gap.type === "cleaning"}
                  <Checkbox checked={e.done} disabled={locked} label="Done" onchange={(on) => (e.done = on)} />
                {:else}
                  <span class="temp"><TextInput bind:value={e.temp} disabled={locked} placeholder="°C" ariaLabel={`${gapLabel(e.gap)} temperature`} /></span>
                {/if}
                <span class="time"><TextInput bind:value={e.time} disabled={locked} placeholder="HH:MM" ariaLabel={`${gapLabel(e.gap)} time done`} /></span>
                <span class="result">{#if c}<Chip uppercase={false} tone={resultTone(c)}>{resultText(c)}</Chip>{/if}</span>
                {#if (c?.check?.needs_corrective_action || e.action) && filled(e)}
                  <span class="action"><TextInput bind:value={e.action} block placeholder="Corrective action taken (required)" ariaLabel={`${gapLabel(e.gap)} corrective action`} /></span>
                {/if}
                {#if c?.problem && !c.check?.needs_corrective_action}
                  <span class="problem">{c.problem}</span>
                {/if}
              </li>
            {/each}
          </ul>
        </Card>
      {/each}

      <div class="savebar">
        <span class="muted">
          {#if toSave.length === 0}
            Fill in a temperature or tick Done for each check that was done.
          {:else if ready}
            Every entry checks out. Saving adds {toSave.length} {toSave.length === 1 ? "record" : "records"}, logged by {session.user?.name}.
          {:else}
            {toSave.length} {toSave.length === 1 ? "entry" : "entries"} filled in. Check them to see each result before saving.
          {/if}
        </span>
        <div class="row">
          <Button label={busy && !ready ? "Checking…" : "Check entries"} disabled={toSave.length === 0 || busy} onclick={check} />
          <Button tone="success" surface="solid" label={busy && ready ? "Saving…" : `Save ${toSave.length} ${toSave.length === 1 ? "record" : "records"}`} disabled={!ready || busy} onclick={save} />
        </div>
      </div>
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
  .grow {
    flex: 1 1 240px;
  }
  .entries {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 110px 96px 84px;
    align-items: center;
    gap: var(--space-2) var(--space-4);
    padding: var(--space-3) 0;
    border-bottom: var(--border-width) solid var(--border-default);
  }
  li:last-child {
    border-bottom: 0;
  }
  li.done {
    box-shadow: inset 4px 0 0 var(--accent-blue);
    padding-left: var(--space-3);
  }
  .what {
    display: flex;
    flex-direction: column;
  }
  .temp :global(.kit-text-input),
  .time :global(.kit-text-input) {
    width: 100%;
  }
  .action,
  .problem {
    grid-column: 1 / -1;
  }
  .problem {
    color: var(--accent-red);
    font-size: var(--font-size-sm);
  }
  .savebar {
    position: sticky;
    bottom: var(--space-4);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-4);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
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
    li {
      grid-template-columns: minmax(0, 1fr) 90px 80px;
    }
    .result {
      grid-column: 1 / -1;
    }
  }
</style>
