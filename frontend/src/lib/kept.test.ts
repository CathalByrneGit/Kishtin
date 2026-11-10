import { expect, test } from "bun:test";
import { mine, others, parseKept, summary, toSend, type KeptCheck } from "./kept";

const k = (id: string, userId: string, userName: string): KeptCheck => ({
  id,
  userId,
  userName,
  keptAt: "2026-11-10T09:15:00.000Z",
  label: "Walk-in fridge 3.4°C",
  body: { type: "fridge", reading: { unit: "Walk-in fridge", temp_c: 3.4 } },
});

test("parseKept drops anything that is not a kept check", () => {
  expect(parseKept(null)).toEqual([]);
  expect(parseKept("not json")).toEqual([]);
  expect(parseKept('{"id":"a"}')).toEqual([]);
  const good = k("a", "u1", "Liam Cook");
  expect(parseKept(JSON.stringify([good, { id: 1 }, null, "x"]))).toEqual([good]);
});

test("a kept check is sent with its ID and the time it was kept", () => {
  const sent = toSend(k("a", "u1", "Liam Cook"));
  expect(sent.client_id).toBe("a");
  expect(sent.reading.taken_at).toBe("2026-11-10T09:15:00.000Z");
  const earlier = k("b", "u1", "Liam Cook");
  earlier.body.reading.taken_at = "2026-11-10T07:00:00.000Z";
  expect(toSend(earlier).reading.taken_at).toBe("2026-11-10T07:00:00.000Z");
});

test("only the person who kept a check sends it", () => {
  const list = [k("a", "u1", "Liam Cook"), k("b", "u2", "Aoife Chef"), k("c", "u1", "Liam Cook")];
  expect(mine(list, "u1").map((x) => x.id)).toEqual(["a", "c"]);
  expect(mine(list, undefined)).toEqual([]);
  expect(others(list, "u1")).toEqual(["Aoife Chef"]);
  expect(summary([], "u1")).toBe("");
  expect(summary([list[0]!], "u1")).toBe("1 check is kept on this device and not in Kishtin yet.");
  expect(summary(list, "u1")).toBe("3 checks are kept on this device and not in Kishtin yet. Aoife Chef must log in here to send theirs.");
});
