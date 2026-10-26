// What the logged-in staff member may do, from the server, so screens only
// offer what works. The server checks every action again.

import { api } from "./api";
import { onChange } from "./live.svelte";
import { session } from "./session.svelte";

export type Allowed = "yes" | "password" | "no";

export const permissions = $state({ can: {} as Record<string, string> });

async function refresh(): Promise<void> {
  if (!session.deviceKey || !session.token) {
    permissions.can = {};
    return;
  }
  try {
    permissions.can = (await api.me()).can;
  } catch {
    // Keeps the last known permissions; the server still checks each action.
  }
}

$effect.root(() => {
  $effect(() => {
    void session.token;
    void refresh();
  });
});

onChange((kind) => {
  if (kind === "settings" || kind === "ready") void refresh();
});

/** allowed is whether the logged-in staff member may take an action now,
 * after logging in with a password, or not at all. */
export function allowed(action: string): Allowed {
  const v = permissions.can[action];
  return v === "yes" || v === "password" ? v : "no";
}

/** can reports whether the logged-in staff member may take an action now. */
export function can(action: string): boolean {
  return allowed(action) === "yes";
}

/** canWithPassword reports whether they may, now or after a password login. */
export function canWithPassword(action: string): boolean {
  return allowed(action) !== "no";
}
