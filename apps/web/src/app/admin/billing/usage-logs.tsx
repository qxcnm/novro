"use client";

import { type FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Eye, Filter, RefreshCw, Search } from "lucide-react";
import { useRouter } from "next/navigation";

import { DataPagination } from "@/components/data-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type Rates = { input_price_micros: number; output_price_micros: number; cache_read_price_micros: number; cache_write_price_micros: number; cache_write_1h_price_micros: number; request_price_micros: number };
type Usage = { id: string; user_id: string; username: string; user_display_name: string; request_id: string; status_code: number; error_code?: string; error_message?: string; duration_ms: number; api_key_name: string; model: string; upstream_model_name: string; endpoint: string; input_tokens: number; uncached_input_tokens: number; cache_read_input_tokens: number; cache_write_input_tokens: number; cache_write_1h_input_tokens: number; output_tokens: number; rates: Rates; base_cost_micros: number; multiplier_bps: number; cost_micros: number; reserved_micros: number; billing_group_code: string; billing_group_name: string; calculation_version: string; created_at: string; finished_at: string };
type UsagePage = { usage: Usage[]; models: string[]; total: number; offset: number; limit: number; total_tokens: number; total_cost_micros: number };
type UserRecord = { id: string; username: string; display_name: string; status: "active" | "disabled" };
type UserPage = { users: UserRecord[]; total: number };

const PAGE_SIZE = 20;
const emptyPage: UsagePage = { usage: [], models: [], total: 0, offset: 0, limit: PAGE_SIZE, total_tokens: 0, total_cost_micros: 0 };
const money = (micros: number) => new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000);
const date = (value: string) => new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value));
const duration = (milliseconds: number) => milliseconds < 1000 ? `${milliseconds}ms` : `${(milliseconds / 1000).toFixed(milliseconds >= 10_000 ? 0 : 1)}s`;

function Metric({ label, value }: { label: string; value: string }) {
  return <div className="border-b py-3"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 break-all text-sm font-medium">{value}</p></div>;
}

async function readAllUsers(): Promise<UserRecord[]> {
  const firstResponse = await fetch("/api/admin/users?offset=0&limit=100", { cache: "no-store" });
  if (!firstResponse.ok) throw firstResponse;
  const first = (await firstResponse.json()) as UserPage;
  const pages = [first.users];
  const requests: Promise<Response>[] = [];
  for (let offset = 100; offset < first.total; offset += 100) {
    requests.push(fetch(`/api/admin/users?offset=${offset}&limit=100`, { cache: "no-store" }));
  }
  for (const response of await Promise.all(requests)) {
    if (!response.ok) throw response;
    pages.push(((await response.json()) as UserPage).users);
  }
  return pages.flat().sort((a, b) => a.username.localeCompare(b.username));
}

