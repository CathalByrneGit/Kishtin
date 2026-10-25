import { expect, test } from "bun:test";
import { cellText, gapsByDate, headerGroups, monthLabel, recentMonths } from "./books";

test("months", () => {
  expect(monthLabel("2026-10")).toBe("October 2026");
  expect(recentMonths(new Date("2026-02-15T12:00:00Z"), 3)).toEqual(["2026-02", "2026-01", "2025-12"]);
  // Dublin is on UTC from 25 October, so 23:30 UTC on the 31st is still October.
  expect(recentMonths(new Date("2026-10-31T23:30:00Z"), 1)).toEqual(["2026-10"]);
  // 30 September 23:30 UTC is 1 October 00:30 in Dublin (summer time).
  expect(recentMonths(new Date("2026-09-30T23:30:00Z"), 1)).toEqual(["2026-10"]);
});

test("cell text carries the flag as a word", () => {
  expect(cellText({ text: "3.4°C" })).toBe("3.4°C");
  expect(cellText({ text: "7.2°C", flag: "fail" })).toBe("7.2°C FAIL");
  expect(cellText({ text: "6.0°C", flag: "borderline" })).toBe("6.0°C CHECK");
  expect(cellText({ text: "missed", flag: "missed" })).toBe("missed");
});

test("gaps by date", () => {
  expect(
    gapsByDate([
      { date: "2026-10-02", unit: "Walk-in fridge", check: "AM" },
      { date: "2026-10-01", unit: "Prep fridge", check: "PM" },
      { date: "2026-10-02", unit: "Prep fridge", check: "AM" },
    ]),
  ).toEqual([
    { date: "2026-10-01", checks: ["Prep fridge PM"] },
    { date: "2026-10-02", checks: ["Walk-in fridge AM", "Prep fridge AM"] },
  ]);
});

test("header groups", () => {
  const col = (label: string, group?: string) => ({ label, group, width: 10 });
  expect(headerGroups([col("Date"), col("AM", "Walk-in"), col("PM", "Walk-in"), col("AM", "Prep"), col("Sign")])).toEqual([
    { label: "", span: 1 },
    { label: "Walk-in", span: 2 },
    { label: "Prep", span: 1 },
    { label: "", span: 1 },
  ]);
});
