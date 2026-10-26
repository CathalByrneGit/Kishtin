import { expect, test } from "bun:test";
import type { Schemas } from "./api";
import { draftFrom, grantsFrom, isDefault } from "./permissions";

const actions: Schemas["ActionInfo"][] = [
  { action: "haccp.log", group: "HACCP", label: "Log checks", roles: ["head_chef", "line_cook", "front_of_house", "manager"], pin: true },
  { action: "haccp.sign_off", group: "HACCP", label: "Sign off records", roles: ["manager"], pin: false },
  { action: "users.manage", group: "Setup", label: "Add and change staff", roles: ["manager"], pin: false, fixed: true },
];

test("a draft starts at the saved grants and the defaults", () => {
  const d = draftFrom(actions, { grants: [{ action: "haccp.sign_off", roles: ["head_chef"], pin: true }] });
  expect(d["haccp.sign_off"]).toEqual({ roles: ["head_chef"], users: [], pin: true });
  expect(d["haccp.log"]).toEqual({ roles: ["head_chef", "line_cook", "front_of_house"], users: [], pin: true });
  expect(d["users.manage"]).toBeUndefined();
});

test("only changes are saved, in screen order", () => {
  const d = draftFrom(actions, { grants: [] });
  expect(grantsFrom(actions, d)).toEqual([]);
  d["haccp.sign_off"] = { roles: [], users: ["u-2", "u-1", "u-2"], pin: false };
  d["haccp.log"] = { roles: ["line_cook", "head_chef"], users: [], pin: true };
  expect(grantsFrom(actions, d)).toEqual([
    { action: "haccp.log", roles: ["head_chef", "line_cook"], users: [], pin: true },
    { action: "haccp.sign_off", roles: [], users: ["u-1", "u-2"], pin: false },
  ]);
  expect(isDefault(actions[0]!, { roles: ["front_of_house", "line_cook", "head_chef"], users: [], pin: true })).toBe(true);
});
