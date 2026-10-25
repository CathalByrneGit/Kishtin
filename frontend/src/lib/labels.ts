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
  sign_off: "Sign-off",
  inspection: "Inspection",
  training: "Training",
  fitness: "Fitness to work",
  transport: "To a customer",
};

/** LOGGABLE_TYPES are the record types staff log as readings (a sign-off is saved by signing off). */
export const LOGGABLE_TYPES = Object.keys(TYPE_LABELS).filter((t) => t !== "sign_off");

const FITNESS_REASONS: Record<string, string> = {
  existing: "existing food handler",
  pre_employment: "pre-employment",
  return_after_illness: "return to work after illness",
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
  if (r.type === "sign_off" && rd.period_from && rd.period_to) return `${longDate(rd.period_from)} to ${longDate(rd.period_to)}`;
  if (r.type === "inspection") return "Hygiene inspection";
  if (r.type === "training" || r.type === "fitness") return rd.staff || typeLabel(r.type);
  if (r.type === "transport") return `${rd.product ?? ""} to ${rd.customer ?? ""}`;
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
  if (rd.vehicle_ok === false) parts.push("vehicle failed its check");
  if (rd.batch_code) parts.push(`batch ${rd.batch_code}`);
  if (rd.docket_file) parts.push("docket attached");
  if (rd.use_by) parts.push(`use by ${longDate(rd.use_by)}`);
  if (rd.done === false) parts.push("not done");
  if (rd.area) parts.push(rd.area);
  if (rd.review) parts.push(reviewText(rd.review));
  if (rd.answers?.length) {
    const no = rd.answers.filter((a) => a.ok === false).length;
    const yes = rd.answers.filter((a) => a.ok === true).length;
    parts.push(`${yes} satisfactory, ${no} not`);
  }
  if (rd.topics?.length) parts.push(rd.topics.join(", "));
  if (rd.trainer) parts.push(`by ${rd.trainer}`);
  if (rd.provider) parts.push(rd.provider);
  if (rd.fitness_reason) parts.push(FITNESS_REASONS[rd.fitness_reason] ?? rd.fitness_reason);
  if (rd.fit !== undefined && rd.fit !== null) parts.push(rd.fit ? "fit to work" : "not fit to work");
  if (rd.separated === false) parts.push("raw and ready-to-eat not kept apart");
  return parts.join(" · ");
}

/** reviewText sums up what a sign-off covered. */
export function reviewText(v: { records: number; out_of_limit: number; without_action: number; missed: number }): string {
  const n = (k: number, one: string, many: string) => `${k} ${k === 1 ? one : many}`;
  return [
    n(v.records, "record", "records"),
    `${v.out_of_limit} out of limit`,
    `${v.without_action} without an action`,
    n(v.missed, "missed check", "missed checks"),
  ].join(" · ");
}

/** limitText describes a fridge or freezer limit, such as "5.0°C or below". */
export function limitText(kind: string, maxC: number | null | undefined): string {
  if (maxC === null || maxC === undefined) return "";
  return `${temp(maxC)} or ${kind === "freezer" ? "colder" : "below"}`;
}

/** readingRows lists every field of a reading as label and value, for a details panel. */
export function readingRows(r: { type: string; reading: Reading }): [string, string][] {
  const rd = r.reading;
  const rows: [string, string][] = [];
  const add = (label: string, v: string | undefined | null) => {
    if (v !== undefined && v !== null && v !== "") rows.push([label, v]);
  };
  const yes = (b: boolean | undefined | null) => (b === undefined || b === null ? undefined : b ? "Yes" : "No");
  if (rd.period_from && rd.period_to) add("Period", `${longDate(rd.period_from)} to ${longDate(rd.period_to)}`);
  if (rd.review) add("When signed", reviewText(rd.review));
  add("Unit", rd.unit);
  add("Supplier", rd.supplier);
  add("Product", rd.product);
  add("Batch code", rd.batch_code);
  add("Dish", rd.dish);
  if (r.type === "delivery") add("Vehicle check", yes(rd.vehicle_ok) === undefined ? undefined : rd.vehicle_ok ? "Passed" : "Failed");
  add("Food", rd.food);
  add("Food category", rd.food_category);
  add("Delivery", rd.delivery_kind);
  add(r.type === "cooling" ? "Core temperature going in" : "Temperature", rd.temp_c === undefined || rd.temp_c === null ? undefined : temp(rd.temp_c));
  add("Held for", rd.hold_seconds ? `${Math.round(rd.hold_seconds / 60)} min` : undefined);
  add("Finished cooking", rd.cooked_at ? dublinTime(rd.cooked_at) : undefined);
  add("Into fridge", rd.into_fridge_at ? dublinTime(rd.into_fridge_at) : undefined);
  add("At 2.5 h", rd.temp_at_cutoff_c === undefined || rd.temp_at_cutoff_c === null ? undefined : temp(rd.temp_at_cutoff_c));
  add("Packaging intact", yes(rd.packaging_ok));
  add("In date", yes(rd.in_date));
  add("Made on", rd.made_on ? longDate(rd.made_on) : undefined);
  add("Use by", rd.use_by ? longDate(rd.use_by) : undefined);
  add("Quantity", rd.quantity);
  add("Task", rd.task);
  add("Area", rd.area);
  add("Done", yes(rd.done));
  add("Notes", rd.notes);
  return rows;
}

/** CHECK_LABELS names each check type a limit can apply to. */
export const CHECK_LABELS: Record<string, string> = {
  fridge: "Fridge",
  freezer: "Freezer",
  cook: "Cooking, core",
  reheat: "Reheating, core",
  hot_hold: "Hot holding",
  cooling: "Cooling",
  delivery_chilled: "Chilled delivery",
  delivery_frozen: "Frozen delivery",
};

interface LimitShape {
  check: string;
  max_c?: number | null;
  borderline_max_c?: number | null;
  min_c?: number | null;
  combos?: { min_c: number; hold_seconds: number }[] | null;
  cooling?: { into_fridge_within_seconds: number; cutoff_after_seconds: number; cutoff_max_c: number } | null;
}

const hours = (s: number) => {
  const m = Math.round(s / 60);
  return m % 60 === 0 ? `${m / 60} h` : m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`;
};

/** limitWords describes a limit in plain words, such as "5.0°C or below". */
export function limitWords(l: LimitShape): string {
  const parts: string[] = [];
  const colder = l.check === "freezer" || l.check === "delivery_frozen";
  if (l.max_c !== undefined && l.max_c !== null) parts.push(`${temp(l.max_c)} or ${colder ? "colder" : "below"}`);
  if (l.borderline_max_c !== undefined && l.borderline_max_c !== null) parts.push(`borderline to ${temp(l.borderline_max_c)}`);
  if (l.min_c !== undefined && l.min_c !== null) parts.push(`${temp(l.min_c)} or above`);
  const combos = (l.combos ?? []).map((c) => (c.hold_seconds ? `${temp(c.min_c)} for ${hours(c.hold_seconds)}` : `${temp(c.min_c)} or above`));
  if (combos.length > 0) parts.push(`core ${combos.join(", or ")}`);
  if (l.cooling) {
    parts.push(
      `into the fridge within ${hours(l.cooling.into_fridge_within_seconds)}; serve or discard if above ${temp(l.cooling.cutoff_max_c)} at ${hours(l.cooling.cutoff_after_seconds)}`,
    );
  }
  return parts.join(", ");
}
