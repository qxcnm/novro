import {
  ArrowRight,
  Braces,
  Code2,
  DatabaseZap,
  KeyRound,
  LineChart,
  Route,
  ShieldCheck,
} from "lucide-react";
import Link from "next/link";

import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { modelCatalog } from "@/lib/models";

const foundations = [
  { icon: Route, label: "统一入口", description: "用一个地址连接多家模型，减少重复适配。" },
  { icon: ShieldCheck, label: "权限可控", description: "集中管理成员身份、访问边界与账号状态。" },
  { icon: DatabaseZap, label: "调用清晰", description: "模型、价格与使用信息在同一处查看。" },
  { icon: KeyRound, label: "密钥隔离", description: "服务端保存凭据，避免密钥暴露在客户端。" },
];

const featuredModels = modelCatalog.filter((model) => model.featured);
const vendorCount = new Set(modelCatalog.map((model) => model.vendor)).size;

const overviewStats = [
  { label: "模型目录", value: `${modelCatalog.length} 款`, detail: "可公开查看能力与牌价", icon: DatabaseZap },
  { label: "模型厂商", value: `${vendorCount} 家`, detail: "Kimi、GLM、DeepSeek", icon: Route },
  { label: "兼容协议", value: "3 种", detail: "Chat / Responses / Messages", icon: Code2 },
  { label: "统一入口", value: "/v1", detail: "接入地址保持稳定", icon: ShieldCheck },
];

