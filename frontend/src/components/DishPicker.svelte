<script module lang="ts">
  import type { Course, Schemas } from "../lib/api";

  /** Line is one dish being added to an order. */
  export interface Line {
    key: number;
    dishId: string;
    course: Course;
    quantity: number;
    modifiers: string[];
    notes: string;
  }

  /** toItems turns lines into the API's new items. */
  export function toItems(lines: Line[]): Schemas["NewItem"][] {
    return lines.map((l) => ({
      dish_id: l.dishId,
      course: l.course,
      quantity: l.quantity,
      modifiers: l.modifiers.length > 0 ? l.modifiers : undefined,
      notes: l.notes.trim() || undefined,
    }));
  }
</script>

<script lang="ts">
  import { Button, Chip, IconButton, SelectDropdown, TextInput } from "@kenn-io/kit-ui";
  import { Minus, Plus, X } from "@lucide/svelte";
  import type { Dish, Setup } from "../lib/api";
  import { COURSES, courseLabel } from "../lib/kitchen";

  interface Props {
    menu: Setup;
    lines: Line[];
    /** kitchen-sized controls for phones and tablets */
    touch?: boolean;
  }
  let { menu, lines = $bindable(), touch = false }: Props = $props();
  const small = $derived(touch ? undefined : ("sm" as const));

  let nextKey = 1;
  const dishes = $derived(menu.dishes.filter((d) => d.active));

  function add(d: Dish) {
    if (!d.available) return;
    lines.push({ key: nextKey++, dishId: d.id, course: d.course as Course, quantity: 1, modifiers: [], notes: "" });
  }

  function toggle(l: Line, m: string) {
    l.modifiers = l.modifiers.includes(m) ? l.modifiers.filter((x) => x !== m) : [...l.modifiers, m];
  }

  const dishOf = (id: string) => menu.dishes.find((d) => d.id === id);
</script>

<div class="picker">
  {#if dishes.length === 0}
    <p class="muted">No dishes yet. A manager adds them in Setup, Menu.</p>
  {/if}
  {#each COURSES as c (c)}
    {@const list = dishes.filter((d) => d.course === c)}
    {#if list.length > 0}
      <div class="group">
        <span class="muted small">{courseLabel(c)}</span>
        <div class="dishes">
          {#each list as d (d.id)}
            <button type="button" class="dish kit-control-states" disabled={!d.available} onclick={() => add(d)}>
              <span>{d.name}</span>
              {#if !d.available}<Chip size="sm" tone="danger" uppercase={false}>86</Chip>{/if}
            </button>
          {/each}
        </div>
      </div>
    {/if}
  {/each}

  {#if lines.length > 0}
    <ul class="lines">
      {#each lines as l, i (l.key)}
        {@const d = dishOf(l.dishId)}
        <li>
          <div class="line-top" class:touch>
            <div class="qty">
              <IconButton ariaLabel="One fewer" size={small} disabled={l.quantity <= 1} onclick={() => (l.quantity -= 1)}><Minus size={14} /></IconButton>
              <strong class="num">{l.quantity}×</strong>
              <IconButton ariaLabel="One more" size={small} disabled={l.quantity >= 50} onclick={() => (l.quantity += 1)}><Plus size={14} /></IconButton>
            </div>
            <strong class="name">{d?.name ?? "?"}</strong>
            <SelectDropdown
              value={l.course}
              options={COURSES.map((c) => ({ value: c, label: courseLabel(c, false) }))}
              onchange={(v) => (l.course = v as Course)}
              title="Course"
            />
            <span class="remove"><IconButton ariaLabel={`Remove ${d?.name ?? "item"}`} size={small} onclick={() => lines.splice(i, 1)}><X size={14} /></IconButton></span>
          </div>
          {#if d && d.modifiers.length > 0}
            <div class="mods">
              {#each d.modifiers as m (m.name)}
                {#if touch}
                  <Button
                    label={m.name}
                    tone={l.modifiers.includes(m.name) ? "info" : "neutral"}
                    surface={l.modifiers.includes(m.name) ? "solid" : "outline"}
                    onclick={() => toggle(l, m.name)}
                  />
                {:else}
                  <Chip interactive size="sm" uppercase={false} tone={l.modifiers.includes(m.name) ? "info" : "neutral"} onclick={() => toggle(l, m.name)}>
                    {m.name}
                  </Chip>
                {/if}
              {/each}
            </div>
          {/if}
          <TextInput bind:value={l.notes} size={small} placeholder="Notes for the kitchen (optional)" ariaLabel="Notes" block />
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  .group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .dishes {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: var(--space-3);
  }
  .dish {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    min-height: max(44px, var(--kit-control-height, 0px));
    padding: var(--space-3) var(--space-4);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .dish:disabled {
    cursor: not-allowed;
    color: var(--text-secondary);
  }
  .lines {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .lines li {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4);
    border: var(--border-width) solid var(--border-muted);
    border-radius: var(--radius-md);
    background: var(--bg-inset);
  }
  .line-top {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .qty {
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }
  .name {
    flex: 1;
    min-width: 0;
  }
  /* On phones the dish name gets its own line above the controls. */
  .line-top.touch {
    flex-wrap: wrap;
  }
  .line-top.touch .name {
    order: -2;
    flex: 1 0 calc(100% - var(--kit-control-height, 56px) - var(--space-3));
    font-size: var(--font-size-lg);
  }
  .line-top.touch .remove {
    order: -1;
  }
  .mods {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
