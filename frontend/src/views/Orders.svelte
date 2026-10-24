<script lang="ts">
  import {
    Button,
    Card,
    Chip,
    EmptyState,
    Notice,
    SegmentedControl,
    Spinner,
    Table,
    TableHeaderCell,
    TableSort,
  } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import NewOrderForm from "../components/NewOrderForm.svelte";
  import OrderDrawer from "../components/OrderDrawer.svelte";
  import { api, message, type Order } from "../lib/api";
  import { dublinDate, dublinTime } from "../lib/format";
  import { courseLabel, orderLabel } from "../lib/kitchen";
  import { canTakeOrders } from "../lib/kitchenfeed.svelte";
  import { onChange } from "../lib/live.svelte";
  import { param, setParam } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  let list = $state<Order[] | null>(null);
  let error = $state("");

  const takes = $derived(canTakeOrders(session.user?.role));
  const selected = $derived(param("id"));

  async function loadOrders() {
    try {
      list = (await api.orders(dublinDate(new Date()))).orders;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void loadOrders();
    return onChange((k) => {
      if (k === "order" || k === "ready") void loadOrders();
    });
  });

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
        <NewOrderForm onsent={() => void loadOrders()} />
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
