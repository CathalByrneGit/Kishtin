// Live sync: listens to the server's change stream so every screen shows
// new records straight away. EventSource cannot send the API key header,
// so the stream is read with fetch.

import { authHeaders } from "./api";
import { parseEvents } from "./sse";

export type LiveStatus = "connecting" | "synced" | "offline";

export const live = $state({
  status: "connecting" as LiveStatus,
  /** when the last data arrived from the server */
  lastSeen: null as Date | null,
  /** set when the last save failed because the server could not be reached */
  saveFailed: false,
});

type Listener = (kind: string) => void;
const listeners = new Set<Listener>();

/** onChange calls fn whenever the server reports a change. */
export function onChange(fn: Listener): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

let controller: AbortController | null = null;
let retry = 1000;

async function run(signal: AbortSignal): Promise<void> {
  live.status = "connecting";
  const res = await fetch("/api/events", { headers: authHeaders(), signal });
  if (!res.ok || !res.body) throw new Error(`events: ${res.status}`);
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) throw new Error("events: stream ended");
    buffer += value;
    const parsed = parseEvents(buffer);
    buffer = parsed.rest;
    live.lastSeen = new Date();
    for (const e of parsed.events) {
      if (e.event === "ready") {
        live.status = "synced";
        retry = 1000;
        for (const fn of listeners) fn("ready");
      } else if (e.event === "change") {
        try {
          const { kind } = JSON.parse(e.data) as { kind: string };
          for (const fn of listeners) fn(kind);
        } catch {
          // Ignore a malformed event; the next one refreshes anyway.
        }
      } else if (e.event === "ended") {
        throw new Error("events: session ended");
      }
    }
  }
}

/** connect starts (or restarts) the change stream, retrying with backoff. */
export function connect(): void {
  controller?.abort();
  const c = new AbortController();
  controller = c;
  void (async () => {
    while (!c.signal.aborted) {
      try {
        await run(c.signal);
      } catch {
        if (c.signal.aborted) return;
        live.status = "offline";
        await new Promise((r) => setTimeout(r, retry));
        retry = Math.min(retry * 2, 15_000);
      }
    }
  })();
}

/** disconnect stops the change stream. */
export function disconnect(): void {
  controller?.abort();
  controller = null;
}
