import { describe, expect, test } from "bun:test";
import { SpeechDetector, downsample, encodeWav, rms } from "./wake";

/** feed pushes a level every 50 ms for a duration and collects the events. */
function feed(d: SpeechDetector, level: number, ms: number, t0: number): { events: string[]; t: number } {
  const events: string[] = [];
  let t = t0;
  for (; t < t0 + ms; t += 50) {
    const e = d.push(level, t);
    if (e) events.push(e);
  }
  return { events, t };
}

describe("SpeechDetector", () => {
  test("finds a phrase above the background noise", () => {
    const d = new SpeechDetector();
    let r = feed(d, 0.02, 2000, 0); // a noisy kitchen
    expect(r.events).toEqual([]);
    expect(d.floor).toBeGreaterThan(0.015);
    r = feed(d, 0.2, 1500, r.t); // someone speaks
    expect(r.events).toEqual(["start"]);
    r = feed(d, 0.02, 1000, r.t); // and stops
    expect(r.events).toEqual(["end"]);
  });

  test("drops a short clatter", () => {
    const d = new SpeechDetector();
    let r = feed(d, 0.003, 1500, 0);
    r = feed(d, 0.3, 200, r.t);
    expect(r.events).toEqual(["start"]);
    r = feed(d, 0.003, 1000, r.t);
    expect(r.events).toEqual(["drop"]);
  });

  test("a fan already on is background", () => {
    const d = new SpeechDetector();
    const r = feed(d, 0.05, 20000, 0);
    expect(r.events).toEqual([]);
  });

  test("a fan switched on becomes background after one long phrase", () => {
    const d = new SpeechDetector();
    let r = feed(d, 0.003, 2000, 0);
    r = feed(d, 0.1, 30000, r.t);
    expect(r.events).toEqual(["start", "drop"]);
    // Speech over the fan is still heard.
    r = feed(d, 0.5, 1500, r.t);
    expect(r.events).toEqual(["start"]);
  });
});

describe("audio helpers", () => {
  test("rms", () => {
    expect(rms(new Float32Array([0.5, -0.5, 0.5, -0.5]))).toBeCloseTo(0.5);
    expect(rms(new Float32Array())).toBe(0);
  });

  test("downsample averages", () => {
    const out = downsample(new Float32Array([1, 1, 0, 0, 0, 1]), 48000, 16000);
    expect(out.length).toBe(2);
    expect(out[0]).toBeCloseTo(2 / 3);
    expect(out[1]).toBeCloseTo(1 / 3);
  });

  test("encodeWav writes a 16 kHz mono header and samples", () => {
    const wav = encodeWav(new Float32Array([0, 1, -1]), 16000);
    const v = new DataView(wav.buffer);
    expect(new TextDecoder().decode(wav.slice(0, 4))).toBe("RIFF");
    expect(new TextDecoder().decode(wav.slice(8, 12))).toBe("WAVE");
    expect(v.getUint32(24, true)).toBe(16000);
    expect(v.getUint16(22, true)).toBe(1);
    expect(v.getUint32(40, true)).toBe(6);
    expect(v.getInt16(46, true)).toBe(32767);
    expect(v.getInt16(48, true)).toBe(-32768);
  });
});
