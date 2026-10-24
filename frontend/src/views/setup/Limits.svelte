<script lang="ts">
  import { Button, Card, Chip, Notice, SegmentedControl, Spinner, Table, TableHeaderCell, TableSort, Toggle, showFlash } from "@kenn-io/kit-ui";
  import { History, Plus } from "@lucide/svelte";
  import { onMount } from "svelte";
  import LimitEditor from "../../components/LimitEditor.svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import ReviewChanges, { type Preview } from "../../components/ReviewChanges.svelte";
  import { api, download, message, type Lists, type Schemas } from "../../lib/api";
  import { dublinDate, dublinTime, shortDate } from "../../lib/format";
  import { CHECK_LABELS, limitWords } from "../../lib/labels";
  import { onChange } from "../../lib/live.svelte";
  import { isManagerPassword } from "../../lib/session.svelte";

  type LV = Schemas["LimitVersion"];

  let limits = $state<Schemas["Limits"] | null>(null);
  let lists = $state<Lists | null>(null);
  let rules = $state<Schemas["VersionedRules"] | null>(null);
  let guidance = $state<Schemas["VersionedGuidance"] | null>(null);
  let staffNames = $state(new Map<string, string>());
  let error = $state("");
  let tab = $state("current");

  let editing = $state<{ start: LV | null } | null>(null);
  let rulesDraft = $state<Schemas["Rules"] | null>(null);
  let review = $state<Preview | null>(null);
  let saving = $state(false);

  const canEdit = $derived(isManagerPassword());

  async function load() {
    try {
      const [l, li, r, g, st] = await Promise.all([api.limits(), api.lists(), api.rules(), api.guidance(), api.staff()]);
      limits = l;
      lists = li.value;
      rules = r;
      guidance = g;
      staffNames = new Map(st.staff.map((s) => [s.id, s.name]));
      rulesDraft = { ...r.value };
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((kind) => {
      if (kind === "settings" || kind === "limit") void load();
    });
  });

  const builtin = (l: LV) => l.id.startsWith("fsai-");
  const scopeRank: Record<string, number> = { default: 0, unit: 1, category: 2, dish: 3 };
  const checkOrder = Object.keys(CHECK_LABELS);
  const sorter = new TableSort<LV>(
    {
      check: (l) => checkOrder.indexOf(l.check) * 10 + (scopeRank[l.scope.kind] ?? 9),
      scope: (l) => `${scopeRank[l.scope.kind] ?? 9}${l.scope.ref ?? ""}`,
      limit: (l) => limitWords(l),
      why: (l) => (builtin(l) ? "" : (l.reason ?? "").toLowerCase()),
      since: (l) => l.created_at,
    },
    { key: "check" },
  );
  const historySorter = new TableSort<LV>(
    {
      when: (l) => l.created_at,
      check: (l) => checkOrder.indexOf(l.check),
      scope: (l) => `${scopeRank[l.scope.kind] ?? 9}${l.scope.ref ?? ""}`,
      limit: (l) => limitWords(l),
      who: (l) => (staffNames.get(l.created_by) ?? l.created_by).toLowerCase(),
      status: (l) => (limits?.current.some((c) => c.id === l.id) ? 0 : 1),
    },
    { key: "when", direction: "desc" },
  );

  const rows = $derived(sorter.sort(limits?.current ?? []));
  const history = $derived(historySorter.sort(limits?.history ?? []));

  function scopeText(l: LV): string {
    if (l.scope.kind === "default") return "Every one";
    const what = { unit: "Unit", category: "Category", dish: "Dish" }[l.scope.kind] ?? l.scope.kind;
    return `${what}: ${l.scope.ref}`;
  }

  function when(t: string): string {
    return `${shortDate(dublinDate(new Date(t)))} ${new Date(t).getFullYear()}, ${dublinTime(t)}`;
  }

  const rulesDirty = $derived(rules !== null && rulesDraft !== null && JSON.stringify(rules.value) !== JSON.stringify(rulesDraft));

  async function reviewRules() {
    if (!rulesDraft) return;
    try {
      review = await api.previewRules({ value: rulesDraft, reason: "" });
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function saveRules(reason: string, planRef: string) {
    if (!rulesDraft) return;
    saving = true;
    try {
      await api.saveRules({ value: rulesDraft, reason, plan_ref: planRef || undefined });
      review = null;
      showFlash("Saved", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }

  async function exportHistory() {
    try {
      const { blob, name } = await api.exportLimits();
      download(blob, name);
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Limits and rules</h1>
      <p>
        The critical limits every reading is checked against. A limit applies at the most specific level that has one: dish, then food
        category, then unit, then the check type. Changes make a new version; records already saved keep their result.
      </p>
    </div>
    <div class="row">
      <Button label="Limit history (CSV)" onclick={exportHistory}><History size={16} /></Button>
      {#if canEdit}
        <Button tone="info" surface="solid" label="New limit" disabled={!lists} onclick={() => (editing = { start: null })}><Plus size={16} /></Button>
      {/if}
    </div>
  </header>

  <ManagerGate what="Limits" viewable>
    {#if error}<Notice tone="error" message={error} />{/if}

    {#if !limits || !rules || !guidance}
      {#if !error}<Spinner />{/if}
    {:else}
      <Notice
        tone="info"
        title={`Measured against: ${guidance.value.name}`}
        message={`A limit less strict than this guidance needs a HACCP plan reference. ${guidance.builtin ? "This is the FSAI Safe Catering guidance built into Kishtin." : `Version saved for: ${guidance.version.reason}.`} Change the guidance with kishtin settings set guidance.`}
      />

      <div class="tabs">
        <SegmentedControl
          value={tab}
          onchange={(v) => (tab = v)}
          ariaLabel="Show"
          options={[
            { value: "current", label: `In use (${limits.current.length})` },
            { value: "history", label: `History (${limits.history.length})` },
          ]}
        />
      </div>

      {#if tab === "current"}
        <Card padding="none">
          <Table ariaLabel="Limits in use">
            {#snippet header()}
              <TableHeaderCell label="Check" sort={sorter} column="check" />
              <TableHeaderCell label="Applies to" sort={sorter} column="scope" />
              <TableHeaderCell label="Limit" sort={sorter} column="limit" />
              <TableHeaderCell label="Why" sort={sorter} column="why" />
              <TableHeaderCell label="Since" sort={sorter} column="since" />
              <TableHeaderCell><span class="kit-sr-only">Change</span></TableHeaderCell>
            {/snippet}
            {#each rows as l (l.id)}
              <tr>
                <td class="nowrap"><strong>{CHECK_LABELS[l.check] ?? l.check}</strong></td>
                <td>
                  {scopeText(l)}
                  {#if l.non_food}<Chip size="sm" tone="muted" uppercase={false}>Non-food</Chip>{/if}
                </td>
                <td class="num">{limitWords(l)}</td>
                <td>
                  {#if builtin(l)}
                    <span class="muted">FSAI default</span>
                  {:else}
                    {l.reason}
                    {#if l.plan_ref}<div class="muted small">Plan: {l.plan_ref}</div>{/if}
                  {/if}
                </td>
                <td class="nowrap muted small">{builtin(l) ? "Built in" : when(l.created_at)}</td>
                <td class="end">
                  {#if canEdit}<Button size="sm" label="Change" onclick={() => (editing = { start: l })} />{/if}
                </td>
              </tr>
            {/each}
          </Table>
        </Card>
      {:else}
        <Card padding="none">
          <Table ariaLabel="Limit history">
            {#snippet header()}
              <TableHeaderCell label="When" sort={historySorter} column="when" />
              <TableHeaderCell label="Check" sort={historySorter} column="check" />
              <TableHeaderCell label="Applies to" sort={historySorter} column="scope" />
              <TableHeaderCell label="Limit" sort={historySorter} column="limit" />
              <TableHeaderCell label="Who and why" sort={historySorter} column="who" />
              <TableHeaderCell label="Status" sort={historySorter} column="status" />
            {/snippet}
            {#each history as l (l.id)}
              {@const inUse = limits.current.some((c) => c.id === l.id)}
              <tr>
                <td class="nowrap small">{builtin(l) ? "Built in" : when(l.created_at)}</td>
                <td class="nowrap">{CHECK_LABELS[l.check] ?? l.check}</td>
                <td>{scopeText(l)}</td>
                <td class="num">{limitWords(l)}</td>
                <td>
                  {#if builtin(l)}
                    <span class="muted">FSAI default</span>
                  {:else}
                    {staffNames.get(l.created_by) ?? l.created_by}: {l.reason}
                    {#if l.plan_ref}<div class="muted small">Plan: {l.plan_ref}</div>{/if}
                  {/if}
                </td>
                <td><Chip size="sm" tone={inUse ? "success" : "muted"} uppercase={false}>{inUse ? "In use" : "Replaced"}</Chip></td>
              </tr>
            {/each}
          </Table>
        </Card>
      {/if}

      {#if rulesDraft}
        <Card padding="md" title="Food-safety rules" meta={rules.builtin ? "Defaults" : `Version saved for: ${rules.version.reason}`}>
          <div class="stack">
            <Toggle
              checked={rulesDraft.borderline_needs_action}
              disabled={!canEdit}
              label="A borderline reading needs a corrective action"
              onchange={(on) => rulesDraft && (rulesDraft.borderline_needs_action = on)}
            />
            <Toggle
              checked={rulesDraft.listed_names_only}
              disabled={!canEdit}
              label="Fridges, freezers, categories and tasks must come from the lists"
              onchange={(on) => rulesDraft && (rulesDraft.listed_names_only = on)}
            />
            <p class="muted small">A failed reading always needs a corrective action. That rule cannot be changed.</p>
            {#if canEdit && rulesDirty}
              <div class="row">
                <Button label="Undo" onclick={() => rules && (rulesDraft = { ...rules.value })} />
                <Button tone="info" surface="solid" label="Review rule change" onclick={reviewRules} />
              </div>
            {/if}
          </div>
        </Card>
      {/if}
    {/if}
  </ManagerGate>
</div>

{#if editing && lists}
  <LimitEditor
    start={editing.start}
    {lists}
    onclose={() => (editing = null)}
    onsaved={() => {
      editing = null;
      showFlash("Limit saved", { tone: "success" });
      void load();
    }}
  />
{/if}

{#if review}
  <ReviewChanges title="Review rule change" preview={review} busy={saving} onsave={saveRules} onclose={() => (review = null)} />
{/if}

<style>
  .tabs {
    display: flex;
  }
  .nowrap {
    white-space: nowrap;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  .end {
    text-align: right;
  }
  p {
    margin: 0;
  }
</style>
