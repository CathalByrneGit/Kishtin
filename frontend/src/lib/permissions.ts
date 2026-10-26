// The permissions screen's draft: who may take each action, compared with
// Kishtin's default so only changes are saved.

import type { Schemas } from "./api";

type ActionInfo = Schemas["ActionInfo"];
type Grant = Schemas["Grant"];

/** DELEGABLE_ROLES are the roles a manager can give actions to, in order. */
export const DELEGABLE_ROLES = ["head_chef", "line_cook", "front_of_house"];

export interface Draft {
  roles: string[];
  users: string[];
  pin: boolean;
}

function canonical(d: Draft): Draft {
  return { roles: DELEGABLE_ROLES.filter((r) => d.roles.includes(r)), users: [...new Set(d.users)].sort(), pin: d.pin };
}

/** defaultDraft is Kishtin's default for an action. */
export function defaultDraft(a: ActionInfo): Draft {
  return canonical({ roles: a.roles, users: [], pin: a.pin });
}

/** isDefault reports whether a draft gives an action exactly its default. */
export function isDefault(a: ActionInfo, d: Draft): boolean {
  return JSON.stringify(canonical(d)) === JSON.stringify(defaultDraft(a));
}

/** draftFrom builds a draft for every action that can be changed. */
export function draftFrom(actions: ActionInfo[], p: Schemas["Permissions"]): Record<string, Draft> {
  const out: Record<string, Draft> = {};
  for (const a of actions) {
    if (a.fixed) continue;
    const g = p.grants.find((x) => x.action === a.action);
    out[a.action] = g ? canonical({ roles: g.roles, users: g.users ?? [], pin: g.pin ?? false }) : defaultDraft(a);
  }
  return out;
}

/** grantsFrom lists the actions whose draft differs from the default, in
 * screen order. */
export function grantsFrom(actions: ActionInfo[], draft: Record<string, Draft>): Grant[] {
  const out: Grant[] = [];
  for (const a of actions) {
    const d = draft[a.action];
    if (a.fixed || !d || isDefault(a, d)) continue;
    const c = canonical(d);
    out.push({ action: a.action, roles: c.roles, users: c.users, pin: c.pin });
  }
  return out;
}
