"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Eye, Pencil, Plus, Power, PowerOff, RefreshCw, Route, Search } from "lucide-react";
import { useRouter } from "next/navigation";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type Provider = { id: string; code: string; display_name: string; protocol: "openai" | "anthropic"; status: "active" | "disabled" };
type ModelRouteRecord = { id: string; provider_id: string; public_name: string; display_name: string; upstream_name: string; input_price_micros: number; output_price_micros: number; status: "active" | "disabled"; provider: Provider; created_at: string; updated_at: string };
type RouteForm = { provider_id: string; public_name: string; display_name: string; upstream_name: string; input_price: string; output_price: string };
type ErrorResponse = { error?: { message?: string } };

const INITIAL_FORM: RouteForm = { provider_id: "", public_name: "", display_name: "", upstream_name: "", input_price: "", output_price: "" };

async function readError(response: Response) { const body = (await response.json().catch(() => ({}))) as ErrorResponse; return body.error?.message ?? "操作失败，请稍后重试"; }
function formatDate(value: string) { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)); }
function formatPrice(micros: number) { return new Intl.NumberFormat("zh-CN", { minimumFractionDigits: 0, maximumFractionDigits: 6 }).format(micros / 1_000_000); }
function toMicros(value: string) { const match = value.trim().match(/^(\d{1,9})(?:\.(\d{1,6}))?$/); if (!match) return null; return Number(match[1]) * 1_000_000 + Number((match[2] ?? "").padEnd(6, "0")); }
function protocolLabel(value: Provider["protocol"]) { return value === "openai" ? "OpenAI 兼容" : "Anthropic"; }

