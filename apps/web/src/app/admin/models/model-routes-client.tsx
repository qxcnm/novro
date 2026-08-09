"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Pencil, Plus, Power, PowerOff, RefreshCw, Route, Search, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { BulkActionDialog, ListBulkActions } from "@/components/list-bulk-actions";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { bulkResultMessage, runBulkAction } from "@/lib/bulk-action";
import { useListSelection } from "@/lib/use-list-selection";

type Prices = {
  input_price_micros: number;
  output_price_micros: number;
  cache_read_price_micros: number;
  cache_write_price_micros: number;
  cache_write_1h_price_micros: number;
  request_price_micros: number;
};

type Provider = {
  id: string;
  code: string;
  display_name: string;
  status: "active" | "disabled";
};

type CatalogModel = {
  id: string;
  provider_name: string;
  upstream_name: string;
  display_name: string;
  pricing_configured: boolean;
  status: "active" | "disabled";
  prices: Prices;
};

type ModelRoute = {
  id: string;
  provider_id: string;
  upstream_model_id: string | null;
  public_name: string;
  display_name: string;
  status: "active" | "disabled";
  provider: Provider;
  upstream_model?: CatalogModel;
};

type RouteForm = {
  provider_id: string;
  upstream_model_id: string;
  public_name: string;
  display_name: string;
};

const EMPTY_FORM: RouteForm = { provider_id: "", upstream_model_id: "", public_name: "", display_name: "" };

function money(micros: number) {
  return `¥${(micros / 1_000_000).toLocaleString("zh-CN", { maximumFractionDigits: 6 })}`;
}

