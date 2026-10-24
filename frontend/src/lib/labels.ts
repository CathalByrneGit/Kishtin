// Words for record types, results and readings, shared by every screen.

import type { ChipTone } from "@kenn-io/kit-ui";
import type { HaccpRecord, Reading } from "./api";
import { dublinTime, longDate, temp } from "./format";

export const TYPE_LABELS: Record<string, string> = {
  fridge: "Fridge",
  freezer: "Freezer",
  delivery: "Delivery",
  cook: "Cooking",
  reheat: "Reheating",
  cooling: "Cooling",
  hot_hold: "Hot holding",
  batch_label: "Batch label",
  cleaning: "Cleaning",
};

export function typeLabel(t: string): string {
  return TYPE_LABELS[t] ?? t;
}

export interface OutcomeStyle {
  label: string;
  tone: ChipTone;
}

/** outcomeStyle is the pill for a result: always text plus colour. */
export function outcomeStyle(o: string): OutcomeStyle {
  switch (o) {
    case "pass":
      return { label: "Pass", tone: "success" };
    case "borderline":
      return { label: "Check", tone: "warning" };
    case "fail":
      return { label: "Fail", tone: "danger" };
    default:
      return { label: "Recorded", tone: "neutral" };
  }
}

/** title is what a record is about: the fridge, food, product or task. */
export function title(r: HaccpRecord): string {
  const rd = r.reading;
  const what = rd.unit || rd.food || rd.product || rd.task || typeLabel(r.type);
  return rd.supplier ? `${rd.supplier}: ${what}` : what;
}

/** details sums up the reading in a short line. */
export function details(r: { type: string; reading: Reading }): string {
  const rd = r.reading;
  const parts: string[] = [];
  if (rd.temp_c !== undefined && rd.temp_c !== null) parts.push(temp(rd.temp_c));
  if (rd.hold_seconds) parts.push(`held ${Math.round(rd.hold_seconds / 60)} min`);
  if (rd.cooked_at) {
    let s = `cooked ${dublinTime(rd.cooked_at)}`;
    if (rd.into_fridge_at) s += `, into fridge ${dublinTime(rd.into_fridge_at)}`;
    parts.push(s);
  }
  if (rd.temp_at_cutoff_c !== undefined && rd.temp_at_cutoff_c !== null) parts.push(`${temp(rd.temp_at_cutoff_c)} at 2.5 h`);
  if (rd.delivery_kind) parts.push(rd.delivery_kind);
  if (rd.packaging_ok === false) parts.push("packaging damaged");
  if (rd.in_date === false) parts.push("out of date");
  if (rd.docket_file) parts.push("docket attached");
  if (rd.use_by) parts.push(`use by ${longDate(rd.use_by)}`);
  if (rd.done === false) parts.push("not done");
  if (rd.area) parts.push(rd.area);
  return parts.join(" · ");
}

/** limitText describes a fridge or freezer limit, such as "5.0°C or below". */
export function limitText(kind: string, maxC: number | null | undefined): string {
  if (maxC === null || maxC === undefined) return "";
  return `${temp(maxC)} or ${kind === "freezer" ? "colder" : "below"}`;
}
