// How often a cleaning task is due, in days: 0 is every day.

export const EVERY_OPTIONS = [
  { value: "0", label: "Every day" },
  { value: "7", label: "Every week" },
  { value: "14", label: "Every 2 weeks" },
  { value: "28", label: "Every 4 weeks" },
];

/** everyLabel says how often a task is due: "Every week", "Every 10 days". */
export function everyLabel(days: number | null | undefined): string {
  const d = days ?? 0;
  return EVERY_OPTIONS.find((o) => Number(o.value) === (d <= 1 ? 0 : d))?.label ?? `Every ${d} days`;
}

/** everyOptions are the choices for a task, including its own if unusual. */
export function everyOptions(days: number | null | undefined): { value: string; label: string }[] {
  const d = days ?? 0;
  if (d <= 1 || EVERY_OPTIONS.some((o) => Number(o.value) === d)) return EVERY_OPTIONS;
  return [...EVERY_OPTIONS, { value: String(d), label: `Every ${d} days` }];
}

import type { Schemas } from "./api";
import { shortDate } from "./format";

type Task = Schemas["CleaningToday"];

/** dueToday reports whether a task is one of today's checks. */
export function dueToday(c: Task): boolean {
  return !c.schedule || c.schedule.status !== "not_due";
}

/**
 * scheduleStatus says where a weekly-or-longer task stands, as a chip
 * label and tone and a line of text; null for a daily task.
 */
export function scheduleStatus(c: Task): { label: string; tone: "success" | "warning" | "danger" | "neutral"; text: string } | null {
  const s = c.schedule;
  if (!s) return null;
  const Every = everyLabel(c.every_days);
  const every = Every.toLowerCase();
  switch (s.status) {
    case "done":
      return { label: "Done", tone: "success", text: `Done today, ${every}. Next due ${shortDate(s.next_due ?? "")}.` };
    case "overdue":
      return { label: "Overdue", tone: "danger", text: `Was due ${shortDate(s.next_due ?? "")}, ${every}. Last done ${shortDate(s.last_done ?? "")}.` };
    case "due":
      return s.last_done
        ? { label: "Due today", tone: "warning", text: `${Every}. Last done ${shortDate(s.last_done)}.` }
        : { label: "Due", tone: "warning", text: `${Every}. Not done yet.` };
    default:
      return { label: `Next ${shortDate(s.next_due ?? "")}`, tone: "neutral", text: `${Every}. Last done ${shortDate(s.last_done ?? "")}.` };
  }
}
