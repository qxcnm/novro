import {
  ArrowRight,
  Braces,
  Check,
  Code2,
  DatabaseZap,
  KeyRound,
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

const models = [
  {
    name: "Kimi",
    model: "K2.6",
    description: "原生多模态理解，面向代码与 Agent 工作流。",
    capability: "多模态 / Coding / Agent",
  },
  {
    name: "GLM",
    model: "GLM-5.2",
    description: "1M 长上下文，面向长程任务与工程交付。",
    capability: "长上下文 / 工具调用",
  },
  {
    name: "DeepSeek",
    model: "V4",
    description: "支持思考与非思考模式，兼容主流 API 格式。",
    capability: "推理 / OpenAI / Anthropic",
  },
];

const foundations = [
  { icon: Route, label: "统一入口", description: "用一个地址连接多家模型，减少重复适配。" },
  { icon: ShieldCheck, label: "权限可控", description: "集中管理成员身份、访问边界与账号状态。" },
  { icon: DatabaseZap, label: "调用清晰", description: "模型、价格与使用信息在同一处查看。" },
  { icon: KeyRound, label: "密钥隔离", description: "服务端保存凭据，避免密钥暴露在客户端。" },
];

const featuredModels = modelCatalog.filter((model) => model.featured);

export default function Home() {
  return (
    <div className="min-h-screen bg-background">
      <SiteHeader />
      <main>
        <section className="mx-auto flex min-h-[calc(100svh-4rem)] w-full max-w-7xl flex-col justify-between px-5 pt-12 sm:px-8 sm:pt-20 lg:px-10 lg:pt-24">
          <div className="grid gap-10 pb-10 sm:gap-12 sm:pb-16 lg:grid-cols-[minmax(0,1.3fr)_minmax(20rem,0.7fr)] lg:items-end lg:gap-20 lg:pb-20">
            <div>
              <Badge variant="outline">Novro Gateway</Badge>
              <h1 className="mt-6 max-w-3xl text-[2rem] leading-[1.18] font-semibold sm:text-5xl lg:text-[3.25rem]">
                统一接入主流国产模型
              </h1>
              <p className="mt-7 max-w-2xl text-base leading-7 text-muted-foreground sm:text-lg sm:leading-8">
                用一个地址连接 Kimi、GLM 与 DeepSeek，集中管理成员、权限、API Key 与用量。
              </p>
              <div className="mt-8 flex flex-wrap gap-3">
                <Button asChild size="lg">
                  <Link href="/login">
                    进入控制台
                    <ArrowRight aria-hidden="true" />
                  </Link>
                </Button>
                <Button asChild size="lg" variant="outline">
                  <Link href="/docs">查看接入文档</Link>
                </Button>
              </div>
            </div>

            <div className="border-l pl-6 sm:pl-8">
              <p className="text-sm font-medium text-muted-foreground">团队模型入口</p>
              <p className="mt-3 text-2xl font-semibold">接入更简单，管理更集中</p>
              <ul className="mt-5 space-y-3 text-sm text-muted-foreground">
                {[
                  "统一模型与兼容协议",
                  "集中用户与访问权限",
                  "清晰价格与接入文档",
                ].map((item) => (
                  <li className="flex items-center gap-2" key={item}>
                    <Check className="size-4 text-foreground" aria-hidden="true" />
                    {item}
                  </li>
                ))}
              </ul>
            </div>
          </div>

          <div className="grid border-t sm:grid-cols-3" aria-label="模型厂商">
            {models.map((item, index) => (
              <div className={`py-5 sm:px-6 ${index === 0 ? "sm:pl-0" : "border-t sm:border-t-0 sm:border-l"}`} key={item.name}>
                <div className="flex items-center justify-between gap-3">
                  <p className="font-semibold">{item.name}</p>
                  <Badge variant="secondary">{item.model}</Badge>
                </div>
                <p className="mt-1 text-sm text-muted-foreground">{item.capability}</p>
              </div>
            ))}
          </div>
        </section>

        <section id="models" className="scroll-mt-16 border-t bg-muted/30">
          <div className="mx-auto w-full max-w-7xl px-5 py-20 sm:px-8 lg:px-10 lg:py-28">
            <div className="grid gap-8 lg:grid-cols-[0.7fr_1.3fr] lg:gap-16">
              <div>
                <p className="text-sm font-medium text-muted-foreground">模型能力</p>
                <h2 className="mt-3 text-3xl font-semibold sm:text-4xl">三家模型，一个接入面。</h2>
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
              <h2 className="mt-3 text-3xl font-semibold sm:text-4xl">保留熟悉的客户端，替换一个地址。</h2>
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
              <h2 className="mt-3 text-3xl font-semibold sm:text-4xl">把模型接入和日常管理放在一起。</h2>
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
              <h2 className="mt-3 max-w-2xl text-3xl font-semibold sm:text-4xl">用统一入口连接团队与模型。</h2>
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
