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

type ErrorResponse = { error?: { message?: string } };

export default function SetupPage() {
  const router = useRouter();
  const [form, setForm] = useState({ setup_token: "", username: "admin", display_name: "", password: "", confirm: "" });
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => { void fetch("/api/auth/options", { cache: "no-store" }).then((r) => r.json() as Promise<{ setup_required: boolean }>).then((o) => { if (!o.setup_required) router.replace("/login"); }).catch(() => setError("初始化服务暂时不可用")); }, [router]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (form.password !== form.confirm) { setError("两次输入的密码不一致"); return; }
    setSubmitting(true); setError("");
    const response = await fetch("/api/auth/setup", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ setup_token: form.setup_token, username: form.username, display_name: form.display_name, password: form.password }) });
    if (!response.ok) { const body = (await response.json().catch(() => ({}))) as ErrorResponse; setError(body.error?.message ?? "初始化失败，请稍后重试"); setSubmitting(false); return; }
    router.replace("/admin/users"); router.refresh();
  }

  return <main className="min-h-screen bg-muted/30"><AuthHeader /><section className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-lg items-center px-6 py-12"><Card className="w-full"><CardHeader><CardTitle className="text-2xl">初始化管理员</CardTitle><CardDescription>为这套 Novro 实例创建第一个管理员账号。</CardDescription></CardHeader><CardContent><Alert className="mb-6"><ShieldCheck aria-hidden="true" /><AlertTitle>仅首次安装可用</AlertTitle><AlertDescription>初始化令牌由部署管理员通过服务端环境变量提供，不会保存到数据库。创建成功后此入口永久关闭。</AlertDescription></Alert><form className="space-y-5" onSubmit={submit}><div className="space-y-2"><Label htmlFor="setup-token">初始化令牌</Label><Input autoComplete="off" id="setup-token" onChange={(e) => setForm({ ...form, setup_token: e.target.value })} required type="password" value={form.setup_token} /></div><div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="username">管理员用户名</Label><Input autoComplete="username" id="username" minLength={3} onChange={(e) => setForm({ ...form, username: e.target.value })} required value={form.username} /></div><div className="space-y-2"><Label htmlFor="display-name">显示名称</Label><Input autoComplete="name" id="display-name" onChange={(e) => setForm({ ...form, display_name: e.target.value })} value={form.display_name} /></div></div><div className="space-y-2"><Label htmlFor="password">管理员密码</Label><Input autoComplete="new-password" id="password" minLength={12} onChange={(e) => setForm({ ...form, password: e.target.value })} required type="password" value={form.password} /></div><div className="space-y-2"><Label htmlFor="confirm">确认密码</Label><Input autoComplete="new-password" id="confirm" minLength={12} onChange={(e) => setForm({ ...form, confirm: e.target.value })} required type="password" value={form.confirm} /></div>{error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}<Button className="w-full" disabled={submitting} type="submit">{submitting ? "初始化中..." : "创建管理员"}<ArrowRight aria-hidden="true" /></Button></form></CardContent></Card></section></main>;
}

function AuthHeader() { return <header className="flex h-16 items-center justify-between border-b bg-background px-6 lg:px-10"><Link className="flex items-center gap-3" href="/"><span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"><ShieldCheck className="size-4" aria-hidden="true" /></span><span className="text-sm font-semibold">Novro Console</span></Link><ThemeToggle /></header>; }
