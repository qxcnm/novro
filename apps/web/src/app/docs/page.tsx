import type { Metadata } from "next";
import {
  AlertCircle,
  ArrowRight,
  Braces,
  CheckCircle2,
  CircleDashed,
  ExternalLink,
  KeyRound,
  Radio,
  RefreshCw,
  ShieldCheck,
  Terminal,
  Wrench,
} from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { DocsFrame } from "@/components/docs-frame";
import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export const metadata: Metadata = {
  title: "API 接入文档",
  description: "Novro Gateway API Key、兼容协议、流式响应、工具调用与错误处理接入说明。",
};

const sections = [
  { href: "#quick-start", label: "快速开始" },
  { href: "#authentication", label: "鉴权与地址" },
  { href: "#models", label: "模型选择" },
  { href: "#api-examples", label: "调用示例" },
  { href: "#streaming", label: "流式响应" },
  { href: "#tools", label: "工具调用" },
  { href: "#structured-output", label: "结构化输出" },
  { href: "#errors", label: "错误与重试" },
  { href: "#security", label: "安全建议" },
];

const curlChat = `curl https://api.example.invalid/v1/chat/completions \\
  -H "Authorization: Bearer $NOVRO_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "glm-5.2",
    "messages": [
      {"role": "system", "content": "回答简洁、准确。"},
      {"role": "user", "content": "用三点说明什么是 RAG。"}
    ]
  }'`;

const pythonChat = `import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["NOVRO_API_KEY"],
    base_url="https://api.example.invalid/v1",
)

response = client.chat.completions.create(
    model="glm-5.2",
    messages=[
        {"role": "system", "content": "回答简洁、准确。"},
        {"role": "user", "content": "用三点说明什么是 RAG。"},
    ],
)

print(response.choices[0].message.content)`;

const nodeChat = `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: process.env.NOVRO_API_KEY,
  baseURL: "https://api.example.invalid/v1",
});

const response = await client.chat.completions.create({
  model: "glm-5.2",
  messages: [
    { role: "system", content: "回答简洁、准确。" },
    { role: "user", content: "用三点说明什么是 RAG。" },
  ],
});

console.log(response.choices[0].message.content);`;

const responsesExample = `const response = await client.responses.create({
  model: "deepseek-v4-flash",
  input: "提取这段内容的关键事实，并给出来源位置。",
});

console.log(response.output_text);`;

const anthropicExample = `import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic({
  apiKey: process.env.NOVRO_API_KEY,
  baseURL: "https://api.example.invalid",
});

const message = await client.messages.create({
  model: "kimi-k3",
  max_tokens: 2048,
  messages: [{ role: "user", content: "分析这段代码的风险。" }],
});

console.log(message.content);`;

const streamingExample = `const stream = await client.chat.completions.create({
  model: "deepseek-v4-flash",
  messages: [{ role: "user", content: "写一份发布检查清单。" }],
  stream: true,
});

for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content ?? "");
}`;

const toolsExample = `const tools = [{
  type: "function",
  function: {
    name: "get_weather",
    description: "查询指定城市的天气",
    parameters: {
      type: "object",
      properties: {
        city: { type: "string", description: "城市名称" }
      },
      required: ["city"],
      additionalProperties: false
    }
  }
}];

const first = await client.chat.completions.create({
  model: "glm-5.2",
  messages: [{ role: "user", content: "北京今天适合骑车吗？" }],
  tools,
});

// 执行本地函数后，把 tool_call_id 和结果作为 tool 消息回传。
const call = first.choices[0].message.tool_calls?.[0];`;

const structuredExample = `const response = await client.chat.completions.create({
  model: "kimi-k3",
  messages: [{ role: "user", content: "提取订单号和总金额。" }],
  response_format: {
    type: "json_schema",
    json_schema: {
      name: "order",
      strict: true,
      schema: {
        type: "object",
        properties: {
          order_id: { type: "string" },
          total: { type: "number" }
        },
        required: ["order_id", "total"],
        additionalProperties: false
      }
    }
  }
});`;

