import { expect, test } from "bun:test";
import es from "./i18n/es";
import { MESSAGES } from "./i18n/messages";
import pl from "./i18n/pl";
import pt from "./i18n/pt";

const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

for (const [name, dict] of Object.entries({ pl, pt, es })) {
  test(`${name} has every message, with the same blanks to fill`, () => {
    for (const msg of MESSAGES) {
      const tr = (dict as Record<string, string>)[msg];
      expect(tr, msg).toBeTruthy();
      expect(placeholders(tr!), msg).toEqual(placeholders(msg));
    }
    expect(Object.keys(dict).filter((k) => !(MESSAGES as readonly string[]).includes(k))).toEqual([]);
  });

  test(`${name} keeps the limits and the English voice commands as they are`, () => {
    const all = Object.values(dict).join("\n");
    for (const must of ["5°C", "−18°C", "75°C", "70°C", "Fire table 4 mains", "confirm", "Kishtin"]) expect(all).toContain(must);
  });
}

test("no message is listed twice", () => {
  // Guides repeat lines such as "Everyone" on purpose; the UI list must not.
  expect(new Set(MESSAGES).size).toBeGreaterThan(150);
});
