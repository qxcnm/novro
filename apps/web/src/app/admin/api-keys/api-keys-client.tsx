"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { ChevronLeft, ChevronRight, RefreshCw, Search, ShieldX } from "lucide-react";
import { useRouter } from "next/navigation";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { BulkActionDialog, ListBulkActions } from "@/components/list-bulk-actions";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { bulkResultMessage, runBulkAction } from "@/lib/bulk-action";
import { useListSelection } from "@/lib/use-list-selection";

type APIKeyRecord = {
  id: string;
  name: string;
  key_prefix: string;
  status: "active" | "revoked";
  last_used_at: string | null;
  created_at: string;
  revoked_at: string | null;
  owner: { id: string; username: string; display_name: string };
};

type APIKeyPage = { api_keys: APIKeyRecord[]; total: number; offset: number; limit: number };
type ErrorResponse = { error?: { message?: string } };

const PAGE_SIZE = 20;

async function readError(response: Response) {
  const body = (await response.json().catch(() => ({}))) as ErrorResponse;
  return body.error?.message ?? "操作失败，请稍后重试";
}

function formatDate(value: string | null, empty = "从未使用") {
  if (!value) return empty;
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export default function AdminAPIKeysClient() {
  const router = useRouter();
  const [keys, setKeys] = useState<APIKeyRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "revoked">("all");
  const [revokeKey, setRevokeKey] = useState<APIKeyRecord | null>(null);
  const [bulkRevokeOpen, setBulkRevokeOpen] = useState(false);
  const selection = useListSelection(keys.filter((key) => key.status === "active").map((key) => key.id));

  const loadKeys = useCallback(async () => {
    setLoading(true);
    const query = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) });
    if (search) query.set("search", search);
    if (status !== "all") query.set("status", status);
    const response = await fetch(`/api/admin/api-keys?${query}`, { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) { setMessage(await readError(response)); setLoading(false); return; }
    const page = (await response.json()) as APIKeyPage;
    setKeys(page.api_keys);
    setTotal(page.total);
    setLoading(false);
  }, [offset, router, search, status]);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadKeys(), 0);
    return () => window.clearTimeout(timer);
  }, [loadKeys]);

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setOffset(0);
    setSearch(searchDraft.trim());
  }

  async function revoke() {
    if (!revokeKey) return;
    setBusy(true);
    setMessage("");
    const response = await fetch(`/api/admin/api-keys/${revokeKey.id}/revoke`, { method: "POST" });
    setBusy(false);
    if (!response.ok) { setMessage(await readError(response)); setRevokeKey(null); return; }
    setMessage(`已撤销 ${revokeKey.owner.username} 的 API Key`);
    setRevokeKey(null);
    await loadKeys();
  }

  async function revokeSelected() {
    const ids = selection.selectedIds;
    setBusy(true);
    setMessage("");
    const result = await runBulkAction(ids, (id) => fetch(`/api/admin/api-keys/${id}/revoke`, { method: "POST" }));
    setBusy(false);
    setBulkRevokeOpen(false);
    setMessage(bulkResultMessage("撤销", result));
    await loadKeys();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  const activeCount = keys.filter((key) => key.status === "active").length;
  const revokedCount = keys.filter((key) => key.status === "revoked").length;
  const pageStart = total === 0 ? 0 : offset + 1;
  const pageEnd = Math.min(offset + keys.length, total);

  return (
    <>
      <div className="space-y-5">
        <section className="grid border-y bg-background sm:grid-cols-3">
          <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">匹配总数</p><p className="mt-1 text-xl font-semibold">{total}</p></div>
          <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">本页启用</p><p className="mt-1 text-xl font-semibold">{activeCount}</p></div>
          <div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">本页已撤销</p><p className="mt-1 text-xl font-semibold">{revokedCount}</p></div>
        </section>

        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <form className="flex min-w-0 flex-1 gap-2" onSubmit={submitSearch}>
            <div className="relative max-w-md flex-1"><Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索 API Key" className="pl-8" onChange={(event) => setSearchDraft(event.target.value)} placeholder="搜索名称、前缀或用户" value={searchDraft} /></div>
            <Button type="submit" variant="outline">搜索</Button>
          </form>
          <div className="flex items-center gap-2">
            <Select onValueChange={(value: "all" | "active" | "revoked") => { setStatus(value); setOffset(0); }} value={status}>
              <SelectTrigger aria-label="按状态筛选" className="w-32"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="active">已启用</SelectItem><SelectItem value="revoked">已撤销</SelectItem></SelectContent>
            </Select>
            <Button aria-label="刷新 API Key 列表" disabled={loading} onClick={() => void loadKeys()} size="icon" title="刷新 API Key 列表" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
          </div>
        </div>

        {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}

        <ListBulkActions onClear={selection.clearSelection} selectedCount={selection.selectedIds.length}>
          <Button disabled={busy} onClick={() => setBulkRevokeOpen(true)} size="sm" type="button" variant="destructive"><ShieldX />批量撤销</Button>
        </ListBulkActions>

        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择本页所有可撤销 API Key" checked={selection.checkboxState} disabled={loading || activeCount === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead className="min-w-48">API Key</TableHead><TableHead className="min-w-44">所属用户</TableHead><TableHead>状态</TableHead><TableHead className="min-w-40">最近使用</TableHead><TableHead className="min-w-40">创建时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>
                  {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={7}>加载中...</TableCell></TableRow> : null}
                  {!loading && keys.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={7}>没有匹配的 API Key</TableCell></TableRow> : null}
                  {!loading ? keys.map((key) => (
                    <TableRow key={key.id}>
                      <TableCell><Checkbox aria-label={`选择 ${key.owner.username} 的 ${key.name}`} checked={selection.isSelected(key.id)} disabled={key.status !== "active"} onCheckedChange={(checked) => selection.toggleOne(key.id, checked === true)} /></TableCell>
                      <TableCell><p className="font-medium">{key.name}</p><p className="mt-0.5 font-mono text-xs text-muted-foreground">{key.key_prefix}••••</p></TableCell>
                      <TableCell><p>{key.owner.display_name || key.owner.username}</p><p className="mt-0.5 text-xs text-muted-foreground">@{key.owner.username}</p></TableCell>
                      <TableCell><Badge variant={key.status === "active" ? "outline" : "secondary"}>{key.status === "active" ? "启用" : "已撤销"}</Badge></TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(key.last_used_at)}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(key.created_at)}</TableCell>
                      <TableCell><div className="flex justify-end">{key.status === "active" ? <Button aria-label={`撤销 ${key.owner.username} 的 ${key.name}`} onClick={() => setRevokeKey(key)} size="icon-sm" title="撤销 API Key" variant="ghost"><ShieldX /></Button> : null}</div></TableCell>
                    </TableRow>
                  )) : null}
                </TableBody>
              </Table>
            </div>
            <div className="flex flex-col gap-3 border-t px-4 py-3 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
              <span>显示 {pageStart}-{pageEnd}，共 {total} 个 API Key</span>
              <div className="flex gap-1"><Button aria-label="上一页" disabled={offset === 0 || loading} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))} size="icon-sm" title="上一页" variant="outline"><ChevronLeft /></Button><Button aria-label="下一页" disabled={offset + PAGE_SIZE >= total || loading} onClick={() => setOffset(offset + PAGE_SIZE)} size="icon-sm" title="下一页" variant="outline"><ChevronRight /></Button></div>
            </div>
          </CardContent>
        </Card>

        <AlertDialog onOpenChange={(open) => { if (!open) setRevokeKey(null); }} open={revokeKey !== null}>
          <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>撤销 API Key</AlertDialogTitle><AlertDialogDescription>撤销 {revokeKey?.owner.username} 的 {revokeKey?.name} 后无法恢复，使用该 Key 的客户端将立即失去访问权限。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void revoke(); }} variant="destructive">确认撤销</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
        </AlertDialog>
        <BulkActionDialog busy={busy} confirmLabel="确认批量撤销" description={`将撤销选中的 ${selection.selectedIds.length} 个 API Key，撤销后无法恢复，使用这些 Key 的客户端将立即失去访问权限。`} destructive onConfirm={revokeSelected} onOpenChange={setBulkRevokeOpen} open={bulkRevokeOpen} title="批量撤销 API Key" />
      </div>
    </>
  );
}
