"use client";

import { BookOpen, Boxes, LayoutDashboard, LogOut, Menu, Route, ShieldCheck, Users } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { createContext, type ReactNode, useContext, useEffect, useState } from "react";

import { ThemeToggle } from "@/components/theme-toggle";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

export type CurrentUser = { id: string; username: string; display_name: string; role: "admin" | "member" };

const CurrentUserContext = createContext<CurrentUser | null>(null);

const routeDetails: Record<string, { title: string; description: string }> = {
  "/console": { title: "账户", description: "余额与 API Key" },
  "/console/docs": { title: "API 文档", description: "接入地址、协议与调用示例" },
  "/console/api-keys": { title: "我的 API Keys", description: "创建和撤销你的访问密钥" },
  "/console/billing": { title: "余额与用量", description: "账户资金流水和最近模型调用" },
  "/admin/users": { title: "用户管理", description: "账号、角色与访问状态" },
  "/admin/api-keys": { title: "API Key 审计", description: "跨用户检索和撤销访问密钥" },
  "/admin/providers": { title: "提供商管理", description: "上游协议、端点和访问凭据" },
  "/admin/models": { title: "模型路由", description: "对外模型名、上游目标和按量价格" },
};

export function isConsoleRoute(pathname: string) {
  return pathname === "/console" || pathname.startsWith("/console/") || pathname === "/admin" || pathname.startsWith("/admin/");
}

export function useCurrentUser() {
  const user = useContext(CurrentUserContext);
  if (!user) throw new Error("useCurrentUser must be used inside ConsoleShell");
  return user;
}

export function useOptionalCurrentUser() {
  return useContext(CurrentUserContext);
}

function ConsoleNavigation({ user, onNavigate }: { user: CurrentUser; onNavigate?: () => void }) {
  const pathname = usePathname();
  const sections = [
    { label: "工作区", items: [
      { href: "/console", label: "账户", icon: LayoutDashboard },
      { href: "/console/docs", label: "API 文档", icon: BookOpen },
    ] },
    { label: "管理", items: user.role === "admin" ? [
      { href: "/admin/users", label: "用户管理", icon: Users },
      { href: "/admin/api-keys", label: "Key 审计", icon: ShieldCheck },
      { href: "/admin/providers", label: "提供商", icon: Boxes },
      { href: "/admin/models", label: "模型路由", icon: Route },
    ] : [] },
  ];

  return (
    <nav aria-label="控制台导航" className="space-y-5 px-3">
      {sections.filter((section) => section.items.length > 0).map((section) => (
        <div key={section.label}>
          <p className="px-3 pb-2 text-xs font-medium text-muted-foreground">{section.label}</p>
          <div className="space-y-1">{section.items.map((item) => {
            const active = pathname === item.href;
            return (
              <Link aria-current={active ? "page" : undefined} className={cn("flex h-9 items-center gap-3 rounded-md px-3 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent", active && "bg-sidebar-accent text-sidebar-accent-foreground")} href={item.href} key={item.href} onClick={onNavigate}>
                <item.icon aria-hidden="true" className="size-4" />{item.label}
              </Link>
            );
          })}</div>
        </div>
      ))}
    </nav>
  );
}

function Brand() {
  return (
    <Link className="flex h-16 items-center gap-3 px-5" href="/console">
      <span className="flex size-8 items-center justify-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground"><ShieldCheck aria-hidden="true" className="size-4" /></span>
      <span><span className="block text-sm font-semibold">Novro</span><span className="block text-xs text-muted-foreground">Gateway Console</span></span>
    </Link>
  );
}

export function ConsoleShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [user, setUser] = useState<CurrentUser | null>(null);
  const [mobileOpen, setMobileOpen] = useState(false);

  useEffect(() => {
    let active = true;
    void fetch("/api/auth/me", { cache: "no-store" })
      .then(async (response) => {
        if (response.status === 401) { router.replace("/login"); return null; }
        if (!response.ok) throw new Error();
        return response.json() as Promise<{ user: CurrentUser }>;
      })
      .then((body) => { if (active && body) setUser(body.user); })
      .catch(() => { if (active) router.replace("/login"); });
    return () => { active = false; };
  }, [router]);

  useEffect(() => {
    if (user && pathname.startsWith("/admin") && user.role !== "admin") router.replace("/console");
  }, [pathname, router, user]);

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST" });
    router.replace("/login");
  }

  if (!user) {
    return <main className="flex min-h-screen items-center justify-center bg-muted/30"><p className="text-sm text-muted-foreground" role="status">正在加载控制台...</p></main>;
  }

  const details = routeDetails[pathname] ?? { title: "Novro", description: "Gateway Console" };

  return (
    <CurrentUserContext.Provider value={user}>
      <div className="min-h-screen bg-muted/30 lg:grid lg:grid-cols-[240px_minmax(0,1fr)]">
        <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 border-r bg-sidebar text-sidebar-foreground lg:flex lg:flex-col">
          <Brand />
          <ConsoleNavigation user={user} />
          <div className="mt-auto border-t p-4">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0"><p className="truncate text-sm font-medium">{user.display_name || user.username}</p><p className="truncate text-xs text-muted-foreground">@{user.username}</p></div>
              <Badge variant="secondary">{user.role === "admin" ? "管理员" : "成员"}</Badge>
            </div>
          </div>
        </aside>

        <div className="min-w-0 lg:col-start-2">
          <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b bg-background/95 px-4 backdrop-blur sm:px-6 lg:px-8">
            <div className="flex min-w-0 items-center gap-3">
              <Sheet onOpenChange={setMobileOpen} open={mobileOpen}>
                <SheetTrigger asChild><Button aria-label="打开导航" className="lg:hidden" size="icon" title="打开导航" variant="ghost"><Menu /></Button></SheetTrigger>
                <SheetContent className="w-[280px] bg-sidebar p-0" side="left">
                  <SheetTitle className="sr-only">控制台导航</SheetTitle>
                  <Brand />
                  <ConsoleNavigation onNavigate={() => setMobileOpen(false)} user={user} />
                </SheetContent>
              </Sheet>
              <div className="min-w-0"><h1 className="truncate text-base font-semibold sm:text-lg">{details.title}</h1><p className="hidden truncate text-xs text-muted-foreground sm:block">{details.description}</p></div>
            </div>
            <div className="flex items-center gap-1">
              <ThemeToggle />
              <Button aria-label="退出登录" onClick={logout} size="icon" title="退出登录" variant="ghost"><LogOut /></Button>
            </div>
          </header>
          <main className="mx-auto w-full max-w-7xl p-4 sm:p-6 lg:p-8">{children}</main>
        </div>
      </div>
    </CurrentUserContext.Provider>
  );
}