export default function Home() {
  return (
    <div className="min-h-screen bg-muted/30">
      <SiteHeader />
      <main>
        <section className="border-b bg-background">
          <div className="mx-auto w-full max-w-7xl px-5 py-10 sm:px-8 sm:py-14 lg:px-10 lg:py-16">
            <div className="flex flex-col gap-8 lg:flex-row lg:items-end lg:justify-between">
              <div className="max-w-3xl lg:min-w-0 lg:flex-1 lg:max-w-4xl">
                <Badge variant="outline">Novro Gateway · 客户主页</Badge>
                <h1 className="mt-5 max-w-none text-xl leading-tight font-semibold tracking-tight sm:text-2xl xl:text-3xl">
                  一个入口，连接你的模型工作流
                </h1>
                <p className="mt-5 max-w-2xl text-base leading-7 text-muted-foreground sm:text-lg sm:leading-8">
                  公开查看模型能力、官方牌价和接入方式。登录后，还可以在控制台管理 API Key、余额与每次调用。
                </p>
              </div>
              <div className="flex shrink-0 flex-wrap gap-3">
                <Button asChild size="lg">
                  <Link href="/login">进入控制台 <ArrowRight aria-hidden="true" /></Link>
                </Button>
                <Button asChild size="lg" variant="outline">
                  <Link href="/docs">查看接入文档</Link>
                </Button>
              </div>
            </div>

            <div className="mt-10 overflow-hidden rounded-lg border bg-background">
              <div className="flex flex-col gap-3 border-b px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6">
                <div>
                  <p className="font-semibold">平台概览</p>
                  <p className="mt-1 text-sm text-muted-foreground">面向客户公开的模型、协议与管理能力</p>
                </div>
                <Badge className="w-fit gap-1.5" variant="secondary"><span className="size-1.5 rounded-full bg-emerald-500" />可访问</Badge>
              </div>
              <div className="grid sm:grid-cols-2 lg:grid-cols-4">
                {overviewStats.map((item, index) => (
                  <div className={`px-5 py-5 sm:px-6 ${index > 0 ? "border-t sm:border-l sm:border-t-0" : ""} ${index === 2 ? "lg:border-l" : ""}`} key={item.label}>
                    <div className="flex items-center gap-2 text-sm text-muted-foreground"><item.icon aria-hidden="true" className="size-4" />{item.label}</div>
                    <p className="mt-3 text-2xl font-semibold tracking-tight">{item.value}</p>
                    <p className="mt-1 text-xs text-muted-foreground">{item.detail}</p>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section className="mx-auto w-full max-w-7xl px-5 py-8 sm:px-8 lg:px-10 lg:py-10">
          <div className="grid gap-5 lg:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.65fr)]">
            <Card className="overflow-hidden rounded-lg bg-background">
              <CardHeader className="border-b">
                <div className="flex items-start justify-between gap-4">
                  <div><CardTitle>模型与价格</CardTitle><CardDescription className="mt-1">先比较能力，再选择适合你的模型</CardDescription></div>
                  <Button asChild className="shrink-0" size="sm" variant="outline"><Link href="/models">全部模型 <ArrowRight aria-hidden="true" /></Link></Button>
                </div>
              </CardHeader>
              <CardContent className="p-0">
                <div className="grid divide-y sm:grid-cols-3 sm:divide-x sm:divide-y-0">
                  {featuredModels.map((item) => (
                    <div className="p-5 sm:p-6" key={item.id}>
                      <div className="flex items-center justify-between gap-3"><p className="font-semibold">{item.name}</p><Badge variant="outline">{item.contextLabel}</Badge></div>
                      <p className="mt-1 text-xs text-muted-foreground">{item.vendorName}</p>
                      <p className="mt-4 min-h-12 text-sm leading-6 text-muted-foreground">{item.description}</p>
                      <div className="mt-5 grid grid-cols-2 border-t pt-4 text-sm"><div><span className="text-xs text-muted-foreground">输入 / 百万 tokens</span><p className="mt-1 font-semibold">¥{item.pricing?.[0].input}</p></div><div className="border-l pl-3"><span className="text-xs text-muted-foreground">输出 / 百万 tokens</span><p className="mt-1 font-semibold">¥{item.pricing?.[0].output}</p></div></div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            <div className="grid gap-5">
              <Card className="rounded-lg bg-background">
                <CardHeader><CardTitle className="flex items-center gap-2 text-base"><LineChart aria-hidden="true" className="size-4" />接入状态</CardTitle><CardDescription>从公开信息开始，登录后查看个人数据</CardDescription></CardHeader>
                <CardContent className="space-y-4"><div className="flex items-center justify-between border-b pb-3 text-sm"><span className="text-muted-foreground">模型目录</span><span className="font-medium">已开放</span></div><div className="flex items-center justify-between border-b pb-3 text-sm"><span className="text-muted-foreground">API 文档</span><span className="font-medium">可直接阅读</span></div><div className="flex items-center justify-between text-sm"><span className="text-muted-foreground">余额与用量</span><span className="font-medium">登录后查看</span></div></CardContent>
              </Card>
              <Card className="rounded-lg bg-foreground text-background ring-0 dark:bg-card dark:text-card-foreground dark:ring-1 dark:ring-foreground/10">
                <CardHeader className="border-b border-background/15 dark:border-border"><CardTitle className="text-base">公告</CardTitle><CardDescription className="text-background/60 dark:text-muted-foreground">当前版本已开放模型目录与统一网关</CardDescription></CardHeader>
                <CardContent><p className="text-sm leading-6 text-background/75 dark:text-muted-foreground">管理员配置启用路由后，客户即可使用自己的 API Key 调用模型。</p><Button asChild className="mt-5" variant="secondary"><Link href="/docs">查看接入步骤 <ArrowRight aria-hidden="true" /></Link></Button></CardContent>
              </Card>
            </div>
          </div>
        </section>

        <section id="models" className="scroll-mt-16 border-t bg-background">
          <div className="mx-auto w-full max-w-7xl px-5 py-20 sm:px-8 lg:px-10 lg:py-28">
            <div className="grid gap-8 lg:grid-cols-[0.7fr_1.3fr] lg:gap-16">
              <div>
                <p className="text-sm font-medium text-muted-foreground">模型能力</p>
                <h2 className="mt-3 text-xl font-semibold sm:text-2xl xl:text-3xl">三家模型，一个接入面。</h2>
                <p className="mt-4 max-w-md text-base leading-7 text-muted-foreground">
                  在同一目录比较模型能力、上下文与官方价格，再按场景选择合适的模型。
                </p>
              </div>
              <div className="grid gap-4 md:grid-cols-3">
                {featuredModels.map((item) => (
                  <Card className="rounded-lg" key={item.name}>
                    <CardHeader>
                      <div className="flex items-center justify-between gap-3">
                        <CardTitle>{item.name}</CardTitle>
                        <Badge variant="outline">官方牌价</Badge>
                      </div>
                      <CardDescription>{item.vendorName}</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <p className="leading-6 text-muted-foreground">{item.description}</p>
                      <div className="mt-5 grid grid-cols-2 border-y py-3 text-sm">
                        <div><span className="text-muted-foreground">输入</span><p className="mt-1 font-semibold">¥{item.pricing?.[0].input}</p></div>
                        <div className="border-l pl-4"><span className="text-muted-foreground">输出</span><p className="mt-1 font-semibold">¥{item.pricing?.[0].output}</p></div>
                      </div>
                      <p className="mt-3 text-xs text-muted-foreground">人民币 / 百万 tokens · 厂商官方牌价</p>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </div>
            <Button asChild className="mt-8" variant="outline">
              <Link href="/models">查看全部模型、筛选与价格 <ArrowRight aria-hidden="true" /></Link>
            </Button>
          </div>
        </section>

        <section id="integration" className="scroll-mt-16 border-t">
          <div className="mx-auto grid w-full max-w-7xl gap-12 px-5 py-20 sm:px-8 lg:grid-cols-2 lg:items-center lg:gap-20 lg:px-10 lg:py-28">
            <div>
              <p className="text-sm font-medium text-muted-foreground">接入方式</p>
              <h2 className="mt-3 text-xl font-semibold sm:text-2xl xl:text-3xl">保留熟悉的客户端，替换一个地址。</h2>
              <p className="mt-5 max-w-xl text-base leading-7 text-muted-foreground">
                兼容 OpenAI Chat Completions、Responses 与 Anthropic Messages，现有应用无需重写调用方式。
              </p>
              <div className="mt-7 flex flex-wrap gap-2">
                <Badge variant="secondary"><Code2 aria-hidden="true" /> Chat Completions</Badge>
                <Badge variant="secondary"><Braces aria-hidden="true" /> Responses</Badge>
                <Badge variant="secondary"><Code2 aria-hidden="true" /> Messages</Badge>
              </div>
              <Button asChild className="mt-8" variant="outline">
                <Link href="/docs#api-examples">
                  查看完整接入示例
                  <ArrowRight aria-hidden="true" />
                </Link>
              </Button>
            </div>

            <Card className="rounded-lg bg-foreground text-background ring-0 dark:bg-card dark:text-card-foreground dark:ring-1 dark:ring-foreground/10">
              <CardHeader className="border-b border-background/15 dark:border-border">
                <CardTitle className="font-mono text-sm">novro.ts</CardTitle>
                <CardDescription className="text-background/60 dark:text-muted-foreground">统一调用方式</CardDescription>
              </CardHeader>
              <CardContent>
                <pre className="overflow-x-auto py-2 font-mono text-xs leading-6 sm:text-sm">
                  <code>{`const client = new OpenAI({
  baseURL: "https://api.example.com/v1",
  apiKey: process.env.NOVRO_API_KEY,
});

const response = await client.chat.completions.create({
  model: "glm",
  messages: [{ role: "user", content: "你好" }],
});`}</code>
                </pre>
              </CardContent>
            </Card>
          </div>
        </section>

        <section className="border-t bg-muted/30">
          <div className="mx-auto w-full max-w-7xl px-5 py-20 sm:px-8 lg:px-10 lg:py-24">
            <div className="max-w-2xl">
              <p className="text-sm font-medium text-muted-foreground">为团队而建</p>
              <h2 className="mt-3 text-xl font-semibold sm:text-2xl xl:text-3xl">把模型接入和日常管理放在一起。</h2>
            </div>
            <div className="mt-10 grid border-y sm:grid-cols-2 lg:grid-cols-4">
              {foundations.map((item, index) => (
                <div className={`py-7 sm:px-6 ${index % 2 !== 0 ? "sm:border-l" : ""} ${index > 0 ? "border-t sm:border-t-0" : ""} ${index === 2 ? "lg:border-l" : ""}`} key={item.label}>
                  <item.icon className="size-5" aria-hidden="true" />
                  <h3 className="mt-5 font-semibold">{item.label}</h3>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">{item.description}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="border-t">
          <div className="mx-auto flex w-full max-w-7xl flex-col gap-8 px-5 py-20 sm:px-8 lg:flex-row lg:items-end lg:justify-between lg:px-10 lg:py-24">
            <div>
              <p className="text-sm font-medium text-muted-foreground">Novro Gateway</p>
              <h2 className="mt-3 max-w-2xl text-xl font-semibold sm:text-2xl xl:text-3xl">用统一入口连接团队与模型。</h2>
            </div>
            <div className="flex flex-wrap gap-3">
              <Button asChild><Link href="/login">进入控制台</Link></Button>
              <Button asChild variant="outline"><Link href="/docs">阅读接入文档</Link></Button>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
