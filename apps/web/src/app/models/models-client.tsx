"use client";

import {
  ArrowUpDown,
  ExternalLink,
  FileText,
  ImageIcon,
  Search,
  Video,
} from "lucide-react";
import Image from "next/image";
import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  compareModelRelease,
  formatPrice,
  getStartingPrice,
  type ModelEntry,
  type ModelVendor,
} from "@/lib/models";

type ModelsClientProps = {
  initialModels: ModelEntry[];
};

const vendorOptions: Array<{ value: "all" | ModelVendor; label: string }> = [
  { value: "all", label: "全部厂商" },
  { value: "zhipu", label: "智谱 GLM" },
  { value: "deepseek", label: "DeepSeek" },
  { value: "moonshot", label: "Moonshot Kimi" },
];

const vendorLogos: Record<ModelVendor, { src: string; className: string }> = {
  zhipu: { src: "/brands/zhipu.png", className: "size-8 rounded-md" },
  deepseek: { src: "/brands/deepseek.png", className: "size-8" },
  moonshot: { src: "/brands/kimi.webp", className: "size-8 rounded-md" },
};

const inputIcons = {
  文本: FileText,
  图片: ImageIcon,
  视频: Video,
};

export function ModelsClient({ initialModels }: ModelsClientProps) {
  const [query, setQuery] = useState("");
  const [vendor, setVendor] = useState<"all" | ModelVendor>("all");
  const [context, setContext] = useState("all");
  const [sort, setSort] = useState("release");

  const models = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    const filtered = initialModels.filter((model) => {
      const matchesQuery =
        !normalizedQuery ||
        [model.name, model.vendorName, model.description, ...model.capabilities]
          .join(" ")
          .toLocaleLowerCase()
          .includes(normalizedQuery);
      const matchesVendor = vendor === "all" || model.vendor === vendor;
      const matchesContext =
        context === "all" ||
        (context === "million" && model.contextTokens >= 1_000_000) ||
        (context === "long" && model.contextTokens >= 200_000 && model.contextTokens < 1_000_000);

      return matchesQuery && matchesVendor && matchesContext;
    });

    return filtered.toSorted((left, right) => {
      if (sort === "price") {
        const leftPrice = getStartingPrice(left, "input") ?? Number.POSITIVE_INFINITY;
        const rightPrice = getStartingPrice(right, "input") ?? Number.POSITIVE_INFINITY;
        return leftPrice - rightPrice;
      }
      if (sort === "context") {
        return right.contextTokens - left.contextTokens;
      }
      if (sort === "name") {
        return left.name.localeCompare(right.name, "zh-CN");
      }
      if (sort === "release") {
        return compareModelRelease(left, right);
      }
      return initialModels.indexOf(left) - initialModels.indexOf(right);
    });
  }, [context, initialModels, query, sort, vendor]);

  return (
    <section className="mx-auto w-full max-w-7xl px-5 py-7 sm:px-8 lg:px-10 lg:py-8">
      <div className="grid gap-3 lg:grid-cols-[minmax(16rem,1fr)_11rem_10rem_10rem]">
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          <Input
            className="h-10 bg-background pl-9"
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索模型、厂商或能力"
            aria-label="搜索模型"
          />
        </div>
        <Select value={vendor} onValueChange={(value) => setVendor(value as "all" | ModelVendor)}>
          <SelectTrigger className="h-10 w-full bg-background" aria-label="按厂商筛选">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {vendorOptions.map((option) => (
              <SelectItem value={option.value} key={option.value}>{option.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={context} onValueChange={setContext}>
          <SelectTrigger className="h-10 w-full bg-background" aria-label="按上下文筛选">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部上下文</SelectItem>
            <SelectItem value="long">200K-256K</SelectItem>
            <SelectItem value="million">1M</SelectItem>
          </SelectContent>
        </Select>
        <Select value={sort} onValueChange={setSort}>
          <SelectTrigger className="h-10 w-full bg-background" aria-label="模型排序">
            <ArrowUpDown aria-hidden="true" />
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="release">发布时间</SelectItem>
            <SelectItem value="price">输入价优先</SelectItem>
            <SelectItem value="context">上下文优先</SelectItem>
            <SelectItem value="name">名称排序</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="mt-6 flex items-center justify-between gap-4 border-y py-3 text-sm text-muted-foreground">
        <span>{models.length} / {initialModels.length} 款</span>
        <span>全部状态：即将开放</span>
      </div>

      {models.length ? (
        <div className="mt-6 grid gap-4 lg:grid-cols-2">
          {models.map((model) => <ModelCard model={model} key={model.id} />)}
        </div>
      ) : (
        <div className="py-24 text-center">
          <p className="font-medium">没有匹配的模型</p>
          <p className="mt-2 text-sm text-muted-foreground">调整搜索内容或筛选条件后再试。</p>
        </div>
      )}
    </section>
  );
}

function ModelCard({ model }: { model: ModelEntry }) {
  const vendorLogo = vendorLogos[model.vendor];
  const input = getStartingPrice(model, "input");
  const output = getStartingPrice(model, "output");
  const cachedInput = getStartingPrice(model, "cachedInput");
  const hasTieredPricing = (model.pricing?.length ?? 0) > 1;

  return (
    <Card className="rounded-lg bg-background">
      <CardHeader className="gap-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md border bg-white">
              <Image
                alt=""
                aria-hidden="true"
                className={vendorLogo.className}
                height={32}
                src={vendorLogo.src}
                width={32}
              />
            </span>
            <div className="min-w-0">
              <CardTitle className="text-lg">{model.name}</CardTitle>
              <p className="mt-1 text-xs text-muted-foreground">
                {model.vendorName} · {model.id}
                {model.releasedAt ? ` · ${model.releasedAt} 发布` : ""}
              </p>
            </div>
          </div>
          <Badge variant={model.officialModel ? "outline" : "secondary"}>
            {model.officialModel ? "官方模型" : "Novro 别名"}
          </Badge>
        </div>
        <p className="min-h-12 text-sm leading-6 text-muted-foreground">{model.description}</p>
      </CardHeader>

      <CardContent>
        <div className="grid grid-cols-2 border-y">
          <Price label="输入" value={input} suffix={hasTieredPricing ? " 起" : ""} />
          <Price label="输出" value={output} suffix={hasTieredPricing ? " 起" : ""} bordered />
        </div>
        <div className="grid grid-cols-2 border-b sm:grid-cols-4">
          <Stat label="缓存命中" value={cachedInput === null ? "待定" : `¥${formatPrice(cachedInput)}`} />
          <Stat label="上下文" value={model.contextLabel} bordered />
          <Stat label="最大输出" value={model.maxOutputLabel} className="border-t sm:border-t-0 sm:border-l" />
          <div className="border-t px-3 py-3 sm:border-t-0 sm:border-l">
            <p className="text-xs text-muted-foreground">输入类型</p>
            <div className="mt-2 flex items-center gap-2" aria-label={model.inputTypes.join("、")}>
              {model.inputTypes.map((type) => {
                const Icon = inputIcons[type];
                return <Icon className="size-4" aria-hidden="true" key={type} />;
              })}
            </div>
          </div>
        </div>

        {model.pricing && model.pricing.length > 1 && (
          <div className="mt-4 space-y-1 text-xs leading-5 text-muted-foreground">
            {model.pricing.map((tier) => (
              <p key={tier.label}>{tier.label}：缓存 ¥{formatPrice(tier.cachedInput)} / 输入 ¥{formatPrice(tier.input)} / 输出 ¥{formatPrice(tier.output)}</p>
            ))}
          </div>
        )}

        <div className="mt-4 flex flex-wrap gap-2">
          {model.capabilities.map((capability) => (
            <Badge variant="secondary" key={capability}>{capability}</Badge>
          ))}
        </div>

        <div className="mt-5 flex items-center justify-between gap-4 text-xs text-muted-foreground">
          <span>Novro 状态：即将开放</span>
          <a
            className="inline-flex items-center gap-1 font-medium text-foreground underline-offset-4 hover:underline"
            href={model.officialSource}
            target="_blank"
            rel="noreferrer"
          >
            {model.officialSourceLabel}
            <ExternalLink className="size-3" aria-hidden="true" />
          </a>
        </div>
      </CardContent>
    </Card>
  );
}

function Price({ label, value, suffix = "", bordered = false }: { label: string; value: number | null; suffix?: string; bordered?: boolean }) {
  return (
    <div className={`px-3 py-4 ${bordered ? "border-l" : ""}`}>
      <p className="text-xs text-muted-foreground">官方{label}价</p>
      {value === null ? (
        <p className="mt-1 font-medium">Novro 待定</p>
      ) : (
        <p className="mt-1 text-lg font-semibold">¥{formatPrice(value)}{suffix}<span className="ml-1 text-xs font-normal text-muted-foreground">/ 百万 tokens</span></p>
      )}
    </div>
  );
}

function Stat({ label, value, bordered = false, className = "" }: { label: string; value: string; bordered?: boolean; className?: string }) {
  return (
    <div className={`px-3 py-3 ${bordered ? "border-l" : ""} ${className}`}>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 text-sm font-medium">{value}</p>
    </div>
  );
}
