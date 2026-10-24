<script lang="ts">
  import { Button, Chip, DetailDrawer, Notice, Spinner, Table, TableHeaderCell, showFlash } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import DishPicker, { toItems, type Line } from "./DishPicker.svelte";
  import { api, message, type Order, type Schemas, type Setup } from "../lib/api";
  import { dublinTime } from "../lib/format";
  import { courseLabel, itemName, itemState, orderLabel } from "../lib/kitchen";
  import { canRunPass, canTakeOrders } from "../lib/kitchenfeed.svelte";
  import { onChange } from "../lib/live.svelte";
  import { session } from "../lib/session.svelte";

  interface Props {
    orderId: string;
    onclose: () => void;
  }
  let { orderId, onclose }: Props = $props();

  let order = $state<Order | null>(null);
  let events = $state<Schemas["Event"][]>([]);
  let menu = $state<Setup | null>(null);
  let error = $state("");
  let adding = $state(false);
  let lines = $state<Line[]>([]);
  let busy = $state(false);

  async function load() {
    try {
      const [o, e] = await Promise.all([api.order(orderId), api.orderEvents(orderId)]);
      order = o;
      events = e.events;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((k) => {
      if (k === "order") void load();
    });
  });

  async function startAdding() {
    adding = true;
    if (!menu) {
      try {
        menu = await api.menu();
      } catch (err) {
        error = message(err);
      }
    }
  }

  async function addItems() {
    busy = true;
    try {
      order = await api.addItems(orderId, toItems(lines));
      lines = [];
      adding = false;
      showFlash("Items added", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  async function close() {
    busy = true;
    try {
      await api.closeOrder(orderId);
      showFlash("Order closed", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  const KINDS: Record<string, string> = {
    created: "Order entered",
    item_added: "Item added",
    item_state: "Item moved",
    fired: "Course fired",
    target: "Target changed",
    urgent: "Urgent flag",
    served: "Course served",
    closed: "Order closed",
  };

  function change(e: Schemas["Event"]): string {
    if (e.kind === "item_state") return `${itemState(e.from ?? "").label} → ${itemState(e.to ?? "").label}`;
    if (e.kind === "urgent") return e.to === "true" ? "Urgent" : "Not urgent";
    return "";
  }

  const allDone = $derived(order ? order.items.every((it) => it.state === "served" || it.state === "voided") : false);
</script>

<DetailDrawer title={order ? `${orderLabel(order)} · #${order.number}` : "Order"} {onclose} width="min(720px, 100vw)">
  <div class="body">
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if !order}
      {#if !error}<Spinner />{/if}
    {:else}
      <div class="row">
        <Chip uppercase={false} tone={order.status === "open" ? "info" : "muted"}>{order.status === "open" ? "Open" : "Closed"}</Chip>
        {#if order.allergy}<Chip uppercase={false} tone="danger">Allergy</Chip>{/if}
        <span class="muted">Entered {dublinTime(order.created_at)}{order.covers ? ` · ${order.covers} covers` : ""}</span>
      </div>
      {#if order.notes}<Notice tone="warning" title="Notes" message={order.notes} />{/if}

      {#each order.courses as c (c.course)}
        <section>
          <h3>
            {courseLabel(c.course)}
            <span class="muted small">
              · {c.state === "held" ? "Held" : c.state === "fired" ? `Fired ${dublinTime(c.fired_at ?? "")}, due ${dublinTime(c.target_at ?? "")}` : `Served ${dublinTime(c.served_at ?? "")}`}
              {c.urgent ? " · urgent" : ""}
            </span>
          </h3>
          <ul class="items">
            {#each order.items.filter((it) => it.course === c.course) as it (it.id)}
              {@const st = itemState(it.state)}
              <li>
                <span>{itemName(it)}{#if it.notes}<span class="muted">: {it.notes}</span>{/if}</span>
                <span class="row">
                  {#if it.void_reason}<span class="muted small">{it.void_reason}</span>{/if}
                  <Chip size="sm" uppercase={false} tone={st.tone}>{st.label}</Chip>
                </span>
              </li>
            {/each}
          </ul>
        </section>
      {/each}

      {#if order.status === "open" && canTakeOrders(session.user?.role)}
        {#if adding && menu}
          <section>
            <h3>Add items</h3>
            <DishPicker {menu} bind:lines />
            <div class="row">
              <Button label="Cancel" onclick={() => ((adding = false), (lines = []))} />
              <Button tone="success" surface="solid" label={busy ? "Adding…" : "Add to order"} disabled={busy || lines.length === 0} onclick={addItems} />
            </div>
          </section>
        {:else}
          <div><Button label="Add items" onclick={startAdding} /></div>
        {/if}
      {/if}

      <section>
        <h3>History</h3>
        <Table unsorted ariaLabel="Order history">
          {#snippet header()}
            <TableHeaderCell label="Time" />
            <TableHeaderCell label="What" />
            <TableHeaderCell label="Detail" />
            <TableHeaderCell label="By" />
          {/snippet}
          {#each events as e (e.id)}
            <tr>
              <td class="num">{dublinTime(e.at)}</td>
              <td>{KINDS[e.kind] ?? e.kind}{e.course ? ` · ${courseLabel(e.course, false)}` : ""}</td>
              <td>{e.detail ?? ""}{change(e) ? ` ${change(e)}` : ""}</td>
              <td>{e.by_name}</td>
            </tr>
          {/each}
        </Table>
      </section>
    {/if}
  </div>
  {#snippet footer()}
    {#if order && order.status === "open" && canRunPass(session.user?.role)}
      <div class="row">
        <Button label="Close order" disabled={busy || !allDone} onclick={close} />
        {#if !allDone}<span class="muted small">Close once every item is served or voided.</span>{/if}
      </div>
    {/if}
  {/snippet}
</DetailDrawer>

<style>
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    padding: var(--space-5) var(--space-6);
  }
  section {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  h3 {
    font-size: var(--font-size-md);
  }
  .items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .items li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }
  .small {
    font-size: var(--font-size-sm);
  }
</style>
