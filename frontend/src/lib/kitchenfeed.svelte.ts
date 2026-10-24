// The live kitchen: open orders and the scheduler's plan, refetched after
// every change and on the scheduler's re-plan interval, with a clock that
// ticks every second for countdowns. Call kitchenFeed() while a component
// is being set up.

import { api, message, type KitchenView } from "./api";
import { onChange } from "./live.svelte";

export interface KitchenFeed {
  readonly view: KitchenView | null;
  readonly error: string;
  readonly now: Date;
  reload: () => Promise<void>;
}

export function kitchenFeed(): KitchenFeed {
  const s = $state({ view: null as KitchenView | null, error: "", now: new Date() });
  let timer: ReturnType<typeof setTimeout> | undefined;
  let stopped = false;

  async function load() {
    clearTimeout(timer);
    try {
      s.view = await api.kitchen();
      s.error = "";
    } catch (err) {
      s.error = message(err);
    }
    if (!stopped) timer = setTimeout(load, (s.view?.replan_seconds ?? 15) * 1000);
  }

  $effect(() => {
    stopped = false;
    void load();
    const off = onChange((kind) => {
      if (kind === "order" || kind === "menu" || kind === "settings" || kind === "ready") void load();
    });
    const tick = setInterval(() => (s.now = new Date()), 1000);
    return () => {
      stopped = true;
      off();
      clearInterval(tick);
      clearTimeout(timer);
    };
  });

  return {
    get view() {
      return s.view;
    },
    get error() {
      return s.error;
    },
    get now() {
      return s.now;
    },
    reload: load,
  };
}

/** canRunPass reports whether a role fires, serves and voids. */
export function canRunPass(role: string | undefined): boolean {
  return role === "head_chef" || role === "manager";
}

/** canWorkStation reports whether a role marks items started and done. */
export function canWorkStation(role: string | undefined): boolean {
  return role === "line_cook" || role === "head_chef" || role === "manager";
}

/** canTakeOrders reports whether a role enters orders. */
export function canTakeOrders(role: string | undefined): boolean {
  return role === "front_of_house" || role === "head_chef" || role === "manager";
}
