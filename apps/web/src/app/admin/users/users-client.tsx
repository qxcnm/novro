"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { ChevronLeft, ChevronRight, CircleDollarSign, Eye, KeyRound, Pencil, Plus, Power, PowerOff, RefreshCw, Search, UserRound, UserRoundCheck, UserRoundX, WalletCards } from "lucide-react";
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
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { bulkResultMessage, runBulkAction } from "@/lib/bulk-action";
import { useListSelection } from "@/lib/use-list-selection";

type UserRecord = {
	id: string;
	username: string;
	email: string;
	display_name: string;
  role: "admin" | "member";
  status: "active" | "disabled";
  created_at: string;
  updated_at: string;
	last_login_at: string | null;
	can_access_hidden_groups: boolean;
};

type UserPage = { users: UserRecord[]; total: number; offset: number; limit: number };
type WalletEntry = { id: string; entry_type: "manual_adjustment" | "top_up" | "referral_reward" | "usage_reservation" | "usage_refund" | "usage_settlement" | "usage_compensation"; amount_micros: number; balance_after_micros: number; description: string; created_at: string };
type BalanceSummary = { wallet: { balance_micros: number; updated_at: string }; entries: WalletEntry[] };
type ErrorResponse = { error?: { message?: string } };

const PAGE_SIZE = 20;

async function readError(response: Response) {
  const body = (await response.json().catch(() => ({}))) as ErrorResponse;
  return body.error?.message ?? "操作失败，请稍后重试";
}

function formatDate(value: string | null) {
  if (!value) return "从未登录";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function formatMoney(micros: number) {
  return new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000);
}

function yuanToMicros(value: string) {
  const match = value.trim().match(/^(-?)(\d{1,9})(?:\.(\d{1,6}))?$/);
  if (!match) return null;
  const amount = Number(match[2]) * 1_000_000 + Number((match[3] ?? "").padEnd(6, "0"));
  return match[1] === "-" ? -amount : amount;
}

