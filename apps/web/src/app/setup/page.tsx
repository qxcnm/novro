"use client";

import { FormEvent, useEffect, useState } from "react";
import { ArrowRight, CircleAlert, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { ThemeToggle } from "@/components/theme-toggle";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
  USERNAME_HELP,
  USERNAME_PATTERN,
  USERNAME_TITLE,
  normalizeUsernameInput,
  passwordValidationMessage,
  readUserFormError,
  type UserFormError,
  type UserFormField,
} from "@/lib/user-form";

const setupFieldIDs: Partial<Record<UserFormField, string>> = {
  setup_token: "setup-token",
  username: "username",
  email: "email",
  display_name: "display-name",
  password: "password",
  confirm: "confirm",
};

/**
 * SetupPage 用于更新指定的数据或状态。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function SetupPage() {
  const router = useRouter();
  const [form, setForm] = useState({ setup_token: "", username: "novro", email: "", display_name: "", password: "", confirm: "" });
  const [error, setError] = useState<UserFormError | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let active = true;
    void fetch("/api/auth/options", { cache: "no-store", credentials: "same-origin" }).then(async (response) => {
      if (!response.ok) throw new Error();
      return response.json() as Promise<{ setup_required: boolean }>;
    }).then((options) => {
      if (active && !options.setup_required) router.replace("/login");
    }).catch(() => active && setError({ message: "初始化服务暂时不可用" }));
    return () => { active = false; };
  }, [router]);

  function clearFieldError(field: UserFormField) {
    setError((current) => !current?.field || current.field === field ? null : current);
  }

  function showError(nextError: UserFormError) {
    const fieldID = nextError.field ? setupFieldIDs[nextError.field] : undefined;
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
    if (passwordError) { showError({ field: "password", message: passwordError }); return; }
    if (form.password !== form.confirm) { showError({ field: "confirm", message: "两次输入的密码不一致" }); return; }
    setSubmitting(true); setError(null);
    try {
      const response = await fetch("/api/auth/setup", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "same-origin", body: JSON.stringify({ setup_token: form.setup_token, username: form.username, email: form.email, display_name: form.display_name, password: form.password }) });
      if (!response.ok) { showError(await readUserFormError(response, "初始化失败，请稍后重试")); return; }
      const session = await checkCurrentSession();
      if (session.status === "unauthenticated") throw new Error("管理员已创建，但浏览器未能保存登录状态，请前往登录页重试");
      if (session.status === "unavailable") throw new Error("管理员已创建，但暂时无法确认登录状态，请稍后重试");
      router.replace("/console");
      router.refresh();
    } catch (reason) {
      showError({ message: reason instanceof Error ? reason.message : "初始化失败，请稍后重试" });
    } finally {
      setSubmitting(false);
    }
  }

  return <main className="min-h-screen bg-muted/30"><AuthHeader /><section className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-lg items-center px-6 py-12"><Card className="w-full"><CardHeader><CardTitle className="text-2xl">初始化管理员</CardTitle><CardDescription>为这套 Novro 实例创建第一个管理员账号。</CardDescription></CardHeader><CardContent><Alert className="mb-6"><ShieldCheck aria-hidden="true" /><AlertTitle>仅首次安装可用</AlertTitle><AlertDescription>初始化令牌由部署管理员通过服务端环境变量提供，不会保存到数据库。创建成功后此入口永久关闭。</AlertDescription></Alert><form onSubmit={submit}><FieldGroup><Field data-invalid={Boolean(fieldError("setup_token"))}><FieldLabel htmlFor="setup-token">初始化令牌<RequiredMark /></FieldLabel><Input aria-errormessage={fieldError("setup_token") ? "setup-token-error" : undefined} aria-invalid={Boolean(fieldError("setup_token"))} autoComplete="off" id="setup-token" onChange={(e) => { clearFieldError("setup_token"); setForm({ ...form, setup_token: e.target.value }); }} required type="password" value={form.setup_token} /><FieldError id="setup-token-error">{fieldError("setup_token")}</FieldError></Field><div className="grid gap-4 sm:grid-cols-2"><Field data-invalid={Boolean(fieldError("username"))}><FieldLabel htmlFor="username">管理员用户名<RequiredMark /></FieldLabel><Input aria-describedby="setup-username-help" aria-errormessage={fieldError("username") ? "setup-username-error" : undefined} aria-invalid={Boolean(fieldError("username"))} autoCapitalize="none" autoComplete="username" id="username" maxLength={64} onChange={(e) => { clearFieldError("username"); setForm({ ...form, username: normalizeUsernameInput(e.target.value) }); }} pattern={USERNAME_PATTERN} placeholder="例如 novro-admin" required spellCheck={false} title={USERNAME_TITLE} value={form.username} /><FieldDescription id="setup-username-help">{USERNAME_HELP}</FieldDescription><FieldError id="setup-username-error">{fieldError("username")}</FieldError></Field><Field data-invalid={Boolean(fieldError("email"))}><FieldLabel htmlFor="email">管理员邮箱<RequiredMark /></FieldLabel><Input aria-errormessage={fieldError("email") ? "setup-email-error" : undefined} aria-invalid={Boolean(fieldError("email"))} autoComplete="email" id="email" maxLength={320} onChange={(e) => { clearFieldError("email"); setForm({ ...form, email: e.target.value }); }} placeholder="例如 admin@example.com" required type="email" value={form.email} /><FieldError id="setup-email-error">{fieldError("email")}</FieldError></Field></div><Field data-invalid={Boolean(fieldError("display_name"))}><FieldLabel htmlFor="display-name">显示名称（选填）</FieldLabel><Input aria-describedby="display-name-help" aria-errormessage={fieldError("display_name") ? "display-name-error" : undefined} aria-invalid={Boolean(fieldError("display_name"))} autoComplete="name" id="display-name" maxLength={128} onChange={(e) => { clearFieldError("display_name"); setForm({ ...form, display_name: e.target.value }); }} placeholder="例如 系统管理员" value={form.display_name} /><FieldDescription id="display-name-help">留空时使用用户名作为显示名称。</FieldDescription><FieldError id="display-name-error">{fieldError("display_name")}</FieldError></Field><Field data-invalid={Boolean(fieldError("password"))}><FieldLabel htmlFor="password">管理员密码<RequiredMark /></FieldLabel><Input aria-describedby="setup-password-help" aria-errormessage={fieldError("password") ? "setup-password-error" : undefined} aria-invalid={Boolean(fieldError("password"))} autoComplete="new-password" id="password" maxLength={72} onChange={(e) => { clearFieldError("password"); setForm({ ...form, password: e.target.value }); }} pattern={PASSWORD_PATTERN} required title={PASSWORD_TITLE} type="password" value={form.password} /><FieldDescription id="setup-password-help">{PASSWORD_HELP}</FieldDescription><FieldError id="setup-password-error">{fieldError("password")}</FieldError></Field><Field data-invalid={Boolean(fieldError("confirm"))}><FieldLabel htmlFor="confirm">确认密码<RequiredMark /></FieldLabel><Input aria-errormessage={fieldError("confirm") ? "setup-confirm-error" : undefined} aria-invalid={Boolean(fieldError("confirm"))} autoComplete="new-password" id="confirm" maxLength={72} onChange={(e) => { clearFieldError("confirm"); setForm({ ...form, confirm: e.target.value }); }} pattern={PASSWORD_PATTERN} required title={PASSWORD_TITLE} type="password" value={form.confirm} /><FieldError id="setup-confirm-error">{fieldError("confirm")}</FieldError></Field>{error && !error.field ? <Alert variant="destructive"><CircleAlert aria-hidden="true" /><AlertDescription>{error.message}</AlertDescription></Alert> : null}<Button className="w-full" disabled={submitting} type="submit">{submitting ? <><Spinner data-icon="inline-start" />初始化中...</> : <>创建管理员<ArrowRight aria-hidden="true" data-icon="inline-end" /></>}</Button></FieldGroup></form></CardContent></Card></section></main>;
}

function RequiredMark() { return <><span aria-hidden="true" className="text-destructive">*</span><span className="sr-only">（必填）</span></>; }

/**
 * AuthHeader 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function AuthHeader() { return <header className="flex h-16 items-center justify-between border-b bg-background px-6 lg:px-10"><Link className="flex items-center gap-3" href="/"><span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"><ShieldCheck className="size-4" aria-hidden="true" /></span><span className="text-sm font-semibold">Novro Console</span></Link><ThemeToggle /></header>; }
