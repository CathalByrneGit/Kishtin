<script lang="ts">
  import { Button, Card, Chip, Notice, Spinner, Table, TableHeaderCell } from "@kenn-io/kit-ui";
  import { Printer } from "@lucide/svelte";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { courseLabel } from "../../lib/kitchen";
  import { onChange } from "../../lib/live.svelte";
  import { dublinTime, dublinDate, shortDate } from "../../lib/format";

  let { restaurant = "" }: { restaurant?: string } = $props();

  let chart = $state<Schemas["AllergenChart"] | null>(null);
  let error = $state("");
  let printedAt = $state(new Date());

  // Short column headings, so all 14 fit across a page.
  const SHORT: Record<string, string> = {
    gluten: "Gluten", crustaceans: "Crustaceans", eggs: "Eggs", fish: "Fish", peanuts: "Peanuts", soybeans: "Soya",
    milk: "Milk", nuts: "Nuts", celery: "Celery", mustard: "Mustard", sesame: "Sesame", sulphites: "Sulphites",
    lupin: "Lupin", molluscs: "Molluscs",
  };

  async function load() {
    try {
      chart = await api.allergens();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((k) => {
      if (k === "menu") void load();
    });
  });

  function print() {
    printedAt = new Date();
    setTimeout(() => window.print(), 50);
  }
</script>

<div class="page allergen-chart">
  <header class="head">
    <div>
      <h1>Allergens{restaurant ? ` at ${restaurant}` : ""}</h1>
      <p>
        Each dish on the menu and which of the 14 regulated allergens it contains. Show this to customers who ask, or print it. Allergens are set on
        each dish in Setup › Kitchen › Menu.
      </p>
    </div>
    <div class="no-print">
      <Button label="Print" onclick={print}><Printer size={16} /></Button>
    </div>
  </header>

  {#if error}<Notice tone="error" message={error} />{/if}
  {#if !chart}
    {#if !error}<Spinner />{/if}
  {:else}
    {#if chart.unchecked > 0}
      <Notice
        tone="warning"
        title={`${chart.unchecked} ${chart.unchecked === 1 ? "dish has" : "dishes have"} not had allergens checked`}
        message="They are marked “Not checked” and may contain any allergen. Do not tell a customer they are safe until a manager checks them in Setup › Kitchen › Menu."
      />
    {/if}
    <Card padding="none">
      <Table ariaLabel="Allergen chart" unsorted>
        {#snippet header()}
          <TableHeaderCell label="Dish" />
          {#each chart!.allergens as a (a.code)}
            <TableHeaderCell><span class="col" title={a.name + (a.detail ? `: ${a.detail}` : "")}>{SHORT[a.code] ?? a.name}</span></TableHeaderCell>
          {/each}
        {/snippet}
        {#each chart.dishes as d (d.dish_id)}
          <tr class:unchecked={!d.checked}>
            <td>
              <strong>{d.dish_name}</strong>
              <div class="muted small">
                {courseLabel(d.course, false)}{#if !d.available} · sold out{/if}
              </div>
              {#if !d.checked}<Chip size="sm" tone="warning" uppercase={false}>Not checked</Chip>{/if}
            </td>
            {#each chart.allergens as a (a.code)}
              <td class="cell">
                {#if d.allergens.includes(a.code)}
                  <span class="yes">Yes</span>
                {:else if !d.checked}
                  <span class="muted">?</span>
                {:else}
                  <span class="muted" aria-label="No">–</span>
                {/if}
              </td>
            {/each}
          </tr>
        {:else}
          <tr><td colspan="15" class="muted">No dishes on the menu yet.</td></tr>
        {/each}
      </Table>
    </Card>
    <div class="legend">
      <p class="small">
        <strong>Yes</strong>: contains it. <strong>–</strong>: does not. <strong>?</strong>: not checked yet.
        The 14 allergens are those of Regulation (EU) No 1169/2011, Annex II. Sulphites count above 10 mg per kg or litre.
        Cross-contact in the kitchen is not shown: ask the kitchen.
      </p>
      <p class="small print-only">Printed {shortDate(dublinDate(printedAt))} {printedAt.getFullYear()}, {dublinTime(printedAt.toISOString())}. Check with staff that it is current.</p>
    </div>
  {/if}
</div>

<style>
  .head {
    display: flex;
    justify-content: space-between;
    gap: var(--space-5);
    align-items: flex-start;
    margin-bottom: var(--space-5);
  }
  .col {
    writing-mode: vertical-rl;
    transform: rotate(180deg);
    white-space: nowrap;
  }
  .cell {
    text-align: center;
  }
  .yes {
    font-weight: 700;
  }
  .unchecked td {
    background: var(--bg-inset);
  }
  .legend {
    margin-top: var(--space-4);
  }
  .print-only {
    display: none;
  }
  @media print {
    :global(.kit-app-sidebar),
    :global(.top-bar),
    :global(aside),
    :global(nav),
    .no-print {
      display: none !important;
    }
    .print-only {
      display: block;
    }
    .allergen-chart {
      padding: 0;
    }
  }
</style>
