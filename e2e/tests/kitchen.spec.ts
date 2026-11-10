import { expect, test } from "@playwright/test";
import { loginWithPIN, pair, waitForKitchen } from "./helpers";

// The journeys a kitchen depends on, against a real kishtind with a
// synthetic kitchen (see ../start-server.sh). They run in order and share
// that kitchen.

test.beforeAll(waitForKitchen);

test("Kishtin says it is up, without a key", async ({ request }) => {
  const res = await request.get("/healthz");
  expect(res.ok()).toBeTruthy();
  expect(await res.json()).toMatchObject({ status: "ok" });
  expect((await request.get("/api/haccp/today")).status()).toBe(401);
});

test("a wrong PIN does not log in", async ({ page }) => {
  await pair(page);
  await loginWithPIN(page, "Liam Cook", "0000");
  await expect(page.getByText(/wrong name or pin/i).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Log check" })).toHaveCount(0);
});

test("a cook logs a fridge check and it shows on Today", async ({ page }) => {
  await pair(page);
  await loginWithPIN(page, "Liam Cook", "1234");
  await page.getByRole("button", { name: "Log check" }).click();
  await expect(page.getByText("Logging as Liam Cook.")).toBeVisible();
  await page.getByPlaceholder("0.0").fill("3.4");
  await expect(page.getByText("Pass", { exact: true }).last()).toBeVisible();
  await page.getByRole("button", { name: "Save record" }).click();

  await page.getByRole("link", { name: "Today" }).click();
  await expect(page.getByRole("heading", { name: "Today" })).toBeVisible();
  await expect(page.getByText("3.4°C").first()).toBeVisible();
});

test("a failed reading cannot be saved without a corrective action", async ({ page }) => {
  await pair(page);
  await loginWithPIN(page, "Liam Cook", "1234");
  await page.getByRole("button", { name: "Log check" }).click();
  await page.getByPlaceholder("0.0").fill("9.5");
  await expect(page.getByText("Fail", { exact: true }).first()).toBeVisible();
  const save = page.getByRole("button", { name: "Save record" });
  await expect(save).toBeDisabled();

  await page.getByLabel(/Corrective action/).fill("Moved food to the prep fridge");
  await expect(save).toBeEnabled();
  await save.click();

  await page.getByRole("link", { name: "Today" }).click();
  await expect(page.getByText("Moved food to the prep fridge").first()).toBeVisible();
});

test("a cook cannot fire a course; the head chef fires mains and the grill sees them", async ({ page }) => {
  await pair(page);
  await loginWithPIN(page, "Liam Cook", "1234");
  await expect(page.getByText("Table 4")).toBeVisible();
  await expect(page.getByRole("button", { name: "Fire mains" })).toHaveCount(0);

  await page.context().clearCookies();
  await page.evaluate(() => sessionStorage.clear());
  await page.reload();
  // Switch staff: the device stays paired.
  if (!(await page.getByText("Who is logging in?").isVisible().catch(() => false))) {
    await page.getByRole("button", { name: /Switch staff|Log out/ }).first().click();
  }
  await loginWithPIN(page, "Aoife Chef", "2222");
  await page.getByRole("button", { name: "Fire mains" }).click();
  await expect(page.getByText(/On track|At risk/).first()).toBeVisible();

  await page.getByRole("link", { name: "Stations" }).click();
  await expect(page.getByText("Synthetic burger").first()).toBeVisible();
  await expect(page.getByRole("button", { name: /^Start/ }).first()).toBeVisible();
});
