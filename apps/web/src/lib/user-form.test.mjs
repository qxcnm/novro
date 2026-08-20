import assert from "node:assert/strict";
import test from "node:test";

import {
  USERNAME_PATTERN,
  isValidUsername,
  normalizeUsernameInput,
  parseUserFormError,
  passwordValidationMessage,
} from "./user-form.ts";

test("username rules reject email addresses and enforce backend boundaries", () => {
  assert.equal(isValidUsername("yuuang4099@gmail.com"), false);
  assert.equal(isValidUsername("ab"), false);
  assert.equal(isValidUsername("-alice"), false);
  assert.equal(isValidUsername("alice.chen_01"), true);
  assert.equal(isValidUsername(`a${"b".repeat(63)}`), true);
  assert.equal(isValidUsername(`a${"b".repeat(64)}`), false);
  assert.equal(normalizeUsernameInput("Alice.Chen"), "alice.chen");
  assert.doesNotThrow(() => new RegExp(`^(?:${USERNAME_PATTERN})$`, "v"));
});

test("password validation follows the server byte-based policy", () => {
  assert.match(passwordValidationMessage("short1"), /8 到 72 字节/);
  assert.match(passwordValidationMessage("abcdefgh"), /8 到 72 字节/);
  assert.equal(passwordValidationMessage("Abcdefg1"), "");
  assert.equal(passwordValidationMessage(`${"密".repeat(21)}abcdefg1`), "");
  assert.match(passwordValidationMessage(`${"密".repeat(22)}abcdefg1`), /8 到 72 字节/);
});

test("API errors keep field context and map legacy verification codes", () => {
  assert.deepEqual(parseUserFormError({ error: { field: "username", message: "用户名格式错误" } }, "fallback"), {
    field: "username",
    message: "用户名格式错误",
  });
  assert.deepEqual(parseUserFormError({ error: { code: "verification_expired", message: "验证码已过期" } }, "fallback"), {
    field: "verification_code",
    message: "验证码已过期",
  });
  assert.deepEqual(parseUserFormError({}, "fallback"), { message: "fallback" });
});
