// Formatting and time helpers. Every date and time is shown in
// Europe/Dublin, whatever time zone the device is set to.

export const ZONE = "Europe/Dublin";

const partsFmt = new Intl.DateTimeFormat("en-IE", {
  timeZone: ZONE,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
});

interface Parts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

/** dublinParts returns the wall-clock parts of t in Dublin. */
export function dublinParts(t: Date): Parts {
  const out: Record<string, number> = {};
  for (const p of partsFmt.formatToParts(t)) {
    if (p.type !== "literal") out[p.type] = Number(p.value);
  }
  return {
    year: out.year ?? 0,
    month: out.month ?? 0,
    day: out.day ?? 0,
    hour: out.hour ?? 0,
    minute: out.minute ?? 0,
    second: out.second ?? 0,
  };
}

const pad = (n: number) => String(n).padStart(2, "0");

/** dublinDate is t's date in Dublin, as YYYY-MM-DD. */
export function dublinDate(t: Date): string {
  const p = dublinParts(t);
  return `${p.year}-${pad(p.month)}-${pad(p.day)}`;
}

/** dublinTime is t's time in Dublin, as HH:MM. */
export function dublinTime(t: Date | string): string {
  const p = dublinParts(typeof t === "string" ? new Date(t) : t);
  return `${pad(p.hour)}:${pad(p.minute)}`;
}

/** addDays moves a YYYY-MM-DD date by n days. */
export function addDays(date: string, n: number): string {
  const [y, m, d] = date.split("-").map(Number);
  const t = new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, (d ?? 1) + n));
  return `${t.getUTCFullYear()}-${pad(t.getUTCMonth() + 1)}-${pad(t.getUTCDate())}`;
}

/**
 * dublinToDate turns a Dublin wall-clock date and HH:MM time into an
 * instant, allowing for summer time.
 */
export function dublinToDate(date: string, hhmm: string): Date {
  const [y, m, d] = date.split("-").map(Number);
  const [hh, mm] = hhmm.split(":").map(Number);
  const wanted = Date.UTC(y ?? 1970, (m ?? 1) - 1, d ?? 1, hh ?? 0, mm ?? 0);
  // Guess UTC, see what Dublin shows for it, and correct by the difference.
  let guess = wanted;
  for (let i = 0; i < 2; i++) {
    const p = dublinParts(new Date(guess));
    const shown = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute);
    guess += wanted - shown;
  }
  return new Date(guess);
}

const longDateFmt = new Intl.DateTimeFormat("en-IE", {
  timeZone: "UTC",
  weekday: "long",
  day: "numeric",
  month: "long",
});

/** longDate writes a YYYY-MM-DD date as "Friday 1 May". */
export function longDate(date: string): string {
  const [y, m, d] = date.split("-").map(Number);
  const parts = longDateFmt.formatToParts(new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, d ?? 1)));
  const get = (t: string) => parts.find((p) => p.type === t)?.value ?? "";
  // Built from parts: browsers differ on commas between them.
  return `${get("weekday")} ${get("day")} ${get("month")}`;
}

/** temp writes a temperature as "3.4°C" or "−18.0°C", with a true minus sign. */
export function temp(c: number | null | undefined): string {
  if (c === null || c === undefined || Number.isNaN(c)) return "";
  const s = Math.abs(c).toFixed(1);
  return `${c < 0 && s !== "0.0" ? "−" : ""}${s}°C`;
}

/** parseTemp reads a typed temperature, accepting "−", "-" and ",". */
export function parseTemp(s: string): number | null {
  const t = s.trim().replace("−", "-").replace(",", ".");
  if (!/^-?\d+(\.\d+)?$/.test(t)) return null;
  return Number(t);
}

/** isClock reports whether s is a 24-hour HH:MM time. */
export function isClock(s: string): boolean {
  return /^([01]\d|2[0-3]):[0-5]\d$/.test(s.trim());
}

/** normaliseClock turns "930", "9:30" or "09.30" into "09:30". */
export function normaliseClock(s: string): string {
  const t = s.trim().replace(".", ":");
  let m = /^(\d{1,2}):(\d{2})$/.exec(t);
  if (!m) m = /^(\d{1,2})(\d{2})$/.exec(t);
  if (!m) return s.trim();
  return `${pad(Number(m[1]))}:${m[2]}`;
}

export interface CoolingTimes {
  cookedAt: Date;
  intoFridgeAt: Date;
  /** minutes between cooking and the fridge */
  minutes: number;
}

/**
 * coolingTimes works out when cooking finished and when food went into the
 * fridge, from two HH:MM times on a Dublin date. A fridge time earlier than
 * the cooking time means cooking finished the evening before (over
 * midnight). It returns null when the gap would be 12 hours or more, which
 * means the times need checking.
 */
export function coolingTimes(date: string, cooked: string, fridge: string): CoolingTimes | null {
  if (!isClock(cooked) || !isClock(fridge)) return null;
  const into = dublinToDate(date, fridge);
  let start = dublinToDate(date, cooked);
  if (start > into) start = dublinToDate(addDays(date, -1), cooked);
  const minutes = Math.round((into.getTime() - start.getTime()) / 60000);
  if (minutes >= 12 * 60) return null;
  return { cookedAt: start, intoFridgeAt: into, minutes };
}

/** shortDate writes a YYYY-MM-DD date as "Fri 1 May". */
export function shortDate(date: string): string {
  const [weekday = "", ...rest] = longDate(date).split(" ");
  return [weekday.slice(0, 3), ...rest].join(" ");
}
