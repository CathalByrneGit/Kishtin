import { expect, type Page } from "@playwright/test";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const DATA = join(dirname(fileURLToPath(import.meta.url)), "..", ".data");

/** waitForKitchen waits until start-server.sh has finished seeding. */
export async function waitForKitchen(): Promise<void> {
  for (let i = 0; i < 300; i++) {
    if (existsSync(join(DATA, "ready"))) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("the test kitchen was not seeded in time; see e2e/start-server.sh");
}

/** pair connects this browser with the test tablet's device key. */
export async function pair(page: Page): Promise<void> {
  await page.goto("/");
  await page.getByRole("textbox").first().fill(readFileSync(join(DATA, "devkey"), "utf8").trim());
  await page.getByRole("button", { name: "Pair this device" }).click();
  await expect(page.getByText("Who is logging in?")).toBeVisible();
}

/** loginWithPIN picks a member of staff and types their PIN on the keypad. */
export async function loginWithPIN(page: Page, name: string, pin: string): Promise<void> {
  await page.getByText(name, { exact: true }).click();
  for (const digit of pin) await page.getByRole("button", { name: digit, exact: true }).click();
  await page.getByRole("button", { name: "Log in" }).click();
}
