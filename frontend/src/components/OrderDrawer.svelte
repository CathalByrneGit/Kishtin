<script lang="ts">
  import { Button, Chip, DetailDrawer, Notice, Spinner, Table, TableHeaderCell, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import DishPicker, { toItems, type Line } from "./DishPicker.svelte";
  import { api, message, type Course, type Order, type OrderItem, type Schemas, type Setup } from "../lib/api";
  import { dublinTime } from "../lib/format";
  import { courseLabel, itemName, itemState, nextLabel, orderLabel } from "../lib/kitchen";
  import { canRunPass, canTakeOrders, canWorkStation } from "../lib/kitchenfeed.svelte";
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
  let naming = $state<string | null>(null);
  // changing is the item being replaced, with the replacement as one line.
  let changing = $state<{ item: OrderItem; lines: Line[]; reason: string } | null>(null);

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

  async function loadMenu() {
    if (menu) return;
    try {
      menu = await api.menu();
    } catch (err) {
      error = message(err);
    }
  }

  async function startAdding() {
    adding = true;
    await loadMenu();
  }

  async function startChange(it: OrderItem) {
    await loadMenu();
    changing = {
      item: it,
      reason: "",
      lines: [{ key: 0, dishId: it.dish_id, course: it.course as Course, quantity: 1, modifiers: [...(it.modifiers ?? [])], notes: it.notes ?? "" }],
    };
  }

  async function saveChange() {
    const item = changing ? toItems(changing.lines)[0] : undefined;
    if (!changing || changing.lines.length !== 1 || !item) return;
    busy = true;
    try {
      order = await api.replaceItem(changing.item.id, { item, reason: changing.reason.trim() || undefined });
      showFlash(`${changing.item.dish_name} replaced`, { tone: "success" });
      changing = null;
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  async function step(it: OrderItem) {
    busy = true;
    try {
      await api.moveItems({ item_ids: [it.id], from: it.state });
      showFlash(`${nextLabel(it.state)}: ${itemName(it)}`, { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
      await load();
    } finally {
      busy = false;
    }
  }

  async function hold(c: Course) {
    busy = true;
    try {
      order = await api.hold(orderId, c);
      showFlash(`${courseLabel(c)} held again`, { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  // canChange: front of house can change items in a held course; once
  // fired, only the pass.
  function canChange(it: OrderItem): boolean {
    if (!order || order.status !== "open" || it.state === "served" || it.state === "voided") return false;
    return it.state === "held" ? canTakeOrders() : canRunPass();
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

  async function saveName() {
    if (naming === null) return;
    busy = true;
    try {
      order = await api.renameOrder(orderId, naming.trim());
      showFlash(naming.trim() ? `Named ${naming.trim()}` : "Name cleared", { tone: "success" });
      naming = null;
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
    held: "Course held again",
    target: "Target changed",
    urgent: "Urgent flag",
    served: "Course served",
    closed: "Order closed",
    renamed: "Name changed",
    pos: "Changed on the till",
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
      {#if order.status === "open" && canTakeOrders()}
        {#if naming === null}
          <div class="row">
            <span>{order.name ? `Name: ${order.name}` : "No name: shown by its table or ticket."}</span>
            <Button size="sm" label={order.name ? "Change name" : "Add a name"} onclick={() => (naming = order?.name ?? "")} />
          </div>
        {:else}
          <form
            class="row"
            onsubmit={(e) => {
              e.preventDefault();
              void saveName();
            }}
          >
            <TextInput bind:value={naming} size="sm" placeholder="Such as the guest's name (empty clears it)" ariaLabel="Order name" />
            <Button size="sm" label="Cancel" onclick={() => (naming = null)} />
            <Button size="sm" tone="success" surface="solid" type="submit" label="Save name" disabled={busy} />
          </form>
        {/if}
      {/if}
      {#if order.notes}<Notice tone="warning" title="Notes" message={order.notes} />{/if}

      {#each order.courses as c (c.course)}
        {@const courseItems = order.items.filter((it) => it.course === c.course)}
        <section>
          <h3>
            {courseLabel(c.course)}
            <span class="muted small">
              · {c.state === "held" ? "Held" : c.state === "fired" ? `Fired ${dublinTime(c.fired_at ?? "")}, due ${dublinTime(c.target_at ?? "")}` : `Served ${dublinTime(c.served_at ?? "")}`}
              {c.urgent ? " · urgent" : ""}
            </span>
          </h3>
          {#if c.state === "fired" && order.status === "open" && canRunPass() && courseItems.every((it) => it.state === "fired" || it.state === "held" || it.state === "voided")}
            <div><Button size="sm" label="Hold again" disabled={busy} onclick={() => hold(c.course as Course)} /></div>
          {/if}
          <ul class="items">
            {#each courseItems as it (it.id)}
              {@const st = itemState(it.state)}
              <li>
                <span>{itemName(it)}{#if it.notes}<span class="muted">: {it.notes}</span>{/if}</span>
                <span class="row">
                  {#if it.void_reason}<span class="muted small">{it.void_reason}</span>{/if}
                  <Chip size="sm" uppercase={false} tone={st.tone}>{st.label}</Chip>
                  {#if canWorkStation() && ["fired", "cooking", "resting", "plating"].includes(it.state)}
                    <Button size="sm" tone="info" surface="solid" label={nextLabel(it.state)} disabled={busy} onclick={() => step(it)} />
                  {/if}
                  {#if canChange(it)}<Button size="sm" label="Change" disabled={busy || changing !== null} onclick={() => startChange(it)} />{/if}
                </span>
              </li>
              {#if changing?.item.id === it.id && menu}
                <li class="change">
                  <p class="muted small">
                    Change {itemName(it)}: the old item is voided and the new one added in its place{it.state === "held" ? "" : ", straight to its station"}. Edit the line below, or remove it and tap the right dish.
                  </p>
                  <DishPicker {menu} bind:lines={changing.lines} />
                  <label class="field">
                    <span>Reason (optional, for the log)</span>
                    <TextInput bind:value={changing.reason} placeholder="For example: asked for well done" block size="sm" />
                  </label>
                  {#if changing.lines.length !== 1}<Notice tone="info" message="Keep exactly one line: the item to put in its place." />{/if}
                  <div class="row">
                    <Button label="Cancel" onclick={() => (changing = null)} />
                    <Button tone="success" surface="solid" label={busy ? "Saving…" : "Replace item"} disabled={busy || changing.lines.length !== 1} onclick={saveChange} />
                  </div>
                </li>
              {/if}
            {/each}
          </ul>
        </section>
      {/each}

      {#if order.status === "open" && canTakeOrders()}
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
    {#if order && order.status === "open" && canRunPass()}
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
  .items li.change {
    flex-direction: column;
    align-items: stretch;
    padding: var(--space-4);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
  }
  .small {
    font-size: var(--font-size-sm);
  }
</style>
