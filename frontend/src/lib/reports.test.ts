import { expect, test } from "bun:test";
import { duration, hourLabel, lastDays, overText } from "./reports";

test("durations read as minutes and seconds", () => {
  expect(duration(45)).toBe("45s");
  expect(duration(750)).toBe("12m 30s");
  expect(duration(720)).toBe("12m");
  expect(duration(3900)).toBe("1h 05m");
  expect(duration(-90)).toBe("-1m 30s");
});

test("dish times against set times", () => {
  expect(overText(120)).toBe("2m slower");
  expect(overText(-75)).toBe("1m 15s quicker");
  expect(overText(10)).toBe("As set");
  expect(hourLabel(9)).toBe("09:00");
});

test("the last n days end today", () => {
  expect(lastDays(28, "2026-10-26")).toEqual({ from: "2026-09-29", to: "2026-10-26" });
  expect(lastDays(1, "2026-03-01")).toEqual({ from: "2026-03-01", to: "2026-03-01" });
});
