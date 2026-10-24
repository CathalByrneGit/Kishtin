import { describe, expect, test } from "bun:test";
import type { HaccpRecord } from "./api";
import { details, limitText, limitWords, outcomeStyle, title, typeLabel } from "./labels";

const rec = (r: Partial<HaccpRecord>): HaccpRecord => ({ type: "fridge", reading: {}, ...r }) as HaccpRecord;

test("labels", () => {
  expect(typeLabel("hot_hold")).toBe("Hot holding");
  expect(outcomeStyle("borderline")).toEqual({ label: "Check", tone: "warning" });
  expect(outcomeStyle("recorded").label).toBe("Recorded");
  expect(limitText("freezer", -18)).toBe("−18.0°C or colder");
  expect(limitText("fridge", 5)).toBe("5.0°C or below");
});

test("title and details", () => {
  expect(title(rec({ reading: { unit: "Walk-in fridge", temp_c: 3.4 } }))).toBe("Walk-in fridge");
  expect(title(rec({ type: "delivery", reading: { supplier: "Synthetic Dairy", product: "Cream" } }))).toBe("Synthetic Dairy: Cream");
  expect(
    details(rec({ type: "delivery", reading: { temp_c: 6.2, delivery_kind: "chilled", packaging_ok: false, in_date: true } })),
  ).toBe("6.2°C · chilled · packaging damaged");
  expect(details(rec({ type: "batch_label", reading: { use_by: "2026-05-06" } }))).toBe("use by Wednesday 6 May");
});

describe("limitWords", () => {
  test("describes each kind of limit", () => {
    expect(limitWords({ check: "fridge", max_c: 5 })).toBe("5.0°C or below");
    expect(limitWords({ check: "delivery_frozen", max_c: -18, borderline_max_c: -15 })).toBe("−18.0°C or colder, borderline to −15.0°C");
    expect(limitWords({ check: "cook", combos: [{ min_c: 75, hold_seconds: 0 }, { min_c: 70, hold_seconds: 120 }] })).toBe(
      "core 75.0°C or above, or 70.0°C for 2 min",
    );
    expect(
      limitWords({ check: "cooling", cooling: { into_fridge_within_seconds: 7200, cutoff_after_seconds: 9000, cutoff_max_c: 10 } }),
    ).toBe("into the fridge within 2 h; serve or discard if above 10.0°C at 2 h 30 min");
  });
});
