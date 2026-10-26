import { expect, test } from "bun:test";
import { entryFor, filled, gapLabel, toRecord } from "./catchup";

const fridge = { date: "2026-10-20", type: "fridge", unit: "Walk-in fridge", check: "AM", time: "09:00" };
const clean = { date: "2026-10-20", type: "cleaning", task: "Floors swept & mopped" };

test("an entry starts at the check's window", () => {
  const e = entryFor(fridge);
  expect(e.time).toBe("09:00");
  expect(filled(e)).toBe(false);
  expect(gapLabel(fridge)).toBe("Walk-in fridge · AM");
  expect(gapLabel({ ...clean, area: "Kitchen" })).toBe("Floors swept & mopped (Kitchen)");
});

test("a fridge entry becomes a record at the time it was done", () => {
  const e = { ...entryFor(fridge), temp: "3,4", time: "9.15" };
  expect(filled(e)).toBe(true);
  expect(toRecord(e, " Aoife ")).toEqual({
    type: "fridge",
    reading: { unit: "Walk-in fridge", temp_c: 3.4, taken_at: "2026-10-20T08:15:00.000Z", signed_by: "Aoife" },
  });
  expect(toRecord({ ...e, temp: "warm" }, "")).toContain("must be a number");
});

test("a cleaning entry needs a time", () => {
  const e = { ...entryFor(clean), done: true };
  expect(toRecord(e, "")).toContain("Enter the time");
  expect(toRecord({ ...e, time: "22:00" }, "")).toEqual({
    type: "cleaning",
    reading: { task: "Floors swept & mopped", done: true, taken_at: "2026-10-20T21:00:00.000Z" },
  });
});