export default function UsersClient() {
	const router = useRouter();
	const [users, setUsers] = useState<UserRecord[]>([]);
	const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [formError, setFormError] = useState("");
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "disabled">("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [detailUser, setDetailUser] = useState<UserRecord | null>(null);
  const [resetUser, setResetUser] = useState<UserRecord | null>(null);
  const [statusUser, setStatusUser] = useState<UserRecord | null>(null);
  const [balanceUser, setBalanceUser] = useState<UserRecord | null>(null);
  const [balanceSummary, setBalanceSummary] = useState<BalanceSummary | null>(null);
  const [balanceLoading, setBalanceLoading] = useState(false);
  const [balanceError, setBalanceError] = useState("");
  const [adjustmentAmount, setAdjustmentAmount] = useState("");
  const [adjustmentNote, setAdjustmentNote] = useState("");
  const [adjustmentIdempotencyKey, setAdjustmentIdempotencyKey] = useState("");
	const [form, setForm] = useState({ username: "", email: "", display_name: "", password: "", role: "member" as "admin" | "member", can_access_hidden_groups: false });
	const [editForm, setEditForm] = useState({ display_name: "", role: "member" as "admin" | "member", can_access_hidden_groups: false });
  const [resetPassword, setResetPassword] = useState("");
  const [bulkStatus, setBulkStatus] = useState<"active" | "disabled" | null>(null);
  const selection = useListSelection(users.map((user) => user.id));

	const loadUsers = useCallback(async () => {
		setLoading(true);
		const query = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) });
		if (search) query.set("search", search);
		if (status !== "all") query.set("status", status);
		const response = await fetch(`/api/admin/users?${query}`, { cache: "no-store" });
		if (response.status === 401) { router.replace("/login"); return; }
		if (response.status === 403) { router.replace("/console"); return; }
		if (!response.ok) { setMessage(await readError(response)); setLoading(false); return; }
		const page = (await response.json()) as UserPage;
		setUsers(page.users);
		setTotal(page.total);
		setLoading(false);
	}, [offset, router, search, status]);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadUsers(), 0);
    return () => window.clearTimeout(timer);
  }, [loadUsers]);

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setOffset(0);
    setSearch(searchDraft.trim());
  }

	function openDetails(record: UserRecord) {
		setDetailUser(record);
		setEditForm({ display_name: record.display_name, role: record.role, can_access_hidden_groups: record.can_access_hidden_groups });
	}

  async function createUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true); setMessage(""); setFormError("");
    const response = await fetch("/api/admin/users", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ...form, can_access_hidden_groups: form.role === "admin" || form.can_access_hidden_groups }) });
    setBusy(false);
	if (!response.ok) { setFormError(await readError(response)); return; }
	setForm({ username: "", email: "", display_name: "", password: "", role: "member", can_access_hidden_groups: false });
	setCreateOpen(false); setOffset(0); setMessage("用户已创建");
    await loadUsers();
  }

  async function updateUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!detailUser) return;
    setBusy(true); setMessage(""); setFormError("");
    const response = await fetch(`/api/admin/users/${detailUser.id}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ...editForm, can_access_hidden_groups: editForm.role === "admin" || editForm.can_access_hidden_groups }) });
    setBusy(false);
    if (!response.ok) { setFormError(await readError(response)); return; }
    const body = (await response.json()) as { user: UserRecord };
    setDetailUser(body.user); setMessage("用户资料已更新");
    await loadUsers();
  }

  async function toggleStatus() {
    if (!statusUser) return;
    setBusy(true); setMessage("");
    const nextStatus = statusUser.status === "active" ? "disabled" : "active";
    const response = await fetch(`/api/admin/users/${statusUser.id}/status`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ status: nextStatus }) });
    setBusy(false);
    if (!response.ok) { setMessage(await readError(response)); setStatusUser(null); return; }
    setStatusUser(null); setMessage(nextStatus === "active" ? "用户已启用" : "用户已停用");
    await loadUsers();
  }

  async function applyBulkStatus() {
    if (!bulkStatus) return;
    setBusy(true);
    setMessage("");
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/users/${id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: bulkStatus }),
    }));
    setBusy(false);
    setBulkStatus(null);
    setMessage(bulkResultMessage(bulkStatus === "active" ? "启用用户" : "停用用户", result));
    await loadUsers();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  async function submitReset(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!resetUser) return;
    setBusy(true); setMessage(""); setFormError("");
    const response = await fetch(`/api/admin/users/${resetUser.id}/reset-password`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password: resetPassword }) });
    setBusy(false);
    if (!response.ok) { setFormError(await readError(response)); return; }
    setResetUser(null); setResetPassword(""); setMessage("密码已重置，该用户的原有会话已退出");
  }

  async function openBalance(record: UserRecord) {
    setBalanceUser(record); setBalanceSummary(null); setBalanceLoading(true); setBalanceError("");
    const response = await fetch(`/api/admin/users/${record.id}/balance`, { cache: "no-store" });
    setBalanceLoading(false);
    if (!response.ok) { setBalanceError(await readError(response)); return; }
    setBalanceSummary((await response.json()) as BalanceSummary);
  }

  async function adjustBalance(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!balanceUser) return;
    const amount = yuanToMicros(adjustmentAmount);
    if (amount === null || amount === 0) { setBalanceError("请输入非零金额，最多保留 6 位小数"); return; }
    setBusy(true); setBalanceError(""); setMessage("");
    const idempotencyKey = adjustmentIdempotencyKey || crypto.randomUUID();
    if (!adjustmentIdempotencyKey) setAdjustmentIdempotencyKey(idempotencyKey);
    try {
      const response = await fetch(`/api/admin/users/${balanceUser.id}/balance-adjustments`, { method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify({ amount_micros: amount, note: adjustmentNote }) });
      if (!response.ok) { setBalanceError(await readError(response)); return; }
      setBalanceSummary((await response.json()) as BalanceSummary);
      setAdjustmentAmount(""); setAdjustmentNote(""); setAdjustmentIdempotencyKey(""); setMessage("用户余额已调整");
    } catch {
      setBalanceError("网络异常，未能确认调整结果。请保持金额和备注不变后重试");
    } finally {
      setBusy(false);
    }
  }

  const activeCount = users.filter((record) => record.status === "active").length;
  const adminCount = users.filter((record) => record.role === "admin").length;
  const pageStart = total === 0 ? 0 : offset + 1;
  const pageEnd = Math.min(offset + users.length, total);

  return (
    <>
      <div className="space-y-5">
        <section className="grid border-y bg-background sm:grid-cols-3">
          <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">用户总数</p><p className="mt-1 text-xl font-semibold">{total}</p></div>
          <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">本页启用</p><p className="mt-1 text-xl font-semibold">{activeCount}</p></div>
          <div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">本页管理员</p><p className="mt-1 text-xl font-semibold">{adminCount}</p></div>
        </section>

        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <form className="flex min-w-0 flex-1 gap-2" onSubmit={submitSearch}>
            <div className="relative max-w-md flex-1"><Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索用户" className="pl-8" onChange={(event) => setSearchDraft(event.target.value)} placeholder="搜索用户名、邮箱或显示名称" value={searchDraft} /></div>
            <Button type="submit" variant="outline">搜索</Button>
          </form>
          <div className="flex flex-wrap items-center gap-2">
            <Select onValueChange={(value: "all" | "active" | "disabled") => { setStatus(value); setOffset(0); }} value={status}>
              <SelectTrigger aria-label="按状态筛选" className="w-32"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="active">已启用</SelectItem><SelectItem value="disabled">已停用</SelectItem></SelectContent>
            </Select>
            <Button aria-label="刷新用户列表" disabled={loading} onClick={() => void loadUsers()} size="icon" title="刷新用户列表" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
			<Button onClick={() => { setForm({ username: "", email: "", display_name: "", password: "", role: "member", can_access_hidden_groups: false }); setCreateOpen(true); }}><Plus />创建用户</Button>
          </div>
        </div>

        {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}

        <ListBulkActions onClear={selection.clearSelection} selectedCount={selection.selectedIds.length}>
          <Button disabled={busy} onClick={() => setBulkStatus("active")} size="sm" type="button" variant="outline"><Power />批量启用</Button>
          <Button disabled={busy} onClick={() => setBulkStatus("disabled")} size="sm" type="button" variant="destructive"><PowerOff />批量停用</Button>
        </ListBulkActions>

        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
				<TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择本页所有用户" checked={selection.checkboxState} disabled={loading || users.length === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead className="min-w-48">用户</TableHead><TableHead>角色</TableHead><TableHead>状态</TableHead><TableHead className="min-w-40">最近登录</TableHead><TableHead className="min-w-40">创建时间</TableHead><TableHead className="w-36 text-right">操作</TableHead></TableRow></TableHeader>
				<TableBody>
				  {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={7}>加载中...</TableCell></TableRow> : null}
				  {!loading && users.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={7}>没有符合条件的用户</TableCell></TableRow> : null}
                  {!loading ? users.map((record) => (
                    <TableRow key={record.id}>
					  <TableCell><Checkbox aria-label={`选择用户 ${record.username}`} checked={selection.isSelected(record.id)} onCheckedChange={(checked) => selection.toggleOne(record.id, checked === true)} /></TableCell>
                      <TableCell><button className="max-w-64 text-left outline-none focus-visible:underline" onClick={() => openDetails(record)} type="button"><span className="block font-medium">{record.display_name || record.username}</span><span className="block text-xs text-muted-foreground">@{record.username}</span><span className="block truncate text-xs text-muted-foreground">{record.email || "未设置邮箱"}</span></button></TableCell>
					  <TableCell><div className="flex flex-wrap gap-1"><Badge variant={record.role === "admin" ? "default" : "secondary"}>{record.role === "admin" ? "管理员" : "成员"}</Badge>{record.role !== "admin" && record.can_access_hidden_groups ? <Badge variant="outline">隐藏分组</Badge> : null}</div></TableCell>
					  <TableCell><Badge variant={record.status === "active" ? "outline" : "destructive"}>{record.status === "active" ? "启用" : "停用"}</Badge></TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(record.last_login_at)}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(record.created_at)}</TableCell>
                      <TableCell><div className="flex justify-end gap-1">
                        <Button aria-label={`查看 ${record.username}`} onClick={() => openDetails(record)} size="icon-sm" title="查看与编辑" variant="ghost"><Eye /></Button>
                        <Button aria-label={`管理 ${record.username} 的余额`} onClick={() => void openBalance(record)} size="icon-sm" title="余额与流水" variant="ghost"><WalletCards /></Button>
                        <Button aria-label={`重置 ${record.username} 的密码`} onClick={() => setResetUser(record)} size="icon-sm" title="重置密码" variant="ghost"><KeyRound /></Button>
                        <Button aria-label={`${record.status === "active" ? "停用" : "启用"} ${record.username}`} onClick={() => setStatusUser(record)} size="icon-sm" title={record.status === "active" ? "停用用户" : "启用用户"} variant="ghost">{record.status === "active" ? <UserRoundX /> : <UserRoundCheck />}</Button>
                      </div></TableCell>
                    </TableRow>
                  )) : null}
                </TableBody>
              </Table>
            </div>
            <div className="flex flex-col gap-3 border-t px-4 py-3 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
              <span>显示 {pageStart}-{pageEnd}，共 {total} 个用户</span>
              <div className="flex gap-1"><Button aria-label="上一页" disabled={offset === 0 || loading} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))} size="icon-sm" title="上一页" variant="outline"><ChevronLeft /></Button><Button aria-label="下一页" disabled={offset + PAGE_SIZE >= total || loading} onClick={() => setOffset(offset + PAGE_SIZE)} size="icon-sm" title="下一页" variant="outline"><ChevronRight /></Button></div>
            </div>
          </CardContent>
        </Card>

		<Dialog onOpenChange={(open) => { setCreateOpen(open); setFormError(""); if (!open) setForm({ username: "", email: "", display_name: "", password: "", role: "member", can_access_hidden_groups: false }); }} open={createOpen}>
          <DialogContent>
            <DialogHeader><DialogTitle>创建用户</DialogTitle><DialogDescription>创建后用户立即启用。初始密码为 8 到 72 字节，且必须包含英文和数字。</DialogDescription></DialogHeader>
            <form className="space-y-4" id="create-user-form" onSubmit={createUser}>
              <div className="space-y-2"><Label htmlFor="new-username">用户名</Label><Input autoComplete="off" id="new-username" onChange={(event) => setForm({ ...form, username: event.target.value })} pattern="[a-z0-9][a-z0-9._-]{2,63}" placeholder="例如 alice.chen" required title="用户名需为 3 到 64 位，只能使用小写字母、数字、点号、下划线和连字符，且必须以字母或数字开头" value={form.username} /><p className="text-xs text-muted-foreground">3 到 64 位；允许小写字母、数字、点号、下划线和连字符，必须以字母或数字开头。</p></div>
              <div className="space-y-2"><Label htmlFor="new-email">邮箱</Label><Input autoComplete="email" id="new-email" maxLength={320} onChange={(event) => setForm({ ...form, email: event.target.value })} placeholder="例如 alice@example.com" required type="email" value={form.email} /></div>
              <div className="space-y-2"><Label htmlFor="new-display-name">显示名称</Label><Input id="new-display-name" maxLength={128} onChange={(event) => setForm({ ...form, display_name: event.target.value })} placeholder="例如 Alice Chen" value={form.display_name} /></div>
              <div className="space-y-2"><Label htmlFor="new-password">初始密码</Label><Input autoComplete="new-password" id="new-password" minLength={8} onChange={(event) => setForm({ ...form, password: event.target.value })} pattern="(?=.*[A-Za-z])(?=.*[0-9]).{8,}" required title="密码需为 8 到 72 字节，且必须同时包含英文和数字" type="password" value={form.password} /><p className="text-xs text-muted-foreground">8 到 72 字节，且必须同时包含英文和数字。</p></div>
			  <div className="space-y-2"><Label htmlFor="new-role">角色</Label><Select onValueChange={(role: "admin" | "member") => setForm({ ...form, role })} value={form.role}><SelectTrigger id="new-role"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="member">成员</SelectItem><SelectItem value="admin">管理员</SelectItem></SelectContent></Select></div>
			  <label className="flex cursor-pointer items-start gap-3 border-y py-3"><Checkbox aria-label="允许访问隐藏分组" checked={form.role === "admin" || form.can_access_hidden_groups} disabled={form.role === "admin"} onCheckedChange={(checked) => setForm({ ...form, can_access_hidden_groups: checked === true })} /><span><span className="block text-sm font-medium">允许访问隐藏分组</span><span className="mt-1 block text-xs leading-5 text-muted-foreground">授权后可查看隐藏分组价格、创建该组 API Key 并使用该组已有 Key。管理员自动拥有权限。</span></span></label>
			  {formError ? <p className="text-sm text-destructive" role="alert">{formError}</p> : null}
            </form>
            <DialogFooter><Button onClick={() => setCreateOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy} form="create-user-form" type="submit"><UserRound />{busy ? "正在创建..." : "创建用户"}</Button></DialogFooter>
          </DialogContent>
        </Dialog>

        <Sheet onOpenChange={(open) => { setFormError(""); if (!open) setDetailUser(null); }} open={detailUser !== null}>
          <SheetContent className="w-full overflow-y-auto sm:max-w-md" side="right">
            {detailUser ? <><SheetHeader className="border-b px-6 py-5"><SheetTitle>{detailUser.display_name || detailUser.username}</SheetTitle><SheetDescription>@{detailUser.username}{detailUser.email ? ` · ${detailUser.email}` : " · 未设置邮箱"}</SheetDescription></SheetHeader>
              <form className="space-y-6 px-6" id="edit-user-form" onSubmit={updateUser}>
                <div className="grid grid-cols-2 gap-4 border-b pb-5 text-sm"><div><p className="text-xs text-muted-foreground">状态</p><Badge className="mt-2" variant={detailUser.status === "active" ? "outline" : "destructive"}>{detailUser.status === "active" ? "启用" : "停用"}</Badge></div><div><p className="text-xs text-muted-foreground">最近登录</p><p className="mt-2 leading-5">{formatDate(detailUser.last_login_at)}</p></div><div><p className="text-xs text-muted-foreground">创建时间</p><p className="mt-2 leading-5">{formatDate(detailUser.created_at)}</p></div><div><p className="text-xs text-muted-foreground">最近更新</p><p className="mt-2 leading-5">{formatDate(detailUser.updated_at)}</p></div></div>
                <div className="space-y-2"><Label htmlFor="edit-display-name">显示名称</Label><Input id="edit-display-name" maxLength={128} onChange={(event) => setEditForm({ ...editForm, display_name: event.target.value })} value={editForm.display_name} /></div>
                <div className="space-y-2"><Label htmlFor="edit-email">邮箱</Label><Input disabled id="edit-email" value={detailUser.email || "未设置邮箱"} /></div>
				<div className="space-y-2"><Label htmlFor="edit-role">角色</Label><Select onValueChange={(role: "admin" | "member") => setEditForm({ ...editForm, role })} value={editForm.role}><SelectTrigger id="edit-role"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="member">成员</SelectItem><SelectItem value="admin">管理员</SelectItem></SelectContent></Select><p className="text-xs leading-5 text-muted-foreground">最后一个启用的管理员不能降级为普通成员。</p></div>
				<label className="flex cursor-pointer items-start gap-3 border-y py-3"><Checkbox aria-label="允许访问隐藏分组" checked={editForm.role === "admin" || editForm.can_access_hidden_groups} disabled={editForm.role === "admin"} onCheckedChange={(checked) => setEditForm({ ...editForm, can_access_hidden_groups: checked === true })} /><span><span className="block text-sm font-medium">允许访问隐藏分组</span><span className="mt-1 block text-xs leading-5 text-muted-foreground">撤销后，该用户绑定隐藏分组的 API Key 将立即停止认证。管理员自动拥有权限。</span></span></label>
				{formError ? <p className="text-sm text-destructive" role="alert">{formError}</p> : null}
              </form>
              <SheetFooter className="border-t px-6"><Button disabled={busy} form="edit-user-form" type="submit"><Pencil />{busy ? "正在保存..." : "保存修改"}</Button><Button onClick={() => { setDetailUser(null); setResetUser(detailUser); }} type="button" variant="outline"><KeyRound />重置密码</Button></SheetFooter></> : null}
          </SheetContent>
        </Sheet>

        <Sheet onOpenChange={(open) => { if (!open) { setBalanceUser(null); setBalanceSummary(null); setBalanceError(""); setAdjustmentAmount(""); setAdjustmentNote(""); setAdjustmentIdempotencyKey(""); } }} open={balanceUser !== null}>
          <SheetContent className="w-full overflow-y-auto sm:max-w-lg" side="right">
            {balanceUser ? <><SheetHeader className="border-b px-6 py-5"><SheetTitle>{balanceUser.display_name || balanceUser.username} 的余额</SheetTitle><SheetDescription>@{balanceUser.username}</SheetDescription></SheetHeader>
              <div className="space-y-6 px-6">
                <div className="border-y py-5"><p className="text-xs text-muted-foreground">可用余额</p><p className="mt-2 text-2xl font-semibold">{balanceLoading ? "加载中..." : balanceSummary ? formatMoney(balanceSummary.wallet.balance_micros) : "--"}</p></div>
                <form className="space-y-4" id="adjust-balance-form" onSubmit={adjustBalance}>
                  <div className="space-y-2"><Label htmlFor="adjustment-amount">调整金额（元）</Label><Input id="adjustment-amount" inputMode="decimal" onChange={(event) => { setAdjustmentAmount(event.target.value); setAdjustmentIdempotencyKey(""); }} placeholder="增加输入 100，扣减输入 -20" required value={adjustmentAmount} /></div>
                  <div className="space-y-2"><Label htmlFor="adjustment-note">备注</Label><Input id="adjustment-note" maxLength={255} onChange={(event) => { setAdjustmentNote(event.target.value); setAdjustmentIdempotencyKey(""); }} placeholder="例如 初始额度" required value={adjustmentNote} /></div>
                  {balanceError ? <p className="text-sm text-destructive" role="alert">{balanceError}</p> : null}
                  <Button disabled={busy || balanceLoading} type="submit"><CircleDollarSign />{busy ? "正在调整..." : "确认调整"}</Button>
                </form>
                <div><h3 className="mb-3 text-sm font-semibold">最近流水</h3><div className="overflow-x-auto border-y"><Table><TableHeader><TableRow><TableHead>说明</TableHead><TableHead>变动</TableHead><TableHead>余额</TableHead></TableRow></TableHeader><TableBody>{balanceSummary?.entries.map((entry) => <TableRow key={entry.id}><TableCell><p>{entry.description}</p><p className="mt-1 text-xs text-muted-foreground">{formatDate(entry.created_at)}</p></TableCell><TableCell>{entry.amount_micros >= 0 ? "+" : ""}{formatMoney(entry.amount_micros)}</TableCell><TableCell>{formatMoney(entry.balance_after_micros)}</TableCell></TableRow>)}{!balanceLoading && (balanceSummary?.entries.length ?? 0) === 0 ? <TableRow><TableCell className="h-20 text-center text-muted-foreground" colSpan={3}>还没有余额流水</TableCell></TableRow> : null}</TableBody></Table></div></div>
              </div>
              <SheetFooter className="border-t px-6"><Button onClick={() => setBalanceUser(null)} type="button" variant="outline">关闭</Button></SheetFooter></> : null}
          </SheetContent>
        </Sheet>

        <Dialog onOpenChange={(open) => { setFormError(""); if (!open) { setResetUser(null); setResetPassword(""); } }} open={resetUser !== null}>
          <DialogContent>
            <DialogHeader><DialogTitle>重置密码</DialogTitle><DialogDescription>为 {resetUser?.username} 设置新密码。保存后该用户的所有现有会话都会立即失效。</DialogDescription></DialogHeader>
            <form className="space-y-2" id="reset-password-form" onSubmit={submitReset}><Label htmlFor="reset-password">新密码</Label><Input autoComplete="new-password" id="reset-password" minLength={8} onChange={(event) => setResetPassword(event.target.value)} pattern="(?=.*[A-Za-z])(?=.*[0-9]).{8,}" required title="密码需为 8 到 72 字节，且必须同时包含英文和数字" type="password" value={resetPassword} /><p className="text-xs text-muted-foreground">8 到 72 字节，且必须同时包含英文和数字。</p>{formError ? <p className="text-sm text-destructive" role="alert">{formError}</p> : null}</form>
            <DialogFooter><Button onClick={() => setResetUser(null)} type="button" variant="outline">取消</Button><Button disabled={busy} form="reset-password-form" type="submit"><KeyRound />{busy ? "正在重置..." : "确认重置"}</Button></DialogFooter>
          </DialogContent>
        </Dialog>

        <AlertDialog onOpenChange={(open) => { if (!open) setStatusUser(null); }} open={statusUser !== null}>
          <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{statusUser?.status === "active" ? "停用用户" : "启用用户"}</AlertDialogTitle><AlertDialogDescription>{statusUser?.status === "active" ? `停用 ${statusUser.username} 后，该账号将无法继续登录。` : `启用 ${statusUser?.username ?? "该用户"} 后，该账号可以重新登录。`}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }} variant={statusUser?.status === "active" ? "destructive" : "default"}>{statusUser?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
        </AlertDialog>
        <BulkActionDialog busy={busy} confirmLabel={bulkStatus === "active" ? "确认批量启用" : "确认批量停用"} description={bulkStatus === "active" ? `将启用选中的 ${selection.selectedIds.length} 个用户，停用状态的账号将可以重新登录。` : `将停用选中的 ${selection.selectedIds.length} 个用户，账号将无法继续登录；最后一个启用的管理员仍会受到保护。`} destructive={bulkStatus === "disabled"} onConfirm={applyBulkStatus} onOpenChange={(open) => { if (!open) setBulkStatus(null); }} open={bulkStatus !== null} title={bulkStatus === "active" ? "批量启用用户" : "批量停用用户"} />
      </div>
    </>
  );
}
