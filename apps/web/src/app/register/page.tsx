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

type ErrorResponse = { error?: { message?: string } };

export default function RegisterPage() {
  const router = useRouter();
  const [form, setForm] = useState({ username: "", display_name: "", password: "", confirm: "" });
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    void fetch("/api/auth/options", { cache: "no-store" }).then(async (response) => {
      if (!response.ok) throw new Error();
      return response.json() as Promise<{ registration_enabled: boolean; setup_required: boolean }>;
    }).then((options) => {
      if (!options.registration_enabled || options.setup_required) router.replace(options.setup_required ? "/setup" : "/login");
    }).catch(() => setError("注册服务暂时不可用"));
  }, [router]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (form.password !== form.confirm) { setError("两次输入的密码不一致"); return; }
    setSubmitting(true); setError("");
    const response = await fetch("/api/auth/register", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username: form.username, display_name: form.display_name, password: form.password }) });
    if (!response.ok) { const body = (await response.json().catch(() => ({}))) as ErrorResponse; setError(body.error?.message ?? "注册失败，请稍后重试"); setSubmitting(false); return; }
    router.replace("/console");
  }

  return <main className="min-h-screen bg-muted/30"><AuthHeader /><section className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-md items-center px-6 py-12"><Card className="w-full"><CardHeader><CardTitle className="text-2xl">创建账号</CardTitle><CardDescription>注册后即可使用 Novro 控制台。</CardDescription></CardHeader><CardContent><form className="space-y-5" onSubmit={submit}><div className="space-y-2"><Label htmlFor="username">用户名</Label><Input autoComplete="username" id="username" minLength={3} onChange={(e) => setForm({ ...form, username: e.target.value })} required value={form.username} /></div><div className="space-y-2"><Label htmlFor="display-name">显示名称</Label><Input autoComplete="name" id="display-name" onChange={(e) => setForm({ ...form, display_name: e.target.value })} value={form.display_name} /></div><div className="space-y-2"><Label htmlFor="password">密码</Label><Input autoComplete="new-password" id="password" minLength={12} onChange={(e) => setForm({ ...form, password: e.target.value })} required type="password" value={form.password} /></div><div className="space-y-2"><Label htmlFor="confirm">确认密码</Label><Input autoComplete="new-password" id="confirm" minLength={12} onChange={(e) => setForm({ ...form, confirm: e.target.value })} required type="password" value={form.confirm} /></div>{error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}<Button className="w-full" disabled={submitting} type="submit">{submitting ? "创建中..." : "创建账号"}<ArrowRight aria-hidden="true" /></Button></form><p className="mt-6 text-center text-sm text-muted-foreground">已有账号？ <Link className="font-medium text-foreground hover:underline" href="/login">返回登录</Link></p></CardContent></Card></section></main>;
}

function AuthHeader() { return <header className="flex h-16 items-center justify-between border-b bg-background px-6 lg:px-10"><Link className="flex items-center gap-3" href="/"><span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"><ShieldCheck className="size-4" aria-hidden="true" /></span><span className="text-sm font-semibold">Novro Console</span></Link><ThemeToggle /></header>; }
