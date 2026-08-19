"use client";

import { type FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Eye, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";

import { DataPagination } from "@/components/data-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import AdminUsageLogs from "./usage-logs";

type UserRecord = { id: string; username: string; display_name: string; role: "admin" | "member"; status: "active" | "disabled"; balance_micros?: number };
type UserPage = { users: UserRecord[]; total: number; offset: number; limit: number };
type WalletEntry = { id: string; reference_id: string; entry_type: "manual_adjustment" | "top_up" | "referral_reward" | "usage_reservation" | "usage_refund" | "usage_settlement" | "usage_compensation"; amount_micros: number; balance_after_micros: number; description: string; created_at: string };
type BalanceSummary = { wallet: { id: string; user_id: string; balance_micros: number; updated_at: string }; entries: WalletEntry[]; entries_total: number; entries_offset: number; entries_limit: number; reserved_micros: number };

const PAGE_SIZE = 20;
const emptyUserPage: UserPage = { users: [], total: 0, offset: 0, limit: PAGE_SIZE };
const money = (micros: number) => new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000);
const date = (value: string) => new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
const entryLabel = (type: WalletEntry["entry_type"]) => ({ manual_adjustment: "人工调整", top_up: "在线充值", referral_reward: "邀请返现", usage_reservation: "调用预占", usage_refund: "预占释放", usage_settlement: "结算补扣", usage_compensation: "异常补偿" })[type];

async function readAllUsers() {
  const firstResponse = await fetch("/api/admin/users?offset=0&limit=100", { cache: "no-store" });
  if (!firstResponse.ok) throw firstResponse;
  const first = (await firstResponse.json()) as UserPage;
  const pages: UserRecord[][] = [first.users];
  const requests: Promise<Response>[] = [];
  for (let offset = 100; offset < first.total; offset += 100) requests.push(fetch(`/api/admin/users?offset=${offset}&limit=100`, { cache: "no-store" }));
  for (const response of await Promise.all(requests)) {
    if (!response.ok) throw response;
    pages.push(((await response.json()) as UserPage).users);
  }
  return pages.flat().sort((a, b) => a.username.localeCompare(b.username));
}

