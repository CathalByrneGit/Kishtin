import { expect, test } from "bun:test";
import { countdown, courseLabel, durationSeconds, itemName, minutesText, nextLabel, orderLabel, seconds } from "./kitchen";

test("labels", () => {
  expect(courseLabel("main")).toBe("Mains");
  expect(courseLabel("starter", false)).toBe("Starter");
  expect(orderLabel({ kind: "table", table: "4", number: 1 })).toBe("Table 4");
  expect(orderLabel({ kind: "takeaway", number: 12, name: "Aoife" })).toBe("Takeaway #12 (Aoife)");
  expect(itemName({ dish_name: "Burger", modifiers: ["Well done"] })).toBe("Burger (Well done)");
  expect(itemName({ dish_name: "Burger", modifiers: [] })).toBe("Burger");
  expect(nextLabel("cooking")).toBe("Done cooking");
});

test("durations", () => {
  expect(seconds(480)).toBe("8 min");
  expect(seconds(90)).toBe("1 min 30 s");
  expect(seconds(45)).toBe("45 s");
  expect(seconds(0)).toBe("–");
  expect(durationSeconds("8")).toBe(480);
  expect(durationSeconds("1:30")).toBe(90);
  expect(durationSeconds("1m30s")).toBe(90);
  expect(durationSeconds("90s")).toBe(90);
  expect(durationSeconds("0.5")).toBe(30);
  expect(durationSeconds("")).toBe(0);
  expect(durationSeconds("soon")).toBeNull();
  expect(minutesText(480)).toBe("8");
  expect(minutesText(90)).toBe("1:30");
  expect(minutesText(0)).toBe("");
});

test("countdown", () => {
  const now = new Date("2026-05-01T18:00:00Z");
  expect(countdown("2026-05-01T18:04:05Z", now)).toBe("4:05");
  expect(countdown("2026-05-01T17:58:50Z", now)).toBe("−1:10");
  expect(countdown(now, now)).toBe("0:00");
});
