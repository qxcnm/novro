"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Check, Clipboard, KeyRound, Plus, RefreshCw, ScrollText, Trash2 } from "lucide-react";
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
import { copyText } from "@/lib/clipboard";
import { useListSelection } from "@/lib/use-list-selection";

type APIKeyRecord = { id: string; billing_group_id: string; billing_group: { id: string; code: string; display_name: string; multiplier_bps: number }; name: string; key_prefix: string; can_copy_secret: boolean; status: "active" | "revoked"; last_used_at: string | null; created_at: string; revoked_at: string | null };
type CreateResult = { api_key: APIKeyRecord; key: string };
type BillingGroup = { id: string; display_name: string; multiplier_bps: number; is_default: boolean; status: "active" | "disabled" };
type ErrorResponse = { error?: { message?: string } };

async function readError(response: Response) {
  const body = (await response.json().catch(() => ({}))) as ErrorResponse;
  return body.error?.message ?? "操作失败，请稍后重试";
}

function formatDate(value: string | null) {
  if (!value) return "从未使用";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export default function APIKeysClient() {
  const router = useRouter();
  const [keys, setKeys] = useState<APIKeyRecord[]>([]);
  const [billingGroups, setBillingGroups] = useState<BillingGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [billingGroupID, setBillingGroupID] = useState("");
  const [created, setCreated] = useState<CreateResult | null>(null);
  const [copied, setCopied] = useState(false);
  const [copiedKeyID, setCopiedKeyID] = useState<string | null>(null);
  const [revokeKey, setRevokeKey] = useState<APIKeyRecord | null>(null);
  const [bulkRevokeOpen, setBulkRevokeOpen] = useState(false);
  const activeKeys = keys.filter((key) => key.status === "active");
  const selection = useListSelection(activeKeys.map((key) => key.id));

  const loadKeys = useCallback(async () => {
    setLoading(true);
    const [response, groupsResponse] = await Promise.all([
      fetch("/api/account/api-keys", { cache: "no-store" }),
      fetch("/api/account/billing-groups", { cache: "no-store" }),
    ]);
    if (response.status === 401) { router.replace("/login"); return; }
    if (!response.ok) { setMessage(await readError(response)); setLoading(false); return; }
    const body = (await response.json()) as { api_keys: APIKeyRecord[] };
    setKeys(body.api_keys);
    if (groupsResponse.ok) {
      setBillingGroups(((await groupsResponse.json()) as { billing_groups: BillingGroup[] }).billing_groups);
    } else {
      setMessage(await readError(groupsResponse));
    }
    setLoading(false);
  }, [router]);

  useEffect(() => { const timer = window.setTimeout(() => void loadKeys(), 0); return () => window.clearTimeout(timer); }, [loadKeys]);

  useEffect(() => {
    if (!copiedKeyID) return;
    const timer = window.setTimeout(() => setCopiedKeyID(null), 1500);
    return () => window.clearTimeout(timer);
  }, [copiedKeyID]);

  async function createKey(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!billingGroupID) {
      setError("请选择计费分组");
      return;
    }
    setBusy(true); setError("");
    const response = await fetch("/api/account/api-keys", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name, billing_group_id: billingGroupID }) });
    setBusy(false);
    if (!response.ok) { setError(await readError(response)); return; }
    const result = (await response.json()) as CreateResult;
    setCreated(result); setName(""); setBillingGroupID(""); await loadKeys();
  }

  async function copyKey() {
    if (!created) return;
    const success = await copyText(created.key);
    if (success) {
      setCopied(true);
      setError("");
    } else {
      setCopied(false);
      setError("复制失败，请手动选择并复制完整密钥");
    }
  }

  async function copyStoredKey(key: APIKeyRecord) {
    if (!key.can_copy_secret) {
      setMessage("该 API Key 没有可重新复制的副本，请重新创建");
      return;
    }
    try {
      const response = await fetch(`/api/account/api-keys/${key.id}/secret`, { cache: "no-store" });
      if (response.status === 401) {
        router.replace("/login");
        return;
      }
      if (!response.ok) {
        setMessage(await readError(response));
        return;
      }
      const body = (await response.json()) as { key?: string };
      if (!body.key) {
        setMessage("该 API Key 没有可重新复制的副本，请重新创建");
        return;
      }
      const success = await copyText(body.key);
      if (success) {
        setCopiedKeyID(key.id);
        setMessage("");
      } else {
        setMessage("复制失败，请手动选择并复制完整密钥");
      }
    } catch {
      setMessage("复制失败，请稍后重试");
    }
  }

  async function revoke() {
    if (!revokeKey) return;
    setBusy(true);
    const response = await fetch(`/api/account/api-keys/${revokeKey.id}`, { method: "DELETE" });
    setBusy(false);
    if (!response.ok) { setMessage(await readError(response)); setRevokeKey(null); return; }
    setMessage("API Key 已删除"); setRevokeKey(null); await loadKeys();
  }

  async function revokeSelected() {
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/account/api-keys/${id}`, { method: "DELETE" }));
    setBusy(false);
    setBulkRevokeOpen(false);
    setMessage(bulkResultMessage("删除", result));
    await loadKeys();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  const activeCount = activeKeys.length;
  const defaultBillingGroupID = billingGroups.find((group) => group.is_default)?.id ?? billingGroups[0]?.id ?? "";

  return (
    <>
      <div className="space-y-5">
        <section className="flex flex-col gap-4 border-y bg-background px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div><p className="text-xs text-muted-foreground">启用中的 Key</p><p className="mt-1 text-xl font-semibold">{activeCount} <span className="text-sm font-normal text-muted-foreground">/ 10</span></p></div>
          <div className="flex gap-2"><Button aria-label="刷新 API Key" disabled={loading} onClick={() => void loadKeys()} size="icon" title="刷新 API Key" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button><Button onClick={() => { setBillingGroupID(defaultBillingGroupID); setCreateOpen(true); }}><Plus />创建 API Key</Button></div>
        </section>
        {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}
        <ListBulkActions onClear={selection.clearSelection} selectedCount={selection.selectedIds.length}><Button disabled={busy} onClick={() => setBulkRevokeOpen(true)} size="sm" type="button" variant="destructive"><Trash2 />批量删除</Button></ListBulkActions>
        <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择所有可撤销 API Key" checked={selection.checkboxState} disabled={loading || activeCount === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead>名称</TableHead><TableHead>计费分组</TableHead><TableHead>前缀</TableHead><TableHead>状态</TableHead><TableHead>最近使用</TableHead><TableHead>创建时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>
          {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={8}>加载中...</TableCell></TableRow> : null}
          {!loading && activeKeys.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={8}>还没有 API Key</TableCell></TableRow> : null}
          {!loading ? activeKeys.map((key) => <TableRow key={key.id}><TableCell><Checkbox aria-label={`选择 ${key.name}`} checked={selection.isSelected(key.id)} onCheckedChange={(checked) => selection.toggleOne(key.id, checked === true)} /></TableCell><TableCell className="font-medium">{key.name}</TableCell><TableCell><p>{key.billing_group?.display_name ?? "默认分组"}</p><p className="font-mono text-xs text-muted-foreground">{((key.billing_group?.multiplier_bps ?? 10_000) / 10_000).toFixed(4)}×</p></TableCell><TableCell className="font-mono text-xs">{key.key_prefix}••••</TableCell><TableCell><Badge variant="outline">启用</Badge></TableCell><TableCell className="text-muted-foreground">{formatDate(key.last_used_at)}</TableCell><TableCell className="text-muted-foreground">{formatDate(key.created_at)}</TableCell><TableCell><div className="flex justify-end gap-1"><Button aria-label={`重新复制 ${key.name} 的 API Key`} disabled={!key.can_copy_secret} onClick={() => void copyStoredKey(key)} size="icon-sm" title={key.can_copy_secret ? "重新复制 API Key" : "该 Key 没有可重新复制的副本"} variant="ghost">{copiedKeyID === key.id ? <Check /> : <Clipboard />}</Button><Button aria-label={`查看 ${key.name} 使用日志`} onClick={() => router.push(`/console/logs?key_id=${key.id}`)} size="icon-sm" title="查看使用日志" variant="ghost"><ScrollText /></Button><Button aria-label={`删除 ${key.name}`} onClick={() => setRevokeKey(key)} size="icon-sm" title="删除 API Key" variant="ghost"><Trash2 /></Button></div></TableCell></TableRow>) : null}
        </TableBody></Table></div></CardContent></Card>

        <Dialog onOpenChange={(open) => { setCreateOpen(open); setError(""); if (!open) { setCreated(null); setCopied(false); setName(""); setBillingGroupID(""); } }} open={createOpen}>
          <DialogContent>{created ? <><DialogHeader><DialogTitle>保存你的 API Key</DialogTitle><DialogDescription>完整密钥只显示这一次。关闭后仍可在列表中重新复制。</DialogDescription></DialogHeader><div className="space-y-3"><Label htmlFor="created-api-key">API Key</Label><div className="flex gap-2"><Input className="font-mono text-xs" id="created-api-key" readOnly value={created.key} /><Button aria-label="复制 API Key" onClick={() => void copyKey()} size="icon" title="复制 API Key" variant="outline">{copied ? <Check /> : <Clipboard />}</Button></div><p className="text-xs text-muted-foreground">计费分组：{created.api_key.billing_group?.display_name ?? "默认分组"} · {((created.api_key.billing_group?.multiplier_bps ?? 10_000) / 10_000).toFixed(4)}×</p>{error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}</div><DialogFooter><Button onClick={() => setCreateOpen(false)}>{copied ? "已保存，关闭" : "我已保存"}</Button></DialogFooter></> : <><DialogHeader><DialogTitle>创建 API Key</DialogTitle><DialogDescription>为不同环境使用独立名称和计费分组，便于单独撤销和成本管理。</DialogDescription></DialogHeader><form className="space-y-4" id="create-key-form" onSubmit={createKey}><div className="space-y-2"><Label htmlFor="key-name">名称</Label><Input autoFocus id="key-name" maxLength={64} onChange={(event) => setName(event.target.value)} placeholder="例如 Production" required value={name} /></div><div className="space-y-2"><Label htmlFor="key-billing-group">计费分组</Label><Select onValueChange={setBillingGroupID} value={billingGroupID}><SelectTrigger id="key-billing-group"><SelectValue placeholder="选择计费分组" /></SelectTrigger><SelectContent>{billingGroups.map((group) => <SelectItem key={group.id} value={group.id}>{group.display_name} · {(group.multiplier_bps / 10_000).toFixed(4)}×</SelectItem>)}</SelectContent></Select></div>{error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}</form><DialogFooter><Button onClick={() => setCreateOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy} form="create-key-form" type="submit"><KeyRound />{busy ? "正在创建..." : "创建"}</Button></DialogFooter></>}</DialogContent>
        </Dialog>

        <AlertDialog onOpenChange={(open) => { if (!open) setRevokeKey(null); }} open={revokeKey !== null}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除 API Key</AlertDialogTitle><AlertDialogDescription>删除 {revokeKey?.name} 后无法恢复，使用该 Key 的客户端将立即失去访问权限。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void revoke(); }} variant="destructive">确认删除</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
        <BulkActionDialog busy={busy} confirmLabel="确认批量删除" description={`将删除选中的 ${selection.selectedIds.length} 个 API Key，删除后无法恢复，使用这些 Key 的客户端将立即失去访问权限。`} destructive onConfirm={revokeSelected} onOpenChange={setBulkRevokeOpen} open={bulkRevokeOpen} title="批量删除 API Key" />
      </div>
    </>
  );
}
