import assert from "node:assert/strict";
import test from "node:test";

import { checkCurrentSession } from "./auth-session.ts";

test("checkCurrentSession returns authenticated user and includes same-origin credentials", async () => {
  let options;
  const result = await checkCurrentSession(async (_url, init) => {
    options = init;
    return new Response(JSON.stringify({ user: { id: "user-1", username: "alice", email: "alice@example.com", display_name: "Alice", role: "member" } }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  assert.equal(result.status, "authenticated");
  assert.equal(result.user.username, "alice");
  assert.equal(options.credentials, "same-origin");
});

test("checkCurrentSession only classifies 401 as unauthenticated", async () => {
  assert.deepEqual(await checkCurrentSession(async () => new Response(null, { status: 401 })), { status: "unauthenticated" });
  assert.deepEqual(await checkCurrentSession(async () => new Response(null, { status: 503 })), { status: "unavailable" });
  assert.deepEqual(await checkCurrentSession(async () => { throw new Error("network down"); }), { status: "unavailable" });
});
