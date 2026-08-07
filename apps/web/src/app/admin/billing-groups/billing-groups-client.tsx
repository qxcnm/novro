"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Pencil, Plus, Power, PowerOff, RefreshCw, Search, Trash2, UsersRound } from "lucide-react";
import { useRouter } from "next/navigation";

import { BulkActionDialog, ListBulkActions } from "@/components/list-bulk-actions";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { bulkResultMessage, runBulkAction } from "@/lib/bulk-action";
import { useListSelection } from "@/lib/use-list-selection";

type Group = {
  id: string;
  code: string;
  display_name: string;
  multiplier_bps: number;
  is_default: boolean;
  status: "active" | "disabled";
  user_count: number;
};

type Form = { code: string; display_name: string; multiplier: string };

const emptyForm: Form = { code: "", display_name: "", multiplier: "1" };

async function errorMessage(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

function multiplierBPS(value: string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 && parsed <= 100 ? Math.round(parsed * 10_000) : null;
}

export default function BillingGroupsClient() {
  const router = useRouter();
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [query, setQuery] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<Group | null>(null);
  const [statusGroup, setStatusGroup] = useState<Group | null>(null);
  const [deletingGroup, setDeletingGroup] = useState<Group | null>(null);
  const [form, setForm] = useState<Form>(emptyForm);
  const [bulkStatus, setBulkStatus] = useState<"active" | "disabled" | null>(null);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    const response = await fetch("/api/admin/billing-groups", { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) {
      setMessage(await errorMessage(response));
      setLoading(false);
      return;
    }
    setGroups(((await response.json()) as { billing_groups: Group[] }).billing_groups);
    setLoading(false);
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return needle ? groups.filter((group) => `${group.code} ${group.display_name}`.toLowerCase().includes(needle)) : groups;
  }, [groups, query]);
  const selection = useListSelection(filtered.filter((group) => !group.is_default).map((group) => group.id));

  function beginEdit(group: Group) {
    setEditing(group);
    setForm({ code: group.code, display_name: group.display_name, multiplier: String(group.multiplier_bps / 10_000) });
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const bps = multiplierBPS(form.multiplier);
    if (bps === null) { setMessage("倍率必须大于 0 且不超过 100"); return; }
    const body = editing
      ? { display_name: form.display_name, multiplier_bps: bps }
      : { code: form.code, display_name: form.display_name, multiplier_bps: bps };
    setBusy(true);
    const response = await fetch(editing ? `/api/admin/billing-groups/${editing.id}` : "/api/admin/billing-groups", {
      method: editing ? "PATCH" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    setBusy(false);
    if (!response.ok) { setMessage(await errorMessage(response)); return; }
    setEditing(null);
    setCreateOpen(false);
    setForm(emptyForm);
    setMessage(editing ? "计费分组已更新" : "计费分组已创建");
    await load();
  }

  async function toggleStatus() {
    if (!statusGroup) return;
    const next = statusGroup.status === "active" ? "disabled" : "active";
    setBusy(true);
    const response = await fetch(`/api/admin/billing-groups/${statusGroup.id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: next }),
    });
    setBusy(false);
    if (!response.ok) setMessage(await errorMessage(response));
    else {
      setMessage(next === "active" ? "计费分组已启用" : "计费分组已停用");
      await load();
    }
    setStatusGroup(null);
  }

  async function applyBulkStatus() {
    if (!bulkStatus) return;
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/billing-groups/${id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: bulkStatus }),
    }));
    setBusy(false);
    setBulkStatus(null);
    setMessage(bulkResultMessage(bulkStatus === "active" ? "启用计费分组" : "停用计费分组", result));
    await load();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  async function deleteOneGroup() {
    if (!deletingGroup) return;
    setBusy(true);
    const response = await fetch(`/api/admin/billing-groups/${deletingGroup.id}`, { method: "DELETE" });
    setBusy(false);
    if (!response.ok) setMessage(await errorMessage(response));
    else {
      setMessage("计费分组已删除，历史用量记录已保留");
      await load();
    }
    setDeletingGroup(null);
  }

  async function deleteSelected() {
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/billing-groups/${id}`, { method: "DELETE" }));
    setBusy(false);
    setBulkDeleteOpen(false);
    setMessage(bulkResultMessage("删除计费分组", result));
    await load();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  const fields = <>
    <div className="space-y-2"><Label htmlFor="group-code">分组标识</Label><Input disabled={editing !== null} id="group-code" maxLength={64} onChange={(event) => setForm({ ...form, code: event.target.value })} pattern="[a-z0-9][a-z0-9-]{1,62}[a-z0-9]" placeholder="例如 vip" required value={form.code} /></div>
    <div className="space-y-2"><Label htmlFor="group-name">显示名称</Label><Input id="group-name" maxLength={128} onChange={(event) => setForm({ ...form, display_name: event.target.value })} required value={form.display_name} /></div>
    <div className="space-y-2"><Label htmlFor="group-multiplier">计费倍率</Label><Input id="group-multiplier" inputMode="decimal" max="100" min="0.0001" onChange={(event) => setForm({ ...form, multiplier: event.target.value })} required step="0.0001" type="number" value={form.multiplier} /><p className="text-xs text-muted-foreground">1.0000 表示按上游基础价格计费，1.2000 表示加价 20%。</p></div>
  </>;

  return (
    <div className="space-y-5">
      <section className="grid border-y bg-background sm:grid-cols-3">
        <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">计费分组</p><p className="mt-1 text-xl font-semibold">{groups.length}</p></div>
        <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">已分配用户</p><p className="mt-1 text-xl font-semibold">{groups.reduce((sum, group) => sum + group.user_count, 0)}</p></div>
        <div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">默认倍率</p><p className="mt-1 text-xl font-semibold">{((groups.find((group) => group.is_default)?.multiplier_bps ?? 10_000) / 10_000).toFixed(4)}x</p></div>
      </section>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative max-w-md flex-1"><Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索计费分组" className="pl-8" onChange={(event) => { setQuery(event.target.value); selection.clearSelection(); }} placeholder="搜索标识或名称" value={query} /></div>
        <div className="flex gap-2"><Button aria-label="刷新" disabled={loading} onClick={() => void load()} size="icon" title="刷新" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button><Button onClick={() => { setForm(emptyForm); setCreateOpen(true); }}><Plus />新增分组</Button></div>
      </div>

      {message ? <p className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</p> : null}

      <ListBulkActions onClear={selection.clearSelection} selectedCount={selection.selectedIds.length}>
        <Button disabled={busy} onClick={() => setBulkStatus("active")} size="sm" type="button" variant="outline"><Power />批量启用</Button>
        <Button disabled={busy} onClick={() => setBulkStatus("disabled")} size="sm" type="button" variant="destructive"><PowerOff />批量停用</Button>
        <Button disabled={busy} onClick={() => setBulkDeleteOpen(true)} size="sm" type="button" variant="destructive"><Trash2 />批量删除</Button>
      </ListBulkActions>

      <Card className="overflow-hidden">
        <CardContent className="p-0">
          <Table>
            <TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择所有可操作计费分组" checked={selection.checkboxState} disabled={loading || filtered.filter((group) => !group.is_default).length === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead>分组</TableHead><TableHead>倍率</TableHead><TableHead>用户数</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={6}>加载中...</TableCell></TableRow> : null}
              {!loading && filtered.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={6}>还没有计费分组</TableCell></TableRow> : null}
              {filtered.map((group) => <TableRow key={group.id}>
                <TableCell><Checkbox aria-label={`选择 ${group.display_name}`} checked={selection.isSelected(group.id)} disabled={group.is_default} onCheckedChange={(checked) => selection.toggleOne(group.id, checked === true)} /></TableCell>
                <TableCell><p className="font-medium">{group.display_name} {group.is_default ? <Badge className="ml-1" variant="secondary">默认</Badge> : null}</p><p className="font-mono text-xs text-muted-foreground">{group.code}</p></TableCell>
                <TableCell className="font-mono">{(group.multiplier_bps / 10_000).toFixed(4)}x</TableCell>
                <TableCell><span className="inline-flex items-center gap-1"><UsersRound className="size-4 text-muted-foreground" />{group.user_count}</span></TableCell>
                <TableCell><Badge variant={group.status === "active" ? "outline" : "destructive"}>{group.status === "active" ? "启用" : "停用"}</Badge></TableCell>
                <TableCell><div className="flex justify-end gap-1"><Button aria-label={`编辑 ${group.display_name}`} onClick={() => beginEdit(group)} size="icon-sm" title="编辑" variant="ghost"><Pencil /></Button><Button aria-label={`${group.status === "active" ? "停用" : "启用"} ${group.display_name}`} disabled={group.is_default} onClick={() => setStatusGroup(group)} size="icon-sm" title={group.is_default ? "默认分组不能停用" : group.status === "active" ? "停用" : "启用"} variant="ghost">{group.status === "active" ? <PowerOff /> : <Power />}</Button><Button aria-label={`删除 ${group.display_name}`} disabled={group.is_default} onClick={() => setDeletingGroup(group)} size="icon-sm" title={group.is_default ? "默认分组不能删除" : "删除计费分组"} variant="ghost"><Trash2 /></Button></div></TableCell>
              </TableRow>)}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Dialog onOpenChange={setCreateOpen} open={createOpen}>
        <DialogContent><DialogHeader><DialogTitle>新增计费分组</DialogTitle><DialogDescription>用户归入分组后，后续调用按该倍率结算。</DialogDescription></DialogHeader><form className="space-y-4" id="create-group-form" onSubmit={submit}>{fields}</form><DialogFooter><Button onClick={() => setCreateOpen(false)} variant="outline">取消</Button><Button disabled={busy} form="create-group-form" type="submit"><Plus />创建分组</Button></DialogFooter></DialogContent>
      </Dialog>

      <Sheet onOpenChange={(open) => { if (!open) { setEditing(null); setForm(emptyForm); } }} open={editing !== null}>
        <SheetContent className="w-full sm:max-w-md" side="right"><SheetHeader className="border-b px-6 py-5"><SheetTitle>编辑计费分组</SheetTitle></SheetHeader><form className="space-y-5 px-6" id="edit-group-form" onSubmit={submit}>{fields}</form><SheetFooter className="border-t px-6"><Button disabled={busy} form="edit-group-form" type="submit"><Pencil />保存修改</Button></SheetFooter></SheetContent>
      </Sheet>

      <AlertDialog onOpenChange={(open) => { if (!open) setStatusGroup(null); }} open={statusGroup !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{statusGroup?.status === "active" ? "停用计费分组" : "启用计费分组"}</AlertDialogTitle><AlertDialogDescription>停用后不能再把新用户分配到该分组，已有用户需要迁移到其他启用分组。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }}>{statusGroup?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>

      <AlertDialog onOpenChange={(open) => { if (!open) setDeletingGroup(null); }} open={deletingGroup !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除计费分组</AlertDialogTitle><AlertDialogDescription>将删除 {deletingGroup?.display_name ?? "该计费分组"}。历史用量记录会继续保留；如果仍有用户属于此分组，请先迁移用户。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void deleteOneGroup(); }} variant="destructive">确认删除</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>

      <BulkActionDialog busy={busy} confirmLabel={bulkStatus === "active" ? "确认批量启用" : "确认批量停用"} description={`将${bulkStatus === "active" ? "启用" : "停用"}选中的 ${selection.selectedIds.length} 个计费分组；默认分组不能被批量选中。`} destructive={bulkStatus === "disabled"} onConfirm={applyBulkStatus} onOpenChange={(open) => { if (!open) setBulkStatus(null); }} open={bulkStatus !== null} title={bulkStatus === "active" ? "批量启用计费分组" : "批量停用计费分组"} />
      <BulkActionDialog busy={busy} confirmLabel="确认批量删除" description={`将删除选中的 ${selection.selectedIds.length} 个计费分组。历史用量记录会继续保留；仍有用户的分组会被拒绝并保持选中。`} destructive onConfirm={deleteSelected} onOpenChange={setBulkDeleteOpen} open={bulkDeleteOpen} title="批量删除计费分组" />
    </div>
  );
}
