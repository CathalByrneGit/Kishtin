// The language of the staff screens. Each person can have their own on a
// shared device, so two cooks on one tablet each read their own; before
// anyone logs in, the device's language is used.
//
// English is the text in the code. Other languages are in ./i18n/*.ts.
// Only the staff screens are translated (see i18n/messages.ts). What the
// server says (why a reading failed, error messages), the management
// screens and voice commands stay in English.

import es from "./i18n/es";
import type { Dictionary, Msg } from "./i18n/messages";
import pl from "./i18n/pl";
import pt from "./i18n/pt";
import { session } from "./session.svelte";
import { load, save } from "./storage";

export type Lang = "en" | "pl" | "pt" | "es";

export const LANGUAGES: { value: Lang; label: string }[] = [
  { value: "en", label: "English" },
  { value: "pl", label: "Polski" },
  { value: "pt", label: "Português" },
  { value: "es", label: "Español" },
];

const DICTIONARIES: Record<Exclude<Lang, "en">, Dictionary> = { pl, pt, es };
const DEVICE_KEY = "kishtin-lang";
const userKey = (id: string) => `kishtin-lang-${id}`;

const isLang = (v: string | null): v is Lang => LANGUAGES.some((l) => l.value === v);
const read = (key: string): Lang | null => {
  const v = load(key);
  return isLang(v) ? v : null;
};

const state = $state({
  device: read(DEVICE_KEY) ?? ("en" as Lang),
  /** choices made since the page loaded, by user ID */
  users: {} as Record<string, Lang>,
});

/** lang is the language in use now. */
export function lang(): Lang {
  const id = session.user?.id;
  if (!id) return state.device;
  // Read, never written, here: this runs while a screen is drawn. A choice
  // made in this page load is in state; an earlier one is in storage.
  return state.users[id] ?? read(userKey(id)) ?? state.device;
}

/**
 * setLang sets the logged-in person's language on this device, or the
 * device's own when nobody is logged in.
 */
export function setLang(l: Lang): void {
  const id = session.user?.id;
  if (id) {
    state.users[id] = l;
    save(userKey(id), l);
  } else {
    state.device = l;
    save(DEVICE_KEY, l);
  }
  if (typeof document !== "undefined") document.documentElement.lang = l === "en" ? "en-IE" : l;
}

/** fill puts values into a message: "{done} of {total}". */
export function fill(text: string, params?: Record<string, string | number>): string {
  if (!params) return text;
  return text.replace(/\{(\w+)\}/g, (whole, name: string) => (name in params ? String(params[name]) : whole));
}

/** translate looks a message up in a language, falling back to English. */
export function translate(l: Lang, msg: Msg, params?: Record<string, string | number>): string {
  return fill(l === "en" ? msg : (DICTIONARIES[l][msg] ?? msg), params);
}

/** t is a message in the language in use. */
export function t(msg: Msg, params?: Record<string, string | number>): string {
  return translate(lang(), msg, params);
}

/** tAny translates text that may not be in the list, such as a label from data. */
export function tAny(text: string): string {
  const l = lang();
  return l === "en" ? text : ((DICTIONARIES[l] as Record<string, string>)[text] ?? text);
}