export default function ModelRoutesClient() {
  const router = useRouter();
  const [routes, setRoutes] = useState<ModelRouteRecord[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [formError, setFormError] = useState("");
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "disabled">("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [detailRoute, setDetailRoute] = useState<ModelRouteRecord | null>(null);
  const [statusRoute, setStatusRoute] = useState<ModelRouteRecord | null>(null);
  const [form, setForm] = useState<RouteForm>(INITIAL_FORM);
  const [editForm, setEditForm] = useState<RouteForm>(INITIAL_FORM);

  const load = useCallback(async () => {
    setLoading(true);
    const query = new URLSearchParams(); if (search) query.set("search", search); if (status !== "all") query.set("status", status);
    const [routesResponse, providersResponse] = await Promise.all([fetch(`/api/admin/model-routes?${query}`, { cache: "no-store" }), fetch("/api/admin/providers", { cache: "no-store" })]);
    if (routesResponse.status === 401 || providersResponse.status === 401) { router.replace("/login"); return; }
    if (routesResponse.status === 403 || providersResponse.status === 403) { router.replace("/console"); return; }
    if (!routesResponse.ok) { setMessage(await readError(routesResponse)); setLoading(false); return; }
    if (!providersResponse.ok) { setMessage(await readError(providersResponse)); setLoading(false); return; }
    setRoutes(((await routesResponse.json()) as { model_routes: ModelRouteRecord[] }).model_routes);
    setProviders(((await providersResponse.json()) as { providers: Provider[] }).providers);
    setLoading(false);
  }, [router, search, status]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);

  function submitSearch(event: FormEvent<HTMLFormElement>) { event.preventDefault(); setSearch(searchDraft.trim()); }
  function openDetails(record: ModelRouteRecord) { setDetailRoute(record); setEditForm({ provider_id: record.provider_id, public_name: record.public_name, display_name: record.display_name, upstream_name: record.upstream_name, input_price: formatPrice(record.input_price_micros), output_price: formatPrice(record.output_price_micros) }); setFormError(""); }
  function payloadFrom(formValue: RouteForm, includeName: boolean) {
    const input = toMicros(formValue.input_price); const output = toMicros(formValue.output_price);
    if (input === null || output === null) return null;
    return { provider_id: formValue.provider_id, ...(includeName ? { public_name: formValue.public_name } : {}), display_name: formValue.display_name, upstream_name: formValue.upstream_name, input_price_micros: input, output_price_micros: output };
  }

  async function createRoute(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const payload = payloadFrom(form, true); if (!payload) { setFormError("价格最多保留 6 位小数"); return; }
    setBusy(true); setFormError(""); setMessage("");
    const response = await fetch("/api/admin/model-routes", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) });
    setBusy(false); if (!response.ok) { setFormError(await readError(response)); return; }
    setForm(INITIAL_FORM); setCreateOpen(false); setMessage("模型路由已创建"); await load();
  }

  async function updateRoute(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!detailRoute) return; const payload = payloadFrom(editForm, false); if (!payload) { setFormError("价格最多保留 6 位小数"); return; }
    setBusy(true); setFormError(""); setMessage("");
    const response = await fetch(`/api/admin/model-routes/${detailRoute.id}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) });
    setBusy(false); if (!response.ok) { setFormError(await readError(response)); return; }
    const record = ((await response.json()) as { model_route: ModelRouteRecord }).model_route; openDetails(record); setMessage("模型路由已更新"); await load();
  }

  async function toggleStatus() {
    if (!statusRoute) return; const nextStatus = statusRoute.status === "active" ? "disabled" : "active"; setBusy(true); setMessage("");
    const response = await fetch(`/api/admin/model-routes/${statusRoute.id}/status`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ status: nextStatus }) });
    setBusy(false); if (!response.ok) { setMessage(await readError(response)); setStatusRoute(null); return; }
    setStatusRoute(null); setMessage(nextStatus === "active" ? "模型路由已启用" : "模型路由已停用"); await load();
  }

  const activeCount = routes.filter((route) => route.status === "active").length;

  return (
    <>
      <div className="space-y-5">
        <section className="grid border-y bg-background sm:grid-cols-3"><div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">匹配路由</p><p className="mt-1 text-xl font-semibold">{routes.length}</p></div><div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">启用路由</p><p className="mt-1 text-xl font-semibold">{activeCount}</p></div><div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">可选提供商</p><p className="mt-1 text-xl font-semibold">{providers.length}</p></div></section>
        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between"><form className="flex min-w-0 flex-1 gap-2" onSubmit={submitSearch}><div className="relative max-w-md flex-1"><Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索模型路由" className="pl-8" onChange={(event) => setSearchDraft(event.target.value)} placeholder="搜索对外名称、上游名称或提供商" value={searchDraft} /></div><Button type="submit" variant="outline">搜索</Button></form><div className="flex flex-wrap items-center gap-2"><Select onValueChange={(value: "all" | "active" | "disabled") => setStatus(value)} value={status}><SelectTrigger aria-label="按状态筛选" className="w-32"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="active">已启用</SelectItem><SelectItem value="disabled">已停用</SelectItem></SelectContent></Select><Button aria-label="刷新模型路由" disabled={loading} onClick={() => void load()} size="icon" title="刷新模型路由" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button><Button disabled={providers.length === 0} onClick={() => setCreateOpen(true)}><Plus />添加模型</Button></div></div>
        {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}
        <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead className="min-w-52">对外模型</TableHead><TableHead className="min-w-44">提供商</TableHead><TableHead className="min-w-52">上游模型</TableHead><TableHead>输入价</TableHead><TableHead>输出价</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>
          {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={7}>加载中...</TableCell></TableRow> : null}{!loading && routes.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={7}>没有匹配的模型路由</TableCell></TableRow> : null}
          {!loading ? routes.map((record) => <TableRow key={record.id}><TableCell><p className="font-medium">{record.display_name}</p><p className="mt-0.5 font-mono text-xs text-muted-foreground">{record.public_name}</p></TableCell><TableCell><p>{record.provider.display_name}</p><p className="mt-0.5 text-xs text-muted-foreground">{protocolLabel(record.provider.protocol)}</p></TableCell><TableCell className="font-mono text-xs">{record.upstream_name}</TableCell><TableCell>¥{formatPrice(record.input_price_micros)}</TableCell><TableCell>¥{formatPrice(record.output_price_micros)}</TableCell><TableCell><Badge variant={record.status === "active" ? "outline" : "secondary"}>{record.status === "active" ? "启用" : "停用"}</Badge></TableCell><TableCell><div className="flex justify-end gap-1"><Button aria-label={`查看 ${record.public_name}`} onClick={() => openDetails(record)} size="icon-sm" title="查看与编辑" variant="ghost"><Eye /></Button><Button aria-label={`${record.status === "active" ? "停用" : "启用"} ${record.public_name}`} onClick={() => setStatusRoute(record)} size="icon-sm" title={record.status === "active" ? "停用模型" : "启用模型"} variant="ghost">{record.status === "active" ? <PowerOff /> : <Power />}</Button></div></TableCell></TableRow>) : null}
        </TableBody></Table></div><div className="border-t px-4 py-3 text-xs text-muted-foreground">价格单位：人民币 / 百万 tokens</div></CardContent></Card>

        <Dialog onOpenChange={(open) => { setCreateOpen(open); setFormError(""); if (!open) setForm(INITIAL_FORM); }} open={createOpen}><DialogContent><DialogHeader><DialogTitle>添加模型路由</DialogTitle><DialogDescription>对外模型名创建后不可修改，客户端通过该名称选择上游模型。</DialogDescription></DialogHeader><RouteFormFields form={form} idPrefix="new" onChange={setForm} onSubmit={createRoute} providers={providers} showPublicName />{formError ? <p className="text-sm text-destructive" role="alert">{formError}</p> : null}<DialogFooter><Button onClick={() => setCreateOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy} form="new-route-form" type="submit"><Route />{busy ? "正在创建..." : "创建模型"}</Button></DialogFooter></DialogContent></Dialog>

        <Sheet onOpenChange={(open) => { setFormError(""); if (!open) setDetailRoute(null); }} open={detailRoute !== null}><SheetContent className="w-full overflow-y-auto sm:max-w-lg" side="right">{detailRoute ? <><SheetHeader className="border-b px-6 py-5"><SheetTitle>{detailRoute.display_name}</SheetTitle><SheetDescription>{detailRoute.public_name}</SheetDescription></SheetHeader><div className="grid grid-cols-2 gap-4 border-b px-6 pb-5 text-sm"><div><p className="text-xs text-muted-foreground">状态</p><Badge className="mt-2" variant={detailRoute.status === "active" ? "outline" : "secondary"}>{detailRoute.status === "active" ? "启用" : "停用"}</Badge></div><div><p className="text-xs text-muted-foreground">最近更新</p><p className="mt-2">{formatDate(detailRoute.updated_at)}</p></div></div><RouteFormFields form={editForm} idPrefix="edit" onChange={setEditForm} onSubmit={updateRoute} providers={providers} />{formError ? <p className="px-6 text-sm text-destructive" role="alert">{formError}</p> : null}<SheetFooter className="border-t px-6"><Button disabled={busy} form="edit-route-form" type="submit"><Pencil />{busy ? "正在保存..." : "保存修改"}</Button><Button onClick={() => { setDetailRoute(null); setStatusRoute(detailRoute); }} type="button" variant="outline">{detailRoute.status === "active" ? <PowerOff /> : <Power />}{detailRoute.status === "active" ? "停用模型" : "启用模型"}</Button></SheetFooter></> : null}</SheetContent></Sheet>

        <AlertDialog onOpenChange={(open) => { if (!open) setStatusRoute(null); }} open={statusRoute !== null}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{statusRoute?.status === "active" ? "停用模型路由" : "启用模型路由"}</AlertDialogTitle><AlertDialogDescription>{statusRoute?.status === "active" ? `停用 ${statusRoute.public_name} 后，新请求将无法再选择该模型。` : `启用 ${statusRoute?.public_name ?? "该模型"} 后，新请求可以选择该路由。`}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }} variant={statusRoute?.status === "active" ? "destructive" : "default"}>{statusRoute?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
      </div>
    </>
  );
}

function RouteFormFields({ form, idPrefix, onChange, onSubmit, providers, showPublicName = false }: { form: RouteForm; idPrefix: string; onChange: (value: RouteForm) => void; onSubmit?: (event: FormEvent<HTMLFormElement>) => void; providers: Provider[]; showPublicName?: boolean }) {
  const submit = onSubmit ?? (() => undefined);
  return <form className={idPrefix === "edit" ? "space-y-5 px-6" : "space-y-4"} id={`${idPrefix}-route-form`} onSubmit={submit}>{showPublicName ? <div className="space-y-2"><Label htmlFor={`${idPrefix}-public-name`}>对外模型名</Label><Input id={`${idPrefix}-public-name`} maxLength={128} minLength={2} onChange={(event) => onChange({ ...form, public_name: event.target.value })} pattern="[A-Za-z0-9][A-Za-z0-9._:/-]{1,127}" placeholder="例如 deepseek-chat" required value={form.public_name} /></div> : null}<div className="space-y-2"><Label htmlFor={`${idPrefix}-display-name`}>显示名称</Label><Input id={`${idPrefix}-display-name`} maxLength={128} onChange={(event) => onChange({ ...form, display_name: event.target.value })} required value={form.display_name} /></div><div className="space-y-2"><Label htmlFor={`${idPrefix}-provider`}>提供商</Label><Select onValueChange={(provider_id) => onChange({ ...form, provider_id })} value={form.provider_id}><SelectTrigger className="w-full" id={`${idPrefix}-provider`}><SelectValue placeholder="选择提供商" /></SelectTrigger><SelectContent>{providers.map((provider) => <SelectItem key={provider.id} value={provider.id}>{provider.display_name} · {protocolLabel(provider.protocol)}{provider.status === "disabled" ? "（停用）" : ""}</SelectItem>)}</SelectContent></Select></div><div className="space-y-2"><Label htmlFor={`${idPrefix}-upstream-name`}>上游模型名</Label><Input id={`${idPrefix}-upstream-name`} maxLength={256} onChange={(event) => onChange({ ...form, upstream_name: event.target.value })} placeholder="提供商接受的 model 值" required value={form.upstream_name} /></div><div className="grid grid-cols-2 gap-4"><div className="space-y-2"><Label htmlFor={`${idPrefix}-input-price`}>输入价</Label><Input id={`${idPrefix}-input-price`} inputMode="decimal" onChange={(event) => onChange({ ...form, input_price: event.target.value })} placeholder="2.00" required value={form.input_price} /></div><div className="space-y-2"><Label htmlFor={`${idPrefix}-output-price`}>输出价</Label><Input id={`${idPrefix}-output-price`} inputMode="decimal" onChange={(event) => onChange({ ...form, output_price: event.target.value })} placeholder="8.00" required value={form.output_price} /></div></div></form>;
}
