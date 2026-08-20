// HTML's `pattern` uses the Unicode Sets (`v`) flag, where `-` must be escaped
// even at the end of a character class.
export const USERNAME_PATTERN = "[a-z0-9][a-z0-9._\\-]{2,63}";
export const USERNAME_TITLE = "用户名需为 3 到 64 位，只能使用小写字母、数字、点号、下划线和连字符，且必须以字母或数字开头";
export const USERNAME_HELP = "不能填写邮箱地址。3 到 64 位；只允许小写字母、数字、点号、下划线和连字符，且必须以字母或数字开头。";

export const PASSWORD_PATTERN = "(?=.*[A-Za-z])(?=.*[0-9]).*";
export const PASSWORD_TITLE = "密码需为 8 到 72 字节，且必须同时包含英文和数字";
export const PASSWORD_HELP = "8 到 72 字节，且必须同时包含英文和数字。";

export const REFERRAL_CODE_PATTERN = "[A-Z0-9]{12}";
export const REFERRAL_CODE_TITLE = "邀请码必须为 12 位字母或数字";

export type UserFormField =
  | "username"
  | "email"
  | "verification_code"
  | "password"
  | "confirm"
  | "referral_code"
  | "display_name"
  | "role"
  | "setup_token";

export type UserFormError = {
  field?: UserFormField;
  message: string;
};

const userFormFields = new Set<UserFormField>([
  "username",
  "email",
  "verification_code",
  "password",
  "confirm",
  "referral_code",
  "display_name",
  "role",
  "setup_token",
]);

const errorCodeFields: Partial<Record<string, UserFormField>> = {
  email_taken: "email",
  invalid_email: "email",
  invalid_referral_code: "referral_code",
  invalid_setup_token: "setup_token",
  username_taken: "username",
  verification_expired: "verification_code",
  verification_invalid: "verification_code",
};

export function normalizeUsernameInput(value: string) {
  return value.toLowerCase();
}

export function isValidUsername(value: string) {
  return new RegExp(`^(?:${USERNAME_PATTERN})$`).test(value.trim().toLowerCase());
}

export function passwordValidationMessage(value: string) {
  const byteLength = new TextEncoder().encode(value).byteLength;
  if (byteLength < 8 || byteLength > 72 || !/[A-Za-z]/.test(value) || !/[0-9]/.test(value)) {
    return PASSWORD_TITLE;
  }
  return "";
}

export function parseUserFormError(value: unknown, fallback: string): UserFormError {
  if (!value || typeof value !== "object" || !("error" in value)) {
    return { message: fallback };
  }
  const error = value.error;
  if (!error || typeof error !== "object") {
    return { message: fallback };
  }
  const message = "message" in error && typeof error.message === "string" && error.message ? error.message : fallback;
  const rawField = "field" in error && typeof error.field === "string" ? error.field : "";
  const code = "code" in error && typeof error.code === "string" ? error.code : "";
  const field = userFormFields.has(rawField as UserFormField)
    ? rawField as UserFormField
    : errorCodeFields[code];
  return field ? { field, message } : { message };
}

export async function readUserFormError(response: Response, fallback: string) {
  const body = await response.json().catch(() => null) as unknown;
  return parseUserFormError(body, fallback);
}
