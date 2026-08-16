"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Boxes, Clock3, LayoutGrid, Pencil, Plus, Power, PowerOff, RefreshCw, RotateCcw, Save, Search, Send, Trash2, X } from "lucide-react";
import { useRouter } from "next/navigation";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { BulkActionDialog, ListBulkActions } from "@/components/list-bulk-actions";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { bulkResultMessage, runBulkAction } from "@/lib/bulk-action";
import { useListSelection } from "@/lib/use-list-selection";

type Prices = {
  input_price_micros: number;
  output_price_micros: number;
  cache_read_price_micros: number;
  cache_write_price_micros: number;
  cache_write_1h_price_micros: number;
  request_price_micros: number;
};

export type CatalogModel = {
  id: string;
  provider_name: string;
  upstream_name: string;
  display_name: string;
  prices: Prices;
  pricing_configured: boolean;
  status: "active" | "disabled";
};

type ModelForm = {
  providerName: string;
  upstreamName: string;
  displayName: string;
  input: string;
  cacheRead: string;
  cacheWrite: string;
  cacheWrite1h: string;
  output: string;
  request: string;
};

type RateForm = Omit<ModelForm, "providerName" | "upstreamName" | "displayName">;

type PriceWindow = {
  id: string;
  label: string;
  weekday_mask: number;
  start_minute: number;
  end_minute: number;
  rates: Prices;
};

type PricePlan = {
  id: string;
  version: number;
  mode: "fixed" | "scheduled";
  timezone: string;
  effective_from: string;
  effective_to?: string;
  status: "draft" | "published" | "retired";
  default_rates: Prices;
  windows: PriceWindow[];
};

type PricingWindowForm = {
  label: string;
  weekdayMask: number;
  start: string;
  end: string;
  rates: RateForm;
};

type PricingForm = {
  mode: "fixed" | "scheduled";
  timezone: string;
  defaultRates: RateForm;
  windows: PricingWindowForm[];
};

const EMPTY_FORM: ModelForm = {
  providerName: "",
  upstreamName: "",
  displayName: "",
  input: "",
  cacheRead: "",
  cacheWrite: "",
  cacheWrite1h: "",
  output: "",
  request: "",
};

const EMPTY_RATE_FORM: RateForm = { input: "", cacheRead: "", cacheWrite: "", cacheWrite1h: "", output: "", request: "" };
const EMPTY_PRICING_FORM: PricingForm = { mode: "fixed", timezone: "UTC", defaultRates: EMPTY_RATE_FORM, windows: [] };
const WEEKDAYS = ["日", "一", "二", "三", "四", "五", "六"];

const ALL_PROVIDERS = "__all__";
const preferredProviderOrder = ["deepseek", "glm", "智谱", "kimi", "moonshot"];