async function errorMessage(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

export default function ModelRoutesClient({ refreshKey = 0 }: { refreshKey?: number }) {
  const router = useRouter();
  const [routes, setRoutes] = useState<ModelRoute[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [catalog, setCatalog] = useState<CatalogModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [query, setQuery] = useState("");
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<ModelRoute | null>(null);
  const [statusRoute, setStatusRoute] = useState<ModelRoute | null>(null);
  const [deletingRoute, setDeletingRoute] = useState<ModelRoute | null>(null);
  const [form, setForm] = useState<RouteForm>(EMPTY_FORM);
  const [bulkStatus, setBulkStatus] = useState<"active" | "disabled" | null>(null);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    const [routesResponse, providersResponse, catalogResponse] = await Promise.all([
      fetch("/api/admin/model-routes", { cache: "no-store" }),
      fetch("/api/admin/providers", { cache: "no-store" }),
      fetch("/api/admin/upstream-models", { cache: "no-store" }),
    ]);
    if (routesResponse.status === 401) { router.replace("/login"); return; }
    if (routesResponse.status === 403) { router.replace("/console"); return; }
    if (!routesResponse.ok || !providersResponse.ok || !catalogResponse.ok) {
      setMessage("加载模型路由失败");
      setLoading(false);
      return;
    }
    setRoutes(((await routesResponse.json()) as { model_routes: ModelRoute[] }).model_routes);
    setProviders(((await providersResponse.json()) as { providers: Provider[] }).providers);
    setCatalog(((await catalogResponse.json()) as { upstream_models: CatalogModel[] }).upstream_models);
    setLoading(false);
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load, refreshKey]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return needle
      ? routes.filter((route) => `${route.public_name} ${route.display_name} ${route.provider.display_name} ${route.upstream_model?.upstream_name ?? ""}`.toLowerCase().includes(needle))
      : routes;
  }, [query, routes]);
  const selection = useListSelection(filtered.map((route) => route.id));

  function beginCreate() {
    setEditing(null);
    setForm(EMPTY_FORM);
    setEditorOpen(true);
  }

  function beginEdit(route: ModelRoute) {
    setEditing(route);
    setForm({
      provider_id: route.provider_id,
      upstream_model_id: route.upstream_model_id ?? "",
      public_name: route.public_name,
      display_name: route.display_name,
    });
    setEditorOpen(true);
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    const body = editing
      ? { provider_id: form.provider_id, upstream_model_id: form.upstream_model_id, display_name: form.display_name }
      : form;
    const response = await fetch(editing ? `/api/admin/model-routes/${editing.id}` : "/api/admin/model-routes", {
      method: editing ? "PATCH" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    setBusy(false);
    if (!response.ok) { setMessage(await errorMessage(response)); return; }
    const updated = editing !== null;
    setEditorOpen(false);
    setEditing(null);
    setForm(EMPTY_FORM);
    setMessage(updated ? "模型路由已更新" : "模型路由已创建");
    await load();
  }

  async function toggleStatus() {
    if (!statusRoute) return;
    const next = statusRoute.status === "active" ? "disabled" : "active";
    setBusy(true);
    const response = await fetch(`/api/admin/model-routes/${statusRoute.id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: next }),
    });
    setBusy(false);
    if (!response.ok) {
      setMessage(await errorMessage(response));
    } else {
      setMessage(next === "active" ? "模型路由已启用" : "模型路由已停用");
      await load();
    }
    setStatusRoute(null);
  }

  async function applyBulkStatus() {
    if (!bulkStatus) return;
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/model-routes/${id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: bulkStatus }),
    }));
    setBusy(false);
    setBulkStatus(null);
    setMessage(bulkResultMessage(bulkStatus === "active" ? "启用模型路由" : "停用模型路由", result));
    await load();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  async function deleteOneRoute() {
    if (!deletingRoute) return;
    setBusy(true);
    const response = await fetch(`/api/admin/model-routes/${deletingRoute.id}`, { method: "DELETE" });
    setBusy(false);
    if (!response.ok) setMessage(await errorMessage(response));
    else {
      setMessage("模型路由已删除");
      await load();
    }
    setDeletingRoute(null);
  }

  async function deleteSelected() {
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/model-routes/${id}`, { method: "DELETE" }));
    setBusy(false);
    setBulkDeleteOpen(false);
    setMessage(bulkResultMessage("删除模型路由", result));
    await load();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative max-w-md flex-1">
          <Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="搜索模型路由" className="pl-8" onChange={(event) => { setQuery(event.target.value); selection.clearSelection(); }} placeholder="搜索对外名称、提供商或目录模型" value={query} />
        </div>
        <div className="flex gap-2">
          <Button aria-label="刷新模型路由" disabled={loading} onClick={() => void load()} size="icon" title="刷新模型路由" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
          <Button onClick={beginCreate}><Plus />新增关联路由</Button>
        </div>
      </div>

      {message ? <p className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</p> : null}

      <ListBulkActions onClear={selection.clearSelection} selectedCount={selection.selectedIds.length}>
        <Button disabled={busy} onClick={() => setBulkStatus("active")} size="sm" type="button" variant="outline"><Power />批量启用</Button>
        <Button disabled={busy} onClick={() => setBulkStatus("disabled")} size="sm" type="button" variant="destructive"><PowerOff />批量停用</Button>
        <Button disabled={busy} onClick={() => setBulkDeleteOpen(true)} size="sm" type="button" variant="destructive"><Trash2 />批量删除</Button>
      </ListBulkActions>

      <Card className="overflow-hidden">
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <Table>
              <TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择所有模型路由" checked={selection.checkboxState} disabled={loading || filtered.length === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead>对外模型</TableHead><TableHead>提供商配置</TableHead><TableHead>目录模型</TableHead><TableHead>输入 / 缓存命中</TableHead><TableHead>缓存创建 / 输出</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={8}>加载中...</TableCell></TableRow> : null}
                {!loading && filtered.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={8}>还没有关联模型路由</TableCell></TableRow> : null}
                {filtered.map((route) => (
                  <TableRow key={route.id}>
                    <TableCell><Checkbox aria-label={`选择 ${route.display_name}`} checked={selection.isSelected(route.id)} onCheckedChange={(checked) => selection.toggleOne(route.id, checked === true)} /></TableCell>
                    <TableCell><p className="font-medium">{route.display_name}</p><p className="font-mono text-xs text-muted-foreground">{route.public_name}</p></TableCell>
                    <TableCell><p>{route.provider.display_name}</p><p className="font-mono text-xs text-muted-foreground">{route.provider.code}</p></TableCell>
                    <TableCell><p>{route.upstream_model?.display_name ?? "未关联"}</p><p className="text-xs text-muted-foreground">{route.upstream_model ? `${route.upstream_model.provider_name} · ${route.upstream_model.upstream_name}` : "-"}</p></TableCell>
                    <TableCell><p>{money(route.upstream_model?.prices.input_price_micros ?? 0)}</p><p className="text-xs text-muted-foreground">命中 {money(route.upstream_model?.prices.cache_read_price_micros ?? 0)}</p></TableCell>
                    <TableCell><p>{money(route.upstream_model?.prices.cache_write_price_micros ?? 0)}</p><p className="text-xs text-muted-foreground">输出 {money(route.upstream_model?.prices.output_price_micros ?? 0)}</p></TableCell>
                    <TableCell><Badge variant={route.status === "active" ? "outline" : "secondary"}>{route.status === "active" ? "启用" : "停用"}</Badge></TableCell>
                    <TableCell><div className="flex justify-end gap-1"><Button aria-label={`编辑 ${route.display_name}`} onClick={() => beginEdit(route)} size="icon-sm" title="编辑" variant="ghost"><Pencil /></Button><Button aria-label={`${route.status === "active" ? "停用" : "启用"} ${route.display_name}`} onClick={() => setStatusRoute(route)} size="icon-sm" title={route.status === "active" ? "停用" : "启用"} variant="ghost">{route.status === "active" ? <PowerOff /> : <Power />}</Button><Button aria-label={`删除 ${route.display_name}`} onClick={() => setDeletingRoute(route)} size="icon-sm" title="删除模型路由" variant="ghost"><Trash2 /></Button></div></TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>

      <Dialog onOpenChange={(open) => { setEditorOpen(open); if (!open) { setEditing(null); setForm(EMPTY_FORM); } }} open={editorOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>{editing ? "编辑关联模型路由" : "新增关联模型路由"}</DialogTitle><DialogDescription>路由将一个对外模型名称绑定到提供商配置和模型目录记录。</DialogDescription></DialogHeader>
          <form className="space-y-4" id="model-route-form" onSubmit={submit}>
            <div className="space-y-2"><Label htmlFor="route-provider">提供商配置</Label><Select onValueChange={(provider_id) => setForm({ ...form, provider_id })} value={form.provider_id}><SelectTrigger className="w-full" id="route-provider"><SelectValue placeholder="选择提供商配置" /></SelectTrigger><SelectContent>{providers.map((provider) => <SelectItem key={provider.id} value={provider.id}>{provider.display_name}{provider.status === "disabled" ? "（已停用）" : ""}</SelectItem>)}</SelectContent></Select></div>
            <div className="space-y-2"><Label htmlFor="route-catalog-model">目录模型</Label><Select onValueChange={(upstream_model_id) => setForm({ ...form, upstream_model_id })} value={form.upstream_model_id}><SelectTrigger className="w-full" id="route-catalog-model"><SelectValue placeholder="选择目录模型" /></SelectTrigger><SelectContent>{catalog.map((model) => <SelectItem key={model.id} value={model.id}>{model.provider_name} · {model.display_name}{!model.pricing_configured ? "（待定价）" : model.status === "disabled" ? "（已停用）" : ""}</SelectItem>)}</SelectContent></Select></div>
            <div className="space-y-2"><Label htmlFor="route-public-name">对外模型名称</Label><Input disabled={editing !== null} id="route-public-name" maxLength={256} minLength={2} onChange={(event) => setForm({ ...form, public_name: event.target.value })} pattern="[A-Za-z0-9][A-Za-z0-9._:/-]{1,255}" placeholder="例如 deepseek-chat" required value={form.public_name} /></div>
            <div className="space-y-2"><Label htmlFor="route-display-name">显示名称</Label><Input id="route-display-name" maxLength={128} onChange={(event) => setForm({ ...form, display_name: event.target.value })} placeholder="例如 DeepSeek Chat" required value={form.display_name} /></div>
          </form>
          <DialogFooter><Button onClick={() => setEditorOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy} form="model-route-form" type="submit"><Route />{busy ? "正在保存..." : "保存模型路由"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog onOpenChange={(open) => { if (!open) setStatusRoute(null); }} open={statusRoute !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{statusRoute?.status === "active" ? "停用模型路由" : "启用模型路由"}</AlertDialogTitle><AlertDialogDescription>停用后该渠道不再参与对应对外模型的请求轮询。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }}>{statusRoute?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
      <AlertDialog onOpenChange={(open) => { if (!open) setDeletingRoute(null); }} open={deletingRoute !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除模型路由</AlertDialogTitle><AlertDialogDescription>将删除 {deletingRoute?.display_name ?? "该模型路由"} 对应的渠道；同名的其他启用渠道不受影响，历史调用和计费记录会继续保留。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void deleteOneRoute(); }} variant="destructive">确认删除</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
      <BulkActionDialog busy={busy} confirmLabel={bulkStatus === "active" ? "确认批量启用" : "确认批量停用"} description={`将${bulkStatus === "active" ? "启用" : "停用"}选中的 ${selection.selectedIds.length} 条模型路由。`} destructive={bulkStatus === "disabled"} onConfirm={applyBulkStatus} onOpenChange={(open) => { if (!open) setBulkStatus(null); }} open={bulkStatus !== null} title={bulkStatus === "active" ? "批量启用模型路由" : "批量停用模型路由"} />
      <BulkActionDialog busy={busy} confirmLabel="确认批量删除" description={`将删除选中的 ${selection.selectedIds.length} 条模型路由。历史调用和计费记录会继续保留，失败项目仍会保持选中。`} destructive onConfirm={deleteSelected} onOpenChange={setBulkDeleteOpen} open={bulkDeleteOpen} title="批量删除模型路由" />
    </div>
  );
}
