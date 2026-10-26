import { expect, test } from "bun:test";
import { applySetting, checklistText, parseChecklist, copyStatus, draftBooklets, moved, newCustom, slug, switchBundle, withSetting, type Bundle, type Settings, type Template } from "./recordbooks";

const sc2: Template = {
  id: "fsai-sc2",
  code: "SC2",
  title: "Fridge records",
  builder: "unit_grid",
  units_per_page: 6,
  columns: [{ label: "Date", field: "date", width: 14 }, { label: "Unit", field: "unit_checks", width: 168 }],
  notes: ["Chilled food: 0°C to 5°C."],
  manager_check: true,
  source: { url: "https://example.invalid/sc2.pdf", published: "2012-10", sha256: "a".repeat(64) },
};
const sc7: Template = { id: "fsai-sc7", code: "SC7", title: "Fitness", missing: "Not recorded" };
const bundle: Bundle = { id: "fsai", name: "FSAI", country: "Ireland", issuer: "FSAI", language: "en", templates: [sc2, sc7] };

test("apply a booklet's changes", () => {
  const t = applySetting(sc2, { template: "fsai-sc2", title: "Fridge log", notes: ["Our plan, section 4."], units_per_page: 4, manager_check: false });
  expect(t.title).toBe("Fridge log");
  expect(t.notes).toEqual(["Chilled food: 0°C to 5°C.", "Our plan, section 4."]);
  expect(t.units_per_page).toBe(4);
  expect(t.manager_check).toBe(false);
  expect(t.columns).toBe(sc2.columns);
  expect(applySetting(sc2, undefined)).toBe(sc2);
});

test("draft booklets, changes and switching bundles", () => {
  let s: Settings = { bundle: "fsai", books: [], custom: [] };
  s = withSetting(s, { template: "fsai-sc7", off: true });
  s = withSetting(s, { template: "fsai-sc2", title: "Fridge log" });
  const custom = newCustom("T 9", "Relevés", "unit_grid", sc2, s);
  expect(custom.id).toBe("custom-t-9");
  expect(custom.columns).toEqual(sc2.columns);
  expect(custom.columns).not.toBe(sc2.columns);
  s = { ...s, custom: [custom] };
  expect(newCustom("T 9", "Again", "hot_hold", undefined, s).id).toBe("custom-t-9-2");
  s = withSetting(s, { template: custom.id, off: true });

  const d = draftBooklets(bundle, s);
  expect(d.map((b) => [b.template.code, b.off, b.custom, b.changed.join(",")])).toEqual([
    ["SC2", false, false, "title"],
    ["SC7", true, false, ""],
    ["T 9", true, true, ""],
  ]);
  // Turning a change back off drops the entry.
  expect(withSetting(s, { template: "fsai-sc7" }).books?.some((b) => b.template === "fsai-sc7")).toBe(false);

  const moved_ = switchBundle(s, "general");
  expect(moved_.bundle).toBe("general");
  expect(moved_.books?.map((b) => b.template)).toEqual([custom.id]);
});

test("copy status", () => {
  expect(copyStatus(sc2, undefined)).toBeNull();
  expect(copyStatus(sc2, "a".repeat(64))?.tone).toBe("success");
  expect(copyStatus(sc2, "b".repeat(64))?.text).toContain("not the 2012-10 edition");
  expect(copyStatus(sc7, "b".repeat(64))?.tone).toBe("neutral");
});

test("helpers", () => {
  expect(slug("Relevés T2")).toBe("releves-t2");
  expect(moved([1, 2, 3], 0, 1)).toEqual([2, 1, 3]);
  expect(moved([1, 2, 3], 0, -1)).toEqual([1, 2, 3]);
});

test("checklist text", () => {
  const secs = [{ items: ["Basic hygiene rules"] }, { title: "Pest Control", items: ["Any pests?", "Flyscreens fitted?"] }];
  const text = checklistText(secs);
  expect(text).toBe("Basic hygiene rules\n\n# Pest Control\nAny pests?\nFlyscreens fitted?");
  expect(parseChecklist(text)).toEqual(secs);
  expect(parseChecklist("# Empty\n\n#Bar\n  Ice machine clean? \n")).toEqual([{ title: "Bar", items: ["Ice machine clean?"] }]);
});
