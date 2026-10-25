import { expect, test } from "bun:test";
import { dublinDate, dublinTime } from "./format";
import { emptyRow, fromRecord, toRecord } from "./paper";

test("paper rows", () => {
  const r = { ...emptyRow(), date: "2026-04-28", time: "9:00", what: "Walk-in fridge", temp: "3.5", signedBy: "A Jones" };
  const rec = toRecord(r);
  expect(typeof rec).toBe("object");
  if (typeof rec === "string") return;
  expect(rec.type).toBe("fridge");
  expect(rec.reading.unit).toBe("Walk-in fridge");
  expect(rec.reading.taken_at).toBe("2026-04-28T08:00:00.000Z");
  expect(rec.reading.signed_by).toBe("A Jones");
  expect(toRecord({ ...r, date: "28/4" })).toContain("YYYY-MM-DD");
  expect(toRecord({ ...r, temp: "" })).toBe("Enter the temperature.");
  expect(toRecord({ ...r, type: "hot_hold", what: "" })).toBe("Enter the food.");

  const back = fromRecord(rec, dublinDate, dublinTime);
  expect(back).toEqual({ ...r, time: "09:00" });
  expect(emptyRow(back).date).toBe("2026-04-28");
  expect(emptyRow(back).temp).toBe("");
});
