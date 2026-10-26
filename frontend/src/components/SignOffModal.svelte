<script lang="ts">
  import { Button, DatePicker, MetricCard, Modal, Notice, Spinner, showFlash } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { api, message, type HaccpRecord, type Schemas } from "../lib/api";
  import { gapsByDate } from "../lib/books";
  import { dublinDate, dublinTime, longDate } from "../lib/format";
  import { details, doneAt, doneBy, title } from "../lib/labels";

  interface Props {
    onsigned: (r: HaccpRecord) => void;
    onclose: () => void;
  }
  let { onsigned, onclose }: Props = $props();

  let from = $state<string | null>(null);
  let to = $state<string | null>(null);
  let preview = $state<Schemas["SignOffPreview"] | null>(null);
  let error = $state("");
  let note = $state("");
  let busy = $state(false);
  const today = dublinDate(new Date());

  async function load(f?: string, t?: string) {
    try {
      preview = await api.previewSignOff(f, t);
      if (!preview.up_to_date) {
        from = preview.from;
        to = preview.to;
      }
      error = "";
    } catch (err) {
      error = message(err);
    }
  }
  onMount(() => void load());

  function choose(which: "from" | "to", d: string) {
    if (which === "from") from = d;
    else to = d;
    if (from && to) void load(from, to);
  }

  const gaps = $derived(preview ? gapsByDate(preview.missed) : []);
  const canSign = $derived(Boolean(preview) && !preview?.up_to_date && !busy && (!preview?.note_required || note.trim() !== ""));

  async function sign() {
    if (!preview) return;
    busy = true;
    try {
      const r = await api.signOff({ from: preview.from, to: preview.to, note: note.trim() || undefined });
      showFlash(`Signed off ${longDate(preview.from)} to ${longDate(preview.to)}`, { tone: "success" });
      onsigned(r);
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }
</script>

<Modal title="Sign off records" {onclose} maxWidth="760px">
  <div class="stack">
    <p class="muted">
      Check the period, then sign. The sign-off is saved in the record chain with what the period holds right now, and fills the manager check line of every record book.
    </p>
    <div class="two">
      <label class="field"><span>From</span><DatePicker value={from} onchange={(d) => choose("from", d)} ariaLabel="From" maxDate={today} block /></label>
      <label class="field"><span>To (inclusive)</span><DatePicker value={to} onchange={(d) => choose("to", d)} ariaLabel="To" maxDate={today} block /></label>
    </div>
    {#if error}
      <Notice tone="error" message={error} />
    {:else if !preview}
      <Spinner />
    {:else if preview.up_to_date}
      <Notice tone="success" title="Up to date" message="Every day up to today is signed off. To sign off a period again, choose its dates." />
    {:else}
      {#if preview.last}
        <p class="small muted">Last sign-off: {title(preview.last)}, by {preview.last.logged_by_name} on {longDate(dublinDate(new Date(preview.last.logged_at)))}.</p>
      {/if}
      <div class="metrics">
        <MetricCard label="Records" value={String(preview.review.records)} level="inset" />
        <MetricCard label="Out of limit" value={String(preview.review.out_of_limit)} tone={preview.review.out_of_limit > 0 ? "warning" : "neutral"} level="inset" />
        <MetricCard label="Without an action" value={String(preview.review.without_action)} tone={preview.review.without_action > 0 ? "danger" : "neutral"} level="inset" />
        <MetricCard label="Missed checks" value={String(preview.review.missed)} tone={preview.review.missed > 0 ? "warning" : "neutral"} level="inset" />
      </div>
      {#each preview.overlaps as o (o.id)}
        <Notice tone="info" message={`Record #${o.seq} already signs off ${title(o)}. Signing again adds a second sign-off; both are kept.`} />
      {/each}
      {#if preview.without_action.length > 0}
        <div>
          <strong>Readings without a corrective action</strong>
          <ul>
            {#each preview.without_action as r (r.id)}
              <li>{longDate(dublinDate(new Date(doneAt(r))))} {dublinTime(doneAt(r))}: {title(r)}, {details(r)} ({r.outcome === "fail" ? "Fail" : "Check"}), by {doneBy(r)}</li>
            {/each}
          </ul>
        </div>
      {/if}
      {#if gaps.length > 0}
        <div>
          <strong>Missed checks</strong>
          <ul>
            {#each gaps.slice(0, 14) as g (g.date)}<li>{longDate(g.date)}: {g.checks.join(", ")}</li>{/each}
            {#if gaps.length > 14}<li class="muted">and {gaps.length - 14} more days</li>{/if}
          </ul>
        </div>
      {/if}
      <label class="field">
        <span>{preview.note_required ? "Note (required): what was done about the missed checks and readings without an action" : "Note (optional)"}</span>
        <textarea class="text-area" bind:value={note} aria-label="Note" placeholder="For example: closed on Monday 6th; prep fridge reading taken but not logged"></textarea>
      </label>
    {/if}
  </div>
  {#snippet footer()}
    <div class="row">
      <Button label="Cancel" onclick={onclose} />
      <Button tone="success" surface="solid" label={busy ? "Signing…" : "Sign off"} disabled={!canSign} onclick={sign} />
    </div>
  {/snippet}
</Modal>

<style>
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-4);
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
    gap: var(--space-3);
  }
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
  }
  ul {
    margin: var(--space-2) 0 0;
    padding-left: var(--space-6);
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
</style>
