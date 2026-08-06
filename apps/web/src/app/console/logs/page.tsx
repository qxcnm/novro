"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Filter, RefreshCw, Search } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type Usage = { id: string; request_id: string; api_key_id: string; api_key_name: string; model: string; endpoint: string; input_tokens: number; output_tokens: number; cost_micros: number; estimated: boolean; created_at: string };
type Key = { id: string; name: string };
const money = (micros: number) => new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000);
const date = (value: string) => new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));

export default function LogsPage() {
  const router = useRouter();
  const params = useSearchParams();
  const [usage, setUsage] = useState<Usage[]>([]);
  const [keys, setKeys] = useState<Key[]>([]);
  const [model, setModel] = useState("");
  const [keyId, setKeyId] = useState(params.get("key_id") ?? "all");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const load = useCallback(async () => {
    setLoading(true); setMessage("");
    try {
      const [usageResponse, keysResponse] = await Promise.all([fetch("/api/account/usage", { cache: "no-store" }), fetch("/api/account/api-keys", { cache: "no-store" })]);
      if (usageResponse.status === 401 || keysResponse.status === 401) { router.replace("/login"); return; }
      if (!usageResponse.ok || !keysResponse.ok) throw new Error();
      setUsage(((await usageResponse.json()) as { usage: Usage[] }).usage);
      setKeys(((await keysResponse.json()) as { api_keys: Key[] }).api_keys);
    } catch { setMessage("日志加载失败，请稍后重试"); }
    finally { setLoading(false); }
  }, [router]);
  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);
  const models = useMemo(() => Array.from(new Set(usage.map((item) => item.model))).sort(), [usage]);
  const filtered = usage.filter((item) => (keyId === "all" || item.api_key_id === keyId) && (!model || item.model === model) && (!query || item.model.toLowerCase().includes(query.toLowerCase()) || item.request_id.includes(query)));
  const totalCost = filtered.reduce((sum, item) => sum + item.cost_micros, 0);
  const totalTokens = filtered.reduce((sum, item) => sum + item.input_tokens + item.output_tokens, 0);
  return <div className="space-y-5">
    <div className="flex items-end justify-between gap-4"><div><p className="text-sm text-muted-foreground">每次请求的模型、密钥、Tokens 与费用</p><h2 className="mt-1 text-2xl font-semibold tracking-tight">使用日志</h2></div><Button aria-label="刷新使用日志" disabled={loading} onClick={() => void load()} size="icon" title="刷新使用日志" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button></div>
    {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}
    <Card><CardContent className="grid gap-3 p-4 md:grid-cols-[1.2fr_1fr_1fr_auto]"><div className="relative"><Search aria-hidden="true" className="absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input aria-label="搜索模型或请求 ID" className="pl-9" onChange={(event) => setQuery(event.target.value)} placeholder="搜索模型或请求 ID" value={query} /></div><Select onValueChange={setKeyId} value={keyId}><SelectTrigger aria-label="按 API Key 筛选"><SelectValue placeholder="全部 API Key" /></SelectTrigger><SelectContent><SelectItem value="all">全部 API Key</SelectItem>{keys.map((key) => <SelectItem key={key.id} value={key.id}>{key.name}</SelectItem>)}</SelectContent></Select><Select onValueChange={(value) => setModel(value === "all" ? "" : value)} value={model || "all"}><SelectTrigger aria-label="按模型筛选"><SelectValue placeholder="全部模型" /></SelectTrigger><SelectContent><SelectItem value="all">全部模型</SelectItem>{models.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select><Button aria-label="清除筛选" onClick={() => { setKeyId("all"); setModel(""); setQuery(""); }} size="icon" title="清除筛选" variant="outline"><Filter /></Button></CardContent></Card>
    <div className="grid gap-3 sm:grid-cols-2"><Card><CardContent className="p-4"><p className="text-xs text-muted-foreground">当前筛选调用</p><p className="mt-1 text-xl font-semibold">{filtered.length}</p></CardContent></Card><Card><CardContent className="p-4"><p className="text-xs text-muted-foreground">Tokens / 费用</p><p className="mt-1 text-xl font-semibold">{totalTokens.toLocaleString("zh-CN")} <span className="text-sm font-normal text-muted-foreground">· {money(totalCost)}</span></p></CardContent></Card></div>
    <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>时间</TableHead><TableHead>API Key</TableHead><TableHead>模型</TableHead><TableHead>接口</TableHead><TableHead>Tokens</TableHead><TableHead>费用</TableHead></TableRow></TableHeader><TableBody>{loading ? <TableRow><TableCell className="h-28 text-center" colSpan={6}>加载中...</TableCell></TableRow> : null}{!loading && filtered.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={6}>没有符合条件的调用</TableCell></TableRow> : null}{filtered.map((item) => <TableRow key={item.id}><TableCell className="whitespace-nowrap text-muted-foreground">{date(item.created_at)}</TableCell><TableCell className="max-w-36 truncate font-medium">{item.api_key_name || "未命名 Key"}</TableCell><TableCell className="font-mono text-xs">{item.model}</TableCell><TableCell>{item.endpoint.replaceAll("_", " ")}</TableCell><TableCell>{(item.input_tokens + item.output_tokens).toLocaleString("zh-CN")} <span className="text-xs text-muted-foreground">({item.input_tokens}/{item.output_tokens})</span></TableCell><TableCell>{money(item.cost_micros)} {item.estimated ? <Badge className="ml-1" variant="secondary">估算</Badge> : null}</TableCell></TableRow>)}</TableBody></Table></div></CardContent></Card>
  </div>;
}
