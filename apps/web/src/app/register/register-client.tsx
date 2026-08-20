"use client";

import { FormEvent, useEffect, useState } from "react";
import { ArrowRight, CircleAlert, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { ThemeToggle } from "@/components/theme-toggle";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { checkCurrentSession } from "@/lib/auth-session";
import {
  PASSWORD_HELP,
  PASSWORD_PATTERN,
  PASSWORD_TITLE,
  REFERRAL_CODE_PATTERN,
  REFERRAL_CODE_TITLE,
  USERNAME_HELP,
  USERNAME_PATTERN,
  USERNAME_TITLE,
  normalizeUsernameInput,
  passwordValidationMessage,
  readUserFormError,
  type UserFormError,
  type UserFormField,
} from "@/lib/user-form";

const registerFieldIDs: Partial<Record<UserFormField, string>> = {
  username: "username",
  email: "email",
  verification_code: "verification-code",
  password: "password",
  confirm: "confirm",
  referral_code: "referral-code",
};

/**
 * RegisterClient 渲染对应的 React 界面组件。
 * @param initialReferralCode 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function RegisterClient({ initialReferralCode }: { initialReferralCode: string }) {
  const router = useRouter();
  const [form, setForm] = useState({
    username: "",
    email: "",
    password: "",
    confirm: "",
    verification_code: "",
    referral_code: initialReferralCode,
  });
  const [error, setError] = useState<UserFormError | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [sendingCode, setSendingCode] = useState(false);
  const [codeSent, setCodeSent] = useState(false);
  const [countdown, setCountdown] = useState(0);

  useEffect(() => {
    if (countdown <= 0) return;
    const timer = window.setInterval(() => setCountdown((value) => Math.max(0, value - 1)), 1000);
    return () => window.clearInterval(timer);
  }, [countdown]);

  useEffect(() => {
    let active = true;
    void checkCurrentSession().then((result) => {
      if (!active || result.status !== "authenticated") return;
      router.replace("/console");
      router.refresh();
    });
    void fetch("/api/auth/options", { cache: "no-store", credentials: "same-origin" })
      .then(async (response) => {
        if (!response.ok) throw new Error();
        return response.json() as Promise<{ registration_enabled: boolean; setup_required: boolean }>;
      })
      .then((options) => {
        if (!active) return;
        if (!options.registration_enabled || options.setup_required) {
          router.replace(options.setup_required ? "/setup" : "/login");
        }
      })
      .catch(() => active && setError({ message: "注册服务暂时不可用" }));
    return () => { active = false; };
  }, [router]);

  function clearFieldError(field: UserFormField) {
    setError((current) => !current?.field || current.field === field ? null : current);
  }

  function showError(nextError: UserFormError) {
    const fieldID = nextError.field ? registerFieldIDs[nextError.field] : undefined;
    setError(fieldID ? nextError : { message: nextError.message });
    if (fieldID) {
      window.requestAnimationFrame(() => document.getElementById(fieldID)?.focus());
    }
  }

  function fieldError(field: UserFormField) {
    return error?.field === field ? error.message : undefined;
  }

  /**
   * submit 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const passwordError = passwordValidationMessage(form.password);
    if (passwordError) {
      showError({ field: "password", message: passwordError });
      return;
    }
    if (form.password !== form.confirm) {
      showError({ field: "confirm", message: "两次输入的密码不一致" });
      return;
    }
    if (!/^[0-9]{6}$/.test(form.verification_code)) {
      showError({ field: "verification_code", message: "请填写邮件中的 6 位数字验证码" });
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const response = await fetch("/api/auth/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({
          username: form.username,
          email: form.email,
          password: form.password,
          verification_code: form.verification_code,
          referral_code: form.referral_code,
        }),
      });
      if (!response.ok) {
        showError(await readUserFormError(response, "注册失败，请稍后重试"));
        return;
      }
      const session = await checkCurrentSession();
      if (session.status === "unauthenticated") {
        throw new Error("账号已创建，但浏览器未能保存登录状态，请前往登录页重试");
      }
      if (session.status === "unavailable") {
        throw new Error("账号已创建，但暂时无法确认登录状态，请稍后重试");
      }
      router.replace("/console");
      router.refresh();
    } catch (reason) {
      showError({ message: reason instanceof Error ? reason.message : "注册失败，请稍后重试" });
    } finally {
      setSubmitting(false);
    }
  }

  /**
   * sendCode 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function sendCode() {
    if (!form.email || sendingCode || countdown > 0) return;
    const emailInput = document.getElementById("email") as HTMLInputElement | null;
    if (!emailInput?.checkValidity()) {
      showError({ field: "email", message: "请输入有效的邮箱地址" });
      emailInput?.reportValidity();
      return;
    }
    setSendingCode(true);
    setError(null);
    try {
      const response = await fetch("/api/auth/register/send-code", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ email: form.email }),
      });
      if (!response.ok) {
        showError(await readUserFormError(response, "验证码发送失败，请稍后重试"));
        return;
      }
      setCodeSent(true);
      setCountdown(60);
    } catch (reason) {
      showError({ message: reason instanceof Error ? reason.message : "验证码发送失败，请稍后重试" });
    } finally {
      setSendingCode(false);
    }
  }

  return (
    <main className="min-h-screen bg-muted/30">
      <AuthHeader />
      <section className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-md items-center px-6 py-12">
        <Card className="w-full">
          <CardHeader>
            <CardTitle className="text-2xl">创建账号</CardTitle>
            <CardDescription>使用用户名、邮箱和密码创建 Novro 账号。</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={submit}>
              <FieldGroup>
                <Field data-invalid={Boolean(fieldError("username"))}>
                  <FieldLabel htmlFor="username">用户名<RequiredMark /></FieldLabel>
                  <Input
                    aria-describedby="username-help"
                    aria-errormessage={fieldError("username") ? "username-error" : undefined}
                    aria-invalid={Boolean(fieldError("username"))}
                    autoCapitalize="none"
                    autoComplete="username"
                    id="username"
                    maxLength={64}
                    name="username"
                    onChange={(event) => {
                      clearFieldError("username");
                      setForm({ ...form, username: normalizeUsernameInput(event.target.value) });
                    }}
                    pattern={USERNAME_PATTERN}
                    placeholder="例如 yuang4099"
                    required
                    spellCheck={false}
                    title={USERNAME_TITLE}
                    value={form.username}
                  />
                  <FieldDescription id="username-help">{USERNAME_HELP}</FieldDescription>
                  <FieldError id="username-error">{fieldError("username")}</FieldError>
                </Field>
                <Field data-invalid={Boolean(fieldError("email"))}>
                  <FieldLabel htmlFor="email">邮箱<RequiredMark /></FieldLabel>
                  <div className="flex gap-2">
                    <Input
                      aria-describedby="email-help"
                      aria-errormessage={fieldError("email") ? "email-error" : undefined}
                      aria-invalid={Boolean(fieldError("email"))}
                      autoComplete="email"
                      className="min-w-0 flex-1"
                      id="email"
                      maxLength={320}
                      name="email"
                      onChange={(event) => {
                        clearFieldError("email");
                        setForm({ ...form, email: event.target.value, verification_code: "" });
                        setCodeSent(false);
                        setCountdown(0);
                      }}
                      placeholder="例如 name@example.com"
                      required
                      type="email"
                      value={form.email}
                    />
                    <Button className="shrink-0" disabled={sendingCode || countdown > 0 || !form.email} onClick={sendCode} type="button" variant="outline">
                      {sendingCode ? <><Spinner data-icon="inline-start" />发送中...</> : countdown > 0 ? `${countdown}s 后重发` : codeSent ? "重新获取" : "获取验证码"}
                    </Button>
                  </div>
                  <FieldDescription id="email-help">用于接收注册验证码和账号通知。</FieldDescription>
                  <FieldError id="email-error">{fieldError("email")}</FieldError>
                </Field>
                <Field data-invalid={Boolean(fieldError("verification_code"))}>
                  <FieldLabel htmlFor="verification-code">邮箱验证码<RequiredMark /></FieldLabel>
                  <Input
                    aria-describedby="verification-code-help"
                    aria-errormessage={fieldError("verification_code") ? "verification-code-error" : undefined}
                    aria-invalid={Boolean(fieldError("verification_code"))}
                    autoComplete="one-time-code"
                    id="verification-code"
                    inputMode="numeric"
                    maxLength={6}
                    name="verification_code"
                    onChange={(event) => {
                      clearFieldError("verification_code");
                      setForm({ ...form, verification_code: event.target.value.replace(/\D/g, "").slice(0, 6) });
                    }}
                    pattern="[0-9]{6}"
                    placeholder="输入 6 位验证码"
                    required
                    value={form.verification_code}
                  />
                  <FieldDescription aria-live="polite" id="verification-code-help">
                    {codeSent ? `验证码已发送至 ${form.email}，请检查收件箱；10 分钟内有效。` : "请先获取验证码，再填写邮件中的 6 位数字。"}
                  </FieldDescription>
                  <FieldError id="verification-code-error">{fieldError("verification_code")}</FieldError>
                </Field>
                <Field data-invalid={Boolean(fieldError("password"))}>
                  <FieldLabel htmlFor="password">密码<RequiredMark /></FieldLabel>
                  <Input
                    aria-describedby="password-help"
                    aria-errormessage={fieldError("password") ? "password-error" : undefined}
                    aria-invalid={Boolean(fieldError("password"))}
                    autoComplete="new-password"
                    id="password"
                    maxLength={72}
                    name="password"
                    onChange={(event) => {
                      clearFieldError("password");
                      setForm({ ...form, password: event.target.value });
                    }}
                    pattern={PASSWORD_PATTERN}
                    required
                    title={PASSWORD_TITLE}
                    type="password"
                    value={form.password}
                  />
                  <FieldDescription id="password-help">{PASSWORD_HELP}</FieldDescription>
                  <FieldError id="password-error">{fieldError("password")}</FieldError>
                </Field>
                <Field data-invalid={Boolean(fieldError("confirm"))}>
                  <FieldLabel htmlFor="confirm">确认密码<RequiredMark /></FieldLabel>
                  <Input
                    aria-errormessage={fieldError("confirm") ? "confirm-error" : undefined}
                    aria-invalid={Boolean(fieldError("confirm"))}
                    autoComplete="new-password"
                    id="confirm"
                    maxLength={72}
                    name="confirm"
                    onChange={(event) => {
                      clearFieldError("confirm");
                      setForm({ ...form, confirm: event.target.value });
                    }}
                    pattern={PASSWORD_PATTERN}
                    required
                    title={PASSWORD_TITLE}
                    type="password"
                    value={form.confirm}
                  />
                  <FieldError id="confirm-error">{fieldError("confirm")}</FieldError>
                </Field>
                <Field data-invalid={Boolean(fieldError("referral_code"))}>
                  <FieldLabel htmlFor="referral-code">邀请码（选填）</FieldLabel>
                  <Input
                    aria-describedby="referral-code-help"
                    aria-errormessage={fieldError("referral_code") ? "referral-code-error" : undefined}
                    aria-invalid={Boolean(fieldError("referral_code"))}
                    autoCapitalize="characters"
                    autoComplete="off"
                    className="font-mono uppercase"
                    id="referral-code"
                    maxLength={12}
                    name="referral_code"
                    onChange={(event) => {
                      clearFieldError("referral_code");
                      setForm({ ...form, referral_code: event.target.value.replace(/[^a-z0-9]/gi, "").toUpperCase().slice(0, 12) });
                    }}
                    pattern={REFERRAL_CODE_PATTERN}
                    placeholder="12 位邀请码"
                    title={REFERRAL_CODE_TITLE}
                    value={form.referral_code}
                  />
                  <FieldDescription id="referral-code-help">填写邀请人提供的 12 位字母或数字；系统会自动转为大写。</FieldDescription>
                  <FieldError id="referral-code-error">{fieldError("referral_code")}</FieldError>
                </Field>
                {error && !error.field ? <Alert variant="destructive"><CircleAlert aria-hidden="true" /><AlertDescription>{error.message}</AlertDescription></Alert> : null}
                <Button className="w-full" disabled={submitting} type="submit">
                  {submitting ? <><Spinner data-icon="inline-start" />创建中...</> : <>创建账号<ArrowRight aria-hidden="true" data-icon="inline-end" /></>}
                </Button>
              </FieldGroup>
            </form>
            <p className="mt-6 text-center text-sm text-muted-foreground">
              已有账号？ <Link className="font-medium text-foreground hover:underline" href="/login">返回登录</Link>
            </p>
          </CardContent>
        </Card>
      </section>
    </main>
  );
}

function RequiredMark() {
  return <><span aria-hidden="true" className="text-destructive">*</span><span className="sr-only">（必填）</span></>;
}

/**
 * AuthHeader 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function AuthHeader() {
  return (
    <header className="flex h-16 items-center justify-between border-b bg-background px-6 lg:px-10">
      <Link className="flex items-center gap-3" href="/">
        <span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <ShieldCheck className="size-4" aria-hidden="true" />
        </span>
        <span className="text-sm font-semibold">Novro Console</span>
      </Link>
      <ThemeToggle />
    </header>
  );
}
