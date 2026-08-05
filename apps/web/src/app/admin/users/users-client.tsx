"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { LogOut, Plus, RefreshCw, ShieldCheck, UserRound, UserRoundX } from "lucide-react";
import { useRouter } from "next/navigation";

import { ThemeToggle } from "@/components/theme-toggle";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type UserRecord = {
  id: string;
  username: string;
  display_name: string;
  role: "admin" | "member";
  status: "active" | "disabled";
  created_at: string;
  last_login_at: string | null;
};

type UserPage = { users: UserRecord[]; total: number };
type ErrorResponse = { error?: { message?: string } };

async function readError(response: Response) {
  const body = (await response.json().catch(() => ({}))) as ErrorResponse;
  return body.error?.message ?? "操作失败，请稍后重试";
}

function formatDate(value: string | null) {
  if (!value) return "从未登录";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export default function UsersClient() {
  const router = useRouter();
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [resetUser, setResetUser] = useState<UserRecord | null>(null);
  const [form, setForm] = useState({ username: "", display_name: "", password: "", role: "member" as "admin" | "member" });
  const [resetPassword, setResetPassword] = useState("");

  const loadUsers = useCallback(async () => {
    setLoading(true);
    const response = await fetch("/api/admin/users?limit=100", { cache: "no-store" });
    if (response.status === 401 || response.status === 403) {
      router.replace("/login");
      return;
    }
    if (!response.ok) {
      setMessage(await readError(response));
      setLoading(false);
      return;
    }
    const page = (await response.json()) as UserPage;
    setUsers(page.users);
    setTotal(page.total);
    setLoading(false);
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadUsers(), 0);
    return () => window.clearTimeout(timer);
  }, [loadUsers]);

  async function createUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("");
    const response = await fetch("/api/admin/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(form),
    });
    if (!response.ok) {
      setMessage(await readError(response));
      return;
    }
    setForm({ username: "", display_name: "", password: "", role: "member" });
    setCreateOpen(false);
    setMessage("用户已创建");
    await loadUsers();
  }

  async function toggleStatus(record: UserRecord) {
    setMessage("");
    const response = await fetch(`/api/admin/users/${record.id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: record.status === "active" ? "disabled" : "active" }),
    });
    if (!response.ok) {
      setMessage(await readError(response));
      return;
    }
    await loadUsers();
  }

  async function submitReset(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!resetUser) return;
    const response = await fetch(`/api/admin/users/${resetUser.id}/reset-password`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: resetPassword }),
    });
    if (!response.ok) {
      setMessage(await readError(response));
      return;
    }
    setResetUser(null);
    setResetPassword("");
    setMessage("密码已重置，原有会话已退出");
  }

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST" });
    router.replace("/login");
  }

  return (
    <main className="min-h-screen bg-muted/30">
      <header className="flex h-16 items-center justify-between border-b bg-background px-6 lg:px-10">
        <div className="flex items-center gap-3">
          <span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <ShieldCheck className="size-4" aria-hidden="true" />
          </span>
          <span className="text-sm font-semibold">Novro Console</span>
          <Separator className="mx-2 h-5" orientation="vertical" />
          <span className="text-sm text-muted-foreground">用户管理</span>
        </div>
        <div className="flex items-center gap-1">
          <ThemeToggle />
          <Button aria-label="退出登录" onClick={logout} size="icon" title="退出登录" variant="ghost">
            <LogOut aria-hidden="true" />
          </Button>
        </div>
      </header>

      <section className="mx-auto w-full max-w-6xl space-y-6 px-6 py-8 lg:px-10">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <p className="text-sm text-muted-foreground">管理后台</p>
            <h1 className="mt-1 text-2xl font-semibold">用户</h1>
            <p className="mt-1 text-sm text-muted-foreground">共 {total} 个用户</p>
          </div>
          <div className="flex gap-2">
            <Button onClick={() => void loadUsers()} variant="outline">
              <RefreshCw aria-hidden="true" />
              刷新
            </Button>
            <Button onClick={() => setCreateOpen((open) => !open)}>
              <Plus aria-hidden="true" />
              创建用户
            </Button>
          </div>
        </div>

        {message ? <p className="text-sm text-muted-foreground" role="status">{message}</p> : null}

        {createOpen ? (
          <Card>
            <CardHeader>
              <CardTitle>创建用户</CardTitle>
              <CardDescription>密码只用于创建时验证，不会在列表中显示。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4 md:grid-cols-4" onSubmit={createUser}>
                <div className="space-y-2"><Label htmlFor="new-username">用户名</Label><Input id="new-username" onChange={(e) => setForm({ ...form, username: e.target.value })} required value={form.username} /></div>
                <div className="space-y-2"><Label htmlFor="new-display-name">显示名称</Label><Input id="new-display-name" onChange={(e) => setForm({ ...form, display_name: e.target.value })} value={form.display_name} /></div>
                <div className="space-y-2"><Label htmlFor="new-password">初始密码</Label><Input id="new-password" minLength={12} onChange={(e) => setForm({ ...form, password: e.target.value })} required type="password" value={form.password} /></div>
                <div className="space-y-2"><Label>角色</Label><Select onValueChange={(role: "admin" | "member") => setForm({ ...form, role })} value={form.role}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="member">成员</SelectItem><SelectItem value="admin">管理员</SelectItem></SelectContent></Select></div>
                <div className="flex items-end gap-2"><Button type="submit"><UserRound aria-hidden="true" />保存</Button><Button onClick={() => setCreateOpen(false)} type="button" variant="outline">取消</Button></div>
              </form>
            </CardContent>
          </Card>
        ) : null}

        {resetUser ? (
          <Card>
            <CardHeader><CardTitle>重置密码：{resetUser.username}</CardTitle><CardDescription>重置后该用户的已有会话会立即失效。</CardDescription></CardHeader>
            <CardContent><form className="flex max-w-lg flex-wrap gap-2" onSubmit={submitReset}><Input minLength={12} onChange={(e) => setResetPassword(e.target.value)} placeholder="输入新密码" required type="password" value={resetPassword} /><Button type="submit">确认重置</Button><Button onClick={() => setResetUser(null)} type="button" variant="outline">取消</Button></form></CardContent>
          </Card>
        ) : null}

        <Card>
          <CardContent className="p-0">
            <Table>
              <TableHeader><TableRow><TableHead>用户</TableHead><TableHead>角色</TableHead><TableHead>状态</TableHead><TableHead>最近登录</TableHead><TableHead>创建时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {loading ? <TableRow><TableCell className="h-24 text-center" colSpan={6}>加载中...</TableCell></TableRow> : null}
                {!loading && users.length === 0 ? <TableRow><TableCell className="h-24 text-center text-muted-foreground" colSpan={6}>暂无用户</TableCell></TableRow> : null}
                {!loading ? users.map((record) => <TableRow key={record.id}>
                  <TableCell><div className="font-medium">{record.display_name || record.username}</div><div className="text-xs text-muted-foreground">{record.username}</div></TableCell>
                  <TableCell><Badge variant={record.role === "admin" ? "default" : "secondary"}>{record.role === "admin" ? "管理员" : "成员"}</Badge></TableCell>
                  <TableCell><Badge variant={record.status === "active" ? "outline" : "destructive"}>{record.status === "active" ? "启用" : "停用"}</Badge></TableCell>
                  <TableCell className="text-muted-foreground">{formatDate(record.last_login_at)}</TableCell>
                  <TableCell className="text-muted-foreground">{formatDate(record.created_at)}</TableCell>
                  <TableCell><div className="flex justify-end gap-1"><Button onClick={() => void toggleStatus(record)} size="sm" variant="ghost">{record.status === "active" ? <UserRoundX aria-hidden="true" /> : <UserRound aria-hidden="true" />}{record.status === "active" ? "停用" : "启用"}</Button><Button onClick={() => setResetUser(record)} size="sm" variant="ghost">重置密码</Button></div></TableCell>
                </TableRow>) : null}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </section>
    </main>
  );
}
