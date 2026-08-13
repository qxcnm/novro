"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Activity, Coins, RefreshCw, Server, Zap } from "lucide-react";
import { useRouter } from "next/navigation";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { todayUsageURL } from "@/lib/dashboard-usage";

type Usage = { id: string; api_key_id: string; api_key_name: string; model: string; input_tokens: number; output_tokens: number; cost_micros: number; created_at: string };
type UsagePage = { usage: Usage[]; total: number; total_tokens: number; total_cost_micros: number };
type Balance = { wallet: { balance_micros: number } };
type Key = { id: string; name: string; status: "active" | "revoked" };
type UsageRate = { window_seconds: number; requests: number; input_tokens: number; output_tokens: number; total_tokens: number; rpm: number; tpm: number; calculated_at: string };

/**
 * money 封装该名称对应的业务处理逻辑。
 * @param micros 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
const money = (micros: number) => new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000);
/**
 * dateKey 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
const dateKey = (value: string) => new Intl.DateTimeFormat("zh-CN", { month: "numeric", day: "numeric" }).format(new Date(value));

/**
 * DashboardPage 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function DashboardPage() {
  const router = useRouter();
  const [usage, setUsage] = useState<Usage[]>([]);
  const [todayUsage, setTodayUsage] = useState<UsagePage | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [keys, setKeys] = useState<Key[]>([]);
  const [rate, setRate] = useState<UsageRate | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refreshLiveUsage = useCallback(async () => {
    try {
      const [todayResponse, rateResponse] = await Promise.all([fetch(todayUsageURL(), { cache: "no-store" }), fetch("/api/account/usage/rate", { cache: "no-store" })]);
      if (todayResponse.status === 401 || rateResponse.status === 401) { router.replace("/login"); return; }
      if (todayResponse.ok) setTodayUsage((await todayResponse.json()) as UsagePage);
      if (rateResponse.ok) { setRate((await rateResponse.json()) as UsageRate); return; }
      setRate(null);
    } catch { setRate(null); }
  }, [router]);

  const load = useCallback(async () => {
    setLoading(true); setError("");
    try {
      const responses = await Promise.all([fetch("/api/account/usage?limit=50", { cache: "no-store" }), fetch(todayUsageURL(), { cache: "no-store" }), fetch("/api/account/balance", { cache: "no-store" }), fetch("/api/account/api-keys", { cache: "no-store" }), fetch("/api/account/usage/rate", { cache: "no-store" })]);
      if (responses.some((response) => response.status === 401)) { router.replace("/login"); return; }
      if (responses.some((response) => !response.ok)) throw new Error("数据暂时不可用");
      const [usageBody, todayUsageBody, balanceBody, keysBody, rateBody] = await Promise.all(responses.map((response) => response.json()));
      setUsage((usageBody as { usage: Usage[] }).usage);
      setTodayUsage(todayUsageBody as UsagePage);
      setBalance(balanceBody as Balance);
      setKeys((keysBody as { api_keys: Key[] }).api_keys);
      setRate(rateBody as UsageRate);
    } catch { setError("看板数据加载失败，请稍后重试"); }
    finally { setLoading(false); }
  }, [router]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);

  useEffect(() => {
    let timer: number;
    /**
     * schedule 封装该名称对应的业务处理逻辑。
     * @param none 无参数。
     * @author Gao Hongshun
     * @date 2026-08-13
     */
    const schedule = () => {
      const now = new Date();
      const nextMidnight = new Date(now);
      nextMidnight.setHours(24, 0, 0, 0);
      timer = window.setTimeout(() => { void load(); schedule(); }, nextMidnight.getTime() - now.getTime());
    };
    schedule();
    return () => window.clearTimeout(timer);
  }, [load]);

  useEffect(() => {
    const timer = window.setInterval(() => void refreshLiveUsage(), 15_000);
    return () => window.clearInterval(timer);
  }, [refreshLiveUsage]);

  const modelTotals = useMemo(() => Object.entries(usage.reduce<Record<string, number>>((result, item) => { result[item.model] = (result[item.model] ?? 0) + 1; return result; }, {})).sort((a, b) => b[1] - a[1]), [usage]);
  const daily = useMemo(() => {
    const result = new Map<string, number>();
    usage.forEach((item) => result.set(dateKey(item.created_at), (result.get(dateKey(item.created_at)) ?? 0) + 1));
    return Array.from(result.entries()).slice(-7);
  }, [usage]);
  const maxDaily = Math.max(1, ...daily.map(([, value]) => value));

  return <div className="space-y-6">
    <div className="flex items-center justify-between gap-4"><p className="text-sm text-muted-foreground">今日 00:00 至今</p><Button aria-label="刷新看板" disabled={loading} onClick={() => void load()} size="icon" title="刷新看板" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button></div>
    {error ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{error}</div> : null}
    <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {[{ label: "实时 RPM", detail: "最近 60 秒", value: rate?.rpm.toLocaleString("zh-CN") ?? "--", icon: Activity }, { label: "实时 TPM", detail: "最近 60 秒", value: rate?.tpm.toLocaleString("zh-CN") ?? "--", icon: Zap }, { label: "调用次数", detail: "今日 00:00 至今", value: todayUsage?.total.toLocaleString("zh-CN") ?? "--", icon: Activity }, { label: "Token 总量", detail: "今日 00:00 至今", value: todayUsage?.total_tokens.toLocaleString("zh-CN") ?? "--", icon: Zap }, { label: "累计费用", detail: "今日 00:00 至今", value: todayUsage ? money(todayUsage.total_cost_micros) : "--", icon: Coins }, { label: "可用密钥", detail: "当前账户", value: `${keys.filter((key) => key.status === "active").length}`, icon: Server }].map((item) => <Card key={item.label}><CardContent className="flex min-h-28 items-start justify-between p-5"><div><p className="text-sm text-muted-foreground">{item.label}</p><p className="mt-2 text-2xl font-semibold">{loading ? "--" : item.value}</p><p className="mt-1 text-xs text-muted-foreground">{item.detail}</p></div><item.icon aria-hidden="true" className="size-5 text-muted-foreground" /></CardContent></Card>)}
    </section>
    <section className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
      <Card><CardHeader className="flex-row items-center justify-between space-y-0"><div><CardTitle className="text-base">调用节奏</CardTitle><p className="mt-1 text-sm text-muted-foreground">最近 7 个有记录的日期</p></div><Badge variant="outline">按请求计</Badge></CardHeader><CardContent><div className="flex h-48 items-end gap-3 border-b px-2 pt-4">{daily.length === 0 ? <p className="m-auto text-sm text-muted-foreground">暂无调用记录</p> : daily.map(([label, value]) => <div className="flex min-w-0 flex-1 flex-col items-center gap-2" key={label}><span className="text-xs text-muted-foreground">{value}</span><div className="w-full max-w-10 rounded-t-sm bg-primary/80" style={{ height: `${Math.max(8, (value / maxDaily) * 130)}px` }} /><span className="text-xs text-muted-foreground">{label}</span></div>)}</div></CardContent></Card>
      <Card><CardHeader><CardTitle className="text-base">模型使用占比</CardTitle><p className="mt-1 text-sm text-muted-foreground">帮助你识别主要流量来源</p></CardHeader><CardContent className="space-y-4">{modelTotals.length === 0 ? <p className="py-12 text-center text-sm text-muted-foreground">暂无模型数据</p> : modelTotals.slice(0, 5).map(([model, count]) => <div key={model}><div className="flex items-center justify-between gap-3 text-sm"><span className="truncate font-mono">{model}</span><span className="text-muted-foreground">{count} 次</span></div><div className="mt-2 h-2 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary" style={{ width: `${(count / (modelTotals[0]?.[1] ?? 1)) * 100}%` }} /></div></div>)}</CardContent></Card>
    </section>
    <Card><CardHeader><CardTitle className="text-base">账户状态</CardTitle></CardHeader><CardContent className="grid gap-3 sm:grid-cols-3"><div><p className="text-xs text-muted-foreground">当前余额</p><p className="mt-1 font-semibold">{balance ? money(balance.wallet.balance_micros) : "--"}</p></div><div><p className="text-xs text-muted-foreground">最近一次调用</p><p className="mt-1 font-semibold">{usage[0] ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(usage[0].created_at)) : "暂无"}</p></div><div><p className="text-xs text-muted-foreground">主力模型</p><p className="mt-1 truncate font-mono font-semibold">{modelTotals[0]?.[0] ?? "暂无"}</p></div></CardContent></Card>
  </div>;
}
