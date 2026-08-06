"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Activity, Coins, RefreshCw, Server, Zap } from "lucide-react";
import { useRouter } from "next/navigation";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

type Usage = { id: string; api_key_id: string; api_key_name: string; model: string; input_tokens: number; output_tokens: number; cost_micros: number; created_at: string };
type Balance = { wallet: { balance_micros: number } };
type Key = { id: string; name: string; status: "active" | "revoked" };

const money = (micros: number) => new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000);
const dateKey = (value: string) => new Intl.DateTimeFormat("zh-CN", { month: "numeric", day: "numeric" }).format(new Date(value));

export default function DashboardPage() {
  const router = useRouter();
  const [usage, setUsage] = useState<Usage[]>([]);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [keys, setKeys] = useState<Key[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true); setError("");
    try {
      const responses = await Promise.all([fetch("/api/account/usage", { cache: "no-store" }), fetch("/api/account/balance", { cache: "no-store" }), fetch("/api/account/api-keys", { cache: "no-store" })]);
      if (responses.some((response) => response.status === 401)) { router.replace("/login"); return; }
      if (responses.some((response) => !response.ok)) throw new Error("数据暂时不可用");
      const [usageBody, balanceBody, keysBody] = await Promise.all(responses.map((response) => response.json()));
      setUsage((usageBody as { usage: Usage[] }).usage);
      setBalance(balanceBody as Balance);
      setKeys((keysBody as { api_keys: Key[] }).api_keys);
    } catch { setError("看板数据加载失败，请稍后重试"); }
    finally { setLoading(false); }
  }, [router]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);

  const totalTokens = usage.reduce((sum, item) => sum + item.input_tokens + item.output_tokens, 0);
  const totalCost = usage.reduce((sum, item) => sum + item.cost_micros, 0);
  const modelTotals = useMemo(() => Object.entries(usage.reduce<Record<string, number>>((result, item) => { result[item.model] = (result[item.model] ?? 0) + 1; return result; }, {})).sort((a, b) => b[1] - a[1]), [usage]);
  const daily = useMemo(() => {
    const result = new Map<string, number>();
    usage.forEach((item) => result.set(dateKey(item.created_at), (result.get(dateKey(item.created_at)) ?? 0) + 1));
    return Array.from(result.entries()).slice(-7);
  }, [usage]);
  const maxDaily = Math.max(1, ...daily.map(([, value]) => value));

  return <div className="space-y-6">
    <div className="flex items-end justify-between gap-4"><div><p className="text-sm text-muted-foreground">最近 50 次调用</p><h2 className="mt-1 text-2xl font-semibold tracking-tight">把使用情况变成可行动的信号</h2></div><Button aria-label="刷新看板" disabled={loading} onClick={() => void load()} size="icon" title="刷新看板" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button></div>
    {error ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{error}</div> : null}
    <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {[{ label: "调用次数", value: usage.length.toLocaleString("zh-CN"), icon: Activity }, { label: "Token 总量", value: totalTokens.toLocaleString("zh-CN"), icon: Zap }, { label: "累计费用", value: money(totalCost), icon: Coins }, { label: "可用密钥", value: `${keys.filter((key) => key.status === "active").length}`, icon: Server }].map((item) => <Card key={item.label}><CardContent className="flex items-start justify-between p-5"><div><p className="text-sm text-muted-foreground">{item.label}</p><p className="mt-2 text-2xl font-semibold">{loading ? "--" : item.value}</p></div><item.icon aria-hidden="true" className="size-5 text-muted-foreground" /></CardContent></Card>)}
    </section>
    <section className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
      <Card><CardHeader className="flex-row items-center justify-between space-y-0"><div><CardTitle className="text-base">调用节奏</CardTitle><p className="mt-1 text-sm text-muted-foreground">最近 7 个有记录的日期</p></div><Badge variant="outline">按请求计</Badge></CardHeader><CardContent><div className="flex h-48 items-end gap-3 border-b px-2 pt-4">{daily.length === 0 ? <p className="m-auto text-sm text-muted-foreground">暂无调用记录</p> : daily.map(([label, value]) => <div className="flex min-w-0 flex-1 flex-col items-center gap-2" key={label}><span className="text-xs text-muted-foreground">{value}</span><div className="w-full max-w-10 rounded-t-sm bg-primary/80" style={{ height: `${Math.max(8, (value / maxDaily) * 130)}px` }} /><span className="text-xs text-muted-foreground">{label}</span></div>)}</div></CardContent></Card>
      <Card><CardHeader><CardTitle className="text-base">模型使用占比</CardTitle><p className="mt-1 text-sm text-muted-foreground">帮助你识别主要流量来源</p></CardHeader><CardContent className="space-y-4">{modelTotals.length === 0 ? <p className="py-12 text-center text-sm text-muted-foreground">暂无模型数据</p> : modelTotals.slice(0, 5).map(([model, count]) => <div key={model}><div className="flex items-center justify-between gap-3 text-sm"><span className="truncate font-mono">{model}</span><span className="text-muted-foreground">{count} 次</span></div><div className="mt-2 h-2 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary" style={{ width: `${(count / (modelTotals[0]?.[1] ?? 1)) * 100}%` }} /></div></div>)}</CardContent></Card>
    </section>
    <Card><CardHeader><CardTitle className="text-base">账户状态</CardTitle></CardHeader><CardContent className="grid gap-3 sm:grid-cols-3"><div><p className="text-xs text-muted-foreground">当前余额</p><p className="mt-1 font-semibold">{balance ? money(balance.wallet.balance_micros) : "--"}</p></div><div><p className="text-xs text-muted-foreground">最近一次调用</p><p className="mt-1 font-semibold">{usage[0] ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(usage[0].created_at)) : "暂无"}</p></div><div><p className="text-xs text-muted-foreground">主力模型</p><p className="mt-1 truncate font-mono font-semibold">{modelTotals[0]?.[0] ?? "暂无"}</p></div></CardContent></Card>
  </div>;
}
