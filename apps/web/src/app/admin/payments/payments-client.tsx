"use client";

import { type FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clipboard, CreditCard, KeyRound, Landmark, Pencil, Plus, QrCode, RefreshCw, Save, Search, Smartphone, Trash2, WalletCards, XCircle } from "lucide-react";
import { useRouter } from "next/navigation";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { copyText } from "@/lib/clipboard";

type PaymentMethod = { code: string; name: string; icon: string; min_micros: number; enabled: boolean };
type BonusTier = { threshold_micros: number; bonus_bps: number };
type PaymentConfig = {
  provider: string;
  enabled: boolean;
  configured: boolean;
  api_url: string;
  merchant_id: string;
  site_name: string;
  channels: string[];
  methods: PaymentMethod[];
  min_micros: number;
  max_micros: number;
  preset_amounts_micros: number[];
  bonus_tiers: BonusTier[];
  notify_url: string;
  return_url: string;
  has_merchant_key: boolean;
  updated_at?: string;
};
type PaymentConfigResponse = Partial<Omit<PaymentConfig, "channels" | "methods" | "preset_amounts_micros" | "bonus_tiers">> & {
  channels?: string[] | null;
  methods?: PaymentMethod[] | null;
  preset_amounts_micros?: number[] | null;
  bonus_tiers?: BonusTier[] | null;
};
type ConfigForm = Omit<PaymentConfig, "configured" | "has_merchant_key" | "updated_at" | "min_micros" | "max_micros"> & {
  merchant_key: string;
  min_amount: string;
  max_amount: string;
};
type MethodForm = { code: string; name: string; icon: string; min_amount: string; enabled: boolean };
type BonusForm = { threshold: string; percent: string };
type TopUpOrder = {
  id: string;
  out_trade_no: string;
  provider: string;
  channel: string;
  amount_micros: number;
  credited_micros: number;
  status: "pending" | "paid";
  provider_trade_no?: string;
  paid_at?: string;
  created_at: string;
  owner: { id: string; username: string; display_name: string };
};
type TopUpPage = { orders: TopUpOrder[]; total: number; offset: number; limit: number };

const defaultMinTopUpMicros = 1_000_000;
const defaultPresets = [10_000_000, 50_000_000, 100_000_000, 500_000_000];
const emptyMethod: MethodForm = { code: "", name: "", icon: "wallet", min_amount: "1", enabled: true };
const emptyBonus: BonusForm = { threshold: "", percent: "" };
const iconOptions = [
  { value: "wallet", label: "钱包" },
  { value: "smartphone", label: "手机" },
  { value: "qr-code", label: "二维码" },
  { value: "card", label: "银行卡" },
  { value: "landmark", label: "银行" },
];

