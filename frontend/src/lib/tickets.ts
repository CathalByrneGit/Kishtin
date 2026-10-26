// Tickets, all-day counts and recently done items for the station screen
// and the pass, worked out from the kitchen view. Pure functions, so they
// are tested without a browser.

import type { KitchenView, Order, OrderItem, Schemas } from "./api";
import { itemName } from "./kitchen";

type Action = Schemas["Action"];
type Progress = Schemas["Progress"];
type CourseStatus = Schemas["CourseStatus"];

/** TicketLine is one item on a ticket, with what the scheduler says about it. */
export interface TicketLine {
  item: OrderItem;
  /** the scheduler's start for an item not started yet */
  action?: Action;
  /** when the current phase ends, for an item started */
  progress?: Progress;
}

/** Ticket is one order's course at one station. */
export interface Ticket {
  key: string;
  order: Order;
  course: string;
  firedAt?: string;
  targetAt?: string;
  status?: CourseStatus;
  lines: TicketLine[];
  /** when this ticket next needs a hand: its earliest start or phase end */
  nextAt: number;
}

const ACTIVE = new Set(["fired", "cooking", "resting", "plating"]);
const COOKING = new Set(["cooking", "resting", "plating"]);

function indexByItem<T extends { item_ids?: string[]; item_id?: string }>(list: T[]): Map<string, T> {
  const m = new Map<string, T>();
  for (const x of list) {
    for (const id of x.item_ids ?? (x.item_id ? [x.item_id] : [])) m.set(id, x);
  }
  return m;
}

/**
 * stationTickets lists the tickets with work for a station: fired courses
 * with at least one of the station's items not yet ready. Tickets that
 * need a hand soonest come first.
 */
export function stationTickets(view: KitchenView, stationId: string): Ticket[] {
  const sp = view.plan.stations.find((s) => s.station_id === stationId);
  const actions = indexByItem(sp?.actions ?? []);
  const progress = indexByItem(sp?.in_progress ?? []);
  const statuses = new Map((view.plan.courses ?? []).map((c) => [`${c.order_id}|${c.course}`, c]));
  const out: Ticket[] = [];
  for (const o of view.orders) {
    for (const c of o.courses) {
      if (c.state !== "fired") continue;
      const items = o.items.filter((it) => it.course === c.course && it.station_id === stationId && it.state !== "voided");
      if (!items.some((it) => ACTIVE.has(it.state))) continue;
      const lines = items.map((it) => ({ item: it, action: actions.get(it.id), progress: progress.get(it.id) }));
      const times = lines.flatMap((l) => {
        if (l.action) return [l.action.now ? 0 : Date.parse(l.action.start_at)];
        if (l.progress) return [Date.parse(l.progress.phase_ends_at)];
        return [];
      });
      out.push({
        key: `${o.id}|${c.course}`,
        order: o,
        course: c.course,
        firedAt: c.fired_at,
        targetAt: c.target_at,
        status: statuses.get(`${o.id}|${c.course}`),
        lines,
        nextAt: times.length ? Math.min(...times) : Number.MAX_SAFE_INTEGER,
      });
    }
  }
  return out.sort((a, b) => a.nextAt - b.nextAt || a.order.number - b.order.number);
}

/** Variant is one way a dish is ordered: "regular", "well done", "well done, no onion". */
export interface Variant {
  label: string;
  /** fired and not yet ready */
  active: number;
  held: number;
  ready: number;
}

/** AllDay is how many of one dish are in the kitchen, with its variants. */
export interface AllDay {
  /** the dish ID */
  key: string;
  name: string;
  /** held: in courses not fired yet */
  held: number;
  /** fired, waiting to be started */
  toStart: number;
  /** cooking, resting or plating */
  cooking: number;
  /** at the pass */
  ready: number;
  /** items the scheduler says to start now */
  startNow: string[];
  /** the variant those items share, or "" when they differ */
  startNowVariant: string;
  /** the ways it is ordered, regular first, then the most common */
  variants: Variant[];
}