const errors = [
  ["400", "invalid_request / unsupported_endpoint", "请求字段、参数或 API 协议不合法", "修正请求后再发，不要原样重试"],
  ["401", "invalid_api_key", "API Key 缺失、无效或已撤销", "检查服务端环境中的 Key"],
  ["402", "insufficient_balance", "账户余额不足，网关未调用上游", "调整余额后再重试"],
  ["404", "not_found / model_not_found", "路径或模型标识不存在", "检查 Base URL、路径和模型 ID"],
  ["500", "internal_error / billing_error", "网关内部或计费记录错误", "记录 request_id 后有限重试"],
  ["502", "upstream_unavailable / upstream_error", "上游暂时不可用或响应无效", "指数退避并设置最大重试次数"],
];

export function ApiDocumentation() {
  return (
    <DocsFrame publicFooter={<SiteFooter />} publicHeader={<SiteHeader />}>
          <aside className="hidden lg:block">
            <nav className="sticky top-24 space-y-1" aria-label="接入文档目录">
              <p className="mb-3 px-2 text-xs font-medium text-muted-foreground">API 接入文档</p>
              {sections.map((section) => (
                <Button asChild className="w-full justify-start" key={section.href} variant="ghost">
                  <a href={section.href}>{section.label}</a>
                </Button>
              ))}
            </nav>
          </aside>

          <article className="min-w-0 max-w-4xl">
            <section id="quick-start" className="scroll-mt-24">
              <Badge variant="outline">接入中心</Badge>
              <h1 className="mt-5 text-xl font-semibold sm:text-2xl xl:text-3xl">Novro API 接入文档</h1>
              <p className="mt-5 max-w-3xl text-lg leading-8 text-muted-foreground">
                用 OpenAI 或 Anthropic 客户端接入 Kimi、GLM 与 DeepSeek。正式上线后只需配置一个 API Key 和一个 Base URL。
              </p>

              <Alert className="mt-8">
                <CircleDashed aria-hidden="true" />
                <AlertTitle>模型网关已开放</AlertTitle>
                <AlertDescription>
                  管理员完成提供商和模型路由配置后，API Key、余额和 <code>/v1</code> 模型请求即可使用。示例域名 <code>api.example.invalid</code> 不是生产地址，请替换为部署地址。
                </AlertDescription>
              </Alert>

              <div className="mt-8 grid gap-4 sm:grid-cols-3">
                <Step icon={KeyRound} number="01" title="创建 API Key">上线后在控制台创建；密钥首次展示后仍可在 Key 列表中重新复制。</Step>
                <Step icon={Braces} number="02" title="选择协议">按现有客户端选择 Chat、Responses 或 Messages。</Step>
                <Step icon={ArrowRight} number="03" title="发送请求">替换 Base URL 和模型 ID，保留熟悉的 SDK。</Step>
              </div>
            </section>

            <Separator className="my-14" />

            <section id="authentication" className="scroll-mt-24">
              <SectionHeading eyebrow="01 · 基础信息" title="鉴权与请求地址">
                API Key 只放在服务端环境变量中，并通过 Bearer 认证头发送。不要把 Key 写入浏览器代码、移动端安装包、公开仓库或日志。
              </SectionHeading>
              <div className="mt-7 overflow-hidden rounded-lg border">
                <InfoRow label="示例 Base URL"><code>https://api.example.invalid/v1</code></InfoRow>
                <InfoRow label="鉴权"><code>Authorization: Bearer nvr_xxx</code></InfoRow>
                <InfoRow label="请求格式"><code>Content-Type: application/json</code></InfoRow>
                <InfoRow label="请求追踪">响应头或错误体中的 <code>request_id</code></InfoRow>
              </div>
              <CodeBlock label="环境变量">{`NOVRO_API_KEY=nvr_your_api_key`}</CodeBlock>
            </section>

            <Separator className="my-14" />

            <section id="models" className="scroll-mt-24">
              <SectionHeading eyebrow="02 · 模型选择" title="使用稳定的模型标识">
                请求中的 <code>model</code> 决定具体能力和厂商。上下文、输入类型、官方价格和规划状态在模型目录集中维护。
              </SectionHeading>
              <div className="mt-7 grid gap-3 sm:grid-cols-3">
                {[
                  ["glm-5.2", "1M 长上下文与工程任务"],
                  ["deepseek-v4-flash", "高吞吐与 Responses"],
                  ["kimi-k3", "长程 Coding 与多模态"],
                ].map(([model, description]) => (
                  <Card className="rounded-lg" size="sm" key={model}>
                    <CardHeader><CardTitle className="font-mono text-xs">{model}</CardTitle></CardHeader>
                    <CardContent className="text-sm text-muted-foreground">{description}</CardContent>
                  </Card>
                ))}
              </div>
              <Button asChild className="mt-6" variant="outline">
                <Link href="/models">查看全部模型和官方价格 <ArrowRight aria-hidden="true" /></Link>
              </Button>
            </section>

            <Separator className="my-14" />

            <section id="api-examples" className="scroll-mt-24">
              <SectionHeading eyebrow="03 · API 示例" title="选择你已有的客户端">
                Chat Completions 适合现有聊天 SDK；Responses 适合新应用；Messages 用于 Anthropic 生态客户端。字段支持度将随模型而异。
              </SectionHeading>

              <h3 className="mt-8 text-xl font-semibold">OpenAI Chat Completions</h3>
              <p className="mt-3 leading-7 text-muted-foreground"><code>POST /v1/chat/completions</code>，支持消息列表、非流式与 SSE 流式响应。</p>
              <Tabs defaultValue="curl" className="mt-5">
                <TabsList aria-label="Chat Completions 示例语言">
                  <TabsTrigger value="curl">cURL</TabsTrigger>
                  <TabsTrigger value="python">Python</TabsTrigger>
                  <TabsTrigger value="node">Node.js</TabsTrigger>
                </TabsList>
                <TabsContent value="curl"><CodeBlock label="cURL" flush>{curlChat}</CodeBlock></TabsContent>
                <TabsContent value="python"><CodeBlock label="Python" flush>{pythonChat}</CodeBlock></TabsContent>
                <TabsContent value="node"><CodeBlock label="TypeScript" flush>{nodeChat}</CodeBlock></TabsContent>
              </Tabs>

              <h3 className="mt-10 text-xl font-semibold">OpenAI Responses</h3>
              <p className="mt-3 leading-7 text-muted-foreground"><code>POST /v1/responses</code>，转发文本输入、流式输出和基本工具调用字段，能力取决于上游模型。</p>
              <CodeBlock label="TypeScript">{responsesExample}</CodeBlock>

              <h3 className="mt-10 text-xl font-semibold">Anthropic Messages</h3>
              <p className="mt-3 leading-7 text-muted-foreground"><code>POST /v1/messages</code>，使用复数路径；Anthropic SDK 的 Base URL 不带末尾 <code>/v1</code>。</p>
              <CodeBlock label="TypeScript">{anthropicExample}</CodeBlock>
            </section>

            <Separator className="my-14" />

            <section id="streaming" className="scroll-mt-24">
              <SectionHeading eyebrow="04 · 流式响应" title="用 SSE 持续接收增量">
                请求设置 <code>stream: true</code> 后，客户端逐块处理输出。连接可能中断，应用应保留已接收内容并给用户明确的重试入口。
              </SectionHeading>
              <CodeBlock icon={Radio} label="TypeScript">{streamingExample}</CodeBlock>
              <Alert className="mt-5">
                <RefreshCw aria-hidden="true" />
                <AlertTitle>不要盲目续传</AlertTitle>
                <AlertDescription>只有在尚未收到任何内容时才自动重试。已经向用户展示部分内容后，建议由用户确认重试，避免重复答案或重复执行工具。</AlertDescription>
              </Alert>
            </section>

            <Separator className="my-14" />

            <section id="tools" className="scroll-mt-24">
              <SectionHeading eyebrow="05 · 工具调用" title="模型决定调用，应用负责执行">
                先把函数定义放入 <code>tools</code>；模型返回 <code>tool_calls</code> 后，由你的服务校验参数、执行本地函数，再把结果回传给模型。模型永远不应直接获得数据库或系统权限。
              </SectionHeading>
              <CodeBlock icon={Wrench} label="TypeScript">{toolsExample}</CodeBlock>
              <div className="mt-6 grid gap-3 sm:grid-cols-3">
                {["校验函数名与参数", "为工具设置超时", "高风险操作二次确认"].map((item) => (
                  <div className="flex items-center gap-2 border-y py-3 text-sm" key={item}>
                    <CheckCircle2 className="size-4 shrink-0" aria-hidden="true" />{item}
                  </div>
                ))}
              </div>
            </section>

            <Separator className="my-14" />

            <section id="structured-output" className="scroll-mt-24">
              <SectionHeading eyebrow="06 · 结构化输出" title="优先使用 JSON Schema">
                结构化数据场景优先使用 <code>json_schema</code>；仅要求合法 JSON 时使用 <code>json_object</code>。客户端仍需校验响应，不能把模型输出直接写入数据库或拼接 SQL。
              </SectionHeading>
              <CodeBlock icon={Braces} label="TypeScript">{structuredExample}</CodeBlock>
            </section>

            <Separator className="my-14" />

            <section id="errors" className="scroll-mt-24">
              <SectionHeading eyebrow="07 · 错误处理" title="按错误类型决定是否重试">
                错误响应使用稳定的 <code>error.code</code>、固定的 <code>error.type</code>、面向用户的 <code>error.message</code> 和可追踪的 <code>request_id</code>。程序应根据状态码和错误代码分支，不要依赖文案。
              </SectionHeading>
              <div className="mt-7 overflow-x-auto rounded-lg border">
                <table className="w-full min-w-[44rem] text-left text-sm">
                  <thead className="border-b bg-muted/50 text-xs text-muted-foreground">
                    <tr><th className="px-4 py-3">状态</th><th className="px-4 py-3">错误代码</th><th className="px-4 py-3">含义</th><th className="px-4 py-3">处理</th></tr>
                  </thead>
                  <tbody className="divide-y">
                    {errors.map(([status, code, meaning, action]) => (
                      <tr key={status}><td className="px-4 py-3 font-mono text-xs">{status}</td><td className="px-4 py-3 font-mono text-xs">{code}</td><td className="px-4 py-3 text-muted-foreground">{meaning}</td><td className="px-4 py-3 text-muted-foreground">{action}</td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <Accordion type="single" collapsible className="mt-6">
                <AccordionItem value="retry">
                  <AccordionTrigger>推荐的重试策略</AccordionTrigger>
                  <AccordionContent className="leading-6 text-muted-foreground">只重试 500 和 502。使用带随机抖动的指数退避，例如 1 秒、2 秒、4 秒，最多 3 次，并为整个请求设置总超时。余额不足、鉴权失败、请求参数错误和模型不存在不应自动重试。</AccordionContent>
                </AccordionItem>
                <AccordionItem value="request-id">
                  <AccordionTrigger>排查问题需要保留什么</AccordionTrigger>
                  <AccordionContent className="leading-6 text-muted-foreground">记录时间、HTTP 状态、模型 ID、客户端版本和 <code>request_id</code>。不要记录完整提示词、Authorization 头、API Key 或可能包含敏感信息的模型输出。</AccordionContent>
                </AccordionItem>
              </Accordion>
            </section>

            <Separator className="my-14" />

            <section id="security" className="scroll-mt-24">
              <SectionHeading eyebrow="08 · 安全" title="把 API Key 当作生产密码">
                每个服务使用独立 Key，定期轮换；怀疑泄露时先撤销再排查。浏览器前端应调用你自己的后端，由后端访问 Novro。
              </SectionHeading>
              <div className="mt-7 divide-y border-y">
                {[
                  ["仅服务端使用", "不要放进 NEXT_PUBLIC_*、网页脚本或客户端安装包"],
                  ["最小暴露", "日志和错误追踪中脱敏 Authorization、Cookie 与敏感提示词"],
                  ["限制工具权限", "模型生成的参数必须经过白名单、Schema 和业务权限校验"],
                  ["控制成本", "设置请求超时、最大输出、并发限制和异常用量告警"],
                ].map(([title, description]) => (
                  <div className="flex gap-3 py-4" key={title}>
                    <ShieldCheck className="mt-0.5 size-5 shrink-0" aria-hidden="true" />
                    <div><p className="font-medium">{title}</p><p className="mt-1 text-sm leading-6 text-muted-foreground">{description}</p></div>
                  </div>
                ))}
              </div>
              <Alert className="mt-8" variant="destructive">
                <AlertCircle aria-hidden="true" />
                <AlertTitle>不要提交真实密钥</AlertTitle>
                <AlertDescription>示例只使用变量名和占位符。发现密钥进入 Git 历史后，删除文本并不足够，必须立即撤销并重新创建。</AlertDescription>
              </Alert>
            </section>

            <Separator className="my-14" />

            <section aria-labelledby="official-resources">
              <h2 id="official-resources" className="text-2xl font-semibold">模型厂商资料</h2>
              <p className="mt-3 leading-7 text-muted-foreground">模型能力和牌价以厂商页面为准，Novro 模型目录记录最近一次核验值。</p>
              <div className="mt-5 flex flex-wrap gap-3">
                {[
                  ["Kimi 官方定价", "https://platform.kimi.com/docs/pricing/chat"],
                  ["GLM-5.2 官方文档", "https://docs.bigmodel.cn/cn/guide/models/text/glm-5.2"],
                  ["DeepSeek 官方定价", "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/"],
                ].map(([label, href]) => (
                  <Button asChild variant="outline" key={href}><a href={href} target="_blank" rel="noreferrer">{label}<ExternalLink aria-hidden="true" /></a></Button>
                ))}
              </div>
            </section>
          </article>
    </DocsFrame>
  );
}

export default function DocsPage() {
  return <ApiDocumentation />;
}

function SectionHeading({ eyebrow, title, children }: { eyebrow: string; title: string; children: ReactNode }) {
  return <><p className="text-sm font-medium text-muted-foreground">{eyebrow}</p><h2 className="mt-3 text-3xl font-semibold">{title}</h2><p className="mt-4 leading-7 text-muted-foreground">{children}</p></>;
}

function Step({ icon: Icon, number, title, children }: { icon: typeof KeyRound; number: string; title: string; children: ReactNode }) {
  return <Card className="rounded-lg" size="sm"><CardHeader><div className="flex items-center justify-between"><Icon className="size-5" aria-hidden="true" /><span className="font-mono text-xs text-muted-foreground">{number}</span></div><CardTitle className="mt-3">{title}</CardTitle></CardHeader><CardContent className="leading-6 text-muted-foreground">{children}</CardContent></Card>;
}

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return <div className="grid gap-2 border-b px-4 py-4 last:border-b-0 sm:grid-cols-[10rem_minmax(0,1fr)]"><span className="text-sm text-muted-foreground">{label}</span><span className="min-w-0 break-all text-sm">{children}</span></div>;
}

function CodeBlock({ children, label, icon: Icon = Terminal, flush = false }: { children: string; label: string; icon?: typeof Terminal; flush?: boolean }) {
  return (
    <Card className={`${flush ? "mt-0" : "mt-6"} rounded-lg bg-foreground text-background ring-0 dark:bg-card dark:text-card-foreground dark:ring-1 dark:ring-foreground/10`}>
      <CardHeader className="border-b border-background/15 py-3 dark:border-border"><CardTitle className="flex items-center gap-2 font-mono text-xs"><Icon className="size-3.5" aria-hidden="true" />{label}</CardTitle></CardHeader>
      <CardContent><pre className="overflow-x-auto py-1 font-mono text-xs leading-6 sm:text-sm"><code>{children}</code></pre></CardContent>
    </Card>
  );
}
