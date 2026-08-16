"use client";

import { Check, Clipboard, Factory, RefreshCw, Search } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { copyText } from "@/lib/clipboard";

type Prices = {
  input_price_micros: number;
  output_price_micros: number;
  cache_read_price_micros: number;
  cache_write_price_micros: number;
  cache_write_1h_price_micros: number;
  request_price_micros: number;
};

type AvailableModel = {
  id: string;
  display_name: string;
  provider_name: string;
  protocol: "openai" | "anthropic";
  channel_count: number;
  prices: Prices;
};

type BillingGroup = { id: string; code: string; display_name: string; multiplier_bps: number; effective_multiplier_bps?: number; is_default?: boolean; status?: "active" | "disabled" };

type ErrorResponse = { error?: { message?: string } };

/**
 * formatPrice 封装该名称对应的业务处理逻辑。
 * @param micros 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatPrice(micros: number) {
  return new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "CNY",
    minimumFractionDigits: 0,
    maximumFractionDigits: 6,
  }).format(micros / 1_000_000);
}

/**
 * readError 封装该名称对应的业务处理逻辑。
 * @param response 当前响应数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
async function readError(response: Response) {
  /**
   * body 封装该名称对应的业务处理逻辑。
   * @param await 本次操作需要使用的输入参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  const body = (await response.json().catch(() => ({}))) as ErrorResponse;
  return body.error?.message ?? "可用模型加载失败，请稍后重试";
}

const priceFields: Array<{ key: keyof Prices; label: string; unit: string }> = [
  { key: "input_price_micros", label: "普通输入", unit: "/ 1M tokens" },
  { key: "cache_read_price_micros", label: "缓存命中", unit: "/ 1M tokens" },
  { key: "cache_write_price_micros", label: "缓存创建", unit: "/ 1M tokens" },
  { key: "cache_write_1h_price_micros", label: "缓存创建 1h", unit: "/ 1M tokens" },
  { key: "output_price_micros", label: "输出", unit: "/ 1M tokens" },
  { key: "request_price_micros", label: "请求固定费", unit: "/ 次" },
];

/**
 * AvailableModelsClient 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function AvailableModelsClient() {
  const router = useRouter();
  const [models, setModels] = useState<AvailableModel[]>([]);
  const [billingGroups, setBillingGroups] = useState<BillingGroup[]>([]);
  const [selectedBillingGroupID, setSelectedBillingGroupID] = useState("");
  const [billingGroup, setBillingGroup] = useState<BillingGroup | null>(null);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [query, setQuery] = useState("");
  const [protocol, setProtocol] = useState("all");
  const [provider, setProvider] = useState("all");
  const [copiedID, setCopiedID] = useState("");

  const load = useCallback(async (requestedBillingGroupID?: string) => {
    setLoading(true);
    setMessage("");
    const groupsResponse = await fetch("/api/account/billing-groups", { cache: "no-store" });
    if (groupsResponse.status === 401) { router.replace("/login"); return; }
    if (!groupsResponse.ok) {
      setMessage(await readError(groupsResponse));
      setLoading(false);
      return;
    }
    /**
     * groups 封装该名称对应的业务处理逻辑。
     * @param none 无参数。
     * @author Gao Hongshun
     * @date 2026-08-13
     */
    const groups = ((await groupsResponse.json()) as { billing_groups: BillingGroup[] }).billing_groups;
    setBillingGroups(groups);
    const nextBillingGroupID = requestedBillingGroupID ?? groups.find((group) => group.is_default)?.id ?? groups[0]?.id ?? "";
    if (!nextBillingGroupID) {
      setModels([]);
      setBillingGroup(null);
      setSelectedBillingGroupID("");
      setMessage("当前没有可用计费分组");
      setLoading(false);
      return;
    }
    setSelectedBillingGroupID(nextBillingGroupID);
    const query = new URLSearchParams({ billing_group_id: nextBillingGroupID });
    const response = await fetch(`/api/account/models?${query}`, { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (!response.ok) {
      setMessage(await readError(response));
      setLoading(false);
      return;
    }
    /**
     * body 封装该名称对应的业务处理逻辑。
     * @param await 本次操作需要使用的输入参数。
     * @author Gao Hongshun
     * @date 2026-08-13
     */
    const body = (await response.json()) as { models: AvailableModel[]; billing_group: BillingGroup };
    setModels(body.models);
    setBillingGroup(body.billing_group);
    setProvider("all");
    setLoading(false);
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const providers = useMemo(() => {
    const counts = new Map<string, number>();
    for (const model of models) counts.set(model.provider_name, (counts.get(model.provider_name) ?? 0) + 1);
    return Array.from(counts, ([name, count]) => ({ name, count })).sort((first, second) => first.name.localeCompare(second.name, "zh-CN"));
  }, [models]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return models.filter((model) => {
      const matchesProtocol = protocol === "all" || model.protocol === protocol;
      const matchesProvider = provider === "all" || model.provider_name === provider;
      const matchesQuery = !needle || `${model.id} ${model.display_name} ${model.provider_name}`.toLowerCase().includes(needle);
      return matchesProtocol && matchesProvider && matchesQuery;
    });
  }, [models, protocol, provider, query]);

  /**
   * copyModelID 封装该名称对应的业务处理逻辑。
   * @param id 目标资源的唯一标识。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function copyModelID(id: string) {
    const success = await copyText(id);
    if (success) {
      setCopiedID(id);
      setMessage(`已复制模型 ID：${id}`);
      window.setTimeout(() => setCopiedID((current) => current === id ? "" : current), 1600);
    } else {
      setMessage("复制失败，请手动选择模型 ID");
    }
  }

  const billingGroupName = billingGroup?.display_name ?? "--";
  const multiplier = billingGroup ? `${((billingGroup.effective_multiplier_bps ?? billingGroup.multiplier_bps) / 10_000).toFixed(2)}x` : "--";

  return (
    <div className="space-y-5">
      <section className="grid border-y bg-background sm:grid-cols-3">
        <div className="px-4 py-4 sm:border-r"><p className="text-xs text-muted-foreground">可用模型</p><p className="mt-1 text-xl font-semibold">{loading ? "--" : models.length}</p></div>
        <div className="border-t px-4 py-4 sm:border-r sm:border-t-0"><p className="text-xs text-muted-foreground">计费分组</p><p className="mt-1 truncate text-xl font-semibold">{billingGroupName}</p></div>
        <div className="border-t px-4 py-4 sm:border-t-0"><p className="text-xs text-muted-foreground">结算倍率</p><p className="mt-1 text-xl font-semibold">{multiplier}</p></div>
      </section>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative min-w-0 flex-1">
          <Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="搜索可用模型" className="pl-8" onChange={(event) => setQuery(event.target.value)} placeholder="搜索模型 ID、名称或提供商" value={query} />
        </div>
        <div className="flex gap-2">
          <Select onValueChange={(value) => void load(value)} value={selectedBillingGroupID}>
            <SelectTrigger aria-label="选择计费分组" className="min-w-44"><SelectValue placeholder="选择计费分组" /></SelectTrigger>
            <SelectContent>{billingGroups.map((group) => <SelectItem key={group.id} value={group.id}>{group.display_name} · {((group.effective_multiplier_bps ?? group.multiplier_bps) / 10_000).toFixed(4)}×</SelectItem>)}</SelectContent>
          </Select>
          <Select onValueChange={setProtocol} value={protocol}>
            <SelectTrigger aria-label="按兼容协议筛选" className="min-w-36"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="all">全部协议</SelectItem><SelectItem value="openai">OpenAI</SelectItem><SelectItem value="anthropic">Anthropic</SelectItem></SelectContent>
          </Select>
          <Button aria-label="刷新可用模型" disabled={loading} onClick={() => void load(selectedBillingGroupID)} size="icon" title="刷新可用模型" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
        </div>
      </div>

      {message ? <p className="border-y bg-background px-4 py-3 text-sm" role="status">{message}</p> : null}

      {loading ? <div className="grid gap-4 lg:grid-cols-2" role="status">{[0, 1].map((item) => <div className="h-72 animate-pulse rounded-lg border bg-muted/40" key={item} />)}</div> : null}
      {!loading && models.length > 0 ? <div className="grid items-start gap-5 lg:grid-cols-[14rem_minmax(0,1fr)]">
        <aside className="border-y py-3 lg:sticky lg:top-4" aria-label="按模型厂商筛选">
          <div className="flex items-center gap-2 px-2 pb-3 text-sm font-medium"><Factory className="size-4" aria-hidden="true" />模型厂商</div>
          <div className="flex gap-2 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible">
            <Button className="shrink-0 justify-between lg:w-full" onClick={() => setProvider("all")} size="sm" variant={provider === "all" ? "secondary" : "ghost"}><span>全部厂商</span><Badge variant="outline">{models.length}</Badge></Button>
            {providers.map((item) => <Button className="shrink-0 justify-between lg:w-full" key={item.name} onClick={() => setProvider(item.name)} size="sm" variant={provider === item.name ? "secondary" : "ghost"}><span className="truncate">{item.name}</span><Badge variant="outline">{item.count}</Badge></Button>)}
          </div>
        </aside>
        <div className="min-w-0">
          <div className="mb-3 flex items-center justify-between gap-3"><p className="text-sm font-medium">{filtered.length} 个模型</p>{provider !== "all" ? <Button onClick={() => setProvider("all")} size="sm" variant="ghost">清除厂商筛选</Button> : null}</div>
          {filtered.length === 0 ? <div className="border-y bg-background py-20 text-center"><p className="font-medium">没有匹配的模型</p><p className="mt-1 text-sm text-muted-foreground">调整搜索内容、厂商或协议筛选后再试。</p></div> : null}
          {filtered.length > 0 ? <div className="grid gap-4 xl:grid-cols-2">{filtered.map((model) => (
        <Card className="rounded-lg" key={`${model.id}-${model.protocol}`}>
          <CardHeader className="gap-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0"><CardTitle className="truncate text-base">{model.display_name}</CardTitle><p className="mt-1 text-xs text-muted-foreground">{model.provider_name} · {model.channel_count} 个渠道</p></div>
              <Badge variant="outline">{model.protocol === "anthropic" ? "Anthropic" : "OpenAI"}</Badge>
            </div>
            <div className="flex min-w-0 items-center gap-2 border-y py-2">
              <code className="min-w-0 flex-1 truncate text-xs">{model.id}</code>
              <Button aria-label={`复制模型 ID ${model.id}`} onClick={() => void copyModelID(model.id)} size="icon-sm" title="复制模型 ID" variant="ghost">{copiedID === model.id ? <Check /> : <Clipboard />}</Button>
            </div>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-2 gap-x-4 gap-y-3">
              {priceFields.map((field) => <div className="min-w-0" key={field.key}><dt className="text-xs text-muted-foreground">{field.label}</dt><dd className="mt-1 truncate font-medium tabular-nums">{formatPrice(model.prices[field.key])} <span className="text-xs font-normal text-muted-foreground">{field.unit}</span></dd></div>)}
            </dl>
          </CardContent>
        </Card>
          ))}</div> : null}
        </div>
      </div> : null}
      {!loading && models.length === 0 ? <div className="border-y bg-background py-20 text-center"><p className="font-medium">暂时没有可用模型</p><p className="mt-1 text-sm text-muted-foreground">模型路由启用后会显示在这里。</p></div> : null}
    </div>
  );
}
