import assert from "node:assert/strict";
import test from "node:test";

import { bulkResultMessage, runBulkAction } from "./bulk-action.ts";

test("runBulkAction limits concurrent requests and keeps safe failures", async () => {
  let active = 0;
  let maximumActive = 0;
  const result = await runBulkAction(["one", "two", "three", "four"], async (id) => {
    active += 1;
    maximumActive = Math.max(maximumActive, active);
    await new Promise((resolve) => setTimeout(resolve, 5));
    active -= 1;
    if (id === "two") return new Response(JSON.stringify({ error: { message: "项目不可用" } }), { status: 409 });
    return new Response(null, { status: 204 });
  }, 2);

  assert.ok(maximumActive <= 2);
  assert.deepEqual(new Set(result.succeeded), new Set(["one", "three", "four"]));
  assert.deepEqual(result.failed, [{ id: "two", message: "项目不可用" }]);
  assert.equal(bulkResultMessage("启用", result), "已批量启用 3 项，1 项失败：项目不可用");
});

test("runBulkAction turns network errors into a retryable failure", async () => {
  const result = await runBulkAction(["one"], async () => {
    throw new Error("details should stay out of the UI");
  });

  assert.deepEqual(result.succeeded, []);
  assert.deepEqual(result.failed, [{ id: "one", message: "网络请求失败，请稍后重试" }]);
});
