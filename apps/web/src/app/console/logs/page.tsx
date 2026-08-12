"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Eye, Filter, RefreshCw, Search } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";

import { DataPagination } from "@/components/data-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type Rates = { input_price_micros: number; output_price_micros: number; cache_read_price_micros: number; cache_write_price_micros: number; cache_write_1h_price_micros: number; request_price_micros: number };
type Usage = { id: string; request_id: string; status_code: number; error_code?: string; error_message?: string; duration_ms: number; api_key_id: string; api_key_name: string; model: string; upstream_model_name: string; endpoint: string; input_tokens: number; uncached_input_tokens: number; cache_read_input_tokens: number; cache_write_input_tokens: number; cache_write_1h_input_tokens: number; output_tokens: number; rates: Rates; base_cost_micros: number; multiplier_bps: number; cost_micros: number; reserved_micros: number; billing_group_code: string; billing_group_name: string; calculation_version: string; estimated: boolean; upstream_request_id?: string; created_at: string; finished_at: string };
type UsagePage = { usage: Usage[]; models: string[]; total: number; offset: number; limit: number; total_tokens: number; total_cost_micros: number };
type Key = { id: string; name: string };

const PAGE_SIZE = 20;
const money = (micros: number) => new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000);
const date = (value: string) => new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value));
const duration = (milliseconds: number) => milliseconds < 1000 ? `${milliseconds}ms` : `${(milliseconds / 1000).toFixed(milliseconds >= 10_000 ? 0 : 1)}s`;

function Metric({ label, value }: { label: string; value: string }) {
  return <div className="border-b py-3"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 break-all text-sm font-medium">{value}</p></div>;
}

