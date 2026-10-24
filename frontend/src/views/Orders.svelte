<script lang="ts">
  import {
    Button,
    Card,
    Checkbox,
    Chip,
    EmptyState,
    Notice,
    SegmentedControl,
    Spinner,
    Table,
    TableHeaderCell,
    TableSort,
    TextInput,
    showFlash,
  } from "@kenn-io/kit-ui";
  import { Send } from "@lucide/svelte";
  import { onMount } from "svelte";
  import DishPicker, { toItems, type Line } from "../components/DishPicker.svelte";
  import OrderDrawer from "../components/OrderDrawer.svelte";
  import { api, message, type Course, type Order, type Setup } from "../lib/api";
  import { dublinDate, dublinTime } from "../lib/format";
  import { COURSES, courseLabel, orderLabel } from "../lib/kitchen";
  import { canRunPass, canTakeOrders } from "../lib/kitchenfeed.svelte";
  import { onChange } from "../lib/live.svelte";
  import { param, setParam } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  let menu = $state<Setup | null>(null);
  let list = $state<Order[] | null>(null);
  let error = $state("");

  // New order form.
  let kind = $state<"table" | "takeaway">("table");
  let table = $state("");
  let name = $state("");
  let coversText = $state("");
  let allergy = $state(false);
  let notes = $state("");
  let lines = $state<Line[]>([]);
  let fire = $state<Course[]>([]);
  let sending = $state(false);

  const takes = $derived(canTakeOrders(session.user?.role));
  const expo = $derived(canRunPass(session.user?.role));
  const selected = $derived(param("id"));

  async function loadMenu() {
    try {
      menu = await api.menu();
    } catch (err) {
      error = message(err);
    }
  }

  async function loadOrders() {
    try {
      list = (await api.orders(dublinDate(new Date()))).orders;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void loadMenu();
    void loadOrders();
    return onChange((k) => {
      if (k === "menu") void loadMenu();
      if (k === "order" || k === "ready") void loadOrders();
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
        name: kind === "takeaway" && name.trim() ? name.trim() : undefined,
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
      await loadOrders();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      sending = false;
    }
  }

  const show = $derived(param("show") ?? "open");
  const sorter = new TableSort<Order>(
    { number: (o) => o.number, order: (o) => orderLabel(o), at: (o) => o.created_at, items: (o) => o.items.length, status: (o) => o.status },
    { key: "number", direction: "desc" },
  );
  const rows = $derived(sorter.sort((list ?? []).filter((o) => show === "all" || o.status === "open")));

  function summary(o: Order): string {
    return o.courses.map((c) => `${courseLabel(c.course)} ${c.state === "fired" ? "fired" : c.state}`).join(", ");
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Orders</h1>
      <p>Enter orders for tables and takeaways. Courses wait at the pass until the expo fires them.</p>
    </div>
  </header>

  {#if error}<Notice tone="error" message={error} />{/if}

  <div class="grid">
    {#if takes}
      <Card padding="md" title="New order">
        {#if !menu}
          <Spinner />
        {:else}
          <div class="stack">
            <SegmentedControl
              ariaLabel="Kind"
              value={kind}
              onchange={(v) => (kind = v as "table" | "takeaway")}
              options={[
                { value: "table", label: "Table" },
                { value: "takeaway", label: "Takeaway" },
              ]}
            />
            <div class="two">
              {#if kind === "table"}
                <label class="field"><span>Table</span><TextInput bind:value={table} placeholder="4" block /></label>
              {:else}
                <label class="field"><span>Customer name (optional)</span><TextInput bind:value={name} block /></label>
              {/if}
              <label class="field"><span>Covers (optional)</span><TextInput bind:value={coversText} placeholder="2" block /></label>
            </div>
            <Checkbox bind:checked={allergy} label="This order has an allergy" />
            <label class="field"><span>Notes for the kitchen (optional)</span><TextInput bind:value={notes} block /></label>

            <DishPicker {menu} bind:lines />

            {#if expo && courses.length > 0}
              <div class="field">
                <span>Fire straight away</span>
                <div class="row">
                  {#each courses as c (c)}
                    <Checkbox
                      checked={fire.includes(c)}
                      label={courseLabel(c)}
                      onchange={(on) => (fire = on ? [...fire, c] : fire.filter((x) => x !== c))}
                    />
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
      </Card>
    {:else}
      <Notice tone="info" title="Front of house, head chef or manager" message="Ask someone who takes orders to log in on this device to enter one." />
    {/if}

    <div class="stack">
      <div class="row between">
        <h2 class="section-title">Today's orders</h2>
        <SegmentedControl
          ariaLabel="Show"
          value={show}
          onchange={(v) => setParam("show", v === "open" ? null : v)}
          options={[
            { value: "open", label: "Open" },
            { value: "all", label: "All today" },
          ]}
        />
      </div>
      {#if list === null}
        <Spinner />
      {:else if rows.length === 0}
        <EmptyState title="No orders" description={show === "open" ? "No open orders." : "No orders yet today."} />
      {:else}
        <Table ariaLabel="Today's orders">
          {#snippet header()}
            <TableHeaderCell label="#" numeric sort={sorter} column="number" />
            <TableHeaderCell label="Order" sort={sorter} column="order" />
            <TableHeaderCell label="Entered" sort={sorter} column="at" />
            <TableHeaderCell label="Items" numeric sort={sorter} column="items" />
            <TableHeaderCell label="Status" sort={sorter} column="status" />
          {/snippet}
          {#each rows as o (o.id)}
            <tr class="clickable" onclick={() => setParam("id", o.id)}>
              <td class="num">{o.number}</td>
              <td>
                <strong>{orderLabel(o)}</strong>
                {#if o.allergy}<Chip size="sm" tone="danger" uppercase={false}>Allergy</Chip>{/if}
                <div class="muted small">{summary(o)}</div>
              </td>
              <td class="num">{dublinTime(o.created_at)}</td>
              <td class="num">{o.items.length}</td>
              <td><Chip uppercase={false} tone={o.status === "open" ? "info" : "muted"}>{o.status === "open" ? "Open" : "Closed"}</Chip></td>
            </tr>
          {/each}
        </Table>
      {/if}
    </div>
  </div>
</div>

{#if selected}
  <OrderDrawer orderId={selected} onclose={() => setParam("id", null)} />
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: minmax(360px, 520px) minmax(0, 1fr);
    gap: var(--space-6);
    align-items: start;
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-4);
  }
  .between {
    justify-content: space-between;
  }
  .section-title {
    margin: 0;
  }
  .clickable {
    cursor: pointer;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  @media (max-width: 900px) {
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
