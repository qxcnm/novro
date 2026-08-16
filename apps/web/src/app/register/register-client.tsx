"use client";

import { FormEvent, useEffect, useState } from "react";
import { ArrowRight, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { ThemeToggle } from "@/components/theme-toggle";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { checkCurrentSession } from "@/lib/auth-session";

type ErrorResponse = { error?: { message?: string } };

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
  const [error, setError] = useState("");
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
      .catch(() => active && setError("注册服务暂时不可用"));
    return () => { active = false; };
  }, [router]);

  /**
   * submit 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (form.password !== form.confirm) {
      setError("两次输入的密码不一致");
      return;
    }
    if (!form.verification_code) {
      setError("请先获取并填写邮箱验证码");
      return;
    }
    setSubmitting(true);
    setError("");
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
        /**
         * body 封装该名称对应的业务处理逻辑。
         * @param await 本次操作需要使用的输入参数。
         * @author Gao Hongshun
         * @date 2026-08-13
         */
        const body = (await response.json().catch(() => ({}))) as ErrorResponse;
        throw new Error(body.error?.message ?? "注册失败，请稍后重试");
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
      setError(reason instanceof Error ? reason.message : "注册失败，请稍后重试");
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
    setSendingCode(true);
    setError("");
    try {
      const response = await fetch("/api/auth/register/send-code", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ email: form.email }),
      });
      if (!response.ok) {
        /**
         * body 封装该名称对应的业务处理逻辑。
         * @param await 本次操作需要使用的输入参数。
         * @author Gao Hongshun
         * @date 2026-08-13
         */
        const body = (await response.json().catch(() => ({}))) as ErrorResponse;
        throw new Error(body.error?.message ?? "验证码发送失败，请稍后重试");
      }
      setCodeSent(true);
      setCountdown(60);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "验证码发送失败，请稍后重试");
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
            <form className="space-y-5" onSubmit={submit}>
              <div className="space-y-2">
                <Label htmlFor="username">用户名</Label>
                <Input autoComplete="username" id="username" minLength={3} onChange={(event) => setForm({ ...form, username: event.target.value })} required value={form.username} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="email">邮箱</Label>
                <div className="flex gap-2">
                  <Input
                    className="min-w-0 flex-1"
                    autoComplete="email"
                    id="email"
                    maxLength={320}
                    onChange={(event) => {
                      setForm({ ...form, email: event.target.value, verification_code: "" });
                      setCodeSent(false);
                      setCountdown(0);
                    }}
                    required
                    type="email"
                    value={form.email}
                  />
                  <Button className="shrink-0" disabled={sendingCode || countdown > 0 || !form.email} onClick={sendCode} type="button" variant="outline">
                    {sendingCode ? "发送中..." : countdown > 0 ? `${countdown}s 后重发` : codeSent ? "重新获取" : "获取验证码"}
                  </Button>
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="verification-code">邮箱验证码</Label>
                <Input
                  autoComplete="one-time-code"
                  id="verification-code"
                  inputMode="numeric"
                  maxLength={6}
                  pattern="[0-9]{6}"
                  onChange={(event) => setForm({ ...form, verification_code: event.target.value.replace(/\D/g, "").slice(0, 6) })}
                  required
                  value={form.verification_code}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">密码</Label>
                <Input autoComplete="new-password" id="password" minLength={8} onChange={(event) => setForm({ ...form, password: event.target.value })} pattern="(?=.*[A-Za-z])(?=.*[0-9]).{8,}" required title="密码至少 8 位，且必须包含英文和数字" type="password" value={form.password} />
                <p className="text-xs text-muted-foreground">至少 8 位，且必须包含英文和数字。</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="referral-code">邀请码（选填）</Label>
                <Input
                  autoCapitalize="characters"
                  autoComplete="off"
                  className="font-mono uppercase"
                  id="referral-code"
                  maxLength={12}
                  onChange={(event) => setForm({ ...form, referral_code: event.target.value.replace(/[^a-z0-9]/gi, "").toUpperCase().slice(0, 12) })}
                  value={form.referral_code}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="confirm">确认密码</Label>
                <Input autoComplete="new-password" id="confirm" minLength={8} onChange={(event) => setForm({ ...form, confirm: event.target.value })} pattern="(?=.*[A-Za-z])(?=.*[0-9]).{8,}" required title="密码至少 8 位，且必须包含英文和数字" type="password" value={form.confirm} />
              </div>
              {error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}
              <Button className="w-full" disabled={submitting} type="submit">
                {submitting ? "创建中..." : "创建账号"}<ArrowRight aria-hidden="true" />
              </Button>
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