export default function LogsPage() {
  const router = useRouter();
  const params = useSearchParams();
  const [page, setPage] = useState<UsagePage>({ usage: [], models: [], total: 0, offset: 0, limit: PAGE_SIZE, total_tokens: 0, total_cost_micros: 0 });
  const [keys, setKeys] = useState<Key[]>([]);
  const [keyFilterAvailable, setKeyFilterAvailable] = useState(true);
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState(PAGE_SIZE);
  const [model, setModel] = useState("");
  const [keyId, setKeyId] = useState(params.get("key_id") ?? "all");
  const [status, setStatus] = useState("all");
  const [timeRange, setTimeRange] = useState("all");
  const [queryDraft, setQueryDraft] = useState("");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [selected, setSelected] = useState<Usage | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setMessage("");
    try {
      const usageQuery = new URLSearchParams({ offset: String(offset), limit: String(pageSize) });
      if (keyId !== "all") usageQuery.set("api_key_id", keyId);
      if (model) usageQuery.set("model", model);
      if (status !== "all") usageQuery.set("status", status);
      if (query) usageQuery.set("search", query);
      if (timeRange !== "all") usageQuery.set("from", new Date(Date.now() - Number(timeRange) * 60 * 60 * 1000).toISOString());
      const [usageResponse, keysResponse] = await Promise.all([fetch(`/api/account/usage?${usageQuery}`, { cache: "no-store" }), fetch("/api/account/api-keys", { cache: "no-store" })]);
      if (usageResponse.status === 401 || keysResponse.status === 401) { router.replace("/login"); return; }
      if (!usageResponse.ok) throw new Error();
      setPage((await usageResponse.json()) as UsagePage);
      if (keysResponse.ok) {
        setKeys(((await keysResponse.json()) as { api_keys: Key[] }).api_keys);
        setKeyFilterAvailable(true);
      } else {
        setKeys([]);
        setKeyFilterAvailable(false);
        setMessage("使用记录已加载，API Key 筛选暂时不可用");
      }
    } catch { setMessage("日志加载失败，请稍后重试"); }
    finally { setLoading(false); }
  }, [keyId, model, offset, pageSize, query, router, status, timeRange]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setOffset(0);
    setQuery(queryDraft.trim());
  }

  function clearFilters() {
    setOffset(0); setKeyId("all"); setModel(""); setStatus("all"); setTimeRange("all"); setQueryDraft(""); setQuery("");
  }

  const models = useMemo(() => page.models, [page.models]);
  return <div className="space-y-5"><div className="flex items-end justify-between gap-4"><div><p className="text-sm text-muted-foreground">成功和失败请求都会保留，失败请求不会计入费用</p><h2 className="mt-1 text-2xl font-semibold">使用日志</h2></div><Button aria-label="刷新使用日志" disabled={loading} onClick={() => void load()} size="icon" title="刷新使用日志" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button></div>{message ? <div className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}<Card><CardContent className="grid gap-3 p-4 md:grid-cols-2 xl:grid-cols-[1.2fr_1fr_1fr_1fr_1fr_auto]"><form className="relative" onSubmit={submitSearch}><Search className="absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input aria-label="搜索模型或请求 ID" className="pl-9" onChange={(event) => setQueryDraft(event.target.value)} placeholder="搜索模型或请求 ID" value={queryDraft} /></form><Select disabled={!keyFilterAvailable} onValueChange={(value) => { setOffset(0); setKeyId(value); }} value={keyId}><SelectTrigger aria-label="按 API Key 筛选"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部 API Key</SelectItem>{keys.map((key) => <SelectItem key={key.id} value={key.id}>{key.name}</SelectItem>)}</SelectContent></Select><Select onValueChange={(value) => { setOffset(0); setModel(value === "all" ? "" : value); }} value={model || "all"}><SelectTrigger aria-label="按模型筛选"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部模型</SelectItem>{models.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select><Select onValueChange={(value) => { setOffset(0); setStatus(value); }} value={status}><SelectTrigger aria-label="按请求状态筛选"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="success">成功</SelectItem><SelectItem value="failed">失败</SelectItem></SelectContent></Select><Select onValueChange={(value) => { setOffset(0); setTimeRange(value); }} value={timeRange}><SelectTrigger aria-label="按时间范围筛选"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部时间</SelectItem><SelectItem value="24">最近 24 小时</SelectItem><SelectItem value="168">最近 7 天</SelectItem><SelectItem value="720">最近 30 天</SelectItem></SelectContent></Select><Button aria-label="清除筛选" onClick={clearFilters} size="icon" title="清除筛选" type="button" variant="outline"><Filter /></Button></CardContent></Card><section className="grid border-y bg-background sm:grid-cols-2"><div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">当前筛选调用</p><p className="mt-1 text-xl font-semibold">{page.total.toLocaleString("zh-CN")}</p></div><div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">筛选 Tokens / 费用</p><p className="mt-1 text-xl font-semibold">{page.total_tokens.toLocaleString("zh-CN")} <span className="text-sm font-normal text-muted-foreground">· {money(page.total_cost_micros)}</span></p></div></section><Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>时间</TableHead><TableHead>API Key</TableHead><TableHead>模型</TableHead><TableHead>状态</TableHead><TableHead>用时</TableHead><TableHead>Token 明细</TableHead><TableHead>费用</TableHead><TableHead>错误</TableHead><TableHead /></TableRow></TableHeader><TableBody>{loading ? <TableRow><TableCell className="h-28 text-center" colSpan={9}>加载中...</TableCell></TableRow> : null}{!loading && page.usage.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={9}>没有符合条件的调用</TableCell></TableRow> : null}{page.usage.map((item) => <TableRow key={item.id}><TableCell className="whitespace-nowrap text-muted-foreground">{date(item.created_at)}</TableCell><TableCell>{item.api_key_name || "未命名 Key"}</TableCell><TableCell><p className="font-mono text-xs">{item.model}</p><p className="text-xs text-muted-foreground">{item.endpoint.replaceAll("_", " ")}</p></TableCell><TableCell><Badge variant={item.status_code >= 400 ? "destructive" : "secondary"}>{item.status_code}</Badge></TableCell><TableCell className="whitespace-nowrap text-muted-foreground">{duration(item.duration_ms)}</TableCell><TableCell>{item.status_code >= 400 ? <span className="text-muted-foreground">-</span> : <><p>{(item.input_tokens + item.output_tokens).toLocaleString("zh-CN")}</p><p className="text-xs text-muted-foreground">普通 {item.uncached_input_tokens} · 命中 {item.cache_read_input_tokens} · 创建 {item.cache_write_input_tokens + item.cache_write_1h_input_tokens} · 输出 {item.output_tokens}</p></>}</TableCell><TableCell>{item.status_code >= 400 ? <span className="text-muted-foreground">-</span> : <>{money(item.cost_micros)} {item.estimated ? <Badge className="ml-1" variant="secondary">usage 不完整</Badge> : null}</>}</TableCell><TableCell className="max-w-64"><p className="truncate text-sm text-destructive">{item.error_message || "-"}</p></TableCell><TableCell><Button aria-label="查看请求明细" onClick={() => setSelected(item)} size="icon-sm" title="查看请求明细" variant="ghost"><Eye /></Button></TableCell></TableRow>)}</TableBody></Table></div><DataPagination loading={loading} offset={offset} onOffsetChange={setOffset} onPageSizeChange={(size) => { setOffset(0); setPageSize(size); }} pageSize={pageSize} total={page.total} /></CardContent></Card><Sheet onOpenChange={(open) => { if (!open) setSelected(null); }} open={selected !== null}><SheetContent className="w-full overflow-y-auto sm:max-w-lg" side="right"><SheetHeader className="border-b px-6 py-5"><SheetTitle>{selected && selected.status_code >= 400 ? "失败请求明细" : "计费明细"}</SheetTitle></SheetHeader>{selected ? <div className="px-6 pb-8"><Metric label="请求 ID" value={selected.request_id} /><Metric label="状态" value={`${selected.status_code} · ${selected.status_code >= 400 ? "失败" : "成功"}`} /><Metric label="用时" value={duration(selected.duration_ms)} />{selected.error_message ? <Metric label="错误" value={`${selected.error_code || "request_failed"} · ${selected.error_message}`} /> : null}<Metric label="模型映射" value={`${selected.model} → ${selected.upstream_model_name || "-"}`} /><div className="grid grid-cols-2 gap-x-4"><Metric label="普通输入" value={`${selected.uncached_input_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.input_price_micros)}/1M`} /><Metric label="缓存命中" value={`${selected.cache_read_input_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.cache_read_price_micros)}/1M`} /><Metric label="缓存创建（5 分钟）" value={`${selected.cache_write_input_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.cache_write_price_micros)}/1M`} /><Metric label="缓存创建（1 小时）" value={`${selected.cache_write_1h_input_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.cache_write_1h_price_micros)}/1M`} /><Metric label="输出" value={`${selected.output_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.output_price_micros)}/1M`} /><Metric label="请求固定费" value={money(selected.rates.request_price_micros)} /></div><Metric label="基础成本" value={money(selected.base_cost_micros)} /><Metric label="请求时计费分组" value={`${selected.billing_group_name || "默认分组"} ${selected.billing_group_code ? `(${selected.billing_group_code})` : ""} · ${(selected.multiplier_bps / 10_000).toFixed(4)}×`} /><Metric label="预占金额" value={money(selected.reserved_micros)} /><Metric label="最终扣费" value={selected.status_code >= 400 ? "未扣费" : money(selected.cost_micros)} /><Metric label="算法版本" value={selected.calculation_version || "-"} /><Metric label="完成时间" value={date(selected.finished_at)} />{selected.estimated ? <p className="mt-4 border-y py-3 text-sm text-amber-700 dark:text-amber-300">上游未返回完整 usage，本笔记录仅按已确认的 Token 计费，未确认部分已释放预占。</p> : null}</div> : null}</SheetContent></Sheet></div>;
}
