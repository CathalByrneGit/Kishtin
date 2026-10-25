// A small typed client for the Kishtin API. Types come from the generated
// api.d.ts, which comes from the server's OpenAPI spec.

import type { components } from "./schema";
import { endSession, session } from "./session.svelte";

export type Schemas = components["schemas"];
export type HaccpRecord = Schemas["Record"];
export type Reading = Schemas["Reading"];
export type NewRecord = Schemas["NewRecord"];
export type Today = Schemas["TodayView"];
export type Check = Schemas["Check"];
export type Lists = Schemas["Lists"];
export type System = Schemas["System"];
export type Setup = Schemas["Setup"];
export type Dish = Schemas["Dish"];
export type Order = Schemas["Order"];
export type OrderItem = Schemas["Item"];
export type KitchenView = Schemas["KitchenView"];
export type Course = "starter" | "main" | "dessert";
export type VoiceReply = Schemas["VoiceReply"];
export type VoiceStatus = Schemas["VoiceStatus"];

/** ApiError is an error the server explained. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

/** offline is set when the server cannot be reached. */
export class OfflineError extends Error {
  constructor() {
    super("Cannot reach the Kishtin server. Check the Wi-Fi, then try again.");
  }
}

export function authHeaders(): Record<string, string> {
  const h: Record<string, string> = {};
  if (session.deviceKey) h.Authorization = `Bearer ${session.deviceKey}`;
  if (session.token) h["X-Kishtin-Session"] = session.token;
  return h;
}

type Query = Record<string, string | string[] | number | boolean | undefined>;

function url(path: string, query?: Query): string {
  const u = new URL(`/api${path}`, location.origin);
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v === undefined || v === "") continue;
    for (const item of Array.isArray(v) ? v : [v]) u.searchParams.append(k, String(item));
  }
  return u.pathname + u.search;
}

interface SendOpts {
  query?: Query;
  body?: unknown;
  raw?: Blob;
  /** rawType is the raw body's type; octet-stream when not set. */
  rawType?: string;
}

async function send(method: string, path: string, opts: SendOpts = {}): Promise<Response> {
  const headers = authHeaders();
  let body: BodyInit | undefined;
  if (opts.raw) {
    headers["Content-Type"] = opts.rawType || "application/octet-stream";
    body = opts.raw;
  } else if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }
  let res: Response;
  try {
    res = await fetch(url(path, opts.query), { method, headers, body });
  } catch {
    throw new OfflineError();
  }
  if (res.ok) return res;
  let detail = `The server said ${res.status}.`;
  try {
    const p = (await res.json()) as { detail?: string; errors?: { message?: string; location?: string }[] };
    if (p.detail) detail = p.detail;
    for (const e of p.errors ?? []) {
      if (e.message) detail += `; ${(e.location ?? "").replace(/^body\./, "")} ${e.message}`;
    }
  } catch {
    // Not a problem document.
  }
  if (res.status === 401 && session.token && /session|log in/i.test(detail)) endSession();
  throw new ApiError(detail.charAt(0).toUpperCase() + detail.slice(1), res.status);
}

