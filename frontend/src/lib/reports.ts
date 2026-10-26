// Words and numbers for the Reports screen. Pure, so they are tested
// without a browser.

/** duration says seconds as "45s", "12m 30s" or "1h 05m". */
export function duration(seconds: number): string {
  const sign = seconds < 0 ? "-" : "";
  let s = Math.abs(Math.round(seconds));
  if (s < 60) return `${sign}${s}s`;
  if (s < 3600) {
    const m = Math.floor(s / 60);
    s %= 60;
    return s ? `${sign}${m}m ${String(s).padStart(2, "0")}s` : `${sign}${m}m`;
  }
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return `${sign}${h}h ${String(m).padStart(2, "0")}m`;
}

/** hourLabel says an hour of the day, "18:00". */
export function hourLabel(h: number): string {
  return `${String(h).padStart(2, "0")}:00`;
}

/** overText says how a dish's actual time compares with its set time. */
export function overText(over: number): string {
  if (Math.abs(over) < 30) return "As set";
  return over > 0 ? `${duration(over)} slower` : `${duration(-over)} quicker`;
}

/** lastDays returns the period of the n days up to today, as YYYY-MM-DD. */
export function lastDays(n: number, today: string): { from: string; to: string } {
  const [y, m, d] = today.split("-").map(Number) as [number, number, number];
  const from = new Date(Date.UTC(y, m - 1, d - (n - 1)));
  return { from: from.toISOString().slice(0, 10), to: today };
}