/**
 * providerOrder 封装该名称对应的业务处理逻辑。
 * @param name 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function providerOrder(name: string) {
  const normalized = name.toLowerCase();
  const index = preferredProviderOrder.findIndex((keyword) => normalized.includes(keyword));
  return index === -1 ? preferredProviderOrder.length : index;
}

/**
 * money 封装该名称对应的业务处理逻辑。
 * @param micros 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function money(micros: number) {
  return `¥${(micros / 1_000_000).toLocaleString("zh-CN", { maximumFractionDigits: 6 })}`;
}

/**
 * toMicros 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function toMicros(value: string) {
  const parsed = Number(value || "0");
  return Number.isFinite(parsed) && parsed >= 0 ? Math.round(parsed * 1_000_000) : null;
}

/**
 * fromMicros 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function fromMicros(value: number) {
  return String(value / 1_000_000);
}

function rateFormFromPrices(prices: Prices): RateForm {
  return {
    input: fromMicros(prices.input_price_micros),
    cacheRead: fromMicros(prices.cache_read_price_micros),
    cacheWrite: fromMicros(prices.cache_write_price_micros),
    cacheWrite1h: fromMicros(prices.cache_write_1h_price_micros),
    output: fromMicros(prices.output_price_micros),
    request: fromMicros(prices.request_price_micros),
  };
}

function minuteTime(value: number) {
  if (value === 1440) return "00:00";
  const hours = Math.floor(value / 60).toString().padStart(2, "0");
  const minutes = (value % 60).toString().padStart(2, "0");
  return `${hours}:${minutes}`;
}

function parseMinute(value: string) {
  const [hours, minutes] = value.split(":").map(Number);
  return Number.isInteger(hours) && Number.isInteger(minutes) && hours >= 0 && hours <= 23 && minutes >= 0 && minutes <= 59 ? hours * 60 + minutes : null;
}

function ratePayload(rate: RateForm): Prices | null {
  const values = [rate.input, rate.output, rate.cacheRead, rate.cacheWrite, rate.cacheWrite1h, rate.request].map(toMicros);
  if (values.some((value) => value === null)) return null;
  return {
    input_price_micros: values[0] as number,
    output_price_micros: values[1] as number,
    cache_read_price_micros: values[2] as number,
    cache_write_price_micros: values[3] as number,
    cache_write_1h_price_micros: values[4] as number,
    request_price_micros: values[5] as number,
  };
}

function planPayload(form: PricingForm) {
  const defaultRates = ratePayload(form.defaultRates);
  if (!defaultRates) return null;
  if (form.mode === "fixed") {
    return {
      mode: "fixed" as const,
      timezone: "UTC",
      default_rates: defaultRates,
      windows: [],
    };
  }
  const windows = [] as Array<{ label: string; weekday_mask: number; start_minute: number; end_minute: number; rates: Prices }>;
  for (const window of form.windows) {
    const rates = ratePayload(window.rates);
    const start = parseMinute(window.start);
    const parsedEnd = parseMinute(window.end);
    const end = parsedEnd === 0 && start !== null && start > 0 ? 1440 : parsedEnd;
    if (!rates || start === null || end === null) return null;
    windows.push({
      label: window.label,
      weekday_mask: window.weekdayMask,
      start_minute: start,
      end_minute: end,
      rates,
    });
  }
  return {
    mode: form.mode,
    timezone: form.timezone,
    default_rates: defaultRates,
    windows,
  };
}

function pricingFormFromModel(model: CatalogModel): PricingForm {
  return { ...EMPTY_PRICING_FORM, defaultRates: rateFormFromPrices(model.prices) };
}

function pricingFormFromPlan(plan: PricePlan): PricingForm {
  return {
    mode: plan.mode,
    timezone: plan.timezone,
    defaultRates: rateFormFromPrices(plan.default_rates),
    windows: plan.windows.map((window) => ({
      label: window.label,
      weekdayMask: window.weekday_mask,
      start: minuteTime(window.start_minute),
      end: minuteTime(window.end_minute),
      rates: rateFormFromPrices(window.rates),
    })),
  };
}

/**
 * sortPricePlans 按版本号从新到旧整理价格方案，保证发布新版本后历史版本仍稳定展示。
 * @param plans 待整理的价格方案列表。
 * @return 不修改原数组的版本排序结果。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
function sortPricePlans(plans: PricePlan[]) {
  return [...plans].sort((left, right) => right.version - left.version);
}

/**
 * formatPlanDate 展示价格版本的生效时间，并将初始化哨兵时间转换为业务可读文案。
 * @param value 价格版本的 ISO 时间字符串。
 * @return 面向管理员展示的本地化时间文本。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
function formatPlanDate(value: string) {
  const timestamp = new Date(value).getTime();
  if (Number.isNaN(timestamp)) return "时间无效";
  if (timestamp <= new Date("1971-01-01T00:00:00Z").getTime()) return "系统初始化";
  return new Date(value).toLocaleString("zh-CN");
}

/**
 * planStatusLabel 将数据库状态转换为管理员能直接理解的版本状态。
 * @param plan 当前价格版本。
 * @return 版本状态和是否仍是当前生效版本。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
function planStatusLabel(plan: PricePlan) {
  const label = plan.status === "draft" ? "草稿" : plan.status === "retired" ? "已退役" : plan.effective_to ? "历史版本" : "已发布";
  const current = plan.status === "published" && !plan.effective_to;
  return { label, current };
}

/**
 * errorMessage 封装该名称对应的业务处理逻辑。
 * @param response 当前响应数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
async function errorMessage(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

/**
 * payload 封装该名称对应的业务处理逻辑。
 * @param form 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function payload(form: ModelForm) {
  const values = [form.input, form.output, form.cacheRead, form.cacheWrite, form.cacheWrite1h, form.request].map(toMicros);
  if (values.some((value) => value === null)) return null;
  return {
    provider_name: form.providerName,
    upstream_name: form.upstreamName,
    display_name: form.displayName,
    input_price_micros: values[0],
    output_price_micros: values[1],
    cache_read_price_micros: values[2],
    cache_write_price_micros: values[3],
    cache_write_1h_price_micros: values[4],
    request_price_micros: values[5],
  };
}

/**
 * PriceFields 用于计算并返回对应结果。
 * @param form 本次操作需要使用的输入参数。
 * @param setForm 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function PriceFields({ form, setForm }: { form: ModelForm; setForm: (form: ModelForm) => void }) {
  const fields: Array<[keyof ModelForm, string, string]> = [
    ["input", "普通输入", "元 / 1M tokens"],
    ["cacheRead", "缓存命中", "元 / 1M tokens"],
    ["cacheWrite", "缓存创建（5 分钟）", "元 / 1M tokens"],
    ["cacheWrite1h", "缓存创建（1 小时）", "元 / 1M tokens"],
    ["output", "输出", "元 / 1M tokens"],
    ["request", "请求固定费", "元 / 次"],
  ];
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {fields.map(([key, label, suffix]) => (
        <div className="space-y-2" key={key}>
          <Label htmlFor={`catalog-price-${key}`}>{label}</Label>
          <div className="relative">
            <Input
              className="pr-28"
              id={`catalog-price-${key}`}
              inputMode="decimal"
              min="0"
              onChange={(event) => setForm({ ...form, [key]: event.target.value })}
              placeholder="0"
              required
              step="0.000001"
              title={`${label}价格必须是非负数字，可保留到 6 位小数`}
              type="number"
              value={form[key]}
            />
            <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{suffix}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

function PricingRateFields({ rate, setRate, prefix }: { rate: RateForm; setRate: (rate: RateForm) => void; prefix: string }) {
  const fields: Array<[keyof RateForm, string, string]> = [
    ["input", "普通输入", "元 / 1M"],
    ["cacheRead", "缓存命中", "元 / 1M"],
    ["cacheWrite", "缓存创建（5 分钟）", "元 / 1M"],
    ["cacheWrite1h", "缓存创建（1 小时）", "元 / 1M"],
    ["output", "输出", "元 / 1M"],
    ["request", "请求固定费", "元 / 次"],
  ];
  return (
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      {fields.map(([key, label, suffix]) => (
        <div className="flex flex-col gap-2" key={key}>
          <Label htmlFor={`${prefix}-${key}`}>{label}</Label>
          <div className="relative">
            <Input className="pr-20" id={`${prefix}-${key}`} inputMode="decimal" min="0" onChange={(event) => setRate({ ...rate, [key]: event.target.value })} placeholder="0" required step="0.000001" title={`${label}价格必须是非负数字，可保留到 6 位小数`} type="number" value={rate[key]} />
            <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{suffix}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

function WeekdayFields({ mask, onChange, prefix }: { mask: number; onChange: (mask: number) => void; prefix: string }) {
  return (
    <fieldset className="flex flex-wrap gap-x-4 gap-y-2">
      <legend className="mb-2 text-sm font-medium">重复星期</legend>
      {WEEKDAYS.map((day, index) => {
        const id = `${prefix}-${index}`;
        return (
          <label className="flex items-center gap-2 text-sm" htmlFor={id} key={day}>
            <Checkbox checked={(mask & (1 << index)) !== 0} id={id} onCheckedChange={(checked) => onChange(checked === true ? mask | (1 << index) : mask & ~(1 << index))} />
            周{day}
          </label>
        );
      })}
    </fieldset>
  );
}

/**
 * UpstreamModelsClient 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function UpstreamModelsClient() {
  const router = useRouter();
  const [models, setModels] = useState<CatalogModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [query, setQuery] = useState("");
  const [selectedProvider, setSelectedProvider] = useState(ALL_PROVIDERS);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<CatalogModel | null>(null);
  const [statusModel, setStatusModel] = useState<CatalogModel | null>(null);
  const [deletingModel, setDeletingModel] = useState<CatalogModel | null>(null);
  const [form, setForm] = useState<ModelForm>(EMPTY_FORM);
  const [pricingOpen, setPricingOpen] = useState(false);
  const [pricingModel, setPricingModel] = useState<CatalogModel | null>(null);
  const [pricingPlans, setPricingPlans] = useState<PricePlan[]>([]);
  const [pricingDraftID, setPricingDraftID] = useState<string | null>(null);
  const [pricingForm, setPricingForm] = useState<PricingForm>(EMPTY_PRICING_FORM);
  const [pricingBusy, setPricingBusy] = useState(false);
  const [pricingMessage, setPricingMessage] = useState("");
  const [bulkStatus, setBulkStatus] = useState<"active" | "disabled" | null>(null);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    const response = await fetch("/api/admin/upstream-models", { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) {
      setMessage("加载模型目录失败");
      setLoading(false);
      return;
    }
    setModels(((await response.json()) as { upstream_models: CatalogModel[] }).upstream_models);
    setLoading(false);
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const providers = useMemo(() => {
    const counts = new Map<string, number>();
    for (const model of models) counts.set(model.provider_name, (counts.get(model.provider_name) ?? 0) + 1);
    return [...counts.entries()]
      .map(([name, count]) => ({ name, count }))
      .sort((left, right) => providerOrder(left.name) - providerOrder(right.name) || left.name.localeCompare(right.name, "zh-CN"));
  }, [models]);
  const activeProvider = selectedProvider === ALL_PROVIDERS || providers.some((provider) => provider.name === selectedProvider) ? selectedProvider : ALL_PROVIDERS;

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return models.filter((model) => {
      const providerMatches = activeProvider === ALL_PROVIDERS || model.provider_name === activeProvider;
      const queryMatches = !needle || `${model.display_name} ${model.upstream_name} ${model.provider_name}`.toLowerCase().includes(needle);
      return providerMatches && queryMatches;
    });
  }, [activeProvider, models, query]);
  const selection = useListSelection(filtered.map((model) => model.id));

  /**
   * chooseProvider 封装该名称对应的业务处理逻辑。
   * @param provider 本次操作需要使用的输入参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function chooseProvider(provider: string) {
    setSelectedProvider(provider);
    selection.clearSelection();
  }

  /**
   * beginCreate 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function beginCreate() {
    setEditing(null);
    setForm(EMPTY_FORM);
    setEditorOpen(true);
  }

  /**
   * beginEdit 封装该名称对应的业务处理逻辑。
   * @param model 本次操作需要使用的输入参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  function beginEdit(model: CatalogModel) {
    setEditing(model);
    setForm({
      providerName: model.provider_name,
      upstreamName: model.upstream_name,
      displayName: model.display_name,
      input: fromMicros(model.prices.input_price_micros),
      cacheRead: fromMicros(model.prices.cache_read_price_micros),
      cacheWrite: fromMicros(model.prices.cache_write_price_micros),
      cacheWrite1h: fromMicros(model.prices.cache_write_1h_price_micros),
      output: fromMicros(model.prices.output_price_micros),
      request: fromMicros(model.prices.request_price_micros),
    });
    setEditorOpen(true);
  }

  async function beginPricing(model: CatalogModel) {
    setPricingModel(model);
    setPricingPlans([]);
    setPricingDraftID(null);
    setPricingForm(pricingFormFromModel(model));
    setPricingMessage("");
    setPricingBusy(true);
    setPricingOpen(true);
    const response = await fetch(`/api/admin/upstream-models/${model.id}/price-plans`, { cache: "no-store" });
    setPricingBusy(false);
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) { setPricingMessage("加载价格方案失败"); return; }
    const plans = sortPricePlans(((await response.json()) as { price_plans: PricePlan[] }).price_plans);
    const draft = plans.find((plan) => plan.status === "draft");
    const source = draft ?? plans[0];
    setPricingPlans(plans);
    setPricingDraftID(draft?.id ?? null);
    if (draft) setPricingForm(pricingFormFromPlan(draft));
    else if (source) setPricingForm(pricingFormFromPlan(source));
  }

  function addPricingWindow() {
    setPricingForm((current) => ({
      ...current,
      mode: "scheduled",
      timezone: current.timezone === "UTC" ? "Asia/Shanghai" : current.timezone,
      windows: [...current.windows, { label: `高峰时段 ${current.windows.length + 1}`, weekdayMask: 127, start: "09:00", end: "12:00", rates: { ...current.defaultRates } }],
    }));
  }

  function removePricingWindow(index: number) {
    setPricingForm((current) => ({ ...current, windows: current.windows.filter((_, itemIndex) => itemIndex !== index) }));
  }

  async function submitPricing(publish: boolean) {
    if (!pricingModel) return;
    const body = planPayload(pricingForm);
    if (!body) { setPricingMessage("请检查价格和时段填写"); return; }
    if (pricingForm.mode === "scheduled" && pricingForm.windows.length === 0) { setPricingMessage("分时价格至少需要一个特殊时段"); return; }
    setPricingBusy(true);
    const endpoint = pricingDraftID ? `/api/admin/upstream-models/${pricingModel.id}/price-plans/${pricingDraftID}` : `/api/admin/upstream-models/${pricingModel.id}/price-plans`;
    const response = await fetch(endpoint, { method: pricingDraftID ? "PATCH" : "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (!response.ok) { setPricingBusy(false); setPricingMessage(await errorMessage(response)); return; }
    const created = ((await response.json()) as { price_plan: PricePlan }).price_plan;
    setPricingDraftID(created.id);
    if (publish) {
      const published = await fetch(`/api/admin/upstream-models/${pricingModel.id}/price-plans/${created.id}/publish`, { method: "POST" });
      if (!published.ok) { setPricingBusy(false); setPricingMessage(await errorMessage(published)); return; }
      setPricingMessage("价格方案已发布，目录模型和可用关联路由已启用");
      setPricingDraftID(null);
    } else {
      setPricingMessage("价格方案草稿已保存");
    }
    const refreshed = await fetch(`/api/admin/upstream-models/${pricingModel.id}/price-plans`, { cache: "no-store" });
    if (refreshed.ok) {
      const plans = sortPricePlans(((await refreshed.json()) as { price_plans: PricePlan[] }).price_plans);
      setPricingPlans(plans);
      if (publish) {
        const publishedPlan = plans.find((plan) => plan.id === created.id);
        if (publishedPlan) setPricingForm(pricingFormFromPlan(publishedPlan));
        await load();
      }
    }
    setPricingBusy(false);
  }

  /**
   * republishPricingPlan 切换历史价格版本的生效区间，不创建新的价格版本。
   * @param plan 要切换到的历史价格版本。
   * @return none 无返回值。
   * @author Gao Hongshun
   * @date 2026-08-14
   */
  async function republishPricingPlan(plan: PricePlan) {
    if (!pricingModel || pricingBusy) return;
    setPricingBusy(true);
    setPricingMessage(`正在切换到版本 ${plan.version}...`);
    const response = await fetch(`/api/admin/upstream-models/${pricingModel.id}/price-plans/${plan.id}/republish`, { method: "POST" });
    if (!response.ok) {
      setPricingBusy(false);
      setPricingMessage(await errorMessage(response));
      return;
    }
    await response.json();
    const refreshed = await fetch(`/api/admin/upstream-models/${pricingModel.id}/price-plans`, { cache: "no-store" });
    if (refreshed.ok) {
      const plans = sortPricePlans(((await refreshed.json()) as { price_plans: PricePlan[] }).price_plans);
      setPricingPlans(plans);
      const current = plans.find((item) => item.status === "published" && !item.effective_to);
      if (current) {
        setPricingForm(pricingFormFromPlan(current));
        setPricingDraftID(null);
      }
    }
    setPricingMessage(`已切换到版本 ${plan.version}，未生成新版本`);
    await load();
    setPricingBusy(false);
  }

  /**
   * submit 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const body = editing ? { provider_name: form.providerName, upstream_name: form.upstreamName, display_name: form.displayName } : payload(form);
    if (!body) { setMessage("价格必须是非负数字，可保留到 6 位小数"); return; }
    setBusy(true);
    const response = await fetch(editing ? `/api/admin/upstream-models/${editing.id}` : "/api/admin/upstream-models", {
      method: editing ? "PATCH" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    setBusy(false);
    if (!response.ok) { setMessage(await errorMessage(response)); return; }
    setEditorOpen(false);
    setEditing(null);
    setForm(EMPTY_FORM);
    setMessage(editing ? "目录模型已更新" : "目录模型已创建");
    await load();
  }

  /**
   * toggleStatus 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function toggleStatus() {
    if (!statusModel) return;
    setBusy(true);
    const next = statusModel.status === "active" ? "disabled" : "active";
    const response = await fetch(`/api/admin/upstream-models/${statusModel.id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: next }),
    });
    setBusy(false);
    if (!response.ok) {
      setMessage(await errorMessage(response));
    } else {
      setMessage(next === "active" ? "目录模型已启用" : "目录模型已停用");
      await load();
    }
    setStatusModel(null);
  }

  /**
   * applyBulkStatus 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function applyBulkStatus() {
    if (!bulkStatus) return;
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/upstream-models/${id}/status`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: bulkStatus }),
    }));
    setBusy(false);
    setBulkStatus(null);
    setMessage(bulkResultMessage(bulkStatus === "active" ? "启用目录模型" : "停用目录模型", result));
    await load();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  /**
   * deleteOneModel 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function deleteOneModel() {
    if (!deletingModel) return;
    setBusy(true);
    const response = await fetch(`/api/admin/upstream-models/${deletingModel.id}`, { method: "DELETE" });
    setBusy(false);
    if (!response.ok) setMessage(await errorMessage(response));
    else {
      setMessage("目录模型已删除，关联路由已一并移出列表");
      await load();
    }
    setDeletingModel(null);
  }

  /**
   * deleteSelected 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function deleteSelected() {
    setBusy(true);
    const result = await runBulkAction(selection.selectedIds, (id) => fetch(`/api/admin/upstream-models/${id}`, { method: "DELETE" }));
    setBusy(false);
    setBulkDeleteOpen(false);
    setMessage(bulkResultMessage("删除目录模型", result));
    await load();
    selection.replaceSelection(result.failed.map((failure) => failure.id));
  }

  return (
    <>
      <div className="grid min-w-0 gap-5 lg:grid-cols-[200px_minmax(0,1fr)]">
      <aside className="min-w-0 border-b pb-4 lg:border-b-0 lg:border-r lg:pb-0 lg:pr-4">
        <nav aria-label="按提供商筛选模型" className="flex gap-1 overflow-x-auto pb-1 lg:sticky lg:top-24 lg:block lg:space-y-1 lg:overflow-visible lg:pb-0">
          <Button aria-pressed={activeProvider === ALL_PROVIDERS} className="h-9 shrink-0 justify-between gap-4 lg:w-full" onClick={() => chooseProvider(ALL_PROVIDERS)} variant={activeProvider === ALL_PROVIDERS ? "secondary" : "ghost"}><span className="flex items-center gap-2"><LayoutGrid />全部模型</span><span className="text-xs tabular-nums text-muted-foreground">{models.length}</span></Button>
          {providers.map((provider) => <Button aria-pressed={activeProvider === provider.name} className="h-9 shrink-0 justify-between gap-4 lg:w-full" key={provider.name} onClick={() => chooseProvider(provider.name)} title={provider.name} variant={activeProvider === provider.name ? "secondary" : "ghost"}><span className="max-w-28 truncate">{provider.name}</span><span className="text-xs tabular-nums text-muted-foreground">{provider.count}</span></Button>)}
        </nav>
      </aside>

      <div className="min-w-0 space-y-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="relative max-w-lg flex-1">
            <Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input aria-label="搜索模型目录" className="pl-8" onChange={(event) => { setQuery(event.target.value); selection.clearSelection(); }} placeholder={activeProvider === ALL_PROVIDERS ? "搜索模型名称或模型 ID" : `在 ${activeProvider} 标签中搜索模型`} value={query} />
          </div>
          <div className="flex gap-2">
            <Button aria-label="刷新模型目录" disabled={loading} onClick={() => void load()} size="icon" title="刷新模型目录" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
            <Button onClick={beginCreate}><Plus />新增目录模型</Button>
          </div>
        </div>

        <div className="flex min-h-5 items-center justify-between gap-3 text-xs text-muted-foreground"><span>{activeProvider === ALL_PROVIDERS ? "全部提供商" : activeProvider}</span><span>{filtered.length} 个结果</span></div>

        {message ? <p className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</p> : null}

        <ListBulkActions onClear={selection.clearSelection} selectedCount={selection.selectedIds.length}>
          <Button disabled={busy} onClick={() => setBulkStatus("active")} size="sm" type="button" variant="outline"><Power />批量启用</Button>
          <Button disabled={busy} onClick={() => setBulkStatus("disabled")} size="sm" type="button" variant="destructive"><PowerOff />批量停用</Button>
          <Button disabled={busy} onClick={() => setBulkDeleteOpen(true)} size="sm" type="button" variant="destructive"><Trash2 />批量删除</Button>
        </ListBulkActions>

        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader><TableRow><TableHead className="w-10"><Checkbox aria-label="选择所有目录模型" checked={selection.checkboxState} disabled={loading || filtered.length === 0} onCheckedChange={(checked) => selection.toggleAll(checked === true)} /></TableHead><TableHead>模型与统一价格</TableHead><TableHead>普通输入</TableHead><TableHead>缓存命中</TableHead><TableHead>缓存创建</TableHead><TableHead>输出</TableHead><TableHead>请求费</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>
                  {loading ? <TableRow><TableCell className="h-28 text-center" colSpan={9}>加载中...</TableCell></TableRow> : null}
                  {!loading && filtered.length === 0 ? <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={9}>{query.trim() ? "没有匹配的目录模型" : "该提供商暂无目录模型"}</TableCell></TableRow> : null}
                  {filtered.map((model) => (
                    <TableRow key={model.id}>
                      <TableCell><Checkbox aria-label={`选择 ${model.display_name}`} checked={selection.isSelected(model.id)} onCheckedChange={(checked) => selection.toggleOne(model.id, checked === true)} /></TableCell>
                      <TableCell><p className="font-medium">{model.display_name}</p><p className="text-xs text-muted-foreground">{model.provider_name} · <span className="font-mono">{model.upstream_name}</span></p></TableCell>
                      <TableCell>{money(model.prices.input_price_micros)}</TableCell>
                      <TableCell>{money(model.prices.cache_read_price_micros)}</TableCell>
                      <TableCell><p>{money(model.prices.cache_write_price_micros)}</p><p className="text-xs text-muted-foreground">1h {money(model.prices.cache_write_1h_price_micros)}</p></TableCell>
                      <TableCell>{money(model.prices.output_price_micros)}</TableCell>
                      <TableCell>{money(model.prices.request_price_micros)}</TableCell>
                      <TableCell>{!model.pricing_configured ? <Badge variant="destructive">待定价</Badge> : <Badge variant={model.status === "active" ? "outline" : "secondary"}>{model.status === "active" ? "启用" : "停用"}</Badge>}</TableCell>
                      <TableCell><div className="flex justify-end gap-1"><Button aria-label={`配置 ${model.display_name} 的价格方案`} onClick={() => void beginPricing(model)} size="icon-sm" title="价格方案" variant="ghost"><Clock3 /></Button><Button aria-label={`编辑 ${model.display_name}`} onClick={() => beginEdit(model)} size="icon-sm" title="编辑" variant="ghost"><Pencil /></Button><Button aria-label={`${model.status === "active" ? "停用" : "启用"} ${model.display_name}`} onClick={() => setStatusModel(model)} size="icon-sm" title={model.status === "active" ? "停用" : "启用"} variant="ghost">{model.status === "active" ? <PowerOff /> : <Power />}</Button><Button aria-label={`删除 ${model.display_name}`} onClick={() => setDeletingModel(model)} size="icon-sm" title="删除目录模型" variant="ghost"><Trash2 /></Button></div></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      </div>
      </div>

      {editing ? <Dialog onOpenChange={(open) => { setEditorOpen(open); if (!open) { setEditing(null); setForm(EMPTY_FORM); } }} open={editorOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
          <DialogHeader><DialogTitle>编辑目录模型</DialogTitle><DialogDescription>模型 ID 全局唯一，厂商标签仅用于分类，不绑定具体提供商配置。</DialogDescription></DialogHeader>
          <form className="flex flex-col gap-5" id="catalog-model-form" onSubmit={submit}>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="catalog-provider-name">厂商标签</Label><Input id="catalog-provider-name" maxLength={128} onChange={(event) => setForm({ ...form, providerName: event.target.value })} placeholder="例如 DeepSeek" required title="厂商标签不能为空，最长 128 个字符" value={form.providerName} /></div>
              <div className="space-y-2"><Label htmlFor="catalog-upstream-name">模型 ID</Label><Input id="catalog-upstream-name" maxLength={256} onChange={(event) => setForm({ ...form, upstreamName: event.target.value })} placeholder="例如 deepseek-chat" required title="模型 ID 不能为空，最长 256 个字符；需要与客户端请求或上游返回的模型 ID 保持一致" value={form.upstreamName} /><p className="text-xs text-muted-foreground">填写客户端请求使用的模型 ID 或上游同步返回的模型 ID，最长 256 个字符。</p></div>
            </div>
            <div className="space-y-2"><Label htmlFor="catalog-display-name">显示名称</Label><Input id="catalog-display-name" maxLength={128} onChange={(event) => setForm({ ...form, displayName: event.target.value })} placeholder="例如 DeepSeek Chat" required value={form.displayName} /></div>
            <p className="border-y py-3 text-sm text-muted-foreground">价格通过模型列表中的价格方案入口单独维护。</p>
          </form>
          <DialogFooter><Button disabled={busy} form="catalog-model-form" type="submit"><Boxes />{busy ? "正在保存..." : "保存目录模型"}</Button></DialogFooter>
        </DialogContent>
      </Dialog> : <Sheet onOpenChange={(open) => { setEditorOpen(open); if (!open) setForm(EMPTY_FORM); }} open={editorOpen}>
        <SheetContent className="grid grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden data-[side=right]:w-full! sm:data-[side=right]:w-[min(48rem,calc(100vw-2rem))]! sm:data-[side=right]:max-w-none!" side="right">
          <SheetHeader className="border-b px-6 py-5">
            <SheetTitle>新增目录模型</SheetTitle>
            <SheetDescription>模型 ID 全局唯一，所有提供商关联同一份目录价格；厂商标签仅用于分类，不绑定具体提供商配置。</SheetDescription>
          </SheetHeader>
          <form className="flex flex-col gap-5 overflow-y-auto px-6 py-5" id="catalog-model-form" onSubmit={submit}>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="catalog-provider-name">厂商标签</Label><Input id="catalog-provider-name" maxLength={128} onChange={(event) => setForm({ ...form, providerName: event.target.value })} placeholder="例如 DeepSeek" required title="厂商标签不能为空，最长 128 个字符" value={form.providerName} /></div>
              <div className="space-y-2"><Label htmlFor="catalog-upstream-name">模型 ID</Label><Input id="catalog-upstream-name" maxLength={256} onChange={(event) => setForm({ ...form, upstreamName: event.target.value })} placeholder="例如 deepseek-chat" required title="模型 ID 不能为空，最长 256 个字符；需要与客户端请求或上游返回的模型 ID 保持一致" value={form.upstreamName} /><p className="text-xs text-muted-foreground">填写客户端请求使用的模型 ID 或上游同步返回的模型 ID，最长 256 个字符。</p></div>
            </div>
            <div className="space-y-2"><Label htmlFor="catalog-display-name">显示名称</Label><Input id="catalog-display-name" maxLength={128} onChange={(event) => setForm({ ...form, displayName: event.target.value })} placeholder="例如 DeepSeek Chat" required value={form.displayName} /></div>
            <PriceFields form={form} setForm={setForm} />
          </form>
          <SheetFooter className="border-t px-6 py-4"><Button disabled={busy} form="catalog-model-form" type="submit"><Boxes />{busy ? "正在保存..." : "保存目录模型"}</Button></SheetFooter>
        </SheetContent>
      </Sheet>}

      <Sheet onOpenChange={(open) => { setPricingOpen(open); if (!open) { setPricingModel(null); setPricingPlans([]); setPricingDraftID(null); setPricingForm(EMPTY_PRICING_FORM); setPricingMessage(""); } }} open={pricingOpen}>
        <SheetContent className="w-full! overflow-y-auto data-[side=right]:w-full! sm:data-[side=right]:w-[min(64rem,calc(100vw-2rem))]! sm:data-[side=right]:max-w-none!" side="right">
          <SheetHeader className="border-b px-6 py-5">
            <SheetTitle>{pricingModel ? `${pricingModel.display_name} 价格方案` : "价格方案"}</SheetTitle>
            <SheetDescription>这里配置 Novro 对客户结算使用的价格。发布后会启用目录模型和可用关联路由，已发布版本不可编辑。</SheetDescription>
          </SheetHeader>
          <form className="flex flex-col gap-6 px-6" id="model-pricing-form" onSubmit={(event) => { event.preventDefault(); void submitPricing(false); }}>
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
              <div className="flex flex-col gap-2">
                <Label htmlFor="pricing-mode">计价方式</Label>
                <Select value={pricingForm.mode} onValueChange={(value) => setPricingForm((current) => ({
                  ...current,
                  mode: value as PricingForm["mode"],
                  timezone: value === "fixed" ? "UTC" : current.timezone === "UTC" ? "Asia/Shanghai" : current.timezone,
                  windows: value === "fixed" ? [] : current.windows,
                }))}>
                  <SelectTrigger id="pricing-mode"><SelectValue /></SelectTrigger>
                  <SelectContent><SelectItem value="fixed">固定价格</SelectItem><SelectItem value="scheduled">分时价格</SelectItem></SelectContent>
                </Select>
              </div>
              {pricingForm.mode === "fixed" ? <div className="self-end sm:pb-2 xl:col-span-3"><Badge variant="outline">发布即生效</Badge></div> : (
                <>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="pricing-timezone">计价时区</Label>
                    <Input id="pricing-timezone" list="pricing-timezones" maxLength={64} onChange={(event) => setPricingForm((current) => ({ ...current, timezone: event.target.value }))} required value={pricingForm.timezone} />
                    <datalist id="pricing-timezones"><option value="Asia/Shanghai" /><option value="UTC" /><option value="America/Los_Angeles" /><option value="Europe/London" /></datalist>
                  </div>
                  <div className="self-end sm:pb-2 xl:col-span-2"><Badge variant="outline">发布即生效</Badge></div>
                </>
              )}
            </div>

            <section className="flex flex-col gap-4 border-t pt-5" aria-labelledby="default-pricing-title">
              <div><h3 className="text-sm font-semibold" id="default-pricing-title">默认价格</h3><p className="mt-1 text-xs text-muted-foreground">固定计价直接使用此价格；分时计价在未命中特殊时段时使用此价格。</p></div>
              <PricingRateFields prefix="pricing-default" rate={pricingForm.defaultRates} setRate={(defaultRates) => setPricingForm((current) => ({ ...current, defaultRates }))} />
            </section>

            {pricingForm.mode === "scheduled" ? (
              <section className="flex flex-col gap-4 border-t pt-5" aria-labelledby="pricing-windows-title">
                <div className="flex items-center justify-between gap-3"><h3 className="text-sm font-semibold" id="pricing-windows-title">特殊时段</h3><Button onClick={addPricingWindow} size="sm" type="button" variant="outline"><Plus />添加时段</Button></div>
                {pricingForm.windows.map((window, index) => (
                  <fieldset className="flex flex-col gap-4 rounded-md border p-4" key={`pricing-window-${index}`}>
                    <legend className="sr-only">特殊时段 {index + 1}</legend>
                    <div className="flex items-end gap-3">
                      <div className="flex flex-1 flex-col gap-2"><Label htmlFor={`pricing-window-${index}-label`}>时段名称</Label><Input id={`pricing-window-${index}-label`} maxLength={64} onChange={(event) => setPricingForm((current) => ({ ...current, windows: current.windows.map((item, itemIndex) => itemIndex === index ? { ...item, label: event.target.value } : item) }))} required value={window.label} /></div>
                      <Button aria-label={`删除特殊时段 ${index + 1}`} onClick={() => removePricingWindow(index)} size="icon" title="删除时段" type="button" variant="ghost"><X /></Button>
                    </div>
                    <WeekdayFields mask={window.weekdayMask} onChange={(weekdayMask) => setPricingForm((current) => ({ ...current, windows: current.windows.map((item, itemIndex) => itemIndex === index ? { ...item, weekdayMask } : item) }))} prefix={`pricing-window-${index}-weekday`} />
                    <div className="grid gap-4 sm:grid-cols-2">
                      <div className="flex flex-col gap-2"><Label htmlFor={`pricing-window-${index}-start`}>开始时间</Label><Input id={`pricing-window-${index}-start`} onChange={(event) => setPricingForm((current) => ({ ...current, windows: current.windows.map((item, itemIndex) => itemIndex === index ? { ...item, start: event.target.value } : item) }))} required type="time" value={window.start} /></div>
                      <div className="flex flex-col gap-2"><Label htmlFor={`pricing-window-${index}-end`}>结束时间</Label><Input id={`pricing-window-${index}-end`} onChange={(event) => setPricingForm((current) => ({ ...current, windows: current.windows.map((item, itemIndex) => itemIndex === index ? { ...item, end: event.target.value } : item) }))} required type="time" value={window.end} /></div>
                    </div>
                    <PricingRateFields prefix={`pricing-window-${index}-rate`} rate={window.rates} setRate={(rates) => setPricingForm((current) => ({ ...current, windows: current.windows.map((item, itemIndex) => itemIndex === index ? { ...item, rates } : item) }))} />
                  </fieldset>
                ))}
                {pricingForm.windows.length === 0 ? <p className="border-y py-4 text-sm text-muted-foreground">尚未添加特殊时段。</p> : null}
              </section>
            ) : null}

            <section className="flex flex-col gap-3 border-t pt-5" aria-labelledby="pricing-history-title">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h3 className="text-sm font-semibold" id="pricing-history-title">版本记录</h3>
                {pricingPlans.length > 0 ? <span className="text-xs text-muted-foreground">共 {pricingPlans.length} 个版本，历史版本不会删除</span> : null}
              </div>
              <p className="text-xs text-muted-foreground">切换历史版本会直接调整已有版本的生效区间，不会生成新版本。</p>
              {pricingPlans.length === 0 ? <p className="text-sm text-muted-foreground">{pricingBusy ? "正在加载..." : "暂无价格版本"}</p> : (
                <div className="max-h-80 divide-y overflow-y-auto rounded-md border">
                  {pricingPlans.map((plan) => {
                    const status = planStatusLabel(plan);
                    return <article className="flex flex-col gap-2 px-4 py-3 text-sm" data-testid={`pricing-version-${plan.version}`} key={plan.id}>
                      <div className="flex flex-wrap items-start justify-between gap-2">
                        <div>
                          <p className="font-medium">版本 {plan.version} · {plan.mode === "fixed" ? "固定价格" : "分时价格"}</p>
                          <p className="mt-1 text-xs text-muted-foreground">{plan.status === "draft" ? "待发布" : `发布：${formatPlanDate(plan.effective_from)}${plan.effective_to ? ` · 替换：${formatPlanDate(plan.effective_to)}` : ""}`}</p>
                        </div>
                        <div className="flex items-center gap-2">
                          <Badge variant={plan.status === "draft" ? "secondary" : "outline"}>{status.label}</Badge>
                          {status.current ? <Badge>当前生效</Badge> : null}
                          {plan.status === "published" && plan.effective_to ? <Button aria-label={`切换并发布版本 ${plan.version}`} data-icon="inline-start" disabled={pricingBusy} onClick={() => void republishPricingPlan(plan)} size="sm" title="直接调整此版本的生效区间，不创建新版本" type="button" variant="outline"><RotateCcw />切换并发布</Button> : null}
                        </div>
                      </div>
                      <p className="text-xs text-muted-foreground">默认输入 {money(plan.default_rates.input_price_micros)} · 输出 {money(plan.default_rates.output_price_micros)}{plan.mode === "scheduled" ? ` · 特殊时段 ${plan.windows.length} 个` : ""}</p>
                    </article>;
                  })}
                </div>
              )}
            </section>
            {pricingMessage ? <p className="border-y bg-background px-4 py-3 text-sm" role="status">{pricingMessage}</p> : null}
          </form>
          <SheetFooter className="border-t px-6">
            <Button disabled={pricingBusy || !pricingModel} form="model-pricing-form" type="submit" variant="outline"><Save />{pricingBusy ? "正在保存..." : "保存草稿"}</Button>
            <Button disabled={pricingBusy || !pricingModel} onClick={() => void submitPricing(true)} type="button"><Send />保存并发布</Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <AlertDialog onOpenChange={(open) => { if (!open) setStatusModel(null); }} open={statusModel !== null}>
        <AlertDialogContent>
          <AlertDialogHeader><AlertDialogTitle>{statusModel?.status === "active" ? "停用目录模型" : "启用目录模型"}</AlertDialogTitle><AlertDialogDescription>{statusModel?.status === "active" ? "停用后，所有关联该目录模型的路由都会停止转发。" : "只有已完成定价的目录模型可以启用。"}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void toggleStatus(); }}>{statusModel?.status === "active" ? "确认停用" : "确认启用"}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog onOpenChange={(open) => { if (!open) setDeletingModel(null); }} open={deletingModel !== null}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除目录模型</AlertDialogTitle><AlertDialogDescription>将删除 {deletingModel?.display_name ?? "该目录模型"} 并同时删除关联路由。历史调用和计费记录会继续保留，此操作不提供恢复入口。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void deleteOneModel(); }} variant="destructive">确认删除</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
      <BulkActionDialog busy={busy} confirmLabel={bulkStatus === "active" ? "确认批量启用" : "确认批量停用"} description={bulkStatus === "active" ? `将启用选中的 ${selection.selectedIds.length} 个目录模型。尚未完成定价的模型会由服务端逐项拒绝，并继续保留在失败选择中。` : `将停用选中的 ${selection.selectedIds.length} 个目录模型，关联路由也会停止转发。`} destructive={bulkStatus === "disabled"} onConfirm={applyBulkStatus} onOpenChange={(open) => { if (!open) setBulkStatus(null); }} open={bulkStatus !== null} title={bulkStatus === "active" ? "批量启用目录模型" : "批量停用目录模型"} />
      <BulkActionDialog busy={busy} confirmLabel="确认批量删除" description={`将删除选中的 ${selection.selectedIds.length} 个目录模型及其关联路由。历史调用和计费记录会继续保留，失败项目仍会保持选中。`} destructive onConfirm={deleteSelected} onOpenChange={setBulkDeleteOpen} open={bulkDeleteOpen} title="批量删除目录模型" />
    </>
  );
}