async function json<T>(method: string, path: string, opts?: SendOpts): Promise<T> {
  const res = await send(method, path, opts);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export interface RecordFilter {
  from?: string;
  to?: string;
  type?: string[];
  outcome?: string[];
  limit?: number;
  newest?: boolean;
}

export interface ExportResult {
  blob: Blob;
  name: string;
  /** evidence the server sends with every export */
  records: number;
  chainHead: string;
  chainOK: boolean;
}

export const api = {
  staff: () => json<Schemas["StaffOutBody"]>("GET", "/auth/staff"),
  login: (body: { user_id: string; pin?: string; password?: string }) =>
    json<Schemas["LoginResult"]>("POST", "/auth/login", { body }),
  logout: () => json<void>("POST", "/auth/logout"),
  me: () => json<Schemas["MeOutBody"]>("GET", "/auth/me"),

  today: () => json<Today>("GET", "/haccp/today"),
  check: (body: NewRecord) => json<Check>("POST", "/haccp/check", { body }),
  log: (body: NewRecord) => json<HaccpRecord>("POST", "/haccp/records", { body }),
  records: (q: RecordFilter) => json<Schemas["RecordsOutBody"]>("GET", "/haccp/records", { query: { ...q } }),
  limits: () => json<Schemas["Limits"]>("GET", "/haccp/limits"),
  upload: (file: File) => json<Schemas["FileMeta"]>("POST", "/files", { query: { name: file.name }, raw: file }),

  exportFile: async (kind: "csv" | "pdf", q: RecordFilter): Promise<ExportResult> => {
    const res = await send("GET", kind === "csv" ? "/haccp/export" : "/haccp/export/pdf", { query: { ...q } });
    const cd = res.headers.get("Content-Disposition") ?? "";
    const name = /filename="?([^";]+)"?/.exec(cd)?.[1] ?? `haccp-records.${kind}`;
    return {
      blob: await res.blob(),
      name,
      records: Number(res.headers.get("X-Kishtin-Records") ?? "0"),
      chainHead: res.headers.get("X-Kishtin-Chain-Head") ?? "",
      chainOK: res.headers.get("X-Kishtin-Chain-OK") === "true",
    };
  },

  guidance: () => json<Schemas["VersionedGuidance"]>("GET", "/settings/guidance"),
  lists: () => json<Schemas["VersionedLists"]>("GET", "/settings/lists"),
  previewLists: (body: Schemas["SettingsChangeLists"]) =>
    json<Schemas["SettingsPreviewLists"]>("POST", "/settings/lists/preview", { body }),
  saveLists: (body: Schemas["SettingsChangeLists"]) => json<Schemas["VersionedLists"]>("PUT", "/settings/lists", { body }),
  system: () => json<Schemas["VersionedSystem"]>("GET", "/settings/system"),
  previewSystem: (body: Schemas["SettingsChangeSystem"]) =>
    json<Schemas["SettingsPreviewSystem"]>("POST", "/settings/system/preview", { body }),
  saveSystem: (body: Schemas["SettingsChangeSystem"]) => json<Schemas["VersionedSystem"]>("PUT", "/settings/system", { body }),
  rules: () => json<Schemas["VersionedRules"]>("GET", "/settings/rules"),
  previewRules: (body: Schemas["SettingsChangeRules"]) =>
    json<Schemas["SettingsPreviewRules"]>("POST", "/settings/rules/preview", { body }),
  saveRules: (body: Schemas["SettingsChangeRules"]) => json<Schemas["VersionedRules"]>("PUT", "/settings/rules", { body }),

  record: (id: string) => json<HaccpRecord>("GET", `/haccp/records/${encodeURIComponent(id)}`),
  verify: () => json<Schemas["VerifyReport"]>("GET", "/haccp/verify"),
  previewLimit: (body: Schemas["NewLimit"]) => json<Schemas["LimitPreview"]>("POST", "/haccp/limits/preview", { body }),
  createLimit: (body: Schemas["NewLimit"]) => json<Schemas["LimitVersion"]>("POST", "/haccp/limits", { body }),
  exportLimits: async (): Promise<{ blob: Blob; name: string }> => {
    const res = await send("GET", "/haccp/limits/export");
    return { blob: await res.blob(), name: "kishtin-limits.csv" };
  },
  /** file fetches an attachment as an object URL; revoke it when done. */
  file: async (sha: string): Promise<{ url: string; type: string }> => {
    const res = await send("GET", `/files/${encodeURIComponent(sha)}`);
    const blob = await res.blob();
    return { url: URL.createObjectURL(blob), type: blob.type };
  },

  users: () => json<Schemas["UsersOutBody"]>("GET", "/users"),
  createUser: (body: Schemas["NewUser"]) => json<Schemas["User"]>("POST", "/users", { body }),
  updateUser: (id: string, body: Schemas["UserChange"]) =>
    json<Schemas["User"]>("PATCH", `/users/${encodeURIComponent(id)}`, { body }),
  keys: () => json<Schemas["KeysOutBody"]>("GET", "/keys"),
  createKey: (body: Schemas["NewKeyInBody"]) => json<Schemas["NewKeyResult"]>("POST", "/keys", { body }),
  revokeKey: (id: string) => json<void>("DELETE", `/keys/${encodeURIComponent(id)}`),
  backups: () => json<Schemas["BackupsOutBody"]>("GET", "/backups"),
  backupNow: () => json<Schemas["Backup"]>("POST", "/backups"),

  menu: () => json<Setup>("GET", "/menu"),
  createStation: (body: Schemas["StationInput"]) => json<Schemas["Station"]>("POST", "/menu/stations", { body }),
  updateStation: (id: string, body: Schemas["StationInput"]) =>
    json<Schemas["Station"]>("PUT", `/menu/stations/${encodeURIComponent(id)}`, { body }),
  createEquipment: (body: Schemas["EquipmentInput"]) => json<Schemas["Equipment"]>("POST", "/menu/equipment", { body }),
  updateEquipment: (id: string, body: Schemas["EquipmentInput"]) =>
    json<Schemas["Equipment"]>("PUT", `/menu/equipment/${encodeURIComponent(id)}`, { body }),
  createDish: (body: Schemas["DishInput"]) => json<Dish>("POST", "/menu/dishes", { body }),
  updateDish: (id: string, body: Schemas["DishInput"]) => json<Dish>("PUT", `/menu/dishes/${encodeURIComponent(id)}`, { body }),
  setAvailability: (id: string, available: boolean) =>
    json<Dish>("PUT", `/menu/dishes/${encodeURIComponent(id)}/availability`, { body: { available } }),

  orders: (date?: string) => json<Schemas["OrdersBody"]>("GET", "/orders", { query: { date } }),
  createOrder: (body: Schemas["NewOrder"]) => json<Order>("POST", "/orders", { body }),
  order: (id: string) => json<Order>("GET", `/orders/${encodeURIComponent(id)}`),
  orderEvents: (id: string) => json<Schemas["EventsBody"]>("GET", `/orders/${encodeURIComponent(id)}/events`),
  addItems: (id: string, items: Schemas["NewItem"][]) =>
    json<Order>("POST", `/orders/${encodeURIComponent(id)}/items`, { body: { items } }),
  fire: (id: string, course: Course) => json<Order>("POST", `/orders/${encodeURIComponent(id)}/courses/${course}/fire`),
  hold: (id: string, course: Course, reason?: string) =>
    json<Order>("POST", `/orders/${encodeURIComponent(id)}/courses/${course}/hold`, { body: { reason } }),
  replaceItem: (id: string, body: Schemas["ItemReplace"]) =>
    json<Order>("POST", `/kitchen/items/${encodeURIComponent(id)}/replace`, { body }),
  ready: (id: string, course: Course) => json<Order>("POST", `/orders/${encodeURIComponent(id)}/courses/${course}/ready`),
  serve: (id: string, course: Course) => json<Order>("POST", `/orders/${encodeURIComponent(id)}/courses/${course}/serve`),
  setTarget: (id: string, course: Course, target_at: string, reason: string) =>
    json<Order>("PUT", `/orders/${encodeURIComponent(id)}/courses/${course}/target`, { body: { target_at, reason } }),
  setUrgent: (id: string, course: Course, urgent: boolean) =>
    json<Order>("PUT", `/orders/${encodeURIComponent(id)}/courses/${course}/urgent`, { body: { urgent } }),
  closeOrder: (id: string) => json<Order>("POST", `/orders/${encodeURIComponent(id)}/close`),

  kitchen: () => json<KitchenView>("GET", "/kitchen"),
  moveItems: (body: Schemas["ItemMove"]) => json<Schemas["OrdersBody"]>("POST", "/kitchen/items", { body }),

  voiceStatus: () => json<VoiceStatus>("GET", "/voice"),
  voiceCommand: (text: string) => json<VoiceReply>("POST", "/voice/command", { body: { text } }),
  voiceAudio: (audio: Blob) => json<VoiceReply>("POST", "/voice/audio", { raw: audio, rawType: audio.type || "audio/webm" }),
  /** voiceSpeak returns the reply as WAV audio from the server's local text-to-speech. */
  voiceSpeak: async (text: string): Promise<Blob> => (await send("POST", "/voice/speak", { body: { text } })).blob(),
  voiceLog: (limit = 100) => json<Schemas["VoiceLogBody"]>("GET", "/voice/log", { query: { limit } }),

  square: () => json<Schemas["SquareStatus"]>("GET", "/square"),
  previewSquare: (body: Schemas["SettingsChangePOSSettings"]) =>
    json<Schemas["SettingsPreviewPOSSettings"]>("POST", "/settings/square/preview", { body }),
  saveSquare: (body: Schemas["SettingsChangePOSSettings"]) => json<Schemas["VersionedPOSSettings"]>("PUT", "/settings/square", { body }),
  squareCatalog: () => json<Schemas["CatalogBody"]>("GET", "/square/catalog"),
  saveSquareMappings: (mappings: Schemas["MappingInput"][]) =>
    json<Schemas["MappingsBody"]>("PUT", "/square/mappings", { body: { mappings } }),
  squareSync: () => json<Schemas["SyncResult"]>("POST", "/square/sync"),

  scheduler: () => json<Schemas["VersionedSettings"]>("GET", "/settings/scheduler"),
  previewScheduler: (body: Schemas["SettingsChangeSettings"]) =>
    json<Schemas["SettingsPreviewSettings"]>("POST", "/settings/scheduler/preview", { body }),
  saveScheduler: (body: Schemas["SettingsChangeSettings"]) =>
    json<Schemas["VersionedSettings"]>("PUT", "/settings/scheduler", { body }),
};

/** download hands a file to the browser to save. */
export function download(blob: Blob, name: string): void {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 10_000);
}

/** message turns any error into words for staff. */
export function message(err: unknown): string {
  if (err instanceof ApiError || err instanceof OfflineError) return err.message;
  return "Something went wrong. Try again.";
}
