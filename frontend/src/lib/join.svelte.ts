// A tablet that opens a pairing link (…/#join=CODE) from a manager's QR code
// pairs itself: it swaps the one-time code for a device key.

import { api, message } from "./api";
import { pairDevice } from "./session.svelte";

export const joining = $state({ busy: false, problem: "", name: "" });

/** takeJoinFromURL reads a #join=… code, removes it from the address, and
 * returns it. */
export function takeJoinFromURL(): string | null {
  const m = /^#join=(.+)$/.exec(location.hash);
  if (!m?.[1]) return null;
  history.replaceState(null, "", location.pathname + location.search);
  return decodeURIComponent(m[1]);
}

/** join pairs this device with a one-time code. */
export async function join(code: string): Promise<void> {
  joining.busy = true;
  joining.problem = "";
  try {
    const res = await api.pairDevice(code);
    joining.name = res.name;
    pairDevice(res.secret);
  } catch (err) {
    joining.problem = message(err);
  } finally {
    joining.busy = false;
  }
}
