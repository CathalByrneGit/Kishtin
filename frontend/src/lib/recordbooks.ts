// Draft record book settings on the setup screen: the bundle's booklets
// with the restaurant's changes, worked out the same way as the server
// (internal/books Resolve). Pure functions, so they are tested without a
// browser. The server checks everything again before saving.

import type { Schemas } from "./api";

export type Settings = Schemas["RecordBookSettings"];
export type BookSetting = Schemas["BookSetting"];
export type Template = Schemas["Template"];
export type Column = Schemas["Column"];
export type Bundle = Schemas["Bundle"];
export type Builder = Schemas["Builder"];

export const CUSTOM_PREFIX = "custom-";

/** DraftBooklet is one booklet as the draft settings would print it. */
export interface DraftBooklet {
  base: Template;
  template: Template;
  setting: BookSetting | undefined;
  custom: boolean;
  off: boolean;
  changed: string[];
}

/** applySetting makes a booklet's changes to its template, as the server does. */
export function applySetting(t: Template, b: BookSetting | undefined): Template {
  if (!b) return t;
  return {
    ...t,
    title: b.title || t.title,
    columns: b.columns?.length ? b.columns : t.columns,
    notes: b.notes?.length ? [...(t.notes ?? []), ...b.notes] : t.notes,
    units_per_page: b.units_per_page || t.units_per_page,
    manager_check: b.manager_check ?? t.manager_check,
  };
}

/** changedParts names what a booklet's changes touch. */
export function changedParts(b: BookSetting | undefined): string[] {
  if (!b) return [];
  const out: string[] = [];
  if (b.title) out.push("title");
  if (b.columns?.length) out.push("columns");
  if (b.notes?.length) out.push("notes");
  if (b.units_per_page) out.push("sheet layout");
  if (b.manager_check !== undefined && b.manager_check !== null) out.push("manager check");
  return out;
}

/** draftBooklets lists the bundle's booklets, then the added ones, with the draft's changes. */
export function draftBooklets(bundle: Bundle | undefined, s: Settings): DraftBooklet[] {
  const books = s.books ?? [];
  const find = (id: string) => books.find((b) => b.template === id);
  const one = (t: Template, custom: boolean): DraftBooklet => {
    const setting = find(t.id);
    return { base: t, template: applySetting(t, setting), setting, custom, off: Boolean(setting?.off), changed: changedParts(setting) };
  };
  return [...(bundle?.templates ?? []).map((t) => one(t, false)), ...(s.custom ?? []).map((t) => one(t, true))];
}

/** isEmpty reports whether a booklet setting changes nothing, so it can be dropped. */
export function isEmpty(b: BookSetting): boolean {
  return !b.off && !b.title && !b.columns?.length && !b.notes?.length && !b.units_per_page && (b.manager_check === undefined || b.manager_check === null) && !b.copy_file;
}

/** withSetting returns the settings with one booklet's changes replaced (and dropped when empty). */
export function withSetting(s: Settings, b: BookSetting): Settings {
  const books = (s.books ?? []).filter((x) => x.template !== b.template);
  if (!isEmpty(b)) books.push(b);
  return { ...s, books };
}

/** switchBundle moves to another bundle, keeping only changes to added booklets. */
export function switchBundle(s: Settings, bundle: string): Settings {
  const custom = new Set((s.custom ?? []).map((t) => t.id));
  return { ...s, bundle, books: (s.books ?? []).filter((b) => custom.has(b.template)) };
}

/** copyStatus says how a restaurant's uploaded copy compares with the edition a booklet was made from. */
export function copyStatus(t: Template, file: string | undefined): { tone: "success" | "warning" | "neutral"; text: string } | null {
  if (!file) return null;
  if (!t.source) return { tone: "neutral", text: "Your copy is uploaded for reference." };
  if (t.source.sha256 === file) return { tone: "success", text: `Your copy is the ${t.source.published} edition this booklet was made from.` };
  return {
    tone: "warning",
    text: `Your copy is not the ${t.source.published} edition this booklet was made from. Check its columns match your copy, and change them here if they do not.`,
  };
}

/** slug makes an ID part from a code: "T 9" becomes "t-9". */
export function slug(s: string): string {
  return s
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

/** newCustom makes an added booklet of a kind, starting from another booklet's columns. */
export function newCustom(code: string, title: string, builder: string, from: Template | undefined, s: Settings): Template {
  let id = CUSTOM_PREFIX + (slug(code) || "booklet");
  const taken = new Set((s.custom ?? []).map((t) => t.id));
  for (let n = 2; taken.has(id); n++) id = `${CUSTOM_PREFIX}${slug(code) || "booklet"}-${n}`;
  return {
    id,
    code: code.trim(),
    title: title.trim(),
    builder,
    columns: from?.columns?.map((c) => ({ ...c })) ?? [],
    units_per_page: builder === "unit_grid" ? (from?.units_per_page ?? 6) : undefined,
    manager_check: from?.manager_check ?? true,
    notes: [],
  };
}

/** moved returns the list with item i moved by delta places. */
export function moved<T>(list: T[], i: number, delta: number): T[] {
  const j = i + delta;
  if (j < 0 || j >= list.length) return list;
  const out = [...list];
  [out[i], out[j]] = [out[j]!, out[i]!];
  return out;
}
