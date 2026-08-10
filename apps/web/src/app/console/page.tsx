"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { Activity, Check, ChartNoAxesCombined, Clipboard, KeyRound, Plus, RefreshCw, ScrollText, ShieldX, UserRound } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { useCurrentUser } from "@/components/console-shell";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { copyText } from "@/lib/clipboard";

type APIKeyRecord = { id: string; name: string; key_prefix: string; status: "active" | "revoked"; last_used_at: string | null; created_at: string; revoked_at: string | null };
type CreateResult = { api_key: APIKeyRecord; key: string };
type BalanceSummary = { wallet: { balance_micros: number; updated_at: string } };
type ErrorResponse = { error?: { message?: string } };

async function readError(response: Response) {
  const body = (await response.json().catch(() => ({}))) as ErrorResponse;
  return body.error?.message ?? "操作失败，请稍后重试";
}

function formatMoney(micros: number) {
  return new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000);
}

function formatDate(value: string | null) {
  if (!value) return "从未使用";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export default function ConsolePage() {
  const router = useRouter();
  const user = useCurrentUser();
  const [keys, setKeys] = useState<APIKeyRecord[]>([]);
  const [summary, setSummary] = useState<BalanceSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [created, setCreated] = useState<CreateResult | null>(null);
  const [copied, setCopied] = useState(false);
  const [revokeKey, setRevokeKey] = useState<APIKeyRecord | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setMessage("");
    try {
      const [keysResponse, balanceResponse] = await Promise.all([
        fetch("/api/account/api-keys", { cache: "no-store" }),
        fetch("/api/account/balance", { cache: "no-store" }),
      ]);
      if (keysResponse.status === 401 || balanceResponse.status === 401) { router.replace("/login"); return; }

      const errors: string[] = [];
      if (keysResponse.ok) setKeys(((await keysResponse.json()) as { api_keys: APIKeyRecord[] }).api_keys);
      else errors.push(await readError(keysResponse));
      if (balanceResponse.ok) setSummary((await balanceResponse.json()) as BalanceSummary);
      else errors.push(await readError(balanceResponse));
      if (errors.length > 0) setMessage([...new Set(errors)].join("；"));
    } catch {
      setMessage("账户信息加载失败，请稍后重试");
    } finally {
      setLoading(false);
    }
  }, [router]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);

  async function createKey(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch("/api/account/api-keys", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name }) });
      if (response.status === 401) { router.replace("/login"); return; }
      if (!response.ok) { setMessage(await readError(response)); return; }
      const result = (await response.json()) as CreateResult;
      setCreated(result);
      setName("");
      await load();
    } catch {
      setMessage("API Key 创建失败，请稍后重试");
    } finally {
      setBusy(false);
    }
  }

  async function copyCreatedKey() {
    if (!created) return;
    const success = await copyText(created.key);
    setCopied(success);
    if (!success) setMessage("复制失败，请手动选择完整密钥");
  }

  async function revoke() {
    if (!revokeKey) return;
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch(`/api/account/api-keys/${revokeKey.id}`, { method: "DELETE" });
      if (!response.ok) { setMessage(await readError(response)); return; }
      setMessage("API Key 已撤销");
      await load();
    } catch {
      setMessage("API Key 撤销失败，请稍后重试");
    } finally {
      setBusy(false);
      setRevokeKey(null);
    }
  }

  const activeCount = keys.filter((key) => key.status === "active").length;
  const displayName = user.display_name || user.username;

  return (
    <div className="space-y-5">
      <section className="grid gap-3 md:grid-cols-3">
        {[{ href: "/console/dashboard", label: "数据看板", note: "看趋势、模型和成本", icon: ChartNoAxesCombined }, { href: "/console/logs", label: "使用日志", note: "按 Key 检查每次调用", icon: ScrollText }, { href: "/console/profile", label: "个人资料", note: "维护你的显示名称", icon: UserRound }].map((item) => <Link className="group rounded-lg border bg-background p-4 transition-colors hover:border-primary/50" href={item.href} key={item.href}><div className="flex items-center justify-between"><item.icon aria-hidden="true" className="size-5 text-muted-foreground group-hover:text-primary" /><Activity aria-hidden="true" className="size-4 text-muted-foreground" /></div><p className="mt-4 text-sm font-semibold">{item.label}</p><p className="mt-1 text-xs text-muted-foreground">{item.note}</p></Link>)}
      </section>
      <section className="overflow-hidden rounded-lg border bg-background">
        <div className="flex min-h-32 items-center justify-between gap-6 border-b px-5 py-6 sm:px-7">
          <div className="flex min-w-0 items-center gap-4">
            <span className="flex size-12 shrink-0 items-center justify-center rounded-full bg-muted text-foreground"><UserRound aria-hidden="true" className="size-6" /></span>
            <div className="min-w-0"><p className="truncate text-xl font-semibold">{displayName}</p><p className="mt-1 truncate text-sm text-muted-foreground">@{user.username}</p></div>
          </div>
          <Badge variant="outline">{user.role === "admin" ? "管理员" : "成员"}</Badge>
        </div>
        <div className="grid sm:grid-cols-[minmax(0,1fr)_minmax(180px,0.45fr)_auto] sm:items-stretch">
          <div className="px-5 py-5 sm:px-7">
            <p className="text-xs text-muted-foreground">当前余额</p>
            {loading && !summary ? <div className="mt-2 h-9 w-32 animate-pulse rounded-md bg-muted" /> : <p className="mt-1 text-3xl font-semibold">{summary ? formatMoney(summary.wallet.balance_micros) : "--"}</p>}
          </div>
          <div className="border-t px-5 py-5 sm:border-l sm:border-t-0">
            <p className="text-xs text-muted-foreground">可用 API Key</p>
            <p className="mt-1 text-2xl font-semibold">{loading && keys.length === 0 ? "--" : activeCount}<span className="ml-1 text-sm font-normal text-muted-foreground">/ 10</span></p>
          </div>
          <div className="flex items-center gap-2 border-t px-5 py-4 sm:border-l sm:border-t-0">
            <Button aria-label="刷新账户信息" disabled={loading} onClick={() => void load()} size="icon" title="刷新账户信息" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
            <Button disabled={activeCount >= 10} onClick={() => setCreateOpen(true)}><Plus />创建 API Key</Button>
          </div>
        </div>
      </section>

      {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}

      <section className="overflow-hidden rounded-lg border bg-background">
        <div className="flex items-center justify-between gap-4 border-b px-4 py-4 sm:px-5">
          <div><h2 className="text-sm font-semibold">我的 API Keys</h2><p className="mt-1 text-xs text-muted-foreground">完整密钥仅在创建时显示一次</p></div>
          <Badge variant="secondary">{activeCount} 个启用</Badge>
        </div>
        <div className="divide-y">
          {loading && keys.length === 0 ? Array.from({ length: 2 }).map((_, index) => <div className="flex h-[88px] items-center gap-4 px-4 sm:px-5" key={index}><div className="size-9 animate-pulse rounded-md bg-muted" /><div className="flex-1"><div className="h-4 w-28 animate-pulse rounded bg-muted" /><div className="mt-2 h-3 w-44 animate-pulse rounded bg-muted" /></div></div>) : null}
          {!loading && keys.length === 0 ? <div className="flex min-h-36 flex-col items-center justify-center px-4 py-8 text-center"><KeyRound aria-hidden="true" className="size-6 text-muted-foreground" /><p className="mt-3 text-sm font-medium">还没有 API Key</p><Button className="mt-4" onClick={() => setCreateOpen(true)} size="sm"><Plus />创建第一个 Key</Button></div> : null}
          {keys.map((key) => (
            <div className="flex min-h-[88px] items-center gap-3 px-4 py-4 sm:gap-4 sm:px-5" key={key.id}>
              <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-secondary"><KeyRound aria-hidden="true" className="size-4" /></span>
              <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><p className="truncate text-sm font-medium">{key.name}</p><Badge variant={key.status === "active" ? "outline" : "secondary"}>{key.status === "active" ? "启用" : "已撤销"}</Badge></div><p className="mt-1 truncate font-mono text-xs text-muted-foreground">{key.key_prefix}•••••••• · {key.last_used_at ? `最近使用 ${formatDate(key.last_used_at)}` : `创建于 ${formatDate(key.created_at)}`}</p></div>
              {key.status === "active" ? <Button aria-label={`撤销 ${key.name}`} onClick={() => setRevokeKey(key)} size="icon" title="撤销 API Key" variant="ghost"><ShieldX /></Button> : null}
            </div>
          ))}
        </div>
      </section>

      <Dialog onOpenChange={(open) => { setCreateOpen(open); setMessage(""); if (!open) { setCreated(null); setCopied(false); setName(""); } }} open={createOpen}>
        <DialogContent>{created ? <><DialogHeader><DialogTitle>保存你的 API Key</DialogTitle><DialogDescription>完整密钥关闭后无法再次查看。</DialogDescription></DialogHeader><div className="space-y-3"><Label htmlFor="created-api-key">API Key</Label><div className="flex gap-2"><Input className="font-mono text-xs" id="created-api-key" readOnly value={created.key} /><Button aria-label="复制 API Key" onClick={() => void copyCreatedKey()} size="icon" title="复制 API Key" variant="outline">{copied ? <Check /> : <Clipboard />}</Button></div>{copied ? <p className="text-sm text-emerald-700 dark:text-emerald-400" role="status">已复制到剪贴板</p> : null}</div><DialogFooter><Button onClick={() => setCreateOpen(false)}>{copied ? "已保存，关闭" : "关闭"}</Button></DialogFooter></> : <><DialogHeader><DialogTitle>创建 API Key</DialogTitle><DialogDescription>为不同设备或环境使用容易识别的名称。</DialogDescription></DialogHeader><form className="space-y-2" id="create-key-form" onSubmit={createKey}><Label htmlFor="key-name">名称</Label><Input autoFocus id="key-name" maxLength={64} onChange={(event) => setName(event.target.value)} placeholder="例如 Production" required value={name} /></form><DialogFooter><Button onClick={() => setCreateOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy} form="create-key-form" type="submit"><KeyRound />{busy ? "正在创建..." : "创建"}</Button></DialogFooter></>}</DialogContent>
      </Dialog>

      <AlertDialog onOpenChange={(open) => { if (!open) setRevokeKey(null); }} open={revokeKey !== null}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>撤销 API Key</AlertDialogTitle><AlertDialogDescription>撤销 {revokeKey?.name} 后无法恢复，使用该 Key 的客户端将立即失去访问权限。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void revoke(); }} variant="destructive">确认撤销</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
    </div>
  );
}
