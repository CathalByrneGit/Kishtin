// The kept checks on this device, and sending them when Kishtin is back.

import { ApiError, api, OfflineError, type NewRecord } from "./api";
import { KEPT_KEY, MAX_KEPT, mine, parseKept, toSend, type KeptCheck } from "./kept";
import { session } from "./session.svelte";
import { load, save } from "./storage";

export const kept = $state({
  list: parseKept(load(KEPT_KEY)) as KeptCheck[],
  sending: false,
});

function persist() {
  save(KEPT_KEY, kept.list.length > 0 ? JSON.stringify(kept.list) : null);
}

function newID(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}-${Math.random().toString(36).slice(2)}`;
}

/** keep stores a check on this device. It returns false when it cannot. */
export function keep(body: NewRecord, label: string): boolean {
  const u = session.user;
  if (!u || kept.list.length >= MAX_KEPT) return false;
  kept.list.push({ id: newID(), userId: u.id, userName: u.name, keptAt: new Date().toISOString(), label, body });
  persist();
  // Storage can be blocked: only say it is kept if it really is.
  return parseKept(load(KEPT_KEY)).length === kept.list.length;
}

export function remove(id: string) {
  kept.list = kept.list.filter((k) => k.id !== id);
  persist();
}

/** setAction adds the corrective action Kishtin asked for. */
export function setAction(id: string, action: string) {
  const k = kept.list.find((x) => x.id === id);
  if (!k) return;
  k.body = { ...k.body, corrective_action: action };
  persist();
}

/**
 * sendKept sends the logged-in person's kept checks, oldest first. It stops
 * if Kishtin cannot be reached. A check Kishtin refuses stays, with why.
 * It returns how many were saved.
 */
export async function sendKept(): Promise<number> {
  if (kept.sending) return 0;
  kept.sending = true;
  let sent = 0;
  try {
    for (const k of mine(kept.list, session.user?.id)) {
      try {
        await api.log(toSend(k));
        remove(k.id);
        sent++;
      } catch (err) {
        if (err instanceof OfflineError) break;
        const item = kept.list.find((x) => x.id === k.id);
        if (item) item.problem = err instanceof ApiError ? err.message : "Kishtin could not save this check.";
        persist();
      }
    }
  } finally {
    kept.sending = false;
  }
  return sent;
}

/** LISTS_KEY holds the last fridges and tasks, for logging while offline. */
export const LISTS_KEY = "kishtin-lists";

/**
 * rememberLists keeps the fridges and cleaning tasks on this device, so a
 * check can be kept when Kishtin cannot be reached even if the log form
 * was never opened here before.
 */
export async function rememberLists(): Promise<void> {
  if (!session.token) return;
  try {
    save(LISTS_KEY, JSON.stringify((await api.lists()).value));
  } catch {
    // Keep whatever was remembered before.
  }
}
