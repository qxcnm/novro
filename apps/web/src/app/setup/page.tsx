"use client";

import { FormEvent, useEffect, useState } from "react";
import { ArrowRight, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { ThemeToggle } from "@/components/theme-toggle";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { checkCurrentSession } from "@/lib/auth-session";

type ErrorResponse = { error?: { message?: string } };

/**
 * SetupPage 用于更新指定的数据或状态。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function SetupPage() {
  const router = useRouter();
  const [form, setForm] = useState({ setup_token: "", username: "novro", email: "", display_name: "", password: "", confirm: "" });
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let active = true;
    void fetch("/api/auth/options", { cache: "no-store", credentials: "same-origin" }).then(async (response) => {
      if (!response.ok) throw new Error();
      return response.json() as Promise<{ setup_required: boolean }>;
    }).then((options) => {
      if (active && !options.setup_required) router.replace("/login");
    }).catch(() => active && setError("初始化服务暂时不可用"));
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
    if (form.password !== form.confirm) { setError("两次输入的密码不一致"); return; }
    setSubmitting(true); setError("");
    try {
      const response = await fetch("/api/auth/setup", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "same-origin", body: JSON.stringify({ setup_token: form.setup_token, username: form.username, email: form.email, display_name: form.display_name, password: form.password }) });
      if (!response.ok) { const body = (await response.json().catch(() => ({}))) as ErrorResponse; throw new Error(body.error?.message ?? "初始化失败，请稍后重试"); }
      const session = await checkCurrentSession();
      if (session.status === "unauthenticated") throw new Error("管理员已创建，但浏览器未能保存登录状态，请前往登录页重试");
      if (session.status === "unavailable") throw new Error("管理员已创建，但暂时无法确认登录状态，请稍后重试");
      router.replace("/console");
      router.refresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "初始化失败，请稍后重试");
    } finally {
      setSubmitting(false);
    }
  }

  return <main className="min-h-screen bg-muted/30"><AuthHeader /><section className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-lg items-center px-6 py-12"><Card className="w-full"><CardHeader><CardTitle className="text-2xl">初始化管理员</CardTitle><CardDescription>为这套 Novro 实例创建第一个管理员账号。</CardDescription></CardHeader><CardContent><Alert className="mb-6"><ShieldCheck aria-hidden="true" /><AlertTitle>仅首次安装可用</AlertTitle><AlertDescription>初始化令牌由部署管理员通过服务端环境变量提供，不会保存到数据库。创建成功后此入口永久关闭。</AlertDescription></Alert><form className="space-y-5" onSubmit={submit}><div className="space-y-2"><Label htmlFor="setup-token">初始化令牌</Label><Input autoComplete="off" id="setup-token" onChange={(e) => setForm({ ...form, setup_token: e.target.value })} required type="password" value={form.setup_token} /></div><div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="username">管理员用户名</Label><Input autoComplete="username" id="username" minLength={3} onChange={(e) => setForm({ ...form, username: e.target.value })} required value={form.username} /></div><div className="space-y-2"><Label htmlFor="email">管理员邮箱</Label><Input autoComplete="email" id="email" maxLength={320} onChange={(e) => setForm({ ...form, email: e.target.value })} required type="email" value={form.email} /></div></div><div className="space-y-2"><Label htmlFor="display-name">显示名称</Label><Input autoComplete="name" id="display-name" onChange={(e) => setForm({ ...form, display_name: e.target.value })} value={form.display_name} /></div><div className="space-y-2"><Label htmlFor="password">管理员密码</Label><Input autoComplete="new-password" id="password" minLength={8} onChange={(e) => setForm({ ...form, password: e.target.value })} pattern="(?=.*[A-Za-z])(?=.*[0-9]).{8,}" required title="密码至少 8 位，且必须包含英文和数字" type="password" value={form.password} /><p className="text-xs text-muted-foreground">至少 8 位，且必须包含英文和数字。</p></div><div className="space-y-2"><Label htmlFor="confirm">确认密码</Label><Input autoComplete="new-password" id="confirm" minLength={8} onChange={(e) => setForm({ ...form, confirm: e.target.value })} pattern="(?=.*[A-Za-z])(?=.*[0-9]).{8,}" required title="密码至少 8 位，且必须包含英文和数字" type="password" value={form.confirm} /></div>{error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}<Button className="w-full" disabled={submitting} type="submit">{submitting ? "初始化中..." : "创建管理员"}<ArrowRight aria-hidden="true" /></Button></form></CardContent></Card></section></main>;
}

/**
 * AuthHeader 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function AuthHeader() { return <header className="flex h-16 items-center justify-between border-b bg-background px-6 lg:px-10"><Link className="flex items-center gap-3" href="/"><span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"><ShieldCheck className="size-4" aria-hidden="true" /></span><span className="text-sm font-semibold">Novro Console</span></Link><ThemeToggle /></header>; }
