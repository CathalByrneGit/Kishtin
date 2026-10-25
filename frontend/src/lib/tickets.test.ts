import { expect, test } from "bun:test";
import type { KitchenView } from "./api";
import { active, allDay, lineKey, recentlyDone, stationTickets, stepBack } from "./tickets";

const MIN = 60e9; // a minute in nanoseconds, as timings arrive

function item(id: string, order: string, state: string, extra: Record<string, unknown> = {}) {
  return {
    id,
    order_id: order,
    position: 1,
    dish_id: "d-burger",
    dish_name: "Burger",
    course: "main",
    modifiers: [] as string[],
    state,
    station_id: "grill",
    hold_seconds: 180,
    batchable: true,
    timing: { cook: 8 * MIN, rest: 2 * MIN, plate: MIN },
    state_at: "2026-10-25T18:00:00Z",
    ...extra,
  };
}

function order(id: string, n: number, table: string, items: ReturnType<typeof item>[], course = "fired") {
  return {
    id,
    number: n,
    kind: "table",
    table,
    status: "open",
    created_by: "u",
    created_at: "2026-10-25T17:50:00Z",
    courses: [{ order_id: id, course: "main", state: course, fired_at: "2026-10-25T17:55:00Z", target_at: "2026-10-25T18:10:00Z" }],
    items,
  };
}

const view = {
  at: "2026-10-25T18:00:00Z",
  replan_seconds: 15,
  stations: [],
  orders: [
    order("o4", 1, "4", [item("a", "o4", "fired"), item("b", "o4", "cooking"), item("f", "o4", "fired", { station_id: "fryer", dish_id: "d-fries", dish_name: "Fries" })]),
    order("o7", 2, "7", [item("c", "o7", "fired", { modifiers: ["Well done"] }), item("d", "o7", "ready", { state_at: "2026-10-25T17:58:00Z" })]),
    order("o9", 3, "9", [item("e", "o9", "held")], "held"),
  ],
  plan: {
    at: "2026-10-25T18:00:00Z",
    settings_version: "v",
    strategy: "backward",
    courses: [{ order_id: "o4", label: "Table 4", course: "main", status: "at_risk", reason: "late" }],
    stations: [
      {
        station_id: "grill",
        name: "Grill",
        actions: [
          { station_id: "grill", dish_id: "d-burger", dish_name: "Burger", quantity: 1, item_ids: ["a"], order_ids: ["o4"], labels: ["Table 4"], start_at: "2026-10-25T18:00:00Z", now: true, text: "", reason: "start by 18:00" },
          { station_id: "grill", dish_id: "d-burger", dish_name: "Burger", modifiers: ["Well done"], quantity: 1, item_ids: ["c"], order_ids: ["o7"], labels: ["Table 7"], start_at: "2026-10-25T18:05:00Z", now: false, text: "", reason: "start by 18:05" },
        ],
        in_progress: [{ item_id: "b", order_id: "o4", label: "Table 4", dish_name: "Burger", state: "cooking", phase_ends_at: "2026-10-25T18:03:00Z" }],
      },
    ],
  },
} as unknown as KitchenView;

test("station tickets: one per fired course with work, soonest first", () => {
  const t = stationTickets(view, "grill");
  expect(t.map((x) => x.order.table)).toEqual(["4", "7"]);
  expect(t[0]!.lines.map((l) => l.item.id)).toEqual(["a", "b"]);
  expect(t[0]!.lines[0]!.action?.now).toBe(true);
  expect(t[0]!.lines[1]!.progress?.phase_ends_at).toBe("2026-10-25T18:03:00Z");
  expect(t[0]!.status?.status).toBe("at_risk");
  expect(t[1]!.lines.map((l) => l.item.state)).toEqual(["fired", "ready"], );
  expect(stationTickets(view, "fryer").map((x) => x.order.table)).toEqual(["4"]);
});

test("all day counts dishes by state, with what to start now", () => {
  const rows = allDay(view, "grill");
  expect(rows.map((r) => r.name)).toEqual(["Burger", "Burger (Well done)"]);
  expect(rows[0]).toMatchObject({ held: 1, toStart: 1, cooking: 1, ready: 1, startNow: ["a"] });
  expect(rows[1]).toMatchObject({ toStart: 1, ready: 0, startNow: [] });
  expect(active(rows[0]!)).toBe(2);
  const kitchen = allDay(view);
  expect(kitchen.map((r) => r.name)).toContain("Fries");
  expect(lineKey(view.orders[1]!.items[0]!)).toBe("d-burger|Well done");
});

test("recently done and undo", () => {
  const done = recentlyDone(view, "grill", new Date("2026-10-25T18:05:00Z"));
  expect(done.map((d) => d.item.id)).toEqual(["d"]);
  expect(recentlyDone(view, "grill", new Date("2026-10-25T19:00:00Z"))).toEqual([]);
  expect(stepBack({ state: "ready", timing: { cook: MIN, rest: MIN, plate: MIN } })).toBe("plating");
  expect(stepBack({ state: "ready", timing: { cook: MIN, rest: MIN, plate: 0 } })).toBe("resting");
  expect(stepBack({ state: "ready", timing: { cook: MIN, rest: 0, plate: 0 } })).toBe("cooking");
  expect(stepBack({ state: "ready", timing: { cook: 0, rest: 0, plate: 0 } })).toBe("fired");
});
