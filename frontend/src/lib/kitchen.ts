// Words, tones and helpers for the menu, orders and the pass.

import type { ChipTone } from "@kenn-io/kit-ui";
import type { Course, Order, OrderItem } from "./api";

export const COURSES: Course[] = ["starter", "main", "dessert"];

const COURSE_WORDS: Record<string, { one: string; many: string }> = {
  starter: { one: "Starter", many: "Starters" },
  main: { one: "Main", many: "Mains" },
  dessert: { one: "Dessert", many: "Desserts" },
};

/** courseLabel is "Mains" (plural) or "Main". */
export function courseLabel(c: string, plural = true): string {
  const w = COURSE_WORDS[c];
  if (!w) return c;
  return plural ? w.many : w.one;
}

const STATES: Record<string, { label: string; tone: ChipTone }> = {
  held: { label: "Held", tone: "neutral" },
  fired: { label: "Waiting", tone: "info" },
  cooking: { label: "Cooking", tone: "warning" },
  resting: { label: "Resting", tone: "warning" },
  plating: { label: "Plating", tone: "warning" },
  ready: { label: "Ready", tone: "success" },
  served: { label: "Served", tone: "muted" },
  voided: { label: "Voided", tone: "muted" },
};

/** itemState is the chip for an item's state: always text plus colour. */
export function itemState(s: string): { label: string; tone: ChipTone } {
  return STATES[s] ?? { label: s, tone: "neutral" };
}

const STATUS: Record<string, { label: string; tone: ChipTone }> = {
  on_track: { label: "On track", tone: "success" },
  at_risk: { label: "At risk", tone: "warning" },
  late: { label: "Late", tone: "danger" },
  ready: { label: "Ready", tone: "info" },
};

/** courseStatus is the chip for a course's plan status. */
export function courseStatus(s: string): { label: string; tone: ChipTone } {
  return STATUS[s] ?? { label: s, tone: "neutral" };
}

type OrderNaming = Pick<Order, "kind" | "table" | "number" | "name">;

/** orderPlace is where an order goes: "Table 4" or "Takeaway #12". */
export function orderPlace(o: OrderNaming): string {
  return o.kind === "table" ? `Table ${o.table ?? ""}` : `Takeaway #${o.number}`;
}

/** orderLabel names an order in one line: "Table 4", "Table 4 (Aoife)", "Takeaway #12 (Aoife)". */
export function orderLabel(o: OrderNaming): string {
  return o.name ? `${orderPlace(o)} (${o.name})` : orderPlace(o);
}

/** orderTitle is an order's heading: its name if it has one, else where it goes. */
export function orderTitle(o: OrderNaming): string {
  return o.name || orderPlace(o);
}

/** itemName is "Burger (Well done)". */
export function itemName(it: Pick<OrderItem, "dish_name" | "modifiers">): string {
  return it.modifiers && it.modifiers.length > 0 ? `${it.dish_name} (${it.modifiers.join(", ")})` : it.dish_name;
}

/** nextLabel is what the button that moves an item one step forward says. */
export function nextLabel(state: string): string {
  switch (state) {
    case "fired":
      return "Start";
    case "cooking":
      return "Done cooking";
    case "resting":
      return "Done resting";
    case "plating":
      return "Ready";
    case "ready":
      return "Served";
  }
  return "Next";
}

/** seconds writes a duration in seconds as "8 min", "1 min 30 s" or "45 s". */
export function seconds(n: number | null | undefined): string {
  if (!n) return "–";
  const m = Math.floor(n / 60);
  const s = n % 60;
  if (m === 0) return `${s} s`;
  return s === 0 ? `${m} min` : `${m} min ${s} s`;
}

/** countdown writes the time from now to t as "4:05", or "−1:10" when past. */
export function countdown(t: string | Date, now: Date): string {
  const ms = new Date(t).getTime() - now.getTime();
  const total = Math.round(Math.abs(ms) / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${ms < 0 && total > 0 ? "−" : ""}${m}:${String(s).padStart(2, "0")}`;
}

/** durationSeconds reads "8", "8m", "1m30s", "90s" or "1:30" as seconds; null if unreadable. */
export function durationSeconds(text: string): number | null {
  const t = text.trim().toLowerCase();
  if (t === "") return 0;
  let m = /^(\d+):([0-5]\d)$/.exec(t);
  if (m) return Number(m[1]) * 60 + Number(m[2]);
  if (/^\d+(\.\d+)?$/.test(t)) return Math.round(Number(t) * 60);
  m = /^(?:(\d+)\s*m(?:in)?)?\s*(?:(\d+)\s*s)?$/.exec(t);
  if (m && (m[1] || m[2])) return Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0);
  return null;
}

/** minutesText writes seconds back for an input: "8", "1:30". */
export function minutesText(n: number | null | undefined): string {
  if (!n) return "";
  return n % 60 === 0 ? String(n / 60) : `${Math.floor(n / 60)}:${String(n % 60).padStart(2, "0")}`;
}
