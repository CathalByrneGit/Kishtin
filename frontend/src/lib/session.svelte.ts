// Who is using this device: its device key, and the staff member logged in.

import type { Schemas } from "./api";
import { load, save } from "./storage";

const DEVICE_KEY = "kishtin-device-key";
const SESSION = "kishtin-session";

interface Stored {
  token: string;
  user: Schemas["User"];
  method: string;
  expiresAt: string;
}

function readSession(): Stored | null {
  const raw = load(SESSION);
  if (!raw) return null;
  try {
    const s = JSON.parse(raw) as Stored;
    return new Date(s.expiresAt) > new Date() ? s : null;
  } catch {
    return null;
  }
}

const initial = readSession();

export const session = $state({
  deviceKey: load(DEVICE_KEY),
  token: initial?.token ?? null,
  user: initial?.user ?? null,
  method: initial?.method ?? null,
  expiresAt: initial?.expiresAt ?? null,
});

/** pairDevice stores the device key this tablet or phone uses. */
export function pairDevice(key: string): void {
  session.deviceKey = key.trim();
  save(DEVICE_KEY, session.deviceKey);
}

/** unpairDevice forgets the device key and any login. */
export function unpairDevice(): void {
  session.deviceKey = null;
  save(DEVICE_KEY, null);
  endSession();
}

/** startSession records a successful login. */
export function startSession(login: Schemas["LoginResult"]): void {
  session.token = login.token;
  session.user = login.user;
  session.method = login.session.method;
  session.expiresAt = login.session.expires_at;
  save(
    SESSION,
    JSON.stringify({ token: login.token, user: login.user, method: login.session.method, expiresAt: login.session.expires_at }),
  );
}

/** endSession forgets the logged-in staff member. */
export function endSession(): void {
  session.token = null;
  session.user = null;
  session.method = null;
  session.expiresAt = null;
  save(SESSION, null);
}

/** isManagerPassword reports whether the session allows manager changes. */
export function isManagerPassword(): boolean {
  return session.user?.role === "manager" && session.method === "password";
}

/** takePairingFromURL reads a device key from a #pair=… link, then removes it. */
export function takePairingFromURL(): void {
  const m = /^#pair=(.+)$/.exec(location.hash);
  if (!m?.[1]) return;
  pairDevice(decodeURIComponent(m[1]));
  history.replaceState(null, "", location.pathname + location.search);
}
