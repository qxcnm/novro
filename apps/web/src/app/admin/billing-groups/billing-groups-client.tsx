"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { KeyRound, Pencil, Plus, Power, PowerOff, RefreshCw, Search, ServerCog, Trash2, UsersRound } from "lucide-react";
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
  is_hidden: boolean;
  status: "active" | "disabled";
  api_key_count: number;
  provider_count: number;
  authorized_users: AuthorizedUser[] | null;
};

type AuthorizedUser = {
  id: string;
  username: string;
  display_name: string;
  status: "active" | "disabled";
};

type UserOption = AuthorizedUser & { email: string; role: "admin" | "member" };
type UserPage = { users: UserOption[]; total: number; offset: number; limit: number };

type Form = { code: string; display_name: string; multiplier: string; is_hidden: boolean; authorized_user_ids: string[] };

const emptyForm: Form = { code: "", display_name: "", multiplier: "1", is_hidden: false, authorized_user_ids: [] };

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
  const [userOptions, setUserOptions] = useState<UserOption[]>([]);
  const [userOptionsLoading, setUserOptionsLoading] = useState(false);
  const [userOptionsLoaded, setUserOptionsLoaded] = useState(false);
  const [userQuery, setUserQuery] = useState("");

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

  const loadUserOptions = useCallback(async () => {
    if (userOptionsLoaded || userOptionsLoading) return;
    setUserOptionsLoading(true);
    const loaded: UserOption[] = [];
    let offset = 0;
    let total = 0;
    do {
      const response = await fetch(`/api/admin/users?limit=100&offset=${offset}`, { cache: "no-store" });
      if (!response.ok) {
        setMessage(await errorMessage(response));
        setUserOptionsLoading(false);
        return;
      }
      const page = (await response.json()) as UserPage;
      loaded.push(...page.users.filter((record) => record.role === "member"));
      total = page.total;
      offset += page.users.length;
      if (page.users.length === 0) break;
    } while (offset < total);
    setUserOptions(loaded);
    setUserOptionsLoaded(true);
    setUserOptionsLoading(false);
  }, [userOptionsLoaded, userOptionsLoading]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return needle ? groups.filter((group) => `${group.code} ${group.display_name}`.toLowerCase().includes(needle)) : groups;
  }, [groups, query]);
  const filteredUserOptions = useMemo(() => {
    const needle = userQuery.trim().toLowerCase();
    if (!needle) return userOptions;
    return userOptions.filter((record) => `${record.username} ${record.display_name} ${record.email}`.toLowerCase().includes(needle));
  }, [userOptions, userQuery]);
  const selection = useListSelection(filtered.filter((group) => !group.is_default).map((group) => group.id));

  function beginEdit(group: Group) {
    setEditing(group);
    setUserQuery("");
    setForm({ code: group.code, display_name: group.display_name, multiplier: String(group.multiplier_bps / 10_000), is_hidden: group.is_hidden, authorized_user_ids: (group.authorized_users ?? []).map((record) => record.id) });
    void loadUserOptions();
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const bps = multiplierBPS(form.multiplier);
    if (bps === null) { setMessage("计费倍率必须在 0.0001 到 100 之间，最多保留 4 位小数"); return; }
    const body = editing
      ? { display_name: form.display_name, multiplier_bps: bps, is_hidden: form.is_hidden, authorized_user_ids: form.is_hidden ? form.authorized_user_ids : [] }
      : { code: form.code, display_name: form.display_name, multiplier_bps: bps, is_hidden: form.is_hidden, authorized_user_ids: form.is_hidden ? form.authorized_user_ids : [] };
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
    setUserQuery("");
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
    <div className="space-y-2"><Label htmlFor="group-code">分组标识</Label><Input disabled={editing !== null} id="group-code" maxLength={64} onChange={(event) => setForm({ ...form, code: event.target.value })} pattern="[a-z0-9][a-z0-9-]{1,62}[a-z0-9]" placeholder="例如 vip" required title="分组标识需为 3 到 64 位，只能使用小写字母、数字和连字符，不能包含点号、小数点、下划线或空格，且必须以字母或数字开头和结尾" value={form.code} /><p className="text-xs text-muted-foreground">3 到 64 位；只允许小写字母、数字和连字符，不允许点号、小数点、下划线或空格。</p></div>
    <div className="space-y-2"><Label htmlFor="group-name">显示名称</Label><Input id="group-name" maxLength={128} onChange={(event) => setForm({ ...form, display_name: event.target.value })} required value={form.display_name} /></div>
    <div className="space-y-2"><Label htmlFor="group-multiplier">计费倍率</Label><Input id="group-multiplier" inputMode="decimal" max="100" min="0.0001" onChange={(event) => setForm({ ...form, multiplier: event.target.value })} required step="0.0001" title="计费倍率必须在 0.0001 到 100 之间，最多保留 4 位小数" type="number" value={form.multiplier} /><p className="text-xs text-muted-foreground">范围 0.0001 到 100；1.0000 表示按模型目录基础价格计费，1.2000 表示加价 20%。</p></div>
    <label className="flex cursor-pointer items-start gap-3 border-y py-3"><Checkbox aria-label="隐藏计费分组" checked={form.is_hidden} disabled={editing?.is_default === true} onCheckedChange={(checked) => setForm({ ...form, is_hidden: checked === true, authorized_user_ids: checked === true ? form.authorized_user_ids : [] })} /><span><span className="block text-sm font-medium">隐藏分组</span><span className="mt-1 block text-xs leading-5 text-muted-foreground">仅管理员和此分组已授权的用户可以查看、选择和使用。</span></span></label>
    {form.is_hidden ? <div className="space-y-3"><div className="flex items-center justify-between gap-3"><div><Label htmlFor="authorized-user-search">授权用户</Label><p className="mt-1 text-xs leading-5 text-muted-foreground">每个隐藏分组独立授权；管理员无需选择即可访问。</p></div><Badge variant="secondary">已选 {form.authorized_user_ids.length}</Badge></div><div className="relative"><Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input id="authorized-user-search" className="pl-8" onChange={(event) => setUserQuery(event.target.value)} placeholder="搜索用户名、显示名称或邮箱" value={userQuery} /></div><div className="max-h-56 overflow-y-auto border-y" role="group" aria-label="选择授权用户">{userOptionsLoading ? <p className="px-3 py-6 text-center text-sm text-muted-foreground">正在加载用户...</p> : null}{!userOptionsLoading && filteredUserOptions.length === 0 ? <p className="px-3 py-6 text-center text-sm text-muted-foreground">没有符合条件的普通成员</p> : null}{!userOptionsLoading ? filteredUserOptions.map((record) => { const selected = form.authorized_user_ids.includes(record.id); return <label className="flex cursor-pointer items-center gap-3 border-b px-3 py-2.5 last:border-b-0 hover:bg-muted/50" key={record.id}><Checkbox aria-label={`授权用户 ${record.username}`} checked={selected} onCheckedChange={(checked) => setForm({ ...form, authorized_user_ids: checked === true ? [...form.authorized_user_ids, record.id] : form.authorized_user_ids.filter((id) => id !== record.id) })} /><span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{record.display_name || record.username}</span><span className="block truncate text-xs text-muted-foreground">@{record.username}{record.email ? ` · ${record.email}` : ""}</span></span>{record.status === "disabled" ? <Badge variant="outline">已停用</Badge> : null}</label>; }) : null}</div></div> : null}
  </>;

  return (
    <div className="space-y-5">
      <section className="grid border-y bg-background sm:grid-cols-4">
        <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">计费分组</p><p className="mt-1 text-xl font-semibold">{groups.length}</p></div>
        <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">API Key</p><p className="mt-1 text-xl font-semibold">{groups.reduce((sum, group) => sum + group.api_key_count, 0)}</p></div>
        <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">供应商</p><p className="mt-1 text-xl font-semibold">{groups.reduce((sum, group) => sum + group.provider_count, 0)}</p></div>
        <div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">默认倍率</p><p className="mt-1 text-xl font-semibold">{((groups.find((group) => group.is_default)?.multiplier_bps ?? 10_000) / 10_000).toFixed(4)}x</p></div>
      </section>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative max-w-md flex-1"><Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索计费分组" className="pl-8" onChange={(event) => { setQuery(event.target.value); selection.clearSelection(); }} placeholder="搜索标识或名称" value={query} /></div>
        <div className="flex gap-2"><Button aria-label="刷新" disabled={loading} onClick={() => void load()} size="icon" title="刷新" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button><Button onClick={() => { setForm(emptyForm); setUserQuery(""); setCreateOpen(true); void loadUserOptions(); }}><Plus />新增分组</Button></div>
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
            <TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择所有可操作计费分组" checked={selection.checkboxState} disabled={loading || filtered.filter((group) => !group.is_default).length === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead>分组</TableHead><TableHead>倍率</TableHead><TableHead>授权用户</TableHead><TableHead>API Key</TableHead><TableHead>供应商</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={8}>加载中...</TableCell></TableRow> : null}
              {!loading && filtered.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={8}>还没有计费分组</TableCell></TableRow> : null}
              {filtered.map((group) => <TableRow key={group.id}>
                <TableCell><Checkbox aria-label={`选择 ${group.display_name}`} checked={selection.isSelected(group.id)} disabled={group.is_default} onCheckedChange={(checked) => selection.toggleOne(group.id, checked === true)} /></TableCell>
                <TableCell><p className="font-medium">{group.display_name} {group.is_default ? <Badge className="ml-1" variant="secondary">默认</Badge> : null}{group.is_hidden ? <Badge className="ml-1" variant="outline">隐藏</Badge> : null}</p><p className="font-mono text-xs text-muted-foreground">{group.code}</p></TableCell>
                <TableCell className="font-mono">{(group.multiplier_bps / 10_000).toFixed(4)}x</TableCell>
                <TableCell>{group.is_hidden ? <span className="inline-flex items-center gap-1"><UsersRound className="size-4 text-muted-foreground" />{(group.authorized_users ?? []).length}</span> : <span className="text-muted-foreground">全部用户</span>}</TableCell>
                <TableCell><span className="inline-flex items-center gap-1"><KeyRound className="size-4 text-muted-foreground" />{group.api_key_count}</span></TableCell>
                <TableCell><span className="inline-flex items-center gap-1"><ServerCog className="size-4 text-muted-foreground" />{group.provider_count}</span></TableCell>
                <TableCell><Badge variant={group.status === "active" ? "outline" : "destructive"}>{group.status === "active" ? "启用" : "停用"}</Badge></TableCell>
                <TableCell><div className="flex justify-end gap-1"><Button aria-label={`编辑 ${group.display_name}`} onClick={() => beginEdit(group)} size="icon-sm" title="编辑" variant="ghost"><Pencil /></Button><Button aria-label={`${group.status === "active" ? "停用" : "启用"} ${group.display_name}`} disabled={group.is_default} onClick={() => setStatusGroup(group)} size="icon-sm" title={group.is_default ? "默认分组不能停用" : group.status === "active" ? "停用" : "启用"} variant="ghost">{group.status === "active" ? <PowerOff /> : <Power />}</Button><Button aria-label={`删除 ${group.display_name}`} disabled={group.is_default} onClick={() => setDeletingGroup(group)} size="icon-sm" title={group.is_default ? "默认分组不能删除" : "删除计费分组"} variant="ghost"><Trash2 /></Button></div></TableCell>
              </TableRow>)}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Dialog onOpenChange={(open) => { setCreateOpen(open); if (!open) { setForm(emptyForm); setUserQuery(""); } }} open={createOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg"><DialogHeader><DialogTitle>新增计费分组</DialogTitle><DialogDescription>API Key 和供应商选择同一分组后，调用会按该倍率结算并只使用同组供应商。</DialogDescription></DialogHeader><form className="space-y-4" id="create-group-form" onSubmit={submit}>{fields}</form><DialogFooter><Button onClick={() => setCreateOpen(false)} variant="outline">取消</Button><Button disabled={busy} form="create-group-form" type="submit"><Plus />创建分组</Button></DialogFooter></DialogContent>
      </Dialog>

      <Sheet onOpenChange={(open) => { if (!open) { setEditing(null); setForm(emptyForm); setUserQuery(""); } }} open={editing !== null}>
        <SheetContent className="grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden data-[side=right]:w-full! sm:data-[side=right]:w-[28rem]! sm:data-[side=right]:max-w-none!" side="right"><SheetHeader className="border-b px-6 py-5"><SheetTitle>编辑计费分组</SheetTitle></SheetHeader><form className="space-y-5 overflow-y-auto px-6" id="edit-group-form" onSubmit={submit}>{fields}</form><SheetFooter className="border-t px-6"><Button disabled={busy} form="edit-group-form" type="submit"><Pencil />保存修改</Button></SheetFooter></SheetContent>
      </Sheet>

      <AlertDialog onOpenChange={(open) => { if (!open) setStatusGroup(null); }} open={statusGroup !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{statusGroup?.status === "active" ? "停用计费分组" : "启用计费分组"}</AlertDialogTitle><AlertDialogDescription>停用后不能再新建该组 API Key 或供应商，该组现有 API Key 也会停止认证。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }}>{statusGroup?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>

      <AlertDialog onOpenChange={(open) => { if (!open) setDeletingGroup(null); }} open={deletingGroup !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除计费分组</AlertDialogTitle><AlertDialogDescription>将删除 {deletingGroup?.display_name ?? "该计费分组"}。历史用量记录会继续保留；如果仍有启用中的 API Key 或供应商使用此分组，请先迁移或删除。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void deleteOneGroup(); }} variant="destructive">确认删除</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>

      <BulkActionDialog busy={busy} confirmLabel={bulkStatus === "active" ? "确认批量启用" : "确认批量停用"} description={`将${bulkStatus === "active" ? "启用" : "停用"}选中的 ${selection.selectedIds.length} 个计费分组；默认分组不能被批量选中。`} destructive={bulkStatus === "disabled"} onConfirm={applyBulkStatus} onOpenChange={(open) => { if (!open) setBulkStatus(null); }} open={bulkStatus !== null} title={bulkStatus === "active" ? "批量启用计费分组" : "批量停用计费分组"} />
      <BulkActionDialog busy={busy} confirmLabel="确认批量删除" description={`将删除选中的 ${selection.selectedIds.length} 个计费分组。历史用量记录会继续保留；仍被 API Key 或供应商使用的分组会被拒绝并保持选中。`} destructive onConfirm={deleteSelected} onOpenChange={setBulkDeleteOpen} open={bulkDeleteOpen} title="批量删除计费分组" />
    </div>
  );
}
