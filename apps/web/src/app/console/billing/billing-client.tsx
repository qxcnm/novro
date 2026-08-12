"use client";

import { type FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { CreditCard, Landmark, QrCode, RefreshCw, Smartphone, WalletCards } from "lucide-react";
import { useRouter } from "next/navigation";

import { DataPagination } from "@/components/data-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

type WalletEntry = { id: string; reference_id: string; entry_type: "manual_adjustment" | "top_up" | "referral_reward" | "usage_reservation" | "usage_refund" | "usage_settlement"; amount_micros: number; balance_after_micros: number; description: string; created_at: string };
type BalanceSummary = { wallet: { id: string; user_id: string; balance_micros: number; updated_at: string }; entries: WalletEntry[]; entries_total: number; entries_offset: number; entries_limit: number; reserved_micros: number };
type Usage = { id: string; request_id: string; model: string; endpoint: string; input_tokens: number; uncached_input_tokens: number; cache_read_input_tokens: number; cache_write_input_tokens: number; cache_write_1h_input_tokens: number; output_tokens: number; multiplier_bps: number; billing_group_name: string; cost_micros: number; reserved_micros: number; estimated: boolean; created_at: string };
type UsagePage = { usage: Usage[]; total: number; offset: number; limit: number; total_tokens: number; total_cost_micros: number };
type PaymentMethod = { code: string; name: string; icon: string; min_micros: number; enabled: boolean };
type BonusTier = { threshold_micros: number; bonus_bps: number };
type TopUpConfig = { enabled: boolean; provider?: string; channels: string[]; methods: PaymentMethod[]; min_micros: number; max_micros: number; preset_amounts_micros: number[]; bonus_tiers: BonusTier[] };
type TopUpOrder = { id: string; out_trade_no: string; provider: string; channel: string; amount_micros: number; credited_micros: number; status: "pending" | "paid"; provider_trade_no?: string; paid_at?: string; created_at: string };
type TopUpPage = { orders: TopUpOrder[]; total: number; offset: number; limit: number };
type Checkout = { action: string; method: string; fields: Record<string, string> };
type ErrorResponse = { error?: { message?: string } };

const PAGE_SIZE = 20;
const defaultMinTopUpMicros = 1_000_000;
const emptyUsagePage: UsagePage = { usage: [], total: 0, offset: 0, limit: PAGE_SIZE, total_tokens: 0, total_cost_micros: 0 };
const emptyTopUpPage: TopUpPage = { orders: [], total: 0, offset: 0, limit: PAGE_SIZE };

function formatMoney(micros: number) { return new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 6 }).format(micros / 1_000_000); }
function formatDate(value: string) { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)); }
function entryLabel(type: WalletEntry["entry_type"]) { if (type === "manual_adjustment") return "人工调整"; if (type === "top_up") return "在线充值"; if (type === "referral_reward") return "邀请返现"; if (type === "usage_reservation") return "调用预占"; if (type === "usage_settlement") return "结算补扣"; return "预占释放"; }
function moneyInput(micros: number) { return String(micros / 1_000_000); }
function topUpStatus(status: TopUpOrder["status"]) { return status === "paid" ? "已到账" : "待支付"; }
async function readError(response: Response) { const body = (await response.json().catch(() => ({}))) as ErrorResponse; return body.error?.message ?? "加载失败，请稍后重试"; }

function parseAmountMicros(value: string) {
  const normalized = value.trim();
  if (!/^\d{1,8}(\.\d{1,2})?$/.test(normalized)) return null;
  const [yuan, fraction = ""] = normalized.split(".");
  const cents = Number(yuan) * 100 + Number(fraction.padEnd(2, "0"));
  return Number.isSafeInteger(cents) ? cents * 10_000 : null;
}

function submitCheckout(checkout: Checkout) {
  const form = document.createElement("form");
  form.method = checkout.method;
  form.action = checkout.action;
  form.style.display = "none";
  for (const [name, value] of Object.entries(checkout.fields)) {
    const input = document.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = value;
    form.appendChild(input);
  }
  document.body.appendChild(form);
  form.submit();
}

function MethodIcon({ icon }: { icon: string }) {
  if (icon === "smartphone") return <Smartphone />;
  if (icon === "qr-code") return <QrCode />;
  if (icon === "card") return <CreditCard />;
  if (icon === "landmark") return <Landmark />;
  return <WalletCards />;
}

