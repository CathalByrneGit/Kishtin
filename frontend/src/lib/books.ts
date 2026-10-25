// Helpers for the record books screen. Pure functions, so they are tested
// without a browser.

import type { Schemas } from "./api";
import { dublinParts } from "./format";

type Cell = Schemas["Cell"];
type Gap = Schemas["Gap"];

const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

/** monthLabel turns 2026-10 into "October 2026". */
export function monthLabel(month: string): string {
  const [y, m] = month.split("-").map(Number);
  return m && y ? `${MONTHS[m - 1]} ${y}` : month;
}

/** recentMonths lists this month (in Dublin) and the n-1 before it, newest first, as YYYY-MM. */
export function recentMonths(now: Date, n: number): string[] {
  const p = dublinParts(now);
  const out: string[] = [];
  let y = p.year;
  let m = p.month;
  for (let i = 0; i < n; i++) {
    out.push(`${y}-${String(m).padStart(2, "0")}`);
    m--;
    if (m === 0) {
      m = 12;
      y--;
    }
  }
  return out;
}

/** cellText is what a booklet box shows, with its flag as a word: status is never colour alone. */
export function cellText(c: Cell): string {
  switch (c.flag) {
    case "fail":
      return `${c.text} FAIL`.trim();
    case "borderline":
      return `${c.text} CHECK`.trim();
    case "missed":
      return "missed";
  }
  return c.text;
}

/** gapsByDate groups missed checks by day, in date order: "Walk-in fridge AM", ... */
export function gapsByDate(gaps: Gap[]): { date: string; checks: string[] }[] {
  const by = new Map<string, string[]>();
  for (const g of gaps) {
    const list = by.get(g.date) ?? [];
    list.push(`${g.unit} ${g.check}`);
    by.set(g.date, list);
  }
  return [...by].sort(([a], [b]) => a.localeCompare(b)).map(([date, checks]) => ({ date, checks }));
}

export interface HeaderGroup {
  label: string;
  span: number;
}

/** headerGroups merges neighbouring columns with the same group into one heading. */
export function headerGroups(cols: Schemas["Column"][]): HeaderGroup[] {
  const out: HeaderGroup[] = [];
  for (const c of cols) {
    const g = c.group ?? "";
    const last = out.at(-1);
    if (last && g !== "" && last.label === g) last.span++;
    else out.push({ label: g, span: 1 });
  }
  return out;
}
