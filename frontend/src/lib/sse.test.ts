import { expect, test } from "bun:test";
import { parseEvents } from "./sse";

test("parseEvents splits complete events and keeps the rest", () => {
  const { events, rest } = parseEvents(
    'event: ready\ndata: {}\n\n: ping\n\nevent: change\ndata: {"kind":"record","id":"r1"}\n\nevent: chan',
  );
  expect(events).toEqual([
    { event: "ready", data: "{}" },
    { event: "change", data: '{"kind":"record","id":"r1"}' },
  ]);
  expect(rest).toBe("event: chan");
});