function creditedAmount(amountMicros: number, tiers: BonusTier[]) {
  const bonusBPS = tiers.reduce((current, tier) => amountMicros >= tier.threshold_micros ? tier.bonus_bps : current, 0);
  return amountMicros + Math.floor(amountMicros * bonusBPS / 10_000);
}

export default function BillingClient() {
  const router = useRouter();
  const returnHandled = useRef(false);
  const [summary, setSummary] = useState<BalanceSummary | null>(null);
  const [usagePage, setUsagePage] = useState<UsagePage>(emptyUsagePage);
  const [usageOffset, setUsageOffset] = useState(0);
  const [usageLimit, setUsageLimit] = useState(PAGE_SIZE);
  const [entriesOffset, setEntriesOffset] = useState(0);
  const [entriesLimit, setEntriesLimit] = useState(PAGE_SIZE);
  const [topUpConfig, setTopUpConfig] = useState<TopUpConfig | null>(null);
  const [topUpPage, setTopUpPage] = useState<TopUpPage>(emptyTopUpPage);
  const [topUpOffset, setTopUpOffset] = useState(0);
  const [topUpLimit, setTopUpLimit] = useState(PAGE_SIZE);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [checkingOrder, setCheckingOrder] = useState("");
  const [message, setMessage] = useState("");
  const [topUpError, setTopUpError] = useState("");
  const [topUpOpen, setTopUpOpen] = useState(false);
  const [amount, setAmount] = useState("1");
  const [channel, setChannel] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setMessage("");
    const responses = await Promise.all([
      fetch(`/api/account/balance?offset=${entriesOffset}&limit=${entriesLimit}`, { cache: "no-store" }),
      fetch(`/api/account/usage?offset=${usageOffset}&limit=${usageLimit}`, { cache: "no-store" }),
      fetch("/api/account/top-ups/config", { cache: "no-store" }),
      fetch(`/api/account/top-ups?offset=${topUpOffset}&limit=${topUpLimit}`, { cache: "no-store" }),
    ]);
    if (responses.some((response) => response.status === 401)) { router.replace("/login"); return; }
    const failed = responses.find((response) => !response.ok);
    if (failed) { setMessage(await readError(failed)); setLoading(false); return; }
    const [balanceResponse, usageResponse, configResponse, ordersResponse] = responses;
    const rawConfig = (await configResponse.json()) as Partial<TopUpConfig>;
    const methods = Array.isArray(rawConfig.methods) ? rawConfig.methods : [];
    const config: TopUpConfig = {
      enabled: rawConfig.enabled === true,
      provider: rawConfig.provider,
      channels: Array.isArray(rawConfig.channels) ? rawConfig.channels : [],
      methods,
      min_micros: rawConfig.min_micros ?? defaultMinTopUpMicros,
      max_micros: rawConfig.max_micros ?? 50_000_000_000,
      preset_amounts_micros: Array.isArray(rawConfig.preset_amounts_micros) ? rawConfig.preset_amounts_micros : [],
      bonus_tiers: Array.isArray(rawConfig.bonus_tiers) ? rawConfig.bonus_tiers : [],
    };
    setSummary((await balanceResponse.json()) as BalanceSummary);
    setUsagePage((await usageResponse.json()) as UsagePage);
    setTopUpConfig(config);
    setChannel((current) => config.methods.some((method) => method.code === current) ? current : (config.methods[0]?.code ?? ""));
    setAmount((current) => {
      const currentMicros = parseAmountMicros(current);
      if (currentMicros !== null && currentMicros >= config.min_micros && currentMicros <= config.max_micros) return current;
      return moneyInput(config.preset_amounts_micros[0] ?? config.min_micros);
    });
    setTopUpPage((await ordersResponse.json()) as TopUpPage);
    setLoading(false);
  }, [entriesLimit, entriesOffset, router, topUpLimit, topUpOffset, usageLimit, usageOffset]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);

  useEffect(() => {
    const searchParams = new URLSearchParams(window.location.search);
    const paymentStatus = searchParams.get("payment");
    if (returnHandled.current || !paymentStatus) return;
    returnHandled.current = true;
    if (paymentStatus === "returned" && searchParams.get("out_trade_no")) {
      const query = searchParams.toString();
      void fetch(`/api/payments/epay/return?${query}`, { cache: "no-store" }).finally(() => {
        router.replace("/console/billing");
        void load().then(() => setMessage("支付结果已完成核对，请以订单状态和余额流水为准。"));
      });
      return;
    }
    const notice = paymentStatus === "returned"
      ? "支付结果已完成核对，请以订单状态和余额流水为准。"
      : paymentStatus === "failed"
        ? "支付结果暂未确认；如果已经扣款，请在待支付订单右侧查询支付结果。"
        : paymentStatus === "unavailable"
          ? "充值服务暂不可用，请稍后查询订单状态。"
          : "";
    const timer = window.setTimeout(() => {
      if (notice) setMessage(notice);
      router.replace("/console/billing");
    }, 0);
    return () => window.clearTimeout(timer);
  }, [load, router]);

  async function createTopUp(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const amountMicros = parseAmountMicros(amount);
    if (!topUpConfig || amountMicros === null || amountMicros < topUpConfig.min_micros || amountMicros > topUpConfig.max_micros) {
      setTopUpError(`充值金额需在 ${formatMoney(topUpConfig?.min_micros ?? defaultMinTopUpMicros)} 至 ${formatMoney(topUpConfig?.max_micros ?? 50_000_000_000)} 之间`);
      return;
    }
    const method = topUpConfig.methods.find((item) => item.code === channel);
    if (!method) { setTopUpError("请选择支付方式"); return; }
    if (amountMicros < method.min_micros) { setTopUpError(`${method.name}最低充值金额为 ${formatMoney(method.min_micros)}`); return; }
    setBusy(true);
    setTopUpError("");
    const response = await fetch("/api/account/top-ups", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ amount_micros: amountMicros, channel }),
    });
    if (response.status === 401) { router.replace("/login"); return; }
    if (!response.ok) { setTopUpError(await readError(response)); setBusy(false); return; }
    const result = (await response.json()) as { order: TopUpOrder; checkout: Checkout };
    setTopUpOpen(false);
    submitCheckout(result.checkout);
  }

  async function reconcileTopUp(order: TopUpOrder) {
    setCheckingOrder(order.out_trade_no);
    setMessage("");
    try {
      const response = await fetch(`/api/account/top-ups/${encodeURIComponent(order.out_trade_no)}/reconcile`, { method: "POST" });
      if (response.status === 401) { router.replace("/login"); return; }
      if (!response.ok) { setMessage(await readError(response)); return; }
      await load();
      setMessage("已从支付平台确认到账，余额和流水已刷新。");
    } finally { setCheckingOrder(""); }
  }

  const amountMicros = parseAmountMicros(amount);
  const previewCredit = topUpConfig && amountMicros !== null ? creditedAmount(amountMicros, topUpConfig.bonus_tiers) : null;

  return <>
    <div className="space-y-5">
      <div className="flex justify-end"><Button aria-label="刷新余额与用量" disabled={loading} onClick={() => void load()} size="icon" title="刷新余额与用量" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button></div>
      <section className="grid border-y bg-background sm:grid-cols-2 xl:grid-cols-4">
        <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">可用余额</p><p className="mt-1 text-xl font-semibold">{summary ? formatMoney(summary.wallet.balance_micros) : "--"}</p></div>
        <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">当前调用预占</p><p className="mt-1 text-xl font-semibold">{summary ? formatMoney(summary.reserved_micros) : "--"}</p></div>
        <div className="border-t px-4 py-4 xl:border-r xl:border-t-0"><p className="text-xs text-muted-foreground">累计调用 Tokens</p><p className="mt-1 text-xl font-semibold">{usagePage.total_tokens.toLocaleString("zh-CN")}</p></div>
        <div className="border-t px-4 py-4 xl:border-t-0"><p className="text-xs text-muted-foreground">累计调用费用</p><p className="mt-1 text-xl font-semibold">{formatMoney(usagePage.total_cost_micros)}</p></div>
      </section>
      {message ? <div className="rounded-md border bg-background px-4 py-3 text-sm" role="status">{message}</div> : null}

      <Tabs defaultValue="top-up">
        <TabsList aria-label="余额与用量" className="justify-start" variant="line"><TabsTrigger value="top-up">充值</TabsTrigger><TabsTrigger value="usage">最近调用</TabsTrigger><TabsTrigger value="entries">余额流水</TabsTrigger></TabsList>

        <TabsContent className="space-y-4 pt-3" value="top-up">
          <div className="flex justify-end"><Button disabled={!topUpConfig?.enabled || loading} onClick={() => { setTopUpError(""); setTopUpOpen(true); }} title={topUpConfig?.enabled ? "余额充值" : "充值暂未开放"}><CreditCard />充值</Button></div>
          <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>订单号</TableHead><TableHead>支付方式</TableHead><TableHead>金额</TableHead><TableHead>状态</TableHead><TableHead>创建时间</TableHead><TableHead className="w-12"><span className="sr-only">操作</span></TableHead></TableRow></TableHeader><TableBody>
            {loading ? <TableRow><TableCell className="h-24 text-center" colSpan={6}>加载中...</TableCell></TableRow> : null}
            {!loading && topUpPage.orders.length === 0 ? <TableRow><TableCell className="h-24 text-center text-muted-foreground" colSpan={6}>还没有充值订单</TableCell></TableRow> : null}
            {!loading ? topUpPage.orders.map((order) => <TableRow key={order.id}><TableCell className="font-mono text-xs">{order.out_trade_no}</TableCell><TableCell>{topUpConfig?.methods.find((method) => method.code === order.channel)?.name ?? order.channel}</TableCell><TableCell><p>{formatMoney(order.amount_micros)}</p>{order.credited_micros > order.amount_micros ? <p className="text-xs text-emerald-600 dark:text-emerald-400">到账 {formatMoney(order.credited_micros)}</p> : null}</TableCell><TableCell><Badge variant={order.status === "paid" ? "default" : "secondary"}>{topUpStatus(order.status)}</Badge></TableCell><TableCell className="text-muted-foreground">{formatDate(order.created_at)}</TableCell><TableCell>{order.status === "pending" ? <Button aria-label={`查询订单 ${order.out_trade_no} 的支付结果`} disabled={checkingOrder !== ""} onClick={() => void reconcileTopUp(order)} size="icon" title="查询支付结果" variant="ghost"><RefreshCw className={checkingOrder === order.out_trade_no ? "animate-spin" : ""} /></Button> : null}</TableCell></TableRow>) : null}
          </TableBody></Table></div><DataPagination loading={loading} offset={topUpOffset} onOffsetChange={setTopUpOffset} onPageSizeChange={(limit) => { setTopUpOffset(0); setTopUpLimit(limit); }} pageSize={topUpLimit} total={topUpPage.total} /></CardContent></Card>
        </TabsContent>

        <TabsContent className="pt-3" value="usage">
          <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>模型</TableHead><TableHead>Token 明细</TableHead><TableHead>分组倍率</TableHead><TableHead>预占 / 费用</TableHead><TableHead>时间</TableHead></TableRow></TableHeader><TableBody>
            {loading ? <TableRow><TableCell className="h-24 text-center" colSpan={5}>加载中...</TableCell></TableRow> : null}
            {!loading && usagePage.usage.length === 0 ? <TableRow><TableCell className="h-24 text-center text-muted-foreground" colSpan={5}>还没有调用记录</TableCell></TableRow> : null}
            {!loading ? usagePage.usage.map((item) => <TableRow key={item.id}><TableCell><p className="font-mono text-xs">{item.model}</p><p className="text-xs text-muted-foreground">{item.endpoint.replaceAll("_", " ")}</p></TableCell><TableCell><p>{(item.input_tokens + item.output_tokens).toLocaleString("zh-CN")}</p><p className="text-xs text-muted-foreground">普通 {item.uncached_input_tokens} · 命中 {item.cache_read_input_tokens} · 创建 {item.cache_write_input_tokens + item.cache_write_1h_input_tokens} · 输出 {item.output_tokens}</p></TableCell><TableCell><p>{item.billing_group_name || "默认分组"}</p><p className="font-mono text-xs text-muted-foreground">{(item.multiplier_bps / 10_000).toFixed(4)}×</p></TableCell><TableCell><p>{formatMoney(item.reserved_micros)} / {formatMoney(item.cost_micros)}</p>{item.estimated ? <Badge className="mt-1" variant="secondary">usage 不完整</Badge> : null}</TableCell><TableCell className="text-muted-foreground">{formatDate(item.created_at)}</TableCell></TableRow>) : null}
          </TableBody></Table></div><DataPagination loading={loading} offset={usageOffset} onOffsetChange={setUsageOffset} onPageSizeChange={(limit) => { setUsageOffset(0); setUsageLimit(limit); }} pageSize={usageLimit} total={usagePage.total} /></CardContent></Card>
        </TabsContent>

        <TabsContent className="pt-3" value="entries">
          <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>类型</TableHead><TableHead>说明</TableHead><TableHead>变动</TableHead><TableHead>变动后余额</TableHead><TableHead>时间</TableHead></TableRow></TableHeader><TableBody>
            {loading ? <TableRow><TableCell className="h-24 text-center" colSpan={5}>加载中...</TableCell></TableRow> : null}
            {!loading && (summary?.entries.length ?? 0) === 0 ? <TableRow><TableCell className="h-24 text-center text-muted-foreground" colSpan={5}>还没有余额流水</TableCell></TableRow> : null}
            {summary?.entries.map((entry) => <TableRow key={entry.id}><TableCell><Badge variant="secondary">{entryLabel(entry.entry_type)}</Badge></TableCell><TableCell>{entry.description}</TableCell><TableCell className={entry.amount_micros >= 0 ? "text-emerald-600 dark:text-emerald-400" : "text-foreground"}>{entry.amount_micros >= 0 ? "+" : ""}{formatMoney(entry.amount_micros)}</TableCell><TableCell>{formatMoney(entry.balance_after_micros)}</TableCell><TableCell className="text-muted-foreground">{formatDate(entry.created_at)}</TableCell></TableRow>)}
          </TableBody></Table></div><DataPagination loading={loading} offset={entriesOffset} onOffsetChange={setEntriesOffset} onPageSizeChange={(limit) => { setEntriesOffset(0); setEntriesLimit(limit); }} pageSize={entriesLimit} total={summary?.entries_total ?? 0} /></CardContent></Card>
        </TabsContent>
      </Tabs>
    </div>

    <Dialog onOpenChange={(open) => { if (!busy) setTopUpOpen(open); }} open={topUpOpen}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader><DialogTitle>余额充值</DialogTitle><DialogDescription>充值规则和支付方式由平台统一配置，到账记录只对你本人可见。</DialogDescription></DialogHeader>
        <form className="space-y-5" onSubmit={createTopUp}>
          <section className="grid grid-cols-2 border-y bg-muted/30"><div className="px-3 py-3"><p className="text-xs text-muted-foreground">最低充值</p><p className="mt-1 font-semibold">{topUpConfig ? formatMoney(topUpConfig.min_micros) : "--"}</p></div><div className="border-l px-3 py-3"><p className="text-xs text-muted-foreground">最高充值</p><p className="mt-1 font-semibold">{topUpConfig ? formatMoney(topUpConfig.max_micros) : "--"}</p></div></section>
          <div className="space-y-2"><Label htmlFor="top-up-amount">充值金额</Label><div className="relative"><span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span><Input className="pl-7" disabled={busy} id="top-up-amount" inputMode="decimal" onChange={(event) => { setAmount(event.target.value); setTopUpError(""); }} value={amount} /></div><div className="grid grid-cols-2 gap-2 sm:grid-cols-4">{topUpConfig?.preset_amounts_micros.map((value) => <Button aria-pressed={amount === moneyInput(value)} disabled={busy} key={value} onClick={() => setAmount(moneyInput(value))} type="button" variant={amount === moneyInput(value) ? "default" : "outline"}>{formatMoney(value)}</Button>)}</div>{previewCredit !== null && amountMicros !== null && previewCredit > amountMicros ? <p className="text-sm text-emerald-600 dark:text-emerald-400">预计到账 {formatMoney(previewCredit)}，含赠送 {formatMoney(previewCredit - amountMicros)}</p> : null}</div>
          {topUpConfig && topUpConfig.bonus_tiers.length > 0 ? <section className="space-y-2"><h3 className="text-sm font-medium">充值赠送</h3><div className="divide-y border-y">{topUpConfig.bonus_tiers.map((tier) => <div className="flex items-center justify-between gap-3 py-2 text-sm" key={tier.threshold_micros}><span>满 {formatMoney(tier.threshold_micros)}</span><span className="font-medium text-emerald-600 dark:text-emerald-400">赠送 {(tier.bonus_bps / 100).toFixed(2)}%</span></div>)}</div></section> : null}
          <fieldset className="space-y-2"><legend className="text-sm font-medium">支付方式</legend><div className="grid grid-cols-2 gap-2">{topUpConfig?.methods.map((method) => <Button aria-pressed={channel === method.code} className="h-auto min-h-14 justify-start py-2 text-left" disabled={busy} key={method.code} onClick={() => { setChannel(method.code); setTopUpError(""); }} type="button" variant={channel === method.code ? "default" : "outline"}><MethodIcon icon={method.icon} /><span><span className="block">{method.name}</span><span className="block text-xs font-normal opacity-70">最低 {formatMoney(method.min_micros)}</span></span></Button>)}</div></fieldset>
          {topUpError ? <div className="rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive" role="alert">{topUpError}</div> : null}
          <DialogFooter><Button disabled={busy || !channel} type="submit"><CreditCard />{busy ? "正在创建订单..." : "前往支付"}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </>;
}
