<script lang="ts">
  import { Button, Checkbox, Notice, SegmentedControl, Spinner, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { Send } from "@lucide/svelte";
  import { onMount } from "svelte";
  import DishPicker, { toItems, type Line } from "./DishPicker.svelte";
  import { api, message, type Course, type Order, type Setup } from "../lib/api";
  import { COURSES, courseLabel, orderLabel } from "../lib/kitchen";
  import { canRunPass } from "../lib/kitchenfeed.svelte";
  import { onChange } from "../lib/live.svelte";
  import { session } from "../lib/session.svelte";

  interface Props {
    onsent?: (o: Order) => void;
    /** kitchen-sized controls for phones and tablets */
    touch?: boolean;
  }
  let { onsent, touch = false }: Props = $props();

  let menu = $state<Setup | null>(null);
  let error = $state("");
  let kind = $state<"table" | "takeaway">("table");
  let table = $state("");
  let name = $state("");
  let coversText = $state("");
  let allergy = $state(false);
  let notes = $state("");
  let lines = $state<Line[]>([]);
  let fire = $state<Course[]>([]);
  let sending = $state(false);

  const expo = $derived(canRunPass());

  async function loadMenu() {
    try {
      menu = await api.menu();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void loadMenu();
    return onChange((k) => {
      if (k === "menu") void loadMenu();
    });
  });

  const courses = $derived(COURSES.filter((c) => lines.some((l) => l.course === c)));
  const covers = $derived(/^\d*$/.test(coversText.trim()) ? Number(coversText || 0) : NaN);
  const problem = $derived(
    lines.length === 0
      ? "Add at least one dish."
      : kind === "table" && !table.trim()
        ? "Enter the table."
        : Number.isNaN(covers)
          ? "Covers must be a whole number."
          : "",
  );

  async function send() {
    sending = true;
    try {
      const o = await api.createOrder({
        kind,
        table: kind === "table" ? table.trim() : undefined,
        name: name.trim() || undefined,
        covers: covers || undefined,
        allergy: allergy || undefined,
        notes: notes.trim() || undefined,
        items: toItems(lines),
        fire: fire.filter((c) => courses.includes(c)),
      });
      showFlash(`${orderLabel(o)} sent to the kitchen as #${o.number}`, { tone: "success" });
      table = "";
      name = "";
      coversText = "";
      allergy = false;
      notes = "";
      lines = [];
      fire = [];
      onsent?.(o);
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      sending = false;
    }
  }
</script>

{#if error}<Notice tone="error" message={error} />{/if}
{#if !menu}
  {#if !error}<Spinner />{/if}
{:else}
  <div class="stack">
    <SegmentedControl
      ariaLabel="Kind"
      value={kind}
      block
      onchange={(v) => (kind = v as "table" | "takeaway")}
      options={[
        { value: "table", label: "Table" },
        { value: "takeaway", label: "Takeaway" },
      ]}
    />
    <div class="two">
      {#if kind === "table"}
        <label class="field"><span>Table</span><TextInput bind:value={table} placeholder="4" block /></label>
      {/if}
      <label class="field">
        <span>Name (optional)</span>
        <TextInput bind:value={name} placeholder={kind === "table" ? "Shown instead of the table, such as the guest" : "The customer's name"} block />
      </label>
      <label class="field"><span>Covers (optional)</span><TextInput bind:value={coversText} placeholder="2" block /></label>
    </div>
    <Checkbox bind:checked={allergy} label="This order has an allergy" />
    <label class="field"><span>Notes for the kitchen (optional)</span><TextInput bind:value={notes} block /></label>

    <DishPicker {menu} bind:lines {touch} />

    {#if expo && courses.length > 0}
      <div class="field">
        <span>Fire straight away</span>
        <div class="row">
          {#each courses as c (c)}
            <Checkbox checked={fire.includes(c)} label={courseLabel(c)} onchange={(on) => (fire = on ? [...fire, c] : fire.filter((x) => x !== c))} />
          {/each}
        </div>
      </div>
    {/if}

    {#if problem && lines.length > 0}<Notice tone="info" message={problem} />{/if}
    <Button tone="success" surface="solid" label={sending ? "Sending…" : "Send order"} disabled={sending || Boolean(problem)} onclick={send}>
      <Send size={16} />
    </Button>
  </div>
{/if}

<style>
  .two {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 160px), 1fr));
    gap: var(--space-4);
  }
</style>
