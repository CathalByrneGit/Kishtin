// Checks kept on this device while Kishtin could not be reached. They are
// not HACCP records until Kishtin has saved them: the screens say so, and
// each is sent when the connection is back, by the person who logged it.
// Pure functions, so they can be tested; kept.svelte.ts holds the state.

import type { NewRecord } from "./api";

export interface KeptCheck {
  /** id is sent as client_id, so a check sent twice is saved once. */
  id: string;
  /** who kept it; only they send it, so the record names who did the check */
  userId: string;
  userName: string;
  /** when it was kept, which is when the check was done (ISO) */
  keptAt: string;
  /** what to show: "Walk-in fridge 3.4°C" */
  label: string;
  body: NewRecord;
  /** why Kishtin refused it when it was sent, if it did */
  problem?: string;
}

export const KEPT_KEY = "kishtin-kept-checks";
/** MAX_KEPT stops a device that never reconnects filling its storage. */
export const MAX_KEPT = 200;

/** parseKept reads the stored list, dropping anything malformed. */
export function parseKept(raw: string | null): KeptCheck[] {
  if (!raw) return [];
  try {
    const v: unknown = JSON.parse(raw);
    if (!Array.isArray(v)) return [];
    return v.filter(
      (k): k is KeptCheck =>
        typeof k === "object" && k !== null && typeof k.id === "string" && typeof k.userId === "string" && typeof k.keptAt === "string" && typeof k.body === "object" && k.body !== null,
    );
  } catch {
    return [];
  }
}

/**
 * toSend is a kept check as it goes to Kishtin: with its ID, and with the
 * time it was kept as the time the check was done, unless the form gave one.
 */
export function toSend(k: KeptCheck): NewRecord {
  return { ...k.body, client_id: k.id, reading: { ...k.body.reading, taken_at: k.body.reading.taken_at ?? k.keptAt } };
}

/** mine are the kept checks the logged-in person can send. */
export function mine(list: KeptCheck[], userId: string | undefined): KeptCheck[] {
  return userId ? list.filter((k) => k.userId === userId) : [];
}

/** others names the other people with kept checks waiting on this device. */
export function others(list: KeptCheck[], userId: string | undefined): string[] {
  return [...new Set(list.filter((k) => k.userId !== userId).map((k) => k.userName))];
}

/** summary is the one line the banner shows. */
export function summary(list: KeptCheck[], userId: string | undefined): string {
  const n = list.length;
  if (n === 0) return "";
  const who = others(list, userId);
  let s = `${n} ${n === 1 ? "check is" : "checks are"} kept on this device and not in Kishtin yet.`;
  if (who.length > 0) s += ` ${who.join(" and ")} must log in here to send theirs.`;
  return s;
}
