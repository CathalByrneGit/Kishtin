// Rows typed from a photo of a paper record book page. Pure functions, so
// they are tested without a browser.

import type { NewRecord } from "./api";
import { dublinToDate, isClock, normaliseClock, parseTemp } from "./format";

export const PAPER_KINDS: { value: string; label: string }[] = [
  { value: "fridge", label: "Fridge" },
  { value: "freezer", label: "Freezer" },
  { value: "cook", label: "Cooking" },
  { value: "reheat", label: "Reheating" },
  { value: "hot_hold", label: "Hot holding" },
];

/** PaperRow is one row as typed: text fields, checked when sent. */
export interface PaperRow {
  date: string;
  time: string;
  type: string;
  /** the fridge or freezer, or the food */
  what: string;
  temp: string;
  signedBy: string;
  action: string;
}

export function emptyRow(prev?: PaperRow): PaperRow {
  return { date: prev?.date ?? "", time: prev?.time ?? "", type: prev?.type ?? "fridge", what: prev?.what ?? "", temp: "", signedBy: prev?.signedBy ?? "", action: "" };
}

/** toRecord turns a typed row into a record, or says what is missing. */
export function toRecord(r: PaperRow): NewRecord | string {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(r.date.trim())) return "Enter the date as YYYY-MM-DD.";
  const time = normaliseClock(r.time);
  if (!isClock(time)) return "Enter the time, such as 09:00.";
  if (!r.what.trim()) return r.type === "fridge" || r.type === "freezer" ? "Choose the fridge or freezer." : "Enter the food.";
  const t = parseTemp(r.temp);
  if (t === null) return "Enter the temperature.";
  const reading: NewRecord["reading"] = { temp_c: t, taken_at: dublinToDate(r.date.trim(), time).toISOString() };
  if (r.type === "fridge" || r.type === "freezer") reading.unit = r.what.trim();
  else reading.food = r.what.trim();
  if (r.signedBy.trim()) reading.signed_by = r.signedBy.trim();
  const rec: NewRecord = { type: r.type, reading };
  if (r.action.trim()) rec.corrective_action = r.action.trim();
  return rec;
}

/** fromRecord fills a typed row from a draft record read by the local model. */
export function fromRecord(rec: NewRecord, dublinDate: (d: Date) => string, dublinTime: (d: Date) => string): PaperRow {
  const rd = rec.reading;
  const at = rd.taken_at ? new Date(rd.taken_at) : null;
  return {
    date: at ? dublinDate(at) : "",
    time: at ? dublinTime(at) : "",
    type: PAPER_KINDS.some((k) => k.value === rec.type) ? rec.type : "fridge",
    what: rd.unit || rd.food || "",
    temp: rd.temp_c === undefined || rd.temp_c === null ? "" : String(rd.temp_c),
    signedBy: rd.signed_by ?? "",
    action: rec.corrective_action ?? "",
  };
}
