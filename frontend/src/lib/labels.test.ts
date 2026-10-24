import { expect, test } from "bun:test";
import type { HaccpRecord } from "./api";
import { details, limitText, outcomeStyle, title, typeLabel } from "./labels";

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