export default function AdminUsageLogs() {
  const router = useRouter();
  const [page, setPage] = useState<UsagePage>(emptyPage);
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [userID, setUserID] = useState("all");
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState(PAGE_SIZE);
  const [model, setModel] = useState("");
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
      const search = new URLSearchParams({ offset: String(offset), limit: String(pageSize) });
      if (userID !== "all") search.set("user_id", userID);
      if (model) search.set("model", model);
      if (status !== "all") search.set("status", status);
      if (query) search.set("search", query);
      if (timeRange !== "all") search.set("from", new Date(Date.now() - Number(timeRange) * 60 * 60 * 1000).toISOString());
      const response = await fetch(`/api/admin/usage?${search}`, { cache: "no-store" });
      if (response.status === 401) { router.replace("/login"); return; }
      if (response.status === 403) { router.replace("/console"); return; }
      if (!response.ok) throw new Error();
      setPage((await response.json()) as UsagePage);
    } catch {
      setMessage("日志加载失败，请稍后重试");
    } finally {
      setLoading(false);
    }
  }, [model, offset, pageSize, query, router, status, timeRange, userID]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);
  useEffect(() => {
    let active = true;
    void readAllUsers().then((records) => { if (active) setUsers(records); }).catch((error: unknown) => {
      if (!active) return;
      if (error instanceof Response && error.status === 401) router.replace("/login");
      else if (error instanceof Response && error.status === 403) router.replace("/console");
      else setMessage("日志已加载，用户筛选暂时不可用");
    });
    return () => { active = false; };
  }, [router]);

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setOffset(0);
    setQuery(queryDraft.trim());
  }

  function clearFilters() {
    setOffset(0); setUserID("all"); setModel(""); setStatus("all"); setTimeRange("all"); setQueryDraft(""); setQuery("");
  }

  const models = useMemo(() => page.models, [page.models]);
  return <div className="flex flex-col gap-5">
    {message ? <div className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}
    <Card><CardContent className="grid gap-3 p-4 md:grid-cols-2 xl:grid-cols-[1.2fr_1fr_1fr_1fr_1fr_auto]">
      <form className="relative" onSubmit={submitSearch}><Search aria-hidden="true" className="absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input aria-label="搜索模型或请求 ID" className="pl-9" onChange={(event) => setQueryDraft(event.target.value)} placeholder="搜索模型或请求 ID" value={queryDraft} /></form>
      <Select onValueChange={(value) => { setOffset(0); setUserID(value); }} value={userID}><SelectTrigger aria-label="按用户筛选"><SelectValue /></SelectTrigger><SelectContent><SelectGroup><SelectItem value="all">全部用户</SelectItem>{users.map((record) => <SelectItem key={record.id} value={record.id}>{record.display_name || record.username} (@{record.username}){record.status === "disabled" ? " · 已停用" : ""}</SelectItem>)}</SelectGroup></SelectContent></Select>
      <Select onValueChange={(value) => { setOffset(0); setModel(value === "all" ? "" : value); }} value={model || "all"}><SelectTrigger aria-label="按模型筛选"><SelectValue /></SelectTrigger><SelectContent><SelectGroup><SelectItem value="all">全部模型</SelectItem>{models.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectGroup></SelectContent></Select>
      <Select onValueChange={(value) => { setOffset(0); setStatus(value); }} value={status}><SelectTrigger aria-label="按请求状态筛选"><SelectValue /></SelectTrigger><SelectContent><SelectGroup><SelectItem value="all">全部状态</SelectItem><SelectItem value="success">成功</SelectItem><SelectItem value="failed">失败</SelectItem></SelectGroup></SelectContent></Select>
      <Select onValueChange={(value) => { setOffset(0); setTimeRange(value); }} value={timeRange}><SelectTrigger aria-label="按时间范围筛选"><SelectValue /></SelectTrigger><SelectContent><SelectGroup><SelectItem value="all">全部时间</SelectItem><SelectItem value="24">最近 24 小时</SelectItem><SelectItem value="168">最近 7 天</SelectItem><SelectItem value="720">最近 30 天</SelectItem></SelectGroup></SelectContent></Select>
      <div className="flex gap-2"><Button aria-label="清除筛选" onClick={clearFilters} size="icon" title="清除筛选" type="button" variant="outline"><Filter /></Button><Button aria-label="刷新使用日志" disabled={loading} onClick={() => void load()} size="icon" title="刷新使用日志" type="button" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button></div>
    </CardContent></Card>
    <section className="grid border-y bg-background sm:grid-cols-2"><div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">当前筛选调用</p><p className="mt-1 text-xl font-semibold">{page.total.toLocaleString("zh-CN")}</p></div><div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">筛选 Tokens / 费用</p><p className="mt-1 text-xl font-semibold">{page.total_tokens.toLocaleString("zh-CN")} <span className="text-sm font-normal text-muted-foreground">· {money(page.total_cost_micros)}</span></p></div></section>
    <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>时间</TableHead><TableHead>用户</TableHead><TableHead>API Key</TableHead><TableHead>模型</TableHead><TableHead>状态</TableHead><TableHead>用时</TableHead><TableHead>Token 明细</TableHead><TableHead>费用</TableHead><TableHead>错误</TableHead><TableHead /></TableRow></TableHeader><TableBody>
      {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={10}>加载中...</TableCell></TableRow> : null}
      {!loading && page.usage.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={10}>没有符合条件的调用</TableCell></TableRow> : null}
      {page.usage.map((item) => <TableRow key={item.id}><TableCell className="whitespace-nowrap text-muted-foreground">{date(item.created_at)}</TableCell><TableCell><p className="font-medium">{item.user_display_name || item.username}</p><p className="text-xs text-muted-foreground">@{item.username}</p></TableCell><TableCell>{item.api_key_name || "未命名 Key"}</TableCell><TableCell><p className="font-mono text-xs">{item.model}</p><p className="text-xs text-muted-foreground">{item.endpoint.replaceAll("_", " ")}</p></TableCell><TableCell><Badge variant={item.status_code >= 400 ? "destructive" : "secondary"}>{item.status_code}</Badge></TableCell><TableCell className="whitespace-nowrap text-muted-foreground">{duration(item.duration_ms)}</TableCell><TableCell>{item.status_code >= 400 ? <span className="text-muted-foreground">-</span> : <><p>{(item.input_tokens + item.output_tokens).toLocaleString("zh-CN")}</p><p className="text-xs text-muted-foreground">普通 {item.uncached_input_tokens} · 命中 {item.cache_read_input_tokens} · 创建 {item.cache_write_input_tokens + item.cache_write_1h_input_tokens} · 输出 {item.output_tokens}</p></>}</TableCell><TableCell>{item.status_code >= 400 ? <span className="text-muted-foreground">-</span> : money(item.cost_micros)}</TableCell><TableCell className="max-w-64"><p className="truncate text-sm text-destructive">{item.error_message || "-"}</p></TableCell><TableCell><Button aria-label={`查看 ${item.username} 的请求明细`} onClick={() => setSelected(item)} size="icon-sm" title="查看请求明细" variant="ghost"><Eye /></Button></TableCell></TableRow>)}
    </TableBody></Table></div><DataPagination loading={loading} offset={offset} onOffsetChange={setOffset} onPageSizeChange={(size) => { setOffset(0); setPageSize(size); }} pageSize={pageSize} total={page.total} /></CardContent></Card>
    <Sheet onOpenChange={(open) => { if (!open) setSelected(null); }} open={selected !== null}><SheetContent className="w-full overflow-y-auto sm:max-w-lg" side="right"><SheetHeader className="border-b px-6 py-5"><SheetTitle>{selected && selected.status_code >= 400 ? "失败请求明细" : "计费明细"}</SheetTitle>{selected ? <SheetDescription>{selected.user_display_name || selected.username} (@{selected.username})</SheetDescription> : null}</SheetHeader>{selected ? <div className="px-6 pb-8"><Metric label="请求 ID" value={selected.request_id} /><Metric label="状态" value={`${selected.status_code} · ${selected.status_code >= 400 ? "失败" : "成功"}`} /><Metric label="用时" value={duration(selected.duration_ms)} />{selected.error_message ? <Metric label="错误" value={`${selected.error_code || "request_failed"} · ${selected.error_message}`} /> : null}<Metric label="模型映射" value={`${selected.model} -> ${selected.upstream_model_name || "-"}`} /><div className="grid grid-cols-2 gap-x-4"><Metric label="普通输入" value={`${selected.uncached_input_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.input_price_micros)}/1M`} /><Metric label="缓存命中" value={`${selected.cache_read_input_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.cache_read_price_micros)}/1M`} /><Metric label="缓存创建（5 分钟）" value={`${selected.cache_write_input_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.cache_write_price_micros)}/1M`} /><Metric label="缓存创建（1 小时）" value={`${selected.cache_write_1h_input_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.cache_write_1h_price_micros)}/1M`} /><Metric label="输出" value={`${selected.output_tokens.toLocaleString("zh-CN")} tokens × ${money(selected.rates.output_price_micros)}/1M`} /><Metric label="请求固定费" value={money(selected.rates.request_price_micros)} /></div><Metric label="基础成本" value={money(selected.base_cost_micros)} /><Metric label="请求时计费分组" value={`${selected.billing_group_name || "默认分组"} ${selected.billing_group_code ? `(${selected.billing_group_code})` : ""} · ${(selected.multiplier_bps / 10_000).toFixed(4)}×`} /><Metric label="预占金额" value={money(selected.reserved_micros)} /><Metric label="最终扣费" value={selected.status_code >= 400 ? "未扣费" : money(selected.cost_micros)} /><Metric label="算法版本" value={selected.calculation_version || "-"} /><Metric label="完成时间" value={date(selected.finished_at)} /></div> : null}</SheetContent></Sheet>
  </div>;
}
