"use client";

import { FormEvent, Suspense, useEffect, useState } from "react";
import { ArrowRight, Building2, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";

import { ThemeToggle } from "@/components/theme-toggle";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";

type ErrorResponse = { error?: { message?: string } };
type AuthOptions = {
  setup_required: boolean;
  setup_enabled: boolean;
  registration_enabled: boolean;
  oidc_enabled: boolean;
  oidc_display_name: string;
};
export default function LoginPage() {
  return (
    <Suspense fallback={<LoginPageShell />}>
      <LoginForm />
    </Suspense>
  );
}

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [options, setOptions] = useState<AuthOptions | null>(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let active = true;
    void fetch("/api/auth/options", { cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) throw new Error("无法读取登录配置");
        return response.json() as Promise<AuthOptions>;
      })
      .then((value) => {
        if (!active) return;
        setOptions(value);
        if (value.setup_required && value.setup_enabled) router.replace("/setup");
      })
      .catch(() => active && setError("登录服务暂时不可用"));
    return () => { active = false; };
  }, [router]);

  const displayedError = error || authErrorMessage(searchParams.get("error"));

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      const response = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as ErrorResponse;
        throw new Error(body.error?.message ?? "登录失败，请稍后重试");
      }
      router.replace("/console");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "登录失败，请稍后重试");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="min-h-screen bg-muted/30">
      <AuthHeader />
      <section className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-md items-center px-6 py-12">
        <Card className="w-full">
          <CardHeader>
            <CardTitle className="text-2xl">登录控制台</CardTitle>
            <CardDescription>使用 Novro 账号或企业身份继续。</CardDescription>
          </CardHeader>
          <CardContent>
            {options?.oidc_enabled ? (
              <>
                <Button asChild className="w-full" variant="outline">
                  <a href="/api/auth/oidc/start"><Building2 aria-hidden="true" />使用{options.oidc_display_name}登录</a>
                </Button>
                <div className="my-5 flex items-center gap-3"><Separator className="flex-1" /><span className="text-xs text-muted-foreground">或</span><Separator className="flex-1" /></div>
              </>
            ) : null}
            <form className="space-y-5" onSubmit={submit}>
              <div className="space-y-2"><Label htmlFor="username">用户名</Label><Input autoComplete="username" id="username" onChange={(event) => setUsername(event.target.value)} placeholder="输入用户名" required value={username} /></div>
              <div className="space-y-2"><Label htmlFor="password">密码</Label><Input autoComplete="current-password" id="password" onChange={(event) => setPassword(event.target.value)} placeholder="输入密码" required type="password" value={password} /></div>
              {displayedError ? <p className="text-sm text-destructive" role="alert">{displayedError}</p> : null}
              <Button className="w-full" disabled={submitting} type="submit">{submitting ? "登录中..." : "登录"}<ArrowRight aria-hidden="true" /></Button>
            </form>
            {options?.registration_enabled && !options.setup_required ? <p className="mt-6 text-center text-sm text-muted-foreground">还没有账号？ <Link className="font-medium text-foreground underline-offset-4 hover:underline" href="/register">创建账号</Link></p> : null}
            {options?.setup_required && !options.setup_enabled ? <p className="mt-6 text-sm text-destructive" role="alert">系统尚未初始化。请先由部署管理员配置一次性初始化令牌。</p> : null}
          </CardContent>
        </Card>
      </section>
    </main>
  );
}

function LoginPageShell() {
  return <main className="min-h-screen bg-muted/30"><AuthHeader /></main>;
}

function authErrorMessage(code: string | null) {
  if (code === "oidc_not_provisioned") return "该企业账号尚未获得 Novro 访问权限";
  if (code === "oidc_failed") return "企业账号登录失败，请重试";
  if (code === "setup_required") return "请先初始化管理员账号";
  return "";
}

function AuthHeader() {
  return <header className="flex h-16 items-center justify-between border-b bg-background px-6 lg:px-10"><Link className="flex items-center gap-3" href="/"><span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"><ShieldCheck className="size-4" aria-hidden="true" /></span><span className="text-sm font-semibold">Novro Console</span></Link><ThemeToggle /></header>;
}
