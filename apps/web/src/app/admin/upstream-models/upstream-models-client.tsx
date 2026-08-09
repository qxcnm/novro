"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Boxes, LayoutGrid, Pencil, Plus, Power, PowerOff, RefreshCw, Search, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { BulkActionDialog, ListBulkActions } from "@/components/list-bulk-actions";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
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

export type CatalogModel = {
  id: string;
  provider_name: string;
  upstream_name: string;
  display_name: string;
  prices: Prices;
  pricing_configured: boolean;
  status: "active" | "disabled";
};

type ModelForm = {
  providerName: string;
  upstreamName: string;
  displayName: string;
  input: string;
  cacheRead: string;
  cacheWrite: string;
  cacheWrite1h: string;
  output: string;
  request: string;
};

const EMPTY_FORM: ModelForm = {
  providerName: "",
  upstreamName: "",
  displayName: "",
  input: "",
  cacheRead: "",
  cacheWrite: "",
  cacheWrite1h: "",
  output: "",
  request: "",
};

const ALL_PROVIDERS = "__all__";
const preferredProviderOrder = ["deepseek", "glm", "智谱", "kimi", "moonshot"];

function providerOrder(name: string) {
  const normalized = name.toLowerCase();
  const index = preferredProviderOrder.findIndex((keyword) => normalized.includes(keyword));
  return index === -1 ? preferredProviderOrder.length : index;
}

function money(micros: number) {
  return `¥${(micros / 1_000_000).toLocaleString("zh-CN", { maximumFractionDigits: 6 })}`;
}

function toMicros(value: string) {
  const parsed = Number(value || "0");
  return Number.isFinite(parsed) && parsed >= 0 ? Math.round(parsed * 1_000_000) : null;
}

function fromMicros(value: number) {
  return String(value / 1_000_000);
}

