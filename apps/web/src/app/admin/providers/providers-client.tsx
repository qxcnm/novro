"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { CloudDownload, KeyRound, Link2, Pencil, Plus, Power, PowerOff, RefreshCw, Route, Search, ServerCog, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";

import ModelRoutesClient from "@/app/admin/models/model-routes-client";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { BulkActionDialog, ListBulkActions } from "@/components/list-bulk-actions";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { bulkResultMessage, runBulkAction } from "@/lib/bulk-action";
import { useListSelection } from "@/lib/use-list-selection";

type Protocol = "openai" | "anthropic";
type ProviderRecord = {
  id: string;
  billing_group_id: string;
  billing_group: { id: string; code: string; display_name: string; multiplier_bps: number };
  code: string;
  display_name: string;
  protocol: Protocol;
  base_url: string;
  model_list_path: string;
  api_key_hint: string;
  has_api_key: boolean;
  status: "active" | "disabled";
  created_at: string;
  updated_at: string;
};

type PickerModel = {
  id: string;
  provider_name: string;
  upstream_name: string;
  display_name: string;
  pricing_configured: boolean;
  status: "active" | "disabled";
  added?: boolean;
  restored?: boolean;
};

type RouteRecord = {
  provider_id: string;
  upstream_model_id: string | null;
};

type ErrorResponse = { error?: { message?: string } };
type BillingGroup = { id: string; display_name: string; multiplier_bps: number; is_default: boolean; status: "active" | "disabled" };
type CreateForm = { code: string; display_name: string; protocol: Protocol; base_url: string; model_list_path: string; api_key: string; billing_group_id: string };
type EditForm = { display_name: string; protocol: Protocol; base_url: string; model_list_path: string; api_key: string; billing_group_id: string };

const INITIAL_CREATE: CreateForm = {
  code: "",
  display_name: "",
  protocol: "openai",
  base_url: "https://api.openai.com/v1",
  model_list_path: "",
  api_key: "",
  billing_group_id: "",
};

const MODEL_SYNC_TIMEOUT_MS = 35_000;

async function fetchWithTimeout(input: RequestInfo | URL, init: RequestInit = {}, timeoutMs = MODEL_SYNC_TIMEOUT_MS) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(input, { ...init, signal: controller.signal });
  } finally {
    window.clearTimeout(timer);
  }
}

