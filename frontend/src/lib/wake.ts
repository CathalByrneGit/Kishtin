// Hands-free listening on this device: a simple speech detector that
// works on sound level alone, and a WAV encoder for the phrases it finds.
// Nothing here understands words; each phrase goes to the Kishtin
// server's local speech-to-text, which checks for the wake word and
// ignores the rest.

export interface DetectorOptions {
  /** speech starts when the level is this many times the background noise */
  startRatio: number;
  /** and ends when it falls below this many times the background noise */
  endRatio: number;
  /** the quietest level that counts as speech at all (RMS, 0 to 1) */
  minLevel: number;
  /** how long the level must stay up before it counts as speech */
  startMs: number;
  /** how long it must stay down before the phrase ends */
  endMs: number;
  /** phrases shorter than this are dropped (a clatter, a cough) */
  minMs: number;
  /** a phrase is cut off at this length */
  maxMs: number;
  /** at first, only learn the room's background noise for this long */
  warmupMs: number;
}

export const DEFAULT_DETECTOR: DetectorOptions = {
  startRatio: 3,
  endRatio: 2,
  minLevel: 0.01,
  startMs: 120,
  endMs: 700,
  minMs: 500,
  maxMs: 8000,
  warmupMs: 1000,
};

export type DetectorEvent = "start" | "end" | "drop" | null;

/**
 * SpeechDetector finds phrases in a stream of sound levels. It follows the
 * background noise (extractor fans, fryers) so it only reacts to sound
 * clearly louder than the room.
 */
export class SpeechDetector {
  floor = 0.005;
  speaking = false;
  private aboveSince: number | null = null;
  private belowSince: number | null = null;
  private startedAt = 0;
  private firstAt: number | null = null;
  private readonly o: DetectorOptions;

  constructor(options: Partial<DetectorOptions> = {}) {
    this.o = { ...DEFAULT_DETECTOR, ...options };
  }

  /** push takes the level at time t (ms) and says if a phrase started or ended. */
  push(level: number, t: number): DetectorEvent {
    const o = this.o;
    this.firstAt ??= t;
    if (t - this.firstAt < o.warmupMs) {
      // Learn the room quickly before listening for speech.
      this.floor = this.floor * 0.8 + level * 0.2;
      return null;
    }
    // A sound that stays on (a fan switched on) slowly becomes background.
    if (level > this.floor) this.floor += (level - this.floor) * 0.002;
    if (!this.speaking) {
      const loud = level > Math.max(this.floor * o.startRatio, o.minLevel);
      if (!loud) {
        this.aboveSince = null;
        // Learn the room's background noise while nobody is speaking.
        this.floor = this.floor * 0.95 + level * 0.05;
        return null;
      }
      this.aboveSince ??= t;
      if (t - this.aboveSince >= o.startMs) {
        this.speaking = true;
        this.startedAt = this.aboveSince;
        this.belowSince = null;
        return "start";
      }
      return null;
    }
    const length = t - this.startedAt;
    if (length >= o.maxMs) {
      // Eight seconds without a pause is not a command: it is the room.
      this.floor = Math.max(this.floor, level * 0.8);
      return this.stop("drop");
    }
    if (level < this.floor * o.endRatio || level < o.minLevel) {
      this.belowSince ??= t;
      // Only the part with sound counts, not the pause that ended it.
      if (t - this.belowSince >= o.endMs) return this.stop(this.belowSince - this.startedAt >= o.minMs ? "end" : "drop");
    } else {
      this.belowSince = null;
    }
    return null;
  }

  private stop(e: DetectorEvent): DetectorEvent {
    this.speaking = false;
    this.aboveSince = null;
    this.belowSince = null;
    return e;
  }
}

/** rms is the loudness of a block of samples, 0 to 1. */
export function rms(samples: Float32Array): number {
  let sum = 0;
  for (const s of samples) sum += s * s;
  return samples.length ? Math.sqrt(sum / samples.length) : 0;
}

/** downsample averages samples down to a lower rate, such as 16 kHz for speech. */
export function downsample(samples: Float32Array, from: number, to: number): Float32Array {
  if (to >= from) return samples;
  const ratio = from / to;
  const out = new Float32Array(Math.floor(samples.length / ratio));
  for (let i = 0; i < out.length; i++) {
    const a = Math.floor(i * ratio);
    const b = Math.min(samples.length, Math.floor((i + 1) * ratio));
    let sum = 0;
    for (let j = a; j < b; j++) sum += samples[j] ?? 0;
    out[i] = sum / Math.max(1, b - a);
  }
  return out;
}

/** encodeWav makes a 16-bit mono WAV file, which speech-to-text reads directly. */
export function encodeWav(samples: Float32Array, rate: number): Uint8Array<ArrayBuffer> {
  const buf = new ArrayBuffer(44 + samples.length * 2);
  const v = new DataView(buf);
  const text = (at: number, s: string) => {
    for (let i = 0; i < s.length; i++) v.setUint8(at + i, s.charCodeAt(i));
  };
  text(0, "RIFF");
  v.setUint32(4, 36 + samples.length * 2, true);
  text(8, "WAVE");
  text(12, "fmt ");
  v.setUint32(16, 16, true);
  v.setUint16(20, 1, true);
  v.setUint16(22, 1, true);
  v.setUint32(24, rate, true);
  v.setUint32(28, rate * 2, true);
  v.setUint16(32, 2, true);
  v.setUint16(34, 16, true);
  text(36, "data");
  v.setUint32(40, samples.length * 2, true);
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i] ?? 0));
    v.setInt16(44 + i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true);
  }
  return new Uint8Array(buf);
}
