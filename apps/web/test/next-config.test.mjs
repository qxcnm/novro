import assert from "node:assert/strict";
import test from "node:test";

import nextConfig, { normalizeBackendURL } from "../next.config.ts";

test("normalizeBackendURL accepts pure origins and canonicalizes them", () => {
  assert.equal(normalizeBackendURL(undefined, true), "http://127.0.0.1:8080");
  assert.equal(normalizeBackendURL(" http://127.1:9090/ ", true), "http://127.0.0.1:9090");
  assert.equal(normalizeBackendURL("https://[::1]:8443", true), "https://[::1]:8443");
  assert.equal(normalizeBackendURL("https://dev-api.example.com/", false), "https://dev-api.example.com");
});

test("normalizeBackendURL rejects unsafe URL components", () => {
  for (const value of [
    "",
    "not-a-url",
    "ftp://127.0.0.1:8080",
    "http://user@127.0.0.1:8080",
    "http://127.0.0.1:8080/api",
    "http://127.0.0.1:8080?debug=true",
    "http://127.0.0.1:8080#fragment",
  ]) {
    assert.throws(() => normalizeBackendURL(value, false), /must be an absolute http or https origin/);
  }
});

test("normalizeBackendURL requires loopback in production", () => {
  for (const value of ["https://api.example.com", "http://10.0.0.10:8080", "http://0.0.0.0:8080"]) {
    assert.throws(() => normalizeBackendURL(value, true), /must use a loopback host/);
  }
});

test("rewrites use the normalized backend origin", async () => {
  const previous = process.env.NOVRO_SERVER_URL;
  try {
    process.env.NOVRO_SERVER_URL = "http://127.1:9090/";
    const rewrites = await nextConfig.rewrites();
    assert.deepEqual(rewrites, [{ source: "/api/:path*", destination: "http://127.0.0.1:9090/api/:path*" }]);
  } finally {
    if (previous === undefined) {
      delete process.env.NOVRO_SERVER_URL;
    } else {
      process.env.NOVRO_SERVER_URL = previous;
    }
  }
});