export default function AdminBillingPage() {
  const router = useRouter();
  const [userPage, setUserPage] = useState<UserPage>(emptyUserPage);
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [userOffset, setUserOffset] = useState(0);
  const [userLimit, setUserLimit] = useState(PAGE_SIZE);
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [selectedUserID, setSelectedUserID] = useState("");
  const [summary, setSummary] = useState<BalanceSummary | null>(null);
  const [entryOffset, setEntryOffset] = useState(0);
  const [entryLimit, setEntryLimit] = useState(PAGE_SIZE);
  const [loadingUsers, setLoadingUsers] = useState(true);
  const [loadingEntries, setLoadingEntries] = useState(false);
  const [message, setMessage] = useState("");
  const [tab, setTab] = useState("users");

  const loadUsers = useCallback(async () => {
    setLoadingUsers(true);
    setMessage("");
    const query = new URLSearchParams({ offset: String(userOffset), limit: String(userLimit) });
    if (search) query.set("search", search);
    if (status !== "all") query.set("status", status);
    try {
      const response = await fetch(`/api/admin/users?${query}`, { cache: "no-store" });
      if (response.status === 401) { router.replace("/login"); return; }
      if (response.status === 403) { router.replace("/console"); return; }
      if (!response.ok) throw new Error();
      setUserPage((await response.json()) as UserPage);
    } catch { setMessage("用户余额加载失败，请稍后重试"); }
    finally { setLoadingUsers(false); }
  }, [router, search, status, userLimit, userOffset]);

  const loadEntries = useCallback(async () => {
    if (!selectedUserID) { setSummary(null); return; }
    setLoadingEntries(true);
    try {
      const response = await fetch(`/api/admin/users/${selectedUserID}/balance?offset=${entryOffset}&limit=${entryLimit}`, { cache: "no-store" });
      if (response.status === 401) { router.replace("/login"); return; }
      if (response.status === 403) { router.replace("/console"); return; }
      if (!response.ok) throw new Error();
      setSummary((await response.json()) as BalanceSummary);
    } catch { setMessage("余额流水加载失败，请稍后重试"); }
    finally { setLoadingEntries(false); }
  }, [entryLimit, entryOffset, router, selectedUserID]);

  useEffect(() => { const timer = window.setTimeout(() => void loadUsers(), 0); return () => window.clearTimeout(timer); }, [loadUsers]);
  useEffect(() => { const timer = window.setTimeout(() => void loadEntries(), 0); return () => window.clearTimeout(timer); }, [loadEntries]);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (new URLSearchParams(window.location.search).get("tab") === "usage") setTab("usage");
    }, 0);
    return () => window.clearTimeout(timer);
  }, []);
  useEffect(() => {
    let active = true;
    void readAllUsers().then((records) => { if (active) setUsers(records); }).catch((error: unknown) => {
      if (!active) return;
      if (error instanceof Response && error.status === 401) router.replace("/login");
      else if (error instanceof Response && error.status === 403) router.replace("/console");
      else setMessage("余额已加载，用户流水筛选暂时不可用");
    });
    return () => { active = false; };
  }, [router]);

  const pageBalance = useMemo(() => userPage.users.reduce((total, user) => total + (user.balance_micros ?? 0), 0), [userPage.users]);
  const selectedUser = users.find((user) => user.id === selectedUserID) ?? userPage.users.find((user) => user.id === selectedUserID);
  const chooseUser = (id: string) => { setSelectedUserID(id); setEntryOffset(0); setTab("entries"); };

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setUserOffset(0);
    setSearch(searchDraft.trim());
  }

  function clearSearch() { setSearchDraft(""); setSearch(""); setStatus("all"); setUserOffset(0); }

  return <div className="flex flex-col gap-5">
    <div className="flex items-end justify-between gap-4"><div><p className="text-sm text-muted-foreground">统一查看所有用户的账户余额、资金流水与模型调用</p><h2 className="mt-1 text-2xl font-semibold">余额与用量</h2></div>{tab !== "usage" ? <Button aria-label="刷新余额与流水" disabled={loadingUsers || loadingEntries} onClick={() => { void loadUsers(); void loadEntries(); }} size="icon" title="刷新余额与流水" variant="outline"><RefreshCw className={(loadingUsers || loadingEntries) ? "animate-spin" : ""} /></Button> : null}</div>
    {tab !== "usage" && message ? <div className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}
    {tab !== "usage" ? <section className="grid border-y bg-background sm:grid-cols-2 xl:grid-cols-4"><div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">用户总数</p><p className="mt-1 text-xl font-semibold">{userPage.total.toLocaleString("zh-CN")}</p></div><div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">本页余额合计</p><p className="mt-1 text-xl font-semibold">{money(pageBalance)}</p></div><div className="border-t px-4 py-4 xl:border-r xl:border-t-0"><p className="text-xs text-muted-foreground">当前用户余额</p><p className="mt-1 text-xl font-semibold">{summary ? money(summary.wallet.balance_micros) : "--"}</p></div><div className="border-t px-4 py-4 xl:border-t-0"><p className="text-xs text-muted-foreground">当前用户预占</p><p className="mt-1 text-xl font-semibold">{summary ? money(summary.reserved_micros) : "--"}</p></div></section> : null}
    <Tabs onValueChange={(value) => { setTab(value); if (value === "entries" && !selectedUserID && users[0]) chooseUser(users[0].id); }} value={tab}>
      <TabsList aria-label="管理员余额与用量" className="justify-start" variant="line"><TabsTrigger value="users">用户余额</TabsTrigger><TabsTrigger value="entries">余额流水</TabsTrigger><TabsTrigger value="usage">使用日志</TabsTrigger></TabsList>
      <TabsContent className="flex flex-col gap-4 pt-3" value="users">
        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between"><form className="flex min-w-0 flex-1 gap-2" onSubmit={submitSearch}><Input aria-label="搜索用户" className="max-w-md" onChange={(event) => setSearchDraft(event.target.value)} placeholder="搜索用户名或显示名称" value={searchDraft} /><Button type="submit" variant="outline">搜索</Button></form><div className="flex gap-2"><Select onValueChange={(value) => { setStatus(value); setUserOffset(0); }} value={status}><SelectTrigger aria-label="按状态筛选" className="w-32"><SelectValue /></SelectTrigger><SelectContent><SelectGroup><SelectItem value="all">全部状态</SelectItem><SelectItem value="active">启用</SelectItem><SelectItem value="disabled">停用</SelectItem></SelectGroup></SelectContent></Select><Button onClick={clearSearch} type="button" variant="outline">清除筛选</Button></div></div>
        <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>用户</TableHead><TableHead>角色</TableHead><TableHead>状态</TableHead><TableHead className="text-right">可用余额</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{loadingUsers ? <TableRow><TableCell className="h-28 text-center" colSpan={5}>加载中...</TableCell></TableRow> : null}{!loadingUsers && userPage.users.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={5}>没有符合条件的用户</TableCell></TableRow> : null}{!loadingUsers ? userPage.users.map((user) => <TableRow key={user.id}><TableCell><p className="font-medium">{user.display_name || user.username}</p><p className="text-xs text-muted-foreground">@{user.username}</p></TableCell><TableCell><Badge variant={user.role === "admin" ? "default" : "secondary"}>{user.role === "admin" ? "管理员" : "成员"}</Badge></TableCell><TableCell><Badge variant={user.status === "active" ? "outline" : "secondary"}>{user.status === "active" ? "启用" : "停用"}</Badge></TableCell><TableCell className="text-right font-medium tabular-nums">{money(user.balance_micros ?? 0)}</TableCell><TableCell><div className="flex justify-end"><Button aria-label={`查看 ${user.username} 的余额流水`} onClick={() => chooseUser(user.id)} size="icon-sm" title="查看余额流水" variant="ghost"><Eye /></Button></div></TableCell></TableRow>) : null}</TableBody></Table></div><DataPagination loading={loadingUsers} offset={userOffset} onOffsetChange={setUserOffset} onPageSizeChange={(limit) => { setUserOffset(0); setUserLimit(limit); }} pageSize={userLimit} total={userPage.total} /></CardContent></Card>
      </TabsContent>
      <TabsContent className="flex flex-col gap-4 pt-3" value="entries">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><Select disabled={users.length === 0} onValueChange={chooseUser} value={selectedUserID || undefined}><SelectTrigger aria-label="选择用户" className="w-full sm:w-80"><SelectValue placeholder="选择要查看流水的用户" /></SelectTrigger><SelectContent><SelectGroup>{users.map((user) => <SelectItem key={user.id} value={user.id}>{user.display_name || user.username} (@{user.username})</SelectItem>)}</SelectGroup></SelectContent></Select>{selectedUser ? <p className="text-sm text-muted-foreground">当前查看：{selectedUser.display_name || selectedUser.username}</p> : null}</div>
        {!selectedUserID ? <Card><CardContent className="p-8 text-center text-muted-foreground">请先选择用户查看余额流水</CardContent></Card> : <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>类型</TableHead><TableHead>说明</TableHead><TableHead>变动</TableHead><TableHead>变动后余额</TableHead><TableHead>时间</TableHead></TableRow></TableHeader><TableBody>{loadingEntries ? <TableRow><TableCell className="h-28 text-center" colSpan={5}>加载中...</TableCell></TableRow> : null}{!loadingEntries && (summary?.entries.length ?? 0) === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={5}>还没有余额流水</TableCell></TableRow> : null}{summary?.entries.map((entry) => <TableRow key={entry.id}><TableCell><Badge variant="secondary">{entryLabel(entry.entry_type)}</Badge></TableCell><TableCell>{entry.description}</TableCell><TableCell className={entry.amount_micros >= 0 ? "text-emerald-600 dark:text-emerald-400" : "text-foreground"}>{entry.amount_micros >= 0 ? "+" : ""}{money(entry.amount_micros)}</TableCell><TableCell>{money(entry.balance_after_micros)}</TableCell><TableCell className="whitespace-nowrap text-muted-foreground">{date(entry.created_at)}</TableCell></TableRow>)}</TableBody></Table></div><DataPagination loading={loadingEntries} offset={entryOffset} onOffsetChange={setEntryOffset} onPageSizeChange={(limit) => { setEntryOffset(0); setEntryLimit(limit); }} pageSize={entryLimit} total={summary?.entries_total ?? 0} /></CardContent></Card>}
      </TabsContent>
      <TabsContent className="pt-3" value="usage">
        <AdminUsageLogs />
      </TabsContent>
    </Tabs>
  </div>;
}
