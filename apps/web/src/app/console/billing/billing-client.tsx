"use client";

import { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type WalletEntry = { id: string; reference_id: string; entry_type: "manual_adjustment" | "usage_reservation" | "usage_refund"; amount_micros: number; balance_after_micros: number; description: string; created_at: string };
type BalanceSummary = { wallet: { id: string; user_id: string; balance_micros: number; updated_at: string }; entries: WalletEntry[] };
type Usage = { id: string; request_id: string; model: string; endpoint: string; input_tokens: number; output_tokens: number; cost_micros: number; estimated: boolean; created_at: string };
type ErrorResponse = { error?: { message?: string } };

function formatMoney(micros: number) { return new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000); }
function formatDate(value: string) { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)); }
function entryLabel(type: WalletEntry["entry_type"]) { if (type === "manual_adjustment") return "人工调整"; if (type === "usage_reservation") return "调用预占"; return "预占释放"; }
async function readError(response: Response) { const body = (await response.json().catch(() => ({}))) as ErrorResponse; return body.error?.message ?? "加载失败，请稍后重试"; }

export default function BillingClient() {
  const router = useRouter();
  const [summary, setSummary] = useState<BalanceSummary | null>(null);
  const [usage, setUsage] = useState<Usage[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true); setMessage("");
    const [balanceResponse, usageResponse] = await Promise.all([fetch("/api/account/balance", { cache: "no-store" }), fetch("/api/account/usage", { cache: "no-store" })]);
    if (balanceResponse.status === 401 || usageResponse.status === 401) { router.replace("/login"); return; }
    if (!balanceResponse.ok) { setMessage(await readError(balanceResponse)); setLoading(false); return; }
    if (!usageResponse.ok) { setMessage(await readError(usageResponse)); setLoading(false); return; }
    setSummary((await balanceResponse.json()) as BalanceSummary);
    setUsage(((await usageResponse.json()) as { usage: Usage[] }).usage);
    setLoading(false);
  }, [router]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);

  const totalCost = usage.reduce((sum, item) => sum + item.cost_micros, 0);
  const totalTokens = usage.reduce((sum, item) => sum + item.input_tokens + item.output_tokens, 0);

  return (
    <>
      <div className="space-y-5">
        <div className="flex justify-end"><Button aria-label="刷新余额与用量" disabled={loading} onClick={() => void load()} size="icon" title="刷新余额与用量" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button></div>
        <section className="grid border-y bg-background sm:grid-cols-3">
          <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">可用余额</p><p className="mt-1 text-xl font-semibold">{summary ? formatMoney(summary.wallet.balance_micros) : "--"}</p></div>
          <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">最近调用 Tokens</p><p className="mt-1 text-xl font-semibold">{totalTokens.toLocaleString("zh-CN")}</p></div>
          <div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">最近调用费用</p><p className="mt-1 text-xl font-semibold">{formatMoney(totalCost)}</p></div>
        </section>
        {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}

        <section><h2 className="mb-3 text-sm font-semibold">最近调用</h2><Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>模型</TableHead><TableHead>接口</TableHead><TableHead>输入 Tokens</TableHead><TableHead>输出 Tokens</TableHead><TableHead>费用</TableHead><TableHead>时间</TableHead></TableRow></TableHeader><TableBody>
          {loading ? <TableRow><TableCell className="h-24 text-center" colSpan={6}>加载中...</TableCell></TableRow> : null}
          {!loading && usage.length === 0 ? <TableRow><TableCell className="h-24 text-center text-muted-foreground" colSpan={6}>还没有调用记录</TableCell></TableRow> : null}
          {!loading ? usage.map((item) => <TableRow key={item.id}><TableCell className="font-mono text-xs">{item.model}</TableCell><TableCell>{item.endpoint.replaceAll("_", " ")}</TableCell><TableCell>{item.input_tokens.toLocaleString("zh-CN")}</TableCell><TableCell>{item.output_tokens.toLocaleString("zh-CN")}</TableCell><TableCell><span>{formatMoney(item.cost_micros)}</span>{item.estimated ? <Badge className="ml-2" variant="secondary">估算</Badge> : null}</TableCell><TableCell className="text-muted-foreground">{formatDate(item.created_at)}</TableCell></TableRow>) : null}
        </TableBody></Table></div></CardContent></Card></section>

        <section><h2 className="mb-3 text-sm font-semibold">余额流水</h2><Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>类型</TableHead><TableHead>说明</TableHead><TableHead>变动</TableHead><TableHead>变动后余额</TableHead><TableHead>时间</TableHead></TableRow></TableHeader><TableBody>
          {!loading && (summary?.entries.length ?? 0) === 0 ? <TableRow><TableCell className="h-24 text-center text-muted-foreground" colSpan={5}>还没有余额流水</TableCell></TableRow> : null}
          {summary?.entries.map((entry) => <TableRow key={entry.id}><TableCell><Badge variant="secondary">{entryLabel(entry.entry_type)}</Badge></TableCell><TableCell>{entry.description}</TableCell><TableCell className={entry.amount_micros >= 0 ? "text-emerald-600 dark:text-emerald-400" : "text-foreground"}>{entry.amount_micros >= 0 ? "+" : ""}{formatMoney(entry.amount_micros)}</TableCell><TableCell>{formatMoney(entry.balance_after_micros)}</TableCell><TableCell className="text-muted-foreground">{formatDate(entry.created_at)}</TableCell></TableRow>)}
        </TableBody></Table></div></CardContent></Card></section>
      </div>
    </>
  );
}