async function errorMessage(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

function payload(form: ModelForm) {
  const values = [form.input, form.output, form.cacheRead, form.cacheWrite, form.cacheWrite1h, form.request].map(toMicros);
  if (values.some((value) => value === null)) return null;
  return {
    provider_name: form.providerName,
    upstream_name: form.upstreamName,
    display_name: form.displayName,
    input_price_micros: values[0],
    output_price_micros: values[1],
    cache_read_price_micros: values[2],
    cache_write_price_micros: values[3],
    cache_write_1h_price_micros: values[4],
    request_price_micros: values[5],
  };
}

function PriceFields({ form, setForm }: { form: ModelForm; setForm: (form: ModelForm) => void }) {
  const fields: Array<[keyof ModelForm, string, string]> = [
    ["input", "普通输入", "元 / 1M tokens"],
    ["cacheRead", "缓存命中", "元 / 1M tokens"],
    ["cacheWrite", "缓存创建（5 分钟）", "元 / 1M tokens"],
    ["cacheWrite1h", "缓存创建（1 小时）", "元 / 1M tokens"],
    ["output", "输出", "元 / 1M tokens"],
    ["request", "请求固定费", "元 / 次"],
  ];
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {fields.map(([key, label, suffix]) => (
        <div className="space-y-2" key={key}>
          <Label htmlFor={`catalog-price-${key}`}>{label}</Label>
          <div className="relative">
            <Input
              className="pr-28"
              id={`catalog-price-${key}`}
              inputMode="decimal"
              min="0"
              onChange={(event) => setForm({ ...form, [key]: event.target.value })}
              placeholder="0"
              required
              step="0.000001"
              type="number"
              value={form[key]}
            />
            <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{suffix}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

export default function UpstreamModelsClient() {
  const router = useRouter();
  const [models, setModels] = useState<CatalogModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [query, setQuery] = useState("");
  const [selectedProvider, setSelectedProvider] = useState(ALL_PROVIDERS);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<CatalogModel | null>(null);
  const [statusModel, setStatusModel] = useState<CatalogModel | null>(null);
  const [deletingModel, setDeletingModel] = useState<CatalogModel | null>(null);
  const [form, setForm] = useState<ModelForm>(EMPTY_FORM);
  const [bulkStatus, setBulkStatus] = useState<"active" | "disabled" | null>(null);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    const response = await fetch("/api/admin/upstream-models", { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) {
      setMessage("加载模型目录失败");
      setLoading(false);
      return;
    }
    setModels(((await response.json()) as { upstream_models: CatalogModel[] }).upstream_models);
    setLoading(false);
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const providers = useMemo(() => {
    const counts = new Map<string, number>();
    for (const model of models) counts.set(model.provider_name, (counts.get(model.provider_name) ?? 0) + 1);
    return [...counts.entries()]
      .map(([name, count]) => ({ name, count }))
      .sort((left, right) => providerOrder(left.name) - providerOrder(right.name) || left.name.localeCompare(right.name, "zh-CN"));
  }, [models]);
  const activeProvider = selectedProvider === ALL_PROVIDERS || providers.some((provider) => provider.name === selectedProvider) ? selectedProvider : ALL_PROVIDERS;

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return models.filter((model) => {
      const providerMatches = activeProvider === ALL_PROVIDERS || model.provider_name === activeProvider;
      const queryMatches = !needle || `${model.display_name} ${model.upstream_name} ${model.provider_name}`.toLowerCase().includes(needle);
      return providerMatches && queryMatches;
    });
  }, [activeProvider, models, query]);
  const selection = useListSelection(filtered.map((model) => model.id));

  function chooseProvider(provider: string) {
    setSelectedProvider(provider);
    selection.clearSelection();
  }

  function beginCreate() {
    setEditing(null);
    setForm(EMPTY_FORM);
    setEditorOpen(true);
  }

  function beginEdit(model: CatalogModel) {
    setEditing(model);
    setForm({
      providerName: model.provider_name,
      upstreamName: model.upstream_name,
      displayName: model.display_name,
      input: fromMicros(model.prices.input_price_micros),
      cacheRead: fromMicros(model.prices.cache_read_price_micros),
      cacheWrite: fromMicros(model.prices.cache_write_price_micros),
      cacheWrite1h: fromMicros(model.prices.cache_write_1h_price_micros),
      output: fromMicros(model.prices.output_price_micros),
      request: fromMicros(model.prices.request_price_micros),
    });
    setEditorOpen(true);
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const body = payload(form);
    if (!body) { setMessage("价格格式无效"); return; }
    setBusy(true);
    const response = await fetch(editing ? `/api/admin/upstream-models/${editing.id}` : "/api/admin/upstream-models", {
      method: editing ? "PATCH" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    setBusy(false);
    if (!response.ok) { setMessage(await errorMessage(response)); return; }
    setEditorOpen(false);
    setEditing(null);
    setForm(EMPTY_FORM);
    setMessage(editing ? "目录模型已更新" : "目录模型已创建");
    await load();
  }

  async function toggleStatus() {
    if (!statusModel) return;
    setBusy(true);
    const next = statusModel.status === "active" ? "disabled" : "active";
    const response = await fetch(`/api/admin/upstream-models/${statusModel.id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: next }),
    });
    setBusy(false);
    if (!response.ok) {
      setMessage(await errorMessage(response));
    } else {
      setMessage(next === "active" ? "目录模型已启用" : "目录模型已停用");
      await load();
    }
    setStatusModel(null);
  }

  async function applyBulkStatus() {
    if (!bulkStatus) return;
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/upstream-models/${id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: bulkStatus }),
    }));
    setBusy(false);
    setBulkStatus(null);
    setMessage(bulkResultMessage(bulkStatus === "active" ? "启用目录模型" : "停用目录模型", result));
    await load();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  async function deleteOneModel() {
    if (!deletingModel) return;
    setBusy(true);
    const response = await fetch(`/api/admin/upstream-models/${deletingModel.id}`, { method: "DELETE" });
    setBusy(false);
    if (!response.ok) setMessage(await errorMessage(response));
    else {
      setMessage("目录模型已删除，关联路由已一并移出列表");
      await load();
    }
    setDeletingModel(null);
  }

  async function deleteSelected() {
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/upstream-models/${id}`, { method: "DELETE" }));
    setBusy(false);
    setBulkDeleteOpen(false);
    setMessage(bulkResultMessage("删除目录模型", result));
    await load();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  return (
    <>
      <div className="grid min-w-0 gap-5 lg:grid-cols-[200px_minmax(0,1fr)]">
      <aside className="min-w-0 border-b pb-4 lg:border-b-0 lg:border-r lg:pb-0 lg:pr-4">
        <nav aria-label="按提供商筛选模型" className="flex gap-1 overflow-x-auto pb-1 lg:sticky lg:top-24 lg:block lg:space-y-1 lg:overflow-visible lg:pb-0">
          <Button aria-pressed={activeProvider === ALL_PROVIDERS} className="h-9 shrink-0 justify-between gap-4 lg:w-full" onClick={() => chooseProvider(ALL_PROVIDERS)} variant={activeProvider === ALL_PROVIDERS ? "secondary" : "ghost"}><span className="flex items-center gap-2"><LayoutGrid />全部模型</span><span className="text-xs tabular-nums text-muted-foreground">{models.length}</span></Button>
          {providers.map((provider) => <Button aria-pressed={activeProvider === provider.name} className="h-9 shrink-0 justify-between gap-4 lg:w-full" key={provider.name} onClick={() => chooseProvider(provider.name)} title={provider.name} variant={activeProvider === provider.name ? "secondary" : "ghost"}><span className="max-w-28 truncate">{provider.name}</span><span className="text-xs tabular-nums text-muted-foreground">{provider.count}</span></Button>)}
        </nav>
      </aside>

      <div className="min-w-0 space-y-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="relative max-w-lg flex-1">
            <Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input aria-label="搜索模型目录" className="pl-8" onChange={(event) => { setQuery(event.target.value); selection.clearSelection(); }} placeholder={activeProvider === ALL_PROVIDERS ? "搜索模型名称或模型 ID" : `在 ${activeProvider} 标签中搜索模型`} value={query} />
          </div>
          <div className="flex gap-2">
            <Button aria-label="刷新模型目录" disabled={loading} onClick={() => void load()} size="icon" title="刷新模型目录" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
            <Button onClick={beginCreate}><Plus />新增目录模型</Button>
          </div>
        </div>

        <div className="flex min-h-5 items-center justify-between gap-3 text-xs text-muted-foreground"><span>{activeProvider === ALL_PROVIDERS ? "全部提供商" : activeProvider}</span><span>{filtered.length} 个结果</span></div>

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
                <TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择所有目录模型" checked={selection.checkboxState} disabled={loading || filtered.length === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead>模型与统一价格</TableHead><TableHead>普通输入</TableHead><TableHead>缓存命中</TableHead><TableHead>缓存创建</TableHead><TableHead>输出</TableHead><TableHead>请求费</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>
                  {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={9}>加载中...</TableCell></TableRow> : null}
                  {!loading && filtered.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={9}>{query.trim() ? "没有匹配的目录模型" : "该提供商暂无目录模型"}</TableCell></TableRow> : null}
                  {filtered.map((model) => (
                    <TableRow key={model.id}>
                      <TableCell><Checkbox aria-label={`选择 ${model.display_name}`} checked={selection.isSelected(model.id)} onCheckedChange={(checked) => selection.toggleOne(model.id, checked === true)} /></TableCell>
                      <TableCell><p className="font-medium">{model.display_name}</p><p className="text-xs text-muted-foreground">{model.provider_name} · <span className="font-mono">{model.upstream_name}</span></p></TableCell>
                      <TableCell>{money(model.prices.input_price_micros)}</TableCell>
                      <TableCell>{money(model.prices.cache_read_price_micros)}</TableCell>
                      <TableCell><p>{money(model.prices.cache_write_price_micros)}</p><p className="text-xs text-muted-foreground">1h {money(model.prices.cache_write_1h_price_micros)}</p></TableCell>
                      <TableCell>{money(model.prices.output_price_micros)}</TableCell>
                      <TableCell>{money(model.prices.request_price_micros)}</TableCell>
                      <TableCell>{!model.pricing_configured ? <Badge variant="destructive">待定价</Badge> : <Badge variant={model.status === "active" ? "outline" : "secondary"}>{model.status === "active" ? "启用" : "停用"}</Badge>}</TableCell>
                      <TableCell><div className="flex justify-end gap-1"><Button aria-label={`编辑 ${model.display_name}`} onClick={() => beginEdit(model)} size="icon-sm" title="编辑" variant="ghost"><Pencil /></Button><Button aria-label={`${model.status === "active" ? "停用" : "启用"} ${model.display_name}`} onClick={() => setStatusModel(model)} size="icon-sm" title={model.status === "active" ? "停用" : "启用"} variant="ghost">{model.status === "active" ? <PowerOff /> : <Power />}</Button><Button aria-label={`删除 ${model.display_name}`} onClick={() => setDeletingModel(model)} size="icon-sm" title="删除目录模型" variant="ghost"><Trash2 /></Button></div></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      </div>
      </div>

      <Sheet onOpenChange={(open) => { setEditorOpen(open); if (!open) { setEditing(null); setForm(EMPTY_FORM); } }} open={editorOpen}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl" side="right">
          <SheetHeader className="border-b px-6 py-5">
            <SheetTitle>{editing ? "编辑目录模型" : "新增目录模型"}</SheetTitle>
            <SheetDescription>模型 ID 全局唯一，所有提供商关联同一份目录价格；厂商标签仅用于分类，不绑定具体提供商配置。</SheetDescription>
          </SheetHeader>
          <form className="space-y-5 px-6" id="catalog-model-form" onSubmit={submit}>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="catalog-provider-name">厂商标签</Label><Input id="catalog-provider-name" maxLength={128} onChange={(event) => setForm({ ...form, providerName: event.target.value })} placeholder="例如 DeepSeek" required value={form.providerName} /></div>
              <div className="space-y-2"><Label htmlFor="catalog-upstream-name">模型 ID</Label><Input id="catalog-upstream-name" maxLength={256} onChange={(event) => setForm({ ...form, upstreamName: event.target.value })} placeholder="例如 deepseek-chat" required value={form.upstreamName} /></div>
            </div>
            <div className="space-y-2"><Label htmlFor="catalog-display-name">显示名称</Label><Input id="catalog-display-name" maxLength={128} onChange={(event) => setForm({ ...form, displayName: event.target.value })} placeholder="例如 DeepSeek Chat" required value={form.displayName} /></div>
            <p className="border-y py-3 text-sm text-muted-foreground">上游同步只发现模型 ID，不会导入上游价格。下面的价格是 Novro 的全局定价，所有提供商关联同一模型 ID 时共用这一份价格。</p>
            <PriceFields form={form} setForm={setForm} />
          </form>
          <SheetFooter className="border-t px-6"><Button disabled={busy} form="catalog-model-form" type="submit"><Boxes />{busy ? "正在保存..." : "保存目录模型"}</Button></SheetFooter>
        </SheetContent>
      </Sheet>

      <AlertDialog onOpenChange={(open) => { if (!open) setStatusModel(null); }} open={statusModel !== null}>
        <AlertDialogContent>
          <AlertDialogHeader><AlertDialogTitle>{statusModel?.status === "active" ? "停用目录模型" : "启用目录模型"}</AlertDialogTitle><AlertDialogDescription>{statusModel?.status === "active" ? "停用后，所有关联该目录模型的路由都会停止转发。" : "只有已完成定价的目录模型可以启用。"}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }}>{statusModel?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog onOpenChange={(open) => { if (!open) setDeletingModel(null); }} open={deletingModel !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除目录模型</AlertDialogTitle><AlertDialogDescription>将删除 {deletingModel?.display_name ?? "该目录模型"} 并同时删除关联路由。历史调用和计费记录会继续保留，此操作不提供恢复入口。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void deleteOneModel(); }} variant="destructive">确认删除</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
      <BulkActionDialog busy={busy} confirmLabel={bulkStatus === "active" ? "确认批量启用" : "确认批量停用"} description={bulkStatus === "active" ? `将启用选中的 ${selection.selectedIds.length} 个目录模型。尚未完成定价的模型会由服务端逐项拒绝，并继续保留在失败选择中。` : `将停用选中的 ${selection.selectedIds.length} 个目录模型，关联路由也会停止转发。`} destructive={bulkStatus === "disabled"} onConfirm={applyBulkStatus} onOpenChange={(open) => { if (!open) setBulkStatus(null); }} open={bulkStatus !== null} title={bulkStatus === "active" ? "批量启用目录模型" : "批量停用目录模型"} />
      <BulkActionDialog busy={busy} confirmLabel="确认批量删除" description={`将删除选中的 ${selection.selectedIds.length} 个目录模型及其关联路由。历史调用和计费记录会继续保留，失败项目仍会保持选中。`} destructive onConfirm={deleteSelected} onOpenChange={setBulkDeleteOpen} open={bulkDeleteOpen} title="批量删除目录模型" />
    </>
  );
}
