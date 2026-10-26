// Voice control on this device: push-to-talk recording, sending commands,
// and speaking replies. Recognition and speech run on the Kishtin server's
// local sidecars; audio never goes anywhere else.

import { api, message, type VoiceReply, type VoiceStatus } from "./api";
import { load, save } from "./storage";
import { SpeechDetector, downsample, encodeWav, rms } from "./wake";

export type VoicePhase = "idle" | "recording" | "sending";

const SPEAK_KEY = "kishtin-speak-replies";

export const voice = $state({
  phase: "idle" as VoicePhase,
  status: null as VoiceStatus | null,
  /** the last reply, shown until the next command */
  reply: null as VoiceReply | null,
  /** a problem with this device or the request, not a command refusal */
  error: "",
  /** speak replies aloud on this device */
  speak: load(SPEAK_KEY) !== "off",
  /** a reply is being spoken */
  speaking: false,
});

/**
 * Hands-free listening. Off until someone turns it on, on this device,
 * for this page load: a reload or switching away turns it off.
 */
export const handsFree = $state({
  on: false,
  /** a phrase is being heard */
  hearing: false,
  /** a phrase is with the server */
  sending: false,
  error: "",
});

/** micProblem says why push-to-talk cannot work in this browser, if so. */
export function micProblem(): string {
  if (typeof window === "undefined") return "";
  if (!window.isSecureContext) {
    return "The microphone needs a secure connection (HTTPS). Typed commands still work. A manager can set up TLS in kishtin.toml.";
  }
  if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") {
    return "This browser cannot record audio. Typed commands still work.";
  }
  return "";
}

export function setSpeak(on: boolean): void {
  voice.speak = on;
  save(SPEAK_KEY, on ? null : "off");
}

export async function loadVoiceStatus(): Promise<void> {
  try {
    voice.status = await api.voiceStatus();
  } catch {
    // Keeps the last known status.
  }
}

let stream: MediaStream | null = null;
let recorder: MediaRecorder | null = null;
let chunks: Blob[] = [];
let startedAt = 0;
let limitTimer: ReturnType<typeof setTimeout> | undefined;

/** The longest recording; commands are short. */
const MAX_MS = 20_000;
/** A press shorter than this is a tap: recording continues until the next tap. */
export const TAP_MS = 400;

function pickType(): string {
  for (const t of ["audio/webm;codecs=opus", "audio/ogg;codecs=opus", "audio/mp4", "audio/webm"]) {
    if (MediaRecorder.isTypeSupported?.(t)) return t;
  }
  return "";
}

/** startRecording asks for the microphone (once) and starts listening. */
export async function startRecording(): Promise<void> {
  if (voice.phase !== "idle") return;
  const problem = micProblem();
  if (problem) {
    voice.error = problem;
    return;
  }
  voice.error = "";
  try {
    if (!stream || stream.getTracks().every((t) => t.readyState === "ended")) {
      stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
    }
  } catch {
    voice.error = "The microphone is blocked. Allow it for this site in the browser settings, or type the command.";
    return;
  }
  const type = pickType();
  recorder = type ? new MediaRecorder(stream, { mimeType: type }) : new MediaRecorder(stream);
  chunks = [];
  recorder.ondataavailable = (e) => {
    if (e.data.size > 0) chunks.push(e.data);
  };
  recorder.start();
  startedAt = performance.now();
  voice.phase = "recording";
  limitTimer = setTimeout(() => void stopRecording(), MAX_MS);
}

/** heldFor is how long the current recording has run, in ms. */
export function heldFor(): number {
  return voice.phase === "recording" ? performance.now() - startedAt : 0;
}

/** stopRecording stops listening and sends what was said. */
export async function stopRecording(): Promise<void> {
  clearTimeout(limitTimer);
  const r = recorder;
  if (!r || voice.phase !== "recording") return;
  recorder = null;
  const stopped = new Promise<void>((resolve) => (r.onstop = () => resolve()));
  r.stop();
  await stopped;
  const blob = new Blob(chunks, { type: r.mimeType || "audio/webm" });
  chunks = [];
  if (blob.size < 1000) {
    voice.phase = "idle";
    voice.error = "Nothing was recorded. Hold the button while you speak.";
    return;
  }
  voice.phase = "sending";
  await run(() => api.voiceAudio(blob));
}

/** cancelRecording stops listening without sending. */
export function cancelRecording(): void {
  clearTimeout(limitTimer);
  if (recorder && recorder.state !== "inactive") recorder.stop();
  recorder = null;
  chunks = [];
  voice.phase = "idle";
}

/** releaseMicrophone stops the microphone, such as when the page is hidden. */
export function releaseMicrophone(): void {
  cancelRecording();
  stream?.getTracks().forEach((t) => t.stop());
  stream = null;
}

/** sendText runs a typed command, or answers a question by tapping. */
export async function sendText(text: string): Promise<void> {
  if (!text.trim() || voice.phase === "sending") return;
  voice.phase = "sending";
  await run(() => api.voiceCommand(text.trim()));
}