async function readError(response: Response) {
  const body = (await response.json().catch(() => ({}))) as ErrorResponse;
  return body.error?.message ?? "操作失败，请稍后重试";
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function protocolLabel(protocol: Protocol) {
  return protocol === "openai" ? "OpenAI 兼容" : "Anthropic";
}

function defaultBaseURL(protocol: Protocol) {
  return protocol === "openai" ? "https://api.openai.com/v1" : "https://api.anthropic.com";
}

export default function ProvidersClient() {
  const router = useRouter();
  const [activeTab, setActiveTab] = useState("providers");
  const [routeRefreshKey, setRouteRefreshKey] = useState(0);
  const [providers, setProviders] = useState<ProviderRecord[]>([]);
  const [billingGroups, setBillingGroups] = useState<BillingGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [formError, setFormError] = useState("");
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "disabled">("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<ProviderRecord | null>(null);
  const [statusProvider, setStatusProvider] = useState<ProviderRecord | null>(null);
  const [deletingProvider, setDeletingProvider] = useState<ProviderRecord | null>(null);
  const [bulkStatus, setBulkStatus] = useState<"active" | "disabled" | null>(null);
  const [bulkSyncOpen, setBulkSyncOpen] = useState(false);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);
  const [createForm, setCreateForm] = useState<CreateForm>(INITIAL_CREATE);
  const [editForm, setEditForm] = useState<EditForm>({ display_name: "", protocol: "openai", base_url: "", model_list_path: "", api_key: "", billing_group_id: "" });
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerProvider, setPickerProvider] = useState<ProviderRecord | null>(null);
  const [pickerModels, setPickerModels] = useState<PickerModel[]>([]);
  const [pickerLoading, setPickerLoading] = useState(false);
  const [pickerNotice, setPickerNotice] = useState("");
  const [pickerQuery, setPickerQuery] = useState("");
  const [selectedModelIDs, setSelectedModelIDs] = useState<Set<string>>(new Set());
  const [linkedModelIDs, setLinkedModelIDs] = useState<Set<string>>(new Set());
  const selection = useListSelection(providers.map((provider) => provider.id));

  const loadProviders = useCallback(async () => {
    setLoading(true);
    const query = new URLSearchParams();
    if (search) query.set("search", search);
    if (status !== "all") query.set("status", status);
    const [response, groupsResponse] = await Promise.all([
      fetch(`/api/admin/providers?${query}`, { cache: "no-store" }),
      fetch("/api/admin/billing-groups", { cache: "no-store" }),
    ]);
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) { setMessage(await readError(response)); setLoading(false); return; }
    setProviders(((await response.json()) as { providers: ProviderRecord[] }).providers);
    if (groupsResponse.ok) {
      setBillingGroups(((await groupsResponse.json()) as { billing_groups: BillingGroup[] }).billing_groups);
    } else {
      setMessage(await readError(groupsResponse));
    }
    setLoading(false);
  }, [router, search, status]);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadProviders(), 0);
    return () => window.clearTimeout(timer);
  }, [loadProviders]);

  const visiblePickerModels = useMemo(() => {
    const needle = pickerQuery.trim().toLowerCase();
    return needle
      ? pickerModels.filter((model) => `${model.provider_name} ${model.display_name} ${model.upstream_name}`.toLowerCase().includes(needle))
      : pickerModels;
  }, [pickerModels, pickerQuery]);

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    selection.clearSelection();
    setSearch(searchDraft.trim());
  }

  function openEditor(record: ProviderRecord) {
    setEditingProvider(record);
    setEditForm({ display_name: record.display_name, protocol: record.protocol, base_url: record.base_url, model_list_path: record.model_list_path, api_key: "", billing_group_id: record.billing_group_id });
    setFormError("");
  }

  async function loadPickerData(record: ProviderRecord, sync: boolean) {
    setPickerProvider(record);
    setPickerOpen(true);
    setPickerLoading(true);
    setPickerNotice(sync ? "正在同步模型..." : "");
    setPickerQuery("");
    setPickerModels([]);
    setSelectedModelIDs(new Set());
    setLinkedModelIDs(new Set());

	try {
	  let syncedIDs = new Set<string>();
	  let syncedModels: PickerModel[] | null = null;
	  if (sync) {
	    try {
	      const syncResponse = await fetchWithTimeout(`/api/admin/providers/${record.id}/models/sync`, { method: "POST" });
	      if (syncResponse.ok) {
				const body = (await syncResponse.json()) as { models: PickerModel[] };
				syncedModels = body.models;
				syncedIDs = new Set(body.models.map((model) => model.id));
			const added = body.models.filter((model) => model.added === true).length;
			const restored = body.models.filter((model) => model.restored === true).length;
			const pendingPricing = body.models.filter((model) => model.pricing_configured !== true).length;
			setPickerNotice(`已检查 ${body.models.length} 个模型：新建 ${added} 个目录记录${restored > 0 ? `，恢复 ${restored} 个` : ""}${pendingPricing > 0 ? `，${pendingPricing} 个等待在模型目录维护价格` : ""}`);
          } else {
            setPickerNotice(await readError(syncResponse));
				}
				} catch (error) {
					setPickerNotice(error instanceof DOMException && error.name === "AbortError" ? "同步模型超时，请检查上游地址和模型获取路径" : "同步模型失败，请检查上游连接");
				}
			}

			if (sync && syncedModels === null) {
				setPickerModels([]);
				return;
			}
			const routesResponse = await fetchWithTimeout("/api/admin/model-routes", { cache: "no-store" }, 15_000);
			if (!routesResponse.ok) {
				setPickerNotice("加载模型路由失败，请稍后重试");
				return;
			}
			const routes = ((await routesResponse.json()) as { model_routes: RouteRecord[] }).model_routes;
			const linked = new Set(routes.filter((route) => route.provider_id === record.id && route.upstream_model_id).map((route) => route.upstream_model_id as string));
			if (syncedModels !== null) {
				setPickerModels(syncedModels);
			} else {
				const catalogResponse = await fetchWithTimeout("/api/admin/upstream-models", { cache: "no-store" }, 15_000);
				if (!catalogResponse.ok) {
					setPickerNotice("加载模型目录失败，请稍后重试");
					return;
				}
				setPickerModels(((await catalogResponse.json()) as { upstream_models: PickerModel[] }).upstream_models);
			}
			setLinkedModelIDs(linked);
      setSelectedModelIDs(new Set([...syncedIDs].filter((id) => !linked.has(id))));
    } catch {
      setPickerNotice("加载模型目录失败，请稍后重试");
    } finally {
      setPickerLoading(false);
    }
  }

  async function createProvider(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!createForm.billing_group_id) {
      setFormError("请选择计费分组");
      return;
    }
    setBusy(true);
    setMessage("");
    setFormError("");
    const response = await fetch("/api/admin/providers", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(createForm),
    });
    setBusy(false);
    if (!response.ok) { setFormError(await readError(response)); return; }
    const created = ((await response.json()) as { provider: ProviderRecord }).provider;
    setCreateForm(INITIAL_CREATE);
    setCreateOpen(false);
    setMessage("提供商已创建");
    await loadProviders();
    await loadPickerData(created, true);
  }

  async function updateProvider(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editingProvider) return;
    setBusy(true);
    setMessage("");
    setFormError("");
    const payload: { display_name: string; protocol: Protocol; base_url: string; model_list_path: string; api_key?: string; billing_group_id?: string } = {
      display_name: editForm.display_name,
      protocol: editForm.protocol,
      base_url: editForm.base_url,
      model_list_path: editForm.model_list_path,
    };
    if (editForm.api_key.trim()) payload.api_key = editForm.api_key;
    if (editForm.billing_group_id && editForm.billing_group_id !== editingProvider.billing_group_id) payload.billing_group_id = editForm.billing_group_id;
    const response = await fetch(`/api/admin/providers/${editingProvider.id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    setBusy(false);
    if (!response.ok) { setFormError(await readError(response)); return; }
    setEditingProvider(null);
    setMessage("提供商配置已更新");
    await loadProviders();
  }

  async function toggleStatus() {
    if (!statusProvider) return;
    setBusy(true);
    setMessage("");
    const nextStatus = statusProvider.status === "active" ? "disabled" : "active";
    const response = await fetch(`/api/admin/providers/${statusProvider.id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: nextStatus }),
    });
    setBusy(false);
    if (!response.ok) {
      setMessage(await readError(response));
      setStatusProvider(null);
      return;
    }
    setStatusProvider(null);
    setMessage(nextStatus === "active" ? "提供商已启用" : "提供商已停用");
    await loadProviders();
  }

  async function applyBulkStatus() {
    if (!bulkStatus) return;
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/providers/${id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: bulkStatus }),
    }));
    setBusy(false);
    setBulkStatus(null);
    setMessage(bulkResultMessage(bulkStatus === "active" ? "启用提供商" : "停用提供商", result));
    await loadProviders();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  async function syncSelected() {
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/providers/${id}/models/sync`, { method: "POST" }));
    setBusy(false);
    setBulkSyncOpen(false);
    setMessage(bulkResultMessage("同步提供商模型", result));
    setRouteRefreshKey((value) => value + 1);
    await loadProviders();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  async function deleteOneProvider() {
    if (!deletingProvider) return;
    setBusy(true);
    const response = await fetch(`/api/admin/providers/${deletingProvider.id}`, { method: "DELETE" });
    setBusy(false);
    if (!response.ok) {
      setMessage(await readError(response));
      setDeletingProvider(null);
      return;
    }
    setDeletingProvider(null);
    setMessage("提供商已删除，关联路由已一并移出列表");
    setRouteRefreshKey((value) => value + 1);
    await loadProviders();
  }

  async function deleteSelected() {
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/providers/${id}`, { method: "DELETE" }));
    setBusy(false);
    setBulkDeleteOpen(false);
    setMessage(bulkResultMessage("删除提供商", result));
    setRouteRefreshKey((value) => value + 1);
    await loadProviders();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  function toggleModel(modelID: string, checked: boolean) {
    setSelectedModelIDs((current) => {
      const next = new Set(current);
      if (checked) next.add(modelID); else next.delete(modelID);
      return next;
    });
  }

  function selectAllAvailable() {
    setSelectedModelIDs(new Set(visiblePickerModels.filter((model) => !linkedModelIDs.has(model.id)).map((model) => model.id)));
  }

  async function linkSelectedModels() {
    if (!pickerProvider || selectedModelIDs.size === 0) {
      setPickerNotice("请选择至少一个模型");
      return;
    }
    setBusy(true);
    const response = await fetch(`/api/admin/providers/${pickerProvider.id}/models`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ model_ids: [...selectedModelIDs] }),
    });
    setBusy(false);
    if (!response.ok) { setPickerNotice(await readError(response)); return; }
	const result = (await response.json()) as { created: number; existing: number; reenabled: number; disabled: number };
	setPickerOpen(false);
	setMessage(`已创建 ${result.created} 条模型路由${result.reenabled > 0 ? `，恢复 ${result.reenabled} 条` : ""}${result.disabled > 0 ? `，其中 ${result.disabled} 条等待模型定价或启用` : ""}`);
    setRouteRefreshKey((value) => value + 1);
    setActiveTab("routes");
  }

  const activeCount = providers.filter((item) => item.status === "active").length;
  const disabledCount = providers.filter((item) => item.status === "disabled").length;
  const activeBillingGroups = billingGroups.filter((group) => group.status === "active");
  const defaultBillingGroupID = activeBillingGroups.find((group) => group.is_default)?.id ?? activeBillingGroups[0]?.id ?? "";

  return (
    <Tabs onValueChange={setActiveTab} value={activeTab}>
      <TabsList variant="line">
        <TabsTrigger value="providers"><ServerCog />提供商配置</TabsTrigger>
        <TabsTrigger value="routes"><Route />关联模型路由配置</TabsTrigger>
      </TabsList>

      <TabsContent className="space-y-5 pt-3" value="providers">
        <section className="grid border-y bg-background sm:grid-cols-3">
          <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">匹配总数</p><p className="mt-1 text-xl font-semibold">{providers.length}</p></div>
          <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">已启用</p><p className="mt-1 text-xl font-semibold">{activeCount}</p></div>
          <div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">已停用</p><p className="mt-1 text-xl font-semibold">{disabledCount}</p></div>
        </section>

        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <form className="flex min-w-0 flex-1 gap-2" onSubmit={submitSearch}>
            <div className="relative max-w-md flex-1"><Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索提供商" className="pl-8" onChange={(event) => setSearchDraft(event.target.value)} placeholder="搜索名称、代码或端点" value={searchDraft} /></div>
            <Button type="submit" variant="outline">搜索</Button>
          </form>
          <div className="flex flex-wrap items-center gap-2">
            <Select onValueChange={(value: "all" | "active" | "disabled") => { selection.clearSelection(); setStatus(value); }} value={status}>
              <SelectTrigger aria-label="按状态筛选" className="w-32"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="active">已启用</SelectItem><SelectItem value="disabled">已停用</SelectItem></SelectContent>
            </Select>
            <Button aria-label="刷新提供商列表" disabled={loading} onClick={() => void loadProviders()} size="icon" title="刷新提供商列表" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
            <Button onClick={() => { setCreateForm({ ...INITIAL_CREATE, billing_group_id: defaultBillingGroupID }); setCreateOpen(true); }}><Plus />添加提供商</Button>
          </div>
        </div>

        {message ? <div className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}

        <ListBulkActions onClear={selection.clearSelection} selectedCount={selection.selectedIds.length}>
          <Button disabled={busy} onClick={() => setBulkStatus("active")} size="sm" type="button" variant="outline"><Power />批量启用</Button>
          <Button disabled={busy} onClick={() => setBulkStatus("disabled")} size="sm" type="button" variant="destructive"><PowerOff />批量停用</Button>
          <Button disabled={busy} onClick={() => setBulkSyncOpen(true)} size="sm" type="button" variant="outline"><CloudDownload />批量同步模型</Button>
          <Button disabled={busy} onClick={() => setBulkDeleteOpen(true)} size="sm" type="button" variant="destructive"><Trash2 />批量删除</Button>
        </ListBulkActions>

        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择所有提供商" checked={selection.checkboxState} disabled={loading || providers.length === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead className="min-w-48">提供商</TableHead><TableHead>计费分组</TableHead><TableHead>协议</TableHead><TableHead className="min-w-64">基础地址</TableHead><TableHead>凭据</TableHead><TableHead>状态</TableHead><TableHead className="min-w-40">更新时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>
                  {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={9}>加载中...</TableCell></TableRow> : null}
                  {!loading && providers.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={9}>没有匹配的提供商</TableCell></TableRow> : null}
                  {!loading ? providers.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell><Checkbox aria-label={`选择提供商 ${item.display_name}`} checked={selection.isSelected(item.id)} onCheckedChange={(checked) => selection.toggleOne(item.id, checked === true)} /></TableCell>
                      <TableCell><p className="font-medium">{item.display_name}</p><p className="mt-0.5 font-mono text-xs text-muted-foreground">{item.code}</p></TableCell>
                      <TableCell><p>{item.billing_group?.display_name ?? "未分组"}</p><p className="font-mono text-xs text-muted-foreground">{((item.billing_group?.multiplier_bps ?? 10_000) / 10_000).toFixed(4)}×</p></TableCell>
                      <TableCell><Badge variant="secondary">{protocolLabel(item.protocol)}</Badge></TableCell>
                      <TableCell><p className="max-w-72 truncate font-mono text-xs" title={item.base_url}>{item.base_url}</p></TableCell>
                      <TableCell className="font-mono text-xs">{item.has_api_key ? `••••${item.api_key_hint}` : "未配置"}</TableCell>
                      <TableCell><Badge variant={item.status === "active" ? "outline" : "secondary"}>{item.status === "active" ? "启用" : "停用"}</Badge></TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(item.updated_at)}</TableCell>
                      <TableCell><div className="flex justify-end gap-1"><Button aria-label={`同步 ${item.display_name} 的模型`} onClick={() => void loadPickerData(item, true)} size="icon-sm" title="同步模型" variant="ghost"><CloudDownload /></Button><Button aria-label={`关联 ${item.display_name} 的模型`} onClick={() => void loadPickerData(item, false)} size="icon-sm" title="关联目录模型" variant="ghost"><Link2 /></Button><Button aria-label={`编辑 ${item.display_name}`} onClick={() => openEditor(item)} size="icon-sm" title="编辑提供商" variant="ghost"><Pencil /></Button><Button aria-label={`${item.status === "active" ? "停用" : "启用"} ${item.display_name}`} onClick={() => setStatusProvider(item)} size="icon-sm" title={item.status === "active" ? "停用提供商" : "启用提供商"} variant="ghost">{item.status === "active" ? <PowerOff /> : <Power />}</Button><Button aria-label={`删除 ${item.display_name}`} onClick={() => setDeletingProvider(item)} size="icon-sm" title="删除提供商" variant="ghost"><Trash2 /></Button></div></TableCell>
                    </TableRow>
                  )) : null}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent className="pt-3" value="routes"><ModelRoutesClient refreshKey={routeRefreshKey} /></TabsContent>

      <Dialog onOpenChange={(open) => { setCreateOpen(open); setFormError(""); if (!open) setCreateForm(INITIAL_CREATE); }} open={createOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>添加提供商</DialogTitle><DialogDescription>提供商代码创建后不可修改，凭据会加密保存。</DialogDescription></DialogHeader>
          <form className="space-y-4" id="create-provider-form" onSubmit={createProvider}>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="provider-code">代码</Label><Input autoComplete="off" id="provider-code" maxLength={64} minLength={3} onChange={(event) => setCreateForm({ ...createForm, code: event.target.value.toLowerCase() })} pattern="[a-z0-9][a-z0-9.-]{1,62}[a-z0-9]" placeholder="例如 kimi-0.2" required value={createForm.code} /><p className="text-xs text-muted-foreground">支持小写字母、数字、点和连字符，不能以点或连字符开头或结尾。</p></div>
              <div className="space-y-2"><Label htmlFor="provider-name">显示名称</Label><Input id="provider-name" maxLength={128} onChange={(event) => setCreateForm({ ...createForm, display_name: event.target.value })} placeholder="例如 DeepSeek 主账号" required value={createForm.display_name} /></div>
            </div>
            <div className="space-y-2"><Label htmlFor="provider-billing-group">计费分组</Label><Select onValueChange={(billing_group_id) => setCreateForm({ ...createForm, billing_group_id })} value={createForm.billing_group_id}><SelectTrigger className="w-full" id="provider-billing-group"><SelectValue placeholder="选择计费分组" /></SelectTrigger><SelectContent>{activeBillingGroups.map((group) => <SelectItem key={group.id} value={group.id}>{group.display_name} · {(group.multiplier_bps / 10_000).toFixed(4)}×</SelectItem>)}</SelectContent></Select></div>
            <div className="space-y-2"><Label htmlFor="provider-protocol">协议</Label><Select onValueChange={(protocol: Protocol) => setCreateForm({ ...createForm, protocol, base_url: defaultBaseURL(protocol) })} value={createForm.protocol}><SelectTrigger className="w-full" id="provider-protocol"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="openai">OpenAI 兼容</SelectItem><SelectItem value="anthropic">Anthropic</SelectItem></SelectContent></Select></div>
            <div className="space-y-2"><Label htmlFor="provider-base-url">基础地址</Label><Input id="provider-base-url" onChange={(event) => setCreateForm({ ...createForm, base_url: event.target.value })} placeholder="https://api.example.com/v1" required type="url" value={createForm.base_url} /></div>
            <div className="space-y-2"><Label htmlFor="provider-model-list-path">模型获取路径</Label><Input id="provider-model-list-path" onChange={(event) => setCreateForm({ ...createForm, model_list_path: event.target.value })} placeholder="留空默认使用 /v1/models，例如 /api/models" value={createForm.model_list_path} /><p className="text-xs text-muted-foreground">填写站点路径，以 / 开头；留空时 OpenAI 兼容和 Anthropic 都使用基础地址 + /v1/models。</p></div>
            <div className="space-y-2"><Label htmlFor="provider-api-key">API Key</Label><Input autoComplete="new-password" id="provider-api-key" maxLength={1024} onChange={(event) => setCreateForm({ ...createForm, api_key: event.target.value })} required type="password" value={createForm.api_key} /></div>
            {formError ? <p className="text-sm text-destructive" role="alert">{formError}</p> : null}
          </form>
          <DialogFooter><Button onClick={() => setCreateOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy} form="create-provider-form" type="submit"><ServerCog />{busy ? "正在创建..." : "创建提供商"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => { setFormError(""); if (!open) setEditingProvider(null); }} open={editingProvider !== null}>
        <DialogContent>
          <DialogHeader><DialogTitle>编辑提供商</DialogTitle><DialogDescription>{editingProvider?.code}</DialogDescription></DialogHeader>
          {editingProvider ? <form className="space-y-4" id="edit-provider-form" onSubmit={updateProvider}>
            <div className="grid grid-cols-2 gap-4 border-y py-4 text-sm"><div><p className="text-xs text-muted-foreground">状态</p><Badge className="mt-2" variant={editingProvider.status === "active" ? "outline" : "secondary"}>{editingProvider.status === "active" ? "启用" : "停用"}</Badge></div><div><p className="text-xs text-muted-foreground">当前凭据</p><p className="mt-2 font-mono text-xs">{editingProvider.has_api_key ? `••••${editingProvider.api_key_hint}` : "未配置"}</p></div></div>
            <div className="space-y-2"><Label htmlFor="edit-provider-name">显示名称</Label><Input id="edit-provider-name" maxLength={128} onChange={(event) => setEditForm({ ...editForm, display_name: event.target.value })} required value={editForm.display_name} /></div>
            <div className="space-y-2"><Label htmlFor="edit-provider-billing-group">计费分组</Label><Select onValueChange={(billing_group_id) => setEditForm({ ...editForm, billing_group_id })} value={editForm.billing_group_id}><SelectTrigger className="w-full" id="edit-provider-billing-group"><SelectValue placeholder="选择计费分组" /></SelectTrigger><SelectContent>{billingGroups.filter((group) => group.status === "active" || group.id === editingProvider.billing_group_id).map((group) => <SelectItem key={group.id} value={group.id}>{group.display_name} · {(group.multiplier_bps / 10_000).toFixed(4)}×{group.status === "disabled" ? " · 已停用" : ""}</SelectItem>)}</SelectContent></Select></div>
            <div className="space-y-2"><Label htmlFor="edit-provider-protocol">协议</Label><Select onValueChange={(protocol: Protocol) => setEditForm({ ...editForm, protocol })} value={editForm.protocol}><SelectTrigger className="w-full" id="edit-provider-protocol"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="openai">OpenAI 兼容</SelectItem><SelectItem value="anthropic">Anthropic</SelectItem></SelectContent></Select></div>
            <div className="space-y-2"><Label htmlFor="edit-provider-base-url">基础地址</Label><Input id="edit-provider-base-url" onChange={(event) => setEditForm({ ...editForm, base_url: event.target.value })} required type="url" value={editForm.base_url} /></div>
            <div className="space-y-2"><Label htmlFor="edit-provider-model-list-path">模型获取路径</Label><Input id="edit-provider-model-list-path" onChange={(event) => setEditForm({ ...editForm, model_list_path: event.target.value })} placeholder="留空默认使用 /v1/models" value={editForm.model_list_path} /><p className="text-xs text-muted-foreground">例如 /api/models。留空时同步模型默认使用基础地址 + /v1/models。</p></div>
            <div className="space-y-2"><Label htmlFor="edit-provider-api-key">替换 API Key</Label><Input autoComplete="new-password" id="edit-provider-api-key" maxLength={1024} onChange={(event) => setEditForm({ ...editForm, api_key: event.target.value })} placeholder="留空则保留当前凭据" type="password" value={editForm.api_key} /><p className="text-xs text-muted-foreground"><KeyRound aria-hidden="true" className="mr-1 inline size-3" />保存新 Key 后，旧凭据会立即被替换。</p></div>
            {formError ? <p className="text-sm text-destructive" role="alert">{formError}</p> : null}
          </form> : null}
          <DialogFooter><Button onClick={() => setEditingProvider(null)} type="button" variant="outline">取消</Button><Button disabled={busy} form="edit-provider-form" type="submit"><Pencil />{busy ? "正在保存..." : "保存修改"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => { setPickerOpen(open); if (!open) { setPickerProvider(null); setPickerModels([]); setSelectedModelIDs(new Set()); } }} open={pickerOpen}>
        <DialogContent className="max-h-[90vh] overflow-hidden sm:max-w-2xl">
          <DialogHeader><DialogTitle>关联目录模型</DialogTitle><DialogDescription>{pickerProvider ? `${pickerProvider.display_name} · ${pickerProvider.code}` : ""}</DialogDescription></DialogHeader>
          <div className="flex min-h-0 flex-col gap-3">
            <div className="flex items-center gap-2"><div className="relative flex-1"><Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索可关联模型" className="pl-8" onChange={(event) => setPickerQuery(event.target.value)} placeholder="搜索模型目录" value={pickerQuery} /></div><Button disabled={pickerLoading} onClick={selectAllAvailable} type="button" variant="outline">全选</Button></div>
            {pickerNotice ? <p className="border-y px-3 py-2 text-sm" role="status">{pickerNotice}</p> : null}
            <div className="max-h-[46vh] overflow-y-auto border-y">
              {pickerLoading ? <p className="px-4 py-10 text-center text-sm text-muted-foreground">正在加载...</p> : null}
              {!pickerLoading && visiblePickerModels.length === 0 ? <p className="px-4 py-10 text-center text-sm text-muted-foreground">模型目录为空</p> : null}
              {!pickerLoading ? visiblePickerModels.map((model) => {
                const linked = linkedModelIDs.has(model.id);
                return <label className="flex cursor-pointer items-start gap-3 border-b px-4 py-3 last:border-b-0 has-disabled:cursor-default has-disabled:opacity-60" htmlFor={`provider-model-${model.id}`} key={model.id}><Checkbox checked={linked || selectedModelIDs.has(model.id)} disabled={linked} id={`provider-model-${model.id}`} onCheckedChange={(checked) => toggleModel(model.id, checked === true)} /><span className="min-w-0 flex-1"><span className="flex flex-wrap items-center gap-2"><span className="font-medium">{model.display_name}</span>{linked ? <Badge variant="secondary">已关联</Badge> : !model.pricing_configured ? <Badge variant="destructive">待定价</Badge> : model.status === "disabled" ? <Badge variant="secondary">已停用</Badge> : null}</span><span className="mt-1 block truncate text-xs text-muted-foreground">统一模型 ID · {model.upstream_name} · 厂商标签 {model.provider_name}</span></span></label>;
              }) : null}
            </div>
          </div>
          <DialogFooter><Button onClick={() => setPickerOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy || pickerLoading || selectedModelIDs.size === 0} onClick={() => void linkSelectedModels()} type="button"><Link2 />{busy ? "正在关联..." : `关联所选模型（${selectedModelIDs.size}）`}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog onOpenChange={(open) => { if (!open) setStatusProvider(null); }} open={statusProvider !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{statusProvider?.status === "active" ? "停用提供商" : "启用提供商"}</AlertDialogTitle><AlertDialogDescription>{statusProvider?.status === "active" ? `停用 ${statusProvider.display_name} 后，该配置关联的路由将停止转发。` : `启用 ${statusProvider?.display_name ?? "该提供商"} 后，已启用的关联路由可以恢复转发。`}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }} variant={statusProvider?.status === "active" ? "destructive" : "default"}>{statusProvider?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
      <AlertDialog onOpenChange={(open) => { if (!open) setDeletingProvider(null); }} open={deletingProvider !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除提供商</AlertDialogTitle><AlertDialogDescription>将删除 {deletingProvider?.display_name ?? "该提供商"} 并同时删除其关联路由。历史调用记录会继续保留，此操作不提供恢复入口。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void deleteOneProvider(); }} variant="destructive">确认删除</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
      <BulkActionDialog busy={busy} confirmLabel={bulkStatus === "active" ? "确认批量启用" : "确认批量停用"} description={`将${bulkStatus === "active" ? "启用" : "停用"}选中的 ${selection.selectedIds.length} 个提供商配置；关联路由会继续遵循服务端状态校验。`} destructive={bulkStatus === "disabled"} onConfirm={applyBulkStatus} onOpenChange={(open) => { if (!open) setBulkStatus(null); }} open={bulkStatus !== null} title={bulkStatus === "active" ? "批量启用提供商" : "批量停用提供商"} />
      <BulkActionDialog busy={busy} confirmLabel="确认批量同步" description={`将为选中的 ${selection.selectedIds.length} 个提供商请求模型目录同步；不支持模型列表或同步失败的项目会继续保留在失败选择中。`} onConfirm={syncSelected} onOpenChange={setBulkSyncOpen} open={bulkSyncOpen} title="批量同步提供商模型" />
      <BulkActionDialog busy={busy} confirmLabel="确认批量删除" description={`将删除选中的 ${selection.selectedIds.length} 个提供商及其关联路由。历史调用记录会继续保留，失败项目仍会保持选中。`} destructive onConfirm={deleteSelected} onOpenChange={setBulkDeleteOpen} open={bulkDeleteOpen} title="批量删除提供商" />
    </Tabs>
  );
}
