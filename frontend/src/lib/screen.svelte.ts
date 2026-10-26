// Which layout this device shows: the desktop app with its sidebar, or a
// full-screen kitchen layout for the pass, one station, or a phone. The
// choice belongs to the device, not the person, so it is kept in the
// browser. A phone-sized screen starts on the phone layout.

import { load, save } from "./storage";

export type ScreenKind = "desktop" | "pass" | "station" | "phone";

const KEY = "kishtin-screen";

interface Stored {
  kind: ScreenKind;
  station: string | null;
}

function initial(): Stored {
  const raw = load(KEY);
  if (raw) {
    try {
      const s = JSON.parse(raw) as Stored;
      if (["desktop", "pass", "station", "phone"].includes(s.kind)) return { kind: s.kind, station: s.station ?? null };
    } catch {
      // Fall through to the default.
    }
  }
  const narrow = typeof window !== "undefined" && window.matchMedia?.("(max-width: 640px)").matches;
  return { kind: narrow ? "phone" : "desktop", station: null };
}

export const screen = $state(initial());

/** setScreen changes this device's layout and remembers it. */
export function setScreen(kind: ScreenKind, station: string | null = null): void {
  screen.kind = kind;
  screen.station = kind === "station" ? station : null;
  save(KEY, JSON.stringify({ kind: screen.kind, station: screen.station }));
}

export const SCREEN_LABELS: Record<ScreenKind, string> = {
  desktop: "Desktop, with the sidebar",
  pass: "Pass screen",
  station: "Station screen",
  phone: "Phone",
};