/** variantLabel names how an item differs from the plain dish. */
export function variantLabel(it: Pick<OrderItem, "modifiers" | "notes">): string {
  const parts = [...(it.modifiers ?? []).map((m) => m.toLowerCase())];
  if (it.notes?.trim()) parts.push(it.notes.trim());
  return parts.length ? parts.join(", ") : "regular";
}

/** allDay counts every dish not yet served, for one station or the whole kitchen. */
export function allDay(view: KitchenView, stationId?: string): AllDay[] {
  const now = new Set<string>();
  for (const sp of view.plan.stations) {
    if (stationId && sp.station_id !== stationId) continue;
    for (const a of sp.actions) if (a.now) for (const id of a.item_ids) now.add(id);
  }
  const rows = new Map<string, AllDay>();
  for (const o of view.orders) {
    for (const it of o.items) {
      if (stationId && it.station_id !== stationId) continue;
      if (it.state === "served" || it.state === "voided") continue;
      let r = rows.get(it.dish_id);
      if (!r) {
        r = { key: it.dish_id, name: it.dish_name, held: 0, toStart: 0, cooking: 0, ready: 0, startNow: [], startNowVariant: "", variants: [] };
        rows.set(it.dish_id, r);
      }
      const label = variantLabel(it);
      let v = r.variants.find((x) => x.label === label);
      if (!v) {
        v = { label, active: 0, held: 0, ready: 0 };
        r.variants.push(v);
      }
      if (it.state === "held") {
        r.held++;
        v.held++;
      } else if (it.state === "ready") {
        r.ready++;
        v.ready++;
      } else {
        if (it.state === "fired") r.toStart++;
        else if (COOKING.has(it.state)) r.cooking++;
        v.active++;
      }
      if (now.has(it.id)) {
        r.startNowVariant = r.startNow.length === 0 || r.startNowVariant === label ? label : "";
        r.startNow.push(it.id);
      }
    }
  }
  for (const r of rows.values()) {
    const n = (v: Variant) => v.active + v.held + v.ready;
    r.variants.sort((a, b) => (a.label === "regular" ? -1 : b.label === "regular" ? 1 : n(b) - n(a) || a.label.localeCompare(b.label)));
  }
  return [...rows.values()].sort((a, b) => active(b) - active(a) || a.name.localeCompare(b.name));
}

/** active is how many of a dish are fired and not yet ready. */
export function active(r: AllDay): number {
  return r.toStart + r.cooking;
}

/** lineKey is the all-day key of an item (its dish), to filter tickets by dish. */
export function lineKey(it: Pick<OrderItem, "dish_id">): string {
  return it.dish_id;
}

/** Done is an item a station finished recently, for Undo. */
export interface Done {
  item: OrderItem;
  order: Order;
}

/** recentlyDone lists a station's items made ready in the last few minutes, newest first. */
export function recentlyDone(view: KitchenView, stationId: string, now: Date, minutes = 15, limit = 6): Done[] {
  const since = now.getTime() - minutes * 60_000;
  const out: Done[] = [];
  for (const o of view.orders) {
    for (const it of o.items) {
      if (it.station_id === stationId && it.state === "ready" && Date.parse(it.state_at) >= since) out.push({ item: it, order: o });
    }
  }
  return out.sort((a, b) => Date.parse(b.item.state_at) - Date.parse(a.item.state_at)).slice(0, limit);
}

/** stepBack is the state one step before ready, to undo a Ready tap. */
export function stepBack(it: Pick<OrderItem, "state" | "timing">): string {
  const t = it.timing;
  if (it.state === "ready" && t.plate > 0) return "plating";
  if ((it.state === "ready" || it.state === "plating") && t.rest > 0) return "resting";
  if (t.cook > 0) return "cooking";
  return "fired";
}
