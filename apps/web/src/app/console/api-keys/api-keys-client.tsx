"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Check, Clipboard, KeyRound, Plus, RefreshCw, ScrollText, ShieldX } from "lucide-react";
import { useRouter } from "next/navigation";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type APIKeyRecord = { id: string; name: string; key_prefix: string; status: "active" | "revoked"; last_used_at: string | null; created_at: string; revoked_at: string | null };
type CreateResult = { api_key: APIKeyRecord; key: string };
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
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [created, setCreated] = useState<CreateResult | null>(null);
  const [copied, setCopied] = useState(false);
  const [revokeKey, setRevokeKey] = useState<APIKeyRecord | null>(null);

  const loadKeys = useCallback(async () => {
    setLoading(true);
    const response = await fetch("/api/account/api-keys", { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (!response.ok) { setMessage(await readError(response)); setLoading(false); return; }
    const body = (await response.json()) as { api_keys: APIKeyRecord[] };
    setKeys(body.api_keys); setLoading(false);
  }, [router]);

  useEffect(() => { const timer = window.setTimeout(() => void loadKeys(), 0); return () => window.clearTimeout(timer); }, [loadKeys]);

  async function createKey(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    const response = await fetch("/api/account/api-keys", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name }) });
    setBusy(false);
    if (!response.ok) { setError(await readError(response)); return; }
    const result = (await response.json()) as CreateResult;
    setCreated(result); setName(""); await loadKeys();
  }

  async function copyKey() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.key);
      setCopied(true);
      setError("");
    } catch {
      setError("复制失败，请手动选择并复制完整密钥");
    }
  }

  async function revoke() {
    if (!revokeKey) return;
    setBusy(true);
    const response = await fetch(`/api/account/api-keys/${revokeKey.id}`, { method: "DELETE" });
    setBusy(false);
    if (!response.ok) { setMessage(await readError(response)); setRevokeKey(null); return; }
    setMessage("API Key 已撤销"); setRevokeKey(null); await loadKeys();
  }

  const activeCount = keys.filter((key) => key.status === "active").length;

  return (
    <>
      <div className="space-y-5">
        <section className="flex flex-col gap-4 border-y bg-background px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div><p className="text-xs text-muted-foreground">启用中的 Key</p><p className="mt-1 text-xl font-semibold">{activeCount} <span className="text-sm font-normal text-muted-foreground">/ 10</span></p></div>
          <div className="flex gap-2"><Button aria-label="刷新 API Key" disabled={loading} onClick={() => void loadKeys()} size="icon" title="刷新 API Key" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button><Button onClick={() => setCreateOpen(true)}><Plus />创建 API Key</Button></div>
        </section>
        {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}
        <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>名称</TableHead><TableHead>前缀</TableHead><TableHead>状态</TableHead><TableHead>最近使用</TableHead><TableHead>创建时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>
          {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={6}>加载中...</TableCell></TableRow> : null}
          {!loading && keys.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={6}>还没有 API Key</TableCell></TableRow> : null}
          {!loading ? keys.map((key) => <TableRow key={key.id}><TableCell className="font-medium">{key.name}</TableCell><TableCell className="font-mono text-xs">{key.key_prefix}••••</TableCell><TableCell><Badge variant={key.status === "active" ? "outline" : "secondary"}>{key.status === "active" ? "启用" : "已撤销"}</Badge></TableCell><TableCell className="text-muted-foreground">{formatDate(key.last_used_at)}</TableCell><TableCell className="text-muted-foreground">{formatDate(key.created_at)}</TableCell><TableCell><div className="flex justify-end gap-1"><Button aria-label={`查看 ${key.name} 使用日志`} onClick={() => router.push(`/console/logs?key_id=${key.id}`)} size="icon-sm" title="查看使用日志" variant="ghost"><ScrollText /></Button>{key.status === "active" ? <Button aria-label={`撤销 ${key.name}`} onClick={() => setRevokeKey(key)} size="icon-sm" title="撤销 API Key" variant="ghost"><ShieldX /></Button> : null}</div></TableCell></TableRow>) : null}
        </TableBody></Table></div></CardContent></Card>

        <Dialog onOpenChange={(open) => { setCreateOpen(open); setError(""); if (!open) { setCreated(null); setCopied(false); setName(""); } }} open={createOpen}>
          <DialogContent>{created ? <><DialogHeader><DialogTitle>保存你的 API Key</DialogTitle><DialogDescription>完整密钥只显示这一次。关闭后无法再次查看，需要时只能重新创建。</DialogDescription></DialogHeader><div className="space-y-3"><Label htmlFor="created-api-key">API Key</Label><div className="flex gap-2"><Input className="font-mono text-xs" id="created-api-key" readOnly value={created.key} /><Button aria-label="复制 API Key" onClick={() => void copyKey()} size="icon" title="复制 API Key" variant="outline">{copied ? <Check /> : <Clipboard />}</Button></div><p className="text-xs text-muted-foreground">Key 前缀：{created.api_key.key_prefix}</p>{error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}</div><DialogFooter><Button onClick={() => setCreateOpen(false)}>{copied ? "已保存，关闭" : "我已保存"}</Button></DialogFooter></> : <><DialogHeader><DialogTitle>创建 API Key</DialogTitle><DialogDescription>为不同环境使用独立名称，便于发现风险时单独撤销。</DialogDescription></DialogHeader><form className="space-y-2" id="create-key-form" onSubmit={createKey}><Label htmlFor="key-name">名称</Label><Input autoFocus id="key-name" maxLength={64} onChange={(event) => setName(event.target.value)} placeholder="例如 Production" required value={name} />{error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}</form><DialogFooter><Button onClick={() => setCreateOpen(false)} type="button" variant="outline">取消</Button><Button disabled={busy} form="create-key-form" type="submit"><KeyRound />{busy ? "正在创建..." : "创建"}</Button></DialogFooter></>}</DialogContent>
        </Dialog>

        <AlertDialog onOpenChange={(open) => { if (!open) setRevokeKey(null); }} open={revokeKey !== null}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>撤销 API Key</AlertDialogTitle><AlertDialogDescription>撤销 {revokeKey?.name} 后无法恢复，使用该 Key 的客户端将失去访问权限。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void revoke(); }}>确认撤销</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
      </div>
    </>
  );
}
