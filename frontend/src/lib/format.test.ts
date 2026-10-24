import { describe, expect, test } from "bun:test";
import {
  addDays,
  coolingTimes,
  dublinDate,
  dublinTime,
  dublinToDate,
  isClock,
  longDate,
  normaliseClock,
  parseTemp,
  temp,
} from "./format";

describe("temperatures", () => {
  test("formats with one decimal and a true minus sign", () => {
    expect(temp(3.4)).toBe("3.4°C");
    expect(temp(-18)).toBe("−18.0°C");
    expect(temp(-0.01)).toBe("0.0°C");
    expect(temp(null)).toBe("");
  });
  test("parses what staff type", () => {
    expect(parseTemp("3.4")).toBe(3.4);
    expect(parseTemp("−18")).toBe(-18);
    expect(parseTemp("-18,5")).toBe(-18.5);
    expect(parseTemp("")).toBeNull();
    expect(parseTemp("3.4.5")).toBeNull();
    expect(parseTemp("hot")).toBeNull();
  });
});

describe("Dublin time", () => {
  test("summer time is UTC+1", () => {
    const t = dublinToDate("2026-05-01", "10:00");
    expect(t.toISOString()).toBe("2026-05-01T09:00:00.000Z");
    expect(dublinTime(t)).toBe("10:00");
    expect(dublinDate(t)).toBe("2026-05-01");
  });
  test("winter time is UTC", () => {
    expect(dublinToDate("2026-12-01", "10:00").toISOString()).toBe("2026-12-01T10:00:00.000Z");
  });
  test("dates near midnight", () => {
    expect(dublinDate(new Date("2026-05-01T23:30:00Z"))).toBe("2026-05-02");
    expect(addDays("2026-03-01", -1)).toBe("2026-02-28");
    expect(longDate("2026-05-01")).toBe("Friday 1 May");
  });
});

describe("clock times", () => {
  test("accepts and normalises", () => {
    expect(isClock("09:30")).toBe(true);
    expect(isClock("24:00")).toBe(false);
    expect(normaliseClock("930")).toBe("09:30");
    expect(normaliseClock("9.30")).toBe("09:30");
    expect(normaliseClock("21:05")).toBe("21:05");
  });
});

describe("cooling times", () => {
  test("same day", () => {
    const c = coolingTimes("2026-05-01", "14:00", "15:45");
    expect(c?.minutes).toBe(105);
    expect(c?.cookedAt.toISOString()).toBe("2026-05-01T13:00:00.000Z");
  });
  test("over midnight", () => {
    const c = coolingTimes("2026-05-02", "23:30", "00:45");
    expect(c?.minutes).toBe(75);
    expect(dublinDate(c!.cookedAt)).toBe("2026-05-01");
  });
  test("a gap of 12 hours or more needs checking", () => {
    expect(coolingTimes("2026-05-01", "08:00", "21:00")).toBeNull();
    expect(coolingTimes("2026-05-01", "nine", "10:00")).toBeNull();
  });
});
