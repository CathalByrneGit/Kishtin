// Entries on the catch-up screen: one per daily check with no record.
// Pure functions, so they are tested without a browser.

import type { NewRecord, Schemas } from "./api";
import { dublinToDate, isClock, normaliseClock, parseTemp } from "./format";

type Gap = Schemas["CatchUpGap"];

/** Entry is what someone has typed for one gap. */
export interface Entry {
  gap: Gap;
  temp: string;
  done: boolean;
  time: string;
  action: string;
}

export function entryFor(gap: Gap): Entry {
  return { gap, temp: "", done: false, time: gap.time ?? "", action: "" };
}

/** filled reports whether anything has been entered for a gap. */
export function filled(e: Entry): boolean {
  return e.gap.type === "cleaning" ? e.done : e.temp.trim() !== "";
}

/** label names a gap: the fridge and check, or the cleaning task. */
export function gapLabel(g: Gap): string {
  if (g.type === "cleaning") return g.area ? `${g.task} (${g.area})` : (g.task ?? "");
  return `${g.unit} · ${g.check}`;
}

/** toRecord turns a filled entry into a record, or says what is missing. */
export function toRecord(e: Entry, doneBy: string): NewRecord | string {
  const time = normaliseClock(e.time);
  if (!isClock(time)) return "Enter the time it was done, such as 09:00.";
  const reading: NewRecord["reading"] = { taken_at: dublinToDate(e.gap.date, time).toISOString() };
  if (doneBy.trim()) reading.signed_by = doneBy.trim();
  let rec: NewRecord;
  if (e.gap.type === "cleaning") {
    rec = { type: "cleaning", reading: { ...reading, task: e.gap.task, done: true } };
  } else {
    const t = parseTemp(e.temp);
    if (t === null) return "The temperature must be a number, such as 3.4.";
    rec = { type: e.gap.type, reading: { ...reading, unit: e.gap.unit, temp_c: t } };
  }
  if (e.action.trim()) rec.corrective_action = e.action.trim();
  return rec;
}
