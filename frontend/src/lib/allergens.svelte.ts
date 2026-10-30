// Each dish's allergens, for the kitchen screens: loaded once, and again
// whenever the menu changes. Tickets for allergy orders show them.

import { api, type Schemas } from "./api";
import { onChange } from "./live.svelte";

const state = $state({ byDish: new Map<string, Schemas["AllergenDish"]>(), names: new Map<string, string>() });
let started = false;

async function load() {
  try {
    const c = await api.allergens();
    state.byDish = new Map(c.dishes.map((d) => [d.dish_id, d]));
    state.names = new Map(c.allergens.map((a) => [a.code, a.name]));
  } catch {
    // Tickets show no allergen line until it loads.
  }
}

function start() {
  if (started) return;
  started = true;
  void load();
  onChange((kind) => {
    if (kind === "menu") void load();
  });
}

/**
 * allergenLine says what a dish contains, for an allergy ticket:
 * "Contains: Milk, Eggs", "Contains none of the 14 allergens", or that
 * its allergens are not checked. Empty until the chart has loaded.
 */
export function allergenLine(dishID: string): { text: string; unchecked: boolean } | null {
  start();
  const d = state.byDish.get(dishID);
  if (!d) return null;
  if (!d.checked) return { text: "Allergens not checked", unchecked: true };
  if (d.allergens.length === 0) return { text: "Contains none of the 14 allergens", unchecked: false };
  return { text: "Contains: " + d.allergens.map((a) => state.names.get(a) ?? a).join(", "), unchecked: false };
}
