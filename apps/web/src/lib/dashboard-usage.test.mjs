import assert from "node:assert/strict";
import test from "node:test";

import { localDayStartISOString, todayUsageURL } from "./dashboard-usage.ts";

test("dashboard usage starts at local midnight", () => {
  const now = new Date(2026, 7, 13, 18, 42, 17, 999);
  const start = new Date(localDayStartISOString(now));

  assert.equal(start.getFullYear(), now.getFullYear());
  assert.equal(start.getMonth(), now.getMonth());
  assert.equal(start.getDate(), now.getDate());
  assert.equal(start.getHours(), 0);
  assert.equal(start.getMinutes(), 0);
  assert.equal(start.getSeconds(), 0);
  assert.equal(start.getMilliseconds(), 0);
});

test("dashboard requests complete aggregates from the start of today", () => {
  const now = new Date(2026, 7, 13, 18, 42, 17, 999);
  const url = new URL(todayUsageURL(now), "https://novro.test");

  assert.equal(url.pathname, "/api/account/usage");
  assert.equal(url.searchParams.get("from"), localDayStartISOString(now));
  assert.equal(url.searchParams.get("limit"), "1");
});
