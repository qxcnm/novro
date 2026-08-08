"use client";

import { BookOpen, Bot, Boxes, ChartNoAxesCombined, CreditCard, House, KeyRound, LayoutDashboard, LogOut, Mail, Menu, Network, Percent, RefreshCw, ScrollText, ShieldCheck, UserRound, Users, UsersRound, WalletCards } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { createContext, type ReactNode, useContext, useEffect, useState } from "react";

import { ThemeToggle } from "@/components/theme-toggle";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { checkCurrentSession, type CurrentUser } from "@/lib/auth-session";
import { cn } from "@/lib/utils";

export type { CurrentUser } from "@/lib/auth-session";

const CurrentUserContext = createContext<CurrentUser | null>(null);

const routeDetails: Record<string, { title: string; description: string }> = {
  "/console": { title: "概览", description: "账户健康与近期活动" },
  "/console/dashboard": { title: "数据看板", description: "调用趋势、模型分布与成本" },
  "/console/logs": { title: "使用日志", description: "按 API Key 追踪每次调用" },
  "/console/models": { title: "可用模型", description: "当前可调用模型与个人结算单价" },
  "/console/profile": { title: "个人资料", description: "账号信息与偏好" },
  "/console/docs": { title: "API 文档", description: "接入地址、协议与调用示例" },
  "/console/api-keys": { title: "我的 API Keys", description: "创建和撤销你的访问密钥" },
  "/console/billing": { title: "余额与用量", description: "账户资金流水和最近模型调用" },
  "/admin/users": { title: "用户管理", description: "账号、角色与访问状态" },
  "/admin/api-keys": { title: "API Key 审计", description: "跨用户检索和撤销访问密钥" },
  "/admin/providers": { title: "提供商与路由", description: "提供商配置、模型同步与关联路由" },
  "/admin/upstream-models": { title: "模型目录", description: "模型标识与各计费维度的基础价格" },
  "/admin/billing-groups": { title: "计费分组", description: "用户分组与结算倍率" },
  "/admin/payments": { title: "支付配置", description: "易支付商户信息与充值渠道" },
  "/admin/referral": { title: "推荐设置", description: "邀请返现比例与生效规则" },
  "/admin/email": { title: "邮件设置", description: "注册验证码的 SMTP 发送配置" },
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
      ...(user.role === "admin" ? [{ href: "/console", label: "概览", icon: LayoutDashboard }] : []),
      { href: "/console/dashboard", label: "数据看板", icon: ChartNoAxesCombined },
      { href: "/console/logs", label: "使用日志", icon: ScrollText },
      { href: "/console/models", label: "可用模型", icon: Bot },
      { href: "/console/api-keys", label: "API 密钥", icon: KeyRound },
      { href: "/console/docs", label: "API 文档", icon: BookOpen },
    ] },
    { label: "个人", items: [
      { href: "/console/profile", label: "个人资料", icon: UserRound },
      { href: "/console/billing", label: "余额与流水", icon: WalletCards },
    ] },
    { label: "管理", items: user.role === "admin" ? [
      { href: "/admin/users", label: "用户管理", icon: Users },
      { href: "/admin/api-keys", label: "Key 审计", icon: ShieldCheck },
      { href: "/admin/providers", label: "提供商与路由", icon: Boxes },
      { href: "/admin/upstream-models", label: "模型目录", icon: Network },
      { href: "/admin/billing-groups", label: "计费分组", icon: UsersRound },
      { href: "/admin/referral", label: "推荐设置", icon: Percent },
      { href: "/admin/payments", label: "支付配置", icon: CreditCard },
      { href: "/admin/email", label: "邮件设置", icon: Mail },
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
    <Link className="group flex h-16 items-center gap-3 px-5" href="/" title="返回主页">
      <span className="flex size-8 items-center justify-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground"><ShieldCheck aria-hidden="true" className="size-4" /></span>
      <span className="min-w-0"><span className="block text-sm font-semibold">Novro</span><span className="block truncate text-xs text-muted-foreground">Gateway Console · 返回主页</span></span>
    </Link>
  );
}

export function ConsoleShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [user, setUser] = useState<CurrentUser | null>(null);
  const [sessionUnavailable, setSessionUnavailable] = useState(false);
  const [sessionAttempt, setSessionAttempt] = useState(0);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);

  useEffect(() => {
    let active = true;
    void checkCurrentSession().then((result) => {
      if (!active) return;
      if (result.status === "unauthenticated") {
        router.replace("/login");
        return;
      }
      if (result.status === "unavailable") {
        setSessionUnavailable(true);
        return;
      }
      setUser(result.user);
    });
    return () => { active = false; };
  }, [router, sessionAttempt]);

  useEffect(() => {
    if (!user || user.role === "admin") return;
    const memberRoutes = ["/console/dashboard", "/console/logs", "/console/models", "/console/api-keys", "/console/docs", "/console/profile", "/console/billing"];
    if (!memberRoutes.includes(pathname)) router.replace("/console/dashboard");
  }, [pathname, router, user]);

  async function logout() {
    if (loggingOut) return;
    setLoggingOut(true);
    try {
      await fetch("/api/auth/logout", { method: "POST", credentials: "same-origin" });
    } finally {
      setUser(null);
      window.location.replace("/login");
    }
  }

  function retrySessionCheck() {
    setSessionUnavailable(false);
    setUser(null);
    setSessionAttempt((value) => value + 1);
  }

  if (sessionUnavailable) {
    return <main className="flex min-h-screen items-center justify-center bg-muted/30 px-6"><div className="text-center"><p className="text-sm font-medium">登录状态检查失败</p><p className="mt-1 text-sm text-muted-foreground">服务暂时不可用，请稍后重试。</p><Button className="mt-4" onClick={retrySessionCheck} variant="outline"><RefreshCw />重新检查</Button></div></main>;
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
              <div className="min-w-0"><p className="truncate text-sm font-medium">{user.display_name || user.username}</p><p className="truncate text-xs text-muted-foreground">@{user.username}{user.billing_group ? ` · ${user.billing_group.display_name}` : ""}</p></div>
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
              <Button asChild className="gap-2" size="sm" title="返回主页" variant="ghost">
                <Link href="/"><House aria-hidden="true" /><span className="hidden sm:inline">返回主页</span></Link>
              </Button>
              <ThemeToggle />
              <Button aria-label={loggingOut ? "正在退出登录" : "退出登录"} disabled={loggingOut} onClick={logout} size="icon" title={loggingOut ? "正在退出..." : "退出登录"} variant="ghost"><LogOut /></Button>
            </div>
          </header>
          <main className="w-full p-4 sm:p-6 lg:p-8">{children}</main>
        </div>
      </div>
    </CurrentUserContext.Provider>
  );
}