async function run(fn: () => Promise<VoiceReply>): Promise<void> {
  voice.error = "";
  try {
    voice.reply = await fn();
    void speakReply(voice.reply.say);
  } catch (err) {
    voice.reply = null;
    voice.error = message(err);
  } finally {
    voice.phase = "idle";
    void loadVoiceStatus();
  }
}

let playing: HTMLAudioElement | null = null;

/**
 * speakReply says a reply with the server's local text-to-speech. Without
 * it, it uses a voice built into this device, and only one that runs on
 * the device itself (some browser voices send text to the cloud).
 */
async function speakReply(text: string): Promise<void> {
  if (!voice.speak || !text) return;
  if (voice.status?.speak) {
    try {
      const blob = await api.voiceSpeak(text);
      playing?.pause();
      const url = URL.createObjectURL(blob);
      playing = new Audio(url);
      voice.speaking = true;
      playing.onended = playing.onerror = () => {
        URL.revokeObjectURL(url);
        voice.speaking = false;
      };
      await playing.play();
      return;
    } catch {
      voice.speaking = false;
      // Fall back to an on-device voice below.
    }
  }
  if (typeof speechSynthesis === "undefined") return;
  const local = speechSynthesis.getVoices().find((v) => v.localService && v.lang.startsWith("en"));
  if (!local) return;
  const u = new SpeechSynthesisUtterance(text);
  u.voice = local;
  voice.speaking = true;
  u.onend = u.onerror = () => (voice.speaking = false);
  speechSynthesis.cancel();
  speechSynthesis.speak(u);
}

// --- hands-free ---

let hf: { stream: MediaStream; ctx: AudioContext; proc: ScriptProcessorNode } | null = null;

/** startHandsFree starts listening for the wake word. */
export async function startHandsFree(): Promise<void> {
  if (hf) return;
  const problem = micProblem();
  if (problem) {
    handsFree.error = problem;
    return;
  }
  handsFree.error = "";
  let stream: MediaStream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
  } catch {
    handsFree.error = "The microphone is blocked. Allow it for this site in the browser settings.";
    return;
  }
  const ctx = new AudioContext();
  const source = ctx.createMediaStreamSource(stream);
  // ScriptProcessorNode is old but works in every kitchen browser, and
  // needs no extra script file.
  const proc = ctx.createScriptProcessor(2048, 1, 1);
  const mute = ctx.createGain();
  mute.gain.value = 0;
  source.connect(proc);
  proc.connect(mute);
  mute.connect(ctx.destination);

  const detector = new SpeechDetector();
  const prerollBlocks = Math.ceil((0.4 * ctx.sampleRate) / 2048);
  const preroll: Float32Array[] = [];
  let phrase: Float32Array[] | null = null;

  proc.onaudioprocess = (e) => {
    const block = new Float32Array(e.inputBuffer.getChannelData(0));
    // Not while push-to-talk is in use, a phrase is with the server, or a
    // reply is playing (it would hear itself).
    if (voice.phase !== "idle" || handsFree.sending || voice.speaking) {
      phrase = null;
      handsFree.hearing = false;
      preroll.length = 0;
      return;
    }
    const ev = detector.push(rms(block), ctx.currentTime * 1000);
    if (ev === "start") {
      phrase = [...preroll, block];
      handsFree.hearing = true;
    } else if (phrase) {
      phrase.push(block);
    }
    if ((ev === "end" || ev === "drop") && phrase) {
      const blocks = phrase;
      phrase = null;
      handsFree.hearing = false;
      if (ev === "end") void sendPhrase(blocks, ctx.sampleRate);
    }
    preroll.push(block);
    if (preroll.length > prerollBlocks) preroll.shift();
  };
  hf = { stream, ctx, proc };
  handsFree.on = true;
}

/** stopHandsFree stops listening and releases the microphone. */
export function stopHandsFree(): void {
  handsFree.on = false;
  handsFree.hearing = false;
  if (!hf) return;
  hf.proc.onaudioprocess = null;
  hf.stream.getTracks().forEach((t) => t.stop());
  void hf.ctx.close();
  hf = null;
}

async function sendPhrase(blocks: Float32Array[], rate: number): Promise<void> {
  const total = blocks.reduce((n, b) => n + b.length, 0);
  const all = new Float32Array(total);
  let at = 0;
  for (const b of blocks) {
    all.set(b, at);
    at += b.length;
  }
  const wav = encodeWav(downsample(all, rate, 16000), Math.min(rate, 16000));
  handsFree.sending = true;
  try {
    const reply = await api.voiceAudio(new Blob([wav], { type: "audio/wav" }), true);
    // Kitchen talk without the wake word: nothing to show.
    if (reply.outcome === "ignored") return;
    voice.reply = reply;
    voice.error = "";
    void speakReply(reply.say);
    void loadVoiceStatus();
  } catch (err) {
    handsFree.error = message(err);
  } finally {
    handsFree.sending = false;
  }
}
