"use client";

import { useEffect, useState } from "react";
import { LogOut, ShieldCheck, UserRound } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { ThemeToggle } from "@/components/theme-toggle";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

type CurrentUser = { username: string; display_name: string; role: "admin" | "member" };

export default function ConsolePage() {
  const router = useRouter();
  const [user, setUser] = useState<CurrentUser | null>(null);

  useEffect(() => {
    void fetch("/api/auth/me", { cache: "no-store" }).then(async (response) => {
      if (response.status === 401) { router.replace("/login"); return null; }
      if (!response.ok) throw new Error();
      return response.json() as Promise<{ user: CurrentUser }>;
    }).then((body) => body && setUser(body.user)).catch(() => router.replace("/login"));
  }, [router]);

  async function logout() { await fetch("/api/auth/logout", { method: "POST" }); router.replace("/login"); }

  return <main className="min-h-screen bg-muted/30"><header className="flex h-16 items-center justify-between border-b bg-background px-6 lg:px-10"><Link className="flex items-center gap-3" href="/"><span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"><ShieldCheck className="size-4" aria-hidden="true" /></span><span className="text-sm font-semibold">Novro Console</span></Link><div className="flex items-center gap-1"><ThemeToggle /><Button aria-label="退出登录" onClick={logout} size="icon" title="退出登录" variant="ghost"><LogOut aria-hidden="true" /></Button></div></header><section className="mx-auto w-full max-w-5xl px-6 py-10 lg:px-10"><div><p className="text-sm text-muted-foreground">控制台</p><h1 className="mt-1 text-2xl font-semibold">账号</h1></div><Card className="mt-6 max-w-xl"><CardHeader><div className="flex items-start justify-between gap-4"><div><CardTitle>{user?.display_name || user?.username || "加载中..."}</CardTitle><CardDescription className="mt-1">{user?.username}</CardDescription></div>{user ? <Badge variant="secondary">{user.role === "admin" ? "管理员" : "成员"}</Badge> : null}</div></CardHeader><CardContent><div className="flex items-start gap-3 border-y py-4"><UserRound className="mt-0.5 size-5" aria-hidden="true" /><div><p className="font-medium">账号已连接</p><p className="mt-1 text-sm leading-6 text-muted-foreground">你可以使用本地账号或企业身份登录 Novro。模型调用功能开放后会出现在此控制台。</p></div></div>{user?.role === "admin" ? <Button asChild className="mt-5" variant="outline"><Link href="/admin/users">管理用户</Link></Button> : null}</CardContent></Card></section></main>;
}
