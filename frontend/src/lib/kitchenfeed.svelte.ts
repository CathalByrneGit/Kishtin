// The live kitchen: open orders and the scheduler's plan, refetched after
// every change and on the scheduler's re-plan interval, with a clock that
// ticks every second for countdowns. Call kitchenFeed() while a component
// is being set up.

import { can } from "./can.svelte";
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

/** canRunPass reports whether the logged-in staff member fires, serves and voids. */
export function canRunPass(): boolean {
  return can("orders.pass");
}

/** canWorkStation reports whether they mark items started and done. */
export function canWorkStation(): boolean {
  return can("orders.station");
}

/** canTakeOrders reports whether they enter orders. */
export function canTakeOrders(): boolean {
  return can("orders.take");
}
