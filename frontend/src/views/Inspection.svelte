<script lang="ts">
  import { Button, Card, MetricCard, Notice, SegmentedControl, SelectDropdown, Spinner, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, OfflineError, type Schemas } from "../lib/api";
  import { live } from "../lib/live.svelte";
  import { navigate } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  type Template = Schemas["Template"];
  interface Item {
    section: string;
    item: string;
    answer: "" | "yes" | "no";
    action: string;
  }

  let checklists = $state<Template[] | null>(null);
  let chosen = $state("");
  let items = $state<Item[]>([]);
  let notes = $state("");
  let error = $state("");
  let saving = $state(false);

  const canInspect = $derived(session.user?.role === "manager" || session.user?.role === "head_chef");

  function start(t: Template | undefined) {
    chosen = t?.id ?? "";
    items = (t?.checklist ?? []).flatMap((sec) => sec.items.map((it) => ({ section: sec.title ?? "", item: it, answer: "" as const, action: "" })));
    notes = "";
  }

  onMount(async () => {
    if (!canInspect) return;
    try {
      const rb = await api.recordBooks();
      checklists = rb.booklets.filter((b) => !b.off && b.template.builder === "checklist").map((b) => b.template);
      start(checklists[0]);
    } catch (err) {
      error = message(err);
    }
  });

  const counts = $derived({
    yes: items.filter((i) => i.answer === "yes").length,
    no: items.filter((i) => i.answer === "no").length,
    open: items.filter((i) => i.answer === "").length,
  });
  const missingAction = $derived(items.filter((i) => i.answer === "no" && !i.action.trim()));
  const canSave = $derived(!saving && counts.yes + counts.no > 0 && missingAction.length === 0);

  async function save() {
    saving = true;
    try {
      await api.log({
        type: "inspection",
        reading: {
          checklist: chosen,
          answers: items.map((i) => ({
            section: i.section || undefined,
            item: i.item,
            ok: i.answer === "" ? undefined : i.answer === "yes",
            action: i.action.trim() || undefined,
          })),
          notes: notes.trim() || undefined,
        },
      });
      live.saveFailed = false;
      showFlash("Inspection saved", { tone: "success" });
      start(checklists?.find((c) => c.id === chosen));
    } catch (err) {
      if (err instanceof OfflineError) live.saveFailed = true;
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Hygiene inspection</h1>
      <p>Walk the kitchen and answer each check. Anything not satisfactory needs the action taken. Checks you leave blank are saved as not checked.</p>
    </div>
    <Button label="Open Record books" onclick={() => navigate("/books")} />
  </header>

  {#if !canInspect}
    <Notice tone="info" title="Head chef or manager only" message="Inspections are done by the head chef or a manager. Ask one of them to log in on this device." />
  {:else if error}
    <Notice tone="error" message={error} />
  {:else if !checklists}
    <Spinner />
  {:else if checklists.length === 0}
    <Notice tone="info" message="The record books in use have no inspection checklist. Add one in Setup › HACCP records." />
  {:else}
    {#if checklists.length > 1}
      <label class="field">
        <span>Checklist</span>
        <SelectDropdown
          value={chosen}
          options={checklists.map((c) => ({ value: c.id, label: `${c.code} ${c.title}` }))}
          onchange={(v) => start(checklists?.find((c) => c.id === v))}
          title="Checklist"
        />
      </label>
    {/if}
    <div class="metrics">
      <MetricCard label="Satisfactory" value={String(counts.yes)} tone={counts.yes > 0 ? "success" : "neutral"} level="inset" />
      <MetricCard label="Not satisfactory" value={String(counts.no)} tone={counts.no > 0 ? "danger" : "neutral"} level="inset" />
      <MetricCard label="Not checked" value={String(counts.open)} level="inset" />
    </div>
    <Card padding="md" title={checklists.find((c) => c.id === chosen)?.title ?? "Checklist"}>
      <ol class="items">
        {#each items as it, i (i)}
          {#if it.section && (i === 0 || items[i - 1]?.section !== it.section)}
            <li class="section">{it.section}</li>
          {/if}
          <li class="item" class:bad={it.answer === "no"}>
            <span class="q">{it.item}</span>
            <SegmentedControl
              ariaLabel={`Satisfactory: ${it.item}`}
              value={it.answer}
              onchange={(v) => (it.answer = v as Item["answer"])}
              options={[
                { value: "yes", label: "Yes" },
                { value: "no", label: "No" },
                { value: "", label: "Not checked" },
              ]}
            />
            {#if it.answer === "no"}
              <div class="action">
                <TextInput bind:value={it.action} block ariaLabel={`Action taken: ${it.item}`} placeholder="Details of action taken (required)" />
              </div>
            {/if}
          </li>
        {/each}
      </ol>
      <label class="field">
        <span>Notes (optional)</span>
        <textarea class="text-area" bind:value={notes} aria-label="Notes"></textarea>
      </label>
    </Card>
    <div class="savebar">
      <span class="muted">
        {#if missingAction.length > 0}
          {missingAction.length} not satisfactory {missingAction.length === 1 ? "check needs" : "checks need"} the action taken.
        {:else}
          Saving as {session.user?.name}. The answers are kept with the record, so changing the checklist later does not change this inspection.
        {/if}
      </span>
      <Button tone="success" surface="solid" label={saving ? "Saving…" : "Save inspection"} disabled={!canSave} onclick={save} />
    </div>
  {/if}
</div>

<style>
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--space-4);
  }
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
  }
  .items {
    list-style: none;
    margin: 0 0 var(--space-5);
    padding: 0;
  }
  .section {
    margin-top: var(--space-5);
    padding-bottom: var(--space-2);
    font-weight: var(--font-weight-semibold);
    border-bottom: var(--border-width) solid var(--border-default);
  }
  .item {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-3) var(--space-5);
    padding: var(--space-3) 0;
    border-bottom: var(--border-width) solid var(--border-default);
  }
  .item.bad {
    box-shadow: inset 4px 0 0 var(--accent-red);
    padding-left: var(--space-3);
  }
  .action {
    grid-column: 1 / -1;
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
  .muted {
    color: var(--text-secondary);
  }
  @media (max-width: 760px) {
    .item {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