/**
 * readError 封装该名称对应的业务处理逻辑。
 * @param response 当前响应数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
async function readError(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

/**
 * formatMoney 封装该名称对应的业务处理逻辑。
 * @param micros 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatMoney(micros: number) {
  return new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY", minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(micros / 1_000_000);
}

/**
 * formatDate 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatDate(value?: string) {
  return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "--";
}

/**
 * moneyInput 封装该名称对应的业务处理逻辑。
 * @param micros 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function moneyInput(micros: number) {
  return Number.isFinite(micros) ? String(micros / 1_000_000) : "";
}

/**
 * parseMoneyMicros 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function parseMoneyMicros(value: string) {
  const normalized = value.trim();
  if (!/^\d{1,8}(\.\d{1,2})?$/.test(normalized)) return null;
  const [yuan, fraction = ""] = normalized.split(".");
  const cents = Number(yuan) * 100 + Number(fraction.padEnd(2, "0"));
  return Number.isSafeInteger(cents) ? cents * 10_000 : null;
}

/**
 * normalizePaymentConfig 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function normalizePaymentConfig(value?: PaymentConfigResponse | null): PaymentConfig {
  return {
    provider: value?.provider ?? "epay",
    enabled: value?.enabled === true,
    configured: value?.configured === true,
    api_url: value?.api_url ?? "",
    merchant_id: value?.merchant_id ?? "",
    site_name: value?.site_name || "Novro",
    channels: Array.isArray(value?.channels) ? value.channels : [],
    methods: Array.isArray(value?.methods) ? value.methods : [],
    min_micros: value?.min_micros ?? defaultMinTopUpMicros,
    max_micros: value?.max_micros ?? 50_000_000_000,
    preset_amounts_micros: Array.isArray(value?.preset_amounts_micros) && value.preset_amounts_micros.length > 0 ? value.preset_amounts_micros : defaultPresets,
    bonus_tiers: Array.isArray(value?.bonus_tiers) ? value.bonus_tiers : [],
    notify_url: value?.notify_url ?? "",
    return_url: value?.return_url ?? "",
    has_merchant_key: value?.has_merchant_key === true,
    updated_at: value?.updated_at,
  };
}

/**
 * configToForm 封装该名称对应的业务处理逻辑。
 * @param config 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function configToForm(config: PaymentConfig): ConfigForm {
  return {
    provider: config.provider,
    enabled: config.enabled,
    api_url: config.api_url,
    merchant_id: config.merchant_id,
    merchant_key: "",
    site_name: config.site_name,
    channels: config.channels,
    methods: config.methods.map((method) => ({ ...method })),
    min_amount: moneyInput(config.min_micros),
    max_amount: moneyInput(config.max_micros),
    preset_amounts_micros: [...config.preset_amounts_micros],
    bonus_tiers: config.bonus_tiers.map((tier) => ({ ...tier })),
    notify_url: config.notify_url,
    return_url: config.return_url,
  };
}

/**
 * MethodIcon 渲染对应的 React 界面组件。
 * @param icon 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function MethodIcon({ icon }: { icon: string }) {
  if (icon === "smartphone") return <Smartphone />;
  if (icon === "qr-code") return <QrCode />;
  if (icon === "card") return <CreditCard />;
  if (icon === "landmark") return <Landmark />;
  return <WalletCards />;
}

/**
 * PaymentsClient 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function PaymentsClient() {
  const router = useRouter();
  const [config, setConfig] = useState<PaymentConfig | null>(null);
  const [form, setForm] = useState<ConfigForm>(() => configToForm(normalizePaymentConfig()));
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [methodQuery, setMethodQuery] = useState("");
  const [methodOpen, setMethodOpen] = useState(false);
  const [methodIndex, setMethodIndex] = useState<number | null>(null);
  const [methodForm, setMethodForm] = useState<MethodForm>(emptyMethod);
  const [deletingMethod, setDeletingMethod] = useState<number | null>(null);
  const [presetInput, setPresetInput] = useState("");
  const [bonusOpen, setBonusOpen] = useState(false);
  const [bonusIndex, setBonusIndex] = useState<number | null>(null);
  const [bonusForm, setBonusForm] = useState<BonusForm>(emptyBonus);
  const [orders, setOrders] = useState<TopUpOrder[]>([]);
  const [orderTotal, setOrderTotal] = useState(0);
  const [ordersLoading, setOrdersLoading] = useState(true);
  const [orderQuery, setOrderQuery] = useState("");
  const [orderStatus, setOrderStatus] = useState("all");
  const [orderChannel, setOrderChannel] = useState("all");
  const [orderOffset, setOrderOffset] = useState(0);
  const orderLimit = 20;

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    const response = await fetch("/api/admin/payments", { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) { setError(await readError(response)); setLoading(false); return; }
    const next = normalizePaymentConfig(((await response.json()) as { payment_config?: PaymentConfigResponse }).payment_config);
    setConfig(next);
    setForm(configToForm(next));
    setLoading(false);
  }, [router]);

  const loadOrders = useCallback(async () => {
    setOrdersLoading(true);
    const query = new URLSearchParams({ offset: String(orderOffset), limit: String(orderLimit) });
    if (orderQuery.trim()) query.set("search", orderQuery.trim());
    if (orderStatus !== "all") query.set("status", orderStatus);
    if (orderChannel !== "all") query.set("channel", orderChannel);
    const response = await fetch(`/api/admin/top-ups?${query.toString()}`, { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) { setError(await readError(response)); setOrdersLoading(false); return; }
    /**
     * page 封装该名称对应的业务处理逻辑。
     * @param await 本次操作需要使用的输入参数。
     * @author Gao Hongshun
     * @date 2026-08-13
     */
    const page = (await response.json()) as TopUpPage;
    setOrders(Array.isArray(page.orders) ? page.orders : []);
    setOrderTotal(page.total ?? 0);
    setOrdersLoading(false);
  }, [orderChannel, orderOffset, orderQuery, orderStatus, router]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => window.clearTimeout(timer); }, [load]);
  useEffect(() => { const timer = window.setTimeout(() => void loadOrders(), 200); return () => window.clearTimeout(timer); }, [loadOrders]);

  const activeMethods = useMemo(() => form.methods.filter((method) => method.enabled).length, [form.methods]);
  const filteredMethods = useMemo(() => {
    const needle = methodQuery.trim().toLowerCase();
    return form.methods.map((method, index) => ({ method, index })).filter(({ method }) => !needle || `${method.name} ${method.code}`.toLowerCase().includes(needle));
  }, [form.methods, methodQuery]);
  const statusLabel = config?.enabled && config.configured ? "已启用" : config?.configured ? "已配置但停用" : "未配置";
  const statusVariant = config?.enabled && config.configured ? "default" : config?.configured ? "secondary" : "outline";

  /**
   * beginMethod 封装该名称对应的业务处理逻辑。
   * @param index 本次操作需要使用的输入参数。
   * @param template 本次操作需要使用的输入参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function beginMethod(index: number | null, template?: Partial<MethodForm>) {
    setMethodIndex(index);
    if (index === null) setMethodForm({ ...emptyMethod, ...template });
    else {
      const method = form.methods[index];
      setMethodForm({ code: method.code, name: method.name, icon: method.icon, min_amount: moneyInput(method.min_micros), enabled: method.enabled });
    }
    setMethodOpen(true);
  }

  /**
   * saveMethod 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function saveMethod(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const code = methodForm.code.trim().toLowerCase();
    const name = methodForm.name.trim();
    const minMicros = parseMoneyMicros(methodForm.min_amount);
    if (!/^[a-z0-9][a-z0-9_-]{0,31}$/.test(code)) { setError("支付处理标识需为 1 到 32 位，只能使用小写字母、数字、下划线和连字符，不能包含点号或空格"); return; }
    if (!name) { setError("请输入支付方式显示名称，最多 32 个字符"); return; }
    if (minMicros === null) { setError("最低充值金额需为人民币金额，最多保留 2 位小数"); return; }
    if (form.methods.some((method, index) => method.code === code && index !== methodIndex)) { setError("支付处理标识不能重复"); return; }
    const nextMethod: PaymentMethod = { code, name, icon: methodForm.icon, min_micros: minMicros, enabled: methodForm.enabled };
    setForm((current) => ({ ...current, methods: methodIndex === null ? [...current.methods, nextMethod] : current.methods.map((method, index) => index === methodIndex ? nextMethod : method) }));
    setMethodOpen(false);
    setMethodIndex(null);
    setError("");
  }

  /**
   * addPreset 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function addPreset() {
    const micros = parseMoneyMicros(presetInput);
    if (micros === null) { setError("预设充值金额需为人民币金额，最多保留 2 位小数"); return; }
    if (form.preset_amounts_micros.includes(micros)) { setError("该预设充值金额已存在"); return; }
    setForm((current) => ({ ...current, preset_amounts_micros: [...current.preset_amounts_micros, micros].sort((a, b) => a - b) }));
    setPresetInput("");
    setError("");
  }

  /**
   * beginBonus 封装该名称对应的业务处理逻辑。
   * @param index 本次操作需要使用的输入参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function beginBonus(index: number | null) {
    setBonusIndex(index);
    setBonusForm(index === null ? emptyBonus : { threshold: moneyInput(form.bonus_tiers[index].threshold_micros), percent: String(form.bonus_tiers[index].bonus_bps / 100) });
    setBonusOpen(true);
  }

  /**
   * saveBonus 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function saveBonus(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const thresholdMicros = parseMoneyMicros(bonusForm.threshold);
    const percent = Number(bonusForm.percent);
    if (thresholdMicros === null) { setError("赠送门槛需为人民币金额，最多保留 2 位小数"); return; }
    if (!Number.isFinite(percent) || percent <= 0 || percent > 100) { setError("赠送比例必须大于 0% 且不超过 100%"); return; }
    const tier = { threshold_micros: thresholdMicros, bonus_bps: Math.round(percent * 100) };
    if (form.bonus_tiers.some((item, index) => item.threshold_micros === thresholdMicros && index !== bonusIndex)) { setError("同一个充值门槛只能设置一个赠送档位"); return; }
    setForm((current) => ({ ...current, bonus_tiers: (bonusIndex === null ? [...current.bonus_tiers, tier] : current.bonus_tiers.map((item, index) => index === bonusIndex ? tier : item)).sort((a, b) => a.threshold_micros - b.threshold_micros) }));
    setBonusOpen(false);
    setBonusIndex(null);
    setError("");
  }

  /**
   * submit 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const minMicros = parseMoneyMicros(form.min_amount);
    const maxMicros = parseMoneyMicros(form.max_amount);
    if (minMicros === null || maxMicros === null) { setError("全局充值金额需为人民币金额，最多保留 2 位小数"); return; }
    if (minMicros > maxMicros) { setError("最低充值金额不能大于最高充值金额"); return; }
    if (form.methods.some((method) => method.min_micros < minMicros || method.min_micros > maxMicros)) { setError("支付方式的最低充值金额必须位于全局金额范围内"); return; }
    if (form.preset_amounts_micros.some((amount) => amount < minMicros || amount > maxMicros)) { setError("预设充值金额必须位于全局金额范围内"); return; }
    if (form.enabled && form.methods.every((method) => !method.enabled)) { setError("启用支付前至少需要一个已启用的支付方式"); return; }
    setBusy(true); setError(""); setMessage("");
    const payload: Record<string, unknown> = {
      enabled: form.enabled, api_url: form.api_url, merchant_id: form.merchant_id, site_name: form.site_name,
      methods: form.methods, min_micros: minMicros, max_micros: maxMicros,
      preset_amounts_micros: form.preset_amounts_micros, bonus_tiers: form.bonus_tiers,
    };
    if (form.merchant_key.trim()) payload.merchant_key = form.merchant_key.trim();
    const response = await fetch("/api/admin/payments", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) });
    setBusy(false);
    if (!response.ok) { setError(await readError(response)); return; }
    const next = normalizePaymentConfig(((await response.json()) as { payment_config?: PaymentConfigResponse }).payment_config);
    setConfig(next);
    setForm(configToForm(next));
    setMessage("支付网关与充值规则已保存");
  }

  /**
   * copyAddress 封装该名称对应的业务处理逻辑。
   * @param value 需要处理的输入值。
   * @param label 本次操作需要使用的输入参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function copyAddress(value: string, label: string) {
    if (!value) return;
    if (await copyText(value)) {
      setMessage(`${label}已复制`);
      setError("");
    } else {
      setError(`${label}复制失败，请手动选择并复制`);
    }
  }

  return (
    <div className="space-y-5">
      <section className="grid border-y bg-background sm:grid-cols-4">
        <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">支付网关</p><p className="mt-1 flex items-center gap-2 text-xl font-semibold"><CreditCard className="size-5 text-muted-foreground" />易支付</p></div>
        <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">当前状态</p><p className="mt-1"><Badge variant={statusVariant}>{config?.enabled && config.configured ? <CheckCircle2 /> : <XCircle />}{statusLabel}</Badge></p></div>
        <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">启用方式</p><p className="mt-1 text-xl font-semibold">{activeMethods}</p></div>
        <div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">充值订单</p><p className="mt-1 text-xl font-semibold">{orderTotal}</p></div>
      </section>

      <form className="space-y-5" onSubmit={submit}>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <Tabs className="min-w-0 flex-1" defaultValue="rules">
            <TabsList className="max-w-full justify-start overflow-x-auto">
              <TabsTrigger value="rules">充值规则</TabsTrigger>
              <TabsTrigger value="epay">易支付</TabsTrigger>
              <TabsTrigger value="records">充值记录</TabsTrigger>
            </TabsList>

            <TabsContent className="mt-5 space-y-5" value="rules">
              <div className="grid gap-5 lg:grid-cols-2">
                <Card><CardHeader><CardTitle>金额范围</CardTitle><CardDescription>用户提交的支付金额必须位于此范围内；金额最多保留 2 位小数。</CardDescription></CardHeader><CardContent className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="payment-min">最低充值</Label><div className="relative"><span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span><Input className="pl-7" id="payment-min" inputMode="decimal" onChange={(event) => setForm({ ...form, min_amount: event.target.value })} title="最低充值金额需为人民币金额，最多保留 2 位小数" value={form.min_amount} /></div></div><div className="space-y-2"><Label htmlFor="payment-max">最高充值</Label><div className="relative"><span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span><Input className="pl-7" id="payment-max" inputMode="decimal" onChange={(event) => setForm({ ...form, max_amount: event.target.value })} title="最高充值金额需为人民币金额，最多保留 2 位小数" value={form.max_amount} /></div></div></CardContent></Card>
                <Card><CardHeader><CardTitle>预设充值金额</CardTitle><CardDescription>这些金额会显示在用户充值弹窗中；必须位于全局金额范围内。</CardDescription></CardHeader><CardContent className="space-y-4"><div className="flex flex-wrap gap-2">{form.preset_amounts_micros.map((amount) => <Badge className="gap-1 py-1 pl-2.5" key={amount} variant="secondary">{formatMoney(amount)}<button aria-label={`删除预设金额 ${formatMoney(amount)}`} className="rounded-sm p-0.5 hover:bg-background" onClick={() => setForm((current) => ({ ...current, preset_amounts_micros: current.preset_amounts_micros.filter((item) => item !== amount) }))} type="button"><XCircle className="size-3.5" /></button></Badge>)}</div><div className="flex gap-2"><Input aria-label="新增预设充值金额" inputMode="decimal" onChange={(event) => setPresetInput(event.target.value)} placeholder="例如 200" title="预设充值金额需为人民币金额，最多保留 2 位小数" value={presetInput} /><Button disabled={!presetInput.trim()} onClick={addPreset} type="button" variant="outline"><Plus />添加</Button></div></CardContent></Card>
              </div>

              <Card className="overflow-hidden"><CardHeader className="flex-row items-start justify-between gap-4"><div><CardTitle>充值赠送</CardTitle><CardDescription>达到门槛后按最高适用档位增加到账余额。</CardDescription></div><Button onClick={() => beginBonus(null)} type="button" variant="outline"><Plus />添加档位</Button></CardHeader><CardContent className="p-0"><Table><TableHeader><TableRow><TableHead>充值门槛</TableHead><TableHead>赠送比例</TableHead><TableHead>示例到账</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{form.bonus_tiers.length === 0 ? <TableRow><TableCell className="h-24 text-center text-muted-foreground" colSpan={4}>暂未设置充值赠送</TableCell></TableRow> : form.bonus_tiers.map((tier, index) => <TableRow key={tier.threshold_micros}><TableCell>{formatMoney(tier.threshold_micros)}</TableCell><TableCell><Badge variant="secondary">+{(tier.bonus_bps / 100).toFixed(2)}%</Badge></TableCell><TableCell>{formatMoney(tier.threshold_micros + tier.threshold_micros * tier.bonus_bps / 10_000)}</TableCell><TableCell><div className="flex justify-end gap-1"><Button aria-label="编辑赠送档位" onClick={() => beginBonus(index)} size="icon-sm" type="button" variant="ghost"><Pencil /></Button><Button aria-label="删除赠送档位" onClick={() => setForm((current) => ({ ...current, bonus_tiers: current.bonus_tiers.filter((_, itemIndex) => itemIndex !== index) }))} size="icon-sm" type="button" variant="ghost"><Trash2 /></Button></div></TableCell></TableRow>)}</TableBody></Table></CardContent></Card>
            </TabsContent>

            <TabsContent className="mt-5 space-y-5" value="epay">
              <Card><CardHeader><CardTitle>网关凭据</CardTitle><CardDescription>商户密钥留空会保留当前已保存的密钥。</CardDescription></CardHeader><CardContent className="space-y-5"><label className="flex cursor-pointer items-start gap-3 rounded-md border p-4"><Checkbox aria-label="启用易支付" checked={form.enabled} disabled={busy || loading} onCheckedChange={(checked) => setForm((current) => ({ ...current, enabled: checked === true }))} /><span><span className="block text-sm font-medium">启用余额充值</span><span className="mt-1 block text-xs text-muted-foreground">关闭后不能创建新订单，已创建订单的支付回调仍会处理。</span></span></label><div className="grid gap-5 md:grid-cols-2"><div className="space-y-2"><Label htmlFor="payment-api-url">易支付接口地址</Label><Input id="payment-api-url" onChange={(event) => setForm({ ...form, api_url: event.target.value })} placeholder="https://pay.example.com" required={form.enabled} value={form.api_url} /></div><div className="space-y-2"><Label htmlFor="payment-site-name">站点名称</Label><Input id="payment-site-name" maxLength={64} onChange={(event) => setForm({ ...form, site_name: event.target.value })} required value={form.site_name} /></div><div className="space-y-2"><Label htmlFor="payment-merchant-id">商户 ID</Label><Input id="payment-merchant-id" maxLength={128} onChange={(event) => setForm({ ...form, merchant_id: event.target.value })} required={form.enabled} value={form.merchant_id} /></div><div className="space-y-2"><Label htmlFor="payment-merchant-key">商户密钥</Label><div className="relative"><KeyRound className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input className="pl-9" id="payment-merchant-key" onChange={(event) => setForm({ ...form, merchant_key: event.target.value })} placeholder={config?.has_merchant_key ? "已保存，留空保持不变" : "请输入商户密钥"} type="password" value={form.merchant_key} /></div></div></div></CardContent></Card>

              <Card><CardHeader><CardTitle>回调地址</CardTitle><CardDescription>将异步通知地址配置到易支付平台；同步返回地址用于支付完成后回到余额页面。</CardDescription></CardHeader><CardContent className="grid gap-4 lg:grid-cols-2"><div className="space-y-2"><Label htmlFor="notify-url">异步通知地址</Label><div className="flex gap-2"><Input id="notify-url" readOnly value={form.notify_url} /><Button aria-label="复制异步通知地址" disabled={!form.notify_url} onClick={() => void copyAddress(form.notify_url, "异步通知地址")} size="icon" type="button" variant="outline"><Clipboard /></Button></div></div><div className="space-y-2"><Label htmlFor="return-url">支付完成返回地址</Label><div className="flex gap-2"><Input id="return-url" readOnly value={form.return_url} /><Button aria-label="复制支付完成返回地址" disabled={!form.return_url} onClick={() => void copyAddress(form.return_url, "支付完成返回地址")} size="icon" type="button" variant="outline"><Clipboard /></Button></div></div></CardContent></Card>

              <Card className="overflow-hidden"><CardHeader className="gap-4"><div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle>支付方式</CardTitle><CardDescription>处理标识会作为易支付的 type 参数提交。</CardDescription></div><div className="flex flex-wrap gap-2"><Button onClick={() => beginMethod(null, { code: "alipay", name: "支付宝", icon: "smartphone" })} type="button" variant="outline">支付宝模板</Button><Button onClick={() => beginMethod(null, { code: "wxpay", name: "微信支付", icon: "qr-code" })} type="button" variant="outline">微信模板</Button><Button onClick={() => beginMethod(null)} type="button"><Plus />添加方式</Button></div></div><div className="relative max-w-md"><Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索支付方式" className="pl-8" onChange={(event) => setMethodQuery(event.target.value)} placeholder="搜索名称或处理标识" value={methodQuery} /></div></CardHeader><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>支付方式</TableHead><TableHead>处理标识</TableHead><TableHead>最低充值</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{filteredMethods.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={5}>还没有支付方式</TableCell></TableRow> : filteredMethods.map(({ method, index }) => <TableRow key={`${method.code}-${index}`}><TableCell><div className="flex items-center gap-3"><span className="flex size-8 items-center justify-center rounded-md border text-muted-foreground [&_svg]:size-4"><MethodIcon icon={method.icon} /></span><span className="font-medium">{method.name}</span></div></TableCell><TableCell><code className="rounded bg-muted px-1.5 py-0.5 text-xs">{method.code}</code></TableCell><TableCell>{formatMoney(method.min_micros)}</TableCell><TableCell><label className="inline-flex cursor-pointer items-center gap-2"><Checkbox aria-label={`${method.enabled ? "停用" : "启用"} ${method.name}`} checked={method.enabled} onCheckedChange={(checked) => setForm((current) => ({ ...current, methods: current.methods.map((item, itemIndex) => itemIndex === index ? { ...item, enabled: checked === true } : item) }))} /><span className="text-sm">{method.enabled ? "启用" : "停用"}</span></label></TableCell><TableCell><div className="flex justify-end gap-1"><Button aria-label={`编辑 ${method.name}`} onClick={() => beginMethod(index)} size="icon-sm" type="button" variant="ghost"><Pencil /></Button><Button aria-label={`删除 ${method.name}`} onClick={() => setDeletingMethod(index)} size="icon-sm" type="button" variant="ghost"><Trash2 /></Button></div></TableCell></TableRow>)}</TableBody></Table></div></CardContent></Card>
            </TabsContent>

            <TabsContent className="mt-5 space-y-4" value="records">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center"><div className="relative flex-1"><Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索充值记录" className="pl-8" onChange={(event) => { setOrderQuery(event.target.value); setOrderOffset(0); }} placeholder="订单号、网关流水号或用户" value={orderQuery} /></div><div className="flex flex-wrap gap-2"><Select onValueChange={(value) => { setOrderStatus(value); setOrderOffset(0); }} value={orderStatus}><SelectTrigger aria-label="筛选订单状态"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="pending">待支付</SelectItem><SelectItem value="paid">已到账</SelectItem></SelectContent></Select><Select onValueChange={(value) => { setOrderChannel(value); setOrderOffset(0); }} value={orderChannel}><SelectTrigger aria-label="筛选支付方式"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部方式</SelectItem>{form.methods.map((method) => <SelectItem key={method.code} value={method.code}>{method.name}</SelectItem>)}</SelectContent></Select><Button aria-label="刷新充值记录" disabled={ordersLoading} onClick={() => void loadOrders()} size="icon" type="button" variant="outline"><RefreshCw className={ordersLoading ? "animate-spin" : ""} /></Button></div></div>
              <Card className="overflow-hidden"><CardContent className="p-0"><div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>用户</TableHead><TableHead>订单号</TableHead><TableHead>支付方式</TableHead><TableHead>支付金额</TableHead><TableHead>到账金额</TableHead><TableHead>状态</TableHead><TableHead>时间</TableHead></TableRow></TableHeader><TableBody>{ordersLoading ? <TableRow><TableCell className="h-28 text-center" colSpan={7}>加载中...</TableCell></TableRow> : null}{!ordersLoading && orders.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={7}>还没有充值记录</TableCell></TableRow> : null}{!ordersLoading ? orders.map((order) => <TableRow key={order.id}><TableCell><p className="font-medium">{order.owner.display_name || order.owner.username}</p><p className="text-xs text-muted-foreground">@{order.owner.username}</p></TableCell><TableCell><p className="font-mono text-xs">{order.out_trade_no}</p>{order.provider_trade_no ? <p className="mt-1 font-mono text-xs text-muted-foreground">{order.provider_trade_no}</p> : null}</TableCell><TableCell>{form.methods.find((method) => method.code === order.channel)?.name ?? order.channel}</TableCell><TableCell>{formatMoney(order.amount_micros)}</TableCell><TableCell className={order.credited_micros > order.amount_micros ? "text-emerald-600 dark:text-emerald-400" : ""}>{formatMoney(order.credited_micros)}</TableCell><TableCell><Badge variant={order.status === "paid" ? "default" : "secondary"}>{order.status === "paid" ? "已到账" : "待支付"}</Badge></TableCell><TableCell className="text-muted-foreground"><p>{formatDate(order.created_at)}</p>{order.paid_at ? <p className="mt-1 text-xs">到账 {formatDate(order.paid_at)}</p> : null}</TableCell></TableRow>) : null}</TableBody></Table></div></CardContent></Card>
              <div className="flex items-center justify-between text-sm text-muted-foreground"><span>共 {orderTotal} 条</span><div className="flex gap-2"><Button disabled={orderOffset === 0 || ordersLoading} onClick={() => setOrderOffset(Math.max(0, orderOffset - orderLimit))} type="button" variant="outline">上一页</Button><Button disabled={orderOffset + orderLimit >= orderTotal || ordersLoading} onClick={() => setOrderOffset(orderOffset + orderLimit)} type="button" variant="outline">下一页</Button></div></div>
            </TabsContent>
          </Tabs>
          <div className="flex shrink-0 gap-2 sm:self-start"><Button aria-label="刷新支付配置" disabled={loading || busy} onClick={() => void load()} size="icon" title="刷新支付配置" type="button" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button><Button disabled={busy || loading} type="submit"><Save />{busy ? "保存中..." : "保存所有设置"}</Button></div>
        </div>
      </form>

      {message ? <p className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</p> : null}
      {error ? <p className="border-y border-destructive/40 bg-destructive/5 px-4 py-3 text-sm text-destructive" role="alert">{error}</p> : null}

      <Dialog onOpenChange={setMethodOpen} open={methodOpen}><DialogContent><DialogHeader><DialogTitle>{methodIndex === null ? "添加支付方式" : "编辑支付方式"}</DialogTitle><DialogDescription>处理标识会作为易支付 type 参数提交；不能使用点号或空格。</DialogDescription></DialogHeader><form className="space-y-4" id="payment-method-form" onSubmit={saveMethod}><div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="method-name">显示名称</Label><Input id="method-name" maxLength={32} onChange={(event) => setMethodForm({ ...methodForm, name: event.target.value })} placeholder="例如 支付宝" required title="显示名称不能为空，最多 32 个字符" value={methodForm.name} /></div><div className="space-y-2"><Label htmlFor="method-code">处理标识</Label><Input id="method-code" maxLength={32} onChange={(event) => setMethodForm({ ...methodForm, code: event.target.value })} pattern="[a-z0-9][a-z0-9_-]{0,31}" placeholder="例如 alipay" required title="处理标识需为 1 到 32 位，只能使用小写字母、数字、下划线和连字符，不能包含点号或空格" value={methodForm.code} /><p className="text-xs text-muted-foreground">1 到 32 位；允许小写字母、数字、下划线和连字符，不允许点号或空格。</p></div><div className="space-y-2"><Label htmlFor="method-icon">图标</Label><Select onValueChange={(value) => setMethodForm({ ...methodForm, icon: value })} value={methodForm.icon}><SelectTrigger className="w-full" id="method-icon"><SelectValue /></SelectTrigger><SelectContent>{iconOptions.map((option) => <SelectItem key={option.value} value={option.value}><MethodIcon icon={option.value} />{option.label}</SelectItem>)}</SelectContent></Select></div><div className="space-y-2"><Label htmlFor="method-min">最低充值</Label><div className="relative"><span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span><Input className="pl-7" id="method-min" inputMode="decimal" onChange={(event) => setMethodForm({ ...methodForm, min_amount: event.target.value })} required title="最低充值金额需为人民币金额，最多保留 2 位小数" value={methodForm.min_amount} /></div><p className="text-xs text-muted-foreground">最多保留 2 位小数，且必须位于全局金额范围内。</p></div></div><label className="flex items-center gap-3 rounded-md border p-3"><Checkbox aria-label="启用支付方式" checked={methodForm.enabled} onCheckedChange={(checked) => setMethodForm({ ...methodForm, enabled: checked === true })} /><span className="text-sm font-medium">启用该支付方式</span></label></form><DialogFooter><Button onClick={() => setMethodOpen(false)} type="button" variant="outline">取消</Button><Button form="payment-method-form" type="submit"><Save />保存方式</Button></DialogFooter></DialogContent></Dialog>

      <Dialog onOpenChange={setBonusOpen} open={bonusOpen}><DialogContent><DialogHeader><DialogTitle>{bonusIndex === null ? "添加赠送档位" : "编辑赠送档位"}</DialogTitle><DialogDescription>用户支付金额不变，赠送部分直接计入到账余额；门槛金额最多保留 2 位小数。</DialogDescription></DialogHeader><form className="grid gap-4 sm:grid-cols-2" id="bonus-tier-form" onSubmit={saveBonus}><div className="space-y-2"><Label htmlFor="bonus-threshold">充值门槛</Label><div className="relative"><span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span><Input className="pl-7" id="bonus-threshold" inputMode="decimal" onChange={(event) => setBonusForm({ ...bonusForm, threshold: event.target.value })} required title="充值门槛需为人民币金额，最多保留 2 位小数" value={bonusForm.threshold} /></div></div><div className="space-y-2"><Label htmlFor="bonus-percent">赠送比例</Label><div className="relative"><Input className="pr-8" id="bonus-percent" inputMode="decimal" max="100" min="0.01" onChange={(event) => setBonusForm({ ...bonusForm, percent: event.target.value })} required title="赠送比例必须大于 0% 且不超过 100%" value={bonusForm.percent} /><span className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">%</span></div></div></form><DialogFooter><Button onClick={() => setBonusOpen(false)} type="button" variant="outline">取消</Button><Button form="bonus-tier-form" type="submit"><Save />保存档位</Button></DialogFooter></DialogContent></Dialog>

      <AlertDialog onOpenChange={(open) => { if (!open) setDeletingMethod(null); }} open={deletingMethod !== null}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除支付方式</AlertDialogTitle><AlertDialogDescription>将删除 {deletingMethod === null ? "该支付方式" : form.methods[deletingMethod]?.name}。历史充值记录仍会保留原处理标识。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction onClick={() => { if (deletingMethod !== null) setForm((current) => ({ ...current, methods: current.methods.filter((_, index) => index !== deletingMethod) })); setDeletingMethod(null); }} variant="destructive">确认删除</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
    </div>
  );
}
