"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Eye, KeyRound, Pencil, Plus, Power, PowerOff, RefreshCw, Search, ServerCog } from "lucide-react";
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

type Protocol = "openai" | "anthropic";
type ProviderRecord = {
  id: string;
  code: string;
  display_name: string;
  protocol: Protocol;
  base_url: string;
  api_key_hint: string;
  has_api_key: boolean;
  status: "active" | "disabled";
  created_at: string;
  updated_at: string;
};

type ErrorResponse = { error?: { message?: string } };
type CreateForm = { code: string; display_name: string; protocol: Protocol; base_url: string; api_key: string };
type EditForm = { display_name: string; protocol: Protocol; base_url: string; api_key: string };

const INITIAL_CREATE: CreateForm = { code: "", display_name: "", protocol: "openai", base_url: "https://api.openai.com/v1", api_key: "" };

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
  const [providers, setProviders] = useState<ProviderRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [formError, setFormError] = useState("");
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "disabled">("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [detailProvider, setDetailProvider] = useState<ProviderRecord | null>(null);
  const [statusProvider, setStatusProvider] = useState<ProviderRecord | null>(null);
  const [createForm, setCreateForm] = useState<CreateForm>(INITIAL_CREATE);
  const [editForm, setEditForm] = useState<EditForm>({ display_name: "", protocol: "openai", base_url: "", api_key: "" });

  const loadProviders = useCallback(async () => {
    setLoading(true);
    const query = new URLSearchParams();
    if (search) query.set("search", search);
    if (status !== "all") query.set("status", status);
    const response = await fetch(`/api/admin/providers?${query}`, { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) { setMessage(await readError(response)); setLoading(false); return; }
    const body = (await response.json()) as { providers: ProviderRecord[] };
    setProviders(body.providers);
    setLoading(false);
  }, [router, search, status]);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadProviders(), 0);
    return () => window.clearTimeout(timer);
  }, [loadProviders]);

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSearch(searchDraft.trim());
  }

  function openDetails(record: ProviderRecord) {
    setDetailProvider(record);
    setEditForm({ display_name: record.display_name, protocol: record.protocol, base_url: record.base_url, api_key: "" });
    setFormError("");
  }

  async function createProvider(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true); setMessage(""); setFormError("");
    const response = await fetch("/api/admin/providers", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(createForm) });
    setBusy(false);
    if (!response.ok) { setFormError(await readError(response)); return; }
    setCreateForm(INITIAL_CREATE); setCreateOpen(false); setMessage("提供商已创建");
    await loadProviders();
  }

  async function updateProvider(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!detailProvider) return;
    setBusy(true); setMessage(""); setFormError("");
    const payload: { display_name: string; protocol: Protocol; base_url: string; api_key?: string } = {
      display_name: editForm.display_name,
      protocol: editForm.protocol,
      base_url: editForm.base_url,
    };
    if (editForm.api_key.trim()) payload.api_key = editForm.api_key;
    const response = await fetch(`/api/admin/providers/${detailProvider.id}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) });
    setBusy(false);
    if (!response.ok) { setFormError(await readError(response)); return; }
    const body = (await response.json()) as { provider: ProviderRecord };
    setDetailProvider(body.provider);
    setEditForm({ display_name: body.provider.display_name, protocol: body.provider.protocol, base_url: body.provider.base_url, api_key: "" });
    setMessage("提供商配置已更新");
    await loadProviders();
  }

  async function toggleStatus() {
    if (!statusProvider) return;
    setBusy(true); setMessage("");
    const nextStatus = statusProvider.status === "active" ? "disabled" : "active";
    const response = await fetch(`/api/admin/providers/${statusProvider.id}/status`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ status: nextStatus }) });
    setBusy(false);
    if (!response.ok) { setMessage(await readError(response)); setStatusProvider(null); return; }
    setStatusProvider(null); setMessage(nextStatus === "active" ? "提供商已启用" : "提供商已停用");
    await loadProviders();
  }

  const activeCount = providers.filter((provider) => provider.status === "active").length;
  const disabledCount = providers.filter((provider) => provider.status === "disabled").length;

  return (
    <>
      <div className="space-y-5">
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
            <Select onValueChange={(value: "all" | "active" | "disabled") => setStatus(value)} value={status}>
              <SelectTrigger aria-label="按状态筛选" className="w-32"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="active">已启用</SelectItem><SelectItem value="disabled">已停用</SelectItem></SelectContent>
            </Select>
            <Button aria-label="刷新提供商列表" disabled={loading} onClick={() => void loadProviders()} size="icon" title="刷新提供商列表" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
            <Button onClick={() => setCreateOpen(true)}><Plus />添加提供商</Button>
          </div>
        </div>

        {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}

        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader><TableRow><TableHead className="min-w-48">提供商</TableHead><TableHead>协议</TableHead><TableHead className="min-w-64">基础地址</TableHead><TableHead>凭据</TableHead><TableHead>状态</TableHead><TableHead className="min-w-40">更新时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>
                  {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={7}>加载中...</TableCell></TableRow> : null}
                  {!loading && providers.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={7}>没有匹配的提供商</TableCell></TableRow> : null}
                  {!loading ? providers.map((provider) => (
                    <TableRow key={provider.id}>
                      <TableCell><p className="font-medium">{provider.display_name}</p><p className="mt-0.5 font-mono text-xs text-muted-foreground">{provider.code}</p></TableCell>
                      <TableCell><Badge variant="secondary">{protocolLabel(provider.protocol)}</Badge></TableCell>
                      <TableCell><p className="max-w-72 truncate font-mono text-xs" title={provider.base_url}>{provider.base_url}</p></TableCell>
                      <TableCell className="font-mono text-xs">{provider.has_api_key ? `••••${provider.api_key_hint}` : "未配置"}</TableCell>
                      <TableCell><Badge variant={provider.status === "active" ? "outline" : "secondary"}>{provider.status === "active" ? "启用" : "停用"}</Badge></TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(provider.updated_at)}</TableCell>
                      <TableCell><div className="flex justify-end gap-1"><Button aria-label={`查看 ${provider.display_name}`} onClick={() => openDetails(provider)} size="icon-sm" title="查看与编辑" variant="ghost"><Eye /></Button><Button aria-label={`${provider.status === "active" ? "停用" : "启用"} ${provider.display_name}`} onClick={() => setStatusProvider(provider)} size="icon-sm" title={provider.status === "active" ? "停用提供商" : "启用提供商"} variant="ghost">{provider.status === "active" ? <PowerOff /> : <Power />}</Button></div></TableCell>
                    </TableRow>
                  )) : null}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>

        <Dialog onOpenChange={(open) => { setCreateOpen(open); setFormError(""); if (!open) setCreateForm(INITIAL_CREATE); }} open={createOpen}>
          <DialogContent>
            <DialogHeader><DialogTitle>添加提供商</DialogTitle><DialogDescription>提供商代码创建后不可修改，凭据会加密保存。</DialogDescription></DialogHeader>
            <form className="space-y-4" id="create-provider-form" onSubmit={createProvider}>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2"><Label htmlFor="provider-code">代码</Label><Input autoComplete="off" id="provider-code" maxLength={64} minLength={3} onChange={(event) => setCreateForm({ ...createForm, code: event.target.value.toLowerCase() })} pattern="[a-z0-9][a-z0-9-]{1,62}[a-z0-9]" placeholder="例如 openai-main" required value={createForm.code} /></div>
                <div className="space-y-2"><Label htmlFor="provider-name">显示名称</Label><Input id="provider-name" maxLength={128} onChange={(event) => setCreateForm({ ...createForm, display_name: event.target.value })} placeholder="例如 OpenAI 主账号" required value={createForm.display_name} /></div>
              </div>
              <div className="space-y-2"><Label htmlFor="provider-protocol">协议</Label><Select onValueChange={(protocol: Protocol) => setCreateForm({ ...createForm, protocol, base_url: defaultBaseURL(protocol) })} value={createForm.protocol}><SelectTrigger className="w-full" id="provider-protocol"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="openai">OpenAI 兼容</SelectItem><SelectItem value="anthropic">Anthropic</SelectItem></SelectContent></Select></div>
              <div className="space-y-2"><Label htmlFor="provider-base-url">基础地址</Label><Input id="provider-base-url" onChange={(event) => setCreateForm({ ...createForm, base_url: event.target.value })} placeholder="https://api.example.com/v1" required type="url" value={createForm.base_url} /></div>
              <div className="space-y-2"><Label htmlFor="provider-api-key">API Key</Label><Input autoComplete="new-password" id="provider-api-key" maxLength={1024} onChange={(event) => setCreateForm({ ...createForm, api_key: event.target.value })} required type="password" value={createForm.api_key} /></div>
              {formError ? <p className="text-sm text-destructive" role="alert">{formError}</p> : null}
            </form>
            <DialogFooter><Button onClick={() => setCreateOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy} form="create-provider-form" type="submit"><ServerCog />{busy ? "正在创建..." : "创建提供商"}</Button></DialogFooter>
          </DialogContent>
        </Dialog>

        <Sheet onOpenChange={(open) => { setFormError(""); if (!open) setDetailProvider(null); }} open={detailProvider !== null}>
          <SheetContent className="w-full overflow-y-auto sm:max-w-lg" side="right">
            {detailProvider ? <><SheetHeader className="border-b px-6 py-5"><SheetTitle>{detailProvider.display_name}</SheetTitle><SheetDescription>{detailProvider.code}</SheetDescription></SheetHeader>
              <form className="space-y-6 px-6" id="edit-provider-form" onSubmit={updateProvider}>
                <div className="grid grid-cols-2 gap-4 border-b pb-5 text-sm"><div><p className="text-xs text-muted-foreground">状态</p><Badge className="mt-2" variant={detailProvider.status === "active" ? "outline" : "secondary"}>{detailProvider.status === "active" ? "启用" : "停用"}</Badge></div><div><p className="text-xs text-muted-foreground">当前凭据</p><p className="mt-2 font-mono text-xs">{detailProvider.has_api_key ? `••••${detailProvider.api_key_hint}` : "未配置"}</p></div><div><p className="text-xs text-muted-foreground">创建时间</p><p className="mt-2 leading-5">{formatDate(detailProvider.created_at)}</p></div><div><p className="text-xs text-muted-foreground">最近更新</p><p className="mt-2 leading-5">{formatDate(detailProvider.updated_at)}</p></div></div>
                <div className="space-y-2"><Label htmlFor="edit-provider-name">显示名称</Label><Input id="edit-provider-name" maxLength={128} onChange={(event) => setEditForm({ ...editForm, display_name: event.target.value })} required value={editForm.display_name} /></div>
                <div className="space-y-2"><Label htmlFor="edit-provider-protocol">协议</Label><Select onValueChange={(protocol: Protocol) => setEditForm({ ...editForm, protocol })} value={editForm.protocol}><SelectTrigger className="w-full" id="edit-provider-protocol"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="openai">OpenAI 兼容</SelectItem><SelectItem value="anthropic">Anthropic</SelectItem></SelectContent></Select></div>
                <div className="space-y-2"><Label htmlFor="edit-provider-base-url">基础地址</Label><Input id="edit-provider-base-url" onChange={(event) => setEditForm({ ...editForm, base_url: event.target.value })} required type="url" value={editForm.base_url} /></div>
                <div className="space-y-2"><Label htmlFor="edit-provider-api-key">替换 API Key</Label><Input autoComplete="new-password" id="edit-provider-api-key" maxLength={1024} onChange={(event) => setEditForm({ ...editForm, api_key: event.target.value })} placeholder="留空则保留当前凭据" type="password" value={editForm.api_key} /><p className="text-xs text-muted-foreground"><KeyRound aria-hidden="true" className="mr-1 inline size-3" />保存新 Key 后，旧凭据会立即被替换。</p></div>
                {formError ? <p className="text-sm text-destructive" role="alert">{formError}</p> : null}
              </form>
              <SheetFooter className="border-t px-6"><Button disabled={busy} form="edit-provider-form" type="submit"><Pencil />{busy ? "正在保存..." : "保存修改"}</Button><Button onClick={() => { setDetailProvider(null); setStatusProvider(detailProvider); }} type="button" variant="outline">{detailProvider.status === "active" ? <PowerOff /> : <Power />}{detailProvider.status === "active" ? "停用提供商" : "启用提供商"}</Button></SheetFooter></> : null}
          </SheetContent>
        </Sheet>

        <AlertDialog onOpenChange={(open) => { if (!open) setStatusProvider(null); }} open={statusProvider !== null}>
          <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{statusProvider?.status === "active" ? "停用提供商" : "启用提供商"}</AlertDialogTitle><AlertDialogDescription>{statusProvider?.status === "active" ? `停用 ${statusProvider.display_name} 后，该配置不会再处于可用状态。` : `启用 ${statusProvider?.display_name ?? "该提供商"} 后，该配置会重新进入可用状态。`}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }} variant={statusProvider?.status === "active" ? "destructive" : "default"}>{statusProvider?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
        </AlertDialog>
      </div>
    </>
  );
}
